package main

import (
	"github.com/gdamore/tcell/v2"
)

// Colors - Dracula theme (modified with blue primary)
var (
	ColorPrimary     = tcell.NewRGBColor(139, 233, 253)  // Blue (dracula cyan/blue)
	ColorSuccess     = tcell.NewRGBColor(80, 250, 123)   // Green (dracula green)
	ColorWarning     = tcell.NewRGBColor(241, 250, 140)  // Yellow (dracula yellow)
	ColorDanger      = tcell.NewRGBColor(255, 85, 85)    // Red (dracula red)
	ColorMuted       = tcell.NewRGBColor(98, 114, 164)   // Comment (dracula comment)
	ColorBackground  = tcell.NewRGBColor(40, 42, 54)     // Background (dracula bg)
	ColorForeground  = tcell.NewRGBColor(248, 248, 242)  // Foreground (dracula fg)
	ColorSelection   = tcell.NewRGBColor(68, 71, 90)     // Current Line (dracula current line)
	ColorAccent      = tcell.NewRGBColor(189, 147, 249)  // Purple (dracula purple)
	ColorOrange      = tcell.NewRGBColor(255, 184, 108)  // Orange (dracula orange)
	ColorPink        = tcell.NewRGBColor(255, 121, 198)  // Pink (dracula pink)
)

// Styles
var (
	StyleDefault     = tcell.StyleDefault.Foreground(ColorForeground).Background(ColorBackground)
	StylePrimary     = tcell.StyleDefault.Foreground(ColorPrimary).Bold(true)
	StyleSuccess     = tcell.StyleDefault.Foreground(ColorSuccess)
	StyleWarning     = tcell.StyleDefault.Foreground(ColorWarning)
	StyleDanger      = tcell.StyleDefault.Foreground(ColorDanger)
	StyleMuted       = tcell.StyleDefault.Foreground(ColorMuted)
	StyleHighlight   = tcell.StyleDefault.Foreground(ColorPrimary).Background(ColorSelection).Bold(true)
	StyleHeader      = tcell.StyleDefault.Foreground(ColorPrimary).Bold(true)
	StyleAccent      = tcell.StyleDefault.Foreground(ColorAccent)
)

// Color tags for tview dynamic colors (Dracula hex values)
const (
	TagPrimary       = "[#8be9fd]"  // Dracula cyan/blue
	TagSuccess       = "[#50fa7b]"  // Dracula green
	TagWarning       = "[#f1fa8c]"  // Dracula yellow
	TagDanger        = "[#ff5555]"  // Dracula red
	TagMuted         = "[#6272a4]"  // Dracula comment
	TagAccent        = "[#bd93f9]"  // Dracula purple
	TagReset         = "[::-]"
	TagBold          = "[::b]"
)

// Unicode characters
const (
	CharDivider      = '─'
	CharVertical     = '│'
	CharCornerTL     = '┌'
	CharCornerTR     = '┐'
	CharCornerBL     = '└'
	CharCornerBR     = '┘'
	CharBullet       = '•'
	CharArrowRight   = '→'
	CharArrowLeft    = '←'
	CharArrowUp      = '↑'
	CharArrowDown    = '↓'
	CharCheck        = '✓'
	CharCross        = '✗'
	CharStar         = '★'
)

// getStatusColor returns the appropriate color for a status
func getStatusColor(status string) tcell.Color {
	switch status {
	case "running":
		return ColorSuccess
	case "creating", "updating":
		return ColorWarning
	case "error", "failed":
		return ColorDanger
	default:
		return ColorMuted
	}
}