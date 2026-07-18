package core

import (
	"strconv"
	"strings"
)

// chipDisplayNames maps known kernel driver names to a human-readable
// label. Chip.Name / Device.Name stay the raw driver name everywhere else
// (JSON, metrics labels, the driver-suggestion engine in §10 will match on
// it); this is presentation-only.
var chipDisplayNames = map[string]string{
	"acpitz":           "ACPI Thermal Zone",
	"k10temp":          "AMD CPU (k10temp)",
	"coretemp":         "Intel CPU (coretemp)",
	"nct6775":          "Super I/O (Nuvoton)",
	"nct6798":          "Super I/O (Nuvoton nct6798)",
	"it87":             "Super I/O (ITE)",
	"gigabyte_wmi":     "Gigabyte Motherboard Sensors",
	"asus_wmi_sensors": "ASUS Motherboard Sensors",
	"asus-ec-sensors":  "ASUS EC Sensors",
	"nvme":             "NVMe Drive",
	"drivetemp":        "SATA/SAS Drive",
	"amdgpu":           "AMD GPU",
	"nouveau":          "NVIDIA GPU (nouveau)",
	"jc42":             "DDR4 DIMM Temp Sensor",
	"spd5118":          "DDR5 DIMM Temp Sensor",
	"cpufreq":          "CPU Core Clocks",
}

// DisplayName returns a human-friendly label for a chip/driver name,
// falling back to the raw name when nothing more descriptive is known.
func DisplayName(chipName string) string {
	if friendly, ok := chipDisplayNames[chipName]; ok {
		return friendly
	}
	if strings.HasPrefix(chipName, "iwlwifi") {
		return "WiFi Adapter"
	}
	if strings.HasPrefix(chipName, "package-") {
		return "CPU Package (RAPL)"
	}
	return chipName
}

// chipChannelLabels maps chip name -> channel ID or driver label -> a
// descriptive display name, following HWiNFO and lm-sensors conventions.
// Like DisplayName this is presentation-only; raw labels stay in JSON and
// metrics. The gigabyte_wmi ordering comes from the kernel driver
// (System1, PCH, CPU, PCIEX16, VRM MOS, System2) and matches Gigabyte's
// own tools on AM4/AM5 boards.
var chipChannelLabels = map[string]map[string]string{
	"acpitz": {
		"temp1": "ACPI Zone 1",
		"temp2": "ACPI Zone 2",
	},
	"gigabyte_wmi": {
		"temp1": "System 1",
		"temp2": "Chipset (PCH)",
		"temp3": "CPU",
		"temp4": "PCIe x16 Slot",
		"temp5": "VRM MOS",
		"temp6": "System 2",
	},
	"k10temp": {
		"Tctl":  "CPU (Tctl)",
		"Tdie":  "CPU (Tdie)",
		"Tccd1": "CPU CCD1",
		"Tccd2": "CPU CCD2",
		"Tccd3": "CPU CCD3",
		"Tccd4": "CPU CCD4",
	},
	"amdgpu": {
		"edge":     "GPU Edge",
		"junction": "GPU Hotspot",
		"mem":      "GPU Memory",
		"vddgpu":   "GPU Core Voltage",
		"vddnb":    "GPU SoC Voltage",
		"PPT":      "GPU Package Power",
	},
	"nvme": {
		"Composite": "Drive Composite",
	},
	"cpufreq": {},
	"rapl:":   {},
}

// genericChannelLabels covers driver labels that mean the same thing on
// every chip (Super-I/O naming from lm-sensors).
var genericChannelLabels = map[string]string{
	"SYSTIN":       "System",
	"CPUTIN":       "CPU Socket",
	"AUXTIN":       "Auxiliary",
	"AUXTIN0":      "Auxiliary 0",
	"AUXTIN1":      "Auxiliary 1",
	"AUXTIN2":      "Auxiliary 2",
	"AUXTIN3":      "Auxiliary 3",
	"PECI Agent 0": "CPU (PECI)",
	"TSI0_TEMP":    "CPU (TSI)",
	"Vcore":        "CPU Core Voltage",
	"Vsoc":         "CPU SoC Voltage",
	"Vddcr_soc":    "CPU SoC Voltage",
	"Icore":        "CPU Core Current",
	"Isoc":         "CPU SoC Current",
	"Vbat":         "Battery Voltage",
	"AVCC":         "Analog +3.3V",
	"3VCC":         "+3.3V",
	"3VSB":         "+3.3V Standby",
	"+3.3V":        "+3.3V",
	"+5V":          "+5V Rail",
	"+12V":         "+12V Rail",
	"VTT":          "Memory Termination (VTT)",
	"DRAM":         "DRAM Voltage",
	"VIN0":         "Voltage Input 0",
	"VIN1":         "Voltage Input 1",
	"VIN2":         "Voltage Input 2",
	"VIN3":         "Voltage Input 3",
}

// raplZoneLabels names the powercap domains.
var raplZoneLabels = map[string]string{
	"core":   "CPU Cores Power",
	"uncore": "Uncore Power",
	"dram":   "DRAM Power",
	"psys":   "Platform Power",
}

// EnrichLabel returns a descriptive sensor name for display, given the
// chip/device name, the channel ID, and the label the driver provided.
// Falls back to the driver label untouched.
func EnrichLabel(chipName, channelID, label string) string {
	if m, ok := chipChannelLabels[chipName]; ok {
		if friendly, ok := m[channelID]; ok {
			return friendly
		}
		if friendly, ok := m[label]; ok {
			return friendly
		}
	}
	if friendly, ok := genericChannelLabels[label]; ok {
		return friendly
	}

	switch chipName {
	case "cpufreq":
		// cpu7 -> "Core 7 Clock"; cpu7_eff -> "Core 7 Effective Clock".
		if n, eff, ok := parseCPUChannel(channelID); ok {
			if eff {
				return "Core " + n + " Effective Clock"
			}
			return "Core " + n + " Clock"
		}
	case "iwlwifi", "iwlwifi_1":
		return "WiFi Module"
	}
	if strings.HasPrefix(chipName, "package-") {
		// RAPL package device: channel "package" is the whole-socket power.
		if channelID == "package" {
			return "CPU Package Power"
		}
		if friendly, ok := raplZoneLabels[label]; ok {
			return friendly
		}
	}
	// Chips without driver labels leave label == channel ID ("in3",
	// "fan2"). Spell those out so the sensor table never shows raw IDs.
	if label == channelID {
		if friendly := describeChannel(channelID); friendly != "" {
			return friendly
		}
	}
	return label
}

// channelKindNames spells out the hwmon channel classes for unlabeled
// channels: "in3" -> "Voltage #3".
var channelKindNames = []struct{ prefix, name string }{
	{"temp", "Temperature #"},
	{"in", "Voltage #"},
	{"fan", "Fan #"},
	{"curr", "Current #"},
	{"power", "Power #"},
	{"pwm", "Fan PWM #"},
	{"energy", "Energy #"},
	{"humidity", "Humidity #"},
}

func describeChannel(id string) string {
	for _, k := range channelKindNames {
		num, found := strings.CutPrefix(id, k.prefix)
		if !found || num == "" {
			continue
		}
		if _, err := strconv.Atoi(num); err == nil {
			return k.name + num
		}
	}
	return ""
}

func parseCPUChannel(id string) (num string, eff bool, ok bool) {
	rest, found := strings.CutPrefix(id, "cpu")
	if !found {
		return "", false, false
	}
	if n, found := strings.CutSuffix(rest, "_eff"); found {
		return n, true, n != ""
	}
	return rest, false, rest != ""
}

// ShortenPath compresses a long stable-topology path for a human-facing
// table, keeping the ends (which carry the identifying information: bus
// type and terminal device) and eliding the noisy middle. The full path
// remains available verbatim in JSON output (-json, /api) for anything
// that needs to match against it.
func ShortenPath(path string, max int) string {
	if len(path) <= max || max < 5 {
		return path
	}
	segs := strings.Split(path, "/")
	if len(segs) <= 2 {
		half := (max - 1) / 2
		return path[:half] + "…" + path[len(path)-half:]
	}
	head, tail := segs[0], segs[len(segs)-1]
	short := head + "/…/" + tail
	if len(short) <= max {
		return short
	}
	return ShortenPath(head+"/"+tail, max)
}
