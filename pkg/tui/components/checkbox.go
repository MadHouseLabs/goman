package components

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// CheckboxComponent represents a checkbox with label
type CheckboxComponent struct {
	*BaseComponent
	label    string
	checked  bool
	focused  bool
	disabled bool
	onChange func(bool)
}

// NewCheckbox creates a new checkbox component
func NewCheckbox(id string, label string) *CheckboxComponent {
	return &CheckboxComponent{
		BaseComponent: NewBaseComponent(id),
		label:         label,
		checked:       false,
		focused:       false,
		disabled:      false,
	}
}

// Init initializes the checkbox
func (c *CheckboxComponent) Init() tea.Cmd {
	return nil
}

// Update handles checkbox messages
func (c *CheckboxComponent) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if c.focused && !c.disabled {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case " ", "enter":
				c.checked = !c.checked
				c.state["checked"] = c.checked
				if c.onChange != nil {
					c.onChange(c.checked)
				}
				return c, nil
			}
		}
	}
	
	return c, nil
}

// View renders the checkbox (no border)
func (c *CheckboxComponent) View() string {
	// Checkbox symbol
	checkMark := "[ ]"
	if c.checked {
		checkMark = "[✓]"
	}
	
	// Build the checkbox view
	view := fmt.Sprintf("%s %s", checkMark, c.label)
	
	// Simple style without border
	style := lipgloss.NewStyle()
	
	// Apply focus/disabled styling
	if c.disabled {
		style = style.Foreground(lipgloss.Color("#6B7280"))
	} else if c.focused {
		style = style.
			Foreground(lipgloss.Color("#3B82F6")).
			Bold(true)
	} else if c.checked {
		style = style.Foreground(lipgloss.Color("#10B981"))
	} else {
		style = style.Foreground(lipgloss.Color("#9CA3AF"))
	}
	
	return style.Render(view)
}

// SetChecked sets the checked state
func (c *CheckboxComponent) SetChecked(checked bool) {
	c.checked = checked
	c.state["checked"] = checked
}

// IsChecked returns whether the checkbox is checked
func (c *CheckboxComponent) IsChecked() bool {
	return c.checked
}

// SetOnChange sets the change handler
func (c *CheckboxComponent) SetOnChange(handler func(bool)) {
	c.onChange = handler
}

// Focus focuses the checkbox
func (c *CheckboxComponent) Focus() {
	c.focused = true
}

// Blur unfocuses the checkbox
func (c *CheckboxComponent) Blur() {
	c.focused = false
}

// Enable enables the checkbox
func (c *CheckboxComponent) Enable() {
	c.disabled = false
}

// Disable disables the checkbox
func (c *CheckboxComponent) Disable() {
	c.disabled = true
}

// CheckboxGroupComponent represents a group of checkboxes
type CheckboxGroupComponent struct {
	*BaseComponent
	checkboxes   []*CheckboxComponent
	label        string
	focusedIndex int
	onChange     func(map[string]bool)
}

// NewCheckboxGroup creates a new checkbox group
func NewCheckboxGroup(id string, label string) *CheckboxGroupComponent {
	return &CheckboxGroupComponent{
		BaseComponent: NewBaseComponent(id),
		checkboxes:    []*CheckboxComponent{},
		label:         label,
		focusedIndex:  0,
	}
}

// AddCheckbox adds a checkbox to the group
func (g *CheckboxGroupComponent) AddCheckbox(id string, label string, checked bool) {
	checkbox := NewCheckbox(g.id+"-"+id, label)
	checkbox.SetChecked(checked)
	checkbox.SetOnChange(func(checked bool) {
		if g.onChange != nil {
			g.onChange(g.GetValues())
		}
	})
	g.checkboxes = append(g.checkboxes, checkbox)
}

// Init initializes the checkbox group
func (g *CheckboxGroupComponent) Init() tea.Cmd {
	if len(g.checkboxes) > 0 {
		g.checkboxes[0].Focus()
		// Initialize state
		for key, value := range g.GetValues() {
			g.state[key] = value
		}
	}
	return nil
}

// Update handles checkbox group messages
func (g *CheckboxGroupComponent) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "tab", "down":
			if g.focusedIndex < len(g.checkboxes)-1 {
				g.checkboxes[g.focusedIndex].Blur()
				g.focusedIndex++
				g.checkboxes[g.focusedIndex].Focus()
			}
			return g, nil
			
		case "shift+tab", "up":
			if g.focusedIndex > 0 {
				g.checkboxes[g.focusedIndex].Blur()
				g.focusedIndex--
				g.checkboxes[g.focusedIndex].Focus()
			}
			return g, nil
		}
	}
	
	// Update the focused checkbox
	if g.focusedIndex >= 0 && g.focusedIndex < len(g.checkboxes) {
		model, cmd := g.checkboxes[g.focusedIndex].Update(msg)
		if checkbox, ok := model.(*CheckboxComponent); ok {
			g.checkboxes[g.focusedIndex] = checkbox
			// Update group state when checkbox changes
			for key, value := range g.GetValues() {
				g.state[key] = value
			}
		}
		return g, cmd
	}
	
	return g, nil
}

// View renders the checkbox group
func (g *CheckboxGroupComponent) View() string {
	var views []string
	
	// Add label if provided
	if g.label != "" {
		labelStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#6B7280")).
			Bold(true).
			MarginBottom(1)
		views = append(views, labelStyle.Render(g.label))
	}
	
	// Add checkboxes (without individual borders in group context)
	for _, checkbox := range g.checkboxes {
		// Render checkbox without border when in group
		checkMark := "[ ]"
		if checkbox.checked {
			checkMark = "[✓]"
		}
		
		// Create style for checkbox item
		itemStyle := lipgloss.NewStyle().
			Padding(0, 1)
		
		if checkbox.disabled {
			itemStyle = itemStyle.
				Foreground(lipgloss.Color("#6B7280"))
		} else if checkbox.focused {
			itemStyle = itemStyle.
				Foreground(lipgloss.Color("#3B82F6")).
				Bold(true)
		} else if checkbox.checked {
			itemStyle = itemStyle.
				Foreground(lipgloss.Color("#10B981"))
		} else {
			itemStyle = itemStyle.
				Foreground(lipgloss.Color("#9CA3AF"))
		}
		
		view := fmt.Sprintf("%s %s", checkMark, checkbox.label)
		views = append(views, itemStyle.Render(view))
	}
	
	// Create container with border for the group
	containerStyle := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		Padding(0, 1).
		BorderForeground(lipgloss.Color("#9CA3AF"))
	
	// Highlight border if any checkbox is focused
	if g.focusedIndex >= 0 && g.focusedIndex < len(g.checkboxes) {
		if g.checkboxes[g.focusedIndex].focused {
			containerStyle = containerStyle.
				BorderForeground(lipgloss.Color("#3B82F6"))
		}
	}
	
	content := strings.Join(views, "\n")
	return containerStyle.Render(content)
}

// GetValues returns the checked state of all checkboxes
func (g *CheckboxGroupComponent) GetValues() map[string]bool {
	values := make(map[string]bool)
	for _, checkbox := range g.checkboxes {
		// Extract the checkbox ID (remove group prefix)
		id := strings.TrimPrefix(checkbox.id, g.id+"-")
		values[id] = checkbox.IsChecked()
	}
	return values
}

// GetState returns the state of the checkbox group
func (g *CheckboxGroupComponent) GetState() State {
	// Return the values as state
	state := make(State)
	for key, value := range g.GetValues() {
		state[key] = value
	}
	return state
}

// SetValue sets the checked state of a specific checkbox
func (g *CheckboxGroupComponent) SetValue(id string, checked bool) {
	for _, checkbox := range g.checkboxes {
		if strings.HasSuffix(checkbox.id, id) {
			checkbox.SetChecked(checked)
			break
		}
	}
}

// SetOnChange sets the change handler
func (g *CheckboxGroupComponent) SetOnChange(handler func(map[string]bool)) {
	g.onChange = handler
	// Update individual checkbox handlers
	for _, checkbox := range g.checkboxes {
		checkbox.SetOnChange(func(checked bool) {
			if g.onChange != nil {
				g.onChange(g.GetValues())
			}
		})
	}
}

// Focus focuses the checkbox group
func (g *CheckboxGroupComponent) Focus() {
	if len(g.checkboxes) > 0 {
		g.checkboxes[g.focusedIndex].Focus()
	}
}

// Blur unfocuses the checkbox group
func (g *CheckboxGroupComponent) Blur() {
	if g.focusedIndex >= 0 && g.focusedIndex < len(g.checkboxes) {
		g.checkboxes[g.focusedIndex].Blur()
	}
}