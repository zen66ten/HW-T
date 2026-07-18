package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mappu/miqt/qt6"

	"github.com/zen66ten/HW-T/internal/core"
	"github.com/zen66ten/HW-T/pkg/client"
)

// sensorTree is the HWiNFO-style Sensor Status table: one group row per
// chip/device, one child row per sensor with a kind icon and
// current/min/max/avg columns.
type sensorTree struct {
	w    *qt6.QTreeWidget
	rows map[string]*qt6.QTreeWidgetItem
}

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
// teal group row per device, one child row per sensor with its kind icon
// and descriptive name.
func (t *sensorTree) rebuild(devices []client.Device, sensors []client.Sensor) {
	t.w.Clear()
	t.rows = map[string]*qt6.QTreeWidgetItem{}

	bold := qt6.NewQFont()
	bold.SetBold(true)

	var group *qt6.QTreeWidgetItem
	lastDev := ""
	for _, s := range sensors {
		if group == nil || s.Device != lastDev {
			lastDev = s.Device
			group = qt6.NewQTreeWidgetItem3(t.w)
			group.SetText(0, core.DisplayName(s.DeviceName))
			group.SetFont(0, bold)
			group.SetForeground(0, brushHead)
			group.SetFirstColumnSpanned(true)
			// PCI AER error counters are a long, rarely interesting
			// list; keep those groups collapsed by default.
			group.SetExpanded(s.Provider != "pci")
		}
		item := qt6.NewQTreeWidgetItem6(group)
		label := core.EnrichLabel(s.DeviceName, channelOf(s), s.Label)
		item.SetText(0, label)
		kind := core.Kind(s.Kind)
		if kind == core.KindPercent && strings.Contains(strings.ToLower(label), "fan") {
			kind = core.KindFan // fan duty reported as %, still a fan
		}
		if ic := kindIcon(kind); ic != nil {
			item.SetIcon(0, ic)
		}
		for c := 1; c <= 4; c++ {
			item.SetTextAlignment2(c, qt6.AlignRight|qt6.AlignVCenter)
		}
		t.rows[s.ID] = item
	}
}

// update refreshes the value columns in place. The current value goes
// orange above the sensor's high limit and red above critical; health
// sensors render PASSED green / FAILED red.
func (t *sensorTree) update(sensors []client.Sensor) {
	for _, s := range sensors {
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
