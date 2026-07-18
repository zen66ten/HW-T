// Package cpuinfo builds the CPU identity inventory HWiNFO and CPU-Z show:
// brand string, vendor, family/model/stepping, microcode, core and thread
// counts, cache sizes, an instruction-set summary and the kernel's
// vulnerability mitigations. Everything comes from /proc/cpuinfo and
// /sys/devices/system/cpu, so it needs no privileges.
package cpuinfo

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/zen66ten/HW-T/internal/core"
)

// Info is the decoded CPU identity.
type Info struct {
	ModelName string
	Vendor    string
	Family    int
	Model     int
	Stepping  int
	Microcode string
	Cores     int
	Threads   int
	Caches    []Cache
	Flags     map[string]bool
}

// Cache is one cache level.
type Cache struct {
	Level int
	Type  string // Data, Instruction, Unified
	Size  string // e.g. "32K"
	Count int    // number of instances at this level
}

// featureDisplay maps /proc/cpuinfo flag names to the CPU-Z-style label,
// in the order CPU-Z lists them. Absent flags are skipped.
var featureDisplay = []struct{ flag, name string }{
	{"mmx", "MMX"}, {"sse", "SSE"}, {"sse2", "SSE2"}, {"pni", "SSE3"},
	{"ssse3", "SSSE3"}, {"sse4_1", "SSE4.1"}, {"sse4_2", "SSE4.2"},
	{"sse4a", "SSE4A"}, {"lm", "x86-64"}, {"svm", "AMD-V"}, {"vmx", "VT-x"},
	{"aes", "AES"}, {"avx", "AVX"}, {"avx2", "AVX2"}, {"avx512f", "AVX-512"},
	{"fma", "FMA3"}, {"sha_ni", "SHA"}, {"bmi1", "BMI1"}, {"bmi2", "BMI2"},
	{"rdrand", "RDRAND"}, {"rdseed", "RDSEED"},
}

// ParseProcCPUInfo decodes /proc/cpuinfo contents. Thread count is the
// number of logical processors; core count comes from "cpu cores".
func ParseProcCPUInfo(content string) Info {
	info := Info{Flags: map[string]bool{}}
	sc := bufio.NewScanner(strings.NewReader(content))
	for sc.Scan() {
		key, val, ok := strings.Cut(sc.Text(), ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		switch key {
		case "processor":
			info.Threads++
		case "model name":
			info.ModelName = val
		case "vendor_id":
			info.Vendor = val
		case "cpu family":
			info.Family, _ = strconv.Atoi(val)
		case "model":
			info.Model, _ = strconv.Atoi(val)
		case "stepping":
			info.Stepping, _ = strconv.Atoi(val)
		case "cpu cores":
			if info.Cores == 0 {
				info.Cores, _ = strconv.Atoi(val)
			}
		case "flags", "Features":
			if len(info.Flags) == 0 {
				for _, f := range strings.Fields(val) {
					info.Flags[f] = true
				}
			}
		}
	}
	return info
}

// Instructions renders the CPU-Z-style instruction-set summary line.
func (i Info) Instructions() string {
	var parts []string
	for _, f := range featureDisplay {
		if i.Flags[f.flag] {
			parts = append(parts, f.name)
		}
	}
	return strings.Join(parts, ", ")
}

// Provider exposes the CPU identity as one inventory device. Static: the
// scheduler discovers it once and never polls it.
type Provider struct {
	sysfs   string
	proc    string
	cpuRoot string
}

func New(sysfsRoot string) *Provider {
	return &Provider{
		sysfs:   sysfsRoot,
		proc:    "/proc/cpuinfo",
		cpuRoot: sysfsRoot + "/devices/system/cpu",
	}
}

func (p *Provider) Name() string                   { return "cpuinfo" }
func (p *Provider) DefaultInterval() time.Duration { return 0 }

func (p *Provider) Collect(ctx context.Context, dev core.DeviceID) ([]core.Reading, error) {
	return nil, nil
}

func (p *Provider) Discover(ctx context.Context) ([]core.Device, error) {
	raw, err := os.ReadFile(p.proc)
	if err != nil {
		return nil, fmt.Errorf("cpuinfo: %w", err)
	}
	info := ParseProcCPUInfo(string(raw))
	info.Caches = p.readCaches()
	info.Microcode = readTrim(filepath.Join(p.cpuRoot, "cpu0", "microcode", "version"))

	attrs := map[string]string{
		"name":     info.ModelName,
		"vendor":   info.Vendor,
		"family":   strconv.Itoa(info.Family),
		"model":    strconv.Itoa(info.Model),
		"stepping": strconv.Itoa(info.Stepping),
	}
	if info.Microcode != "" {
		attrs["microcode"] = info.Microcode
	}
	if info.Cores > 0 {
		attrs["cores"] = strconv.Itoa(info.Cores)
	}
	if info.Threads > 0 {
		attrs["threads"] = strconv.Itoa(info.Threads)
	}
	for _, c := range info.Caches {
		key := "L" + strconv.Itoa(c.Level)
		if c.Type == "Data" {
			key += "d"
		} else if c.Type == "Instruction" {
			key += "i"
		}
		val := c.Size
		if c.Count > 1 {
			val = fmt.Sprintf("%s x%d", c.Size, c.Count)
		}
		attrs["cache_"+key] = val
	}
	if instr := info.Instructions(); instr != "" {
		attrs["instructions"] = instr
	}
	if mit := p.readMitigations(); mit != "" {
		attrs["mitigations"] = mit
	}

	return []core.Device{{
		ID:       core.DeviceID("cpuinfo:0"),
		Provider: "cpuinfo",
		Name:     info.ModelName,
		Attrs:    attrs,
	}}, nil
}

// readCaches aggregates cache descriptors across all CPUs, counting
// instances per (level, type).
func (p *Provider) readCaches() []Cache {
	type key struct {
		level int
		typ   string
	}
	seen := map[key]*Cache{}
	cpus, _ := filepath.Glob(filepath.Join(p.cpuRoot, "cpu*", "cache", "index*"))
	for _, dir := range cpus {
		level, err := strconv.Atoi(readTrim(filepath.Join(dir, "level")))
		if err != nil {
			continue
		}
		typ := readTrim(filepath.Join(dir, "type"))
		size := readTrim(filepath.Join(dir, "size"))
		k := key{level, typ}
		if c, ok := seen[k]; ok {
			c.Count++
		} else {
			seen[k] = &Cache{Level: level, Type: typ, Size: size, Count: 1}
		}
	}
	out := make([]Cache, 0, len(seen))
	for _, c := range seen {
		out = append(out, *c)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Level != out[j].Level {
			return out[i].Level < out[j].Level
		}
		return out[i].Type < out[j].Type
	})
	return out
}

// readMitigations summarises the kernel vulnerability directory: how many
// vulnerabilities are mitigated versus vulnerable.
func (p *Provider) readMitigations() string {
	files, _ := filepath.Glob(filepath.Join(p.cpuRoot, "vulnerabilities", "*"))
	if len(files) == 0 {
		return ""
	}
	vulnerable := 0
	for _, f := range files {
		if strings.HasPrefix(readTrim(f), "Vulnerable") {
			vulnerable++
		}
	}
	if vulnerable == 0 {
		return fmt.Sprintf("all %d mitigated", len(files))
	}
	return fmt.Sprintf("%d of %d vulnerable", vulnerable, len(files))
}

func readTrim(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}
