package nvidia

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/zen66ten/HW-T/internal/core"
)

// Provider exposes each NVIDIA GPU as one device keyed by its UUID.
// Discovery fails cleanly (provider quarantined with a clear reason) when
// nvidia-smi is absent or no driver is loaded.
type Provider struct {
	interval time.Duration
	uuids    map[core.DeviceID]string

	// One nvidia-smi exec serves all GPUs in a collection pass: the result
	// is cached briefly so N devices don't cost N execs per tick.
	cached   []GPU
	cachedAt time.Time
}

// New builds the NVIDIA provider. interval <= 0 uses the medium-group
// default (2s per §4.3: NVML/nvidia-smi queries cost more than sysfs
// reads).
func New(interval time.Duration) *Provider {
	if interval <= 0 {
		interval = 2 * time.Second
	}
	return &Provider{interval: interval, uuids: map[core.DeviceID]string{}}
}

func (p *Provider) Name() string                   { return "nvidia" }
func (p *Provider) DefaultInterval() time.Duration { return p.interval }

func (p *Provider) Discover(ctx context.Context) ([]core.Device, error) {
	if _, err := exec.LookPath("nvidia-smi"); err != nil {
		return nil, fmt.Errorf("nvidia: nvidia-smi not found (no NVIDIA driver installed?)")
	}
	gpus, err := query(ctx)
	if err != nil {
		return nil, err
	}

	p.uuids = map[core.DeviceID]string{}
	devs := make([]core.Device, 0, len(gpus))
	for _, g := range gpus {
		id := core.DeviceID("nvidia:" + g.UUID)
		p.uuids[id] = g.UUID
		attrs := map[string]string{"uuid": g.UUID}
		if g.MemTotalMiB != nil {
			attrs["vram_total_mib"] = fmt.Sprintf("%.0f", *g.MemTotalMiB)
		}
		dev := core.Device{
			ID:       id,
			Provider: "nvidia",
			Name:     g.Name,
			Attrs:    attrs,
			Channels: []core.ChannelInfo{
				{ID: "temp", Kind: core.KindTemp, Label: "GPU Temp"},
				{ID: "power", Kind: core.KindPower, Label: "GPU Power", Limits: limitFrom(g.PowerLimitW)},
				{ID: "clock_graphics", Kind: core.KindFreq, Label: "Graphics Clock"},
				{ID: "clock_sm", Kind: core.KindFreq, Label: "SM Clock"},
				{ID: "clock_mem", Kind: core.KindFreq, Label: "Memory Clock"},
				{ID: "clock_video", Kind: core.KindFreq, Label: "Video Clock"},
				{ID: "util_gpu", Kind: core.KindPercent, Label: "GPU Util"},
				{ID: "util_mem", Kind: core.KindPercent, Label: "Memory Util"},
				{ID: "vram_used", Kind: core.KindData, Label: "VRAM Used"},
				{ID: "fan", Kind: core.KindPercent, Label: "Fan"},
			},
		}
		devs = append(devs, dev)
	}
	return devs, nil
}

func limitFrom(w *float64) map[string]float64 {
	if w == nil {
		return nil
	}
	return map[string]float64{"max": *w}
}

func (p *Provider) Collect(ctx context.Context, dev core.DeviceID) ([]core.Reading, error) {
	uuid, ok := p.uuids[dev]
	if !ok {
		return nil, fmt.Errorf("nvidia: unknown device %s", dev)
	}
	gpus := p.cached
	if time.Since(p.cachedAt) > 500*time.Millisecond {
		var err error
		gpus, err = query(ctx)
		if err != nil {
			return nil, err
		}
		p.cached, p.cachedAt = gpus, time.Now()
	}
	for _, g := range gpus {
		if g.UUID != uuid {
			continue
		}
		return []core.Reading{
			reading("temp", g.TempC),
			reading("power", g.PowerW),
			reading("clock_graphics", g.GraphicsMHz),
			reading("clock_sm", g.SMMHz),
			reading("clock_mem", g.MemMHz),
			reading("clock_video", g.VideoMHz),
			reading("util_gpu", g.UtilGPU),
			reading("util_mem", g.UtilMem),
			reading("vram_used", g.MemUsedMiB),
			reading("fan", g.FanPercent),
		}, nil
	}
	return nil, fmt.Errorf("nvidia: GPU %s no longer reported by nvidia-smi", uuid)
}

func reading(id string, v *float64) core.Reading {
	if v == nil {
		return core.Reading{Channel: id, Err: "not supported on this GPU/driver"}
	}
	return core.Reading{Channel: id, Value: *v}
}
