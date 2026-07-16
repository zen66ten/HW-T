package cpu

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"time"

	"github.com/zen66ten/HW-T/internal/core"
)

const deviceID = core.DeviceID("cpu:cpufreq")

// Provider exposes per-CPU cpufreq clocks as freq channels on one device.
type Provider struct {
	root string
	cpus []int
}

func New(sysfsRoot string) *Provider {
	return &Provider{root: sysfsRoot + "/devices/system/cpu"}
}

func (p *Provider) Name() string                   { return "cpu" }
func (p *Provider) DefaultInterval() time.Duration { return time.Second }

func (p *Provider) Discover(ctx context.Context) ([]core.Device, error) {
	freqs, err := DiscoverFreq(p.root)
	if err != nil {
		return nil, err
	}
	if len(freqs) == 0 {
		return nil, nil
	}

	p.cpus = p.cpus[:0]
	dev := core.Device{
		ID:       deviceID,
		Provider: "cpu",
		Name:     "cpufreq",
		Attrs: map[string]string{
			"driver":   freqs[0].Driver,
			"governor": freqs[0].Governor,
		},
	}
	for _, f := range freqs {
		p.cpus = append(p.cpus, f.CPU)
		dev.Channels = append(dev.Channels, core.ChannelInfo{
			ID:    "cpu" + strconv.Itoa(f.CPU),
			Kind:  core.KindFreq,
			Label: "cpu" + strconv.Itoa(f.CPU),
			Limits: map[string]float64{
				"min": f.MinMHz,
				"max": f.MaxMHz,
			},
		})
	}
	return []core.Device{dev}, nil
}

func (p *Provider) Collect(ctx context.Context, dev core.DeviceID) ([]core.Reading, error) {
	if dev != deviceID {
		return nil, fmt.Errorf("cpu: unknown device %s", dev)
	}
	out := make([]core.Reading, 0, len(p.cpus))
	for _, n := range p.cpus {
		id := "cpu" + strconv.Itoa(n)
		path := filepath.Join(p.root, id, "cpufreq", "scaling_cur_freq")
		r := core.Reading{Channel: id}
		mhz, err := readKHz(path)
		if err != nil {
			r.Err = err.Error()
		} else {
			r.Value = mhz
		}
		out = append(out, r)
	}
	return out, nil
}
