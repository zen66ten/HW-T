// Package smart reads drive identity from sysfs (no root required) and
// full SMART/NVMe health via smartctl JSON (root or disk-group membership
// required for the actual command/data passthrough). Rationale for
// shelling out rather than a native parser: smartmontools' drivedb.h
// vendor-attribute decoding is 20+ years of accumulated knowledge that HW-T
// deliberately does not replicate (§5.4).
package smart

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// Disk is one identified block device, populated entirely from sysfs so
// discovery never needs smartctl or root.
type Disk struct {
	Name        string // e.g. "nvme0n1"
	Path        string // e.g. "/dev/nvme0n1"
	StableID    string // wwid when available, else "dev-<name>" (see §5.4 gap handling)
	Model       string
	Serial      string
	FirmwareRev string
	Capacity    string // human-readable, e.g. "1.0 TB"
	Rotational  bool   // true = HDD, false = SSD/NVMe
}

// humanCapacity converts a 512-byte sector count to a decimal-GB/TB string,
// matching how drive vendors and HWiNFO label capacity.
func humanCapacity(sectors int64) string {
	bytes := float64(sectors) * 512
	switch {
	case bytes >= 1e12:
		return fmt.Sprintf("%.1f TB", bytes/1e12)
	case bytes >= 1e9:
		return fmt.Sprintf("%.0f GB", bytes/1e9)
	default:
		return fmt.Sprintf("%.0f MB", bytes/1e6)
	}
}

// virtualPrefixes are block devices that are never physical drives worth
// SMART-querying: loopback, ramdisk/zram, device-mapper, software RAID,
// optical (sr*, never spun down/up like HDDs and rarely SMART-capable).
var virtualPrefixes = []string{"loop", "ram", "zram", "dm-", "md", "sr"}

// DiscoverDisks enumerates real block devices under sysfsRoot (normally
// /sys) using only sysfs, so it works unprivileged.
func DiscoverDisks(sysfsRoot string) ([]Disk, error) {
	dir := sysfsRoot + "/block"
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("smart: reading %s: %w", dir, err)
	}

	var disks []Disk
	for _, e := range entries {
		name := e.Name()
		if isVirtual(name) {
			continue
		}
		bdir := filepath.Join(dir, name)
		if _, err := os.Stat(filepath.Join(bdir, "device")); err != nil {
			continue // no backing device: virtual, skip
		}

		d := Disk{
			Name: name,
			Path: "/dev/" + name,
		}
		d.Model, _ = readString(filepath.Join(bdir, "device", "model"))
		d.Serial, _ = readString(filepath.Join(bdir, "device", "serial"))
		d.FirmwareRev, _ = readString(filepath.Join(bdir, "device", "firmware_rev"))
		if wwid, err := readString(filepath.Join(bdir, "wwid")); err == nil && wwid != "" {
			d.StableID = wwid
		} else {
			d.StableID = "dev-" + name
		}
		if s, err := readString(filepath.Join(bdir, "size")); err == nil {
			if sectors, err := strconv.ParseInt(s, 10, 64); err == nil && sectors > 0 {
				d.Capacity = humanCapacity(sectors)
			}
		}
		if rot, err := readString(filepath.Join(bdir, "queue", "rotational")); err == nil {
			d.Rotational = rot == "1"
		}
		disks = append(disks, d)
	}
	return disks, nil
}

func isVirtual(name string) bool {
	for _, p := range virtualPrefixes {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

func readString(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

// Health is the subset of smartctl's JSON output HW-T surfaces, matching
// the documented smartmontools JSON schema. Fields are read defensively:
// a firmware/vendor that omits one doesn't fail the whole parse.
type Health struct {
	Skipped          bool   // device was in standby; smartctl (-n standby,0) declined to wake it
	ErrorMessage     string // first error-severity smartctl message, e.g. "Permission denied"
	PassedKnown      bool
	Passed           bool
	TempKnown        bool
	TempC            float64
	PercentUsedKnown bool
	PercentUsed      float64
	MediaErrors      uint64
	NumErrLogEntries uint64
	PowerOnHours     uint64
	CriticalWarning  uint64
}

type smartctlJSON struct {
	Smartctl struct {
		Messages []struct {
			String   string `json:"string"`
			Severity string `json:"severity"`
		} `json:"messages"`
	} `json:"smartctl"`
	SmartStatus *struct {
		Passed bool `json:"passed"`
	} `json:"smart_status"`
	Temperature *struct {
		Current float64 `json:"current"`
	} `json:"temperature"`
	NVMeHealth *struct {
		CriticalWarning  uint64  `json:"critical_warning"`
		PercentageUsed   float64 `json:"percentage_used"`
		MediaErrors      uint64  `json:"media_errors"`
		NumErrLogEntries uint64  `json:"num_err_log_entries"`
		PowerOnHours     uint64  `json:"power_on_hours"`
	} `json:"nvme_smart_health_information_log"`
	PowerOnTime *struct {
		Hours uint64 `json:"hours"`
	} `json:"power_on_time"`
}

// ParseHealth decodes smartctl -j -a output. It never errors on a
// recognizable-but-partial document; only genuinely malformed JSON fails.
func ParseHealth(raw []byte) (*Health, error) {
	var doc smartctlJSON
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("smart: parsing smartctl output: %w", err)
	}

	h := &Health{}
	for _, m := range doc.Smartctl.Messages {
		if strings.Contains(strings.ToUpper(m.String), "STANDBY") {
			h.Skipped = true
		}
		if m.Severity == "error" && h.ErrorMessage == "" {
			h.ErrorMessage = m.String
		}
	}
	if doc.SmartStatus != nil {
		h.PassedKnown, h.Passed = true, doc.SmartStatus.Passed
	}
	if doc.Temperature != nil {
		h.TempKnown, h.TempC = true, doc.Temperature.Current
	}
	if doc.NVMeHealth != nil {
		h.PercentUsedKnown = true
		h.PercentUsed = doc.NVMeHealth.PercentageUsed
		h.MediaErrors = doc.NVMeHealth.MediaErrors
		h.NumErrLogEntries = doc.NVMeHealth.NumErrLogEntries
		h.CriticalWarning = doc.NVMeHealth.CriticalWarning
		h.PowerOnHours = doc.NVMeHealth.PowerOnHours
	}
	if doc.PowerOnTime != nil && h.PowerOnHours == 0 {
		h.PowerOnHours = doc.PowerOnTime.Hours
	}
	return h, nil
}

// runSmartctl execs smartctl -j -a with the standby guard from §5.4:
// "-n standby,0" makes smartctl skip (rather than wake) a sleeping disk
// and exit 0 either way, so a skip is never mistaken for a command
// failure.
func runSmartctl(ctx context.Context, path string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "smartctl", "-j", "-a", "-n", "standby,0", path)
	out, err := cmd.Output()
	if len(out) > 0 {
		// smartctl's own exit code reflects device/SMART state, not
		// whether it produced parseable JSON; prefer the output whenever
		// there is any.
		return out, nil
	}
	return nil, err
}
