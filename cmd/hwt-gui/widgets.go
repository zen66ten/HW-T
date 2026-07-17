package main

import (
	"image/color"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"
)

// valueBox is the GPU-Z-style bordered box holding one monospace value.
type valueBox struct {
	widget.BaseWidget
	mu   sync.Mutex
	text string
	col  color.Color // nil = palette foreground
}

func newValueBox() *valueBox {
	v := &valueBox{text: "-"}
	v.ExtendBaseWidget(v)
	return v
}

// Set updates the text. col nil means "normal foreground", letting the box
// track theme switches automatically.
func (v *valueBox) Set(text string, col color.Color) {
	v.mu.Lock()
	v.text, v.col = text, col
	v.mu.Unlock()
	v.Refresh()
}

func (v *valueBox) CreateRenderer() fyne.WidgetRenderer {
	bg := canvas.NewRectangle(pal.box)
	bg.CornerRadius = 3
	bg.StrokeColor = pal.boxStroke
	bg.StrokeWidth = 1
	txt := canvas.NewText(v.text, pal.foreground)
	txt.TextStyle.Monospace = true
	txt.TextSize = 12
	return &valueBoxRenderer{v: v, bg: bg, txt: txt}
}

type valueBoxRenderer struct {
	v   *valueBox
	bg  *canvas.Rectangle
	txt *canvas.Text
}

func (r *valueBoxRenderer) MinSize() fyne.Size { return fyne.NewSize(96, 20) }
func (r *valueBoxRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.bg, r.txt}
}
func (r *valueBoxRenderer) Destroy() {}

func (r *valueBoxRenderer) Layout(size fyne.Size) {
	r.bg.Resize(size)
	ts := r.txt.MinSize()
	r.txt.Move(fyne.NewPos(size.Width-ts.Width-6, (size.Height-ts.Height)/2))
}

func (r *valueBoxRenderer) Refresh() {
	r.bg.FillColor, r.bg.StrokeColor = pal.box, pal.boxStroke
	r.v.mu.Lock()
	r.txt.Text = r.v.text
	if r.v.col != nil {
		r.txt.Color = r.v.col
	} else {
		r.txt.Color = pal.foreground
	}
	r.v.mu.Unlock()
	r.Layout(r.v.Size())
	canvas.Refresh(r.v)
}

// barCount is the width of the rolling window each sensor row displays.
const barCount = 48

// histBar is the GPU-Z-style rolling bar graph: newest sample on the
// right, bars scaled to the window's min..max.
type histBar struct {
	widget.BaseWidget
	mu   sync.Mutex
	vals []float64
}

func newHistBar() *histBar {
	h := &histBar{}
	h.ExtendBaseWidget(h)
	return h
}

func (h *histBar) Push(v float64) {
	h.mu.Lock()
	h.vals = append(h.vals, v)
	if len(h.vals) > barCount {
		h.vals = h.vals[len(h.vals)-barCount:]
	}
	h.mu.Unlock()
	h.Refresh()
}

func (h *histBar) CreateRenderer() fyne.WidgetRenderer {
	track := canvas.NewRectangle(pal.barTrack)
	track.CornerRadius = 2
	track.StrokeColor = pal.boxStroke
	track.StrokeWidth = 1
	bars := make([]*canvas.Rectangle, barCount)
	for i := range bars {
		b := canvas.NewRectangle(pal.bar)
		b.Hidden = true
		bars[i] = b
	}
	objs := []fyne.CanvasObject{track}
	for _, b := range bars {
		objs = append(objs, b)
	}
	return &histBarRenderer{h: h, track: track, bars: bars, objects: objs}
}

type histBarRenderer struct {
	h       *histBar
	track   *canvas.Rectangle
	bars    []*canvas.Rectangle
	objects []fyne.CanvasObject
}

func (r *histBarRenderer) MinSize() fyne.Size           { return fyne.NewSize(140, 20) }
func (r *histBarRenderer) Objects() []fyne.CanvasObject { return r.objects }
func (r *histBarRenderer) Destroy()                     {}

func (r *histBarRenderer) Refresh() {
	r.track.FillColor, r.track.StrokeColor = pal.barTrack, pal.boxStroke
	for _, b := range r.bars {
		b.FillColor = pal.bar
	}
	r.Layout(r.h.Size())
	canvas.Refresh(r.h)
}

func (r *histBarRenderer) Layout(size fyne.Size) {
	r.track.Resize(size)

	r.h.mu.Lock()
	vals := append([]float64(nil), r.h.vals...)
	r.h.mu.Unlock()

	if len(vals) == 0 {
		for _, b := range r.bars {
			b.Hidden = true
		}
		return
	}

	lo, hi := vals[0], vals[0]
	for _, v := range vals {
		if v < lo {
			lo = v
		}
		if v > hi {
			hi = v
		}
	}

	slot := size.Width / float32(barCount)
	gap := slot * 0.18
	maxH := size.Height - 4
	for i, b := range r.bars {
		vi := len(vals) - barCount + i
		if vi < 0 {
			b.Hidden = true
			continue
		}
		frac := 0.55 // flat series renders mid-height, GPU-Z style
		if hi > lo {
			frac = 0.12 + 0.88*(vals[vi]-lo)/(hi-lo)
		}
		h := float32(frac) * maxH
		b.Hidden = false
		b.Resize(fyne.NewSize(slot-gap, h))
		b.Move(fyne.NewPos(float32(i)*slot+gap/2, size.Height-2-h))
	}
}

// rowLayout arranges [name, value box, bar]: fixed name and value widths,
// the bar takes the remainder.
type rowLayout struct{}

const (
	rowNameW  = 200
	rowValueW = 110
	rowGap    = 5
	rowH      = 22
)

func (rowLayout) MinSize([]fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(rowNameW+rowValueW+140+2*rowGap, rowH)
}

func (rowLayout) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	if len(objs) != 3 {
		return
	}
	name, val, bar := objs[0], objs[1], objs[2]
	name.Move(fyne.NewPos(4, 0))
	name.Resize(fyne.NewSize(rowNameW, size.Height))
	val.Move(fyne.NewPos(rowNameW+rowGap, 1))
	val.Resize(fyne.NewSize(rowValueW, size.Height-2))
	barX := float32(rowNameW + rowValueW + 2*rowGap)
	bar.Move(fyne.NewPos(barX, 1))
	bar.Resize(fyne.NewSize(size.Width-barX-4, size.Height-2))
}

// tappableRow wraps a row so clicking it selects the sensor for the big
// history chart.
type tappableRow struct {
	widget.BaseWidget
	content fyne.CanvasObject
	onTap   func()
}

func newTappableRow(content fyne.CanvasObject, onTap func()) *tappableRow {
	t := &tappableRow{content: content, onTap: onTap}
	t.ExtendBaseWidget(t)
	return t
}

func (t *tappableRow) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(t.content)
}

func (t *tappableRow) Tapped(*fyne.PointEvent) {
	if t.onTap != nil {
		t.onTap()
	}
}
