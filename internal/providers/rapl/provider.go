package rapl

import (
	"context"
	"fmt"
	"time"

	"github.com/zen66ten/HW-T/internal/core"
)

type zoneChannel struct {
	zone     Zone
	lastJ    float64
	lastTime time.Time
	lastSet  bool
}

// Provider groups each RAPL package into one device, with a channel per
// power domain (package, core, uncore, dram, psys). Power is the joule
// rate between successive Collect calls, so the first reading after
// (re)discovery has no delta yet and is reported as an error ("warming
// up") rather than a bogus zero.
type Provider struct {
	root     string
	interval time.Duration
	channels map[core.DeviceID]map[string]*zoneChannel
}

// New builds the RAPL provider. interval <= 0 uses the fast-group default
// (1s per §4.3) since power draw is most useful sampled often.
func New(sysfsRoot string, interval time.Duration) *Provider {
	if interval <= 0 {
		interval = time.Second
	}
	return &Provider{root: sysfsRoot, interval: interval, channels: map[core.DeviceID]map[string]*zoneChannel{}}
}

func (p *Provider) Name() string                   { return "rapl" }
func (p *Provider) DefaultInterval() time.Duration { return p.interval }

func (p *Provider) Discover(ctx context.Context) ([]core.Device, error) {
	zones, err := Discover(p.root)
	if err != nil {
		return nil, err
	}

	byPath := map[string]Zone{}
	for _, z := range zones {
		byPath[z.Path] = z
	}

	p.channels = map[core.DeviceID]map[string]*zoneChannel{}
	var devs []core.Device
	for _, z := range zones {
		if z.Parent != "" {
			continue // handled as a channel under its package below
		}
		id := core.DeviceID("rapl:" + z.Path)
		dev := core.Device{
			ID:       id,
			Provider: "rapl",
			Name:     z.Name,
			Attrs:    map[string]string{"path": z.Path},
			Channels: []core.ChannelInfo{{ID: "package", Kind: core.KindPower, Label: z.Name}},
		}
		chans := map[string]*zoneChannel{"package": {zone: z}}
		for _, c := range zones {
			if c.Parent == z.Path {
				dev.Channels = append(dev.Channels, core.ChannelInfo{ID: c.Name, Kind: core.KindPower, Label: c.Name})
				chans[c.Name] = &zoneChannel{zone: c}
			}
		}
		p.channels[id] = chans
		devs = append(devs, dev)
	}
	return devs, nil
}

func (p *Provider) Collect(ctx context.Context, dev core.DeviceID) ([]core.Reading, error) {
	chans, ok := p.channels[dev]
	if !ok {
		return nil, fmt.Errorf("rapl: unknown device %s", dev)
	}
	now := time.Now()
	out := make([]core.Reading, 0, len(chans))
	for id, ch := range chans {
		r := core.Reading{Channel: id}
		curJ, err := ReadEnergy(p.root, ch.zone)
		if err != nil {
			r.Err = err.Error()
			out = append(out, r)
			continue
		}
		if !ch.lastSet {
			ch.lastJ, ch.lastTime, ch.lastSet = curJ, now, true
			r.Err = "warming up (no delta yet)"
			out = append(out, r)
			continue
		}
		dt := now.Sub(ch.lastTime).Seconds()
		r.Value = Power(ch.lastJ, curJ, ch.zone.MaxEnergyRange, dt)
		ch.lastJ, ch.lastTime = curJ, now
		out = append(out, r)
	}
	return out, nil
}
