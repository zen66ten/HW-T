// Package client is the Go SDK for the hwtd Unix-socket API. The wire
// protocol is newline-delimited JSON; these types mirror the daemon's wire
// format so importers stay decoupled from daemon internals.
package client

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"time"
)

// Sensor is one sensor with its running statistics.
type Sensor struct {
	ID         string             `json:"id"`
	Device     string             `json:"device"`
	DeviceName string             `json:"device_name"`
	Provider   string             `json:"provider"`
	Kind       string             `json:"kind"`
	Label      string             `json:"label"`
	Unit       string             `json:"unit"`
	Limits     map[string]float64 `json:"limits,omitempty"`

	Cur float64   `json:"cur"`
	Min float64   `json:"min"`
	Max float64   `json:"max"`
	Avg float64   `json:"avg"`
	N   uint64    `json:"n"`
	Ts  time.Time `json:"ts"`
	Err string    `json:"err,omitempty"`
}

// Device is one discovered device; inventory devices carry only Attrs.
type Device struct {
	ID       string            `json:"id"`
	Provider string            `json:"provider"`
	Name     string            `json:"name"`
	Attrs    map[string]string `json:"attrs,omitempty"`
}

// Point is one history sample (unix-millisecond timestamp).
type Point struct {
	Ts    int64   `json:"ts"`
	Value float64 `json:"v"`
}

// AlertStatus is the state of one configured alert rule.
type AlertStatus struct {
	Name   string    `json:"name"`
	Sensor string    `json:"sensor"`
	State  string    `json:"state"` // ok | pending | firing
	Value  float64   `json:"value"`
	Since  time.Time `json:"since"`
}

// LogStatus reports the daemon's sensor-logging state.
type LogStatus struct {
	Active bool   `json:"active"`
	Path   string `json:"path,omitempty"`
	Format string `json:"format,omitempty"`
}

type request struct {
	Op         string   `json:"op"`
	ID         string   `json:"id,omitempty"`
	IntervalMs int      `json:"interval_ms,omitempty"`
	Path       string   `json:"path,omitempty"`
	Format     string   `json:"format,omitempty"`
	Note       string   `json:"note,omitempty"`
	Sensors    []string `json:"sensors,omitempty"`
	Sections   []string `json:"sections,omitempty"`
	Redact     bool     `json:"redact,omitempty"`
}

type response struct {
	OK          bool              `json:"ok"`
	Error       string            `json:"error,omitempty"`
	Event       string            `json:"event,omitempty"`
	Sensors     []Sensor          `json:"sensors,omitempty"`
	Devices     []Device          `json:"devices,omitempty"`
	History     []Point           `json:"history,omitempty"`
	Quarantined map[string]string `json:"quarantined,omitempty"`
	Alerts      []AlertStatus     `json:"alerts,omitempty"`
	Log         *LogStatus        `json:"log,omitempty"`
	Report      string            `json:"report,omitempty"`
}

// DefaultSocket returns the conventional socket path: /run/hwtd.sock when it
// exists (root daemon), otherwise the user-session path a non-root hwtd
// binds by default.
func DefaultSocket() string {
	const system = "/run/hwtd.sock"
	if _, err := os.Stat(system); err == nil {
		return system
	}
	if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
		return dir + "/hwtd.sock"
	}
	return system
}

// Client is one connection to the daemon. Not safe for concurrent use.
type Client struct {
	path string
	conn net.Conn
	rd   *bufio.Reader
	enc  *json.Encoder
}

func Dial(socketPath string) (*Client, error) {
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("connecting to hwtd at %s: %w (is hwtd running?)", socketPath, err)
	}
	return &Client{
		path: socketPath,
		conn: conn,
		rd:   bufio.NewReader(conn),
		enc:  json.NewEncoder(conn),
	}, nil
}

func (c *Client) Close() error { return c.conn.Close() }

func (c *Client) roundTrip(req request) (*response, error) {
	if err := c.enc.Encode(req); err != nil {
		return nil, err
	}
	line, err := c.rd.ReadBytes('\n')
	if err != nil {
		return nil, err
	}
	var resp response
	if err := json.Unmarshal(line, &resp); err != nil {
		return nil, err
	}
	if !resp.OK {
		return nil, errors.New(resp.Error)
	}
	return &resp, nil
}

// Sensors returns the current snapshot of all sensors.
func (c *Client) Sensors() ([]Sensor, error) {
	resp, err := c.roundTrip(request{Op: "sensors"})
	if err != nil {
		return nil, err
	}
	return resp.Sensors, nil
}

// Devices returns all discovered devices including DMI inventory.
func (c *Client) Devices() ([]Device, error) {
	resp, err := c.roundTrip(request{Op: "devices"})
	if err != nil {
		return nil, err
	}
	return resp.Devices, nil
}

// History returns the buffered samples for one sensor ID.
func (c *Client) History(id string) ([]Point, error) {
	resp, err := c.roundTrip(request{Op: "history", ID: id})
	if err != nil {
		return nil, err
	}
	return resp.History, nil
}

// Reset clears min/max/avg for one sensor, or for all when id is empty.
func (c *Client) Reset(id string) error {
	_, err := c.roundTrip(request{Op: "reset", ID: id})
	return err
}

// Alerts returns the state of all configured alert rules.
func (c *Client) Alerts() ([]AlertStatus, error) {
	resp, err := c.roundTrip(request{Op: "alerts"})
	if err != nil {
		return nil, err
	}
	return resp.Alerts, nil
}

// LogStart begins sensor logging. Empty path uses the daemon's configured
// default; empty format means CSV; sensors restricts the column set.
func (c *Client) LogStart(path, format string, sensors []string) (*LogStatus, error) {
	resp, err := c.roundTrip(request{Op: "log_start", Path: path, Format: format, Sensors: sensors})
	if err != nil {
		return nil, err
	}
	return resp.Log, nil
}

// LogStop ends sensor logging.
func (c *Client) LogStop() error {
	_, err := c.roundTrip(request{Op: "log_stop"})
	return err
}

// LogMark attaches a note to the next logged row.
func (c *Client) LogMark(note string) error {
	_, err := c.roundTrip(request{Op: "log_mark", Note: note})
	return err
}

// LogStatus reports whether logging is active and to which file.
func (c *Client) LogStatus() (*LogStatus, error) {
	resp, err := c.roundTrip(request{Op: "log_status"})
	if err != nil {
		return nil, err
	}
	return resp.Log, nil
}

// Report renders the inventory+sensors report in the given format
// (text, html, json, yaml, csv). sections filters by provider; redact
// blanks serials, UUIDs and MACs.
func (c *Client) Report(format string, sections []string, redact bool) (string, error) {
	resp, err := c.roundTrip(request{Op: "report", Format: format, Sections: sections, Redact: redact})
	if err != nil {
		return "", err
	}
	return resp.Report, nil
}

// Subscribe opens a dedicated connection and invokes fn with each snapshot
// pushed by the daemon until ctx is cancelled or the connection drops.
func Subscribe(ctx context.Context, socketPath string, interval time.Duration, fn func([]Sensor)) error {
	c, err := Dial(socketPath)
	if err != nil {
		return err
	}
	defer c.Close()
	go func() {
		<-ctx.Done()
		c.conn.Close()
	}()

	if err := c.enc.Encode(request{Op: "subscribe", IntervalMs: int(interval.Milliseconds())}); err != nil {
		return err
	}
	for {
		line, err := c.rd.ReadBytes('\n')
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		var resp response
		if err := json.Unmarshal(line, &resp); err != nil {
			return err
		}
		if resp.Event == "sensors" {
			fn(resp.Sensors)
		}
	}
}
