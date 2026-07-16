package cpu

import "testing"

func TestDiscoverFreq(t *testing.T) {
	freqs, err := DiscoverFreq("../../../testdata/fixtures/basic/sys/devices/system/cpu")
	if err != nil {
		t.Fatal(err)
	}

	want := []Freq{
		{CPU: 0, CurMHz: 3597, MinMHz: 400, MaxMHz: 5084, Governor: "powersave", Driver: "amd-pstate-epp"},
		{CPU: 1, CurMHz: 3720, MinMHz: 400, MaxMHz: 5084, Governor: "powersave", Driver: "amd-pstate-epp"},
	}
	if len(freqs) != len(want) {
		t.Fatalf("got %d cpus, want %d: %+v", len(freqs), len(want), freqs)
	}
	for i, w := range want {
		if freqs[i] != w {
			t.Errorf("cpu %d = %+v, want %+v", i, freqs[i], w)
		}
	}
}
