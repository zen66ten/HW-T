package main

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"github.com/zen66ten/HW-T/internal/core"
	"github.com/zen66ten/HW-T/pkg/client"
)

var gpuChips = map[string]bool{"amdgpu": true, "i915": true, "xe": true, "nouveau": true}
var cpuChips = map[string]bool{"k10temp": true, "coretemp": true, "zenpower": true}
var storageChips = map[string]bool{"nvme": true, "drivetemp": true}
var memoryChips = map[string]bool{"jc42": true, "spd5118": true}

// componentOrder fixes the frame and tree order.
var componentOrder = []string{"CPU", "Motherboard", "Memory", "GPU", "Storage", "System"}

func componentOf(d client.Device) string {
	switch d.Provider {
	case "cpu", "rapl":
		return "CPU"
	case "nvidia", "amdgpu", "intel", "edid":
		return "GPU"
	case "smart":
		return "Storage"
	case "edac":
		return "Memory"
	case "usb", "pci":
		return "System"
	case "dmi":
		switch {
		case strings.HasPrefix(string(d.ID), "dmi:cpu"):
			return "CPU"
		case strings.HasPrefix(string(d.ID), "dmi:mem"):
			return "Memory"
		default:
			return "Motherboard"
		}
	case "hwmon":
		switch {
		case cpuChips[d.Name]:
			return "CPU"
		case gpuChips[d.Name]:
			return "GPU"
		case storageChips[d.Name]:
			return "Storage"
		case memoryChips[d.Name]:
			return "Memory"
		default:
			return "Motherboard"
		}
	}
	return "System"
}

type component struct {
	name    string
	devices []client.Device
	sensors []client.Sensor
}

func classify(devices []client.Device, sensors []client.Sensor) map[string]*component {
	byName := map[string]*component{}
	get := func(name string) *component {
		if byName[name] == nil {
			byName[name] = &component{name: name}
		}
		return byName[name]
	}
	compOfDev := map[string]string{}
	for _, d := range devices {
		c := componentOf(d)
		compOfDev[d.ID] = c
		get(c).devices = append(get(c).devices, d)
	}
	for _, s := range sensors {
		c, ok := compOfDev[s.Device]
		if !ok {
			c = "Motherboard"
		}
		get(c).sensors = append(get(c).sensors, s)
	}
	return byName
}

func channelOf(s client.Sensor) string {
	return strings.TrimPrefix(s.ID, s.Device+":")
}

// =========================================================================
// System Summary window: every component frame at once.
// =========================================================================

func (u *ui) rebuildSummary(sensors []client.Sensor) {
	if u.summaryBody == nil {
		return
	}
	u.mu.Lock()
	devices := u.devices
	u.mu.Unlock()

	comps := classify(devices, sensors)
	u.summaryRows = nil

	frame := func(name string) fyne.CanvasObject {
		c := comps[name]
		if c == nil {
			c = &component{name: name}
		}
		return u.buildFrame(c)
	}
	col1 := container.NewVBox(frame("CPU"))
	col2 := container.NewVBox(frame("Motherboard"), frame("Memory"))
	col3 := container.NewVBox(frame("GPU"), frame("Storage"))
	grid := container.NewGridWithColumns(3, col1, col2, col3)

	u.summaryBody.Objects = []fyne.CanvasObject{container.NewVScroll(grid)}
	u.summaryBody.Refresh()
}

func (u *ui) buildFrame(c *component) fyne.CanvasObject {
	title := c.name
	if t := frameTitle(c); t != "" {
		title = t
	}
	titleTxt := canvas.NewText(title, pal.accent)
	titleTxt.TextStyle.Bold = true
	titleTxt.TextSize = 13

	border := canvas.NewRectangle(nil)
	border.StrokeColor = pal.boxStroke
	border.StrokeWidth = 1
	border.CornerRadius = 4

	content := container.NewVBox(container.NewPadded(titleTxt), u.buildFrameInfo(c))
	return container.NewStack(border, container.NewPadded(content))
}

func frameTitle(c *component) string {
	switch c.name {
	case "CPU":
		for _, d := range c.devices {
			if strings.HasPrefix(d.ID, "dmi:cpu") {
				return d.Name
			}
		}
	case "GPU":
		for _, d := range c.devices {
			if d.Provider == "nvidia" || d.Provider == "amdgpu" || d.Provider == "intel" {
				return d.Name
			}
		}
	case "Motherboard":
		for _, d := range c.devices {
			if d.ID == "dmi:board" {
				return strings.TrimSpace(d.Attrs["manufacturer"] + " " + d.Attrs["product"])
			}
		}
	case "Storage":
		if len(c.devices) == 1 && c.devices[0].Provider == "smart" {
			return c.devices[0].Name
		}
	}
	return ""
}

func (u *ui) buildFrameInfo(c *component) fyne.CanvasObject {
	rows := container.New(layout.NewFormLayout())
	addStatic := func(label, value string) {
		if strings.TrimSpace(value) == "" {
			return
		}
		rows.Add(formLabel(label))
		vb := newValueBox()
		vb.Set(value, nil)
		rows.Add(vb)
	}
	addLive := func(label string, render func(map[string]client.Sensor) string) {
		rows.Add(formLabel(label))
		vb := newValueBox()
		rows.Add(vb)
		u.summaryRows = append(u.summaryRows, summaryRow{box: vb, render: render})
	}
	one := func(id string, kind core.Kind) func(map[string]client.Sensor) string {
		return func(snap map[string]client.Sensor) string {
			s, ok := snap[id]
			if !ok || s.Err != "" || s.N == 0 {
				return "-"
			}
			return core.FormatValue(kind, s.Cur)
		}
	}
	find := func(chip, label string) *client.Sensor {
		for i := range c.sensors {
			if c.sensors[i].DeviceName == chip && c.sensors[i].Label == label {
				return &c.sensors[i]
			}
		}
		return nil
	}
	findDev := func(idPrefix string) *client.Device {
		for i := range c.devices {
			if strings.HasPrefix(c.devices[i].ID, idPrefix) {
				return &c.devices[i]
			}
		}
		return nil
	}
	liveByChannel := func(label, channel string, kind core.Kind) bool {
		for _, s := range c.sensors {
			if channelOf(s) == channel {
				addLive(label, one(s.ID, kind))
				return true
			}
		}
		return false
	}

	switch c.name {
	case "CPU":
		if d := findDev("dmi:cpu"); d != nil {
			addStatic("Name", d.Name)
			addStatic("Socket", d.Attrs["socket"])
			addStatic("Cores / Threads", d.Attrs["cores"]+" / "+d.Attrs["threads"])
			addStatic("Max Speed", d.Attrs["max_speed_mhz"]+" MHz")
		}
		if d := findDev("cpu:cpufreq"); d != nil {
			addStatic("Scaling Driver", d.Attrs["driver"])
			addStatic("Governor", d.Attrs["governor"])
		}
		if s := find("k10temp", "Tctl"); s != nil {
			addLive("CPU (Tctl)", one(s.ID, core.KindTemp))
		} else if s := find("coretemp", "Package id 0"); s != nil {
			addLive("Package Temp", one(s.ID, core.KindTemp))
		}
		for _, s := range c.sensors {
			if s.Provider == "rapl" && channelOf(s) == "package" {
				addLive("Package Power", one(s.ID, core.KindPower))
				break
			}
		}
		var clockIDs []string
		for _, s := range c.sensors {
			if s.Provider == "cpu" && !strings.HasSuffix(s.ID, "_eff") {
				clockIDs = append(clockIDs, s.ID)
			}
		}
		if len(clockIDs) > 0 {
			addLive("Avg Core Clock", func(snap map[string]client.Sensor) string {
				sum, n := 0.0, 0
				for _, id := range clockIDs {
					if s, ok := snap[id]; ok && s.Err == "" && s.N > 0 {
						sum += s.Cur
						n++
					}
				}
				if n == 0 {
					return "-"
				}
				return core.FormatValue(core.KindFreq, sum/float64(n))
			})
		}

	case "GPU":
		for _, d := range c.devices {
			switch d.Provider {
			case "nvidia":
				addStatic("Name", d.Name)
				addStatic("VRAM", d.Attrs["vram_total_mib"]+" MiB")
			case "amdgpu", "intel":
				addStatic("Name", d.Name)
				if d.Attrs["vram_total_mib"] != "" {
					addStatic("VRAM", d.Attrs["vram_total_mib"]+" MiB")
				}
				addStatic("Driver", d.Attrs["driver"])
			case "edid":
				addStatic("Monitor", strings.TrimSpace(d.Name+"  "+d.Attrs["native_mode"]))
			}
		}
		liveByChannel("GPU Temp", "temp", core.KindTemp)
		liveByChannel("GPU Power", "power", core.KindPower)
		if !liveByChannel("GPU Load", "util_gpu", core.KindPercent) {
			liveByChannel("GPU Load", "busy", core.KindPercent)
		}
		liveByChannel("VRAM Used", "vram_used", core.KindData)
		if !liveByChannel("GPU Clock", "clock_graphics", core.KindFreq) {
			liveByChannel("GPU Clock", "gpu_act", core.KindFreq)
		}
		liveByChannel("Fan", "fan", core.KindPercent)

	case "Motherboard":
		if d := findDev("dmi:board"); d != nil {
			addStatic("Board", strings.TrimSpace(d.Attrs["manufacturer"]+" "+d.Attrs["product"]))
		}
		if d := findDev("dmi:bios"); d != nil {
			addStatic("BIOS", strings.TrimSpace(d.Attrs["version"]+" ("+d.Attrs["date"]+")"))
		}
		shown := 0
		for _, s := range c.sensors {
			if core.Kind(s.Kind) != core.KindTemp || shown >= 6 {
				continue
			}
			addLive(core.EnrichLabel(s.DeviceName, channelOf(s), s.Label), one(s.ID, core.KindTemp))
			shown++
		}

	case "Memory":
		totalMB, sticks := 0, 0
		for _, d := range c.devices {
			if strings.HasPrefix(d.ID, "dmi:mem") {
				if mb, err := strconv.Atoi(d.Attrs["size_mb"]); err == nil {
					totalMB += mb
					sticks++
				}
			}
		}
		if sticks > 0 {
			addStatic("Installed", fmt.Sprintf("%d GB in %d modules", totalMB/1024, sticks))
			for _, d := range c.devices {
				if strings.HasPrefix(d.ID, "dmi:mem") {
					addStatic(d.Name, strings.TrimSpace(d.Attrs["type"]+"-"+d.Attrs["configured_mts"]+" "+d.Attrs["manufacturer"]))
				}
			}
		}
		for _, s := range c.sensors {
			addLive(core.EnrichLabel(s.DeviceName, channelOf(s), s.Label), one(s.ID, core.Kind(s.Kind)))
		}

	case "Storage":
		for _, d := range c.devices {
			if d.Provider == "smart" {
				addStatic("Model", d.Name)
				addStatic("Firmware", d.Attrs["firmware_rev"])
			}
		}
		liveByChannel("Health", "health", core.KindHealth)
		liveByChannel("Wear", "percent_used", core.KindPercent)
		liveByChannel("Power-On Hours", "power_on_hours", core.KindCount)
		shown := 0
		for _, s := range c.sensors {
			if core.Kind(s.Kind) == core.KindTemp && shown < 3 {
				addLive(core.EnrichLabel(s.DeviceName, channelOf(s), s.Label), one(s.ID, core.KindTemp))
				shown++
			}
		}
	}

	if len(rows.Objects) == 0 {
		return container.NewPadded(widget.NewLabel("no data (run hwtd as root)"))
	}
	return rows
}

// =========================================================================
// Sensor Status window: aggregated current/min/max/avg table.
// =========================================================================

func (u *ui) rebuildSensors(sensors []client.Sensor) {
	if u.sensorBody == nil {
		return
	}
	u.rows = map[string]*sensorRow{}
	box := container.NewVBox()

	box.Add(container.New(tableRowLayout{},
		colHeader("Sensor", fyne.TextAlignLeading),
		colHeader("Current", fyne.TextAlignTrailing),
		colHeader("Minimum", fyne.TextAlignTrailing),
		colHeader("Maximum", fyne.TextAlignTrailing),
		colHeader("Average", fyne.TextAlignTrailing),
	))

	if len(sensors) == 0 {
		box.Add(widget.NewLabel("waiting for sensor data…"))
	}

	lastDev := ""
	for _, s := range sensors {
		if s.Device != lastDev {
			lastDev = s.Device
			hdr := canvas.NewText(core.DisplayName(s.DeviceName), pal.accent)
			hdr.TextStyle.Bold = true
			hdr.TextSize = 12
			box.Add(container.NewPadded(hdr))
		}
		label := core.EnrichLabel(s.DeviceName, channelOf(s), s.Label)
		name := canvas.NewText(label, pal.foreground)
		name.TextSize = 12
		row := &sensorRow{name: name, cur: valueText(), min: valueText(), max: valueText(), avg: valueText(), kind: core.Kind(s.Kind)}
		u.rows[s.ID] = row

		id := s.ID
		content := container.New(tableRowLayout{}, name, row.cur, row.min, row.max, row.avg)
		box.Add(newTappableRow(content, func() {
			u.mu.Lock()
			u.selected = id
			u.mu.Unlock()
			go u.refreshChart()
		}))
	}

	u.sensorBody.Objects = []fyne.CanvasObject{container.NewVScroll(box)}
	u.sensorBody.Refresh()
}

func colHeader(text string, align fyne.TextAlign) *canvas.Text {
	t := canvas.NewText(text, pal.dim)
	t.TextStyle.Bold = true
	t.TextSize = 12
	t.Alignment = align
	return t
}

func valueText() *canvas.Text {
	t := canvas.NewText("", pal.dim)
	t.TextSize = 12
	t.Alignment = fyne.TextAlignTrailing
	t.TextStyle.Monospace = true
	return t
}

func formLabel(text string) *canvas.Text {
	l := canvas.NewText(text, pal.dim)
	l.TextSize = 12
	l.Alignment = fyne.TextAlignTrailing
	return l
}

// =========================================================================
// Main window: hardware navigation tree + detail pane.
// =========================================================================

func (u *ui) treeChildren(uid widget.TreeNodeID) []widget.TreeNodeID {
	u.mu.Lock()
	defer u.mu.Unlock()
	comps := classify(u.devices, nil)

	if uid == "" {
		var out []widget.TreeNodeID
		for _, name := range componentOrder {
			if c, ok := comps[name]; ok && len(c.devices) > 0 {
				out = append(out, "comp|"+name)
			}
		}
		return out
	}
	if name, ok := strings.CutPrefix(uid, "comp|"); ok {
		c := comps[name]
		if c == nil {
			return nil
		}
		out := make([]widget.TreeNodeID, 0, len(c.devices))
		for _, d := range c.devices {
			out = append(out, "dev|"+d.ID)
		}
		return out
	}
	return nil
}

func (u *ui) treeIsBranch(uid widget.TreeNodeID) bool {
	return uid == "" || strings.HasPrefix(uid, "comp|")
}

func (u *ui) treeCreate(branch bool) fyne.CanvasObject {
	return widget.NewLabel("")
}

func (u *ui) treeUpdate(uid widget.TreeNodeID, branch bool, obj fyne.CanvasObject) {
	l := obj.(*widget.Label)
	if name, ok := strings.CutPrefix(uid, "comp|"); ok {
		l.TextStyle = fyne.TextStyle{Bold: true}
		l.SetText(name)
		return
	}
	l.TextStyle = fyne.TextStyle{}
	devID, _ := strings.CutPrefix(uid, "dev|")
	u.mu.Lock()
	defer u.mu.Unlock()
	for _, d := range u.devices {
		if d.ID == devID {
			l.SetText(core.DisplayName(d.Name))
			return
		}
	}
	l.SetText(devID)
}

func (u *ui) treeSelected(uid widget.TreeNodeID) {
	devID, ok := strings.CutPrefix(uid, "dev|")
	if !ok {
		u.tree.OpenBranch(uid)
		return
	}
	u.mu.Lock()
	var dev client.Device
	found := false
	for _, d := range u.devices {
		if d.ID == devID {
			dev, found = d, true
			break
		}
	}
	u.mu.Unlock()
	if !found {
		return
	}
	u.detail.Objects = []fyne.CanvasObject{attrForm(dev)}
	u.detail.Refresh()
}

func attrForm(d client.Device) fyne.CanvasObject {
	title := canvas.NewText(core.DisplayName(d.Name), pal.accent)
	title.TextStyle.Bold = true
	title.TextSize = 13

	form := container.New(layout.NewFormLayout())
	add := func(k, v string) {
		form.Add(formLabel(k))
		vb := newValueBox()
		vb.Set(v, nil)
		form.Add(vb)
	}
	add("device id", d.ID)
	keys := make([]string, 0, len(d.Attrs))
	for k := range d.Attrs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		add(k, d.Attrs[k])
	}
	return container.NewVBox(container.NewPadded(title), form)
}
