package cpu

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strconv"
	"time"

	"github.com/zen66ten/HW-T/internal/core"
)

const deviceID = core.DeviceID("cpu:cpufreq")

// Provider exposes per-CPU cpufreq clocks as freq channels on one device,
// plus an APERF/MPERF-derived effective-clock channel per core when the msr
// PMU is available and permitted (root/CAP_PERFMON) — see effclock.go.
type Provider struct {
	root     string
	interval time.Duration
	cpus     []int
	eff      *effClock
}

// New builds the cpu provider. interval <= 0 uses the fast-group default
// (1s per §4.3).
func New(sysfsRoot string, interval time.Duration) *Provider {
	if interval <= 0 {
		interval = time.Second
	}
	return &Provider{root: sysfsRoot + "/devices/system/cpu", interval: interval}
}

func (p *Provider) Name() string                   { return "cpu" }
func (p *Provider) DefaultInterval() time.Duration { return p.interval }

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

	if p.eff != nil {
		p.eff.Close()
		p.eff = nil
	}
	if eff, err := newEffClock(p.cpus); err != nil {
		slog.Warn("effective clock unavailable (needs root/CAP_PERFMON)", "err", err)
	} else {
		p.eff = eff
		for _, f := range freqs {
			dev.Channels = append(dev.Channels, core.ChannelInfo{
				ID:    "cpu" + strconv.Itoa(f.CPU) + "_eff",
				Kind:  core.KindFreq,
				Label: "cpu" + strconv.Itoa(f.CPU) + " (eff)",
			})
		}
	}

	return []core.Device{dev}, nil
}

func (p *Provider) Collect(ctx context.Context, dev core.DeviceID) ([]core.Reading, error) {
	if dev != deviceID {
		return nil, fmt.Errorf("cpu: unknown device %s", dev)
	}
	out := make([]core.Reading, 0, len(p.cpus)*2)
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

		if p.eff != nil {
			er := core.Reading{Channel: id + "_eff"}
			effMHz, err := p.eff.Read(n)
			if err != nil {
				er.Err = err.Error()
			} else {
				er.Value = effMHz
			}
			out = append(out, er)
		}
	}
	return out, nil
}
