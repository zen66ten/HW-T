package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func loggerSensors(v1, v2 float64) []Sensor {
	return []Sensor{
		{ID: "test:dev:temp1", Kind: KindTemp, Cur: v1, N: 5},
		{ID: "test:dev:fan1", Kind: KindFan, Cur: v2, N: 5},
	}
}

func TestLoggerCSV(t *testing.T) {
	path := filepath.Join(t.TempDir(), "log.csv")
	l := NewLogger()
	if err := l.Start(path, LogCSV, nil); err != nil {
		t.Fatal(err)
	}

	ts := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	l.Tick(loggerSensors(42.5, 1200), ts)
	l.Mark("stress test start")
	l.Tick(loggerSensors(55, 1500), ts.Add(time.Second))
	// Errored sensor produces an empty cell.
	broken := loggerSensors(60, 0)
	broken[1].Err = "EIO"
	l.Tick(broken, ts.Add(2*time.Second))
	if err := l.Stop(); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	if len(lines) != 4 {
		t.Fatalf("got %d lines, want header+3 rows:\n%s", len(lines), raw)
	}
	if lines[0] != "time,test:dev:temp1,test:dev:fan1,note" {
		t.Errorf("header = %q", lines[0])
	}
	if !strings.HasSuffix(lines[1], ",42.5,1200,") {
		t.Errorf("row 1 = %q", lines[1])
	}
	if !strings.HasSuffix(lines[2], ",55,1500,stress test start") {
		t.Errorf("row 2 (with mark) = %q", lines[2])
	}
	if !strings.HasSuffix(lines[3], ",60,,") {
		t.Errorf("row 3 (errored cell) = %q", lines[3])
	}
}

func TestLoggerNDJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "log.ndjson")
	l := NewLogger()
	if err := l.Start(path, LogNDJSON, nil); err != nil {
		t.Fatal(err)
	}
	l.Mark("boot")
	l.Tick(loggerSensors(42.5, 1200), time.Now())
	l.Stop()

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var row struct {
		Ts     string             `json:"ts"`
		Note   string             `json:"note"`
		Values map[string]float64 `json:"values"`
	}
	if err := json.Unmarshal(raw, &row); err != nil {
		t.Fatalf("bad ndjson: %v\n%s", err, raw)
	}
	if row.Note != "boot" || row.Values["test:dev:temp1"] != 42.5 || row.Values["test:dev:fan1"] != 1200 {
		t.Errorf("row = %+v", row)
	}
}

func TestLoggerSelectedSensors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "log.csv")
	l := NewLogger()
	if err := l.Start(path, LogCSV, []SensorID{"test:dev:temp1"}); err != nil {
		t.Fatal(err)
	}
	l.Tick(loggerSensors(42.5, 1200), time.Now())
	l.Stop()

	raw, _ := os.ReadFile(path)
	if strings.Contains(string(raw), "fan1") {
		t.Errorf("unselected sensor leaked into log:\n%s", raw)
	}
}

func TestLoggerDoubleStart(t *testing.T) {
	dir := t.TempDir()
	l := NewLogger()
	if err := l.Start(filepath.Join(dir, "a.csv"), LogCSV, nil); err != nil {
		t.Fatal(err)
	}
	if err := l.Start(filepath.Join(dir, "b.csv"), LogCSV, nil); err == nil {
		t.Error("second Start did not error")
	}
	l.Stop()
	if err := l.Stop(); err == nil {
		t.Error("Stop on inactive logger did not error")
	}
}
