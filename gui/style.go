package main

import "github.com/mappu/miqt/qt6"

// appStyle is the global stylesheet applied to the whole application. It sets
// the dark HWiNFO-like theme: black background, light text, teal group-box and
// combo borders. Per-widget "classes" (fields, captions, table cells, feature
// flags) are styled with the *QSS constants below, applied via SetStyleSheet.
const appStyle = `
QWidget {
    background-color: #000000;
    color: #e6e9e9;
    font-family: "Segoe UI", "DejaVu Sans", "Noto Sans", sans-serif;
    font-size: 12px;
}
QToolTip {
    color: #ffffff;
    background-color: #0d1414;
    border: 1px solid #2f6d6d;
}
QGroupBox {
    border: 1px solid #1f8a8a;
    border-radius: 2px;
    margin-top: 8px;
    padding: 9px 5px 6px 5px;
    background-color: #000000;
}
QGroupBox::title {
    subcontrol-origin: margin;
    subcontrol-position: top left;
    left: 9px;
    top: 1px;
    padding: 0px 4px;
    color: #ffffff;
}
QComboBox {
    background-color: #0b0f10;
    border: 1px solid #2f6d6d;
    color: #f0f2f2;
    padding: 1px 6px;
}
QComboBox::drop-down {
    subcontrol-origin: padding;
    subcontrol-position: center right;
    width: 16px;
    border-left: 1px solid #2f6d6d;
}
QComboBox::down-arrow {
    width: 0; height: 0;
    border-left: 4px solid transparent;
    border-right: 4px solid transparent;
    border-top: 5px solid #cdd4d4;
    margin-right: 3px;
}
QComboBox QAbstractItemView {
    background-color: #0b0f10;
    color: #f0f2f2;
    border: 1px solid #2f6d6d;
    selection-background-color: #1f8a8a;
    selection-color: #ffffff;
}
`

// Per-widget style "classes".
const (
	fieldQSS = "background-color:#0b1011; border:1px solid #2f6d6d; color:#f2f4f4; padding:2px 5px;"
	capQSS   = "color:#dde1e1; background:transparent; padding:0px 2px; border:none;"
	thQSS    = "background-color:#13201f; color:#ffffff; padding:2px 6px; border:none; border-bottom:1px solid #2f6d6d;"
	tdQSS    = "color:#e2e6e6; background:transparent; padding:2px 6px; border:none;"
	chkQSS   = "color:#33d13a; background:transparent; border:none; font-size:13px; font-weight:bold;"
	greenQSS = "color:#3ac83a; background:transparent; border:none;"
	grayQSS  = "color:#59615f; background:transparent; border:none;"
	redQSS   = "color:#e0463c; background:transparent; border:none;"
)

// lbl builds a QLabel with the given stylesheet and alignment.
func lbl(text, style string, align qt6.AlignmentFlag) *qt6.QLabel {
	l := qt6.NewQLabel3(text)
	if style != "" {
		l.SetStyleSheet(style)
	}
	l.SetAlignment(align)
	return l
}

// Field boxes (bordered value cells).
func field(text string) *qt6.QLabel  { return lbl(text, fieldQSS, qt6.AlignCenter) }
func fieldL(text string) *qt6.QLabel { return lbl(text, fieldQSS, qt6.AlignLeft|qt6.AlignVCenter) }

// Captions (plain labels next to fields).
func capL(text string) *qt6.QLabel { return lbl(text, capQSS, qt6.AlignLeft|qt6.AlignVCenter) }
func capR(text string) *qt6.QLabel { return lbl(text, capQSS, qt6.AlignRight|qt6.AlignVCenter) }
func capC(text string) *qt6.QLabel { return lbl(text, capQSS, qt6.AlignCenter) }

// Colored status labels.
func green(text string) *qt6.QLabel { return lbl(text, greenQSS, qt6.AlignCenter) }
func gray(text string) *qt6.QLabel  { return lbl(text, grayQSS, qt6.AlignCenter) }
func red(text string) *qt6.QLabel   { return lbl(text, redQSS, qt6.AlignCenter) }

// Table header / data cells.
func th(text string, align qt6.AlignmentFlag) *qt6.QLabel { return lbl(text, thQSS, align) }
func td(text string, align qt6.AlignmentFlag) *qt6.QLabel { return lbl(text, tdQSS, align) }

// featLbl renders a CPU feature flag colored by state (0 unsupported/gray,
// 1 supported/green, 2 special/red).
func featLbl(text string, state int) *qt6.QLabel {
	s := grayQSS
	switch state {
	case 1:
		s = greenQSS
	case 2:
		s = redQSS
	}
	return lbl(text, s, qt6.AlignLeft|qt6.AlignVCenter)
}

// combo builds a styled read-only-looking QComboBox.
func combo(items []string, idx int) *qt6.QComboBox {
	c := qt6.NewQComboBox2()
	for _, it := range items {
		c.AddItem(it)
	}
	if idx >= 0 {
		c.SetCurrentIndex(idx)
	}
	return c
}

// group builds a titled QGroupBox.
func group(title string) *qt6.QGroupBox { return qt6.NewQGroupBox3(title) }

// frame builds a bordered container used to wrap tables.
func frame() *qt6.QFrame {
	f := qt6.NewQFrame2()
	f.SetStyleSheet("background-color:#04100f; border:1px solid #2f6d6d;")
	return f
}

// vbox / hbox / grid build layouts with tight, consistent spacing.
func vbox(sp int) *qt6.QVBoxLayout {
	v := qt6.NewQVBoxLayout2()
	v.SetContentsMargins(0, 0, 0, 0)
	v.SetSpacing(sp)
	return v
}

func hbox(sp int) *qt6.QHBoxLayout {
	h := qt6.NewQHBoxLayout2()
	h.SetContentsMargins(0, 0, 0, 0)
	h.SetSpacing(sp)
	return h
}

func grid() *qt6.QGridLayout {
	g := qt6.NewQGridLayout2()
	g.SetContentsMargins(0, 0, 0, 0)
	g.SetHorizontalSpacing(5)
	g.SetVerticalSpacing(5)
	return g
}

// wrap puts a layout inside a fresh QWidget so it can be added to a box layout
// with a stretch factor.
func wrap(l *qt6.QLayout) *qt6.QWidget {
	w := qt6.NewQWidget2()
	w.SetLayout(l)
	return w
}
