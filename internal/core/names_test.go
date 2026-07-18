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
		// Unlabeled channels get the kind spelled out.
		{"weirdchip", "temp9", "temp9", "Temperature #9"},
		{"weirdchip", "in3", "in3", "Voltage #3"},
		{"weirdchip", "fan2", "fan2", "Fan #2"},
		{"weirdchip", "curr1", "curr1", "Current #1"},
		{"weirdchip", "pwm4", "pwm4", "Fan PWM #4"},
		// Voltage rails and driver labels.
		{"it87", "in5", "+12V", "+12V Rail"},
		{"amdgpu", "in0", "vddgpu", "GPU Core Voltage"},
		// zenpower SVI2 telemetry (AMD CPU core/SoC voltage, current,
		// power), confirmed against a real Ryzen 3900X's `sensors` output.
		{"zenpower", "in0", "SVI2_Core", "CPU Core Voltage"},
		{"zenpower", "in1", "SVI2_SoC", "CPU SoC Voltage"},
		{"zenpower", "curr0", "SVI2_C_Core", "CPU Core Current"},
		{"zenpower", "curr1", "SVI2_C_SoC", "CPU SoC Current"},
		{"zenpower", "power0", "SVI2_P_Core", "CPU Core Power (SVI2)"},
		{"zenpower", "power1", "SVI2_P_SoC", "CPU SoC Power (SVI2)"},
		// Driver-provided labels pass through untouched.
		{"nvidia", "temp", "GPU Temp", "GPU Temp"},
		{"weirdchip", "inx", "inx", "inx"},
	}
	for _, c := range cases {
		if got := EnrichLabel(c.chip, c.id, c.label); got != c.want {
			t.Errorf("EnrichLabel(%q, %q, %q) = %q, want %q", c.chip, c.id, c.label, got, c.want)
		}
	}
}
