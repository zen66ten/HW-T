package smart

import (
	"context"
	"fmt"
	"time"

	"github.com/zen66ten/HW-T/internal/core"
)

// Provider exposes drive health. Identity (model/serial/firmware, stable
// ID) comes entirely from sysfs and needs no privilege; health values come
// from smartctl and need root or disk-group membership — Collect reports a
// clear permission error per channel rather than failing the whole device
// when unprivileged.
type Provider struct {
	sysfsRoot string
	interval  time.Duration
	disks     map[core.DeviceID]Disk
}

// New builds the SMART provider. interval <= 0 uses the slow-group default
// (60s per §4.3) since SMART queries must never spin up a standby disk on
// every tick.
func New(sysfsRoot string, interval time.Duration) *Provider {
	if interval <= 0 {
		interval = 60 * time.Second
	}
	return &Provider{sysfsRoot: sysfsRoot, interval: interval, disks: map[core.DeviceID]Disk{}}
}

func (p *Provider) Name() string                   { return "smart" }
func (p *Provider) DefaultInterval() time.Duration { return p.interval }

func (p *Provider) Discover(ctx context.Context) ([]core.Device, error) {
	disks, err := DiscoverDisks(p.sysfsRoot)
	if err != nil {
		return nil, err
	}

	p.disks = map[core.DeviceID]Disk{}
	devs := make([]core.Device, 0, len(disks))
	for _, d := range disks {
		id := core.DeviceID("smart:" + d.StableID)
		p.disks[id] = d
		name := d.Model
		if name == "" {
			name = d.Name
		}
		devs = append(devs, core.Device{
			ID:       id,
			Provider: "smart",
			Name:     name,
			Attrs:    attrsOf(d),
			Channels: []core.ChannelInfo{
				{ID: "health", Kind: core.KindHealth, Label: "Health"},
				{ID: "temp", Kind: core.KindTemp, Label: "Temperature"},
				{ID: "percent_used", Kind: core.KindPercent, Label: "Percentage Used"},
				{ID: "media_errors", Kind: core.KindCount, Label: "Media Errors"},
				{ID: "power_on_hours", Kind: core.KindCount, Label: "Power-On Hours"},
			},
		})
	}
	return devs, nil
}

// attrsOf builds the inventory attributes for a disk, dropping empty ones.
func attrsOf(d Disk) map[string]string {
	kind := "SSD"
	if d.Rotational {
		kind = "HDD"
	}
	attrs := map[string]string{
		"device":       d.Path,
		"serial":       d.Serial,
		"firmware_rev": d.FirmwareRev,
		"capacity":     d.Capacity,
		"type":         kind,
	}
	for k, v := range attrs {
		if v == "" {
			delete(attrs, k)
		}
	}
	return attrs
}

func (p *Provider) Collect(ctx context.Context, dev core.DeviceID) ([]core.Reading, error) {
	d, ok := p.disks[dev]
	if !ok {
		return nil, fmt.Errorf("smart: unknown device %s", dev)
	}

	raw, runErr := runSmartctl(ctx, d.Path)
	if runErr != nil {
		return errorReadings(runErr.Error()), nil
	}
	h, err := ParseHealth(raw)
	if err != nil {
		return errorReadings(err.Error()), nil
	}
	if h.Skipped {
		return errorReadings("standby (skipped to avoid spin-up)"), nil
	}
	if !h.PassedKnown && !h.TempKnown && !h.PercentUsedKnown {
		msg := h.ErrorMessage
		if msg == "" {
			msg = "no health data reported"
		}
		return errorReadings(msg), nil
	}

	out := make([]core.Reading, 0, 5)
	out = append(out, boolReading("health", h.PassedKnown, h.Passed))
	out = append(out, numReading("temp", h.TempKnown, h.TempC))
	out = append(out, numReading("percent_used", h.PercentUsedKnown, h.PercentUsed))
	out = append(out, numReading("media_errors", true, float64(h.MediaErrors)))
	out = append(out, numReading("power_on_hours", true, float64(h.PowerOnHours)))
	return out, nil
}

func errorReadings(msg string) []core.Reading {
	ids := []string{"health", "temp", "percent_used", "media_errors", "power_on_hours"}
	out := make([]core.Reading, len(ids))
	for i, id := range ids {
		out[i] = core.Reading{Channel: id, Err: msg}
	}
	return out
}

func boolReading(id string, known, v bool) core.Reading {
	if !known {
		return core.Reading{Channel: id, Err: "not reported by device"}
	}
	val := 0.0
	if v {
		val = 1
	}
	return core.Reading{Channel: id, Value: val}
}

func numReading(id string, known bool, v float64) core.Reading {
	if !known {
		return core.Reading{Channel: id, Err: "not reported by device"}
	}
	return core.Reading{Channel: id, Value: v}
}
