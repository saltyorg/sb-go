package styles

import (
	"github.com/charmbracelet/lipgloss"
)

// Text colors
//
//goland:noinspection ALL
const (
	// Base ANSI colors
	ColorBlack   = "0"
	ColorRed     = "1"
	ColorGreen   = "2"
	ColorYellow  = "3"
	ColorBlue    = "4"
	ColorMagenta = "5"
	ColorCyan    = "6"
	ColorWhite   = "7"

	// Bright ANSI colors
	ColorBrightBlack   = "8"
	ColorBrightRed     = "9"
	ColorBrightGreen   = "10"
	ColorBrightYellow  = "11"
	ColorBrightBlue    = "12"
	ColorBrightMagenta = "13"
	ColorBrightCyan    = "14"
	ColorBrightWhite   = "15"

	// More specific color codes
	ColorDarkRed     = "160"
	ColorMediumGreen = "40"
	ColorLightBlue   = "39"
	ColorDimGray     = "240"
	ColorOrange      = "208"
	ColorPurple      = "93"
)

// Global style definitions
//
//goland:noinspection GoUnusedGlobalVariable
var (
	// Base styles - can be used as starting points
	DefaultStyle = lipgloss.NewStyle()

	// UI element styles
	HeaderStyle  = lipgloss.NewStyle().Bold(true)
	SuccessStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(ColorMediumGreen))
	ErrorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color(ColorDarkRed)).Bold(true)
	WarningStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(ColorYellow))
	InfoStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color(ColorLightBlue))
	DimStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color(ColorDimGray))

	// Specific use-case styles
	KeyStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color(ColorLightBlue))
	ValueStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color(ColorMediumGreen))
	HighlightStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(ColorBrightYellow))
)
