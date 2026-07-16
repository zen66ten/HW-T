// Package rapl reads CPU package/core/uncore/dram/psys power via the
// powercap RAPL sysfs interface (§5.2). Each zone exposes a monotonic
// energy_uj counter; power is the rate of change between two reads. The
// counter wraps at max_energy_range_uj, which must be handled explicitly.
//
// energy_uj has been 0400 root-only since the PLATYPUS fix
// (CVE-2020-8694/8695), so this provider expects to run as root.
package rapl

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Zone is one powercap RAPL zone (package-0, core, uncore, dram, psys, ...).
// Zones nest (a package zone has core/uncore/dram children); Parent is ""
// for top-level zones.
type Zone struct {
	Path           string  // e.g. "intel-rapl:0:0"
	Name           string  // e.g. "core"
	Parent         string  // parent Zone.Path, or "" for package-level zones
	MaxEnergyRange float64 // joules, for wrap detection
}

var zoneDirRE = regexp.MustCompile(`^intel-rapl:\d+(:\d+)?$`)

const root = "/class/powercap"

// Discover enumerates RAPL zones under sysfsRoot (normally /sys).
func Discover(sysfsRoot string) ([]Zone, error) {
	dir := sysfsRoot + root
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // no RAPL support on this platform
		}
		return nil, fmt.Errorf("rapl: reading %s: %w", dir, err)
	}

	var zones []Zone
	for _, e := range entries {
		if !zoneDirRE.MatchString(e.Name()) {
			continue
		}
		zdir := filepath.Join(dir, e.Name())
		name, err := readString(filepath.Join(zdir, "name"))
		if err != nil {
			continue
		}
		maxRange, _ := readMicrojoules(filepath.Join(zdir, "max_energy_range_uj"))

		z := Zone{Path: e.Name(), Name: name, MaxEnergyRange: maxRange}
		if idx := strings.LastIndex(e.Name(), ":"); idx > 0 && strings.Count(e.Name(), ":") > 1 {
			z.Parent = e.Name()[:idx]
		}
		zones = append(zones, z)
	}

	sort.Slice(zones, func(i, j int) bool { return zones[i].Path < zones[j].Path })
	return zones, nil
}

// ReadEnergy returns the current cumulative energy counter in joules.
func ReadEnergy(sysfsRoot string, z Zone) (float64, error) {
	return readMicrojoules(filepath.Join(sysfsRoot+root, z.Path, "energy_uj"))
}

// Power computes average power in watts from two energy samples (joules)
// taken dtSeconds apart, handling counter wraparound at MaxEnergyRange.
func Power(prevJ, curJ, maxRangeJ, dtSeconds float64) float64 {
	delta := curJ - prevJ
	if delta < 0 {
		delta += maxRangeJ
	}
	if dtSeconds <= 0 {
		return 0
	}
	return delta / dtSeconds
}

func readString(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

func readMicrojoules(path string) (float64, error) {
	s, err := readString(path)
	if err != nil {
		return 0, err
	}
	uj, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0, err
	}
	return float64(uj) / 1e6, nil
}
