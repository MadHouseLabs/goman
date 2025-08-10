package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"
)

// ViewportStyle defines the viewport UI styling
type ViewportStyle struct {
	Width  int
	Height int
}

// Status types
type StatusType string

const (
	StatusReady     StatusType = "ready"
	StatusSettingUp StatusType = "setting_up"
	StatusError     StatusType = "error"
	StatusWarning   StatusType = "warning"
)

// Colors for status
var (
	ColorGreen   = lipgloss.Color("#10b981")
	ColorYellow  = lipgloss.Color("#f59e0b")
	ColorRed     = lipgloss.Color("#ef4444")
	ColorWhite   = lipgloss.Color("#ffffff")
	ColorGray    = lipgloss.Color("#6b7280")
	ColorBorder  = lipgloss.Color("#27272a")
	ColorPrimary = lipgloss.Color("#3b82f6") // Blue primary color
)

// RenderViewport renders the main viewport with title, content, and status
func RenderViewport(width, height int, content string, status StatusType, statusMsg string) string {
	// Title bar
	titleStyle := lipgloss.NewStyle().
		Foreground(ColorWhite).
		Bold(true).
		Padding(0, 1)
	
	title := titleStyle.Render("CLUSTERS")
	
	// Title separator
	separator := strings.Repeat("─", width)
	sepStyle := lipgloss.NewStyle().Foreground(ColorBorder)
	
	// Content area (calculate available height)
	contentHeight := height - 4 // Subtract title(1) + separator(1) + status(1) + spacing(1)
	contentStyle := lipgloss.NewStyle().
		Width(width).
		Height(contentHeight).
		Padding(1, 2)
	
	// Status bar
	var statusColor lipgloss.Color
	var statusText string
	
	switch status {
	case StatusReady:
		statusColor = ColorGreen
		statusText = "● Ready"
	case StatusSettingUp:
		statusColor = ColorYellow
		statusText = "◐ Setting up"
	case StatusError:
		statusColor = ColorRed
		statusText = "✗ Error"
	case StatusWarning:
		statusColor = ColorYellow
		statusText = "⚠ Warning"
	default:
		statusColor = ColorGray
		statusText = "○ Unknown"
	}
	
	if statusMsg != "" {
		statusText = statusText + ": " + statusMsg
	}
	
	statusStyle := lipgloss.NewStyle().
		Foreground(statusColor).
		Width(width).
		Padding(0, 1)
	
	// Build the viewport
	viewport := lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		sepStyle.Render(separator),
		contentStyle.Render(content),
		statusStyle.Render(statusText),
	)
	
	return viewport
}

// RenderEmptyViewport renders viewport with no clusters message
func RenderEmptyViewport(width, height int, status StatusType, statusMsg string) string {
	emptyMsg := lipgloss.NewStyle().
		Foreground(ColorGray).
		Italic(true).
		Render("No clusters found. Press 'c' to create a new cluster.")
	
	return RenderViewport(width, height, emptyMsg, status, statusMsg)
}

// RenderListWithZones renders a list with mouse zones for clicking
func RenderListWithZones(items []string, selectedIndex int) string {
	var result strings.Builder
	
	for i, item := range items {
		indicator := "  "
		if i == selectedIndex {
			indicator = "▸ "
		}
		
		line := indicator + item
		
		// Wrap each item in a mouse zone
		zoneID := fmt.Sprintf("list_item_%d", i)
		clickableLine := zone.Mark(zoneID, line)
		
		result.WriteString(clickableLine)
		if i < len(items)-1 {
			result.WriteString("\n")
		}
	}
	
	return result.String()
}

// RenderConfirmationDialog renders a confirmation dialog box
func RenderConfirmationDialog(title, message, warning, options string) string {
	dialogStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorYellow).
		Padding(1, 2).
		Width(60)

	titleStyle := lipgloss.NewStyle().
		Foreground(ColorYellow).
		Bold(true).
		MarginBottom(1)

	messageStyle := lipgloss.NewStyle().
		Foreground(ColorWhite).
		MarginBottom(1)

	warningStyle := lipgloss.NewStyle().
		Foreground(ColorRed).
		Italic(true).
		MarginBottom(1)

	optionsStyle := lipgloss.NewStyle().
		Foreground(ColorGray).
		MarginTop(1)

	content := lipgloss.JoinVertical(
		lipgloss.Left,
		titleStyle.Render(title),
		messageStyle.Render(message),
		warningStyle.Render(warning),
		optionsStyle.Render(options),
	)

	return dialogStyle.Render(content)
}