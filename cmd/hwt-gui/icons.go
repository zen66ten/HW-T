package main

import (
	"strings"

	"github.com/mappu/miqt/qt6"

	"github.com/zen66ten/HW-T/internal/core"
)

// kindIcon returns the 16x16 icon shown next to a sensor row, keyed by
// sensor kind: red thermometer for temperatures, cyan clock face for
// clocks, yellow lightning bolt for voltages, and so on. Icons are drawn
// once with QPainter and cached; nil means no icon for that kind.
var iconCache = map[core.Kind]*qt6.QIcon{}

func kindIcon(k core.Kind) *qt6.QIcon {
	if ic, ok := iconCache[k]; ok {
		return ic
	}
	var ic *qt6.QIcon
	switch k {
	case core.KindTemp:
		ic = drawIcon(paintThermometer)
	case core.KindFreq:
		ic = drawIcon(paintClock)
	case core.KindIn:
		ic = drawIcon(func(p *qt6.QPainter) { paintBolt(p, 0xf5c211) }) // yellow
	case core.KindCurr:
		ic = drawIcon(func(p *qt6.QPainter) { paintBolt(p, 0x2fb8b8) }) // teal
	case core.KindPower, core.KindEnergy:
		ic = drawIcon(func(p *qt6.QPainter) { paintBolt(p, 0xe8823d) }) // orange
	case core.KindFan, core.KindPWM:
		ic = drawIcon(paintFan)
	case core.KindPercent:
		ic = drawIcon(paintBars)
	case core.KindData:
		ic = drawIcon(paintRAM)
	case core.KindHealth:
		ic = drawIcon(paintCross)
	default:
		ic = nil
	}
	iconCache[k] = ic
	return ic
}

// drawIcon paints one 16x16 transparent pixmap and wraps it in a QIcon.
func drawIcon(paint func(*qt6.QPainter)) *qt6.QIcon {
	pm := qt6.NewQPixmap2(16, 16)
	pm.FillWithFillColor(qt6.NewQColor2(qt6.Transparent))
	p := qt6.NewQPainter2(pm.QPaintDevice)
	p.SetRenderHint(qt6.QPainter__Antialiasing)
	paint(p)
	p.End()
	return qt6.NewQIcon2(pm)
}

func solid(p *qt6.QPainter, rgb uint) *qt6.QBrush {
	c := qt6.NewQColor4(0xff000000 | rgb)
	p.SetPen(c)
	return qt6.NewQBrush3(c)
}

// paintThermometer: red stem with a bulb at the bottom.
func paintThermometer(p *qt6.QPainter) {
	b := solid(p, 0xe0463c)
	p.SetPenWithStyle(qt6.NoPen)
	path := qt6.NewQPainterPath()
	path.AddRoundedRect(qt6.NewQRectF4(6.5, 1, 3, 9), 1.5, 1.5)
	path.AddEllipse2(4.5, 8.5, 7, 7)
	p.FillPath(path, b)
}

// paintClock: cyan circle with hour and minute hands.
func paintClock(p *qt6.QPainter) {
	c := qt6.NewQColor3(0x35, 0xb8, 0xd0)
	pen := qt6.NewQPen3(c)
	pen.SetWidthF(1.6)
	p.SetPenWithPen(pen)
	p.SetBrushWithStyle(qt6.NoBrush)
	p.DrawEllipse2(2, 2, 12, 12)
	p.DrawLine2(8, 8, 8, 4)
	p.DrawLine2(8, 8, 11, 10)
}

// paintBolt: lightning bolt in the given color (voltage / current / power).
func paintBolt(p *qt6.QPainter, rgb uint) {
	b := solid(p, rgb)
	p.SetPenWithStyle(qt6.NoPen)
	path := qt6.NewQPainterPath()
	path.MoveTo2(9.5, 1)
	path.LineTo2(4, 9)
	path.LineTo2(7.5, 9)
	path.LineTo2(6.5, 15)
	path.LineTo2(12, 6.5)
	path.LineTo2(8.5, 6.5)
	path.CloseSubpath()
	p.FillPath(path, b)
}

// paintFan: three blue pie blades around a hub.
func paintFan(p *qt6.QPainter) {
	b := solid(p, 0x3d7de8)
	p.SetPenWithStyle(qt6.NoPen)
	p.SetBrush(b)
	// Qt angles are in 1/16 degree, counterclockwise from 3 o'clock.
	for _, start := range []int{80, 200, 320} {
		p.DrawPie2(1, 1, 14, 14, start*16, 55*16)
	}
	p.DrawEllipse2(6, 6, 4, 4)
}

// paintBars: green utilization bar chart.
func paintBars(p *qt6.QPainter) {
	b := solid(p, 0x3ac83a)
	p.SetPenWithStyle(qt6.NoPen)
	p.SetBrush(b)
	p.DrawRect2(2, 10, 3, 5)
	p.DrawRect2(6, 6, 3, 9)
	p.DrawRect2(11, 2, 3, 13)
}

// paintRAM: purple memory stick with pins.
func paintRAM(p *qt6.QPainter) {
	b := solid(p, 0xa06be0)
	p.SetPenWithStyle(qt6.NoPen)
	p.SetBrush(b)
	p.DrawRoundedRect2(1, 4, 14, 7, 1.5, 1.5)
	for x := 2; x <= 13; x += 3 {
		p.DrawRect2(x, 11, 2, 3)
	}
}

// paintCross: green health cross.
func paintCross(p *qt6.QPainter) {
	b := solid(p, 0x3ac83a)
	p.SetPenWithStyle(qt6.NoPen)
	path := qt6.NewQPainterPath()
	path.AddRoundedRect(qt6.NewQRectF4(6, 2, 4, 12), 1, 1)
	path.AddRoundedRect(qt6.NewQRectF4(2, 6, 12, 4), 1, 1)
	p.FillPath(path, b)
}

// ---------------------------------------------------------------- logos
//
// Vendor logos are lightweight stylized placeholders (colored glyphs and
// wordmarks), not trademarked artwork — same approach as the mock in gui/.

func logoLabel(html, boxQSS string) *qt6.QLabel {
	l := qt6.NewQLabel3("")
	l.SetTextFormat(qt6.RichText)
	l.SetText(html)
	l.SetAlignment(qt6.AlignCenter)
	l.SetStyleSheet(boxQSS)
	l.SetFixedWidth(118)
	l.SetFixedHeight(94)
	return l
}

// cpuLogo picks the CPU vendor badge from the cpuinfo vendor string and
// model name.
func cpuLogo(vendor, model string) *qt6.QLabel {
	m := strings.ToLower(model)
	switch {
	case strings.Contains(vendor, "AuthenticAMD") || strings.Contains(m, "amd"):
		if strings.Contains(m, "ryzen") {
			return logoLabel(`<div align="center" style="line-height:110%;">`+
				`<span style="color:#f5820a; font-size:26px; font-weight:bold;">&#9698;&#9700;</span><br>`+
				`<span style="color:#f2f2f2; font-size:15px; font-weight:bold; letter-spacing:3px;">RYZEN</span>`+
				`</div>`,
				"background-color:#141517; border:1px solid #3b3b3b;")
		}
		return logoLabel(`<div align="center" style="line-height:120%;">`+
			`<span style="color:#009a66; font-size:24px; font-weight:bold;">&#9650;</span><br>`+
			`<span style="color:#f2f2f2; font-size:16px; font-weight:bold; letter-spacing:2px;">AMD</span>`+
			`</div>`,
			"background-color:#141517; border:1px solid #3b3b3b;")
	case strings.Contains(vendor, "GenuineIntel") || strings.Contains(m, "intel"):
		return logoLabel(`<div align="center" style="line-height:115%;">`+
			`<span style="color:#0a94d6; font-size:22px; font-weight:bold; letter-spacing:1px;">intel</span><br>`+
			`<span style="color:#cccccc; font-size:11px; letter-spacing:3px;">CORE</span>`+
			`</div>`,
			"background-color:#0b1420; border:1px solid #163a5e;")
	}
	return logoLabel(`<div align="center">`+
		`<span style="color:#9aa5a5; font-size:14px; font-weight:bold; letter-spacing:2px;">CPU</span>`+
		`</div>`,
		"background-color:#141517; border:1px solid #3b3b3b;")
}

// gpuLogo picks the GPU vendor badge from the provider name and device name.
func gpuLogo(provider, name string) *qt6.QLabel {
	n := strings.ToLower(name)
	switch {
	case provider == "nvidia" || strings.Contains(n, "nvidia") || strings.Contains(n, "geforce"):
		return logoLabel(`<div align="center" style="line-height:112%;">`+
			`<span style="color:#76b900; font-size:22px;">&#9673;</span><br>`+
			`<span style="color:#76b900; font-size:14px; font-weight:bold; letter-spacing:1px;">NVIDIA</span><br>`+
			`<span style="color:#cccccc; font-size:10px; letter-spacing:2px;">GEFORCE</span>`+
			`</div>`,
			"background-color:#0c0c0c; border:1px solid #2a2a2a;")
	case provider == "amdgpu" || strings.Contains(n, "radeon") || strings.Contains(n, "amd"):
		return logoLabel(`<div align="center" style="line-height:112%;">`+
			`<span style="color:#ed1c24; font-size:22px; font-weight:bold;">&#9632;</span><br>`+
			`<span style="color:#f2f2f2; font-size:14px; font-weight:bold; letter-spacing:1px;">RADEON</span><br>`+
			`<span style="color:#ed1c24; font-size:10px; letter-spacing:2px;">GRAPHICS</span>`+
			`</div>`,
			"background-color:#0c0c0c; border:1px solid #2a2a2a;")
	case provider == "intel" || strings.Contains(n, "arc") || strings.Contains(n, "intel"):
		return logoLabel(`<div align="center" style="line-height:112%;">`+
			`<span style="color:#0a94d6; font-size:22px; font-weight:bold; letter-spacing:1px;">intel</span><br>`+
			`<span style="color:#7ce0f0; font-size:14px; font-weight:bold; letter-spacing:4px;">ARC</span>`+
			`</div>`,
			"background-color:#0b1420; border:1px solid #163a5e;")
	}
	return logoLabel(`<div align="center">`+
		`<span style="color:#9aa5a5; font-size:14px; font-weight:bold; letter-spacing:2px;">GPU</span>`+
		`</div>`,
		"background-color:#141517; border:1px solid #3b3b3b;")
}
