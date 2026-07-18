package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/mappu/miqt/qt6"

	"github.com/zen66ten/HW-T/internal/core"
	"github.com/zen66ten/HW-T/pkg/client"
)

// rebuildSummary tears down and rebuilds the three-column System Summary
// (CPU | Motherboard+Memory | GPU+OS+Drives) from the current device
// inventory. Live values register themselves in u.live and are refreshed
// by the tick timer.
func (u *ui) rebuildSummary(devices []client.Device, sensors []client.Sensor) {
	u.live = nil

	content := qt6.NewQWidget2()
	cols := hbox(6)
	cols.SetContentsMargins(0, 0, 0, 0)

	left := vbox(6)
	left.AddWidget(u.buildCPU(devices, sensors).QWidget)
	left.AddStretch()

	mid := vbox(6)
	mid.AddWidget(u.buildMotherboard(devices).QWidget)
	mid.AddWidget(u.buildMemory(devices).QWidget)
	mid.AddWidget(u.buildMemoryModules(devices).QWidget)
	mid.AddStretch()
	midW := wrap(mid.QLayout)
	midW.SetMinimumWidth(440)

	right := vbox(6)
	right.AddWidget(u.buildGPU(devices, sensors).QWidget)
	right.AddWidget(u.buildCurrentClocks(devices, sensors).QWidget)
	right.AddWidget(u.buildOS(devices).QWidget)
	right.AddWidget(u.buildDrives(devices).QWidget)
	right.AddStretch()
	rightW := wrap(right.QLayout)
	rightW.SetMinimumWidth(440)

	cols.AddWidget2(wrap(left.QLayout), 36)
	cols.AddWidget2(midW, 32)
	cols.AddWidget2(rightW, 32)
	content.SetLayout(cols.QLayout)

	old := u.summaryBody
	u.summarySlot.RemoveWidget(old)
	old.DeleteLater()
	u.summaryBody = content
	u.summarySlot.AddWidget(content)
}

// --- lookup helpers ---

func findDev(devices []client.Device, idPrefix string) *client.Device {
	for i := range devices {
		if strings.HasPrefix(devices[i].ID, idPrefix) {
			return &devices[i]
		}
	}
	return nil
}

func devsWithPrefix(devices []client.Device, idPrefix string) []client.Device {
	var out []client.Device
	for _, d := range devices {
		if strings.HasPrefix(d.ID, idPrefix) {
			out = append(out, d)
		}
	}
	return out
}

func gpuDevices(devices []client.Device) []client.Device {
	var out []client.Device
	for _, d := range devices {
		if d.Provider == "nvidia" || d.Provider == "amdgpu" || d.Provider == "intel" {
			out = append(out, d)
		}
	}
	return out
}

// sensorByChannel finds the sensor "<devID>:<channel>" among the known
// sensors, trying each channel name in order (providers name the same
// thing differently: nvidia clock_mem vs amdgpu mclk).
func sensorByChannel(sensors []client.Sensor, devID string, channels ...string) string {
	for _, ch := range channels {
		id := devID + ":" + ch
		for _, s := range sensors {
			if s.ID == id {
				return id
			}
		}
	}
	return ""
}

// voltageByLabel returns the ID of the first voltage sensor whose enriched
// label matches one of names, searching all sensors since voltage rails
// live on a motherboard Super-I/O chip, not the CPU device. Returns "" when
// no driver exposes a matching rail.
func voltageByLabel(sensors []client.Sensor, names ...string) string {
	for _, s := range sensors {
		if core.Kind(s.Kind) != core.KindIn {
			continue
		}
		label := core.EnrichLabel(s.DeviceName, channelOf(s), s.Label)
		for _, n := range names {
			if label == n {
				return s.ID
			}
		}
	}
	return ""
}

func attr(d *client.Device, key string) string {
	if d == nil {
		return ""
	}
	return d.Attrs[key]
}

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}

// --- live render helpers ---

func (u *ui) addLive(l *qt6.QLabel, render func(map[string]client.Sensor) (string, string)) {
	u.live = append(u.live, liveLabel{l: l, render: render})
}

// liveField adds a bordered value box that re-renders one sensor per tick.
func (u *ui) liveField(id string, kind core.Kind) *qt6.QLabel {
	l := field("-")
	u.addLive(l, func(snap map[string]client.Sensor) (string, string) {
		return renderSensor(snap, id, kind), ""
	})
	return l
}

func renderSensor(snap map[string]client.Sensor, id string, kind core.Kind) string {
	s, ok := snap[id]
	if !ok || s.Err != "" || s.N == 0 {
		return "-"
	}
	return core.FormatValue(kind, s.Cur)
}

// avgOf averages the current values of the given sensors.
func avgOf(snap map[string]client.Sensor, ids []string) (float64, bool) {
	sum, n := 0.0, 0
	for _, id := range ids {
		if s, ok := snap[id]; ok && s.Err == "" && s.N > 0 {
			sum += s.Cur
			n++
		}
	}
	if n == 0 {
		return 0, false
	}
	return sum / float64(n), true
}

// ---------------------------------------------------------------- CPU column

func (u *ui) buildCPU(devices []client.Device, sensors []client.Sensor) *qt6.QGroupBox {
	gb := group("CPU")
	root := vbox(6)

	info := findDev(devices, "cpuinfo:")
	dmiCPU := findDev(devices, "dmi:cpu")
	freqDev := findDev(devices, "cpu:cpufreq")

	name := attr(info, "name")
	if name == "" && dmiCPU != nil {
		name = dmiCPU.Name
	}

	// Vendor badge + identity fields.
	top := hbox(8)
	top.AddWidget3(cpuLogo(attr(info, "vendor"), name).QWidget, 0, qt6.AlignTop)

	fms := "-"
	if info != nil {
		fms = attr(info, "family") + " / " + attr(info, "model") + " / " + attr(info, "stepping")
	}
	id := grid()
	id.AddWidget3(field(orDash(name)).QWidget, 0, 0, 1, 4)
	id.AddWidget2(capL("F / M / S").QWidget, 1, 0)
	id.AddWidget2(field(fms).QWidget, 1, 1)
	id.AddWidget2(capL("MCU").QWidget, 1, 2)
	id.AddWidget2(field(orDash(attr(info, "microcode"))).QWidget, 1, 3)
	id.AddWidget2(capL("Socket").QWidget, 2, 0)
	id.AddWidget2(field(orDash(attr(dmiCPU, "socket"))).QWidget, 2, 1)
	id.AddWidget2(capL("Max Speed").QWidget, 2, 2)
	maxSpeed := "-"
	if ms := attr(dmiCPU, "max_speed_mhz"); ms != "" {
		maxSpeed = ms + " MHz"
	}
	id.AddWidget2(field(maxSpeed).QWidget, 2, 3)
	id.AddWidget2(capL("Governor").QWidget, 3, 0)
	id.AddWidget2(field(orDash(attr(freqDev, "governor"))).QWidget, 3, 1)
	id.AddWidget2(capL("Driver").QWidget, 3, 2)
	id.AddWidget2(field(orDash(attr(freqDev, "driver"))).QWidget, 3, 3)
	id.SetColumnStretch(1, 3)
	id.SetColumnStretch(3, 2)
	id.SetColumnMinimumWidth(0, 62)
	id.SetColumnMinimumWidth(2, 40)
	top.AddWidget2(wrap(id.QLayout), 1)
	root.AddLayout(top.QLayout)

	// Core / cache row.
	cores := attr(info, "cores")
	threads := attr(info, "threads")
	cc := grid()
	pairs := [][2]string{
		{"Core", orDash(cores) + " / " + orDash(threads)},
		{"Cache L1", orDash(attr(info, "cache_L1d"))},
		{"L2", orDash(attr(info, "cache_L2"))},
		{"L3", orDash(attr(info, "cache_L3"))},
	}
	for i, p := range pairs {
		cc.AddWidget2(capL(p[0]).QWidget, 0, i*2)
		cc.AddWidget2(field(p[1]).QWidget, 0, i*2+1)
		cc.SetColumnStretch(i*2+1, 1)
	}
	root.AddLayout(cc.QLayout)

	// Live CPU voltages. On AMD these come from the SVI2 telemetry the
	// zenpower driver exposes (k10temp reports none); on Intel/other boards
	// from a Super-I/O chip. Reads "-" until such a driver is loaded.
	vr := hbox(5)
	vr.AddWidget(capL("Core Voltage").QWidget)
	vr.AddWidget2(u.liveField(voltageByLabel(sensors, "CPU Core Voltage"), core.KindIn).QWidget, 1)
	vr.AddWidget(capL("SoC Voltage").QWidget)
	vr.AddWidget2(u.liveField(voltageByLabel(sensors, "CPU SoC Voltage"), core.KindIn).QWidget, 1)
	root.AddLayout(vr.QLayout)

	root.AddWidget(buildFeatures(attr(info, "instructions")).QWidget)
	root.AddWidget(u.opTable(sensors).QWidget)

	if mit := attr(info, "mitigations"); mit != "" {
		r := hbox(5)
		r.AddWidget(capW("Mitigations", 70).QWidget)
		style := greenQSS
		if !strings.Contains(mit, "all") {
			style = redQSS
		}
		r.AddWidget(lbl(mit, style, qt6.AlignLeft|qt6.AlignVCenter).QWidget)
		r.AddStretch()
		root.AddLayout(r.QLayout)
	}

	root.AddStretch()
	gb.SetLayout(root.QLayout)
	gb.SetMinimumWidth(520)
	return gb
}

// featureGrid is the HWiNFO-style instruction-set grid: 7 columns, green
// when supported, gray when absent; virtualization (AMD-V / VT-x) is the
// red highlight. Names match the cpuinfo provider's summary line.
var featureGrid = [][]string{
	{"MMX", "SSE", "SSE2", "SSE3", "SSSE3", "SSE4.1", "SSE4.2"},
	{"SSE4A", "x86-64", "AVX", "AVX2", "AVX-512", "FMA3", "SHA"},
	{"AES", "AMD-V", "VT-x", "BMI1", "BMI2", "RDRAND", "RDSEED"},
}

func buildFeatures(instructions string) *qt6.QGroupBox {
	supported := map[string]bool{}
	for _, f := range strings.Split(instructions, ",") {
		supported[strings.TrimSpace(f)] = true
	}
	gb := group("Features")
	g := grid()
	g.SetHorizontalSpacing(10)
	g.SetVerticalSpacing(3)
	for r, row := range featureGrid {
		for c, f := range row {
			state := 0
			if supported[f] {
				state = 1
				if f == "AMD-V" || f == "VT-x" {
					state = 2
				}
			}
			g.AddWidget2(featLbl(f, state).QWidget, r, c)
		}
	}
	for c := 0; c < 7; c++ {
		g.SetColumnStretch(c, 1)
	}
	gb.SetLayout(g.QLayout)
	return gb
}

// opTable is the Operating Point table: clock limits from cpufreq plus the
// live average active/effective core clocks.
func (u *ui) opTable(sensors []client.Sensor) *qt6.QFrame {
	var clockIDs, effIDs []string
	var minMHz, maxMHz float64
	for _, s := range sensors {
		if s.Provider != "cpu" {
			continue
		}
		if strings.HasSuffix(s.ID, "_eff") {
			effIDs = append(effIDs, s.ID)
			continue
		}
		clockIDs = append(clockIDs, s.ID)
		if minMHz == 0 {
			minMHz = s.Limits["min"]
			maxMHz = s.Limits["max"]
		}
	}

	f := frame()
	g := grid()
	g.SetContentsMargins(2, 2, 2, 2)
	g.SetHorizontalSpacing(4)
	g.SetVerticalSpacing(1)
	for c, h := range []string{"Operating Point", "Clock", "Ratio", "Bus"} {
		al := qt6.AlignRight | qt6.AlignVCenter
		if c == 0 {
			al = qt6.AlignLeft | qt6.AlignVCenter
		}
		g.AddWidget4(th(h, al).QWidget, 0, c, al)
	}

	right := qt6.AlignRight | qt6.AlignVCenter
	leftAl := qt6.AlignLeft | qt6.AlignVCenter
	row := 1
	addStatic := func(name string, mhz float64) {
		if mhz <= 0 {
			return
		}
		g.AddWidget4(td(name, leftAl).QWidget, row, 0, leftAl)
		g.AddWidget4(td(fmt.Sprintf("%.1f MHz", mhz), right).QWidget, row, 1, right)
		g.AddWidget4(td(fmt.Sprintf("x%.2f", mhz/100), right).QWidget, row, 2, right)
		g.AddWidget4(td("100.0 MHz", right).QWidget, row, 3, right)
		row++
	}
	addLiveAvg := func(name string, ids []string) {
		if len(ids) == 0 {
			return
		}
		g.AddWidget4(td(name, leftAl).QWidget, row, 0, leftAl)
		clock := td("-", right)
		ratio := td("-", right)
		g.AddWidget4(clock.QWidget, row, 1, right)
		g.AddWidget4(ratio.QWidget, row, 2, right)
		g.AddWidget4(td("100.0 MHz", right).QWidget, row, 3, right)
		u.addLive(clock, func(snap map[string]client.Sensor) (string, string) {
			v, ok := avgOf(snap, ids)
			if !ok {
				return "-", ""
			}
			return fmt.Sprintf("%.1f MHz", v), ""
		})
		u.addLive(ratio, func(snap map[string]client.Sensor) (string, string) {
			v, ok := avgOf(snap, ids)
			if !ok {
				return "-", ""
			}
			return fmt.Sprintf("x%.2f", v/100), ""
		})
		row++
	}

	addStatic("Minimum Clock", minMHz)
	addStatic("Maximum Clock", maxMHz)
	addLiveAvg("Avg. Active Clock", clockIDs)
	addLiveAvg("Avg. Effective Clock", effIDs)
	if row == 1 {
		g.AddWidget4(td("no cpufreq data", leftAl).QWidget, 1, 0, leftAl)
	}
	g.SetColumnStretch(0, 1)
	f.SetLayout(g.QLayout)
	return f
}

// -------------------------------------------------------- Motherboard column

func (u *ui) buildMotherboard(devices []client.Device) *qt6.QGroupBox {
	gb := group("Motherboard")
	root := vbox(5)

	board := findDev(devices, "dmi:board")
	bios := findDev(devices, "dmi:bios")
	host := findDev(devices, "system:host")

	if board == nil {
		root.AddWidget(lbl("SMBIOS inventory needs hwtd running as root", grayQSS,
			qt6.AlignLeft|qt6.AlignVCenter).QWidget)
	} else {
		root.AddWidget(field(strings.TrimSpace(attr(board, "manufacturer") + " " + attr(board, "product"))).QWidget)
	}

	g := grid()
	g.AddWidget2(capL("BIOS Date").QWidget, 0, 0)
	g.AddWidget2(field(orDash(attr(bios, "date"))).QWidget, 0, 1)
	g.AddWidget2(capL("Version").QWidget, 0, 2)
	g.AddWidget2(field(orDash(attr(bios, "version"))).QWidget, 0, 3)
	fw := attr(host, "firmware")
	fwStyle := greenQSS
	if fw != "UEFI" {
		fwStyle = redQSS
	}
	g.AddWidget4(lbl(orDash(fw), fwStyle, qt6.AlignCenter).QWidget, 0, 4, qt6.AlignLeft|qt6.AlignVCenter)
	g.SetColumnStretch(1, 2)
	g.SetColumnStretch(3, 1)
	g.SetColumnMinimumWidth(0, 62)
	root.AddLayout(g.QLayout)

	gb.SetLayout(root.QLayout)
	return gb
}

func (u *ui) buildMemory(devices []client.Device) *qt6.QGroupBox {
	gb := group("Memory")
	root := vbox(4)

	mods := devsWithPrefix(devices, "dmi:mem")
	host := findDev(devices, "system:host")

	totalMB := 0
	memType, speed := "", ""
	for _, m := range mods {
		if mb, err := strconv.Atoi(m.Attrs["size_mb"]); err == nil {
			totalMB += mb
		}
		if memType == "" {
			memType = m.Attrs["type"]
		}
		if speed == "" {
			speed = m.Attrs["configured_mts"]
		}
	}
	size := "-"
	if totalMB > 0 {
		size = fmt.Sprintf("%d GB", totalMB/1024)
	} else if t := attr(host, "memory_total"); t != "" {
		size = t
	}

	r1 := hbox(5)
	r1.AddWidget(capW("Size", 52).QWidget)
	r1.AddWidget2(field(size).QWidget, 2)
	r1.AddWidget(capW("Type", 40).QWidget)
	r1.AddWidget2(field(orDash(memType)).QWidget, 3)
	root.AddLayout(r1.QLayout)

	clock := "-"
	if mts, err := strconv.Atoi(speed); err == nil && mts > 0 {
		clock = fmt.Sprintf("%.1f MHz", float64(mts)/2)
	}
	r2 := hbox(5)
	r2.AddWidget(capW("Clock", 52).QWidget)
	r2.AddWidget2(field(clock).QWidget, 3)
	r2.AddWidget(sym("=").QWidget)
	rate := "-"
	if speed != "" {
		rate = speed + " MT/s"
	}
	r2.AddWidget2(field(rate).QWidget, 3)
	root.AddLayout(r2.QLayout)

	r3 := hbox(5)
	r3.AddWidget(capW("Modules", 56).QWidget)
	count := "-"
	if len(mods) > 0 {
		count = strconv.Itoa(len(mods))
	}
	r3.AddWidget2(field(count).QWidget, 1)
	r3.AddWidget(capW("ECC", 32).QWidget)
	ecc := "-"
	if findDev(devices, "edac:") != nil {
		ecc = "Yes"
	} else if len(mods) > 0 {
		ecc = "No"
	}
	r3.AddWidget2(field(ecc).QWidget, 1)
	root.AddLayout(r3.QLayout)

	gb.SetLayout(root.QLayout)
	return gb
}

func (u *ui) buildMemoryModules(devices []client.Device) *qt6.QGroupBox {
	gb := group("Memory Modules")
	root := vbox(5)

	mods := devsWithPrefix(devices, "dmi:mem")
	if len(mods) == 0 {
		root.AddWidget(lbl("no module data (run hwtd as root)", grayQSS,
			qt6.AlignLeft|qt6.AlignVCenter).QWidget)
		gb.SetLayout(root.QLayout)
		return gb
	}

	var items []string
	for i, m := range mods {
		items = append(items, fmt.Sprintf("#%d [%s]: %s %s", i, m.Name,
			m.Attrs["manufacturer"], m.Attrs["part_number"]))
	}
	if u.memIndex >= len(mods) {
		u.memIndex = 0
	}
	cb := combo(items, u.memIndex)
	cb.OnCurrentIndexChanged(func(idx int) {
		u.mu.Lock()
		u.memIndex = idx
		u.rebuild = true
		u.mu.Unlock()
	})
	root.AddWidget(cb.QWidget)

	m := mods[u.memIndex]
	sizeGB := "-"
	if mb, err := strconv.Atoi(m.Attrs["size_mb"]); err == nil {
		sizeGB = fmt.Sprintf("%d GB", mb/1024)
	}
	r1 := hbox(5)
	r1.AddWidget(capW("Size", 52).QWidget)
	r1.AddWidget2(field(sizeGB).QWidget, 2)
	r1.AddWidget(capW("Rated", 44).QWidget)
	r1.AddWidget2(field(orDash(m.Attrs["speed_mts"])+" MT/s").QWidget, 2)
	r1.AddWidget(capW("Bank", 36).QWidget)
	r1.AddWidget2(field(orDash(m.Attrs["bank"])).QWidget, 2)
	root.AddLayout(r1.QLayout)

	r2 := hbox(5)
	r2.AddWidget(capW("Type", 52).QWidget)
	typ := strings.TrimSpace(m.Attrs["type"] + "-" + m.Attrs["configured_mts"])
	r2.AddWidget2(field(typ).QWidget, 1)
	root.AddLayout(r2.QLayout)

	gb.SetLayout(root.QLayout)
	return gb
}

// ---------------------------------------------------------------- GPU column

func (u *ui) buildGPU(devices []client.Device, sensors []client.Sensor) *qt6.QGroupBox {
	gb := group("GPU")
	root := vbox(5)

	gpus := gpuDevices(devices)
	if len(gpus) == 0 {
		root.AddWidget(lbl("no GPU detected", grayQSS, qt6.AlignLeft|qt6.AlignVCenter).QWidget)
		gb.SetLayout(root.QLayout)
		return gb
	}
	if u.gpuIndex >= len(gpus) {
		u.gpuIndex = 0
	}
	g := gpus[u.gpuIndex]

	top := hbox(8)
	top.AddWidget3(gpuLogo(g.Provider, g.Name).QWidget, 0, qt6.AlignTop)
	id := vbox(4)
	id.AddWidget(field(g.Name).QWidget)
	drv := g.Attrs["driver_version"]
	if drv == "" {
		drv = g.Attrs["driver"]
	}
	id.AddWidget(fieldL("Driver  " + orDash(drv)).QWidget)
	id.AddWidget(fieldL("VBIOS  " + orDash(g.Attrs["vbios"])).QWidget)
	top.AddWidget2(wrap(id.QLayout), 1)
	root.AddLayout(top.QLayout)

	r1 := hbox(5)
	var names []string
	for i := range gpus {
		names = append(names, fmt.Sprintf("GPU #%d", i))
	}
	cb := combo(names, u.gpuIndex)
	cb.SetFixedWidth(92)
	cb.OnCurrentIndexChanged(func(idx int) {
		u.mu.Lock()
		u.gpuIndex = idx
		u.rebuild = true
		u.mu.Unlock()
	})
	r1.AddWidget(cb.QWidget)
	vram := "-"
	if v, err := strconv.Atoi(g.Attrs["vram_total_mib"]); err == nil {
		vram = fmt.Sprintf("%d GB", (v+512)/1024)
	}
	r1.AddWidget2(field(vram).QWidget, 1)
	r1.AddWidget2(field(orDash(g.Attrs["pcie_link"])).QWidget, 2)
	root.AddLayout(r1.QLayout)

	// Live GPU telemetry: temperature, power, load, VRAM, fan.
	tempID := sensorByChannel(sensors, g.ID, "temp", "edge", "junction")
	powerID := sensorByChannel(sensors, g.ID, "power", "PPT")
	loadID := sensorByChannel(sensors, g.ID, "util_gpu", "busy")
	vramID := sensorByChannel(sensors, g.ID, "vram_used")
	fanID := sensorByChannel(sensors, g.ID, "fan")

	r2 := hbox(5)
	r2.AddWidget(capW("Temp", 40).QWidget)
	r2.AddWidget2(u.liveField(tempID, core.KindTemp).QWidget, 1)
	r2.AddWidget(capW("Power", 44).QWidget)
	r2.AddWidget2(u.liveField(powerID, core.KindPower).QWidget, 1)
	r2.AddWidget(capW("Load", 36).QWidget)
	r2.AddWidget2(u.liveField(loadID, core.KindPercent).QWidget, 1)
	root.AddLayout(r2.QLayout)

	r3 := hbox(5)
	r3.AddWidget(capW("VRAM", 40).QWidget)
	r3.AddWidget2(u.liveField(vramID, core.KindData).QWidget, 1)
	r3.AddWidget(capW("Fan", 44).QWidget)
	r3.AddWidget2(u.liveField(fanID, core.KindPercent).QWidget, 1)
	root.AddLayout(r3.QLayout)

	gb.SetLayout(root.QLayout)
	return gb
}

func (u *ui) buildCurrentClocks(devices []client.Device, sensors []client.Sensor) *qt6.QGroupBox {
	gb := group("Current Clocks (MHz)")
	r := hbox(5)

	gpus := gpuDevices(devices)
	var gpuID, memID, videoID string
	if len(gpus) > 0 && u.gpuIndex < len(gpus) {
		d := gpus[u.gpuIndex]
		gpuID = sensorByChannel(sensors, d.ID, "clock_graphics", "sclk", "rcs_act", "gpu_act")
		memID = sensorByChannel(sensors, d.ID, "clock_mem", "mclk")
		videoID = sensorByChannel(sensors, d.ID, "clock_video", "vcs_act")
	}

	r.AddWidget(capW("GPU", 34).QWidget)
	r.AddWidget2(u.liveField(gpuID, core.KindFreq).QWidget, 1)
	r.AddWidget(capW("Memory", 54).QWidget)
	r.AddWidget2(u.liveField(memID, core.KindFreq).QWidget, 1)
	r.AddWidget(capW("Video", 44).QWidget)
	r.AddWidget2(u.liveField(videoID, core.KindFreq).QWidget, 1)
	gb.SetLayout(r.QLayout)
	return gb
}

func (u *ui) buildOS(devices []client.Device) *qt6.QGroupBox {
	gb := group("Operating System")
	root := vbox(5)

	host := findDev(devices, "system:host")

	flags := hbox(16)
	fwStyle := grayQSS
	if attr(host, "firmware") == "UEFI" {
		fwStyle = greenQSS
	}
	flags.AddWidget(lbl("UEFI Boot", fwStyle, qt6.AlignLeft|qt6.AlignVCenter).QWidget)

	sbStyle := grayQSS
	switch attr(host, "secure_boot") {
	case "enabled":
		sbStyle = greenQSS
	case "disabled":
		sbStyle = redQSS
	}
	flags.AddWidget(lbl("Secure Boot", sbStyle, qt6.AlignLeft|qt6.AlignVCenter).QWidget)

	tpmStyle := grayQSS
	if strings.HasPrefix(attr(host, "tpm"), "present") {
		tpmStyle = greenQSS
	}
	flags.AddWidget(lbl("TPM", tpmStyle, qt6.AlignLeft|qt6.AlignVCenter).QWidget)
	flags.AddStretch()
	root.AddLayout(flags.QLayout)

	root.AddWidget(fieldL(orDash(attr(host, "os"))).QWidget)
	kernel := strings.TrimSpace(attr(host, "kernel") + "   up " + attr(host, "uptime"))
	root.AddWidget(fieldL(kernel).QWidget)

	gb.SetLayout(root.QLayout)
	return gb
}

func (u *ui) buildDrives(devices []client.Device) *qt6.QGroupBox {
	gb := group("Drives")
	root := vbox(0)

	disks := devsWithPrefix(devices, "smart:")
	if len(disks) == 0 {
		root.AddWidget(lbl("no drives detected", grayQSS, qt6.AlignLeft|qt6.AlignVCenter).QWidget)
		gb.SetLayout(root.QLayout)
		return gb
	}

	f := frame()
	g := grid()
	g.SetContentsMargins(2, 2, 2, 2)
	g.SetHorizontalSpacing(6)
	g.SetVerticalSpacing(1)
	leftAl := qt6.AlignLeft | qt6.AlignVCenter
	g.AddWidget2(th("", qt6.AlignLeft).QWidget, 0, 0)
	g.AddWidget4(th("Interface", leftAl).QWidget, 0, 1, leftAl)
	g.AddWidget4(th("Model [Capacity]", leftAl).QWidget, 0, 2, leftAl)

	for i, d := range disks {
		iface := d.Attrs["type"]
		if strings.Contains(d.Attrs["device"], "nvme") {
			iface = "NVMe " + iface
		} else if iface != "" {
			iface = "SATA " + iface
		}
		// Health check mark goes green/red with the drive's SMART status.
		chk := lbl("?", grayQSS, qt6.AlignCenter)
		healthID := d.ID + ":health"
		u.addLive(chk, func(snap map[string]client.Sensor) (string, string) {
			s, ok := snap[healthID]
			if !ok || s.Err != "" || s.N == 0 {
				return "?", grayQSS
			}
			if s.Cur != 0 {
				return "✔", chkQSS
			}
			return "✘", redQSS
		})
		g.AddWidget4(chk.QWidget, i+1, 0, qt6.AlignCenter)
		g.AddWidget4(td(orDash(iface), leftAl).QWidget, i+1, 1, leftAl)
		model := d.Name
		if size := d.Attrs["capacity"]; size != "" {
			model += "  [" + size + "]"
		}
		g.AddWidget4(td(model, leftAl).QWidget, i+1, 2, leftAl)
	}
	g.SetColumnStretch(2, 1)
	f.SetLayout(g.QLayout)
	root.AddWidget(f.QWidget)
	gb.SetLayout(root.QLayout)
	return gb
}
