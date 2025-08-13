package components

import (
	"strings"
	
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// max returns the maximum of two integers
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// FormComponent represents a form with multiple components
type FormComponent struct {
	*BaseComponent
	components    []Component      // All form components
	labels        []string         // Labels for each component
	required      []bool           // Required flags
	sections      []string         // Section headers (empty string for no section)
	currentIndex  int              // Currently focused component
	errors        map[string]string
	onSubmit      func(map[string]interface{})
	onCancel      func()
	submitButton  *ButtonComponent
	cancelButton  *ButtonComponent
	focusOnButtons bool            // Whether focus is on buttons
	scrollOffset  int              // Vertical scroll offset
	height        int              // Fixed height for the form
	width         int              // Fixed width for the form
	title         string           // Form title (header)
	description   string           // Form description (header)
}

// NewForm creates a new form component
func NewForm(id string) *FormComponent {
	submitBtn := NewPrimaryButton("submit", "Submit")
	cancelBtn := NewSecondaryButton("cancel", "Cancel")
	
	return &FormComponent{
		BaseComponent:  NewBaseComponent(id),
		components:     []Component{},
		labels:         []string{},
		required:       []bool{},
		sections:       []string{},
		errors:         make(map[string]string),
		currentIndex:   0,
		submitButton:   submitBtn,
		cancelButton:   cancelBtn,
		focusOnButtons: false,
		scrollOffset:   0,
		height:         20,  // Default height
		width:          60,  // Default width
		title:          "",
		description:    "",
	}
}

// AddSection adds a section header to the form
func (f *FormComponent) AddSection(title string) {
	// Add a nil component to represent the section
	f.components = append(f.components, nil)
	f.labels = append(f.labels, "")
	f.required = append(f.required, false)
	f.sections = append(f.sections, title)
}

// AddInput adds a text input to the form
func (f *FormComponent) AddInput(id string, label string, placeholder string, required bool) {
	input := NewTextInput(f.id + "-" + id)
	input.SetLabel(label)
	input.SetPlaceholder(placeholder)
	// Set width (form width minus borders and padding)
	input.SetWidth(f.width - 6)
	f.components = append(f.components, input)
	f.labels = append(f.labels, label)
	f.required = append(f.required, required)
	f.sections = append(f.sections, "") // No section header for this component
}

// AddPasswordInput adds a password input to the form
func (f *FormComponent) AddPasswordInput(id string, label string, placeholder string, required bool) {
	input := NewPasswordInput(f.id + "-" + id)
	input.SetLabel(label)
	input.SetPlaceholder(placeholder)
	input.SetWidth(f.width - 6)
	f.components = append(f.components, input)
	f.labels = append(f.labels, label)
	f.required = append(f.required, required)
	f.sections = append(f.sections, "")
}

// AddEmailInput adds an email input to the form
func (f *FormComponent) AddEmailInput(id string, label string, placeholder string, required bool) {
	input := NewEmailInput(f.id + "-" + id)
	input.SetLabel(label)
	input.SetPlaceholder(placeholder)
	input.SetWidth(f.width - 6)
	f.components = append(f.components, input)
	f.labels = append(f.labels, label)
	f.required = append(f.required, required)
	f.sections = append(f.sections, "")
}

// AddTextArea adds a text area to the form
func (f *FormComponent) AddTextArea(id string, label string, placeholder string, required bool) {
	textarea := NewTextArea(f.id + "-" + id)
	textarea.SetLabel(label)
	textarea.SetPlaceholder(placeholder)
	textarea.SetDimensions(f.width - 6, 4)
	f.components = append(f.components, textarea)
	f.labels = append(f.labels, label)
	f.required = append(f.required, required)
	f.sections = append(f.sections, "")
}

// AddCheckbox adds a checkbox to the form
func (f *FormComponent) AddCheckbox(id string, label string) {
	checkbox := NewCheckbox(f.id + "-" + id, label)
	f.components = append(f.components, checkbox)
	f.labels = append(f.labels, "")
	f.required = append(f.required, false)
	f.sections = append(f.sections, "")
}

// AddCheckboxGroup adds a checkbox group to the form
func (f *FormComponent) AddCheckboxGroup(id string, label string, options map[string]string) {
	group := NewCheckboxGroup(f.id + "-" + id, label)
	for optID, optLabel := range options {
		group.AddCheckbox(optID, optLabel, false)
	}
	f.components = append(f.components, group)
	f.labels = append(f.labels, "")
	f.required = append(f.required, false)
	f.sections = append(f.sections, "")
}

// AddRadioGroup adds a radio group to the form
func (f *FormComponent) AddRadioGroup(id string, label string, options []string, required bool) {
	group := NewRadioGroup(f.id + "-" + id, label)
	for _, opt := range options {
		group.AddOption(opt, opt, opt)
	}
	f.components = append(f.components, group)
	f.labels = append(f.labels, "")
	f.required = append(f.required, required)
	f.sections = append(f.sections, "")
}

// Init initializes the form
func (f *FormComponent) Init() tea.Cmd {
	// Initialize all components
	var cmds []tea.Cmd
	for _, comp := range f.components {
		// Skip nil components (section headers)
		if comp != nil {
			cmds = append(cmds, comp.Init())
		}
	}
	
	// Focus first non-nil component
	for i, comp := range f.components {
		if comp != nil {
			f.focusComponent(i)
			f.currentIndex = i
			break
		}
	}
	
	// Set button handlers
	f.submitButton.SetOnClick(func() {
		f.submit()
	})
	
	f.cancelButton.SetOnClick(func() {
		if f.onCancel != nil {
			f.onCancel()
		}
	})
	
	// Enable mouse support for scrolling
	cmds = append(cmds, tea.EnableMouseCellMotion)
	
	return tea.Batch(cmds...)
}

// Update handles form messages
func (f *FormComponent) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	
	// Handle form navigation
	switch msg := msg.(type) {
	case tea.MouseMsg:
		// Handle mouse wheel scrolling
		if msg.Type == tea.MouseWheelUp {
			f.scrollOffset -= 3
			if f.scrollOffset < 0 {
				f.scrollOffset = 0
			}
			return f, nil
		} else if msg.Type == tea.MouseWheelDown {
			f.scrollOffset += 3
			f.constrainScrollOffset()
			return f, nil
		}
		
	case tea.KeyMsg:
		switch msg.String() {
		case "tab":
			f.nextField()
			f.updateScrollPosition()
			return f, nil
			
		case "shift+tab":
			f.prevField()
			f.updateScrollPosition()
			return f, nil
			
		case "up", "k":
			// Scroll up by 1 line
			f.scrollOffset--
			if f.scrollOffset < 0 {
				f.scrollOffset = 0
			}
			return f, nil
			
		case "down", "j":
			// Scroll down by 1 line
			f.scrollOffset++
			f.constrainScrollOffset()
			return f, nil
			
		case "pgup":
			// Scroll up by half the height
			f.scrollOffset -= f.getContentHeight() / 2
			if f.scrollOffset < 0 {
				f.scrollOffset = 0
			}
			return f, nil
			
		case "pgdown":
			// Scroll down by half the height
			f.scrollOffset += f.getContentHeight() / 2
			f.constrainScrollOffset()
			return f, nil
			
		case "enter":
			// If on submit button, submit form
			if f.focusOnButtons && f.submitButton.IsFocused() {
				f.submit()
				return f, nil
			}
			// Otherwise move to next field
			f.nextField()
			f.updateScrollPosition()
			return f, nil
			
		case "esc":
			if f.onCancel != nil {
				f.onCancel()
			}
			return f, nil
		}
	}
	
	// Update the current component or buttons
	if f.focusOnButtons {
		// Update buttons
		if f.submitButton.IsFocused() {
			model, cmd := f.submitButton.Update(msg)
			if btn, ok := model.(*ButtonComponent); ok {
				f.submitButton = btn
			}
			cmds = append(cmds, cmd)
		} else if f.cancelButton.IsFocused() {
			model, cmd := f.cancelButton.Update(msg)
			if btn, ok := model.(*ButtonComponent); ok {
				f.cancelButton = btn
			}
			cmds = append(cmds, cmd)
		}
	} else if f.currentIndex < len(f.components) && f.components[f.currentIndex] != nil {
		// Update current field (skip nil components)
		model, cmd := f.components[f.currentIndex].Update(msg)
		if comp, ok := model.(Component); ok {
			f.components[f.currentIndex] = comp
		}
		cmds = append(cmds, cmd)
	}
	
	return f, tea.Batch(cmds...)
}

// View renders the form
func (f *FormComponent) View() string {
	// Get content height for viewport
	contentHeight := f.getContentHeight()
	
	// Calculate actual content width (form width minus border and padding)
	// Border takes 2 chars (left + right), padding takes 2 chars (left + right)
	innerWidth := f.width - 4
	
	// Create professional header
	var header string
	if f.title != "" || f.description != "" {
		headerBox := lipgloss.NewStyle().
			Width(innerWidth).
			Padding(0, 1).
			Background(lipgloss.Color("#1F2937"))
		
		if f.title != "" && f.description != "" {
			titleStyle := lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#F3F4F6"))
			
			descStyle := lipgloss.NewStyle().
				Foreground(lipgloss.Color("#9CA3AF")).
				Italic(true)
			
			headerContent := titleStyle.Render(f.title) + "\n" + descStyle.Render(f.description)
			header = headerBox.Render(headerContent)
		} else if f.title != "" {
			titleStyle := lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#F3F4F6"))
			header = headerBox.Render(titleStyle.Render(f.title))
		} else {
			descStyle := lipgloss.NewStyle().
				Foreground(lipgloss.Color("#9CA3AF"))
			header = headerBox.Render(descStyle.Render(f.description))
		}
		
		// Add gradient separator
		separator := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#3B82F6")).
			Width(innerWidth).
			Render(strings.Repeat("━", innerWidth))
		header = header + "\n" + separator
	}
	
	// Render all form fields with sections
	var allFieldViews []string
	for i, comp := range f.components {
		// Check if this is a section header
		if i < len(f.sections) && f.sections[i] != "" {
			// Add section header with styling
			sectionStyle := lipgloss.NewStyle().
				Foreground(lipgloss.Color("#60A5FA")).
				Bold(true).
				MarginTop(1).
				MarginBottom(0)
			
			// Add separator above section (except for first section)
			if i > 0 {
				separatorStyle := lipgloss.NewStyle().
					Foreground(lipgloss.Color("#374151")).
					Width(innerWidth)
				allFieldViews = append(allFieldViews, separatorStyle.Render(strings.Repeat("─", innerWidth)))
			}
			
			allFieldViews = append(allFieldViews, sectionStyle.Render("▸ " + f.sections[i]))
			continue // Skip to next iteration as section headers don't have components
		}
		
		// Skip nil components (section placeholders)
		if comp == nil {
			continue
		}
		
		// Add required marker if needed
		if i < len(f.required) && f.required[i] {
			if textInput, ok := comp.(*TextInputComponent); ok {
				label := textInput.label
				if label != "" && !strings.HasSuffix(label, " *") {
					textInput.SetLabel(label + " *")
				}
			}
		}
		
		allFieldViews = append(allFieldViews, comp.View())
		
		// Add error if exists
		if err, ok := f.errors[comp.ID()]; ok && err != "" {
			errorStyle := lipgloss.NewStyle().
				Foreground(lipgloss.Color("#EF4444")).
				Italic(true).
				MarginLeft(2)
			allFieldViews = append(allFieldViews, errorStyle.Render("✗ "+err))
		}
	}
	
	// Join all field views
	fullFieldContent := lipgloss.JoinVertical(lipgloss.Left, allFieldViews...)
	
	// Split content into lines for scrolling
	contentLines := strings.Split(fullFieldContent, "\n")
	
	// Calculate visible lines based on scroll offset
	visibleLines := []string{}
	startLine := f.scrollOffset
	endLine := startLine + contentHeight
	
	if endLine > len(contentLines) {
		endLine = len(contentLines)
	}
	
	// Get the visible portion
	if startLine < len(contentLines) {
		visibleLines = contentLines[startLine:endLine]
	}
	
	// Join visible lines and create scrollable content area
	visibleContent := strings.Join(visibleLines, "\n")
	
	// Create viewport for content with exact height
	contentViewport := lipgloss.NewStyle().
		Width(innerWidth).
		Height(contentHeight).
		MaxHeight(contentHeight).
		MaxWidth(innerWidth).
		Render(visibleContent)
	
	// Create scrollbar if needed
	if len(contentLines) > contentHeight {
		// Calculate scrollbar properties
		scrollbarHeight := contentHeight
		thumbHeight := max(1, (contentHeight*contentHeight)/len(contentLines))
		scrollRange := len(contentLines) - contentHeight
		thumbPosition := 0
		if scrollRange > 0 {
			thumbPosition = (f.scrollOffset * (scrollbarHeight - thumbHeight)) / scrollRange
		}
		
		// Build scrollbar
		scrollbarStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#4B5563"))
		thumbStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#9CA3AF"))
		
		lines := strings.Split(contentViewport, "\n")
		for i := 0; i < len(lines) && i < scrollbarHeight; i++ {
			scrollChar := "│"
			if i >= thumbPosition && i < thumbPosition+thumbHeight {
				scrollChar = "█"
				scrollChar = thumbStyle.Render(scrollChar)
			} else {
				scrollChar = scrollbarStyle.Render(scrollChar)
			}
			
			// Pad line to full width and add scrollbar
			lineWidth := lipgloss.Width(lines[i])
			if lineWidth < innerWidth-2 {
				lines[i] = lines[i] + strings.Repeat(" ", innerWidth-2-lineWidth) + " " + scrollChar
			}
		}
		contentViewport = strings.Join(lines, "\n")
	}
	
	// Create professional footer
	footerSeparator := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#3B82F6")).
		Width(innerWidth).
		Render(strings.Repeat("━", innerWidth))
	
	// Create button group
	buttons := lipgloss.JoinHorizontal(
		lipgloss.Top,
		f.submitButton.View(),
		"  ",
		f.cancelButton.View(),
	)
	
	buttonContainer := lipgloss.NewStyle().
		Width(innerWidth).
		Align(lipgloss.Center).
		Render(buttons)
	
	// Combine all sections directly without extra padding
	var formContent string
	if header != "" {
		formContent = header + "\n" + contentViewport + "\n" + footerSeparator + "\n" + buttonContainer
	} else {
		formContent = contentViewport + "\n" + footerSeparator + "\n" + buttonContainer
	}
	
	// Apply form container styling
	// The container width should be exactly f.width including border
	containerStyle := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		Padding(0, 1, 0, 1). // Only horizontal padding, no vertical padding
		Width(f.width).
		BorderForeground(lipgloss.Color("#D1D5DB")) // Light gray border
	
	// Highlight border if focused
	if f.currentIndex < len(f.components) || f.focusOnButtons {
		containerStyle = containerStyle.
			BorderForeground(lipgloss.Color("#2563EB")) // Blue when focused
	}
	
	return containerStyle.Render(formContent)
}

// focusComponent focuses a specific component
func (f *FormComponent) focusComponent(index int) {
	// Blur all components
	for _, comp := range f.components {
		f.blurComponent(comp)
	}
	f.submitButton.Blur()
	f.cancelButton.Blur()
	
	// Focus the target component
	if index < len(f.components) {
		f.focusComponentAt(f.components[index])
	}
}

// Helper to focus different component types
func (f *FormComponent) focusComponentAt(comp Component) {
	switch c := comp.(type) {
	case *TextInputComponent:
		c.Focus()
	case *TextAreaComponent:
		c.Focus()
	case *CheckboxComponent:
		c.Focus()
	case *CheckboxGroupComponent:
		c.Focus()
	case *RadioGroupComponent:
		c.Focus()
	}
}

// Helper to blur different component types
func (f *FormComponent) blurComponent(comp Component) {
	switch c := comp.(type) {
	case *TextInputComponent:
		c.Blur()
	case *TextAreaComponent:
		c.Blur()
	case *CheckboxComponent:
		c.Blur()
	case *CheckboxGroupComponent:
		c.Blur()
	case *RadioGroupComponent:
		c.Blur()
	}
}

// nextField moves to the next field
func (f *FormComponent) nextField() {
	if f.focusOnButtons {
		// If on cancel button, wrap to first field
		if f.cancelButton.IsFocused() {
			f.cancelButton.Blur()
			f.focusOnButtons = false
			// Find first non-nil component
			for i, comp := range f.components {
				if comp != nil {
					f.currentIndex = i
					f.focusComponent(i)
					break
				}
			}
		} else if f.submitButton.IsFocused() {
			// Move from submit to cancel
			f.submitButton.Blur()
			f.cancelButton.Focus()
		}
	} else {
		// Blur current component if not nil
		if f.currentIndex < len(f.components) && f.components[f.currentIndex] != nil {
			f.blurComponent(f.components[f.currentIndex])
		}
		
		// Find next non-nil component
		found := false
		for i := f.currentIndex + 1; i < len(f.components); i++ {
			if f.components[i] != nil {
				f.currentIndex = i
				f.focusComponentAt(f.components[i])
				found = true
				break
			}
		}
		
		if !found {
			// Move to buttons
			f.focusOnButtons = true
			f.submitButton.Focus()
		}
	}
}

// prevField moves to the previous field
func (f *FormComponent) prevField() {
	if f.focusOnButtons {
		if f.submitButton.IsFocused() {
			// Move from submit button back to last non-nil field
			f.submitButton.Blur()
			f.focusOnButtons = false
			// Find last non-nil component
			for i := len(f.components) - 1; i >= 0; i-- {
				if f.components[i] != nil {
					f.currentIndex = i
					f.focusComponentAt(f.components[i])
					break
				}
			}
		} else if f.cancelButton.IsFocused() {
			// Move from cancel to submit
			f.cancelButton.Blur()
			f.submitButton.Focus()
		}
	} else {
		// Blur current component if not nil
		if f.currentIndex < len(f.components) && f.components[f.currentIndex] != nil {
			f.blurComponent(f.components[f.currentIndex])
		}
		
		// Find previous non-nil component
		found := false
		for i := f.currentIndex - 1; i >= 0; i-- {
			if f.components[i] != nil {
				f.currentIndex = i
				f.focusComponentAt(f.components[i])
				found = true
				break
			}
		}
		
		// If no previous component found, stay on current
		if !found && f.currentIndex < len(f.components) && f.components[f.currentIndex] != nil {
			f.focusComponentAt(f.components[f.currentIndex])
		}
	}
}

// GetValues returns all form values
func (f *FormComponent) GetValues() map[string]interface{} {
	values := make(map[string]interface{})
	
	for _, comp := range f.components {
		if comp == nil {
			continue // Skip section headers
		}
		switch c := comp.(type) {
		case *TextInputComponent:
			values[c.ID()] = c.Value()
		case *TextAreaComponent:
			values[c.ID()] = c.Value()
		case *CheckboxComponent:
			values[c.ID()] = c.IsChecked()
		case *CheckboxGroupComponent:
			values[c.ID()] = c.GetValues()
		case *RadioGroupComponent:
			values[c.ID()] = c.GetSelected()
		}
	}
	
	return values
}

// Validate validates all form fields
func (f *FormComponent) Validate() bool {
	valid := true
	f.errors = make(map[string]string)
	
	for i, comp := range f.components {
		if comp == nil {
			continue // Skip section headers
		}
		// Check required fields
		if i < len(f.required) && f.required[i] {
			switch c := comp.(type) {
			case *TextInputComponent:
				if c.Value() == "" {
					f.errors[c.ID()] = "This field is required"
					valid = false
				}
			case *TextAreaComponent:
				if c.Value() == "" {
					f.errors[c.ID()] = "This field is required"
					valid = false
				}
			case *RadioGroupComponent:
				if c.GetSelected() == "" {
					f.errors[c.ID()] = "Please select an option"
					valid = false
				}
			}
		}
	}
	
	return valid
}

// submit submits the form
func (f *FormComponent) submit() {
	if f.Validate() {
		if f.onSubmit != nil {
			f.onSubmit(f.GetValues())
		}
	}
}

// SetOnSubmit sets the submit handler
func (f *FormComponent) SetOnSubmit(handler func(map[string]interface{})) {
	f.onSubmit = handler
}

// SetOnCancel sets the cancel handler
func (f *FormComponent) SetOnCancel(handler func()) {
	f.onCancel = handler
}

// Reset resets the form
func (f *FormComponent) Reset() {
	for _, comp := range f.components {
		if comp == nil {
			continue // Skip section headers
		}
		switch c := comp.(type) {
		case *TextInputComponent:
			c.SetValue("")
		case *TextAreaComponent:
			c.SetValue("")
		case *CheckboxComponent:
			c.SetChecked(false)
		}
	}
	f.errors = make(map[string]string)
	f.scrollOffset = 0
	f.focusOnButtons = false
	
	// Find first non-nil component to focus
	for i, comp := range f.components {
		if comp != nil {
			f.currentIndex = i
			f.focusComponent(i)
			break
		}
	}
}

// SetDimensions sets the form dimensions
func (f *FormComponent) SetDimensions(width, height int) {
	f.width = width
	f.height = height
	
	// Update width of all existing components
	for _, comp := range f.components {
		if comp == nil {
			continue // Skip section headers
		}
		switch c := comp.(type) {
		case *TextInputComponent:
			c.SetWidth(width - 6)
		case *TextAreaComponent:
			c.SetDimensions(width - 6, 4)
		}
	}
}

// SetTitle sets the form title
func (f *FormComponent) SetTitle(title string) {
	f.title = title
}

// SetDescription sets the form description
func (f *FormComponent) SetDescription(description string) {
	f.description = description
}

// getContentHeight calculates the available height for scrollable content
func (f *FormComponent) getContentHeight() int {
	headerHeight := 0
	if f.title != "" && f.description != "" {
		headerHeight = 4 // Title + description + padding + separator
	} else if f.title != "" || f.description != "" {
		headerHeight = 3 // Single line + padding + separator
	}
	
	footerHeight := 2 // Separator + buttons only (no hints, no extra spacing)
	borderAndPadding := 2 // Only border (2), no vertical padding
	
	contentHeight := f.height - headerHeight - footerHeight - borderAndPadding
	if contentHeight < 1 {
		contentHeight = 1
	}
	
	return contentHeight
}

// updateScrollPosition updates scroll position to keep focused field visible
func (f *FormComponent) updateScrollPosition() {
	// Calculate approximate line position of current field
	linesPerField := 3 // Rough estimate
	currentFieldLine := f.currentIndex * linesPerField
	
	visibleHeight := f.getContentHeight()
	
	// If current field is above visible area, scroll up
	if currentFieldLine < f.scrollOffset {
		f.scrollOffset = currentFieldLine
	}
	
	// If current field is below visible area, scroll down
	if currentFieldLine >= f.scrollOffset + visibleHeight - 3 {
		f.scrollOffset = currentFieldLine - visibleHeight + 3
	}
	
	f.constrainScrollOffset()
}

// constrainScrollOffset ensures scroll offset is within valid bounds
func (f *FormComponent) constrainScrollOffset() {
	if f.scrollOffset < 0 {
		f.scrollOffset = 0
	}
	
	// Calculate max scroll based on content
	totalLines := len(f.components) * 3 // Rough estimate lines per component
	maxScroll := totalLines - f.getContentHeight()
	
	if maxScroll < 0 {
		maxScroll = 0
	}
	
	if f.scrollOffset > maxScroll {
		f.scrollOffset = maxScroll
	}
}