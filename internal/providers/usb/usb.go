// Package usb walks /sys/bus/usb/devices (§5.6): device identity from
// sysfs attributes with names resolved via the hwdata usb.ids database.
// Pure inventory — no live channels — and unprivileged.
package usb

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
	"github.com/zen66ten/HW-T/internal/hwids"
)

var idsPaths = []string{
	"/usr/share/hwdata/usb.ids",
	"/usr/share/misc/usb.ids",
	"/usr/share/usb.ids",
}

// deviceDirRE matches USB device directories: root hubs ("usb1") and
// devices by port path ("1-4", "1-4.2"). Interface directories ("1-4:1.0")
// are excluded.
var deviceDirRE = regexp.MustCompile(`^(usb\d+|\d+-\d+(\.\d+)*)$`)

// classNames covers the common bDeviceClass values worth displaying
// (full decode belongs to the ids database's interface-level data).
var classNames = map[string]string{
	"00": "(defined at interface level)",
	"02": "Communications",
	"03": "HID",
	"08": "Mass Storage",
	"09": "Hub",
	"0e": "Video",
	"e0": "Wireless",
	"ef": "Miscellaneous",
	"ff": "Vendor Specific",
}

// Provider exposes each USB device as inventory. Static: rediscovery on
// hotplug arrives with udev integration (§5.1, phase 4).
type Provider struct {
	root string
}

func New(sysfsRoot string) *Provider {
	return &Provider{root: sysfsRoot + "/bus/usb/devices"}
}

func (p *Provider) Name() string                   { return "usb" }
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
		return nil, fmt.Errorf("usb: reading %s: %w", p.root, err)
	}
	db := hwids.Load(idsPaths...)

	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if deviceDirRE.MatchString(e.Name()) {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	devs := make([]core.Device, 0, len(names))
	for _, port := range names {
		dir := filepath.Join(p.root, port)

		vendorID := readHex16(filepath.Join(dir, "idVendor"))
		productID := readHex16(filepath.Join(dir, "idProduct"))
		if vendorID == 0 && productID == 0 {
			continue // interface-less oddity or unreadable
		}

		attrs := map[string]string{
			"port":       port,
			"vendor_id":  fmt.Sprintf("%04x", vendorID),
			"product_id": fmt.Sprintf("%04x", productID),
			"vendor":     db.Vendor(vendorID),
		}
		if v, err := readString(filepath.Join(dir, "manufacturer")); err == nil {
			attrs["manufacturer"] = v
		}
		if v, err := readString(filepath.Join(dir, "serial")); err == nil {
			attrs["serial"] = v
		}
		if v, err := readString(filepath.Join(dir, "speed")); err == nil {
			attrs["speed"] = v + " Mbps"
		}
		if v, err := readString(filepath.Join(dir, "version")); err == nil {
			attrs["usb_version"] = v
		}
		if v, err := readString(filepath.Join(dir, "bDeviceClass")); err == nil {
			if name, ok := classNames[strings.ToLower(v)]; ok {
				attrs["class"] = name
			} else {
				attrs["class"] = "class " + v
			}
		}

		// Best display name: product string from the device itself, then
		// the ids database, then raw IDs.
		name, err := readString(filepath.Join(dir, "product"))
		if err != nil || name == "" {
			name = db.Product(vendorID, productID)
		}

		devs = append(devs, core.Device{
			ID:       core.DeviceID("usb:" + port),
			Provider: "usb",
			Name:     name,
			Attrs:    attrs,
		})
	}
	return devs, nil
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
	v, _ := strconv.ParseUint(s, 16, 16)
	return uint16(v)
}
