package main

import "github.com/mappu/miqt/qt6"

// appStyle is the global stylesheet: the dark HWiNFO-like theme (black
// background, light text, teal group-box borders). Per-widget "classes"
// (fields, captions, table cells, status colors) are the *QSS constants
// below, applied via SetStyleSheet.
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
QPushButton {
    background-color: #0b0f10;
    border: 1px solid #2f6d6d;
    color: #f0f2f2;
    padding: 3px 12px;
}
QPushButton:hover { background-color: #13201f; }
QPushButton:pressed { background-color: #1f8a8a; color: #ffffff; }
QPushButton:checked {
    background-color: #123f12;
    border: 1px solid #45a545;
    color: #3ac83a;
}
QTreeWidget {
    background-color: #000000;
    alternate-background-color: #0a0d0d;
    border: 1px solid #2f6d6d;
    color: #e6e9e9;
}
QTreeWidget::item { padding: 1px 4px; }
QTreeWidget::item:selected { background-color: #13332f; color: #ffffff; }
QHeaderView::section {
    background-color: #13201f;
    color: #ffffff;
    padding: 3px 6px;
    border: none;
    border-right: 1px solid #0a1414;
    border-bottom: 1px solid #2f6d6d;
}
QScrollBar:vertical {
    background: #060909; width: 12px; margin: 0;
}
QScrollBar::handle:vertical {
    background: #1f4d4d; min-height: 24px; border-radius: 3px;
}
QScrollBar::handle:vertical:hover { background: #2f6d6d; }
QScrollBar::add-line:vertical, QScrollBar::sub-line:vertical { height: 0; }
QScrollBar:horizontal {
    background: #060909; height: 12px; margin: 0;
}
QScrollBar::handle:horizontal {
    background: #1f4d4d; min-width: 24px; border-radius: 3px;
}
QScrollBar::add-line:horizontal, QScrollBar::sub-line:horizontal { width: 0; }
`

// Per-widget style "classes".
const (
	fieldQSS  = "background-color:#0b1011; border:1px solid #2f6d6d; color:#f2f4f4; padding:2px 5px;"
	capQSS    = "color:#dde1e1; background:transparent; padding:0px 2px; border:none;"
	thQSS     = "background-color:#13201f; color:#ffffff; padding:2px 6px; border:none; border-bottom:1px solid #2f6d6d;"
	tdQSS     = "color:#e2e6e6; background:transparent; padding:2px 6px; border:none;"
	chkQSS    = "color:#33d13a; background:transparent; border:none; font-size:13px; font-weight:bold;"
	greenQSS  = "color:#3ac83a; background:transparent; border:none;"
	grayQSS   = "color:#59615f; background:transparent; border:none;"
	redQSS    = "color:#e0463c; background:transparent; border:none;"
	statusQSS = "color:#9aa5a5; background:transparent; border:none; padding:0px 6px;"
)

// Value colors used in the sensor table.
var (
	colValue = qt6.NewQColor3(0xf2, 0xf4, 0xf4) // normal current value
	colDim   = qt6.NewQColor3(0x8a, 0x95, 0x95) // min/max/avg
	colGreen = qt6.NewQColor3(0x3a, 0xc8, 0x3a)
	colWarn  = qt6.NewQColor3(0xe8, 0xa3, 0x3d) // above high limit
	colCrit  = qt6.NewQColor3(0xe0, 0x46, 0x3c) // above critical / firing
	colHead  = qt6.NewQColor3(0x35, 0xd0, 0xd0) // device group header text
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

// Colored status labels.
func green(text string) *qt6.QLabel { return lbl(text, greenQSS, qt6.AlignCenter) }

// Table header / data cells.
func th(text string, align qt6.AlignmentFlag) *qt6.QLabel { return lbl(text, thQSS, align) }
func td(text string, align qt6.AlignmentFlag) *qt6.QLabel { return lbl(text, tdQSS, align) }

// featLbl renders a CPU feature flag: green = supported, gray = absent,
// red = virtualization highlight (AMD-V / VT-x), HWiNFO style.
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

// combo builds a styled QComboBox.
func combo(items []string, idx int) *qt6.QComboBox {
	c := qt6.NewQComboBox2()
	for _, it := range items {
		c.AddItem(it)
	}
	if idx >= 0 && idx < len(items) {
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

// wrap puts a layout inside a fresh QWidget so it can be added to a box
// layout with a stretch factor.
func wrap(l *qt6.QLayout) *qt6.QWidget {
	w := qt6.NewQWidget2()
	w.SetLayout(l)
	return w
}
