// hwt-gui is the native desktop client (SPEC §8.2), modelled on HWiNFO's
// multi-window layout. A main window carries the menu, toolbar and a
// hardware navigation tree. Two independent child windows open from the
// toolbar: System Summary (component frames at a glance) and Sensor Status
// (every sensor with current/min/max/avg). Either child window can be
// closed on its own while the main window stays open. A plain client of
// the hwtd UDS API; the only binary needing CGO.
package main

import (
	"context"
	"flag"
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
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

	u := newUI(a, *socket)
	u.main = a.NewWindow("HW-T")
	u.main.Resize(fyne.NewSize(680, 560))
	u.main.SetContent(u.buildMain())
	u.main.SetMainMenu(u.mainMenu())

	u.main.SetOnClosed(func() { u.stopSub() })
	a.Lifecycle().SetOnStarted(func() {
		go u.startSub(time.Second)
		go u.fetchDevices()
		// Open both child windows by default, HWiNFO style.
		fyne.Do(func() {
			u.openSummary()
			u.openSensors()
			u.main.RequestFocus()
		})
	})

	u.main.ShowAndRun()
}

// sensorRow is one line in the Sensor Status table.
type sensorRow struct {
	name, cur, min, max, avg *canvas.Text
	kind                     core.Kind
}

// summaryRow is one live line in a System Summary frame.
type summaryRow struct {
	box    *valueBox
	render func(snap map[string]client.Sensor) string
}

type ui struct {
	app    fyne.App
	main   fyne.Window
	socket string

	mu        sync.Mutex
	signature string
	devices   []client.Device
	sensors   []client.Sensor
	firing    map[string]bool
	selected  string

	// main-window navigation
	tree     *widget.Tree
	detail   *fyne.Container
	mainStat *widget.Label

	// System Summary child window
	summaryWin  fyne.Window
	summaryBody *fyne.Container
	summaryRows []summaryRow

	// Sensor Status child window
	sensorWin  fyne.Window
	sensorBody *fyne.Container
	rows       map[string]*sensorRow
	chart      *chart

	subMu     sync.Mutex
	subCancel context.CancelFunc
	subWG     sync.WaitGroup

	rpcMu sync.Mutex
	rpc   *client.Client
}

func newUI(a fyne.App, socket string) *ui {
	return &ui{
		app: a, socket: socket,
		firing: map[string]bool{},
		rows:   map[string]*sensorRow{},
	}
}

// --- main window ---

func (u *ui) buildMain() fyne.CanvasObject {
	u.mainStat = widget.NewLabel("connecting…")

	summaryBtn := widget.NewButtonWithIcon("Summary", theme.ListIcon(), u.openSummary)
	sensorsBtn := widget.NewButtonWithIcon("Sensors", theme.ComputerIcon(), u.openSensors)
	reportBtn := widget.NewButtonWithIcon("Save Report", theme.DocumentSaveIcon(), u.saveReport)
	toolbar := container.NewHBox(summaryBtn, sensorsBtn, reportBtn)

	u.tree = widget.NewTree(u.treeChildren, u.treeIsBranch, u.treeCreate, u.treeUpdate)
	u.tree.OnSelected = u.treeSelected
	u.detail = container.NewVBox(widget.NewLabel("select a device on the left"))

	split := container.NewHSplit(u.tree, container.NewVScroll(u.detail))
	split.SetOffset(0.42)

	return container.NewBorder(
		container.NewVBox(toolbar, widget.NewSeparator()),
		container.NewVBox(widget.NewSeparator(), u.mainStat),
		nil, nil, split)
}

func (u *ui) mainMenu() *fyne.MainMenu {
	return fyne.NewMainMenu(
		fyne.NewMenu("Program",
			fyne.NewMenuItem("Refresh", func() { go u.fetchDevices() }),
			fyne.NewMenuItem("Quit", func() { u.app.Quit() }),
		),
		fyne.NewMenu("Monitoring",
			fyne.NewMenuItem("System Summary", u.openSummary),
			fyne.NewMenuItem("Sensor Status", u.openSensors),
		),
		fyne.NewMenu("Report",
			fyne.NewMenuItem("Save Report…", u.saveReport),
		),
		fyne.NewMenu("View",
			fyne.NewMenuItem("Toggle Light/Dark", u.toggleTheme),
		),
	)
}

func (u *ui) toggleTheme() {
	if pal == &palLight {
		pal = &palDark
	} else {
		pal = &palLight
	}
	u.app.Settings().SetTheme(&hwtTheme{p: pal})
	u.mu.Lock()
	u.signature = "" // force every open window to rebuild next tick
	u.mu.Unlock()
}

// --- System Summary child window ---

func (u *ui) openSummary() {
	if u.summaryWin != nil {
		u.summaryWin.RequestFocus()
		return
	}
	w := u.app.NewWindow("System Summary")
	w.Resize(fyne.NewSize(1180, 560))
	u.summaryBody = container.NewStack(widget.NewLabel("waiting for data…"))
	w.SetContent(u.summaryBody)
	w.SetOnClosed(func() {
		u.mu.Lock()
		u.summaryWin = nil
		u.summaryBody = nil
		u.summaryRows = nil
		u.mu.Unlock()
	})
	u.summaryWin = w

	u.mu.Lock()
	sensors := u.sensors
	u.mu.Unlock()
	u.rebuildSummary(sensors)
	w.Show()
}

// --- Sensor Status child window ---

func (u *ui) openSensors() {
	if u.sensorWin != nil {
		u.sensorWin.RequestFocus()
		return
	}
	w := u.app.NewWindow("Sensor Status")
	w.Resize(fyne.NewSize(620, 700))
	u.chart = newChart()

	logCheck := widget.NewCheck("Log to file", func(on bool) { go u.toggleLog(on) })
	rate := widget.NewSelect([]string{"0.5 s", "1.0 s", "2.0 s", "5.0 s"}, func(s string) {
		d := map[string]time.Duration{
			"0.5 s": 500 * time.Millisecond, "1.0 s": time.Second,
			"2.0 s": 2 * time.Second, "5.0 s": 5 * time.Second,
		}[s]
		go u.startSub(d)
	})
	rate.SetSelectedIndex(1)
	resetBtn := widget.NewButton("Reset", func() {
		go u.withRPC(func(c *client.Client) error { return c.Reset("") })
	})
	controls := container.NewHBox(logCheck, widget.NewLabel("  rate"), rate, resetBtn)

	u.sensorBody = container.NewStack(widget.NewLabel("waiting for data…"))
	content := container.NewBorder(nil,
		container.NewVBox(widget.NewSeparator(), u.chart, controls),
		nil, nil, u.sensorBody)
	w.SetContent(content)
	w.SetOnClosed(func() {
		u.mu.Lock()
		u.sensorWin = nil
		u.sensorBody = nil
		u.rows = map[string]*sensorRow{}
		u.chart = nil
		u.mu.Unlock()
	})
	u.sensorWin = w

	u.mu.Lock()
	sensors := u.sensors
	u.mu.Unlock()
	u.rebuildSensors(sensors)
	w.Show()
}

func (u *ui) saveReport() {
	var report string
	err := u.withRPC(func(c *client.Client) error {
		var e error
		report, e = c.Report("html", nil, false)
		return e
	})
	if err != nil {
		u.mainStat.SetText("report failed: " + err.Error())
		return
	}
	home, _ := os.UserHomeDir()
	path := filepath.Join(home, "hwt-report.html")
	if err := os.WriteFile(path, []byte(report), 0o644); err != nil {
		u.mainStat.SetText("report failed: " + err.Error())
		return
	}
	u.mainStat.SetText("report saved to " + path)
}

// --- subscription ---

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
			fyne.Do(func() { u.mainStat.SetText(msg) })
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

func snapshotSignature(sensors []client.Sensor) string {
	var b []byte
	for _, s := range sensors {
		b = append(b, s.ID...)
		b = append(b, '\n')
	}
	return string(b)
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

	u.mu.Lock()
	devicesReady := len(u.devices) > 0
	u.mu.Unlock()
	if !devicesReady {
		u.fetchDevices()
	}

	sig := snapshotSignature(sensors)
	snap := make(map[string]client.Sensor, len(sensors))
	for _, s := range sensors {
		snap[s.ID] = s
	}

	fyne.Do(func() {
		u.mu.Lock()
		rebuild := sig != u.signature
		u.signature = sig
		u.firing = firing
		u.sensors = sensors
		summaryOpen := u.summaryWin != nil
		sensorOpen := u.sensorWin != nil
		u.mu.Unlock()

		if rebuild {
			if summaryOpen {
				u.rebuildSummary(sensors)
			}
			if sensorOpen {
				u.rebuildSensors(sensors)
			}
			if u.tree != nil {
				u.tree.Refresh()
			}
		}
		if sensorOpen {
			for _, s := range sensors {
				if row, ok := u.rows[s.ID]; ok {
					u.updateRow(row, s, firing[s.ID])
				}
			}
		}
		if summaryOpen {
			for _, sr := range u.summaryRows {
				sr.box.Set(sr.render(snap), nil)
			}
		}

		txt := fmt.Sprintf("%d sensors, connected", len(sensors))
		if len(firing) > 0 {
			txt = fmt.Sprintf("⚠ %d alert(s)   %s", len(firing), txt)
		}
		u.mainStat.SetText(txt)
	})
	u.refreshChart()
}

func (u *ui) updateRow(row *sensorRow, s client.Sensor, firing bool) {
	set := func(t *canvas.Text, txt string, col color.Color) {
		t.Text, t.Color = txt, col
		t.Refresh()
	}
	if s.Err != "" {
		set(row.cur, "N/A", pal.dim)
		set(row.min, "", pal.dim)
		set(row.max, "", pal.dim)
		set(row.avg, "", pal.dim)
		return
	}
	curCol := color.Color(pal.foreground)
	if firing {
		curCol = pal.crit
	} else if row.kind == core.KindTemp {
		if crit, ok := s.Limits["crit"]; ok && s.Cur >= crit {
			curCol = pal.crit
		} else if high, ok := s.Limits["max"]; ok && s.Cur >= high {
			curCol = pal.warn
		}
	}
	set(row.cur, core.FormatValue(row.kind, s.Cur), curCol)
	set(row.min, core.FormatValue(row.kind, s.Min), pal.dim)
	set(row.max, core.FormatValue(row.kind, s.Max), pal.dim)
	set(row.avg, core.FormatValue(row.kind, s.Avg), pal.dim)
}

func (u *ui) refreshChart() {
	u.mu.Lock()
	id, ch := u.selected, u.chart
	row := u.rows[id]
	u.mu.Unlock()
	if id == "" || row == nil || ch == nil {
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
	label, kind := row.name.Text, row.kind
	fyne.Do(func() { ch.SetSeries(label, kind, pts) })
}

func (u *ui) fetchDevices() {
	var devs []client.Device
	if err := u.withRPC(func(c *client.Client) error {
		var err error
		devs, err = c.Devices()
		return err
	}); err != nil {
		return
	}
	u.mu.Lock()
	u.devices = devs
	u.signature = ""
	u.mu.Unlock()
}

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
	fyne.Do(func() { u.mainStat.SetText(msg) })
}
