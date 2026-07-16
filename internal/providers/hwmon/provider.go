package hwmon

import (
	"context"
	"fmt"
	"time"

	"github.com/zen66ten/HW-T/internal/core"
)

// Provider adapts the hwmon walker to the core provider contract.
type Provider struct {
	root  string
	chips map[core.DeviceID]Chip
}

func New(sysfsRoot string) *Provider {
	return &Provider{
		root:  sysfsRoot + "/class/hwmon",
		chips: map[core.DeviceID]Chip{},
	}
}

func (p *Provider) Name() string                   { return "hwmon" }
func (p *Provider) DefaultInterval() time.Duration { return time.Second }

func (p *Provider) Discover(ctx context.Context) ([]core.Device, error) {
	chips, err := Discover(p.root)
	if err != nil {
		return nil, err
	}
	devs := make([]core.Device, 0, len(chips))
	for _, chip := range chips {
		id := core.DeviceID("hwmon:" + chip.StablePath)
		p.chips[id] = chip
		dev := core.Device{
			ID:       id,
			Provider: "hwmon",
			Name:     chip.Name,
			Attrs:    map[string]string{"path": chip.StablePath, "hwmon": chip.Hwmon},
		}
		for _, ch := range chip.Channels {
			dev.Channels = append(dev.Channels, core.ChannelInfo{
				ID:     ch.ID,
				Kind:   core.Kind(ch.Kind),
				Label:  ch.Label,
				Limits: ch.Limits,
			})
		}
		devs = append(devs, dev)
	}
	return devs, nil
}

func (p *Provider) Collect(ctx context.Context, dev core.DeviceID) ([]core.Reading, error) {
	chip, ok := p.chips[dev]
	if !ok {
		return nil, fmt.Errorf("hwmon: unknown device %s", dev)
	}
	readings := Collect(chip)
	out := make([]core.Reading, 0, len(readings))
	for _, r := range readings {
		cr := core.Reading{Channel: r.Channel.ID, Value: r.Value}
		if r.Err != nil {
			cr.Err = r.Err.Error()
		}
		out = append(out, cr)
	}
	return out, nil
}
