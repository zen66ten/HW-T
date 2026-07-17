package core

import "testing"

func TestEnrichLabel(t *testing.T) {
	cases := []struct {
		chip, id, label, want string
	}{
		{"gigabyte_wmi", "temp3", "temp3", "CPU"},
		{"gigabyte_wmi", "temp5", "temp5", "VRM MOS"},
		{"acpitz", "temp1", "temp1", "ACPI Zone 1"},
		{"k10temp", "temp1", "Tctl", "CPU (Tctl)"},
		{"k10temp", "temp3", "Tccd1", "CPU CCD1"},
		{"nct6798", "temp1", "SYSTIN", "System"},
		{"nct6798", "in0", "Vcore", "CPU Core Voltage"},
		{"amdgpu", "temp2", "junction", "GPU Hotspot"},
		{"cpufreq", "cpu7", "cpu7", "Core 7 Clock"},
		{"cpufreq", "cpu17_eff", "cpu17 (eff)", "Core 17 Effective Clock"},
		{"iwlwifi_1", "temp1", "temp1", "WiFi Module"},
		{"package-0", "package", "package-0", "CPU Package Power"},
		{"package-0", "core", "core", "CPU Cores Power"},
		// Unknown chips and labels pass through untouched.
		{"weirdchip", "temp9", "temp9", "temp9"},
		{"nvidia", "temp", "GPU Temp", "GPU Temp"},
	}
	for _, c := range cases {
		if got := EnrichLabel(c.chip, c.id, c.label); got != c.want {
			t.Errorf("EnrichLabel(%q, %q, %q) = %q, want %q", c.chip, c.id, c.label, got, c.want)
		}
	}
}
