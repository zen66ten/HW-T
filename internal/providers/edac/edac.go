// Package edac reads ECC memory error counters from the kernel EDAC
// subsystem (§5.5): corrected (CE) and uncorrected (UE) counts per memory
// controller and per DIMM. This is the WHEA-analog error reporting HWiNFO
// users expect (§3). Machines without ECC (or without an EDAC driver)
// simply discover zero devices.
package edac

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/zen66ten/HW-T/internal/core"
)

// Provider exposes one device per memory controller with CE/UE channels,
// plus per-DIMM counters when the driver provides them.
type Provider struct {
	root     string
	interval time.Duration
	mcs      map[core.DeviceID]string // -> mc sysfs dir
}

// New builds the EDAC provider. interval <= 0 defaults to the slow group
// (60s); ECC error counters don't need sub-minute resolution.
func New(sysfsRoot string, interval time.Duration) *Provider {
	if interval <= 0 {
		interval = 60 * time.Second
	}
	return &Provider{
		root:     sysfsRoot + "/devices/system/edac/mc",
		interval: interval,
		mcs:      map[core.DeviceID]string{},
	}
}

func (p *Provider) Name() string                   { return "edac" }
func (p *Provider) DefaultInterval() time.Duration { return p.interval }

func (p *Provider) Discover(ctx context.Context) ([]core.Device, error) {
	entries, err := os.ReadDir(p.root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // no ECC / no EDAC driver: not an error
		}
		return nil, fmt.Errorf("edac: reading %s: %w", p.root, err)
	}

	var devs []core.Device
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), "mc") || !e.IsDir() {
			continue
		}
		dir := filepath.Join(p.root, e.Name())
		id := core.DeviceID("edac:" + e.Name())
		p.mcs[id] = dir

		dev := core.Device{
			ID:       id,
			Provider: "edac",
			Name:     "ECC " + e.Name(),
			Attrs:    map[string]string{"controller": e.Name()},
			Channels: []core.ChannelInfo{
				{ID: "ce", Kind: core.KindCount, Label: "Corrected Errors"},
				{ID: "ue", Kind: core.KindCount, Label: "Uncorrected Errors"},
			},
		}
		for _, dimm := range dimmDirs(dir) {
			label, _ := readString(filepath.Join(dir, dimm, "dimm_label"))
			if label == "" {
				label = dimm
			}
			dev.Channels = append(dev.Channels,
				core.ChannelInfo{ID: dimm + "_ce", Kind: core.KindCount, Label: label + " CE"},
				core.ChannelInfo{ID: dimm + "_ue", Kind: core.KindCount, Label: label + " UE"},
			)
		}
		devs = append(devs, dev)
	}
	sort.Slice(devs, func(i, j int) bool { return devs[i].ID < devs[j].ID })
	return devs, nil
}

func (p *Provider) Collect(ctx context.Context, dev core.DeviceID) ([]core.Reading, error) {
	dir, ok := p.mcs[dev]
	if !ok {
		return nil, fmt.Errorf("edac: unknown device %s", dev)
	}

	out := []core.Reading{
		countReading("ce", filepath.Join(dir, "ce_count")),
		countReading("ue", filepath.Join(dir, "ue_count")),
	}
	for _, dimm := range dimmDirs(dir) {
		out = append(out,
			countReading(dimm+"_ce", filepath.Join(dir, dimm, "dimm_ce_count")),
			countReading(dimm+"_ue", filepath.Join(dir, dimm, "dimm_ue_count")),
		)
	}
	return out, nil
}

func dimmDirs(mcDir string) []string {
	entries, err := os.ReadDir(mcDir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() && (strings.HasPrefix(e.Name(), "dimm") || strings.HasPrefix(e.Name(), "rank")) {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out
}

func countReading(channel, path string) core.Reading {
	r := core.Reading{Channel: channel}
	s, err := readString(path)
	if err != nil {
		r.Err = err.Error()
		return r
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		r.Err = err.Error()
		return r
	}
	r.Value = v
	return r
}

func readString(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}
