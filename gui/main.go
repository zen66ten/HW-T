// Command hwinfo-gui recreates the HWiNFO64 "System Summary" window using Go
// and Qt6 (via the miqt bindings). All values are static and taken from the
// reference screenshot; this is a UI recreation, not a real hardware monitor.
package main

import (
	"os"

	"github.com/mappu/miqt/qt6"
)

func main() {
	app := qt6.NewQApplication(os.Args)
	app.SetStyleSheet(appStyle)

	// Frameless top-level window with a thin teal outline, like HWiNFO.
	win := qt6.NewQFrame3(nil, qt6.FramelessWindowHint)
	win.SetStyleSheet("background-color:#000000; border:1px solid #2f6d6d;")
	win.SetWindowTitle("HWiNFO64 - System Summary")

	outer := vbox(0)
	outer.SetContentsMargins(1, 1, 1, 1)
	outer.AddWidget(buildTitleBar(win))
	outer.AddWidget(buildContent())
	win.SetLayout(outer.QLayout)

	// Optional headless screenshot: HWINFO_SHOT=/path.png (works with
	// QT_QPA_PLATFORM=offscreen). Grabs the window then quits.
	if shot := os.Getenv("HWINFO_SHOT"); shot != "" {
		t := qt6.NewQTimer()
		t.SetSingleShot(true)
		t.OnTimeout(func() {
			win.Grab().Save2(shot, "PNG")
			qt6.QCoreApplication_Quit()
		})
		t.Start(600)
	}

	win.Show()
	// Fit height to content (CPU column is tallest); widen a touch for breathing room.
	win.Resize(1820, win.SizeHint().Height())
	qt6.QApplication_Exec()
}

// buildTitleBar creates the custom title bar (title text + red close button)
// and wires window dragging, since the window is frameless.
func buildTitleBar(win *qt6.QFrame) *qt6.QWidget {
	bar := qt6.NewQWidget2()
	bar.SetFixedHeight(26)
	bar.SetStyleSheet("background-color:#000000; border:none;")

	tb := hbox(0)
	tb.SetContentsMargins(8, 0, 0, 0)
	title := lbl("HWiNFO®64 v8.26-5730 @ ASUS   - System Summary",
		"color:#f2f2f2; font-weight:bold; background:transparent; border:none;",
		qt6.AlignLeft|qt6.AlignVCenter)
	tb.AddWidget(title.QWidget)
	tb.AddStretch()

	closeBtn := qt6.NewQPushButton3("✕")
	closeBtn.SetFixedWidth(36)
	closeBtn.SetFixedHeight(26)
	closeBtn.SetStyleSheet(
		"QPushButton{background-color:#c0392b; color:#ffffff; border:none; font-size:13px;}" +
			"QPushButton:hover{background-color:#e04b3a;}")
	closeBtn.OnClicked(func() { qt6.QCoreApplication_Quit() })
	tb.AddWidget(closeBtn.QWidget)
	bar.SetLayout(tb.QLayout)

	// Drag-to-move for the frameless window.
	var dragX, dragY int
	bar.OnMousePressEvent(func(super func(event *qt6.QMouseEvent), event *qt6.QMouseEvent) {
		gp := event.GlobalPos()
		dragX = gp.X() - win.X()
		dragY = gp.Y() - win.Y()
	})
	bar.OnMouseMoveEvent(func(super func(event *qt6.QMouseEvent), event *qt6.QMouseEvent) {
		gp := event.GlobalPos()
		win.Move(gp.X()-dragX, gp.Y()-dragY)
	})
	return bar
}

// buildContent lays out the three top-level columns.
func buildContent() *qt6.QWidget {
	content := qt6.NewQWidget2()
	cols := hbox(6)
	cols.SetContentsMargins(6, 4, 6, 6)

	// Middle column: Motherboard / Memory / Memory Modules.
	mid := vbox(6)
	mid.AddWidget(buildMotherboard().QWidget)
	mid.AddWidget(buildMemory().QWidget)
	mid.AddWidget(buildMemoryModules().QWidget)
	mid.AddStretch()
	midW := wrap(mid.QLayout)
	midW.SetMinimumWidth(560)

	// Right column: GPU / Current Clocks / Operating System / Drives.
	right := vbox(6)
	right.AddWidget(buildGPU().QWidget)
	right.AddWidget(buildCurrentClocks().QWidget)
	right.AddWidget(buildOS().QWidget)
	right.AddWidget(buildDrives().QWidget)
	right.AddStretch()
	rightW := wrap(right.QLayout)
	rightW.SetMinimumWidth(560)

	cols.AddWidget2(buildCPU().QWidget, 34)
	cols.AddWidget2(midW, 33)
	cols.AddWidget2(rightW, 33)
	content.SetLayout(cols.QLayout)
	return content
}

// ---------------------------------------------------------------- CPU column

func buildCPU() *qt6.QGroupBox {
	gb := group("CPU")
	root := vbox(6)

	// Logo + identity fields.
	top := hbox(8)
	top.AddWidget3(ryzenLogo().QWidget, 0, qt6.AlignTop)

	id := grid()
	id.AddWidget3(field("AMD Ryzen 7 5800X").QWidget, 0, 0, 1, 3)
	id.AddWidget2(field("7 nm").QWidget, 0, 3)
	id.AddWidget2(capL("Stepping").QWidget, 1, 0)
	id.AddWidget2(field("VMR-B0").QWidget, 1, 1)
	id.AddWidget2(capL("TDP").QWidget, 1, 2)
	id.AddWidget2(field("105 W").QWidget, 1, 3)
	id.AddWidget2(capL("Codename").QWidget, 2, 0)
	id.AddWidget2(field("Vermeer").QWidget, 2, 1)
	id.AddWidget2(capL("MCU").QWidget, 2, 2)
	id.AddWidget2(field("A20102B").QWidget, 2, 3)
	id.AddWidget2(capL("OPN").QWidget, 3, 0)
	id.AddWidget2(field("100-000000063").QWidget, 3, 1)
	id.AddWidget2(field("").QWidget, 3, 3)
	id.AddWidget2(capL("Platform").QWidget, 4, 0)
	id.AddWidget3(field("AM4").QWidget, 4, 1, 1, 3)
	id.SetColumnStretch(1, 3)
	id.SetColumnStretch(3, 2)
	id.SetColumnMinimumWidth(0, 66)
	id.SetColumnMinimumWidth(2, 42)
	top.AddWidget2(wrap(id.QLayout), 1)
	root.AddLayout(top.QLayout)

	// CPU selector.
	sel := hbox(6)
	cb := combo([]string{"CPU #0", "CPU #1"}, 0)
	cb.SetFixedWidth(122)
	sel.AddWidget(cb.QWidget)
	sel.AddStretch()
	root.AddLayout(sel.QLayout)

	// Core / cache.
	cc := grid()
	pairs := [][2]string{{"Core", "8 / 16"}, {"Cache L1", "8x32 + 8x32"}, {"L2", "8x512"}, {"L3", "32M"}}
	for i, p := range pairs {
		cc.AddWidget2(capL(p[0]).QWidget, 0, i*2)
		cc.AddWidget2(field(p[1]).QWidget, 0, i*2+1)
		cc.SetColumnStretch(i*2+1, 1)
	}
	cc.AddWidget2(field("-").QWidget, 1, 1)
	cc.AddWidget2(field("-").QWidget, 1, 3)
	cc.AddWidget2(field("-").QWidget, 1, 5)
	root.AddLayout(cc.QLayout)

	root.AddWidget(buildFeatures().QWidget)
	root.AddWidget(opTable().QWidget)
	root.AddStretch()

	gb.SetLayout(root.QLayout)
	gb.SetMinimumWidth(630)
	return gb
}

func buildFeatures() *qt6.QGroupBox {
	gb := group("Features")
	g := grid()
	g.SetHorizontalSpacing(10)
	g.SetVerticalSpacing(3)
	for r, row := range features {
		for c, f := range row {
			g.AddWidget2(featLbl(f.name, f.state).QWidget, r, c)
		}
	}
	for c := 0; c < 7; c++ {
		g.SetColumnStretch(c, 1)
	}
	gb.SetLayout(g.QLayout)
	return gb
}

func opTable() *qt6.QFrame {
	f := frame()
	g := grid()
	g.SetContentsMargins(2, 2, 2, 2)
	g.SetHorizontalSpacing(4)
	g.SetVerticalSpacing(1)
	heads := []string{"Operating Point", "Clock", "Ratio", "Bus", "VID"}
	for c, h := range heads {
		al := qt6.AlignRight | qt6.AlignVCenter
		if c == 0 {
			al = qt6.AlignLeft | qt6.AlignVCenter
		}
		g.AddWidget4(th(h, al).QWidget, 0, c, al)
	}
	for r, row := range opRows {
		for c, v := range row {
			al := qt6.AlignRight | qt6.AlignVCenter
			if c == 0 {
				al = qt6.AlignLeft | qt6.AlignVCenter
			}
			g.AddWidget4(td(v, al).QWidget, r+1, c, al)
		}
	}
	g.SetColumnStretch(0, 1)
	f.SetLayout(g.QLayout)
	return f
}

// -------------------------------------------------------- Motherboard column

func buildMotherboard() *qt6.QGroupBox {
	gb := group("Motherboard")
	root := vbox(5)
	root.AddWidget(field("ASUS ROG STRIX B550-F GAMING WIFI II").QWidget)

	g := grid()
	g.AddWidget2(capL("Chipset").QWidget, 0, 0)
	g.AddWidget3(field("AMD B550 (Promontory PROM19 C/C1)").QWidget, 0, 1, 1, 5)
	g.AddWidget2(capL("BIOS Date").QWidget, 1, 0)
	g.AddWidget2(field("09/29/2024").QWidget, 1, 1)
	g.AddWidget2(capL("Version").QWidget, 1, 2)
	g.AddWidget2(field("3611").QWidget, 1, 3)
	g.AddWidget4(green("UEFI").QWidget, 1, 4, qt6.AlignLeft|qt6.AlignVCenter)
	g.SetColumnStretch(1, 2)
	g.SetColumnStretch(3, 1)
	g.SetColumnMinimumWidth(0, 66)
	root.AddLayout(g.QLayout)

	gb.SetLayout(root.QLayout)
	return gb
}

func buildMemory() *qt6.QGroupBox {
	gb := group("Memory")
	root := vbox(4)

	r1 := hbox(5)
	r1.AddWidget(capW("Size", 52).QWidget)
	r1.AddWidget2(field("32 GB").QWidget, 2)
	r1.AddWidget(capW("Type", 40).QWidget)
	r1.AddWidget2(field("DDR4 SDRAM").QWidget, 3)
	root.AddLayout(r1.QLayout)

	r2 := hbox(5)
	r2.AddWidget(capW("Clock", 52).QWidget)
	r2.AddWidget2(field("1197.8 MHz").QWidget, 3)
	r2.AddWidget(sym("=").QWidget)
	r2.AddWidget2(field("12.00").QWidget, 2)
	r2.AddWidget(sym("x").QWidget)
	r2.AddWidget2(field("99.8 MHz").QWidget, 2)
	r2.AddWidget(ramIcon().QWidget)
	root.AddLayout(r2.QLayout)

	r3 := hbox(5)
	r3.AddWidget(capW("Mode", 52).QWidget)
	r3.AddWidget2(field("Dual-Channel").QWidget, 4)
	r3.AddWidget(capW("CR", 24).QWidget)
	r3.AddWidget2(field("2T").QWidget, 1)
	root.AddLayout(r3.QLayout)

	r4 := hbox(4)
	r4.AddWidget(capW("Timing", 52).QWidget)
	r4.AddWidget2(field("16").QWidget, 1)
	r4.AddWidget(sym("-").QWidget)
	r4.AddWidget2(field("16").QWidget, 1)
	r4.AddWidget(sym("-").QWidget)
	r4.AddWidget2(field("16").QWidget, 1)
	r4.AddWidget(sym("-").QWidget)
	r4.AddWidget2(field("39").QWidget, 1)
	r4.AddWidget(capW("tRC", 30).QWidget)
	r4.AddWidget2(field("55").QWidget, 1)
	r4.AddWidget(capW("tRFC", 36).QWidget)
	r4.AddWidget2(field("421").QWidget, 1)
	root.AddLayout(r4.QLayout)

	gb.SetLayout(root.QLayout)
	return gb
}

func buildMemoryModules() *qt6.QGroupBox {
	gb := group("Memory Modules")
	root := vbox(5)

	cb := combo([]string{
		"#0 [BANK 0/DIMM_A1]: Corsair CMK16GX4M2A2400C16",
		"#1 [BANK 1/DIMM_B1]: Corsair CMK16GX4M2A2400C16",
	}, 0)
	root.AddWidget(cb.QWidget)

	r1 := hbox(5)
	r1.AddWidget(capW("Size", 52).QWidget)
	r1.AddWidget2(field("8 GB").QWidget, 2)
	r1.AddWidget(capW("Clock", 40).QWidget)
	r1.AddWidget2(field("1200 MHz").QWidget, 2)
	r1.AddWidget(capW("ECC", 32).QWidget)
	r1.AddWidget2(field("No").QWidget, 1)
	root.AddLayout(r1.QLayout)

	r2 := hbox(5)
	r2.AddWidget(capW("Type", 52).QWidget)
	r2.AddWidget2(field("DDR4-2400 / PC4-19200 DDR4 SDRAM UDIMM").QWidget, 1)
	root.AddLayout(r2.QLayout)

	root.AddWidget(memTable().QWidget)
	gb.SetLayout(root.QLayout)
	return gb
}

func memTable() *qt6.QFrame {
	f := frame()
	g := grid()
	g.SetContentsMargins(2, 2, 2, 2)
	g.SetHorizontalSpacing(4)
	g.SetVerticalSpacing(1)
	for c, h := range memHeaders {
		al := qt6.AlignRight | qt6.AlignVCenter
		g.AddWidget4(th(h, al).QWidget, 0, c, al)
	}
	for r, row := range memRows {
		for c, v := range row {
			al := qt6.AlignRight | qt6.AlignVCenter
			g.AddWidget4(td(v, al).QWidget, r+1, c, al)
		}
	}
	for c := 0; c < len(memHeaders); c++ {
		g.SetColumnStretch(c, 1)
	}
	f.SetLayout(g.QLayout)
	return f
}

// ---------------------------------------------------------------- GPU column

func buildGPU() *qt6.QGroupBox {
	gb := group("GPU")
	root := vbox(5)

	top := hbox(8)
	top.AddWidget3(nvidiaLogo().QWidget, 0, qt6.AlignTop)
	id := vbox(4)
	id.AddWidget(field("DELL GeForce RTX 3080").QWidget)
	id.AddWidget(field("NVIDIA GeForce RTX 3080").QWidget)
	id.AddWidget(field("GA102-200").QWidget)
	top.AddWidget2(wrap(id.QLayout), 1)
	root.AddLayout(top.QLayout)

	root.AddWidget(field("PCIe v4.0 x16 (16.0 GT/s) @ x16 (16.0 GT/s)").QWidget)

	r1 := hbox(5)
	cb := combo([]string{"GPU #0"}, 0)
	cb.SetFixedWidth(92)
	r1.AddWidget(cb.QWidget)
	r1.AddWidget2(field("10 GB").QWidget, 1)
	r1.AddWidget2(field("GDDR6X SDRAM").QWidget, 2)
	r1.AddWidget2(field("320-bit").QWidget, 1)
	root.AddLayout(r1.QLayout)

	r2 := hbox(5)
	r2.AddWidget(capW("ROPs / TMUs", 80).QWidget)
	r2.AddWidget2(field("96 / 272").QWidget, 1)
	r2.AddWidget(capW("SH/RT/TC", 64).QWidget)
	r2.AddWidget2(field("8704 / 68 / 272").QWidget, 2)
	root.AddLayout(r2.QLayout)

	gb.SetLayout(root.QLayout)
	return gb
}

func buildCurrentClocks() *qt6.QGroupBox {
	gb := group("Current Clocks (MHz)")
	r := hbox(5)
	r.AddWidget(capW("GPU", 34).QWidget)
	r.AddWidget2(field("1710.0").QWidget, 1)
	r.AddWidget(capW("Memory", 54).QWidget)
	r.AddWidget2(field("1187.7").QWidget, 1)
	r.AddWidget(capW("Video", 44).QWidget)
	r.AddWidget2(field("1500.0").QWidget, 1)
	gb.SetLayout(r.QLayout)
	return gb
}

func buildOS() *qt6.QGroupBox {
	gb := group("Operating System")
	root := vbox(5)

	flags := hbox(16)
	flags.AddWidget(lbl("UEFI Boot", greenQSS, qt6.AlignLeft|qt6.AlignVCenter).QWidget)
	flags.AddWidget(lbl("Secure Boot", redQSS, qt6.AlignLeft|qt6.AlignVCenter).QWidget)
	flags.AddWidget(lbl("TPM", greenQSS, qt6.AlignLeft|qt6.AlignVCenter).QWidget)
	flags.AddWidget(lbl("HVCI", grayQSS, qt6.AlignLeft|qt6.AlignVCenter).QWidget)
	flags.AddStretch()
	root.AddLayout(flags.QLayout)

	root.AddWidget(field("Microsoft Windows 11 Professional (x64) Build 26100.4351 (24H2)").QWidget)
	gb.SetLayout(root.QLayout)
	return gb
}

func buildDrives() *qt6.QGroupBox {
	gb := group("Drives")
	root := vbox(0)
	root.AddWidget(drivesTable().QWidget)
	gb.SetLayout(root.QLayout)
	return gb
}

func drivesTable() *qt6.QFrame {
	f := frame()
	g := grid()
	g.SetContentsMargins(2, 2, 2, 2)
	g.SetHorizontalSpacing(6)
	g.SetVerticalSpacing(1)
	g.AddWidget2(th("", qt6.AlignLeft).QWidget, 0, 0)
	g.AddWidget4(th("Interface", qt6.AlignLeft|qt6.AlignVCenter).QWidget, 0, 1, qt6.AlignLeft|qt6.AlignVCenter)
	g.AddWidget4(th("Model [Capacity]", qt6.AlignLeft|qt6.AlignVCenter).QWidget, 0, 2, qt6.AlignLeft|qt6.AlignVCenter)
	for i, d := range drives {
		chk := " "
		style := grayQSS
		if d.ok {
			chk = "✔" // heavy check mark
			style = chkQSS
		}
		g.AddWidget4(lbl(chk, style, qt6.AlignCenter).QWidget, i+1, 0, qt6.AlignCenter)
		g.AddWidget4(td(d.iface, qt6.AlignLeft|qt6.AlignVCenter).QWidget, i+1, 1, qt6.AlignLeft|qt6.AlignVCenter)
		g.AddWidget4(td(d.model, qt6.AlignLeft|qt6.AlignVCenter).QWidget, i+1, 2, qt6.AlignLeft|qt6.AlignVCenter)
	}
	g.SetColumnStretch(2, 1)
	f.SetLayout(g.QLayout)
	return f
}

// ------------------------------------------------------------- small widgets

// capW is a fixed-width caption label.
func capW(text string, w int) *qt6.QLabel {
	l := capL(text)
	l.SetFixedWidth(w)
	return l
}

// sym is a narrow centered symbol label ("=", "x", "-").
func sym(text string) *qt6.QLabel {
	l := lbl(text, capQSS, qt6.AlignCenter)
	l.SetFixedWidth(10)
	return l
}

func ryzenLogo() *qt6.QLabel {
	l := qt6.NewQLabel3("")
	l.SetTextFormat(qt6.RichText)
	l.SetText(`<div align="center" style="line-height:110%;">` +
		`<span style="color:#f5820a; font-size:26px; font-weight:bold;">&#9698;&#9700;</span><br>` +
		`<span style="color:#f2f2f2; font-size:15px; font-weight:bold; letter-spacing:3px;">RYZEN</span>` +
		`</div>`)
	l.SetAlignment(qt6.AlignCenter)
	l.SetStyleSheet("background-color:#141517; border:1px solid #3b3b3b;")
	l.SetFixedWidth(118)
	l.SetFixedHeight(94)
	return l
}

func nvidiaLogo() *qt6.QLabel {
	l := qt6.NewQLabel3("")
	l.SetTextFormat(qt6.RichText)
	l.SetText(`<div align="center" style="line-height:112%;">` +
		`<span style="color:#76b900; font-size:22px;">&#9673;</span><br>` +
		`<span style="color:#76b900; font-size:14px; font-weight:bold; letter-spacing:1px;">NVIDIA</span><br>` +
		`<span style="color:#cccccc; font-size:10px; letter-spacing:2px;">GEFORCE</span>` +
		`</div>`)
	l.SetAlignment(qt6.AlignCenter)
	l.SetStyleSheet("background-color:#0c0c0c; border:1px solid #2a2a2a;")
	l.SetFixedWidth(118)
	l.SetFixedHeight(94)
	return l
}

func ramIcon() *qt6.QLabel {
	l := qt6.NewQLabel3("")
	l.SetFixedWidth(28)
	l.SetFixedHeight(15)
	l.SetStyleSheet("background-color:#123f12; border:1px solid #45a545;")
	return l
}
