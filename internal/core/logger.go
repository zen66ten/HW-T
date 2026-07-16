package core

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// LogFormat selects the sensor-log encoding.
type LogFormat string

const (
	LogCSV    LogFormat = "csv"    // HWiNFO-spirit: timestamp + one column per sensor
	LogNDJSON LogFormat = "ndjson" // one JSON object per tick
)

// rotateBytes is the size at which the active log is rolled to "<path>.1".
const rotateBytes = 100 << 20

// Logger writes one row per poll tick while active. Start/Stop/Mark are
// driven by hwtctl over the UDS API (§6.2); the daemon calls Tick on its
// fast cadence regardless, and inactive ticks are free.
type Logger struct {
	mu     sync.Mutex
	f      *os.File
	path   string
	format LogFormat
	ids    []SensorID // column set; frozen at first tick when empty
	note   string
	wrote  int64
}

func NewLogger() *Logger { return &Logger{} }

// Start begins logging to path. ids restricts the column set; empty means
// every sensor present at the first tick.
func (l *Logger) Start(path string, format LogFormat, ids []SensorID) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.f != nil {
		return fmt.Errorf("log already active at %s", l.path)
	}
	if format == "" {
		format = LogCSV
	}
	if format != LogCSV && format != LogNDJSON {
		return fmt.Errorf("unknown log format %q", format)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return err
	}
	l.f, l.path, l.format, l.ids, l.note, l.wrote = f, path, format, ids, "", st.Size()
	return nil
}

func (l *Logger) Stop() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.f == nil {
		return fmt.Errorf("log not active")
	}
	err := l.f.Close()
	l.f = nil
	return err
}

// Mark attaches a note to the next written row (HWiNFO log-note parity).
func (l *Logger) Mark(note string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.note = note
}

// Status returns whether logging is active and to which file.
func (l *Logger) Status() (active bool, path string, format LogFormat) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.f != nil, l.path, l.format
}

// Tick writes one row when active. Errored sensors produce empty cells
// (CSV) or are omitted (NDJSON).
func (l *Logger) Tick(sensors []Sensor, ts time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.f == nil {
		return
	}

	if l.ids == nil {
		l.ids = make([]SensorID, 0, len(sensors))
		for _, s := range sensors {
			l.ids = append(l.ids, s.ID)
		}
		if l.format == LogCSV && l.wrote == 0 {
			l.write(csvHeader(l.ids))
		}
	}

	byID := make(map[SensorID]Sensor, len(sensors))
	for _, s := range sensors {
		byID[s.ID] = s
	}

	var row string
	switch l.format {
	case LogCSV:
		row = l.csvRow(byID, ts)
	case LogNDJSON:
		row = l.ndjsonRow(byID, ts)
	}
	l.note = ""
	l.write(row)

	if l.wrote > rotateBytes {
		l.rotate()
	}
}

func (l *Logger) write(s string) {
	n, _ := l.f.WriteString(s)
	l.wrote += int64(n)
}

func (l *Logger) rotate() {
	l.f.Close()
	os.Rename(l.path, l.path+".1")
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		l.f = nil
		return
	}
	l.f, l.wrote = f, 0
	if l.format == LogCSV {
		l.write(csvHeader(l.ids))
	}
}

func csvHeader(ids []SensorID) string {
	cols := make([]string, 0, len(ids)+2)
	cols = append(cols, "time")
	for _, id := range ids {
		cols = append(cols, csvEscape(string(id)))
	}
	cols = append(cols, "note")
	return strings.Join(cols, ",") + "\n"
}

func (l *Logger) csvRow(byID map[SensorID]Sensor, ts time.Time) string {
	cols := make([]string, 0, len(l.ids)+2)
	cols = append(cols, ts.Format(time.RFC3339))
	for _, id := range l.ids {
		s, ok := byID[id]
		if !ok || s.Err != "" || s.N == 0 {
			cols = append(cols, "")
			continue
		}
		cols = append(cols, strconv.FormatFloat(s.Cur, 'f', -1, 64))
	}
	cols = append(cols, csvEscape(l.note))
	return strings.Join(cols, ",") + "\n"
}

func csvEscape(s string) string {
	if strings.ContainsAny(s, ",\"\n") {
		return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
	}
	return s
}

func (l *Logger) ndjsonRow(byID map[SensorID]Sensor, ts time.Time) string {
	values := make(map[string]float64, len(l.ids))
	for _, id := range l.ids {
		if s, ok := byID[id]; ok && s.Err == "" && s.N > 0 {
			values[string(id)] = s.Cur
		}
	}
	row := struct {
		Ts     string             `json:"ts"`
		Note   string             `json:"note,omitempty"`
		Values map[string]float64 `json:"values"`
	}{ts.Format(time.RFC3339Nano), l.note, values}
	b, err := json.Marshal(row)
	if err != nil {
		return ""
	}
	return string(b) + "\n"
}
