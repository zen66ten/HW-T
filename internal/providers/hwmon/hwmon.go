// Package hwmon enumerates sensor chips exposed through the kernel hwmon
// class and reads their channel values. It is the backbone provider: every
// chip with a mainline driver (Super-I/O, k10temp, amdgpu, drivetemp, NVMe,
// liquid coolers, ...) surfaces here.
//
// Discover takes the class directory as a parameter (normally
// /sys/class/hwmon) so tests can point it at captured fixture trees.
package hwmon

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Kind is a hwmon channel class per Documentation/hwmon/sysfs-interface.rst.
type Kind string

const (
	KindIn       Kind = "in"
	KindFan      Kind = "fan"
	KindPWM      Kind = "pwm"
	KindTemp     Kind = "temp"
	KindCurr     Kind = "curr"
	KindPower    Kind = "power"
	KindEnergy   Kind = "energy"
	KindHumidity Kind = "humidity"
)

// kindOrder matches the display order lm-sensors uses.
var kindOrder = map[Kind]int{
	KindIn: 0, KindFan: 1, KindPWM: 2, KindTemp: 3,
	KindCurr: 4, KindPower: 5, KindEnergy: 6, KindHumidity: 7,
}

// scale converts raw sysfs integers to presentation units
// (m°C -> °C, mV -> V, µW -> W, ...).
var scale = map[Kind]float64{
	KindIn:       1000,
	KindFan:      1,
	KindPWM:      1,
	KindTemp:     1000,
	KindCurr:     1000,
	KindPower:    1e6,
	KindEnergy:   1e6,
	KindHumidity: 1000,
}

// limitAttrs are the per-channel threshold attributes worth surfacing.
var limitAttrs = map[string]bool{
	"min": true, "max": true, "crit": true,
	"lcrit": true, "emergency": true, "cap": true,
}

// Channel is one sensor channel on a chip. Together with the chip's
// StablePath it forms the persistent sensor identity
// (hwmon:<stable-path>:<id>), never the hwmonN index.
type Channel struct {
	ID     string // e.g. "temp1"
	Kind   Kind
	Index  int
	Label  string             // <ID>_label contents, or ID when absent
	Limits map[string]float64 // scaled min/max/crit/lcrit/emergency/cap

	inputPath string
}

// Chip is one hwmon device.
type Chip struct {
	Name       string // chip driver name, e.g. "k10temp"
	Hwmon      string // kernel class index, e.g. "hwmon2"; NOT stable across boots
	StablePath string // device topology path, e.g. "pci-0000:00:18.3"
	Channels   []Channel
}

// Reading is one collected channel value.
type Reading struct {
	Channel Channel
	Value   float64
	Err     error
}

var attrRE = regexp.MustCompile(`^(in|fan|pwm|temp|curr|power|energy|humidity)([0-9]+)(?:_(.+))?$`)

var pciAddrRE = regexp.MustCompile(`^[0-9a-f]{4}:[0-9a-f]{2}:[0-9a-f]{2}\.[0-9a-f]$`)

// Discover enumerates all chips under root (normally /sys/class/hwmon).
// Directories without a name file are skipped; unreadable attributes are
// tolerated so one broken chip never hides the rest.
func Discover(root string) ([]Chip, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("hwmon: reading %s: %w", root, err)
	}

	var chips []Chip
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), "hwmon") {
			continue
		}
		dir := filepath.Join(root, e.Name())
		name, err := readString(filepath.Join(dir, "name"))
		if err != nil {
			continue
		}
		sp := stablePath(dir)
		if sp == "" {
			// No device link at all: key by chip name so two virtual
			// chips never collide.
			sp = "virtual/" + name
		}
		chips = append(chips, Chip{
			Name:       name,
			Hwmon:      e.Name(),
			StablePath: sp,
			Channels:   discoverChannels(dir),
		})
	}

	sort.Slice(chips, func(i, j int) bool {
		return hwmonIndex(chips[i].Hwmon) < hwmonIndex(chips[j].Hwmon)
	})
	return chips, nil
}

// Collect reads the current value of every channel on the chip. This is the
// hot path: no discovery work happens here, only input-file reads.
func Collect(c Chip) []Reading {
	readings := make([]Reading, 0, len(c.Channels))
	for _, ch := range c.Channels {
		r := Reading{Channel: ch}
		r.Value, r.Err = readValue(ch.inputPath, ch.Kind)
		readings = append(readings, r)
	}
	return readings
}

func discoverChannels(dir string) []Channel {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	byID := map[string]*Channel{}
	get := func(kind Kind, index int) *Channel {
		id := string(kind) + strconv.Itoa(index)
		ch, ok := byID[id]
		if !ok {
			ch = &Channel{ID: id, Kind: kind, Index: index, Label: id}
			byID[id] = ch
		}
		return ch
	}

	for _, e := range entries {
		m := attrRE.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		kind := Kind(m[1])
		index, _ := strconv.Atoi(m[2])
		suffix := m[3]
		path := filepath.Join(dir, e.Name())
		ch := get(kind, index)

		switch {
		case suffix == "input", suffix == "" && kind == KindPWM:
			ch.inputPath = path
		case suffix == "average" && kind == KindPower:
			if ch.inputPath == "" {
				ch.inputPath = path
			}
		case suffix == "label":
			if label, err := readString(path); err == nil {
				ch.Label = label
			}
		case limitAttrs[suffix]:
			if v, err := readValue(path, kind); err == nil {
				if ch.Limits == nil {
					ch.Limits = map[string]float64{}
				}
				ch.Limits[suffix] = v
			}
		}
	}

	var channels []Channel
	for _, ch := range byID {
		if ch.inputPath != "" {
			channels = append(channels, *ch)
		}
	}
	sort.Slice(channels, func(i, j int) bool {
		a, b := channels[i], channels[j]
		if a.Kind != b.Kind {
			return kindOrder[a.Kind] < kindOrder[b.Kind]
		}
		return a.Index < b.Index
	})
	return channels
}

// stablePath derives a boot-stable identity for the chip from its device
// symlink (platform/PCI/I2C/USB topology). Returns "" for chips without a
// device link.
func stablePath(hwmonDir string) string {
	resolved, err := filepath.EvalSymlinks(filepath.Join(hwmonDir, "device"))
	if err != nil {
		return ""
	}
	_, rel, found := strings.Cut(resolved, "/devices/")
	if !found {
		return resolved
	}

	// Compress PCI paths: drop root-complex and bridge segments, keep the
	// terminal device address plus anything below it (e.g. nvme/nvme0).
	segs := strings.Split(rel, "/")
	last := -1
	for i, s := range segs {
		if pciAddrRE.MatchString(s) {
			last = i
		}
	}
	if last >= 0 {
		return "pci-" + strings.Join(segs[last:], "/")
	}
	return rel
}

func hwmonIndex(name string) int {
	n, _ := strconv.Atoi(strings.TrimPrefix(name, "hwmon"))
	return n
}

func readString(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

func readValue(path string, kind Kind) (float64, error) {
	s, err := readString(path)
	if err != nil {
		return 0, err
	}
	raw, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, err
	}
	return float64(raw) / scale[kind], nil
}
