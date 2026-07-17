package core

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func reportFixture() ([]Device, []Sensor) {
	devs := []Device{
		{ID: "dmi:system", Provider: "dmi", Name: "System", Attrs: map[string]string{
			"manufacturer": "Gigabyte",
			"serial":       "SN-SECRET",
			"uuid":         "aaaa-bbbb",
		}},
		{ID: "hwmon:pci-0000:00:18.3", Provider: "hwmon", Name: "k10temp",
			Channels: []ChannelInfo{{ID: "temp1", Kind: KindTemp, Label: "Tctl"}}},
		{ID: "usb:1-4", Provider: "usb", Name: "USB Receiver", Attrs: map[string]string{
			"serial": "ABC123",
		}},
	}
	sensors := []Sensor{
		{ID: "hwmon:pci-0000:00:18.3:temp1", Device: "hwmon:pci-0000:00:18.3",
			Kind: KindTemp, Label: "Tctl", Cur: 51.5, Min: 40, Max: 70, Avg: 50.1, N: 100,
			Ts: time.Now()},
	}
	return devs, sensors
}

func TestReportText(t *testing.T) {
	devs, sensors := reportFixture()
	out, err := BuildReport(devs, sensors, ReportOptions{Format: "text"})
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	for _, want := range []string{"===== dmi =====", "Gigabyte", "Tctl:", "+51.5°C", "SN-SECRET"} {
		if !strings.Contains(s, want) {
			t.Errorf("text report missing %q:\n%s", want, s)
		}
	}
}

func TestReportRedact(t *testing.T) {
	devs, sensors := reportFixture()
	out, err := BuildReport(devs, sensors, ReportOptions{Format: "text", Redact: true})
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	for _, leaked := range []string{"SN-SECRET", "aaaa-bbbb", "ABC123"} {
		if strings.Contains(s, leaked) {
			t.Errorf("redacted report leaks %q", leaked)
		}
	}
	if !strings.Contains(s, "[redacted]") {
		t.Error("no [redacted] markers present")
	}
	if !strings.Contains(s, "Gigabyte") {
		t.Error("redact removed non-sensitive attrs")
	}
}

func TestReportSectionFilter(t *testing.T) {
	devs, sensors := reportFixture()
	out, err := BuildReport(devs, sensors, ReportOptions{Format: "text", Sections: []string{"usb"}})
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if strings.Contains(s, "Gigabyte") || strings.Contains(s, "Tctl") {
		t.Errorf("section filter leaked other providers:\n%s", s)
	}
	if !strings.Contains(s, "USB Receiver") {
		t.Errorf("filtered section missing:\n%s", s)
	}
}

func TestReportJSONAndYAML(t *testing.T) {
	devs, sensors := reportFixture()

	out, err := BuildReport(devs, sensors, ReportOptions{Format: "json"})
	if err != nil {
		t.Fatal(err)
	}
	var r Report
	if err := json.Unmarshal(out, &r); err != nil {
		t.Fatalf("json report not parseable: %v", err)
	}
	if len(r.Devices) != 3 || r.Devices[1].Sensors[0].Cur != 51.5 {
		t.Errorf("json roundtrip = %+v", r)
	}

	if _, err := BuildReport(devs, sensors, ReportOptions{Format: "yaml"}); err != nil {
		t.Fatal(err)
	}
}

func TestReportHTMLSelfContained(t *testing.T) {
	devs, sensors := reportFixture()
	out, err := BuildReport(devs, sensors, ReportOptions{Format: "html"})
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, "<!DOCTYPE html>") || !strings.Contains(s, "Tctl") {
		t.Errorf("html report malformed:\n%.400s", s)
	}
	for _, external := range []string{"http://", "https://", "src="} {
		if strings.Contains(s, external) {
			t.Errorf("html report references external asset (%s); must be self-contained", external)
		}
	}
}

func TestReportUnknownFormat(t *testing.T) {
	if _, err := BuildReport(nil, nil, ReportOptions{Format: "docx"}); err == nil {
		t.Error("unknown format did not error")
	}
}
