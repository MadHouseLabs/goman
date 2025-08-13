package components

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// RadioOption represents a single radio option
type RadioOption struct {
	ID    string
	Label string
	Value interface{}
}

// RadioGroupComponent represents a group of radio buttons
type RadioGroupComponent struct {
	*BaseComponent
	options      []RadioOption
	selectedID   string
	label        string
	focusedIndex int
	focused      bool
	disabled     bool
	onChange     func(string, interface{})
}

// NewRadioGroup creates a new radio group component
func NewRadioGroup(id string, label string) *RadioGroupComponent {
	return &RadioGroupComponent{
		BaseComponent: NewBaseComponent(id),
		options:       []RadioOption{},
		label:         label,
		focusedIndex:  0,
		focused:       false,
		disabled:      false,
	}
}

// AddOption adds a radio option to the group
func (r *RadioGroupComponent) AddOption(id string, label string, value interface{}) {
	r.options = append(r.options, RadioOption{
		ID:    id,
		Label: label,
		Value: value,
	})
}

// Init initializes the radio group
func (r *RadioGroupComponent) Init() tea.Cmd {
	// Select first option by default if none selected
	if r.selectedID == "" && len(r.options) > 0 {
		r.selectedID = r.options[0].ID
		r.state["selected"] = r.selectedID
		r.state["value"] = r.options[0].Value
	}
	// Auto-focus the radio group
	r.focused = true
	return nil
}

// Update handles radio group messages
func (r *RadioGroupComponent) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "down", "j", "tab":
			if r.focusedIndex < len(r.options)-1 {
				r.focusedIndex++
			} else {
				r.focusedIndex = 0 // Wrap around
			}
			return r, nil
			
		case "up", "k", "shift+tab":
			if r.focusedIndex > 0 {
				r.focusedIndex--
			} else {
				r.focusedIndex = len(r.options) - 1 // Wrap around
			}
			return r, nil
			
		case " ", "enter":
			if r.focusedIndex >= 0 && r.focusedIndex < len(r.options) {
				oldSelected := r.selectedID
				option := r.options[r.focusedIndex]
				r.selectedID = option.ID
				r.state["selected"] = r.selectedID
				r.state["value"] = option.Value
				
				if oldSelected != r.selectedID && r.onChange != nil {
					r.onChange(r.selectedID, option.Value)
				}
			}
			return r, nil
		}
	}
	
	return r, nil
}

// View renders the radio group
func (r *RadioGroupComponent) View() string {
	var views []string
	
	// Add label if provided
	if r.label != "" {
		labelStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#6B7280")).
			Bold(true)
		views = append(views, labelStyle.Render(r.label))
	}
	
	// Add radio options
	for i, option := range r.options {
		// Radio symbol
		radioMark := "( )"
		if option.ID == r.selectedID {
			radioMark = "(•)"
		}
		
		// Option style
		optionStyle := lipgloss.NewStyle().
			Padding(0, 1)
		
		// Apply focus/disabled styling
		if r.disabled {
			optionStyle = optionStyle.
				Foreground(lipgloss.Color("#6B7280"))
		} else if r.focused && i == r.focusedIndex {
			optionStyle = optionStyle.
				Foreground(lipgloss.Color("#3B82F6")).
				Bold(true)
		} else if option.ID == r.selectedID {
			optionStyle = optionStyle.
				Foreground(lipgloss.Color("#10B981"))
		} else {
			optionStyle = optionStyle.
				Foreground(lipgloss.Color("#9CA3AF"))
		}
		
		optionView := fmt.Sprintf("%s %s", radioMark, option.Label)
		views = append(views, optionStyle.Render(optionView))
	}
	
	// Create container with border
	containerStyle := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		Padding(0, 1).
		BorderForeground(lipgloss.Color("#9CA3AF"))
	
	// Highlight border when focused
	if r.focused {
		containerStyle = containerStyle.
			BorderForeground(lipgloss.Color("#3B82F6"))
	}
	
	content := strings.Join(views, "\n")
	return containerStyle.Render(content)
}

// SetSelected sets the selected option by ID
func (r *RadioGroupComponent) SetSelected(id string) {
	for _, option := range r.options {
		if option.ID == id {
			r.selectedID = id
			r.state["selected"] = id
			r.state["value"] = option.Value
			break
		}
	}
}

// GetSelected returns the selected option ID
func (r *RadioGroupComponent) GetSelected() string {
	return r.selectedID
}

// GetSelectedValue returns the value of the selected option
func (r *RadioGroupComponent) GetSelectedValue() interface{} {
	for _, option := range r.options {
		if option.ID == r.selectedID {
			return option.Value
		}
	}
	return nil
}

// GetState returns the state of the radio group
func (r *RadioGroupComponent) GetState() State {
	return r.state
}

// SetOnChange sets the change handler
func (r *RadioGroupComponent) SetOnChange(handler func(string, interface{})) {
	r.onChange = handler
}

// Focus focuses the radio group
func (r *RadioGroupComponent) Focus() {
	r.focused = true
}

// Blur unfocuses the radio group
func (r *RadioGroupComponent) Blur() {
	r.focused = false
}

// Enable enables the radio group
func (r *RadioGroupComponent) Enable() {
	r.disabled = false
}

// Disable disables the radio group
func (r *RadioGroupComponent) Disable() {
	r.disabled = true
}

// RadioButtonComponent represents a single radio button (for custom layouts)
type RadioButtonComponent struct {
	*BaseComponent
	label    string
	selected bool
	focused  bool
	disabled bool
	groupID  string
	value    interface{}
	onChange func(bool)
}

// NewRadioButton creates a new radio button component
func NewRadioButton(id string, label string, groupID string) *RadioButtonComponent {
	return &RadioButtonComponent{
		BaseComponent: NewBaseComponent(id),
		label:         label,
		selected:      false,
		focused:       false,
		disabled:      false,
		groupID:       groupID,
	}
}

// Init initializes the radio button
func (r *RadioButtonComponent) Init() tea.Cmd {
	return nil
}

// Update handles radio button messages
func (r *RadioButtonComponent) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if r.focused && !r.disabled {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case " ", "enter":
				if !r.selected {
					r.selected = true
					r.state["selected"] = true
					if r.onChange != nil {
						r.onChange(true)
					}
				}
				return r, nil
			}
		}
	}
	
	return r, nil
}

// View renders the radio button
func (r *RadioButtonComponent) View() string {
	// Radio symbol
	radioMark := "( )"
	if r.selected {
		radioMark = "(•)"
	}
	
	// Create style
	style := lipgloss.NewStyle().
		Padding(0, 1)
	
	// Apply focus/disabled styling
	if r.disabled {
		style = style.
			Foreground(lipgloss.Color("#6B7280"))
	} else if r.focused {
		style = style.
			Foreground(lipgloss.Color("#3B82F6")).
			Bold(true)
	} else if r.selected {
		style = style.
			Foreground(lipgloss.Color("#10B981"))
	} else {
		style = style.
			Foreground(lipgloss.Color("#9CA3AF"))
	}
	
	view := fmt.Sprintf("%s %s", radioMark, r.label)
	return style.Render(view)
}

// SetSelected sets the selected state
func (r *RadioButtonComponent) SetSelected(selected bool) {
	r.selected = selected
	r.state["selected"] = selected
}

// IsSelected returns whether the radio button is selected
func (r *RadioButtonComponent) IsSelected() bool {
	return r.selected
}

// SetValue sets the value associated with this radio button
func (r *RadioButtonComponent) SetValue(value interface{}) {
	r.value = value
	r.state["value"] = value
}

// GetValue returns the value associated with this radio button
func (r *RadioButtonComponent) GetValue() interface{} {
	return r.value
}

// SetOnChange sets the change handler
func (r *RadioButtonComponent) SetOnChange(handler func(bool)) {
	r.onChange = handler
}

// Focus focuses the radio button
func (r *RadioButtonComponent) Focus() {
	r.focused = true
}

// Blur unfocuses the radio button
func (r *RadioButtonComponent) Blur() {
	r.focused = false
}

// Enable enables the radio button
func (r *RadioButtonComponent) Enable() {
	r.disabled = false
}

// Disable disables the radio button
func (r *RadioButtonComponent) Disable() {
	r.disabled = true
}