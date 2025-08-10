package styles

import (
	"github.com/charmbracelet/lipgloss"
)

// Color Palette - Professional and modern
var (
	// Primary Colors
	Primary     = lipgloss.Color("#007AFF") // Professional blue
	PrimaryDark = lipgloss.Color("#0051D5") // Darker blue for emphasis
	
	// Secondary Colors
	Secondary = lipgloss.Color("#5856D6") // Purple accent
	Success   = lipgloss.Color("#34C759") // Green for success states
	Warning   = lipgloss.Color("#FF9500") // Orange for warnings
	Danger    = lipgloss.Color("#FF3B30") // Red for errors/danger
	Info      = lipgloss.Color("#5AC8FA") // Light blue for info
	
	// Neutral Colors
	Background     = lipgloss.Color("#0A0E1A") // Dark background
	Surface        = lipgloss.Color("#1C2333") // Card/surface background
	SurfaceLight   = lipgloss.Color("#2A3347") // Lighter surface
	Border         = lipgloss.Color("#3A445C") // Border color
	BorderFocused  = lipgloss.Color("#007AFF") // Focused border
	
	// Text Colors
	TextPrimary   = lipgloss.Color("#FFFFFF") // Primary text
	TextSecondary = lipgloss.Color("#8E95A5") // Secondary/muted text
	TextMuted     = lipgloss.Color("#6B7280") // Even more muted
	TextAccent    = lipgloss.Color("#60A5FA") // Accent text
	
	// Status Colors
	StatusRunning   = lipgloss.Color("#10B981") // Green
	StatusPending   = lipgloss.Color("#F59E0B") // Amber
	StatusStopped   = lipgloss.Color("#6B7280") // Gray
	StatusError     = lipgloss.Color("#EF4444") // Red
	StatusCreating  = lipgloss.Color("#3B82F6") // Blue
	StatusDeleting  = lipgloss.Color("#F97316") // Orange
)

// Base Styles
var (
	// Title Styles
	TitleStyle = lipgloss.NewStyle().
		Foreground(TextPrimary).
		Background(PrimaryDark).
		Bold(true).
		Padding(1, 3).
		MarginBottom(1)

	// Header Styles
	HeaderStyle = lipgloss.NewStyle().
		Foreground(Primary).
		Bold(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderBottom(true).
		BorderForeground(Border).
		PaddingBottom(1).
		MarginBottom(1)

	// Card/Container Styles
	CardStyle = lipgloss.NewStyle().
		Background(Surface).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(Border).
		Padding(1, 2)

	FocusedCardStyle = lipgloss.NewStyle().
		Background(Surface).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(BorderFocused).
		Padding(1, 2)

	// Form Styles
	FormContainerStyle = lipgloss.NewStyle().
		Background(Surface).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(Primary).
		Padding(2, 3).
		Width(55)

	FormTitleStyle = lipgloss.NewStyle().
		Foreground(TextPrimary).
		Background(Primary).
		Bold(true).
		Align(lipgloss.Center).
		Padding(0, 2).
		MarginBottom(2)

	LabelStyle = lipgloss.NewStyle().
		Foreground(TextSecondary).
		Width(15).
		Align(lipgloss.Right).
		PaddingRight(2)

	ActiveLabelStyle = lipgloss.NewStyle().
		Foreground(Primary).
		Bold(true).
		Width(15).
		Align(lipgloss.Right).
		PaddingRight(2)

	InputStyle = lipgloss.NewStyle().
		Foreground(TextPrimary).
		Background(Background).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(Border).
		Padding(0, 1).
		Width(30)

	FocusedInputStyle = lipgloss.NewStyle().
		Foreground(TextPrimary).
		Background(Background).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(Primary).
		Padding(0, 1).
		Width(30)

	// Button Styles
	ButtonStyle = lipgloss.NewStyle().
		Foreground(TextPrimary).
		Background(Primary).
		Padding(0, 3).
		MarginTop(1).
		Bold(true)

	ButtonHoverStyle = lipgloss.NewStyle().
		Foreground(Background).
		Background(Primary).
		Padding(0, 3).
		MarginTop(1).
		Bold(true)

	SecondaryButtonStyle = lipgloss.NewStyle().
		Foreground(TextSecondary).
		Background(Surface).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(Border).
		Padding(0, 3).
		MarginTop(1)

	// List Styles
	ListHeaderStyle = lipgloss.NewStyle().
		Foreground(TextSecondary).
		Bold(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderBottom(true).
		BorderForeground(Border).
		PaddingBottom(1)

	ListItemStyle = lipgloss.NewStyle().
		Foreground(TextPrimary).
		PaddingLeft(2)

	SelectedItemStyle = lipgloss.NewStyle().
		Foreground(TextPrimary).
		Background(PrimaryDark).
		PaddingLeft(2)

	// Status Badge Styles
	StatusBadgeStyle = lipgloss.NewStyle().
		Padding(0, 1).
		Bold(true)

	// Help Styles
	HelpStyle = lipgloss.NewStyle().
		Foreground(TextMuted).
		MarginTop(1)

	HelpKeyStyle = lipgloss.NewStyle().
		Foreground(Primary).
		Bold(true)

	HelpDescStyle = lipgloss.NewStyle().
		Foreground(TextSecondary)

	// Error/Success Styles
	ErrorStyle = lipgloss.NewStyle().
		Foreground(Danger).
		Bold(true)

	SuccessStyle = lipgloss.NewStyle().
		Foreground(Success).
		Bold(true)

	WarningStyle = lipgloss.NewStyle().
		Foreground(Warning).
		Bold(true)

	InfoStyle = lipgloss.NewStyle().
		Foreground(Info)

	// Spinner Style
	SpinnerStyle = lipgloss.NewStyle().
		Foreground(Primary)
)

// GetStatusStyle returns the appropriate style for a status
func GetStatusStyle(status string) lipgloss.Style {
	style := StatusBadgeStyle.Copy()
	
	switch status {
	case "running", "active", "ready":
		return style.Foreground(StatusRunning)
	case "pending", "creating", "provisioning":
		return style.Foreground(StatusCreating)
	case "stopped", "terminated":
		return style.Foreground(StatusStopped)
	case "error", "failed":
		return style.Foreground(StatusError)
	case "deleting", "terminating":
		return style.Foreground(StatusDeleting)
	default:
		return style.Foreground(TextSecondary)
	}
}

// RenderHelp renders help text with proper styling
func RenderHelp(items [][]string) string {
	var help string
	for i, item := range items {
		if i > 0 {
			help += "  "
		}
		help += HelpKeyStyle.Render("[" + item[0] + "]") + " " + HelpDescStyle.Render(item[1])
	}
	return HelpStyle.Render(help)
}