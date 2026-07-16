package nvidia

import "testing"

const sampleCSV = `GPU-30c237fa-265a-9c1e-12c9-ef20c33b877d, NVIDIA GeForce RTX 3080 Ti, 51, 97.70, 350.00, 540, 540, 5001, 1005, 38, 10, 2504, 12288, 62
GPU-aaaabbbb-cccc-dddd-eeee-ffff00001111, NVIDIA T400, 40, [N/A], 31.00, 420, 420, 5001, 780, 0, 0, 128, 2048, [N/A]`

func TestParseCSV(t *testing.T) {
	gpus, err := ParseCSV(sampleCSV)
	if err != nil {
		t.Fatal(err)
	}
	if len(gpus) != 2 {
		t.Fatalf("got %d GPUs, want 2", len(gpus))
	}

	g := gpus[0]
	if g.UUID != "GPU-30c237fa-265a-9c1e-12c9-ef20c33b877d" || g.Name != "NVIDIA GeForce RTX 3080 Ti" {
		t.Errorf("identity = %q / %q", g.UUID, g.Name)
	}
	checks := []struct {
		name string
		got  *float64
		want float64
	}{
		{"TempC", g.TempC, 51},
		{"PowerW", g.PowerW, 97.7},
		{"PowerLimitW", g.PowerLimitW, 350},
		{"GraphicsMHz", g.GraphicsMHz, 540},
		{"MemMHz", g.MemMHz, 5001},
		{"UtilGPU", g.UtilGPU, 38},
		{"MemUsedMiB", g.MemUsedMiB, 2504},
		{"MemTotalMiB", g.MemTotalMiB, 12288},
		{"FanPercent", g.FanPercent, 62},
	}
	for _, c := range checks {
		if c.got == nil {
			t.Errorf("%s = nil, want %v", c.name, c.want)
		} else if *c.got != c.want {
			t.Errorf("%s = %v, want %v", c.name, *c.got, c.want)
		}
	}

	// Second GPU: fanless card reports [N/A] for power draw and fan.
	g2 := gpus[1]
	if g2.PowerW != nil {
		t.Errorf("PowerW = %v, want nil for [N/A]", *g2.PowerW)
	}
	if g2.FanPercent != nil {
		t.Errorf("FanPercent = %v, want nil for [N/A]", *g2.FanPercent)
	}
	if g2.TempC == nil || *g2.TempC != 40 {
		t.Errorf("TempC = %v, want 40", g2.TempC)
	}
}

func TestParseCSVMalformed(t *testing.T) {
	if _, err := ParseCSV("too, few, fields"); err == nil {
		t.Error("short line did not error")
	}
	gpus, err := ParseCSV("\n\n")
	if err != nil || len(gpus) != 0 {
		t.Errorf("blank input: gpus=%v err=%v", gpus, err)
	}
}
