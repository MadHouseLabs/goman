package components

import (
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// TextAreaComponent wraps the Bubble Tea textarea
type TextAreaComponent struct {
	*BaseComponent
	textArea textarea.Model
	label    string
	style    lipgloss.Style
	focused  bool
}

// NewTextArea creates a new text area component
func NewTextArea(id string) *TextAreaComponent {
	base := NewBaseComponent(id)
	ta := textarea.New()
	ta.Focus()
	
	return &TextAreaComponent{
		BaseComponent: base,
		textArea:      ta,
		style:         lipgloss.NewStyle(),
		focused:       false,
	}
}

// Init initializes the text area
func (t *TextAreaComponent) Init() tea.Cmd {
	return textarea.Blink
}

// Update handles text area messages
func (t *TextAreaComponent) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	
	// Update the underlying text area
	t.textArea, cmd = t.textArea.Update(msg)
	
	// Store value in state
	t.state["value"] = t.textArea.Value()
	t.state["lines"] = t.textArea.LineCount()
	
	return t, cmd
}

// View renders the text area
func (t *TextAreaComponent) View() string {
	// Update properties from props
	if placeholder, ok := t.props["placeholder"].(string); ok {
		t.textArea.Placeholder = placeholder
	}
	
	if prompt, ok := t.props["prompt"].(string); ok {
		t.textArea.Prompt = prompt
	}
	
	if charLimit, ok := t.props["charLimit"].(int); ok {
		t.textArea.CharLimit = charLimit
	}
	
	if maxHeight, ok := t.props["maxHeight"].(int); ok {
		t.textArea.MaxHeight = maxHeight
	}
	
	if maxWidth, ok := t.props["maxWidth"].(int); ok {
		t.textArea.MaxWidth = maxWidth
	}
	
	if showLineNumbers, ok := t.props["showLineNumbers"].(bool); ok {
		t.textArea.ShowLineNumbers = showLineNumbers
	}
	
	// Build the view with label above textarea
	view := ""
	if t.label != "" {
		labelStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#9CA3AF")).
			MarginBottom(0)
		view = labelStyle.Render(t.label) + "\n"
	}
	
	// Style the textarea with a subtle border
	textareaStyle := lipgloss.NewStyle()
	
	if t.focused {
		// Focused state - blue border
		textareaStyle = textareaStyle.
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#3B82F6")).
			Padding(0, 1)
	} else {
		// Unfocused state - gray border
		textareaStyle = textareaStyle.
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#4B5563")).
			Padding(0, 1)
	}
	
	view += textareaStyle.Render(t.textArea.View())
	
	// Add margin at bottom for spacing
	containerStyle := lipgloss.NewStyle().
		MarginBottom(1)
	
	// Apply custom styling if provided
	if styleProps, ok := t.props["style"].(lipgloss.Style); ok {
		return styleProps.Render(view)
	}
	
	return containerStyle.Render(view)
}

// SetValue sets the text area value
func (t *TextAreaComponent) SetValue(value string) {
	t.textArea.SetValue(value)
}

// Value returns the current value
func (t *TextAreaComponent) Value() string {
	return t.textArea.Value()
}

// SetPlaceholder sets the placeholder text
func (t *TextAreaComponent) SetPlaceholder(placeholder string) {
	t.textArea.Placeholder = placeholder
}

// SetLabel sets the label for the text area
func (t *TextAreaComponent) SetLabel(label string) {
	t.label = label
}

// Focus focuses the text area
func (t *TextAreaComponent) Focus() tea.Cmd {
	t.focused = true
	return t.textArea.Focus()
}

// Blur unfocuses the text area
func (t *TextAreaComponent) Blur() {
	t.focused = false
	t.textArea.Blur()
}

// SetDimensions sets the text area dimensions
func (t *TextAreaComponent) SetDimensions(width, height int) {
	// Account for the rounded border (2) and padding (2)
	actualWidth := width - 4
	t.textArea.MaxWidth = actualWidth
	t.textArea.MaxHeight = height
	t.textArea.SetWidth(actualWidth)
	t.textArea.SetHeight(height)
}

// SetCharLimit sets the character limit
func (t *TextAreaComponent) SetCharLimit(limit int) {
	t.textArea.CharLimit = limit
}

// SetShowLineNumbers sets whether to show line numbers
func (t *TextAreaComponent) SetShowLineNumbers(show bool) {
	t.textArea.ShowLineNumbers = show
}

// LineCount returns the number of lines
func (t *TextAreaComponent) LineCount() int {
	return t.textArea.LineCount()
}

// CursorInfo returns cursor position info
func (t *TextAreaComponent) CursorInfo() (line, col int) {
	info := t.textArea.LineInfo()
	return info.RowOffset, info.ColumnOffset
}

// NewCodeEditor creates a text area configured for code editing
func NewCodeEditor(id string) *TextAreaComponent {
	ta := NewTextArea(id)
	ta.SetShowLineNumbers(true)
	ta.SetPlaceholder("// Enter code here...")
	ta.textArea.KeyMap.InsertNewline.SetEnabled(true)
	return ta
}

// NewMarkdownEditor creates a text area configured for markdown editing
func NewMarkdownEditor(id string) *TextAreaComponent {
	ta := NewTextArea(id)
	ta.SetPlaceholder("# Enter markdown here...")
	ta.textArea.KeyMap.InsertNewline.SetEnabled(true)
	return ta
}