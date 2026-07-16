package core

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
)

// AlertRule is one user-configured condition: value above/below a threshold
// continuously for a duration. Hysteresis keeps a firing alert from
// flapping: it only resolves once the value retreats past the threshold by
// that margin.
type AlertRule struct {
	Name       string
	Sensor     SensorID
	Above      *float64
	Below      *float64
	For        time.Duration
	Hysteresis float64
	Actions    []string // "journal", "notify", "exec:<cmd>", "webhook:<url>"
}

// AlertState is the lifecycle of one rule.
type AlertState string

const (
	AlertOK      AlertState = "ok"
	AlertPending AlertState = "pending" // condition met, For-duration not yet elapsed
	AlertFiring  AlertState = "firing"
)

// AlertStatus is the queryable state of one rule (§6.1: alert state is
// exposed on all APIs).
type AlertStatus struct {
	Name   string     `json:"name"`
	Sensor SensorID   `json:"sensor"`
	State  AlertState `json:"state"`
	Value  float64    `json:"value"`
	Since  time.Time  `json:"since"`
}

// execRateLimit is the minimum spacing between exec-action runs per rule.
const execRateLimit = 30 * time.Second

type ruleState struct {
	rule     AlertRule
	state    AlertState
	since    time.Time
	value    float64
	kind     Kind
	lastExec time.Time
}

// AlertEngine evaluates rules against the registry on a fixed cadence.
type AlertEngine struct {
	mu    sync.Mutex
	reg   *Registry
	rules []*ruleState

	// fire dispatches one action string for a transition; swapped out in
	// tests. The default implementation handles journal/notify/exec/webhook.
	fire func(rs *ruleState, action, event string)
}

func NewAlertEngine(reg *Registry, rules []AlertRule) *AlertEngine {
	e := &AlertEngine{reg: reg}
	for _, r := range rules {
		if r.Name == "" {
			r.Name = string(r.Sensor)
		}
		e.rules = append(e.rules, &ruleState{rule: r, state: AlertOK})
	}
	e.fire = e.dispatch
	return e
}

// Run evaluates rules every interval until ctx is cancelled.
func (e *AlertEngine) Run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			e.eval(now)
		}
	}
}

// Statuses returns the current state of every rule.
func (e *AlertEngine) Statuses() []AlertStatus {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]AlertStatus, 0, len(e.rules))
	for _, rs := range e.rules {
		out = append(out, AlertStatus{
			Name:   rs.rule.Name,
			Sensor: rs.rule.Sensor,
			State:  rs.state,
			Value:  rs.value,
			Since:  rs.since,
		})
	}
	return out
}

func (e *AlertEngine) eval(now time.Time) {
	snap := e.reg.Snapshot()
	byID := make(map[SensorID]Sensor, len(snap))
	for _, s := range snap {
		byID[s.ID] = s
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	for _, rs := range e.rules {
		s, ok := byID[rs.rule.Sensor]
		if !ok || s.Err != "" || s.N == 0 {
			continue // no data: hold current state rather than flap
		}
		rs.value, rs.kind = s.Cur, s.Kind

		breached := breach(rs.rule, s.Cur)
		switch rs.state {
		case AlertOK:
			if breached {
				rs.state, rs.since = AlertPending, now
				if rs.rule.For == 0 {
					rs.state = AlertFiring
					e.fireAll(rs, "firing")
				}
			}
		case AlertPending:
			if !breached {
				rs.state = AlertOK
			} else if now.Sub(rs.since) >= rs.rule.For {
				rs.state, rs.since = AlertFiring, now
				e.fireAll(rs, "firing")
			}
		case AlertFiring:
			if cleared(rs.rule, s.Cur) {
				rs.state, rs.since = AlertOK, now
				e.fireAll(rs, "resolved")
			}
		}
	}
}

func breach(r AlertRule, v float64) bool {
	if r.Above != nil && v > *r.Above {
		return true
	}
	if r.Below != nil && v < *r.Below {
		return true
	}
	return false
}

// cleared applies hysteresis: the value must retreat past the threshold by
// the margin before a firing alert resolves.
func cleared(r AlertRule, v float64) bool {
	if r.Above != nil {
		return v <= *r.Above-r.Hysteresis
	}
	if r.Below != nil {
		return v >= *r.Below+r.Hysteresis
	}
	return true
}

func (e *AlertEngine) fireAll(rs *ruleState, event string) {
	for _, a := range rs.rule.Actions {
		e.fire(rs, a, event)
	}
}

func (e *AlertEngine) dispatch(rs *ruleState, action, event string) {
	msg := fmt.Sprintf("alert %s %s: sensor %s = %s (rule: %s)",
		rs.rule.Name, event, rs.rule.Sensor, FormatValue(rs.kind, rs.value), describeRule(rs.rule))

	switch {
	case action == "journal":
		if event == "firing" {
			slog.Warn(msg, "alert", rs.rule.Name, "event", event, "value", rs.value)
		} else {
			slog.Info(msg, "alert", rs.rule.Name, "event", event, "value", rs.value)
		}

	case action == "notify":
		// Best-effort desktop notification; works when the daemon runs in a
		// user session. A root daemon has no session bus — journal/webhook
		// are the reliable channels there.
		urgency := "critical"
		if event == "resolved" {
			urgency = "normal"
		}
		cmd := exec.Command("notify-send", "-u", urgency, "HW-T: "+rs.rule.Name+" "+event, msg)
		go cmd.Run()

	case strings.HasPrefix(action, "exec:"):
		now := time.Now()
		if now.Sub(rs.lastExec) < execRateLimit {
			return
		}
		rs.lastExec = now
		go runHook(strings.TrimPrefix(action, "exec:"), rs, event)

	case strings.HasPrefix(action, "webhook:"):
		go postWebhook(strings.TrimPrefix(action, "webhook:"), rs, event)

	default:
		slog.Warn("unknown alert action", "action", action, "alert", rs.rule.Name)
	}
}

func describeRule(r AlertRule) string {
	switch {
	case r.Above != nil:
		return fmt.Sprintf("above %g for %s", *r.Above, r.For)
	case r.Below != nil:
		return fmt.Sprintf("below %g for %s", *r.Below, r.For)
	}
	return "no condition"
}

// runHook execs the configured command with alert context in the
// environment. Per §9, hooks never run with root privileges: a root daemon
// drops to nobody.
func runHook(command string, rs *ruleState, event string) {
	cmd := exec.Command("/bin/sh", "-c", command)
	cmd.Env = append(os.Environ(),
		"HWT_ALERT="+rs.rule.Name,
		"HWT_SENSOR="+string(rs.rule.Sensor),
		"HWT_EVENT="+event,
		fmt.Sprintf("HWT_VALUE=%g", rs.value),
	)
	if os.Geteuid() == 0 {
		cmd.SysProcAttr = &syscall.SysProcAttr{
			Credential: &syscall.Credential{Uid: 65534, Gid: 65534}, // nobody
		}
	}
	if err := cmd.Run(); err != nil {
		slog.Warn("alert exec hook failed", "alert", rs.rule.Name, "err", err)
	}
}

func postWebhook(url string, rs *ruleState, event string) {
	payload, _ := json.Marshal(map[string]any{
		"alert":  rs.rule.Name,
		"sensor": rs.rule.Sensor,
		"event":  event,
		"value":  rs.value,
		"ts":     time.Now().Format(time.RFC3339),
	})
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Post(url, "application/json", bytes.NewReader(payload))
	if err != nil {
		slog.Warn("alert webhook failed", "alert", rs.rule.Name, "err", err)
		return
	}
	resp.Body.Close()
}
