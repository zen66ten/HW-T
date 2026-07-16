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
	Op         string `json:"op"` // sensors | devices | history | reset | subscribe
	ID         string `json:"id,omitempty"`
	IntervalMs int    `json:"interval_ms,omitempty"`
}

// Response is the answer to a request, or one subscription event.
type Response struct {
	OK          bool              `json:"ok"`
	Error       string            `json:"error,omitempty"`
	Event       string            `json:"event,omitempty"`
	Sensors     []core.Sensor     `json:"sensors,omitempty"`
	Devices     []core.Device     `json:"devices,omitempty"`
	History     []core.Point      `json:"history,omitempty"`
	Quarantined map[string]string `json:"quarantined,omitempty"`
}

// Server answers requests from the registry.
type Server struct {
	reg *core.Registry
	ln  net.Listener
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
	default:
		return Response{Error: "unknown op: " + req.Op}
	}
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
