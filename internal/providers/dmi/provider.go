package dmi

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/zen66ten/HW-T/internal/core"
)

// Provider exposes SMBIOS inventory as channel-less devices. Static: the
// scheduler discovers it once and never polls it. Reading the table needs
// root (the sysfs file is 0400); without it the provider reports the error
// and the daemon carries on.
type Provider struct {
	table string
}

func New(sysfsRoot string) *Provider {
	return &Provider{table: sysfsRoot + "/firmware/dmi/tables/DMI"}
}

func (p *Provider) Name() string                   { return "dmi" }
func (p *Provider) DefaultInterval() time.Duration { return 0 }

func (p *Provider) Collect(ctx context.Context, dev core.DeviceID) ([]core.Reading, error) {
	return nil, nil
}

func (p *Provider) Discover(ctx context.Context) ([]core.Device, error) {
	raw, err := os.ReadFile(p.table)
	if err != nil {
		return nil, fmt.Errorf("dmi: %w (root required to read SMBIOS tables)", err)
	}
	info, err := Parse(raw)
	if err != nil {
		return nil, err
	}

	var devs []core.Device
	add := func(id, name string, attrs map[string]string) {
		for k, v := range attrs {
			if v == "" {
				delete(attrs, k)
			}
		}
		devs = append(devs, core.Device{
			ID:       core.DeviceID("dmi:" + id),
			Provider: "dmi",
			Name:     name,
			Attrs:    attrs,
		})
	}

	add("bios", "BIOS", map[string]string{
		"vendor":  info.BIOS.Vendor,
		"version": info.BIOS.Version,
		"date":    info.BIOS.Date,
	})
	add("system", "System", map[string]string{
		"manufacturer": info.System.Manufacturer,
		"product":      info.System.Product,
		"version":      info.System.Version,
		"serial":       info.System.Serial,
		"uuid":         info.System.UUID,
		"family":       info.System.Family,
	})
	add("board", "Baseboard", map[string]string{
		"manufacturer": info.Board.Manufacturer,
		"product":      info.Board.Product,
		"version":      info.Board.Version,
		"serial":       info.Board.Serial,
	})
	for i, proc := range info.Processors {
		add("cpu"+strconv.Itoa(i), proc.Version, map[string]string{
			"socket":        proc.Socket,
			"manufacturer":  proc.Manufacturer,
			"max_speed_mhz": strconv.Itoa(proc.MaxSpeedMHz),
			"cores":         strconv.Itoa(proc.Cores),
			"threads":       strconv.Itoa(proc.Threads),
		})
	}
	for i, m := range info.Memory {
		if m.SizeMB == 0 {
			continue // empty slot
		}
		add("mem"+strconv.Itoa(i), m.Locator, map[string]string{
			"bank":           m.BankLocator,
			"size_mb":        strconv.Itoa(m.SizeMB),
			"type":           m.Type,
			"speed_mts":      strconv.Itoa(m.SpeedMTs),
			"configured_mts": strconv.Itoa(m.ConfiguredMTs),
			"manufacturer":   m.Manufacturer,
			"part_number":    m.PartNumber,
		})
	}
	return devs, nil
}
