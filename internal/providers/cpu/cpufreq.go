// Package cpu reads CPU state from sysfs. Phase 0 covers cpufreq (governor
// view of clocks); CPUID identity, MSR effective clocks and RAPL power land
// in later phases.
package cpu

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Freq is the cpufreq view of one logical CPU. Frequencies are in MHz.
type Freq struct {
	CPU      int
	CurMHz   float64 // scaling_cur_freq: what the governor requested/observed
	MinMHz   float64 // cpuinfo_min_freq: hardware minimum
	MaxMHz   float64 // cpuinfo_max_freq: hardware maximum (incl. boost)
	Governor string
	Driver   string
}

var cpuDirRE = regexp.MustCompile(`^cpu([0-9]+)$`)

// DiscoverFreq enumerates cpufreq state for every logical CPU under root
// (normally /sys/devices/system/cpu). CPUs without a cpufreq directory
// (offline, or no driver) are skipped.
func DiscoverFreq(root string) ([]Freq, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("cpu: reading %s: %w", root, err)
	}

	var freqs []Freq
	for _, e := range entries {
		m := cpuDirRE.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		dir := filepath.Join(root, e.Name(), "cpufreq")
		cur, err := readKHz(filepath.Join(dir, "scaling_cur_freq"))
		if err != nil {
			continue
		}
		index, _ := strconv.Atoi(m[1])
		f := Freq{CPU: index, CurMHz: cur}
		f.MinMHz, _ = readKHz(filepath.Join(dir, "cpuinfo_min_freq"))
		f.MaxMHz, _ = readKHz(filepath.Join(dir, "cpuinfo_max_freq"))
		f.Governor, _ = readString(filepath.Join(dir, "scaling_governor"))
		f.Driver, _ = readString(filepath.Join(dir, "scaling_driver"))
		freqs = append(freqs, f)
	}

	sort.Slice(freqs, func(i, j int) bool { return freqs[i].CPU < freqs[j].CPU })
	return freqs, nil
}

func readString(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

func readKHz(path string) (float64, error) {
	s, err := readString(path)
	if err != nil {
		return 0, err
	}
	khz, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0, err
	}
	return float64(khz) / 1000, nil
}
