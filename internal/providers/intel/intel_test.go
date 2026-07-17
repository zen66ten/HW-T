package intel

import (
	"context"
	"testing"
	"time"
)

const fixtureRoot = "../../../testdata/fixtures/basic/sys"

func TestDiscoverAndCollect(t *testing.T) {
	p := New(fixtureRoot, time.Second)
	devs, err := p.Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// card1 = i915 iGPU (multi-gt layout), card2 = xe Arc. The amdgpu
	// fixture card0 must not appear here.
	if len(devs) != 2 {
		t.Fatalf("got %d devices, want 2: %+v", len(devs), devs)
	}

	var i915, xe *int
	for i := range devs {
		switch devs[i].Attrs["driver"] {
		case "i915":
			v := i
			i915 = &v
		case "xe":
			v := i
			xe = &v
		}
	}
	if i915 == nil || xe == nil {
		t.Fatalf("missing driver coverage: %+v", devs)
	}

	ig := devs[*i915]
	if ig.ID != "intel:pci-0000:00:02.0" {
		t.Errorf("i915 ID = %s", ig.ID)
	}
	if len(ig.Channels) != 2 || ig.Channels[0].Label != "GPU Clock" {
		t.Errorf("i915 channels = %+v", ig.Channels)
	}
	if ig.Channels[0].Limits["max"] != 1400 {
		t.Errorf("i915 limits = %v", ig.Channels[0].Limits)
	}

	readings, err := p.Collect(context.Background(), ig.ID)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]float64{"gpu_act": 1250, "gpu_cur": 1300}
	for _, r := range readings {
		if r.Err != "" {
			t.Errorf("%s: %s", r.Channel, r.Err)
		} else if w, ok := want[r.Channel]; !ok || r.Value != w {
			t.Errorf("%s = %v, want %v", r.Channel, r.Value, want[r.Channel])
		}
	}

	arc := devs[*xe]
	readings, err = p.Collect(context.Background(), arc.ID)
	if err != nil {
		t.Fatal(err)
	}
	want = map[string]float64{"gpu_act": 2100, "gpu_cur": 2400}
	for _, r := range readings {
		if r.Err != "" {
			t.Errorf("xe %s: %s", r.Channel, r.Err)
		} else if w, ok := want[r.Channel]; !ok || r.Value != w {
			t.Errorf("xe %s = %v, want %v", r.Channel, r.Value, want[r.Channel])
		}
	}
}
