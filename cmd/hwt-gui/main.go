// hwt-gui is the native desktop client (SPEC §8.2), an HWiNFO-style
// recreation built with Qt 6 via the miqt bindings. The main window is the
// System Summary: CPU column on the left, motherboard and memory in the
// middle, GPU on the right, each with a vendor badge. A toolbar button
// toggles the separate Sensor Status window (every sensor with
// current/min/max/avg and a per-kind icon). A plain client of the hwtd UDS
// API; the only binary needing CGO.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/mappu/miqt/qt6"

	"github.com/zen66ten/HW-T/pkg/client"
)

func main() {
	socket := flag.String("socket", client.DefaultSocket(), "hwtd unix socket")
	flag.Parse()

	qapp := qt6.NewQApplication(os.Args)
	qapp.SetStyleSheet(appStyle)

	u := &ui{socket: *socket}
	u.buildMainWindow()
	u.buildSensorWindow()

	// Data flows in on background goroutines and is only stored; all Qt
	// work happens on the main thread, driven by a 1 Hz repaint timer.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go u.runSubscription(ctx)
	go u.runDeviceFetch(ctx)

	tick := qt6.NewQTimer()
	tick.OnTimeout(u.refresh)
	tick.Start(1000)

	// Optional headless screenshot: HWT_SHOT=/path/prefix (works with
	// QT_QPA_PLATFORM=offscreen). Grabs both windows then quits.
	if shot := os.Getenv("HWT_SHOT"); shot != "" {
		t := qt6.NewQTimer()
		t.SetSingleShot(true)
		t.OnTimeout(func() {
			u.refresh()
			u.win.Grab().Save2(shot+"-summary.png", "PNG")
			u.sensorWin.Resize(600, 2600) // capture the full sensor list
			u.sensorWin.Grab().Save2(shot+"-sensors.png", "PNG")
			qt6.QCoreApplication_Quit()
		})
		t.Start(4000)
	}

	u.win.Show()
	u.win.Resize(1560, u.win.SizeHint().Height())
	qt6.QApplication_Exec()
}

// ui owns both windows and the shared data snapshot. The mutex guards
// everything below it; background goroutines write, the Qt main thread
// reads inside refresh().
type ui struct {
	socket string

	win         *qt6.QFrame
	summarySlot *qt6.QVBoxLayout // holds the three-column summary, rebuilt on device changes
	summaryBody *qt6.QWidget
	sensorBtn   *qt6.QPushButton
	status      *qt6.QLabel

	sensorWin *qt6.QWidget
	tree      *sensorTree

	live []liveLabel // summary labels refreshed every tick

	mu        sync.Mutex
	devices   []client.Device
	sensors   []client.Sensor
	connected bool
	lastErr   string

	builtFor string // device+sensor signature the summary was last built for
	gpuIndex int    // selected GPU in the summary combo
	memIndex int    // selected memory module in the summary combo
	rebuild  bool   // set when gpuIndex/memIndex change
}

// liveLabel is one summary value that re-renders from each snapshot. A
// non-empty returned style replaces the label's stylesheet (used for
// green/red health marks).
type liveLabel struct {
	l      *qt6.QLabel
	render func(snap map[string]client.Sensor) (text, style string)
}

// --- main window ---

func (u *ui) buildMainWindow() {
	// Frameless top-level window with a thin teal outline, like HWiNFO.
	win := qt6.NewQFrame3(nil, qt6.FramelessWindowHint)
	win.SetStyleSheet("background-color:#000000; border:1px solid #2f6d6d;")
	win.SetWindowTitle("HW-T - System Summary")
	u.win = win

	outer := vbox(0)
	outer.SetContentsMargins(1, 1, 1, 1)
	outer.AddWidget(u.buildTitleBar())
	outer.AddWidget(u.buildToolbar())

	u.summarySlot = vbox(0)
	u.summarySlot.SetContentsMargins(6, 4, 6, 6)
	u.summaryBody = lbl("connecting to hwtd…", statusQSS, qt6.AlignCenter).QWidget
	u.summaryBody.SetMinimumHeight(300)
	u.summarySlot.AddWidget(u.summaryBody)
	outer.AddLayout(u.summarySlot.QLayout)

	win.SetLayout(outer.QLayout)
}

// buildTitleBar creates the custom title bar (title text, minimize and
// close buttons) and wires window dragging, since the window is frameless.
func (u *ui) buildTitleBar() *qt6.QWidget {
	bar := qt6.NewQWidget2()
	bar.SetFixedHeight(26)
	bar.SetStyleSheet("background-color:#000000; border:none;")

	tb := hbox(0)
	tb.SetContentsMargins(8, 0, 0, 0)
	title := lbl("HW-T  - System Summary",
		"color:#f2f2f2; font-weight:bold; background:transparent; border:none;",
		qt6.AlignLeft|qt6.AlignVCenter)
	tb.AddWidget(title.QWidget)
	tb.AddStretch()

	minBtn := qt6.NewQPushButton3("–")
	minBtn.SetFixedSize2(36, 26)
	minBtn.SetStyleSheet(
		"QPushButton{background-color:#13201f; color:#ffffff; border:none; font-size:13px; padding:0;}" +
			"QPushButton:hover{background-color:#1f4d4d;}")
	minBtn.OnClicked(func() { u.win.ShowMinimized() })
	tb.AddWidget(minBtn.QWidget)

	closeBtn := qt6.NewQPushButton3("✕")
	closeBtn.SetFixedSize2(36, 26)
	closeBtn.SetStyleSheet(
		"QPushButton{background-color:#c0392b; color:#ffffff; border:none; font-size:13px; padding:0;}" +
			"QPushButton:hover{background-color:#e04b3a;}")
	closeBtn.OnClicked(func() { qt6.QCoreApplication_Quit() })
	tb.AddWidget(closeBtn.QWidget)
	bar.SetLayout(tb.QLayout)

	// Drag-to-move for the frameless window. StartSystemMove() hands the
	// drag to the compositor's own move grab; a manual Move() per mouse
	// event is a no-op (or worse, a stall) under native Wayland, which
	// doesn't let clients reposition top-level windows directly.
	bar.OnMousePressEvent(func(super func(event *qt6.QMouseEvent), event *qt6.QMouseEvent) {
		if event.Button() == qt6.LeftButton {
			u.win.WindowHandle().StartSystemMove()
		}
	})
	return bar
}

// buildToolbar is the row under the title bar: the Sensors window toggle,
// Save Report, and the connection status on the right.
func (u *ui) buildToolbar() *qt6.QWidget {
	bar := qt6.NewQWidget2()
	bar.SetStyleSheet("background:transparent; border:none;")
	row := hbox(6)
	row.SetContentsMargins(6, 4, 6, 2)

	u.sensorBtn = qt6.NewQPushButton3("Sensors")
	u.sensorBtn.SetCheckable(true)
	u.sensorBtn.SetChecked(true)
	u.sensorBtn.OnToggled(func(on bool) {
		if on {
			u.sensorWin.Show()
			u.sensorWin.Raise()
		} else {
			u.sensorWin.Hide()
		}
	})
	row.AddWidget(u.sensorBtn.QWidget)

	report := qt6.NewQPushButton3("Save Report")
	report.OnClicked(u.saveReport)
	row.AddWidget(report.QWidget)

	row.AddStretch()
	u.status = lbl("connecting…", statusQSS, qt6.AlignRight|qt6.AlignVCenter)
	row.AddWidget(u.status.QWidget)

	bar.SetLayout(row.QLayout)
	return bar
}

func (u *ui) saveReport() {
	go func() {
		var report string
		err := withClient(u.socket, func(c *client.Client) error {
			var e error
			report, e = c.Report("html", nil, false)
			return e
		})
		msg := ""
		if err != nil {
			msg = "report failed: " + err.Error()
		} else {
			home, _ := os.UserHomeDir()
			path := filepath.Join(home, "hwt-report.html")
			if werr := os.WriteFile(path, []byte(report), 0o644); werr != nil {
				msg = "report failed: " + werr.Error()
			} else {
				msg = "report saved to " + path
			}
		}
		u.mu.Lock()
		u.lastErr = msg
		u.mu.Unlock()
	}()
}

// --- data plumbing (background goroutines; no Qt calls here) ---

func (u *ui) runSubscription(ctx context.Context) {
	for ctx.Err() == nil {
		err := client.Subscribe(ctx, u.socket, time.Second, func(sensors []client.Sensor) {
			u.mu.Lock()
			u.sensors = sensors
			u.connected = true
			u.mu.Unlock()
		})
		if ctx.Err() != nil {
			return
		}
		u.mu.Lock()
		u.connected = false
		if err != nil {
			u.lastErr = err.Error()
		}
		u.mu.Unlock()
		select {
		case <-ctx.Done():
			return
		case <-time.After(3 * time.Second):
		}
	}
}

func (u *ui) runDeviceFetch(ctx context.Context) {
	for ctx.Err() == nil {
		var devs []client.Device
		err := withClient(u.socket, func(c *client.Client) error {
			var e error
			devs, e = c.Devices()
			return e
		})
		if err == nil && len(devs) > 0 {
			u.mu.Lock()
			u.devices = devs
			u.mu.Unlock()
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(3 * time.Second):
		}
	}
}

func withClient(socket string, fn func(*client.Client) error) error {
	c, err := client.Dial(socket)
	if err != nil {
		return err
	}
	defer c.Close()
	return fn(c)
}

// --- per-tick refresh (Qt main thread) ---

// signature captures the device/sensor shape the summary and sensor tree
// were built for; when it changes (daemon restart, hotplug) both rebuild.
func signature(devices []client.Device, sensors []client.Sensor) string {
	var b []byte
	for _, d := range devices {
		b = append(b, d.ID...)
		b = append(b, '\n')
	}
	for _, s := range sensors {
		b = append(b, s.ID...)
		b = append(b, '\n')
	}
	return string(b)
}

func (u *ui) refresh() {
	u.mu.Lock()
	devices := u.devices
	sensors := u.sensors
	connected := u.connected
	lastErr := u.lastErr
	rebuild := u.rebuild
	u.rebuild = false
	u.mu.Unlock()

	sig := signature(devices, sensors)
	if sig != u.builtFor || rebuild {
		u.builtFor = sig
		u.rebuildSummary(devices, sensors)
		u.tree.rebuild(devices, sensors)
	}

	snap := make(map[string]client.Sensor, len(sensors))
	for _, s := range sensors {
		snap[s.ID] = s
	}
	for _, ll := range u.live {
		text, style := ll.render(snap)
		ll.l.SetText(text)
		if style != "" {
			ll.l.SetStyleSheet(style)
		}
	}
	u.tree.update(sensors)

	switch {
	case !connected && lastErr != "":
		u.status.SetText(lastErr)
	case !connected:
		u.status.SetText("disconnected, retrying…")
	case lastErr != "":
		u.status.SetText(fmt.Sprintf("%s   |   %d sensors", lastErr, len(sensors)))
	default:
		u.status.SetText(fmt.Sprintf("%d sensors, connected", len(sensors)))
	}
}
