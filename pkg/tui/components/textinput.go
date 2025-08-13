package components

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// TextInputComponent wraps the Bubble Tea text input
type TextInputComponent struct {
	*BaseComponent
	textInput textinput.Model
	label     string
	style     lipgloss.Style
	focused   bool
}

// NewTextInput creates a new text input component
func NewTextInput(id string) *TextInputComponent {
	base := NewBaseComponent(id)
	ti := textinput.New()
	ti.Focus()
	
	return &TextInputComponent{
		BaseComponent: base,
		textInput:     ti,
		style:         lipgloss.NewStyle(),
		focused:       false,
	}
}

// Init initializes the text input
func (t *TextInputComponent) Init() tea.Cmd {
	return textinput.Blink
}

// Update handles text input messages
func (t *TextInputComponent) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	
	// Update the underlying text input
	t.textInput, cmd = t.textInput.Update(msg)
	
	// Store value in state
	t.state["value"] = t.textInput.Value()
	
	return t, cmd
}

// View renders the text input
func (t *TextInputComponent) View() string {
	// Update properties from props
	if placeholder, ok := t.props["placeholder"].(string); ok {
		t.textInput.Placeholder = placeholder
	}
	
	if prompt, ok := t.props["prompt"].(string); ok {
		t.textInput.Prompt = prompt
	}
	
	if charLimit, ok := t.props["charLimit"].(int); ok {
		t.textInput.CharLimit = charLimit
	}
	
	if width, ok := t.props["width"].(int); ok {
		t.textInput.Width = width
	}
	
	if echoMode, ok := t.props["echoMode"].(textinput.EchoMode); ok {
		t.textInput.EchoMode = echoMode
	}
	
	// Build the view with label above input (no border)
	view := ""
	if t.label != "" {
		labelStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#9CA3AF")).
			MarginBottom(0)
		view = labelStyle.Render(t.label) + "\n"
	}
	
	// Style the input field with underline instead of border
	inputStyle := lipgloss.NewStyle()
	
	if t.focused {
		// Focused state - blue underline
		inputStyle = inputStyle.
			BorderStyle(lipgloss.Border{Bottom: "─"}).
			BorderBottom(true).
			BorderForeground(lipgloss.Color("#3B82F6"))
	} else {
		// Unfocused state - gray underline
		inputStyle = inputStyle.
			BorderStyle(lipgloss.Border{Bottom: "─"}).
			BorderBottom(true).
			BorderForeground(lipgloss.Color("#4B5563"))
	}
	
	view += inputStyle.Render(t.textInput.View())
	
	// Add margin at bottom for spacing between fields
	containerStyle := lipgloss.NewStyle().
		MarginBottom(1)
	
	// Apply custom styling if provided
	if styleProps, ok := t.props["style"].(lipgloss.Style); ok {
		return styleProps.Render(view)
	}
	
	return containerStyle.Render(view)
}

// SetValue sets the input value
func (t *TextInputComponent) SetValue(value string) {
	t.textInput.SetValue(value)
}

// Value returns the current value
func (t *TextInputComponent) Value() string {
	return t.textInput.Value()
}

// SetPlaceholder sets the placeholder text
func (t *TextInputComponent) SetPlaceholder(placeholder string) {
	t.textInput.Placeholder = placeholder
}

// SetPrompt sets the prompt string
func (t *TextInputComponent) SetPrompt(prompt string) {
	t.textInput.Prompt = prompt
}

// SetLabel sets the label for the input
func (t *TextInputComponent) SetLabel(label string) {
	t.label = label
}

// Focus focuses the input
func (t *TextInputComponent) Focus() tea.Cmd {
	t.focused = true
	return t.textInput.Focus()
}

// Blur unfocuses the input
func (t *TextInputComponent) Blur() {
	t.focused = false
	t.textInput.Blur()
}

// SetWidth sets the input width
func (t *TextInputComponent) SetWidth(width int) {
	// No border or padding to account for now
	t.textInput.Width = width
}

// SetCharLimit sets the character limit
func (t *TextInputComponent) SetCharLimit(limit int) {
	t.textInput.CharLimit = limit
}

// SetEchoMode sets the echo mode (for passwords)
func (t *TextInputComponent) SetEchoMode(mode textinput.EchoMode) {
	t.textInput.EchoMode = mode
}

// Validate sets a validation function
func (t *TextInputComponent) Validate(fn func(string) error) {
	t.textInput.Validate = fn
}

// NewPasswordInput creates a password input
func NewPasswordInput(id string) *TextInputComponent {
	ti := NewTextInput(id)
	ti.SetEchoMode(textinput.EchoPassword)
	ti.SetPlaceholder("Enter password...")
	return ti
}

// NewEmailInput creates an email input with validation
func NewEmailInput(id string) *TextInputComponent {
	ti := NewTextInput(id)
	ti.SetPlaceholder("email@example.com")
	ti.Validate(func(s string) error {
		// Basic email validation
		// You can add more sophisticated validation here
		return nil
	})
	return ti
}