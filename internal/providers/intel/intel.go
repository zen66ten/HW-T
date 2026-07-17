// Package intel reads Intel GPU clocks from DRM sysfs (SPEC §5.3). It
// covers both integrated graphics and Arc discrete cards, across the three
// sysfs layouts the drivers have used:
//
//   - xe driver:        device/tile0/gt0/freq0/{act,cur,min,max}_freq
//   - i915, multi-gt:   gt/gt0/rps_{act,cur,min,max}_freq_mhz
//   - i915, legacy:     gt_{act,cur,min,max}_freq_mhz
//
// Power, energy and (on Arc) temperature arrive separately through the
// hwmon backbone. Busy percent and VRAM have no clean sysfs interface and
// are tracked as known gaps.
package intel

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/zen66ten/HW-T/internal/core"
)

var cardRE = regexp.MustCompile(`^card[0-9]+$`)

var pciAddrRE = regexp.MustCompile(`^[0-9a-f]{4}:[0-9a-f]{2}:[0-9a-f]{2}\.[0-9a-f]$`)

// gtFreq locates one graphics-tile frequency interface.
type gtFreq struct {
	Name    string // "gt0", "gt1", or "" for the legacy single interface
	ActPath string // actual (effective) frequency, MHz
	CurPath string // requested frequency, MHz
	MinMHz  float64
	MaxMHz  float64
}

// Card is one Intel-driven DRM card.
type Card struct {
	Name       string
	Driver     string // "i915" or "xe"
	StablePath string
	GTs        []gtFreq
}

// DiscoverCards finds DRM cards driven by i915 or xe under sysfsRoot.
func DiscoverCards(sysfsRoot string) ([]Card, error) {
	dir := sysfsRoot + "/class/drm"
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("intel: reading %s: %w", dir, err)
	}

	var cards []Card
	for _, e := range entries {
		if !cardRE.MatchString(e.Name()) {
			continue
		}
		cardDir := filepath.Join(dir, e.Name())
		devDir := filepath.Join(cardDir, "device")
		driver := ""
		if link, err := os.Readlink(filepath.Join(devDir, "driver")); err == nil {
			driver = filepath.Base(link)
		}
		if driver != "i915" && driver != "xe" {
			continue
		}

		c := Card{
			Name:       e.Name(),
			Driver:     driver,
			StablePath: stablePath(devDir, e.Name()),
			GTs:        findGTs(cardDir, driver),
		}
		if len(c.GTs) > 0 {
			cards = append(cards, c)
		}
	}
	return cards, nil
}

func findGTs(cardDir, driver string) []gtFreq {
	if driver == "xe" {
		return findXeGTs(cardDir)
	}

	// i915 multi-gt layout.
	var gts []gtFreq
	if entries, err := os.ReadDir(filepath.Join(cardDir, "gt")); err == nil {
		for _, e := range entries {
			if !strings.HasPrefix(e.Name(), "gt") {
				continue
			}
			base := filepath.Join(cardDir, "gt", e.Name())
			g := gtFreq{
				Name:    e.Name(),
				ActPath: filepath.Join(base, "rps_act_freq_mhz"),
				CurPath: filepath.Join(base, "rps_cur_freq_mhz"),
			}
			if exists(g.CurPath) {
				g.MinMHz, _ = readMHz(filepath.Join(base, "rps_min_freq_mhz"))
				g.MaxMHz, _ = readMHz(filepath.Join(base, "rps_max_freq_mhz"))
				gts = append(gts, g)
			}
		}
	}
	if len(gts) > 0 {
		sort.Slice(gts, func(i, j int) bool { return gts[i].Name < gts[j].Name })
		return gts
	}

	// i915 legacy single-gt layout.
	g := gtFreq{
		ActPath: filepath.Join(cardDir, "gt_act_freq_mhz"),
		CurPath: filepath.Join(cardDir, "gt_cur_freq_mhz"),
	}
	if exists(g.CurPath) {
		g.MinMHz, _ = readMHz(filepath.Join(cardDir, "gt_min_freq_mhz"))
		g.MaxMHz, _ = readMHz(filepath.Join(cardDir, "gt_max_freq_mhz"))
		return []gtFreq{g}
	}
	return nil
}

func findXeGTs(cardDir string) []gtFreq {
	var gts []gtFreq
	tiles, err := os.ReadDir(filepath.Join(cardDir, "device"))
	if err != nil {
		return nil
	}
	for _, tile := range tiles {
		if !strings.HasPrefix(tile.Name(), "tile") {
			continue
		}
		tileDir := filepath.Join(cardDir, "device", tile.Name())
		gtDirs, err := os.ReadDir(tileDir)
		if err != nil {
			continue
		}
		for _, gt := range gtDirs {
			if !strings.HasPrefix(gt.Name(), "gt") {
				continue
			}
			base := filepath.Join(tileDir, gt.Name(), "freq0")
			g := gtFreq{
				Name:    gt.Name(),
				ActPath: filepath.Join(base, "act_freq"),
				CurPath: filepath.Join(base, "cur_freq"),
			}
			if exists(g.CurPath) {
				g.MinMHz, _ = readMHz(filepath.Join(base, "min_freq"))
				g.MaxMHz, _ = readMHz(filepath.Join(base, "max_freq"))
				gts = append(gts, g)
			}
		}
	}
	sort.Slice(gts, func(i, j int) bool { return gts[i].Name < gts[j].Name })
	return gts
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

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func readMHz(path string) (float64, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return strconv.ParseFloat(strings.TrimSpace(string(b)), 64)
}

// Provider exposes the clock channels for every Intel card.
type Provider struct {
	root     string
	interval time.Duration
	cards    map[core.DeviceID]Card
}

// New builds the Intel GPU provider. interval <= 0 uses the fast-group
// default (1s); these are plain sysfs reads.
func New(sysfsRoot string, interval time.Duration) *Provider {
	if interval <= 0 {
		interval = time.Second
	}
	return &Provider{root: sysfsRoot, interval: interval, cards: map[core.DeviceID]Card{}}
}

func (p *Provider) Name() string                   { return "intel" }
func (p *Provider) DefaultInterval() time.Duration { return p.interval }

func (p *Provider) Discover(ctx context.Context) ([]core.Device, error) {
	cards, err := DiscoverCards(p.root)
	if err != nil {
		return nil, err
	}

	p.cards = map[core.DeviceID]Card{}
	devs := make([]core.Device, 0, len(cards))
	for _, c := range cards {
		id := core.DeviceID("intel:" + c.StablePath)
		p.cards[id] = c
		dev := core.Device{
			ID:       id,
			Provider: "intel",
			Name:     "Intel GPU (" + c.Driver + ")",
			Attrs:    map[string]string{"card": c.Name, "driver": c.Driver, "path": c.StablePath},
		}
		for _, g := range c.GTs {
			prefix, label := "gpu", "GPU Clock"
			if g.Name != "" && len(c.GTs) > 1 {
				prefix, label = g.Name, strings.ToUpper(g.Name)+" Clock"
			}
			limits := map[string]float64{}
			if g.MaxMHz > 0 {
				limits["min"], limits["max"] = g.MinMHz, g.MaxMHz
			}
			dev.Channels = append(dev.Channels,
				core.ChannelInfo{ID: prefix + "_act", Kind: core.KindFreq, Label: label, Limits: limits},
				core.ChannelInfo{ID: prefix + "_cur", Kind: core.KindFreq, Label: label + " (Requested)"},
			)
		}
		devs = append(devs, dev)
	}
	return devs, nil
}

func (p *Provider) Collect(ctx context.Context, dev core.DeviceID) ([]core.Reading, error) {
	c, ok := p.cards[dev]
	if !ok {
		return nil, fmt.Errorf("intel: unknown device %s", dev)
	}
	var out []core.Reading
	for _, g := range c.GTs {
		prefix := "gpu"
		if g.Name != "" && len(c.GTs) > 1 {
			prefix = g.Name
		}
		out = append(out, freqReading(prefix+"_act", g.ActPath), freqReading(prefix+"_cur", g.CurPath))
	}
	return out, nil
}

func freqReading(channel, path string) core.Reading {
	r := core.Reading{Channel: channel}
	mhz, err := readMHz(path)
	if err != nil {
		r.Err = err.Error()
		return r
	}
	r.Value = mhz
	return r
}
