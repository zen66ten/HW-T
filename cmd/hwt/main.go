// Phase 0 spike: enumerate hwmon chips and cpufreq, print a sensors-style
// table. Grows into the TUI client in phase 1.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/zen66ten/HW-T/internal/providers/cpu"
	"github.com/zen66ten/HW-T/internal/providers/hwmon"
)

func main() {
	sysfs := flag.String("sysfs", "/sys", "sysfs mount point (point at a fixture tree for testing)")
	flag.Parse()

	chips, err := hwmon.Discover(filepath.Join(*sysfs, "class", "hwmon"))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	for _, chip := range chips {
		fmt.Printf("%s-%s  (%s)\n", chip.Name, chip.StablePath, chip.Hwmon)
		for _, r := range hwmon.Collect(chip) {
			printReading(r)
		}
		fmt.Println()
	}

	freqs, err := cpu.DiscoverFreq(filepath.Join(*sysfs, "devices", "system", "cpu"))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if len(freqs) > 0 {
		fmt.Printf("cpufreq-%s  (governor: %s)\n", freqs[0].Driver, freqs[0].Governor)
		for _, f := range freqs {
			fmt.Printf("%-16s%9s  (min = %s, max = %s)\n",
				fmt.Sprintf("cpu%d:", f.CPU),
				formatMHz(f.CurMHz), formatMHz(f.MinMHz), formatMHz(f.MaxMHz))
		}
	}
}

func printReading(r hwmon.Reading) {
	label := r.Channel.Label + ":"
	if r.Err != nil {
		fmt.Printf("%-16s%9s\n", label, "N/A")
		return
	}
	fmt.Printf("%-16s%9s%s\n", label, formatValue(r.Channel.Kind, r.Value), formatLimits(r.Channel))
}

func formatValue(kind hwmon.Kind, v float64) string {
	switch kind {
	case hwmon.KindTemp:
		return fmt.Sprintf("%+.1f°C", v)
	case hwmon.KindIn:
		return fmt.Sprintf("%+.3f V", v)
	case hwmon.KindFan:
		return fmt.Sprintf("%.0f RPM", v)
	case hwmon.KindCurr:
		return fmt.Sprintf("%+.3f A", v)
	case hwmon.KindPower:
		return fmt.Sprintf("%.2f W", v)
	case hwmon.KindEnergy:
		return fmt.Sprintf("%.2f J", v)
	case hwmon.KindHumidity:
		return fmt.Sprintf("%.1f %%RH", v)
	default:
		return fmt.Sprintf("%.0f", v)
	}
}

// limitOrder mirrors lm-sensors: low/high naming for temperatures,
// min/max for everything else, then the critical thresholds.
var limitOrder = []struct{ attr, tempName, name string }{
	{"min", "low", "min"},
	{"max", "high", "max"},
	{"crit", "crit", "crit"},
	{"lcrit", "crit low", "crit min"},
	{"emergency", "emerg", "emerg"},
	{"cap", "cap", "cap"},
}

func formatLimits(ch hwmon.Channel) string {
	var parts []string
	for _, l := range limitOrder {
		v, ok := ch.Limits[l.attr]
		if !ok {
			continue
		}
		name := l.name
		if ch.Kind == hwmon.KindTemp {
			name = l.tempName
		}
		parts = append(parts, fmt.Sprintf("%s = %s", name, formatValue(ch.Kind, v)))
	}
	if len(parts) == 0 {
		return ""
	}
	out := "  ("
	for i, p := range parts {
		if i > 0 {
			out += ", "
		}
		out += p
	}
	return out + ")"
}

func formatMHz(mhz float64) string {
	return fmt.Sprintf("%.0f MHz", mhz)
}
