package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"
)

// FieldType represents the type of form field
type FieldType string

const (
	FieldTypeText     FieldType = "text"
	FieldTypeNumber   FieldType = "number"
	FieldTypeDropdown FieldType = "dropdown"
)

// FormField represents a single form field
type FormField struct {
	Label       string
	Value       string
	Placeholder string
	Required    bool
	Focused     bool
	Error       string
	Type        FieldType
	Options     []string // For dropdown fields
	DropdownOpen bool
	SelectedIndex int
	SearchTerm string
	FilteredOptions []string
}

// RenderClusterForm renders a cluster form (create/update) within the viewport
func RenderClusterForm(width, height int, isUpdate bool, fields []FormField, focusIndex int, status StatusType, statusMsg string) string {
	// Dynamic title based on mode
	title := "CREATE CLUSTER"
	if isUpdate {
		title = "UPDATE CLUSTER"
	}
	// Title bar
	titleStyle := lipgloss.NewStyle().
		Foreground(ColorWhite).
		Bold(true).
		Padding(0, 1)
	
	titleText := titleStyle.Render(title)
	
	// Title separator
	separator := strings.Repeat("─", width)
	sepStyle := lipgloss.NewStyle().Foreground(ColorBorder)
	
	// Form content
	var formContent strings.Builder
	
	for i, field := range fields {
		// Container for label and input with border
		containerStyle := lipgloss.NewStyle().
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(ColorBorder).
			Width(50)
		
		if i == focusIndex {
			// Highlighted border for focused field
			containerStyle = containerStyle.BorderForeground(ColorWhite)
		}
		
		// Label
		labelStyle := lipgloss.NewStyle().
			Foreground(ColorGray)
		
		if i == focusIndex {
			labelStyle = labelStyle.Foreground(ColorWhite)
		}
		
		label := field.Label
		if field.Required {
			label += " *"
		}
		
		// Input field styling
		inputStyle := lipgloss.NewStyle().Foreground(ColorWhite)
		value := field.Value
		
		// Show placeholder in muted color when empty
		if value == "" && !field.Focused {
			value = field.Placeholder
			inputStyle = inputStyle.Foreground(ColorGray)
		}
		
		// Add field type indicator
		var fieldIndicator string
		switch field.Type {
		case FieldTypeDropdown:
			fieldIndicator = " ▼" // Dropdown arrow
		case FieldTypeNumber:
			fieldIndicator = " #" // Number indicator
		default:
			fieldIndicator = ""
		}
		
		// Add cursor for focused field (only for text/number fields)
		if i == focusIndex && field.Type != FieldTypeDropdown {
			value = value + "│" // Add cursor
		}
		
		// Add type indicator at the end
		displayValue := value + fieldIndicator
		
		// Simple horizontal layout: label | input
		fieldContent := lipgloss.JoinHorizontal(
			lipgloss.Top,
			labelStyle.Width(15).Align(lipgloss.Right).Render(label+" "),
			inputStyle.Render(displayValue),
		)
		
		// Wrap field in a mouse zone for clicking
		zoneID := fmt.Sprintf("field_%d", i)
		clickableField := zone.Mark(zoneID, containerStyle.Render(fieldContent))
		formContent.WriteString(clickableField)
		
		// Error message if any
		if field.Error != "" {
			errorStyle := lipgloss.NewStyle().
				Foreground(ColorRed).
				PaddingLeft(2)
			formContent.WriteString("\n")
			formContent.WriteString(errorStyle.Render("↳ " + field.Error))
		}
		
		formContent.WriteString("\n")
	}
	
	// Button container for both buttons
	formContent.WriteString("\n")
	
	// Cancel button - smaller, secondary style, on the left
	cancelStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(ColorBorder).
		Foreground(ColorGray).
		Width(10).
		Align(lipgloss.Center)
	
	if focusIndex == len(fields) + 1 {
		cancelStyle = cancelStyle.BorderForeground(ColorWhite).Foreground(ColorWhite)
	}
	
	// Submit button - primary color
	submitStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(ColorPrimary).
		Foreground(ColorPrimary).
		Bold(true).
		Width(38). // Wider for primary action
		Align(lipgloss.Center)
	
	if focusIndex == len(fields) {
		submitStyle = submitStyle.BorderForeground(ColorWhite).Foreground(ColorWhite)
	}
	
	// Make buttons clickable with mouse zones
	cancelButton := zone.Mark("button_cancel", cancelStyle.Render("Cancel"))
	submitButton := zone.Mark("button_submit", submitStyle.Render("SUBMIT"))
	
	// Arrange buttons horizontally - Cancel on left, Submit on right
	buttonsRow := lipgloss.JoinHorizontal(
		lipgloss.Top,
		cancelButton,
		"  ", // Space between buttons
		submitButton,
	)
	
	formContent.WriteString(buttonsRow)
	
	// Wrap form content in a bordered container
	formContainer := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(ColorBorder).
		Padding(2, 3).
		Render(formContent.String())
	
	// Check if any dropdown is open and render it as overlay
	var dropdownOverlay string
	for i, field := range fields {
		if field.DropdownOpen && field.Type == FieldTypeDropdown && len(field.Options) > 0 {
			// Calculate position for dropdown (roughly where the field would be)
			dropdownStyle := lipgloss.NewStyle().
				BorderStyle(lipgloss.NormalBorder()).
				BorderForeground(ColorWhite).
				Background(lipgloss.Color("#1a1a1a")).
				Width(30).
				MaxHeight(8)
			
			var dropdownItems strings.Builder
			for idx, option := range field.Options {
				optionStyle := lipgloss.NewStyle().
					Width(28).
					Padding(0, 1)
				
				if idx == field.SelectedIndex {
					// Highlight selected option
					optionStyle = optionStyle.
						Background(ColorGray).
						Foreground(ColorWhite).
						Bold(true)
					dropdownItems.WriteString(optionStyle.Render("▸ " + option))
				} else {
					optionStyle = optionStyle.
						Foreground(ColorWhite)
					dropdownItems.WriteString(optionStyle.Render("  " + option))
				}
				
				if idx < len(field.Options)-1 {
					dropdownItems.WriteString("\n")
				}
			}
			
			// Position the dropdown below the focused field
			// This is approximate - in a real app you'd calculate exact position
			dropdownContent := dropdownStyle.Render(dropdownItems.String())
			
			// Add some spacing based on field index
			topPadding := 8 + (i * 3) // Rough calculation for field position
			dropdownWithPosition := lipgloss.NewStyle().
				MarginTop(topPadding).
				MarginLeft(width/2 - 15). // Center horizontally
				Render(dropdownContent)
			
			dropdownOverlay = dropdownWithPosition
			break // Only show one dropdown at a time
		}
	}
	
	// Center the form container
	contentHeight := height - 4 // Subtract title(1) + separator(1) + status(1) + spacing(1)
	
	// If dropdown is open, overlay it on top of the form
	var centeredForm string
	if dropdownOverlay != "" {
		// Render form first
		formLayer := lipgloss.Place(
			width,
			contentHeight,
			lipgloss.Center,
			lipgloss.Center,
			formContainer,
		)
		
		// Then overlay the dropdown
		centeredForm = lipgloss.JoinVertical(
			lipgloss.Left,
			formLayer[:len(formLayer)-contentHeight+2], // Take the top part
		) + dropdownOverlay
	} else {
		centeredForm = lipgloss.Place(
			width,
			contentHeight,
			lipgloss.Center,
			lipgloss.Center,
			formContainer,
		)
	}
	
	// Status bar
	var statusColor lipgloss.Color
	var statusText string
	
	switch status {
	case StatusReady:
		statusColor = ColorGreen
		statusText = "● Ready"
	case StatusSettingUp:
		statusColor = ColorYellow
		if isUpdate {
			statusText = "◐ Updating cluster..."
		} else {
			statusText = "◐ Creating cluster..."
		}
	case StatusError:
		statusColor = ColorRed
		statusText = "✗ Error"
	default:
		statusColor = ColorGray
		if isUpdate {
			statusText = "○ Ready to update"
		} else {
			statusText = "○ Ready to create"
		}
	}
	
	if statusMsg != "" {
		statusText = statusText + ": " + statusMsg
	}
	
	statusStyle := lipgloss.NewStyle().
		Foreground(statusColor).
		Width(width).
		Padding(0, 1)
	
	// Help text
	helpStyle := lipgloss.NewStyle().
		Foreground(ColorGray).
		Width(width).
		Align(lipgloss.Center)
	
	helpText := "Tab: next field • Enter: submit • Esc: cancel"
	
	// Build the viewport
	viewport := lipgloss.JoinVertical(
		lipgloss.Left,
		titleText,
		sepStyle.Render(separator),
		centeredForm,
		helpStyle.Render(helpText),
		statusStyle.Render(statusText),
	)
	
	return viewport
}

// SimpleFormField for easy form creation
func SimpleFormField(label, value, placeholder string, required, focused bool, err string) FormField {
	return FormField{
		Label:       label,
		Value:       value,
		Placeholder: placeholder,
		Required:    required,
		Focused:     focused,
		Error:       err,
		Type:        FieldTypeText, // Default to text
	}
}

// FormFieldWithType creates a form field with specific type
func FormFieldWithType(label, value, placeholder string, required, focused bool, err string, fieldType FieldType, options []string) FormField {
	return FormField{
		Label:       label,
		Value:       value,
		Placeholder: placeholder,
		Required:    required,
		Focused:     focused,
		Error:       err,
		Type:        fieldType,
		Options:     options,
	}
}

// FormFieldWithDropdown creates a form field with dropdown state
func FormFieldWithDropdown(label, value, placeholder string, required, focused bool, err string, fieldType FieldType, options []string, dropdownOpen bool, selectedIndex int) FormField {
	return FormField{
		Label:       label,
		Value:       value,
		Placeholder: placeholder,
		Required:    required,
		Focused:     focused,
		Error:       err,
		Type:        fieldType,
		Options:     options,
		DropdownOpen: dropdownOpen,
		SelectedIndex: selectedIndex,
	}
}

// FormFieldWithDropdownSearch creates a form field with dropdown and search state
func FormFieldWithDropdownSearch(label, value, placeholder string, required, focused bool, err string, fieldType FieldType, options []string, dropdownOpen bool, selectedIndex int, searchTerm string, filteredOptions []string) FormField {
	return FormField{
		Label:       label,
		Value:       value,
		Placeholder: placeholder,
		Required:    required,
		Focused:     focused,
		Error:       err,
		Type:        fieldType,
		Options:     options,
		DropdownOpen: dropdownOpen,
		SelectedIndex: selectedIndex,
		SearchTerm: searchTerm,
		FilteredOptions: filteredOptions,
	}
}

// RenderFormViewport is a convenience wrapper for RenderClusterForm (defaults to create mode)
func RenderFormViewport(width, height int, title string, fields []FormField, focusIndex int, status StatusType, statusMsg string) string {
	// For backward compatibility, use create mode
	return RenderClusterForm(width, height, false, fields, focusIndex, status, statusMsg)
}

// RenderClusterFormWithDropdown renders form with better dropdown handling
func RenderClusterFormWithDropdown(width, height int, isUpdate bool, fields []FormField, focusIndex int, openDropdownIndex int, status StatusType, statusMsg string) string {
	// Dynamic title based on mode
	title := "CREATE CLUSTER"
	if isUpdate {
		title = "UPDATE CLUSTER"
	}
	
	// Title bar
	titleStyle := lipgloss.NewStyle().
		Foreground(ColorWhite).
		Bold(true).
		Padding(0, 1)
	
	titleText := titleStyle.Render(title)
	
	// Title separator
	separator := strings.Repeat("─", width)
	sepStyle := lipgloss.NewStyle().Foreground(ColorBorder)
	
	// Main form content (without dropdown)
	var formContent strings.Builder
	
	for i, field := range fields {
		// Skip dropdown rendering in the main form
		field.DropdownOpen = false
		
		// Container for label and input with border
		containerStyle := lipgloss.NewStyle().
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(ColorBorder).
			Width(50)
		
		if i == focusIndex {
			containerStyle = containerStyle.BorderForeground(ColorWhite)
		}
		
		// Label
		labelStyle := lipgloss.NewStyle().
			Foreground(ColorGray)
		
		if i == focusIndex {
			labelStyle = labelStyle.Foreground(ColorWhite)
		}
		
		label := field.Label
		if field.Required {
			label += " *"
		}
		
		// Input field styling
		inputStyle := lipgloss.NewStyle().Foreground(ColorWhite)
		value := field.Value
		
		// Show placeholder in muted color when empty
		if value == "" && !field.Focused {
			value = field.Placeholder
			inputStyle = inputStyle.Foreground(ColorGray)
		}
		
		// Add field type indicator
		var fieldIndicator string
		var displayValue string
		
		// Normal display
		switch field.Type {
		case FieldTypeDropdown:
			if i == openDropdownIndex {
				fieldIndicator = " ▲"
			} else {
				fieldIndicator = " ▼"
			}
		case FieldTypeNumber:
			fieldIndicator = " #"
		default:
			fieldIndicator = ""
		}
		
		// Add cursor for focused field (only for text/number fields)
		if i == focusIndex && field.Type != FieldTypeDropdown {
			value = value + "│"
		}
		
		displayValue = value + fieldIndicator
		
		// Simple horizontal layout: label | input
		fieldLine := lipgloss.JoinHorizontal(
			lipgloss.Top,
			labelStyle.Width(15).Align(lipgloss.Right).Render(label+" "),
			inputStyle.Render(displayValue),
		)
		
		// Wrap field in a mouse zone for clicking
		zoneID := fmt.Sprintf("field_%d", i)
		
		
		// Normal field rendering - dropdowns show selected value
		clickableField := zone.Mark(zoneID, containerStyle.Render(fieldLine))
		formContent.WriteString(clickableField)
		
		// Error message if any
		if field.Error != "" {
			errorStyle := lipgloss.NewStyle().
				Foreground(ColorRed).
				PaddingLeft(2)
			formContent.WriteString("\n")
			formContent.WriteString(errorStyle.Render("↳ " + field.Error))
		}
		
		formContent.WriteString("\n")
	}
	
	// Buttons
	formContent.WriteString("\n")
	
	cancelStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(ColorBorder).
		Foreground(ColorGray).
		Width(10).
		Align(lipgloss.Center)
	
	if focusIndex == len(fields) + 1 {
		cancelStyle = cancelStyle.BorderForeground(ColorWhite).Foreground(ColorWhite)
	}
	
	submitStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(ColorPrimary).
		Foreground(ColorPrimary).
		Bold(true).
		Width(38).
		Align(lipgloss.Center)
	
	if focusIndex == len(fields) {
		submitStyle = submitStyle.BorderForeground(ColorWhite).Foreground(ColorWhite)
	}
	
	cancelButton := zone.Mark("button_cancel", cancelStyle.Render("Cancel"))
	submitButton := zone.Mark("button_submit", submitStyle.Render("SUBMIT"))
	
	buttonsRow := lipgloss.JoinHorizontal(
		lipgloss.Top,
		cancelButton,
		"  ",
		submitButton,
	)
	
	formContent.WriteString(buttonsRow)
	
	// Check if a dropdown is open and show list selector
	var listSelector string
	if openDropdownIndex >= 0 && openDropdownIndex < len(fields) {
		field := fields[openDropdownIndex]
		if field.DropdownOpen && len(field.FilteredOptions) > 0 {
			// Create a list selector box
			listStyle := lipgloss.NewStyle().
				BorderStyle(lipgloss.RoundedBorder()).
				BorderForeground(ColorWhite).
				Padding(1, 2).
				MarginTop(1)
			
			var listContent strings.Builder
			listContent.WriteString(lipgloss.NewStyle().
				Foreground(ColorWhite).
				Bold(true).
				Render("Select " + field.Label))
			listContent.WriteString("\n\n")
			
			// Show search term if any
			if field.SearchTerm != "" {
				listContent.WriteString(lipgloss.NewStyle().
					Foreground(ColorYellow).
					Render("Search: " + field.SearchTerm))
				listContent.WriteString("\n\n")
			}
			
			// Show options as a list
			for idx, option := range field.FilteredOptions {
				if idx == field.SelectedIndex {
					listContent.WriteString(lipgloss.NewStyle().
						Background(ColorGray).
						Foreground(ColorWhite).
						Bold(true).
						Width(30).
						Render("▸ " + option))
				} else {
					listContent.WriteString(lipgloss.NewStyle().
						Foreground(ColorWhite).
						Width(30).
						Render("  " + option))
				}
				if idx < len(field.FilteredOptions)-1 {
					listContent.WriteString("\n")
				}
			}
			
			listContent.WriteString("\n\n")
			listContent.WriteString(lipgloss.NewStyle().
				Foreground(ColorGray).
				Italic(true).
				Render("↑/↓: Navigate • Enter: Select • Esc: Cancel"))
			
			listSelector = listStyle.Render(listContent.String())
		}
	}
	
	// Wrap form in container
	formContainerStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(ColorBorder).
		Padding(2, 3)
	
	// Apply the container
	formContainer := formContainerStyle.Render(formContent.String())
	
	// Render final viewport - adjust height if dropdown is open
	contentHeight := height - 4
	
	// Show list selector if dropdown is open, otherwise show form
	var centeredForm string
	if listSelector != "" {
		// Show the list selector instead of the form
		centeredForm = lipgloss.Place(width, contentHeight, lipgloss.Center, lipgloss.Center, listSelector)
	} else {
		// Normal form display
		centeredForm = lipgloss.Place(width, contentHeight, lipgloss.Center, lipgloss.Center, formContainer)
	}
	
	// Help text
	helpStyle := lipgloss.NewStyle().
		Foreground(ColorGray).
		Width(width).
		Align(lipgloss.Center)
	
	helpContent := helpStyle.Render("Tab: next field • Enter: select/submit • Esc: cancel")
	
	// Status bar
	var statusColor lipgloss.Color
	var statusText string
	
	switch status {
	case StatusReady:
		statusColor = ColorGreen
		statusText = "● Ready"
	case StatusSettingUp:
		statusColor = ColorYellow
		if isUpdate {
			statusText = "◐ Updating cluster..."
		} else {
			statusText = "◐ Creating cluster..."
		}
	case StatusError:
		statusColor = ColorRed
		statusText = "✗ Error"
	default:
		statusColor = ColorGray
		if isUpdate {
			statusText = "○ Ready to update"
		} else {
			statusText = "○ Ready to create"
		}
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
		titleText,
		sepStyle.Render(separator),
		centeredForm,
		helpContent,
		statusStyle.Render(statusText),
	)
	
	return viewport
}