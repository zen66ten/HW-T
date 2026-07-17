package main

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// palette is the GPU-T-inspired color set. Custom widgets read the active
// palette at refresh time, so a theme switch takes effect within one tick.
type palette struct {
	dark bool

	background color.NRGBA
	box        color.NRGBA // value boxes / inputs
	boxStroke  color.NRGBA
	foreground color.NRGBA
	dim        color.NRGBA
	accent     color.NRGBA
	bar        color.NRGBA // the GPU-Z red
	barTrack   color.NRGBA
	separator  color.NRGBA
	button     color.NRGBA
	hover      color.NRGBA
	selection  color.NRGBA
	warn       color.NRGBA
	crit       color.NRGBA
	ok         color.NRGBA
}

// palLight mirrors the GPU-T light look: white boxes, gray hairlines, red
// bars on a paper-white track.
var palLight = palette{
	background: color.NRGBA{0xf4, 0xf4, 0xf5, 0xff},
	box:        color.NRGBA{0xff, 0xff, 0xff, 0xff},
	boxStroke:  color.NRGBA{0xc6, 0xc7, 0xcb, 0xff},
	foreground: color.NRGBA{0x1c, 0x1d, 0x1f, 0xff},
	dim:        color.NRGBA{0x63, 0x66, 0x6b, 0xff},
	accent:     color.NRGBA{0x2d, 0x7d, 0xd4, 0xff},
	bar:        color.NRGBA{0xdc, 0x1e, 0x1e, 0xff},
	barTrack:   color.NRGBA{0xff, 0xff, 0xff, 0xff},
	separator:  color.NRGBA{0xd9, 0xda, 0xdd, 0xff},
	button:     color.NRGBA{0xe9, 0xe9, 0xeb, 0xff},
	hover:      color.NRGBA{0xdd, 0xde, 0xe1, 0xff},
	selection:  color.NRGBA{0xbf, 0xd7, 0xf0, 0xff},
	warn:       color.NRGBA{0xa8, 0x6a, 0x00, 0xff},
	crit:       color.NRGBA{0xc4, 0x1a, 0x1a, 0xff},
	ok:         color.NRGBA{0x1d, 0x8e, 0x3a, 0xff},
}

var palDark = palette{
	dark:       true,
	background: color.NRGBA{0x17, 0x19, 0x1d, 0xff},
	box:        color.NRGBA{0x0e, 0x10, 0x13, 0xff},
	boxStroke:  color.NRGBA{0x33, 0x37, 0x3e, 0xff},
	foreground: color.NRGBA{0xdc, 0xdf, 0xe4, 0xff},
	dim:        color.NRGBA{0x8b, 0x93, 0xa0, 0xff},
	accent:     color.NRGBA{0x4c, 0x9c, 0xe6, 0xff},
	bar:        color.NRGBA{0xc8, 0x28, 0x28, 0xff},
	barTrack:   color.NRGBA{0x24, 0x26, 0x2b, 0xff},
	separator:  color.NRGBA{0x2a, 0x2e, 0x35, 0xff},
	button:     color.NRGBA{0x22, 0x25, 0x2a, 0xff},
	hover:      color.NRGBA{0x2c, 0x30, 0x37, 0xff},
	selection:  color.NRGBA{0x2a, 0x45, 0x63, 0xff},
	warn:       color.NRGBA{0xe6, 0xa2, 0x3c, 0xff},
	crit:       color.NRGBA{0xf0, 0x4a, 0x4a, 0xff},
	ok:         color.NRGBA{0x3c, 0xb4, 0x50, 0xff},
}

// pal is the active palette. GPU-T ships light-first; the toolbar toggle
// swaps it.
var pal = &palLight

// hwtTheme adapts the active palette for Fyne's stock widgets, with the
// compact GPU-T sizing.
type hwtTheme struct {
	p *palette
}

func (t *hwtTheme) Color(name fyne.ThemeColorName, _ fyne.ThemeVariant) color.Color {
	variant := theme.VariantLight
	if t.p.dark {
		variant = theme.VariantDark
	}
	switch name {
	case theme.ColorNameBackground:
		return t.p.background
	case theme.ColorNameInputBackground, theme.ColorNameHeaderBackground, theme.ColorNameOverlayBackground, theme.ColorNameMenuBackground:
		return t.p.box
	case theme.ColorNameForeground:
		return t.p.foreground
	case theme.ColorNamePlaceHolder, theme.ColorNameDisabled:
		return t.p.dim
	case theme.ColorNamePrimary, theme.ColorNameFocus, theme.ColorNameHyperlink:
		return t.p.accent
	case theme.ColorNameSeparator:
		return t.p.separator
	case theme.ColorNameButton:
		return t.p.button
	case theme.ColorNameHover:
		return t.p.hover
	case theme.ColorNameSelection:
		return t.p.selection
	case theme.ColorNameScrollBar:
		return t.p.boxStroke
	case theme.ColorNameError:
		return t.p.crit
	case theme.ColorNameWarning:
		return t.p.warn
	case theme.ColorNameSuccess:
		return t.p.ok
	case theme.ColorNameInputBorder:
		return t.p.boxStroke
	}
	return theme.DefaultTheme().Color(name, variant)
}

func (t *hwtTheme) Font(style fyne.TextStyle) fyne.Resource {
	return theme.DefaultTheme().Font(style)
}

func (t *hwtTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return theme.DefaultTheme().Icon(name)
}

func (t *hwtTheme) Size(name fyne.ThemeSizeName) float32 {
	switch name {
	case theme.SizeNamePadding:
		return 3
	case theme.SizeNameText:
		return 13
	case theme.SizeNameCaptionText:
		return 11
	case theme.SizeNameInnerPadding:
		return 4
	case theme.SizeNameScrollBar:
		return 10
	case theme.SizeNameScrollBarSmall:
		return 4
	case theme.SizeNameInputRadius, theme.SizeNameSelectionRadius:
		return 3
	}
	return theme.DefaultTheme().Size(name)
}
