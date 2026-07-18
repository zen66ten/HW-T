// Package nvidia reads NVIDIA GPU telemetry by executing nvidia-smi with a
// CSV query (§5.3's documented fallback path). This keeps the core build
// CGO_ENABLED=0; a go-nvml dlopen provider can later replace the exec under
// the same provider name and device IDs (both key off the GPU UUID, which
// is stable across reboots and driver updates).
package nvidia

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// queryFields is the ordered nvidia-smi --query-gpu list. Discovery and
// collection share it so the parser has exactly one shape to handle.
var queryFields = []string{
	"uuid",
	"name",
	"temperature.gpu",
	"power.draw",
	"power.limit",
	"clocks.current.graphics",
	"clocks.current.sm",
	"clocks.current.memory",
	"clocks.current.video",
	"utilization.gpu",
	"utilization.memory",
	"memory.used",
	"memory.total",
	"fan.speed",
	"driver_version",
	"vbios_version",
	"pcie.link.gen.current",
	"pcie.link.width.current",
}

// GPU is one parsed nvidia-smi CSV row. Value fields are pointers: nil
// means the driver reported "[N/A]"/"[Not Supported]" for that field on
// this generation, which callers surface as per-channel errors rather than
// zeros.
type GPU struct {
	UUID string
	Name string

	TempC       *float64
	PowerW      *float64
	PowerLimitW *float64
	GraphicsMHz *float64
	SMMHz       *float64
	MemMHz      *float64
	VideoMHz    *float64
	UtilGPU     *float64
	UtilMem     *float64
	MemUsedMiB  *float64
	MemTotalMiB *float64
	FanPercent  *float64

	// Identity strings (empty when the driver reports "[N/A]").
	DriverVersion string
	VBIOSVersion  string
	PCIeGen       string
	PCIeWidth     string
}

// ParseCSV decodes `nvidia-smi --query-gpu=... --format=csv,noheader,nounits`
// output, one GPU per line.
func ParseCSV(out string) ([]GPU, error) {
	var gpus []GPU
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Split(line, ", ")
		if len(fields) != len(queryFields) {
			return nil, fmt.Errorf("nvidia: got %d fields, want %d: %q", len(fields), len(queryFields), line)
		}
		g := GPU{
			UUID: strings.TrimSpace(fields[0]),
			Name: strings.TrimSpace(fields[1]),
		}
		g.TempC = num(fields[2])
		g.PowerW = num(fields[3])
		g.PowerLimitW = num(fields[4])
		g.GraphicsMHz = num(fields[5])
		g.SMMHz = num(fields[6])
		g.MemMHz = num(fields[7])
		g.VideoMHz = num(fields[8])
		g.UtilGPU = num(fields[9])
		g.UtilMem = num(fields[10])
		g.MemUsedMiB = num(fields[11])
		g.MemTotalMiB = num(fields[12])
		g.FanPercent = num(fields[13])
		g.DriverVersion = str(fields[14])
		g.VBIOSVersion = str(fields[15])
		g.PCIeGen = str(fields[16])
		g.PCIeWidth = str(fields[17])
		gpus = append(gpus, g)
	}
	return gpus, nil
}

// str trims one CSV value, returning "" for the driver's "[N/A]" markers.
func str(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || strings.HasPrefix(s, "[") || s == "N/A" {
		return ""
	}
	return s
}

// num parses one CSV value; "[N/A]", "[Not Supported]" and similar
// bracketed markers become nil.
func num(s string) *float64 {
	s = strings.TrimSpace(s)
	if s == "" || strings.HasPrefix(s, "[") || s == "N/A" {
		return nil
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil
	}
	return &v
}

func query(ctx context.Context) ([]GPU, error) {
	cmd := exec.CommandContext(ctx, "nvidia-smi",
		"--query-gpu="+strings.Join(queryFields, ","),
		"--format=csv,noheader,nounits")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("nvidia: nvidia-smi: %w", err)
	}
	return ParseCSV(string(out))
}
