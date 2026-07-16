package rapl

import "testing"

const fixtureRoot = "../../../testdata/fixtures/basic/sys"

func TestDiscover(t *testing.T) {
	zones, err := Discover(fixtureRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(zones) != 3 {
		t.Fatalf("got %d zones, want 3: %+v", len(zones), zones)
	}

	byPath := map[string]Zone{}
	for _, z := range zones {
		byPath[z.Path] = z
	}

	pkg, ok := byPath["intel-rapl:0"]
	if !ok || pkg.Name != "package-0" || pkg.Parent != "" {
		t.Errorf("package zone = %+v", pkg)
	}
	if pkg.MaxEnergyRange <= 0 {
		t.Errorf("package MaxEnergyRange = %v, want > 0", pkg.MaxEnergyRange)
	}

	core, ok := byPath["intel-rapl:0:0"]
	if !ok || core.Name != "core" || core.Parent != "intel-rapl:0" {
		t.Errorf("core zone = %+v", core)
	}
	uncore, ok := byPath["intel-rapl:0:1"]
	if !ok || uncore.Name != "uncore" || uncore.Parent != "intel-rapl:0" {
		t.Errorf("uncore zone = %+v", uncore)
	}
}

func TestReadEnergy(t *testing.T) {
	zones, err := Discover(fixtureRoot)
	if err != nil {
		t.Fatal(err)
	}
	for _, z := range zones {
		j, err := ReadEnergy(fixtureRoot, z)
		if err != nil {
			t.Fatal(err)
		}
		if j <= 0 {
			t.Errorf("zone %s: energy = %v, want > 0", z.Path, j)
		}
	}
}

func TestPowerWrap(t *testing.T) {
	const maxRange = 100.0

	// Normal increase: 10J over 2s = 5W.
	if w := Power(10, 20, maxRange, 2); w != 5 {
		t.Errorf("normal delta: got %v, want 5", w)
	}

	// Counter wrapped past max_energy_range: prev=90, cur=5 -> delta = 5+(100-90) = 15.
	if w := Power(90, 5, maxRange, 3); w != 5 {
		t.Errorf("wrapped delta: got %v, want 5", w)
	}

	// Zero elapsed time must not divide by zero.
	if w := Power(10, 20, maxRange, 0); w != 0 {
		t.Errorf("zero dt: got %v, want 0", w)
	}
}
