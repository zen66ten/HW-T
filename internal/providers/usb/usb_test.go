package usb

import (
	"context"
	"testing"
)

func TestDiscover(t *testing.T) {
	p := New("../../../testdata/fixtures/basic/sys")
	devs, err := p.Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// usb1 + 1-4; the interface dir 1-4:1.0 must be excluded.
	if len(devs) != 2 {
		t.Fatalf("got %d devices, want 2: %+v", len(devs), devs)
	}

	recv := devs[0] // sorted: "1-4" < "usb1"
	if recv.ID != "usb:1-4" || recv.Name != "USB Receiver" {
		t.Errorf("device = %s / %q", recv.ID, recv.Name)
	}
	if recv.Attrs["vendor_id"] != "046d" || recv.Attrs["serial"] != "ABC123" ||
		recv.Attrs["speed"] != "12 Mbps" || recv.Attrs["class"] != "(defined at interface level)" {
		t.Errorf("attrs = %v", recv.Attrs)
	}

	hub := devs[1]
	if hub.ID != "usb:usb1" || hub.Attrs["class"] != "Hub" {
		t.Errorf("hub = %s attrs=%v", hub.ID, hub.Attrs)
	}
}
