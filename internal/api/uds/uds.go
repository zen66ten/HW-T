// Package uds serves the daemon's Unix-socket API: newline-delimited JSON
// request/response plus a subscription stream. This is the primary local
// SDK surface (the HWiNFO Shared Memory Interface analog); pkg/client wraps
// the wire protocol.
package uds

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"os"
	"os/user"
	"strconv"
	"time"

	"github.com/zen66ten/HW-T/internal/core"
)

// Request is one client command.
type Request struct {
	Op         string   `json:"op"` // sensors | devices | history | reset | subscribe | alerts | log_start | log_stop | log_mark | log_status | report
	ID         string   `json:"id,omitempty"`
	IntervalMs int      `json:"interval_ms,omitempty"`
	Path       string   `json:"path,omitempty"`     // log_start
	Format     string   `json:"format,omitempty"`   // log_start: csv | ndjson; report: text | html | json | yaml | csv
	Note       string   `json:"note,omitempty"`     // log_mark
	Sensors    []string `json:"sensors,omitempty"`  // log_start: column subset
	Sections   []string `json:"sections,omitempty"` // report: provider filter
	Redact     bool     `json:"redact,omitempty"`   // report: hide serials/UUIDs/MACs
}

// LogStatus reports the logging state.
type LogStatus struct {
	Active bool   `json:"active"`
	Path   string `json:"path,omitempty"`
	Format string `json:"format,omitempty"`
}

// Response is the answer to a request, or one subscription event.
type Response struct {
	OK          bool               `json:"ok"`
	Error       string             `json:"error,omitempty"`
	Event       string             `json:"event,omitempty"`
	Sensors     []core.Sensor      `json:"sensors,omitempty"`
	Devices     []core.Device      `json:"devices,omitempty"`
	History     []core.Point       `json:"history,omitempty"`
	Quarantined map[string]string  `json:"quarantined,omitempty"`
	Alerts      []core.AlertStatus `json:"alerts,omitempty"`
	Log         *LogStatus         `json:"log,omitempty"`
	Report      string             `json:"report,omitempty"`
}

// Server answers requests from the registry. Logger and Alerts are
// optional; ops touching an absent subsystem return an error.
type Server struct {
	reg    *core.Registry
	ln     net.Listener
	Logger *core.Logger
	Alerts *core.AlertEngine
	// DefaultLogPath is used by log_start when the client gives no path.
	DefaultLogPath string
}

// Listen binds the socket, replacing a stale one, and applies the §9
// permission model: group "hwt" with 0660 when the group exists, otherwise
// 0666 with a warning (single-user dev setups).
func Listen(path string, reg *core.Registry) (*Server, error) {
	_ = os.Remove(path)
	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	mode := os.FileMode(0o666)
	if g, err := user.LookupGroup("hwt"); err == nil {
		if gid, err := strconv.Atoi(g.Gid); err == nil && os.Chown(path, -1, gid) == nil {
			mode = 0o660
		}
	} else {
		slog.Warn("group 'hwt' not found; socket is world-accessible", "socket", path)
	}
	if err := os.Chmod(path, mode); err != nil {
		ln.Close()
		return nil, err
	}
	return &Server{reg: reg, ln: ln}, nil
}

// Serve accepts connections until ctx is cancelled.
func (s *Server) Serve(ctx context.Context) error {
	go func() {
		<-ctx.Done()
		s.ln.Close()
	}()
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			return err
		}
		go s.handle(ctx, conn)
	}
}

func (s *Server) handle(ctx context.Context, conn net.Conn) {
	defer conn.Close()
	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	enc := json.NewEncoder(conn)

	for scanner.Scan() {
		var req Request
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			enc.Encode(Response{Error: "bad request: " + err.Error()})
			return
		}
		if req.Op == "subscribe" {
			s.stream(ctx, enc, req)
			return // subscription takes over the connection
		}
		if err := enc.Encode(s.respond(req)); err != nil {
			return
		}
	}
}

func (s *Server) respond(req Request) Response {
	switch req.Op {
	case "sensors":
		return Response{OK: true, Sensors: s.reg.Snapshot(), Quarantined: s.reg.Quarantined()}
	case "devices":
		return Response{OK: true, Devices: s.reg.Devices()}
	case "history":
		pts, ok := s.reg.History(core.SensorID(req.ID))
		if !ok {
			return Response{Error: "unknown sensor: " + req.ID}
		}
		return Response{OK: true, History: pts}
	case "reset":
		if !s.reg.Reset(core.SensorID(req.ID)) {
			return Response{Error: "unknown sensor: " + req.ID}
		}
		return Response{OK: true}
	case "alerts":
		if s.Alerts == nil {
			return Response{OK: true} // no rules configured
		}
		return Response{OK: true, Alerts: s.Alerts.Statuses()}
	case "log_start":
		if s.Logger == nil {
			return Response{Error: "logging not available"}
		}
		path := req.Path
		if path == "" {
			path = s.DefaultLogPath
		}
		if path == "" {
			return Response{Error: "no log path given and no default configured"}
		}
		var ids []core.SensorID
		for _, id := range req.Sensors {
			ids = append(ids, core.SensorID(id))
		}
		if err := s.Logger.Start(path, core.LogFormat(req.Format), ids); err != nil {
			return Response{Error: err.Error()}
		}
		return Response{OK: true, Log: s.logStatus()}
	case "log_stop":
		if s.Logger == nil {
			return Response{Error: "logging not available"}
		}
		if err := s.Logger.Stop(); err != nil {
			return Response{Error: err.Error()}
		}
		return Response{OK: true}
	case "log_mark":
		if s.Logger == nil {
			return Response{Error: "logging not available"}
		}
		s.Logger.Mark(req.Note)
		return Response{OK: true}
	case "log_status":
		if s.Logger == nil {
			return Response{OK: true, Log: &LogStatus{}}
		}
		return Response{OK: true, Log: s.logStatus()}
	case "report":
		out, err := core.BuildReport(s.reg.Devices(), s.reg.Snapshot(), core.ReportOptions{
			Format:   req.Format,
			Sections: req.Sections,
			Redact:   req.Redact,
		})
		if err != nil {
			return Response{Error: err.Error()}
		}
		return Response{OK: true, Report: string(out)}
	default:
		return Response{Error: "unknown op: " + req.Op}
	}
}

func (s *Server) logStatus() *LogStatus {
	active, path, format := s.Logger.Status()
	return &LogStatus{Active: active, Path: path, Format: string(format)}
}

func (s *Server) stream(ctx context.Context, enc *json.Encoder, req Request) {
	interval := time.Duration(req.IntervalMs) * time.Millisecond
	if interval < 100*time.Millisecond {
		interval = time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		if err := enc.Encode(Response{OK: true, Event: "sensors", Sensors: s.reg.Snapshot()}); err != nil {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
