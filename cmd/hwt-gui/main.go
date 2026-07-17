// hwt-gui is the native desktop client (SPEC §8.2), styled after GPU-T:
// dense sensor rows with rolling bar graphs, value boxes, and an inventory
// browser. A plain client of the hwtd UDS API; the only binary needing CGO.
package main

import (
	"context"
	"flag"
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/zen66ten/HW-T/internal/core"
	"github.com/zen66ten/HW-T/pkg/client"
)

func main() {
	socket := flag.String("socket", client.DefaultSocket(), "hwtd unix socket")
	flag.Parse()

	a := app.NewWithID("com.github.zen66ten.hwt")
	a.Settings().SetTheme(&hwtTheme{p: pal})
	w := a.NewWindow("HW-T")
	w.Resize(fyne.NewSize(760, 780))

	u := newUI(w, a, *socket)
	w.SetContent(u.build())
	// UI work must not start before the event loop does (fyne.Do contract).
	a.Lifecycle().SetOnStarted(func() {
		go u.startSub(time.Second)
		go u.fetchDevices()
	})

	w.ShowAndRun()
	u.stopSub()
}

// sensorRow is one live row: name, value box, rolling bars.
type sensorRow struct {
	name  *canvas.Text
	value *valueBox
	bar   *histBar
	kind  core.Kind
}

type ui struct {
	win    fyne.Window
	app    fyne.App
	socket string

	mu        sync.Mutex
	signature string // sensor-ID set fingerprint; change triggers rebuild
	rows      map[string]*sensorRow
	devices   []client.Device
	selected  string // sensor for the big chart
	firing    map[string]bool

	list      *container.Scroll
	chart     *chart
	status    *widget.Label
	invSelect *widget.Select
	invForm   *fyne.Container
	invByName map[string]int // dropdown label -> devices index

	subMu     sync.Mutex // serializes startSub/stopSub (rate changes)
	subCancel context.CancelFunc
	subWG     sync.WaitGroup

	rpcMu sync.Mutex
	rpc   *client.Client
}

func newUI(win fyne.Window, a fyne.App, socket string) *ui {
	return &ui{
		win: win, app: a, socket: socket,
		rows:   map[string]*sensorRow{},
		firing: map[string]bool{},
	}
}

func (u *ui) build() fyne.CanvasObject {
	u.status = widget.NewLabel("connecting…")
	u.chart = newChart()
	u.list = container.NewVScroll(container.NewVBox())

	sensorsTab := container.NewBorder(nil,
		container.NewVBox(widget.NewSeparator(), u.chart), nil, nil, u.list)

	u.invForm = container.New(layout.NewFormLayout())
	u.invSelect = widget.NewSelect(nil, u.showDevice)
	u.invSelect.PlaceHolder = "select device"
	refreshBtn := widget.NewButtonWithIcon("", theme.ViewRefreshIcon(), func() { go u.fetchDevices() })
	inventoryTab := container.NewBorder(
		container.NewBorder(nil, nil, nil, refreshBtn, u.invSelect),
		nil, nil, nil,
		container.NewVScroll(u.invForm))

	tabs := container.NewAppTabs(
		container.NewTabItem("Sensors", sensorsTab),
		container.NewTabItem("Inventory", inventoryTab),
	)

	logCheck := widget.NewCheck("Log to file", func(on bool) { go u.toggleLog(on) })
	rate := widget.NewSelect([]string{"0.5 s", "1.0 s", "2.0 s", "5.0 s"}, func(s string) {
		d := map[string]time.Duration{
			"0.5 s": 500 * time.Millisecond,
			"1.0 s": time.Second,
			"2.0 s": 2 * time.Second,
			"5.0 s": 5 * time.Second,
		}[s]
		// Off the UI thread: stopSub waits for the subscription goroutine,
		// which may itself be waiting on fyne.Do — blocking here would
		// deadlock the event loop.
		go u.startSub(d)
	})
	rate.SetSelectedIndex(1)
	resetBtn := widget.NewButton("Reset", func() {
		go u.withRPC(func(c *client.Client) error { return c.Reset("") })
	})
	themeBtn := widget.NewButtonWithIcon("", theme.ColorPaletteIcon(), nil)
	themeBtn.OnTapped = func() {
		if pal == &palLight {
			pal = &palDark
		} else {
			pal = &palLight
		}
		u.app.Settings().SetTheme(&hwtTheme{p: pal})
		u.mu.Lock()
		u.signature = "" // force row rebuild with the new palette next tick
		u.mu.Unlock()
		if sel := u.invSelect.Selected; sel != "" {
			u.showDevice(sel)
		}
	}

	bottom := container.NewBorder(widget.NewSeparator(), nil,
		container.NewHBox(logCheck, widget.NewLabel("  rate"), rate, resetBtn, themeBtn),
		nil, container.NewHBox(layout.NewSpacer(), u.status))

	return container.NewBorder(nil, bottom, nil, nil, tabs)
}

// --- subscription management (restartable for the rate selector) ---

func (u *ui) startSub(interval time.Duration) {
	u.subMu.Lock()
	defer u.subMu.Unlock()
	u.stopSubLocked()
	ctx, cancel := context.WithCancel(context.Background())
	u.subCancel = cancel
	u.subWG.Add(1)
	go func() {
		defer u.subWG.Done()
		for ctx.Err() == nil {
			err := client.Subscribe(ctx, u.socket, interval, u.onSnapshot)
			if ctx.Err() != nil {
				return
			}
			msg := "disconnected, retrying"
			if err != nil {
				msg = fmt.Sprintf("disconnected (%v), retrying", err)
			}
			fyne.Do(func() { u.status.SetText(msg) })
			select {
			case <-ctx.Done():
				return
			case <-time.After(3 * time.Second):
			}
		}
	}()
}

func (u *ui) stopSub() {
	u.subMu.Lock()
	defer u.subMu.Unlock()
	u.stopSubLocked()
}

func (u *ui) stopSubLocked() {
	if u.subCancel != nil {
		u.subCancel()
		u.subWG.Wait()
		u.subCancel = nil
	}
}

func (u *ui) withRPC(fn func(*client.Client) error) error {
	u.rpcMu.Lock()
	defer u.rpcMu.Unlock()
	for attempt := 0; attempt < 2; attempt++ {
		if u.rpc == nil {
			c, err := client.Dial(u.socket)
			if err != nil {
				return err
			}
			u.rpc = c
		}
		if err := fn(u.rpc); err != nil {
			u.rpc.Close()
			u.rpc = nil
			continue
		}
		return nil
	}
	return fmt.Errorf("rpc failed after reconnect")
}

// --- sensors view ---

func snapshotSignature(sensors []client.Sensor) string {
	var b strings.Builder
	for _, s := range sensors {
		b.WriteString(s.ID)
		b.WriteByte('\n')
	}
	return b.String()
}

func (u *ui) onSnapshot(sensors []client.Sensor) {
	var alerts []client.AlertStatus
	u.withRPC(func(c *client.Client) error {
		var err error
		alerts, err = c.Alerts()
		return err
	})
	firing := map[string]bool{}
	for _, a := range alerts {
		if a.State == "firing" {
			firing[a.Sensor] = true
		}
	}

	sig := snapshotSignature(sensors)
	fyne.Do(func() {
		u.mu.Lock()
		rebuild := sig != u.signature
		u.signature = sig
		u.firing = firing
		u.mu.Unlock()

		if rebuild {
			u.rebuildRows(sensors)
		}
		for _, s := range sensors {
			row, ok := u.rows[s.ID]
			if !ok {
				continue
			}
			u.updateRow(row, s, firing[s.ID])
		}
		nAlerts := len(firing)
		txt := fmt.Sprintf("%d sensors, connected", len(sensors))
		if nAlerts > 0 {
			txt = fmt.Sprintf("⚠ %d alert(s)   %s", nAlerts, txt)
		}
		u.status.SetText(txt)
	})
	u.refreshChart()
}

func (u *ui) rebuildRows(sensors []client.Sensor) {
	box := container.NewVBox()
	u.rows = map[string]*sensorRow{}
	lastDev := ""
	for _, s := range sensors {
		if s.Device != lastDev {
			lastDev = s.Device
			hdr := canvas.NewText(core.DisplayName(s.DeviceName), pal.accent)
			hdr.TextStyle.Bold = true
			hdr.TextSize = 13
			box.Add(container.NewPadded(hdr))
		}
		name := canvas.NewText(s.Label, pal.dim)
		name.TextSize = 12
		row := &sensorRow{name: name, value: newValueBox(), bar: newHistBar(), kind: core.Kind(s.Kind)}
		u.rows[s.ID] = row

		id := s.ID
		content := container.New(rowLayout{}, name, row.value, row.bar)
		box.Add(newTappableRow(content, func() {
			u.mu.Lock()
			u.selected = id
			u.mu.Unlock()
			go u.refreshChart()
		}))
	}
	u.list.Content = box
	u.list.Refresh()
}

func (u *ui) updateRow(row *sensorRow, s client.Sensor, firing bool) {
	if s.Err != "" {
		row.value.Set("N/A", pal.dim)
		return
	}
	var col color.Color // nil = normal foreground
	switch {
	case firing:
		col = pal.crit
	case row.kind == core.KindTemp:
		if crit, ok := s.Limits["crit"]; ok && s.Cur >= crit {
			col = pal.crit
		} else if high, ok := s.Limits["max"]; ok && s.Cur >= high {
			col = pal.warn
		}
	}
	row.value.Set(core.FormatValue(row.kind, s.Cur), col)
	row.bar.Push(s.Cur)

	nameCol := color.Color(pal.dim)
	if firing {
		nameCol = pal.crit
	}
	if row.name.Color != nameCol {
		row.name.Color = nameCol
		row.name.Refresh()
	}
}

func (u *ui) refreshChart() {
	u.mu.Lock()
	id := u.selected
	row := u.rows[id]
	u.mu.Unlock()
	if id == "" || row == nil {
		return
	}
	var pts []client.Point
	if err := u.withRPC(func(c *client.Client) error {
		var err error
		pts, err = c.History(id)
		return err
	}); err != nil {
		return
	}
	label := row.name.Text
	kind := row.kind
	fyne.Do(func() { u.chart.SetSeries(label, kind, pts) })
}

// --- inventory view ---

func (u *ui) fetchDevices() {
	var devs []client.Device
	if err := u.withRPC(func(c *client.Client) error {
		var err error
		devs, err = c.Devices()
		return err
	}); err != nil {
		return
	}

	byName := map[string]int{}
	names := make([]string, 0, len(devs))
	for i, d := range devs {
		label := fmt.Sprintf("%s  [%s]", core.DisplayName(d.Name), d.Provider)
		for dup := 2; ; dup++ {
			if _, taken := byName[label]; !taken {
				break
			}
			label = fmt.Sprintf("%s  [%s #%d]", core.DisplayName(d.Name), d.Provider, dup)
		}
		byName[label] = i
		names = append(names, label)
	}

	fyne.Do(func() {
		u.mu.Lock()
		u.devices = devs
		u.invByName = byName
		u.mu.Unlock()
		sel := u.invSelect.Selected
		u.invSelect.SetOptions(names)
		if sel == "" && len(names) > 0 {
			u.invSelect.SetSelected(names[0])
		}
	})
}

func (u *ui) showDevice(label string) {
	u.mu.Lock()
	idx, ok := u.invByName[label]
	var dev client.Device
	if ok && idx < len(u.devices) {
		dev = u.devices[idx]
	}
	u.mu.Unlock()
	if !ok {
		return
	}

	u.invForm.RemoveAll()
	add := func(k, v string) {
		lbl := canvas.NewText(k, pal.dim)
		lbl.TextSize = 12
		lbl.Alignment = fyne.TextAlignTrailing
		vb := newValueBox()
		vb.Set(v, nil)
		u.invForm.Add(lbl)
		u.invForm.Add(vb)
	}
	add("device id", dev.ID)
	keys := make([]string, 0, len(dev.Attrs))
	for k := range dev.Attrs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		add(k, dev.Attrs[k])
	}
	u.invForm.Refresh()
}

// --- logging control ---

func (u *ui) toggleLog(on bool) {
	if !on {
		u.withRPC(func(c *client.Client) error { return c.LogStop() })
		return
	}
	home, _ := os.UserHomeDir()
	path := filepath.Join(home, fmt.Sprintf("hwt-log-%s.csv", time.Now().Format("20060102-150405")))
	err := u.withRPC(func(c *client.Client) error {
		_, err := c.LogStart(path, "csv", nil)
		return err
	})
	msg := "logging to " + path
	if err != nil {
		msg = "log start failed: " + err.Error()
	}
	fyne.Do(func() { u.status.SetText(msg) })
}
