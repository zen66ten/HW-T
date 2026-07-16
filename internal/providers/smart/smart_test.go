package smart

import "testing"

const fixtureRoot = "../../../testdata/fixtures/basic/sys"

func TestDiscoverDisks(t *testing.T) {
	disks, err := DiscoverDisks(fixtureRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(disks) != 1 {
		t.Fatalf("got %d disks, want 1 (loop0/sr0/zram0 must be filtered): %+v", len(disks), disks)
	}
	d := disks[0]
	if d.Name != "nvme0n1" || d.Path != "/dev/nvme0n1" {
		t.Errorf("disk = %+v", d)
	}
	if d.Model != "PNY CS3030 1000GB SSD" || d.Serial != "PNY3519002567010372E" || d.FirmwareRev != "CS303224" {
		t.Errorf("identity = %+v", d)
	}
	if d.StableID == "dev-nvme0n1" {
		t.Errorf("StableID fell back to device name; wwid file was not picked up")
	}
}

const nvmeHealthyJSON = `{
  "smartctl": {"messages": []},
  "smart_status": {"passed": true},
  "temperature": {"current": 34},
  "nvme_smart_health_information_log": {
    "critical_warning": 0,
    "percentage_used": 5,
    "media_errors": 0,
    "num_err_log_entries": 0,
    "power_on_hours": 1234
  }
}`

const nvmeFailingJSON = `{
  "smartctl": {"messages": []},
  "smart_status": {"passed": false},
  "temperature": {"current": 78},
  "nvme_smart_health_information_log": {
    "critical_warning": 4,
    "percentage_used": 97,
    "media_errors": 12,
    "num_err_log_entries": 3,
    "power_on_hours": 40000
  }
}`

const standbySkippedJSON = `{
  "smartctl": {
    "messages": [{"string": "Device is in STANDBY mode, exit(2)", "severity": "error"}],
    "exit_status": 2
  }
}`

const permissionDeniedJSON = `{
  "smartctl": {
    "messages": [{"string": "Smartctl open device: /dev/nvme0n1 failed: Permission denied", "severity": "error"}],
    "exit_status": 2
  }
}`

func TestParseHealthHealthy(t *testing.T) {
	h, err := ParseHealth([]byte(nvmeHealthyJSON))
	if err != nil {
		t.Fatal(err)
	}
	if !h.PassedKnown || !h.Passed {
		t.Errorf("Passed = %v/%v, want true/true", h.PassedKnown, h.Passed)
	}
	if !h.TempKnown || h.TempC != 34 {
		t.Errorf("TempC = %v/%v, want true/34", h.TempKnown, h.TempC)
	}
	if h.PercentUsed != 5 || h.MediaErrors != 0 || h.PowerOnHours != 1234 {
		t.Errorf("health = %+v", h)
	}
	if h.Skipped {
		t.Error("Skipped = true for a normal read")
	}
}

func TestParseHealthFailing(t *testing.T) {
	h, err := ParseHealth([]byte(nvmeFailingJSON))
	if err != nil {
		t.Fatal(err)
	}
	if h.Passed {
		t.Error("Passed = true, want false for a failing drive")
	}
	if h.MediaErrors != 12 || h.PercentUsed != 97 {
		t.Errorf("health = %+v", h)
	}
}

func TestParseHealthStandbySkipped(t *testing.T) {
	h, err := ParseHealth([]byte(standbySkippedJSON))
	if err != nil {
		t.Fatal(err)
	}
	if !h.Skipped {
		t.Error("Skipped = false, want true (must not spin up a standby disk)")
	}
	if h.PassedKnown || h.TempKnown {
		t.Errorf("skipped read should carry no stale health data: %+v", h)
	}
}

func TestParseHealthPermissionDenied(t *testing.T) {
	h, err := ParseHealth([]byte(permissionDeniedJSON))
	if err != nil {
		t.Fatal(err)
	}
	if h.PassedKnown || h.TempKnown || h.Skipped {
		t.Errorf("permission-denied read should report nothing known: %+v", h)
	}
}

func TestParseHealthMalformed(t *testing.T) {
	if _, err := ParseHealth([]byte("not json")); err == nil {
		t.Error("malformed JSON did not error")
	}
}
