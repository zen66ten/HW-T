package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/mappu/miqt/qt6"

	"github.com/zen66ten/HW-T/internal/core"
	"github.com/zen66ten/HW-T/pkg/client"
)

// sensorTree is the HWiNFO-style Sensor Status table: one group row per
// chip/device, one child row per sensor with a kind icon and
// current/min/max/avg columns. Runs of near-identical per-core sensors
// (Core 0 Clock, Core 1 Clock, ...) collapse under one aggregate parent
// that shows their combined current/min/max/avg, expandable for detail.
type sensorTree struct {
	w      *qt6.QTreeWidget
	rows   map[string]*qt6.QTreeWidgetItem
	groups []aggGroup
}

// aggGroup is a collapsible parent row whose value columns are the
// aggregate of its member sensors, recomputed each tick.
type aggGroup struct {
	item    *qt6.QTreeWidgetItem
	kind    core.Kind
	members []string // member sensor IDs
}

// aggThreshold is how many same-family sensors it takes before they fold
// into a collapsible group instead of listing individually.
const aggThreshold = 4

// Cached brushes for value coloring (allocated once, reused every tick).
var (
	brushValue *qt6.QBrush
	brushDim   *qt6.QBrush
	brushGreen *qt6.QBrush
	brushWarn  *qt6.QBrush
	brushCrit  *qt6.QBrush
	brushHead  *qt6.QBrush
)

func initBrushes() {
	brushValue = qt6.NewQBrush3(colValue)
	brushDim = qt6.NewQBrush3(colDim)
	brushGreen = qt6.NewQBrush3(colGreen)
	brushWarn = qt6.NewQBrush3(colWarn)
	brushCrit = qt6.NewQBrush3(colCrit)
	brushHead = qt6.NewQBrush3(colHead)
}

// buildSensorWindow creates the separate Sensor Status window. It starts
// visible (HWiNFO style); the toolbar button and the window's close button
// both toggle it.
func (u *ui) buildSensorWindow() {
	initBrushes()

	win := qt6.NewQWidget2()
	win.SetWindowTitle("HW-T - Sensor Status")
	win.Resize(600, 840)

	root := vbox(4)
	root.SetContentsMargins(4, 4, 4, 4)

	t := &sensorTree{w: qt6.NewQTreeWidget2(), rows: map[string]*qt6.QTreeWidgetItem{}}
	t.w.SetHeaderLabels([]string{"Sensor", "Current", "Minimum", "Maximum", "Average"})
	t.w.SetRootIsDecorated(true)
	t.w.SetIndentation(14)
	t.w.SetAlternatingRowColors(true)
	t.w.SetIconSize(qt6.NewQSize2(16, 16))
	t.w.Header().SetStretchLastSection(false)
	t.w.Header().SetSectionResizeMode2(0, qt6.QHeaderView__Stretch)
	for c := 1; c <= 4; c++ {
		t.w.SetColumnWidth(c, 86)
	}
	u.tree = t
	root.AddWidget(t.w.QWidget)

	controls := hbox(6)
	logBtn := qt6.NewQPushButton3("Log to CSV")
	logBtn.SetCheckable(true)
	logBtn.OnToggled(func(on bool) { go u.toggleLog(on) })
	controls.AddWidget(logBtn.QWidget)

	resetBtn := qt6.NewQPushButton3("Reset Min/Max")
	resetBtn.OnClicked(func() {
		go func() {
			withClient(u.socket, func(c *client.Client) error { return c.Reset("") })
		}()
	})
	controls.AddWidget(resetBtn.QWidget)
	controls.AddStretch()
	root.AddLayout(controls.QLayout)

	win.SetLayout(root.QLayout)

	// Closing the window unchecks the toolbar toggle (which hides it).
	win.OnCloseEvent(func(super func(event *qt6.QCloseEvent), event *qt6.QCloseEvent) {
		u.sensorBtn.SetChecked(false)
		super(event)
	})

	u.sensorWin = win
	win.Show()
}

func (u *ui) toggleLog(on bool) {
	var msg string
	if on {
		home, _ := os.UserHomeDir()
		path := filepath.Join(home, fmt.Sprintf("hwt-log-%s.csv", time.Now().Format("20060102-150405")))
		err := withClient(u.socket, func(c *client.Client) error {
			_, e := c.LogStart(path, "csv", nil)
			return e
		})
		msg = "logging to " + path
		if err != nil {
			msg = "log start failed: " + err.Error()
		}
	} else {
		withClient(u.socket, func(c *client.Client) error { return c.LogStop() })
		msg = "logging stopped"
	}
	u.mu.Lock()
	u.lastErr = msg
	u.mu.Unlock()
}

func channelOf(s client.Sensor) string {
	return strings.TrimPrefix(s.ID, s.Device+":")
}

// rebuild recreates the whole table for a new sensor set: one spanned,
// teal group row per device. Within each device, families of per-core
// sensors fold into a collapsed aggregate parent; everything else lists
// directly under the device.
func (t *sensorTree) rebuild(devices []client.Device, sensors []client.Sensor) {
	t.w.Clear()
	t.rows = map[string]*qt6.QTreeWidgetItem{}
	t.groups = nil

	bold := qt6.NewQFont()
	bold.SetBold(true)

	for i := 0; i < len(sensors); {
		// Slice off the next run of sensors sharing one device.
		dev := sensors[i].Device
		j := i
		for j < len(sensors) && sensors[j].Device == dev {
			j++
		}
		block := sensors[i:j]
		i = j

		devItem := qt6.NewQTreeWidgetItem3(t.w)
		devItem.SetText(0, core.DisplayName(block[0].DeviceName))
		devItem.SetFont(0, bold)
		devItem.SetForeground(0, brushHead)
		devItem.SetFirstColumnSpanned(true)
		// PCI AER error counters are a long, rarely interesting list;
		// keep those device groups collapsed by default.
		devItem.SetExpanded(block[0].Provider != "pci")

		for _, fam := range families(block) {
			if len(fam.members) >= aggThreshold {
				parent := qt6.NewQTreeWidgetItem6(devItem)
				parent.SetText(0, fam.label)
				if ic := kindIcon(iconKind(fam.members[0])); ic != nil {
					parent.SetIcon(0, ic)
				}
				for c := 1; c <= 4; c++ {
					parent.SetTextAlignment2(c, qt6.AlignRight|qt6.AlignVCenter)
				}
				parent.SetExpanded(false) // collapsed: show the aggregate, hide the cores
				ids := make([]string, 0, len(fam.members))
				for _, s := range fam.members {
					t.addLeaf(parent, s)
					ids = append(ids, s.ID)
				}
				t.groups = append(t.groups, aggGroup{
					item: parent, kind: core.Kind(fam.members[0].Kind), members: ids,
				})
				continue
			}
			for _, s := range fam.members {
				t.addLeaf(devItem, s)
			}
		}
	}
}

// addLeaf appends one sensor row under parent and registers it for updates.
func (t *sensorTree) addLeaf(parent *qt6.QTreeWidgetItem, s client.Sensor) {
	item := qt6.NewQTreeWidgetItem6(parent)
	item.SetText(0, enriched(s))
	if ic := kindIcon(iconKind(s)); ic != nil {
		item.SetIcon(0, ic)
	}
	for c := 1; c <= 4; c++ {
		item.SetTextAlignment2(c, qt6.AlignRight|qt6.AlignVCenter)
	}
	t.rows[s.ID] = item
}

// enriched is the display label for a sensor.
func enriched(s client.Sensor) string {
	return core.EnrichLabel(s.DeviceName, channelOf(s), s.Label)
}

// iconKind is the sensor's kind for icon selection, treating fan-duty
// percentages as fans so they get the fan icon rather than the bar icon.
func iconKind(s client.Sensor) core.Kind {
	kind := core.Kind(s.Kind)
	if kind == core.KindPercent && strings.Contains(strings.ToLower(enriched(s)), "fan") {
		return core.KindFan
	}
	return kind
}

// family is a set of sensors whose labels differ only by an index number.
type family struct {
	label   string // aggregate parent name, e.g. "Core Clocks"
	members []client.Sensor
}

var digitsRE = regexp.MustCompile(`\d+`)

// families partitions a device's sensors into groups whose labels match
// once their first number is blanked out ("Core 0 Clock" and "Core 1
// Clock" share a family; "Core 0 Clock" and "Core 0 Effective Clock" do
// not). Order follows first appearance. Sensors with no number are each
// their own singleton family.
func families(block []client.Sensor) []family {
	order := []string{}
	byKey := map[string]*family{}
	for _, s := range block {
		lbl := enriched(s)
		key := familyKey(lbl)
		f, ok := byKey[key]
		if !ok {
			f = &family{label: familyLabel(lbl)}
			byKey[key] = f
			order = append(order, key)
		}
		f.members = append(f.members, s)
	}
	out := make([]family, 0, len(order))
	for _, k := range order {
		out = append(out, *byKey[k])
	}
	return out
}

// familyKey replaces the first run of digits with a placeholder so that
// only labels differing solely by that number group together.
func familyKey(label string) string {
	loc := digitsRE.FindStringIndex(label)
	if loc == nil {
		return label
	}
	return label[:loc[0]] + "#" + label[loc[1]:]
}

// familyLabel builds the aggregate parent name by dropping the index
// number and pluralizing: "Core 0 Effective Clock" -> "Core Effective
// Clocks".
func familyLabel(label string) string {
	loc := digitsRE.FindStringIndex(label)
	if loc == nil {
		return label
	}
	stripped := strings.Join(strings.Fields(label[:loc[0]]+label[loc[1]:]), " ")
	if stripped != "" && !strings.HasSuffix(stripped, "s") {
		stripped += "s"
	}
	return stripped
}

// update refreshes the value columns in place. The current value goes
// orange above the sensor's high limit and red above critical; health
// sensors render PASSED green / FAILED red. Aggregate group parents are
// recomputed from their members afterwards.
func (t *sensorTree) update(sensors []client.Sensor) {
	snap := make(map[string]client.Sensor, len(sensors))
	for _, s := range sensors {
		snap[s.ID] = s
		item, ok := t.rows[s.ID]
		if !ok {
			continue
		}
		if s.Err != "" {
			item.SetText(1, "N/A")
			item.SetForeground(1, brushDim)
			for c := 2; c <= 4; c++ {
				item.SetText(c, "")
			}
			continue
		}
		kind := core.Kind(s.Kind)
		cur := brushValue
		switch {
		case kind == core.KindHealth:
			if s.Cur != 0 {
				cur = brushGreen
			} else {
				cur = brushCrit
			}
		case overLimit(s, "crit", "lcrit", "emergency"):
			cur = brushCrit
		case overLimit(s, "max"):
			cur = brushWarn
		}
		item.SetText(1, core.FormatValue(kind, s.Cur))
		item.SetForeground(1, cur)
		vals := [3]float64{s.Min, s.Max, s.Avg}
		for i, v := range vals {
			item.SetText(i+2, core.FormatValue(kind, v))
			item.SetForeground(i+2, brushDim)
		}
	}
	for _, g := range t.groups {
		updateGroup(g, snap)
	}
}

// updateGroup fills an aggregate parent's columns: mean current and
// average across members, minimum of minimums, maximum of maximums.
func updateGroup(g aggGroup, snap map[string]client.Sensor) {
	var curSum, avgSum, minV, maxV float64
	n := 0
	for _, id := range g.members {
		s, ok := snap[id]
		if !ok || s.Err != "" || s.N == 0 {
			continue
		}
		curSum += s.Cur
		avgSum += s.Avg
		if n == 0 || s.Min < minV {
			minV = s.Min
		}
		if n == 0 || s.Max > maxV {
			maxV = s.Max
		}
		n++
	}
	if n == 0 {
		g.item.SetText(1, "N/A")
		g.item.SetForeground(1, brushDim)
		for c := 2; c <= 4; c++ {
			g.item.SetText(c, "")
		}
		return
	}
	g.item.SetText(1, core.FormatValue(g.kind, curSum/float64(n)))
	g.item.SetForeground(1, brushValue)
	g.item.SetText(2, core.FormatValue(g.kind, minV))
	g.item.SetForeground(2, brushDim)
	g.item.SetText(3, core.FormatValue(g.kind, maxV))
	g.item.SetForeground(3, brushDim)
	g.item.SetText(4, core.FormatValue(g.kind, avgSum/float64(n)))
	g.item.SetForeground(4, brushDim)
}

// overLimit reports whether the current value crossed any of the named
// limits (temperatures use max/crit; voltages may carry lcrit as well).
func overLimit(s client.Sensor, names ...string) bool {
	for _, n := range names {
		if lim, ok := s.Limits[n]; ok && lim > 0 && s.Cur >= lim {
			return true
		}
	}
	return false
}
