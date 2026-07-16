package core

import "strings"

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
	return chipName
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
