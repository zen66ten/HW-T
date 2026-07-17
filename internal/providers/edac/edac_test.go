package edac

import (
	"context"
	"testing"
	"time"
)

func TestDiscoverAndCollect(t *testing.T) {
	p := New("../../../testdata/fixtures/basic/sys", time.Minute)
	devs, err := p.Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(devs) != 1 {
		t.Fatalf("got %d devices, want 1: %+v", len(devs), devs)
	}
	mc := devs[0]
	if mc.ID != "edac:mc0" {
		t.Errorf("ID = %s", mc.ID)
	}
	// mc-level ce/ue + 2 DIMMs x 2 = 6 channels.
	if len(mc.Channels) != 6 {
		t.Fatalf("channels = %+v", mc.Channels)
	}
	if mc.Channels[2].Label != "mc#0memory#0 CE" {
		t.Errorf("dimm label = %q", mc.Channels[2].Label)
	}

	readings, err := p.Collect(context.Background(), mc.ID)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]float64{
		"ce": 3, "ue": 0,
		"dimm0_ce": 2, "dimm0_ue": 0,
		"dimm1_ce": 1, "dimm1_ue": 0,
	}
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

func TestNoEDACIsNotAnError(t *testing.T) {
	p := New("/nonexistent", time.Minute)
	devs, err := p.Discover(context.Background())
	if err != nil || devs != nil {
		t.Errorf("missing EDAC: devs=%v err=%v, want nil/nil", devs, err)
	}
}
