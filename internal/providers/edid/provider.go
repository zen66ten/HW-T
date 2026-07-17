package edid

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"time"

	"github.com/zen66ten/HW-T/internal/core"
)

// connectorRE matches DRM connector directories like "card0-DP-1",
// "card1-HDMI-A-2".
var connectorRE = regexp.MustCompile(`^card[0-9]+-.+$`)

// Provider exposes each connected monitor as an inventory device. Static;
// hotplug rediscovery arrives with udev integration (§5.1, phase 4).
type Provider struct {
	root string
}

func New(sysfsRoot string) *Provider {
	return &Provider{root: sysfsRoot + "/class/drm"}
}

func (p *Provider) Name() string                   { return "edid" }
func (p *Provider) DefaultInterval() time.Duration { return 0 }

func (p *Provider) Collect(ctx context.Context, dev core.DeviceID) ([]core.Reading, error) {
	return nil, nil
}

func (p *Provider) Discover(ctx context.Context) ([]core.Device, error) {
	entries, err := os.ReadDir(p.root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("edid: reading %s: %w", p.root, err)
	}

	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if connectorRE.MatchString(e.Name()) {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	var devs []core.Device
	for _, conn := range names {
		raw, err := os.ReadFile(filepath.Join(p.root, conn, "edid"))
		if err != nil || len(raw) == 0 {
			continue // disconnected connector: empty blob
		}
		info, err := Parse(raw)
		if err != nil {
			continue
		}

		attrs := map[string]string{
			"connector":    conn,
			"manufacturer": info.Manufacturer,
			"product_code": fmt.Sprintf("%04x", info.ProductCode),
		}
		if info.SerialString != "" {
			attrs["serial"] = info.SerialString
		} else if info.Serial != 0 {
			attrs["serial"] = strconv.FormatUint(uint64(info.Serial), 10)
		}
		if mode := info.ModeString(); mode != "" {
			attrs["native_mode"] = mode
		}
		if size := info.SizeString(); size != "" {
			attrs["size"] = size
		}
		if info.Year > 1990 {
			attrs["manufactured"] = fmt.Sprintf("%d week %d", info.Year, info.Week)
		}
		if info.Digital {
			attrs["signal"] = "digital"
		} else {
			attrs["signal"] = "analog"
		}

		name := info.Name
		if name == "" {
			name = info.Manufacturer + " " + fmt.Sprintf("%04x", info.ProductCode)
		}
		devs = append(devs, core.Device{
			ID:       core.DeviceID("edid:" + conn),
			Provider: "edid",
			Name:     name,
			Attrs:    attrs,
		})
	}
	return devs, nil
}
