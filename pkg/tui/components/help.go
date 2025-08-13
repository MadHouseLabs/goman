package components

import (
	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// HelpComponent wraps the Bubble Tea help component
type HelpComponent struct {
	*BaseComponent
	help  help.Model
	keys  []key.Binding
	style lipgloss.Style
}

// NewHelp creates a new help component
func NewHelp(id string) *HelpComponent {
	base := NewBaseComponent(id)
	h := help.New()
	
	return &HelpComponent{
		BaseComponent: base,
		help:          h,
		keys:          []key.Binding{},
		style:         lipgloss.NewStyle(),
	}
}

// Init initializes the help component
func (h *HelpComponent) Init() tea.Cmd {
	return nil
}

// Update handles help messages
func (h *HelpComponent) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Check for window size changes
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		h.help.Width = msg.Width
	}
	
	return h, nil
}

// View renders the help
func (h *HelpComponent) View() string {
	// Update properties from props
	if width, ok := h.props["width"].(int); ok {
		h.help.Width = width
	}
	
	if showAll, ok := h.props["showAll"].(bool); ok {
		h.help.ShowAll = showAll
	}
	
	if keys, ok := h.props["keys"].([]key.Binding); ok {
		h.keys = keys
	}
	
	// Render based on mode
	view := ""
	if h.help.ShowAll {
		// FullHelpView expects [][]key.Binding for grouped keys
		// Convert single array to grouped array
		groupedKeys := [][]key.Binding{h.keys}
		view = h.help.FullHelpView(groupedKeys)
	} else {
		view = h.help.ShortHelpView(h.keys)
	}
	
	// Apply custom styling
	if styleProps, ok := h.props["style"].(lipgloss.Style); ok {
		return styleProps.Render(view)
	}
	
	return h.style.Render(view)
}

// SetKeys sets the key bindings to display
func (h *HelpComponent) SetKeys(keys []key.Binding) {
	h.keys = keys
}

// AddKey adds a key binding
func (h *HelpComponent) AddKey(k key.Binding) {
	h.keys = append(h.keys, k)
}

// SetWidth sets the help width
func (h *HelpComponent) SetWidth(width int) {
	h.help.Width = width
}

// ShowAll shows all help items
func (h *HelpComponent) ShowAll() {
	h.help.ShowAll = true
}

// ShowShort shows short help
func (h *HelpComponent) ShowShort() {
	h.help.ShowAll = false
}

// Toggle toggles between full and short help
func (h *HelpComponent) Toggle() {
	h.help.ShowAll = !h.help.ShowAll
}

// SetStyles sets custom styles for the help
func (h *HelpComponent) SetStyles(styles help.Styles) {
	h.help.Styles = styles
}

// Common key binding helpers

// NewKeyBinding creates a new key binding
func NewKeyBinding(keys, help string) key.Binding {
	return key.NewBinding(
		key.WithKeys(keys),
		key.WithHelp(keys, help),
	)
}

// DefaultKeyBindings returns common default key bindings
func DefaultKeyBindings() []key.Binding {
	return []key.Binding{
		NewKeyBinding("↑/k", "up"),
		NewKeyBinding("↓/j", "down"),
		NewKeyBinding("←/h", "left"),
		NewKeyBinding("→/l", "right"),
		NewKeyBinding("enter", "select"),
		NewKeyBinding("esc", "back"),
		NewKeyBinding("q", "quit"),
		NewKeyBinding("?", "help"),
	}
}

// NavigationKeyBindings returns navigation key bindings
func NavigationKeyBindings() []key.Binding {
	return []key.Binding{
		NewKeyBinding("↑/↓", "navigate"),
		NewKeyBinding("←/→", "switch tabs"),
		NewKeyBinding("pgup/pgdn", "page up/down"),
		NewKeyBinding("home/end", "first/last"),
	}
}

// FormKeyBindings returns form key bindings
func FormKeyBindings() []key.Binding {
	return []key.Binding{
		NewKeyBinding("tab", "next field"),
		NewKeyBinding("shift+tab", "prev field"),
		NewKeyBinding("enter", "submit"),
		NewKeyBinding("esc", "cancel"),
	}
}

// NewDefaultHelp creates a help component with default key bindings
func NewDefaultHelp(id string) *HelpComponent {
	h := NewHelp(id)
	h.SetKeys(DefaultKeyBindings())
	return h
}

// NewNavigationHelp creates a help component with navigation key bindings
func NewNavigationHelp(id string) *HelpComponent {
	h := NewHelp(id)
	h.SetKeys(NavigationKeyBindings())
	return h
}

// NewFormHelp creates a help component with form key bindings
func NewFormHelp(id string) *HelpComponent {
	h := NewHelp(id)
	h.SetKeys(FormKeyBindings())
	return h
}