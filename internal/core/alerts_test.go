package core

import (
	"testing"
	"time"
)

type firedEvent struct {
	rule   string
	action string
	event  string
}

func alertTestSetup(t *testing.T, rule AlertRule) (*Registry, *AlertEngine, *[]firedEvent) {
	t.Helper()
	reg := NewRegistry(16)
	reg.SetDevices("test", []Device{{
		ID: "test:dev", Provider: "test", Name: "dev",
		Channels: []ChannelInfo{{ID: "temp1", Kind: KindTemp, Label: "T"}},
	}})

	engine := NewAlertEngine(reg, []AlertRule{rule})
	var fired []firedEvent
	engine.fire = func(rs *ruleState, action, event string) {
		fired = append(fired, firedEvent{rs.rule.Name, action, event})
	}
	return reg, engine, &fired
}

func apply(reg *Registry, v float64, ts time.Time) {
	reg.Apply("test:dev", []Reading{{Channel: "temp1", Value: v}}, ts)
}

func f64(v float64) *float64 { return &v }

func TestAlertFiresAfterForDuration(t *testing.T) {
	rule := AlertRule{
		Name: "hot", Sensor: "test:dev:temp1",
		Above: f64(80), For: 10 * time.Second, Hysteresis: 5,
		Actions: []string{"journal"},
	}
	reg, engine, fired := alertTestSetup(t, rule)
	now := time.Now()

	// Breach starts: pending, not firing.
	apply(reg, 85, now)
	engine.eval(now)
	if s := engine.Statuses()[0]; s.State != AlertPending {
		t.Fatalf("state = %s, want pending", s.State)
	}
	if len(*fired) != 0 {
		t.Fatalf("fired too early: %+v", *fired)
	}

	// Still breached before For elapses: still pending.
	engine.eval(now.Add(5 * time.Second))
	if s := engine.Statuses()[0]; s.State != AlertPending {
		t.Fatalf("state = %s, want pending at 5s", s.State)
	}

	// For elapsed: firing, action dispatched once.
	engine.eval(now.Add(11 * time.Second))
	if s := engine.Statuses()[0]; s.State != AlertFiring {
		t.Fatalf("state = %s, want firing", s.State)
	}
	if len(*fired) != 1 || (*fired)[0].event != "firing" {
		t.Fatalf("fired = %+v, want one firing event", *fired)
	}

	// Value drops below threshold but within hysteresis (80-5=75): still firing.
	apply(reg, 78, now.Add(12*time.Second))
	engine.eval(now.Add(12 * time.Second))
	if s := engine.Statuses()[0]; s.State != AlertFiring {
		t.Fatalf("state = %s, want firing inside hysteresis band", s.State)
	}

	// Value clears hysteresis: resolved.
	apply(reg, 70, now.Add(13*time.Second))
	engine.eval(now.Add(13 * time.Second))
	if s := engine.Statuses()[0]; s.State != AlertOK {
		t.Fatalf("state = %s, want ok", s.State)
	}
	if len(*fired) != 2 || (*fired)[1].event != "resolved" {
		t.Fatalf("fired = %+v, want firing+resolved", *fired)
	}
}

func TestAlertPendingResetsWhenConditionLifts(t *testing.T) {
	rule := AlertRule{
		Name: "hot", Sensor: "test:dev:temp1",
		Above: f64(80), For: time.Minute,
		Actions: []string{"journal"},
	}
	reg, engine, fired := alertTestSetup(t, rule)
	now := time.Now()

	apply(reg, 85, now)
	engine.eval(now)
	apply(reg, 75, now.Add(30*time.Second))
	engine.eval(now.Add(30 * time.Second))
	if s := engine.Statuses()[0]; s.State != AlertOK {
		t.Fatalf("state = %s, want ok after condition lifted mid-pending", s.State)
	}

	// Breach again: the For clock must restart, not resume.
	apply(reg, 85, now.Add(40*time.Second))
	engine.eval(now.Add(40 * time.Second))
	engine.eval(now.Add(90 * time.Second)) // only 50s into the new breach
	if s := engine.Statuses()[0]; s.State != AlertPending {
		t.Fatalf("state = %s, want pending (For clock restarted)", s.State)
	}
	if len(*fired) != 0 {
		t.Fatalf("fired = %+v, want none", *fired)
	}
}

func TestAlertBelowRule(t *testing.T) {
	rule := AlertRule{
		Name: "fan-dead", Sensor: "test:dev:temp1",
		Below: f64(100), For: 0, Hysteresis: 50,
		Actions: []string{"journal"},
	}
	reg, engine, fired := alertTestSetup(t, rule)
	now := time.Now()

	// For=0 fires immediately on breach.
	apply(reg, 0, now)
	engine.eval(now)
	if s := engine.Statuses()[0]; s.State != AlertFiring {
		t.Fatalf("state = %s, want firing immediately with For=0", s.State)
	}
	if len(*fired) != 1 {
		t.Fatalf("fired = %+v", *fired)
	}

	// Must exceed below+hysteresis (150) to resolve.
	apply(reg, 120, now.Add(time.Second))
	engine.eval(now.Add(time.Second))
	if s := engine.Statuses()[0]; s.State != AlertFiring {
		t.Fatalf("state = %s, want still firing at 120 (< 150)", s.State)
	}
	apply(reg, 200, now.Add(2*time.Second))
	engine.eval(now.Add(2 * time.Second))
	if s := engine.Statuses()[0]; s.State != AlertOK {
		t.Fatalf("state = %s, want ok at 200", s.State)
	}
}

func TestAlertHoldsStateOnMissingData(t *testing.T) {
	rule := AlertRule{
		Name: "hot", Sensor: "test:dev:temp1",
		Above: f64(80), For: 0,
		Actions: []string{"journal"},
	}
	reg, engine, fired := alertTestSetup(t, rule)
	now := time.Now()

	apply(reg, 85, now)
	engine.eval(now)
	if s := engine.Statuses()[0]; s.State != AlertFiring {
		t.Fatalf("state = %s, want firing", s.State)
	}

	// Sensor read starts erroring: state holds, no resolved event.
	reg.Apply("test:dev", []Reading{{Channel: "temp1", Err: "EIO"}}, now.Add(time.Second))
	engine.eval(now.Add(time.Second))
	if s := engine.Statuses()[0]; s.State != AlertFiring {
		t.Fatalf("state = %s, want firing held through missing data", s.State)
	}
	if len(*fired) != 1 {
		t.Fatalf("fired = %+v, want just the original firing", *fired)
	}
}
