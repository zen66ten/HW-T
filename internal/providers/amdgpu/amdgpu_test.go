package amdgpu

import (
	"context"
	"testing"
	"time"
)

const fixtureRoot = "../../../testdata/fixtures/basic/sys"

func TestDiscoverCards(t *testing.T) {
	cards, err := DiscoverCards(fixtureRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(cards) != 1 {
		t.Fatalf("got %d cards, want 1: %+v", len(cards), cards)
	}
	c := cards[0]
	if c.Name != "card0" || c.StablePath != "pci-0000:0c:00.0" {
		t.Errorf("card = %+v", c)
	}
}

func TestActiveDPMClockMHz(t *testing.T) {
	mhz, err := ActiveDPMClockMHz("0: 500Mhz\n1: 2575Mhz *\n2: 2660Mhz\n")
	if err != nil || mhz != 2575 {
		t.Errorf("sclk = %v, %v; want 2575", mhz, err)
	}
	mhz, err = ActiveDPMClockMHz("0: 96Mhz *\n")
	if err != nil || mhz != 96 {
		t.Errorf("single state = %v, %v; want 96", mhz, err)
	}
	if _, err := ActiveDPMClockMHz("0: 500Mhz\n1: 2575Mhz\n"); err == nil {
		t.Error("no active state did not error")
	}
	if _, err := ActiveDPMClockMHz(""); err == nil {
		t.Error("empty table did not error")
	}
}

func TestProviderEndToEnd(t *testing.T) {
	p := New(fixtureRoot, time.Second)
	devs, err := p.Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(devs) != 1 {
		t.Fatalf("got %d devices, want 1", len(devs))
	}
	d := devs[0]
	if d.ID != "amdgpu:pci-0000:0c:00.0" {
		t.Errorf("device ID = %s", d.ID)
	}
	if d.Attrs["vram_total_mib"] != "16368" {
		t.Errorf("vram_total_mib = %q, want 16368", d.Attrs["vram_total_mib"])
	}

	readings, err := p.Collect(context.Background(), d.ID)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]float64{
		"busy":      43,
		"vram_used": 4096,
		"sclk":      2575,
		"mclk":      1000,
	}
	if len(readings) != len(want) {
		t.Fatalf("got %d readings, want %d: %+v", len(readings), len(want), readings)
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
