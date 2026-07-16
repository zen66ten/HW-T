// Package amdgpu reads the DRM-side telemetry the hwmon provider can't
// see: GPU busy percent, VRAM usage, and the active sclk/mclk DPM states
// (§5.3). Temperatures, fans and power for AMD GPUs arrive via the hwmon
// provider automatically; this provider only adds the DRM extras, so the
// two never duplicate channels.
package amdgpu

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/zen66ten/HW-T/internal/core"
)

var cardRE = regexp.MustCompile(`^card[0-9]+$`)

var pciAddrRE = regexp.MustCompile(`^[0-9a-f]{4}:[0-9a-f]{2}:[0-9a-f]{2}\.[0-9a-f]$`)

// Card is one amdgpu-driven DRM card.
type Card struct {
	Name       string // e.g. "card0"
	DeviceDir  string // .../class/drm/card0/device
	StablePath string // PCI address, e.g. "pci-0000:0c:00.0"
}

// DiscoverCards finds DRM cards driven by amdgpu under sysfsRoot. The
// gpu_busy_percent attribute only exists on amdgpu, so its presence is the
// driver check — no vendor-ID table needed.
func DiscoverCards(sysfsRoot string) ([]Card, error) {
	dir := sysfsRoot + "/class/drm"
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("amdgpu: reading %s: %w", dir, err)
	}

	var cards []Card
	for _, e := range entries {
		if !cardRE.MatchString(e.Name()) {
			continue
		}
		devDir := filepath.Join(dir, e.Name(), "device")
		if _, err := os.Stat(filepath.Join(devDir, "gpu_busy_percent")); err != nil {
			continue
		}
		cards = append(cards, Card{
			Name:       e.Name(),
			DeviceDir:  devDir,
			StablePath: stablePath(devDir, e.Name()),
		})
	}
	return cards, nil
}

func stablePath(devDir, fallback string) string {
	resolved, err := filepath.EvalSymlinks(devDir)
	if err != nil {
		return fallback
	}
	segs := strings.Split(resolved, "/")
	for i := len(segs) - 1; i >= 0; i-- {
		if pciAddrRE.MatchString(segs[i]) {
			return "pci-" + segs[i]
		}
	}
	return fallback
}

// ActiveDPMClockMHz parses a pp_dpm_sclk/pp_dpm_mclk table and returns the
// active state's frequency. Format, one line per power state:
//
//	0: 500Mhz
//	1: 2575Mhz *
//
// The trailing asterisk marks the active state.
func ActiveDPMClockMHz(table string) (float64, error) {
	for _, line := range strings.Split(table, "\n") {
		if !strings.HasSuffix(strings.TrimSpace(line), "*") {
			continue
		}
		_, rest, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		rest = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(rest), "*"))
		rest = strings.TrimSuffix(strings.ToLower(rest), "mhz")
		mhz, err := strconv.ParseFloat(strings.TrimSpace(rest), 64)
		if err != nil {
			return 0, fmt.Errorf("amdgpu: bad DPM line %q: %w", line, err)
		}
		return mhz, nil
	}
	return 0, fmt.Errorf("amdgpu: no active state in DPM table")
}

// Provider exposes the DRM extras for every amdgpu card.
type Provider struct {
	root     string
	interval time.Duration
	cards    map[core.DeviceID]Card
}

// New builds the amdgpu provider. interval <= 0 uses the fast-group
// default (1s per §4.3; these are plain sysfs reads).
func New(sysfsRoot string, interval time.Duration) *Provider {
	if interval <= 0 {
		interval = time.Second
	}
	return &Provider{root: sysfsRoot, interval: interval, cards: map[core.DeviceID]Card{}}
}

func (p *Provider) Name() string                   { return "amdgpu" }
func (p *Provider) DefaultInterval() time.Duration { return p.interval }

func (p *Provider) Discover(ctx context.Context) ([]core.Device, error) {
	cards, err := DiscoverCards(p.root)
	if err != nil {
		return nil, err
	}

	p.cards = map[core.DeviceID]Card{}
	devs := make([]core.Device, 0, len(cards))
	for _, c := range cards {
		id := core.DeviceID("amdgpu:" + c.StablePath)
		p.cards[id] = c
		attrs := map[string]string{"card": c.Name, "path": c.StablePath}
		if totalB, err := readUint(filepath.Join(c.DeviceDir, "mem_info_vram_total")); err == nil {
			attrs["vram_total_mib"] = strconv.FormatUint(totalB/(1024*1024), 10)
		}
		devs = append(devs, core.Device{
			ID:       id,
			Provider: "amdgpu",
			Name:     "amdgpu " + c.Name,
			Attrs:    attrs,
			Channels: []core.ChannelInfo{
				{ID: "busy", Kind: core.KindPercent, Label: "GPU Busy"},
				{ID: "vram_used", Kind: core.KindData, Label: "VRAM Used"},
				{ID: "sclk", Kind: core.KindFreq, Label: "GPU Clock"},
				{ID: "mclk", Kind: core.KindFreq, Label: "Memory Clock"},
			},
		})
	}
	return devs, nil
}

func (p *Provider) Collect(ctx context.Context, dev core.DeviceID) ([]core.Reading, error) {
	c, ok := p.cards[dev]
	if !ok {
		return nil, fmt.Errorf("amdgpu: unknown device %s", dev)
	}

	out := make([]core.Reading, 0, 4)

	busy := core.Reading{Channel: "busy"}
	if v, err := readUint(filepath.Join(c.DeviceDir, "gpu_busy_percent")); err != nil {
		busy.Err = err.Error()
	} else {
		busy.Value = float64(v)
	}
	out = append(out, busy)

	vram := core.Reading{Channel: "vram_used"}
	if v, err := readUint(filepath.Join(c.DeviceDir, "mem_info_vram_used")); err != nil {
		vram.Err = err.Error()
	} else {
		vram.Value = float64(v) / (1024 * 1024)
	}
	out = append(out, vram)

	for id, file := range map[string]string{"sclk": "pp_dpm_sclk", "mclk": "pp_dpm_mclk"} {
		r := core.Reading{Channel: id}
		raw, err := os.ReadFile(filepath.Join(c.DeviceDir, file))
		if err != nil {
			r.Err = err.Error()
		} else if mhz, err := ActiveDPMClockMHz(string(raw)); err != nil {
			r.Err = err.Error()
		} else {
			r.Value = mhz
		}
		out = append(out, r)
	}
	return out, nil
}

func readUint(path string) (uint64, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return strconv.ParseUint(strings.TrimSpace(string(b)), 10, 64)
}
