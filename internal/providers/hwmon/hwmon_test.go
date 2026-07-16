package hwmon

import (
	"maps"
	"testing"
)

const fixtureRoot = "../../../testdata/fixtures/basic/sys/class/hwmon"

func TestDiscover(t *testing.T) {
	chips, err := Discover(fixtureRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(chips) != 3 {
		t.Fatalf("got %d chips, want 3: %+v", len(chips), chips)
	}

	want := []struct {
		name, stablePath string
		channels         []Channel
	}{
		{
			name: "k10temp", stablePath: "pci-0000:00:18.3",
			channels: []Channel{
				{ID: "temp1", Kind: KindTemp, Index: 1, Label: "Tctl", Limits: map[string]float64{"max": 70}},
				{ID: "temp3", Kind: KindTemp, Index: 3, Label: "Tccd1"},
			},
		},
		{
			name: "nct6798", stablePath: "platform/nct6775.656",
			channels: []Channel{
				{ID: "in0", Kind: KindIn, Index: 0, Label: "in0", Limits: map[string]float64{"min": 0, "max": 1.744}},
				{ID: "fan1", Kind: KindFan, Index: 1, Label: "fan1", Limits: map[string]float64{"min": 0}},
				{ID: "pwm1", Kind: KindPWM, Index: 1, Label: "pwm1"},
				{ID: "temp1", Kind: KindTemp, Index: 1, Label: "SYSTIN", Limits: map[string]float64{"max": 80, "crit": 100}},
			},
		},
		{
			name: "iwlwifi_1", stablePath: "virtual",
			channels: []Channel{
				{ID: "temp1", Kind: KindTemp, Index: 1, Label: "temp1"},
			},
		},
	}

	for i, w := range want {
		got := chips[i]
		if got.Name != w.name {
			t.Errorf("chip %d: Name = %q, want %q", i, got.Name, w.name)
		}
		if got.StablePath != w.stablePath {
			t.Errorf("chip %d (%s): StablePath = %q, want %q", i, got.Name, got.StablePath, w.stablePath)
		}
		if len(got.Channels) != len(w.channels) {
			t.Errorf("chip %d (%s): got %d channels, want %d: %+v", i, got.Name, len(got.Channels), len(w.channels), got.Channels)
			continue
		}
		for j, wc := range w.channels {
			gc := got.Channels[j]
			if gc.ID != wc.ID || gc.Kind != wc.Kind || gc.Index != wc.Index || gc.Label != wc.Label {
				t.Errorf("chip %d channel %d = {%s %s %d %q}, want {%s %s %d %q}",
					i, j, gc.ID, gc.Kind, gc.Index, gc.Label, wc.ID, wc.Kind, wc.Index, wc.Label)
			}
			if !maps.Equal(gc.Limits, wc.Limits) {
				t.Errorf("chip %d channel %s: Limits = %v, want %v", i, gc.ID, gc.Limits, wc.Limits)
			}
		}
	}
}

func TestCollect(t *testing.T) {
	chips, err := Discover(fixtureRoot)
	if err != nil {
		t.Fatal(err)
	}

	want := map[string]map[string]float64{
		"k10temp":   {"temp1": 51.25, "temp3": 52.75},
		"nct6798":   {"in0": 0.36, "fan1": 1216, "pwm1": 128, "temp1": 34},
		"iwlwifi_1": {"temp1": 37},
	}

	for _, chip := range chips {
		wantVals := want[chip.Name]
		for _, r := range Collect(chip) {
			if r.Err != nil {
				t.Errorf("%s/%s: %v", chip.Name, r.Channel.ID, r.Err)
				continue
			}
			if w, ok := wantVals[r.Channel.ID]; !ok {
				t.Errorf("%s/%s: unexpected reading %v", chip.Name, r.Channel.ID, r.Value)
			} else if r.Value != w {
				t.Errorf("%s/%s = %v, want %v", chip.Name, r.Channel.ID, r.Value, w)
			}
		}
	}
}
