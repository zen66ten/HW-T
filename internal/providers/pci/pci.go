// Package pci walks /sys/bus/pci/devices (§5.6): identity from config-space
// sysfs attributes with names resolved via the hwdata pci.ids database, link
// speed/width, driver binding, IOMMU group, and PCIe AER error counters as
// live sensor channels.
package pci

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
	"github.com/zen66ten/HW-T/internal/hwids"
)

// idsPaths are tried in order for the pci.ids database.
var idsPaths = []string{
	"/usr/share/hwdata/pci.ids",
	"/usr/share/misc/pci.ids",
	"/usr/share/pci.ids",
}

// aerFiles maps sensor channel IDs to the sysfs AER counter files. Each
// file's TOTAL_ERR_* line carries the running sum.
var aerFiles = map[string]string{
	"aer_correctable": "aer_dev_correctable",
	"aer_nonfatal":    "aer_dev_nonfatal",
	"aer_fatal":       "aer_dev_fatal",
}

// Provider exposes every PCI function as an inventory device; functions
// with AER reporting additionally get error-counter channels polled on the
// slow cadence (errors are rare, reads are informational).
type Provider struct {
	root     string
	interval time.Duration
	devices  map[core.DeviceID]string // -> sysfs device dir
}

// New builds the PCI provider. interval <= 0 defaults to the slow group
// (60s): only AER counters are polled, inventory is static.
func New(sysfsRoot string, interval time.Duration) *Provider {
	if interval <= 0 {
		interval = 60 * time.Second
	}
	return &Provider{
		root:     sysfsRoot + "/bus/pci/devices",
		interval: interval,
		devices:  map[core.DeviceID]string{},
	}
}

func (p *Provider) Name() string                   { return "pci" }
func (p *Provider) DefaultInterval() time.Duration { return p.interval }

func (p *Provider) Discover(ctx context.Context) ([]core.Device, error) {
	entries, err := os.ReadDir(p.root)
	if err != nil {
		return nil, fmt.Errorf("pci: reading %s: %w", p.root, err)
	}
	db := hwids.Load(idsPaths...)

	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)

	p.devices = map[core.DeviceID]string{}
	devs := make([]core.Device, 0, len(names))
	for _, addr := range names {
		dir := filepath.Join(p.root, addr)
		vendorID := readHex16(filepath.Join(dir, "vendor"))
		deviceID := readHex16(filepath.Join(dir, "device"))
		classCode := readHex32(filepath.Join(dir, "class"))

		id := core.DeviceID("pci:" + addr)
		p.devices[id] = dir

		attrs := map[string]string{
			"address":   addr,
			"vendor":    db.Vendor(vendorID),
			"vendor_id": fmt.Sprintf("%04x", vendorID),
			"device_id": fmt.Sprintf("%04x", deviceID),
			"class":     db.ClassName(classCode),
		}
		if rev, err := readString(filepath.Join(dir, "revision")); err == nil {
			attrs["revision"] = rev
		}
		if cur := linkString(dir, "current"); cur != "" {
			attrs["link"] = cur
		}
		if max := linkString(dir, "max"); max != "" {
			attrs["link_max"] = max
		}
		if drv, err := os.Readlink(filepath.Join(dir, "driver")); err == nil {
			attrs["driver"] = filepath.Base(drv)
		}
		if grp, err := os.Readlink(filepath.Join(dir, "iommu_group")); err == nil {
			attrs["iommu_group"] = filepath.Base(grp)
		}

		dev := core.Device{
			ID:       id,
			Provider: "pci",
			Name:     db.Product(vendorID, deviceID),
			Attrs:    attrs,
		}
		if _, err := os.Stat(filepath.Join(dir, "aer_dev_correctable")); err == nil {
			for chID := range aerFiles {
				dev.Channels = append(dev.Channels, core.ChannelInfo{
					ID:    chID,
					Kind:  core.KindCount,
					Label: strings.ReplaceAll(chID, "_", " "),
				})
			}
			sort.Slice(dev.Channels, func(i, j int) bool { return dev.Channels[i].ID < dev.Channels[j].ID })
		}
		devs = append(devs, dev)
	}
	return devs, nil
}

func (p *Provider) Collect(ctx context.Context, dev core.DeviceID) ([]core.Reading, error) {
	dir, ok := p.devices[dev]
	if !ok {
		return nil, fmt.Errorf("pci: unknown device %s", dev)
	}
	out := make([]core.Reading, 0, len(aerFiles))
	for chID, file := range aerFiles {
		r := core.Reading{Channel: chID}
		total, err := readAERTotal(filepath.Join(dir, file))
		if err != nil {
			r.Err = err.Error()
		} else {
			r.Value = total
		}
		out = append(out, r)
	}
	return out, nil
}

// readAERTotal parses an aer_dev_* file: "RxErr 0\nBadTLP 3\n...\nTOTAL_ERR_COR 3".
func readAERTotal(path string) (float64, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	for _, line := range strings.Split(string(raw), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && strings.HasPrefix(fields[0], "TOTAL_ERR") {
			v, err := strconv.ParseFloat(fields[1], 64)
			return v, err
		}
	}
	return 0, fmt.Errorf("no TOTAL_ERR line in %s", filepath.Base(path))
}

// linkString formats "<prefix>_link_speed/width" as "16.0 GT/s x16".
func linkString(dir, prefix string) string {
	speed, err := readString(filepath.Join(dir, prefix+"_link_speed"))
	if err != nil {
		return ""
	}
	speed = strings.TrimSuffix(speed, " PCIe")
	width, err := readString(filepath.Join(dir, prefix+"_link_width"))
	if err != nil {
		return speed
	}
	return speed + " x" + width
}

func readString(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

func readHex16(path string) uint16 {
	s, err := readString(path)
	if err != nil {
		return 0
	}
	v, _ := strconv.ParseUint(strings.TrimPrefix(s, "0x"), 16, 16)
	return uint16(v)
}

func readHex32(path string) uint32 {
	s, err := readString(path)
	if err != nil {
		return 0
	}
	v, _ := strconv.ParseUint(strings.TrimPrefix(s, "0x"), 16, 32)
	return uint32(v)
}
