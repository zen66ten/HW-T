package uds

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/zen66ten/HW-T/internal/core"
	"github.com/zen66ten/HW-T/pkg/client"
)

func testServer(t *testing.T) (string, *core.Registry) {
	t.Helper()
	reg := core.NewRegistry(16)
	reg.SetDevices("test", []core.Device{{
		ID: "test:dev0", Provider: "test", Name: "chip0",
		Channels: []core.ChannelInfo{{ID: "temp1", Kind: core.KindTemp, Label: "T1"}},
	}})
	reg.Apply("test:dev0", []core.Reading{{Channel: "temp1", Value: 42.5}}, time.Now())

	socket := filepath.Join(t.TempDir(), "hwtd.sock")
	srv, err := Listen(socket, reg)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go srv.Serve(ctx)
	return socket, reg
}

func TestRoundTrip(t *testing.T) {
	socket, _ := testServer(t)

	c, err := client.Dial(socket)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	sensors, err := c.Sensors()
	if err != nil {
		t.Fatal(err)
	}
	if len(sensors) != 1 {
		t.Fatalf("got %d sensors, want 1: %+v", len(sensors), sensors)
	}
	s := sensors[0]
	if s.ID != "test:dev0:temp1" || s.Cur != 42.5 || s.Label != "T1" || s.Unit != "°C" {
		t.Errorf("sensor = %+v", s)
	}

	devs, err := c.Devices()
	if err != nil {
		t.Fatal(err)
	}
	if len(devs) != 1 || devs[0].Name != "chip0" {
		t.Errorf("devices = %+v", devs)
	}

	hist, err := c.History("test:dev0:temp1")
	if err != nil {
		t.Fatal(err)
	}
	if len(hist) != 1 || hist[0].Value != 42.5 {
		t.Errorf("history = %+v", hist)
	}
	if _, err := c.History("nope"); err == nil {
		t.Error("history of unknown sensor did not error")
	}

	if err := c.Reset(""); err != nil {
		t.Fatal(err)
	}
	sensors, _ = c.Sensors()
	if sensors[0].N != 0 {
		t.Errorf("stats not reset: %+v", sensors[0])
	}
}

func TestSubscribe(t *testing.T) {
	socket, reg := testServer(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	got := make(chan []client.Sensor, 4)
	go client.Subscribe(ctx, socket, 100*time.Millisecond, func(s []client.Sensor) {
		got <- s
	})

	// First push is immediate; then feed a new value and expect it streamed.
	select {
	case s := <-got:
		if s[0].Cur != 42.5 {
			t.Errorf("first push = %+v", s[0])
		}
	case <-ctx.Done():
		t.Fatal("no initial subscription push")
	}

	reg.Apply("test:dev0", []core.Reading{{Channel: "temp1", Value: 50}}, time.Now())
	deadline := time.After(3 * time.Second)
	for {
		select {
		case s := <-got:
			if s[0].Cur == 50 {
				return
			}
		case <-deadline:
			t.Fatal("updated value never streamed")
		}
	}
}
