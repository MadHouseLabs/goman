package styles

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// FormField represents a styled form field
type FormField struct {
	Label      string
	Value      string
	IsActive   bool
	IsFocused  bool
	IsRequired bool
	HasError   bool
	ErrorMsg   string
	Width      int
}

// Render renders a form field with proper styling
func (f FormField) Render() string {
	// Label
	labelStyle := LabelStyle
	if f.IsActive || f.IsFocused {
		labelStyle = ActiveLabelStyle
	}
	
	label := f.Label
	if f.IsRequired {
		label += " *"
	}
	labelText := labelStyle.Render(label)
	
	// Input text - apply border styling
	var inputText string
	if f.IsFocused {
		// For focused field, apply focus border
		inputText = FocusedInputStyle.Render(f.Value)
	} else {
		// For unfocused fields
		inputStyle := InputStyle
		if f.HasError {
			inputStyle = inputStyle.Copy().BorderForeground(Danger)
		}
		inputText = inputStyle.Render(f.Value)
	}
	
	// Combine with proper alignment
	field := lipgloss.JoinHorizontal(lipgloss.Top, labelText, inputText)
	
	// Add error message if present
	if f.HasError && f.ErrorMsg != "" {
		errorText := ErrorStyle.Copy().
			PaddingLeft(17). // Align with input
			Render("↳ " + f.ErrorMsg)
		field = lipgloss.JoinVertical(lipgloss.Left, field, errorText)
	}
	
	return field
}

// FormSection represents a section in a form
type FormSection struct {
	Title  string
	Fields []FormField
}

// Render renders a form section
func (s FormSection) Render() string {
	if s.Title == "" {
		return s.renderFields()
	}
	
	title := lipgloss.NewStyle().
		Foreground(Primary).
		Bold(true).
		MarginBottom(1).
		Render("── " + s.Title + " ──")
	
	return lipgloss.JoinVertical(lipgloss.Left, title, s.renderFields())
}

func (s FormSection) renderFields() string {
	var fields []string
	for i, field := range s.Fields {
		fields = append(fields, field.Render())
		// Add spacing between fields (but not after the last field)
		if i < len(s.Fields)-1 {
			fields = append(fields, "") // Empty line for spacing
		}
	}
	return lipgloss.JoinVertical(lipgloss.Left, fields...)
}

// StatusIcon returns an icon for a status
func StatusIcon(status string) string {
	switch strings.ToLower(status) {
	case "running", "active", "ready":
		return "●" // Green circle
	case "pending", "creating", "provisioning":
		return "◐" // Half circle
	case "stopped", "terminated":
		return "○" // Empty circle
	case "error", "failed":
		return "✗" // X mark
	case "deleting", "terminating":
		return "◌" // Dotted circle
	default:
		return "·" // Middle dot
	}
}

// ProgressBar creates a styled progress bar
func ProgressBar(percent float64, width int) string {
	if width <= 0 {
		width = 40
	}
	
	filled := int(float64(width) * percent / 100)
	if filled > width {
		filled = width
	}
	
	bar := strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
	
	style := lipgloss.NewStyle().Foreground(Primary)
	if percent >= 100 {
		style = style.Foreground(Success)
	}
	
	return style.Render(bar) + fmt.Sprintf(" %3.0f%%", percent)
}

// Table creates a styled table
type Table struct {
	Headers []string
	Rows    [][]string
	Widths  []int
}

// Render renders a styled table
func (t Table) Render() string {
	// Auto-calculate widths if not provided
	if len(t.Widths) == 0 {
		t.Widths = make([]int, len(t.Headers))
		for i, header := range t.Headers {
			t.Widths[i] = len(header)
		}
		for _, row := range t.Rows {
			for i, cell := range row {
				if i < len(t.Widths) && len(cell) > t.Widths[i] {
					t.Widths[i] = len(cell)
				}
			}
		}
	}
	
	// Render headers
	var headerCells []string
	for i, header := range t.Headers {
		width := 15 // default
		if i < len(t.Widths) {
			width = t.Widths[i]
		}
		cell := ListHeaderStyle.Copy().
			Width(width).
			Render(header)
		headerCells = append(headerCells, cell)
	}
	headerRow := lipgloss.JoinHorizontal(lipgloss.Top, headerCells...)
	
	// Render rows
	var rows []string
	for _, row := range t.Rows {
		var cells []string
		for i, cell := range row {
			width := 15
			if i < len(t.Widths) {
				width = t.Widths[i]
			}
			
			// Special styling for status columns
			style := ListItemStyle.Copy().Width(width)
			if i == 1 && len(t.Headers) > 1 && strings.Contains(strings.ToLower(t.Headers[1]), "status") {
				style = GetStatusStyle(cell).Width(width)
				cell = StatusIcon(cell) + " " + cell
			}
			
			cells = append(cells, style.Render(cell))
		}
		rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, cells...))
	}
	
	return lipgloss.JoinVertical(lipgloss.Left, 
		append([]string{headerRow}, rows...)...)
}

// Dialog creates a styled dialog box
type Dialog struct {
	Title   string
	Content string
	Type    string // "info", "success", "warning", "error"
	Width   int
}

// Render renders a styled dialog
func (d Dialog) Render() string {
	if d.Width == 0 {
		d.Width = 50
	}
	
	// Choose colors based on type
	var borderColor, titleBg lipgloss.Color
	switch d.Type {
	case "success":
		borderColor = Success
		titleBg = Success
	case "warning":
		borderColor = Warning
		titleBg = Warning
	case "error":
		borderColor = Danger
		titleBg = Danger
	default:
		borderColor = Primary
		titleBg = Primary
	}
	
	// Title
	titleStyle := lipgloss.NewStyle().
		Foreground(TextPrimary).
		Background(titleBg).
		Bold(true).
		Align(lipgloss.Center).
		Width(d.Width - 4).
		Padding(0, 1)
	
	// Content
	contentStyle := lipgloss.NewStyle().
		Foreground(TextPrimary).
		Padding(1, 2).
		Width(d.Width - 4)
	
	// Container
	containerStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Width(d.Width)
	
	content := lipgloss.JoinVertical(lipgloss.Center,
		titleStyle.Render(d.Title),
		contentStyle.Render(d.Content),
	)
	
	return containerStyle.Render(content)
}