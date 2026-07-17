package pci

import (
	"context"
	"testing"
	"time"
)

const fixtureRoot = "../../../testdata/fixtures/basic/sys"

func TestDiscoverAndCollect(t *testing.T) {
	p := New(fixtureRoot, time.Minute)
	devs, err := p.Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(devs) != 2 {
		t.Fatalf("got %d devices, want 2: %+v", len(devs), devs)
	}

	// Sorted by address: 0000:00:18.3 first, GPU second.
	host := devs[0]
	if host.ID != "pci:0000:00:18.3" {
		t.Errorf("host bridge ID = %s", host.ID)
	}
	if host.Attrs["vendor_id"] != "1022" || host.Attrs["device_id"] != "166d" {
		t.Errorf("host attrs = %v", host.Attrs)
	}
	if len(host.Channels) != 0 {
		t.Errorf("device without AER got channels: %+v", host.Channels)
	}

	gpu := devs[1]
	if gpu.ID != "pci:0000:09:00.0" {
		t.Errorf("gpu ID = %s", gpu.ID)
	}
	// Name comes from pci.ids when the system database exists; either way
	// it must be non-empty (raw-ID fallback otherwise).
	if gpu.Name == "" {
		t.Error("gpu Name empty")
	}
	if gpu.Attrs["link"] != "16.0 GT/s x16" || gpu.Attrs["link_max"] != "16.0 GT/s x16" {
		t.Errorf("link attrs = %v", gpu.Attrs)
	}
	if gpu.Attrs["driver"] != "nvidia" || gpu.Attrs["iommu_group"] != "2" {
		t.Errorf("driver/iommu attrs = %v", gpu.Attrs)
	}
	if len(gpu.Channels) != 3 {
		t.Fatalf("gpu AER channels = %+v", gpu.Channels)
	}

	readings, err := p.Collect(context.Background(), gpu.ID)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]float64{"aer_correctable": 3, "aer_nonfatal": 0, "aer_fatal": 0}
	for _, r := range readings {
		if r.Err != "" {
			t.Errorf("%s: %s", r.Channel, r.Err)
			continue
		}
		if w, ok := want[r.Channel]; !ok || r.Value != w {
			t.Errorf("%s = %v, want %v", r.Channel, r.Value, want[r.Channel])
		}
	}
}
