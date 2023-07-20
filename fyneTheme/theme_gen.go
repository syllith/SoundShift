package fyneTheme

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

type CustomTheme struct{}

func (CustomTheme) Color(c fyne.ThemeColorName, v fyne.ThemeVariant) color.Color {
	switch c {
	case theme.ColorNameInputBorder:
		return color.NRGBA{R: 0x20, G: 0x25, B: 0x30, A: 0xff}
	case theme.ColorNameMenuBackground:
		return color.NRGBA{R: 0x20, G: 0x25, B: 0x30, A: 0xff}
	case theme.ColorNameOverlayBackground:
		return color.NRGBA{R: 0x20, G: 0x25, B: 0x30, A: 0xff}
	case theme.ColorNameSeparator:
		return color.NRGBA{R: 0x20, G: 0x25, B: 0x30, A: 0xff}
	case theme.ColorNameBackground:
		return color.NRGBA{R: 0x20, G: 0x25, B: 0x30, A: 0xff} // darker background
	case theme.ColorNameButton:
		return color.NRGBA{R: 0x3f, G: 0x7a, B: 0xc3, A: 0xff} // softer button color
	case theme.ColorNameDisabledButton:
		return color.NRGBA{R: 0x39, G: 0x38, B: 0x38, A: 0xff}
	case theme.ColorNameDisabled:
		return color.NRGBA{R: 0xaf, G: 0xb3, B: 0xb1, A: 0xff} // higher contrast for disabled text
	case theme.ColorNameError:
		return color.NRGBA{R: 0xd4, G: 0x33, B: 0x26, A: 0xff} // darker error color
	case theme.ColorNameFocus:
		return color.NRGBA{R: 0x21, G: 0x96, B: 0xf3, A: 0x7f}
	case theme.ColorNameForeground:
		return color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
	case theme.ColorNameHover:
		return color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0x1f} // more subtle hover effect
	case theme.ColorNameInputBackground:
		return color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0x29} // darker input background
	case theme.ColorNamePlaceHolder:
		return color.NRGBA{R: 0x92, G: 0x92, B: 0x92, A: 0xff} // darker placeholder text
	case theme.ColorNamePressed:
		return color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0x66}
	case theme.ColorNamePrimary:
		return color.NRGBA{R: 0x21, G: 0x86, B: 0xe3, A: 0xff} // softer primary color
	case theme.ColorNameScrollBar:
		return color.NRGBA{R: 0x0, G: 0x0, B: 0x0, A: 0xa9} // more visible scroll bar
	case theme.ColorNameShadow:
		return color.NRGBA{R: 0x0, G: 0x0, B: 0x0, A: 0x24} // darker shadow for more depth
	default:
		return theme.DefaultTheme().Color(c, v)
	}
}

func (CustomTheme) Font(s fyne.TextStyle) fyne.Resource {
	if s.Monospace {
		return theme.DefaultTheme().Font(s)
	}
	if s.Bold {
		if s.Italic {
			return theme.DefaultTheme().Font(s)
		}
		return fontLexendMediumTtf
	}
	if s.Italic {
		return theme.DefaultTheme().Font(s)
	}
	return fontLexendTtf
}

func (CustomTheme) Icon(n fyne.ThemeIconName) fyne.Resource {
	return theme.DefaultTheme().Icon(n)
}

func (CustomTheme) Size(s fyne.ThemeSizeName) float32 {
	switch s {
	case theme.SizeNameCaptionText:
		return 11
	case theme.SizeNameInlineIcon:
		return 20
	case theme.SizeNamePadding:
		return 5 // slightly more padding
	case theme.SizeNameScrollBar:
		return 16
	case theme.SizeNameScrollBarSmall:
		return 3
	case theme.SizeNameSeparatorThickness:
		return 1
	case theme.SizeNameText:
		return 12
	case theme.SizeNameInputBorder:
		return 2 // thicker input border
	default:
		return theme.DefaultTheme().Size(s)
	}
}
