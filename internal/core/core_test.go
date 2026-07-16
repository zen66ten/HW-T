package core

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRing(t *testing.T) {
	r := NewRing(3)
	base := time.UnixMilli(1000)
	for i := 0; i < 5; i++ {
		r.Push(base.Add(time.Duration(i)*time.Second), float64(i))
	}
	pts := r.Points()
	if len(pts) != 3 {
		t.Fatalf("got %d points, want 3", len(pts))
	}
	for i, want := range []float64{2, 3, 4} {
		if pts[i].Value != want {
			t.Errorf("point %d = %v, want %v", i, pts[i].Value, want)
		}
	}
	if pts[0].Ts >= pts[2].Ts {
		t.Errorf("points not oldest-first: %+v", pts)
	}
}

func TestStats(t *testing.T) {
	var s Stats
	for _, v := range []float64{5, -2, 9} {
		s.Add(v)
	}
	if s.Min != -2 || s.Max != 9 || s.Avg() != 4 || s.N != 3 {
		t.Errorf("stats = min %v max %v avg %v n %d, want -2 9 4 3", s.Min, s.Max, s.Avg(), s.N)
	}
	s = Stats{}
	if s.Avg() != 0 {
		t.Errorf("empty avg = %v, want 0", s.Avg())
	}
}

func testDevice() Device {
	return Device{
		ID: "test:dev0", Provider: "test", Name: "dev0",
		Channels: []ChannelInfo{
			{ID: "temp1", Kind: KindTemp, Label: "T1"},
			{ID: "temp2", Kind: KindTemp, Label: "T2"},
		},
	}
}

func TestRegistryStatePersistsAcrossRediscovery(t *testing.T) {
	reg := NewRegistry(16)
	reg.SetDevices("test", []Device{testDevice()})
	reg.Apply("test:dev0", []Reading{{Channel: "temp1", Value: 42}}, time.Now())

	// Re-discovery (hotplug, restart of the poll loop) with the same stable
	// IDs must keep stats and history.
	reg.SetDevices("test", []Device{testDevice()})
	snap := reg.Snapshot()
	if len(snap) != 2 {
		t.Fatalf("got %d sensors, want 2", len(snap))
	}
	if snap[0].Cur != 42 || snap[0].N != 1 {
		t.Errorf("state lost across rediscovery: %+v", snap[0])
	}

	pts, ok := reg.History("test:dev0:temp1")
	if !ok || len(pts) != 1 || pts[0].Value != 42 {
		t.Errorf("history lost across rediscovery: %v %v", pts, ok)
	}
}

func TestRegistryReset(t *testing.T) {
	reg := NewRegistry(16)
	reg.SetDevices("test", []Device{testDevice()})
	now := time.Now()
	reg.Apply("test:dev0", []Reading{{Channel: "temp1", Value: 10}, {Channel: "temp2", Value: 20}}, now)
	reg.Apply("test:dev0", []Reading{{Channel: "temp1", Value: 30}, {Channel: "temp2", Value: 40}}, now)

	if !reg.Reset("test:dev0:temp1") {
		t.Fatal("reset of known sensor failed")
	}
	if reg.Reset("test:dev0:nope") {
		t.Fatal("reset of unknown sensor succeeded")
	}
	snap := reg.Snapshot()
	if snap[0].N != 0 {
		t.Errorf("temp1 stats not reset: %+v", snap[0])
	}
	if snap[1].N != 2 || snap[1].Min != 20 || snap[1].Max != 40 {
		t.Errorf("temp2 stats damaged by sibling reset: %+v", snap[1])
	}

	reg.Reset("")
	if s := reg.Snapshot(); s[1].N != 0 {
		t.Errorf("global reset missed temp2: %+v", s[1])
	}
}

func TestRegistryErrorReadingsSkipStats(t *testing.T) {
	reg := NewRegistry(16)
	reg.SetDevices("test", []Device{testDevice()})
	reg.Apply("test:dev0", []Reading{{Channel: "temp1", Value: 42}}, time.Now())
	reg.Apply("test:dev0", []Reading{{Channel: "temp1", Err: "EIO"}}, time.Now())

	s := reg.Snapshot()[0]
	if s.Err != "EIO" {
		t.Errorf("Err = %q, want EIO", s.Err)
	}
	if s.N != 1 || s.Cur != 42 {
		t.Errorf("error reading polluted stats: %+v", s)
	}
}

// panicky implements Provider and panics on every Collect.
type panicky struct{ discovered Device }

func (p *panicky) Name() string                   { return "panicky" }
func (p *panicky) DefaultInterval() time.Duration { return time.Millisecond }
func (p *panicky) Discover(ctx context.Context) ([]Device, error) {
	return []Device{p.discovered}, nil
}
func (p *panicky) Collect(ctx context.Context, dev DeviceID) ([]Reading, error) {
	panic("boom")
}

// steady always works.
type steady struct{ n float64 }

func (p *steady) Name() string                   { return "steady" }
func (p *steady) DefaultInterval() time.Duration { return time.Millisecond }
func (p *steady) Discover(ctx context.Context) ([]Device, error) {
	return []Device{{
		ID: "steady:dev", Provider: "steady", Name: "dev",
		Channels: []ChannelInfo{{ID: "temp1", Kind: KindTemp, Label: "T"}},
	}}, nil
}
func (p *steady) Collect(ctx context.Context, dev DeviceID) ([]Reading, error) {
	p.n++
	return []Reading{{Channel: "temp1", Value: p.n}}, nil
}

func TestSchedulerQuarantinesPanickingProvider(t *testing.T) {
	reg := NewRegistry(16)
	bad := &panicky{discovered: Device{
		ID: "panicky:dev", Provider: "panicky", Name: "dev",
		Channels: []ChannelInfo{{ID: "temp1", Kind: KindTemp, Label: "T"}},
	}}
	good := &steady{}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sched := NewScheduler(reg, bad, good)
	sched.Start(ctx)

	deadline := time.After(2 * time.Second)
	for {
		if _, bad := reg.Quarantined()["panicky"]; bad {
			break
		}
		select {
		case <-deadline:
			t.Fatal("panicky provider never quarantined")
		case <-time.After(5 * time.Millisecond):
		}
	}

	// The healthy provider must keep collecting after the sibling died.
	before := findSensor(t, reg, "steady:dev:temp1").N
	time.Sleep(20 * time.Millisecond)
	after := findSensor(t, reg, "steady:dev:temp1").N
	if after <= before {
		t.Errorf("steady provider stalled after sibling quarantine: %d -> %d", before, after)
	}
}

func TestSchedulerDiscoveryFailure(t *testing.T) {
	reg := NewRegistry(16)
	sched := NewScheduler(reg, &failingDiscovery{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sched.Start(ctx)
	if reason := reg.Quarantined()["failing"]; reason == "" {
		t.Error("failed discovery not recorded")
	}
}

type failingDiscovery struct{}

func (f *failingDiscovery) Name() string                   { return "failing" }
func (f *failingDiscovery) DefaultInterval() time.Duration { return time.Second }
func (f *failingDiscovery) Discover(ctx context.Context) ([]Device, error) {
	return nil, errors.New("no such hardware")
}
func (f *failingDiscovery) Collect(ctx context.Context, dev DeviceID) ([]Reading, error) {
	return nil, nil
}

func findSensor(t *testing.T, reg *Registry, id SensorID) Sensor {
	t.Helper()
	for _, s := range reg.Snapshot() {
		if s.ID == id {
			return s
		}
	}
	t.Fatalf("sensor %s not found", id)
	return Sensor{}
}
