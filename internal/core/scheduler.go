package core

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// maxStrikes is how many consecutive panics quarantine a provider.
const maxStrikes = 3

// Scheduler runs discovery once per provider, then polls each provider's
// devices at its own DefaultInterval (poll groups per §4.3: fast/medium/slow
// providers each configure their own cadence at construction time; the
// scheduler never overrides it, so a slow SMART poll never gets forced onto
// a 1s cadence just because hwmon wants one). Providers are crash-isolated:
// a panic is recovered, and repeated panics quarantine the provider without
// touching the daemon or its siblings.
type Scheduler struct {
	reg       *Registry
	providers []Provider
}

func NewScheduler(reg *Registry, providers ...Provider) *Scheduler {
	return &Scheduler{reg: reg, providers: providers}
}

// Start discovers all providers and launches their poll loops. It returns
// after discovery; loops stop when ctx is cancelled.
func (s *Scheduler) Start(ctx context.Context) {
	for _, p := range s.providers {
		devs, err := s.discover(ctx, p)
		if err != nil {
			slog.Warn("provider discovery failed", "provider", p.Name(), "err", err)
			s.reg.SetQuarantined(p.Name(), err.Error())
			continue
		}
		s.reg.SetDevices(p.Name(), devs)
		slog.Info("provider discovered", "provider", p.Name(), "devices", len(devs))

		interval := p.DefaultInterval()
		if interval == 0 {
			continue // static inventory: no poll loop
		}
		go s.poll(ctx, p, devs, interval)
	}
}

func (s *Scheduler) discover(ctx context.Context, p Provider) (devs []Device, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic during discovery: %v", r)
		}
	}()
	return p.Discover(ctx)
}

func (s *Scheduler) poll(ctx context.Context, p Provider, devs []Device, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	strikes := 0
	for {
		if err := s.collectAll(ctx, p, devs); err != nil {
			strikes++
			slog.Error("provider collect panicked", "provider", p.Name(), "strike", strikes, "err", err)
			if strikes >= maxStrikes {
				s.reg.SetQuarantined(p.Name(), err.Error())
				slog.Error("provider quarantined", "provider", p.Name())
				return
			}
		} else {
			strikes = 0
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// collectAll runs one collection pass; a panic anywhere in the provider is
// converted to an error.
func (s *Scheduler) collectAll(ctx context.Context, p Provider, devs []Device) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic during collect: %v", r)
		}
	}()
	for _, d := range devs {
		if len(d.Channels) == 0 {
			continue
		}
		readings, cerr := p.Collect(ctx, d.ID)
		if cerr != nil {
			slog.Debug("collect failed", "provider", p.Name(), "device", d.ID, "err", cerr)
			continue
		}
		s.reg.Apply(d.ID, readings, time.Now())
	}
	return nil
}
