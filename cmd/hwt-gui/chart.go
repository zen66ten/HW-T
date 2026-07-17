package main

import (
	"fmt"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/zen66ten/HW-T/internal/core"
	"github.com/zen66ten/HW-T/pkg/client"
)

// maxSegments caps the polyline detail; history is downsampled into this
// many buckets regardless of ring size.
const maxSegments = 240

// chart renders one sensor's ring-buffer history as a polyline with
// min/max/current annotations. Call SetSeries from the UI thread.
type chart struct {
	widget.BaseWidget
	mu    sync.Mutex
	label string
	kind  core.Kind
	pts   []client.Point
}

func newChart() *chart {
	c := &chart{}
	c.ExtendBaseWidget(c)
	return c
}

func (c *chart) SetSeries(label string, kind core.Kind, pts []client.Point) {
	c.mu.Lock()
	c.label, c.kind, c.pts = label, kind, pts
	c.mu.Unlock()
	c.Refresh()
}

func (c *chart) CreateRenderer() fyne.WidgetRenderer {
	r := &chartRenderer{c: c}
	r.bg = canvas.NewRectangle(theme.Color(theme.ColorNameInputBackground))
	r.title = canvas.NewText("select a sensor to graph its history", theme.Color(theme.ColorNameForeground))
	r.title.TextStyle.Bold = true
	r.cur = canvas.NewText("", theme.Color(theme.ColorNamePrimary))
	r.cur.TextStyle.Monospace = true
	r.lo = canvas.NewText("", theme.Color(theme.ColorNameForeground))
	r.lo.TextSize = theme.CaptionTextSize()
	r.hi = canvas.NewText("", theme.Color(theme.ColorNameForeground))
	r.hi.TextSize = theme.CaptionTextSize()
	r.span = canvas.NewText("", theme.Color(theme.ColorNameForeground))
	r.span.TextSize = theme.CaptionTextSize()

	r.lines = make([]*canvas.Line, maxSegments)
	for i := range r.lines {
		l := canvas.NewLine(theme.Color(theme.ColorNamePrimary))
		l.StrokeWidth = 1.5
		l.Hidden = true
		r.lines[i] = l
	}

	r.objects = []fyne.CanvasObject{r.bg}
	for _, l := range r.lines {
		r.objects = append(r.objects, l)
	}
	r.objects = append(r.objects, r.title, r.cur, r.lo, r.hi, r.span)
	return r
}

type chartRenderer struct {
	c       *chart
	bg      *canvas.Rectangle
	lines   []*canvas.Line
	title   *canvas.Text
	cur     *canvas.Text
	lo, hi  *canvas.Text
	span    *canvas.Text
	objects []fyne.CanvasObject
}

func (r *chartRenderer) MinSize() fyne.Size           { return fyne.NewSize(400, 170) }
func (r *chartRenderer) Objects() []fyne.CanvasObject { return r.objects }
func (r *chartRenderer) Destroy()                     {}

func (r *chartRenderer) Refresh() {
	r.bg.FillColor = pal.box
	r.title.Color, r.lo.Color, r.hi.Color, r.span.Color = pal.foreground, pal.dim, pal.dim, pal.dim
	r.cur.Color = pal.accent
	for _, l := range r.lines {
		l.StrokeColor = pal.bar
	}
	r.Layout(r.c.Size())
	canvas.Refresh(r.c)
}

func (r *chartRenderer) Layout(size fyne.Size) {
	r.bg.Resize(size)
	r.bg.Move(fyne.NewPos(0, 0))

	r.c.mu.Lock()
	label, kind, pts := r.c.label, r.c.kind, r.c.pts
	r.c.mu.Unlock()

	pad := float32(8)
	top := float32(28)
	bottom := float32(20)
	plotW := size.Width - 2*pad
	plotH := size.Height - top - bottom

	r.title.Move(fyne.NewPos(pad, 4))
	r.cur.Move(fyne.NewPos(size.Width-160, 4))
	r.lo.Move(fyne.NewPos(pad, size.Height-18))
	r.hi.Move(fyne.NewPos(pad, top-14))
	r.span.Move(fyne.NewPos(size.Width-140, size.Height-18))

	if len(pts) < 2 || plotW <= 0 || plotH <= 0 {
		for _, l := range r.lines {
			l.Hidden = true
		}
		if label != "" {
			r.title.Text = label + ", waiting for history"
		}
		r.cur.Text, r.lo.Text, r.hi.Text, r.span.Text = "", "", "", ""
		return
	}

	// Downsample into per-bucket averages so 7200 points still draw fast.
	nSeg := maxSegments
	if int(plotW/3) < nSeg {
		nSeg = int(plotW / 3)
	}
	if nSeg < 2 {
		nSeg = 2
	}
	buckets := resample(pts, nSeg)

	lo, hi := buckets[0], buckets[0]
	for _, v := range buckets {
		if v < lo {
			lo = v
		}
		if v > hi {
			hi = v
		}
	}
	if hi == lo {
		hi = lo + 1 // flat series still renders mid-plot
	}

	xStep := plotW / float32(len(buckets)-1)
	y := func(v float64) float32 {
		return top + plotH - float32((v-lo)/(hi-lo))*plotH
	}
	for i, l := range r.lines {
		if i >= len(buckets)-1 {
			l.Hidden = true
			continue
		}
		l.Hidden = false
		l.Position1 = fyne.NewPos(pad+float32(i)*xStep, y(buckets[i]))
		l.Position2 = fyne.NewPos(pad+float32(i+1)*xStep, y(buckets[i+1]))
	}

	r.title.Text = label
	r.cur.Text = core.FormatValue(kind, pts[len(pts)-1].Value)
	r.lo.Text = core.FormatValue(kind, lo)
	r.hi.Text = core.FormatValue(kind, hi)
	spanD := time.Duration(pts[len(pts)-1].Ts-pts[0].Ts) * time.Millisecond
	r.span.Text = fmt.Sprintf("last %s", spanD.Round(time.Second))
}

// resample averages pts into n equal-width buckets, oldest to newest.
func resample(pts []client.Point, n int) []float64 {
	out := make([]float64, n)
	counts := make([]int, n)
	for i, p := range pts {
		b := i * n / len(pts)
		out[b] += p.Value
		counts[b]++
	}
	last := 0.0
	for i := range out {
		if counts[i] > 0 {
			out[i] /= float64(counts[i])
			last = out[i]
		} else {
			out[i] = last // empty bucket: carry previous value
		}
	}
	return out
}
