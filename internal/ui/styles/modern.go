package styles

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Modern, minimal color palette
var (
	// Minimalist colors
	ModernBg       = lipgloss.Color("#0a0a0a")
	ModernFg       = lipgloss.Color("#fafafa")
	ModernMuted    = lipgloss.Color("#a1a1aa")
	ModernAccent   = lipgloss.Color("#3b82f6")
	ModernSuccess  = lipgloss.Color("#10b981")
	ModernError    = lipgloss.Color("#ef4444")
	ModernBorder   = lipgloss.Color("#27272a")
	ModernInputBg  = lipgloss.Color("#18181b")
)

// Modern styles - clean and minimal
var (
	ModernContainer = lipgloss.NewStyle().
		Background(ModernBg).
		Padding(3, 5)

	ModernTitle = lipgloss.NewStyle().
		Foreground(ModernFg).
		Bold(true).
		MarginBottom(2)

	ModernLabel = lipgloss.NewStyle().
		Foreground(ModernMuted).
		MarginBottom(1)

	ModernActiveLabel = lipgloss.NewStyle().
		Foreground(ModernFg).
		MarginBottom(1)

	ModernInput = lipgloss.NewStyle().
		Foreground(ModernFg).
		Background(ModernInputBg).
		Padding(0, 2).
		Width(40).
		Height(1)

	ModernFocusedInput = lipgloss.NewStyle().
		Foreground(ModernFg).
		Background(ModernInputBg).
		BorderStyle(lipgloss.NormalBorder()).
		BorderLeft(true).
		BorderForeground(ModernMuted).
		Padding(0, 2).
		Width(40).
		Height(1)

	ModernButton = lipgloss.NewStyle().
		Foreground(ModernFg).
		Background(ModernInputBg).
		Padding(1, 4)

	ModernButtonFocused = lipgloss.NewStyle().
		Foreground(ModernFg).
		Background(ModernInputBg).
		Padding(1, 4).
		BorderStyle(lipgloss.NormalBorder()).
		BorderLeft(true).
		BorderForeground(ModernMuted)

	ModernHelp = lipgloss.NewStyle().
		Foreground(ModernMuted).
		MarginTop(2)

	ModernErrorStyle = lipgloss.NewStyle().
		Foreground(ModernError).
		MarginTop(1)
)

// RenderModernForm renders a clean, modern form
func RenderModernForm(title string, fields []ModernField, focusIndex int, errors map[int]string) string {
	var b strings.Builder

	// Title
	b.WriteString(ModernTitle.Render(title))
	b.WriteString("\n\n\n")

	// Fields
	for i, field := range fields {
		// Label
		label := field.Label
		if field.Required {
			label += " *"
		}
		
		// Always use muted color for labels
		b.WriteString(ModernLabel.Render(label))
		b.WriteString("\n")

		// Input
		inputStyle := ModernInput
		indicator := "  "
		if i == focusIndex {
			inputStyle = ModernFocusedInput
			indicator = "▸ "
		}

		value := field.Value
		if value == "" && i != focusIndex {
			value = field.Placeholder
		}

		b.WriteString(indicator)
		b.WriteString(inputStyle.Render(value))
		
		// Error
		if err, ok := errors[i]; ok && err != "" {
			b.WriteString("\n")
			b.WriteString(ModernErrorStyle.Render("  ↳ " + err))
		}
		
		if i < len(fields)-1 {
			b.WriteString("\n\n")
		} else {
			b.WriteString("\n")
		}
	}

	return b.String()
}

// ModernField represents a form field
type ModernField struct {
	Label       string
	Value       string
	Placeholder string
	Required    bool
}

// RenderModernButton renders a modern button
func RenderModernButton(text string, focused bool) string {
	if focused {
		return ModernButtonFocused.Render(text)
	}
	return ModernButton.Render(text)
}

// RenderModernHelp renders help text
func RenderModernHelp(items [][]string) string {
	var parts []string
	for _, item := range items {
		key := lipgloss.NewStyle().Foreground(ModernFg).Render(item[0])
		desc := lipgloss.NewStyle().Foreground(ModernMuted).Render(item[1])
		parts = append(parts, fmt.Sprintf("%s %s", key, desc))
	}
	return ModernHelp.Render(strings.Join(parts, "  •  "))
}

// RenderModernList renders a modern list view
func RenderModernList(title string, headers []string, rows [][]string, selectedIndex int) string {
	var b strings.Builder

	// Title
	b.WriteString(ModernTitle.Render(title))
	b.WriteString("\n\n")

	if len(rows) == 0 {
		emptyMsg := lipgloss.NewStyle().
			Foreground(ModernMuted).
			Italic(true).
			Render("No clusters found. Press 'c' to create a new cluster.")
		return b.String() + emptyMsg
	}

	// Headers
	headerStyle := lipgloss.NewStyle().
		Foreground(ModernMuted).
		Bold(true)
	
	for i, header := range headers {
		width := getColumnWidth(i)
		b.WriteString(headerStyle.Width(width).Render(header))
		b.WriteString("  ")
	}
	b.WriteString("\n")
	
	// Separator
	b.WriteString(lipgloss.NewStyle().
		Foreground(ModernBorder).
		Render(strings.Repeat("─", 80)))
	b.WriteString("\n")

	// Rows
	for i, row := range rows {
		indicator := "  "
		rowStyle := lipgloss.NewStyle().Foreground(ModernFg)
		
		if i == selectedIndex {
			indicator = "▸ "
			rowStyle = rowStyle.Foreground(ModernFg)
		}
		
		b.WriteString(indicator)
		
		for j, cell := range row {
			width := getColumnWidth(j)
			// Special styling for status column
			if j == 1 { // Status column
				cell = getStatusIcon(cell) + " " + cell
			}
			b.WriteString(rowStyle.Width(width).Render(cell))
			b.WriteString("  ")
		}
		b.WriteString("\n")
	}

	return b.String()
}

func getColumnWidth(index int) int {
	widths := []int{20, 12, 8, 10, 12, 10}
	if index < len(widths) {
		return widths[index]
	}
	return 10
}

func getStatusIcon(status string) string {
	switch strings.ToLower(status) {
	case "running":
		return "●"
	case "pending", "creating":
		return "◐"
	case "stopped":
		return "○"
	case "error", "failed":
		return "✗"
	default:
		return "·"
	}
}