package components

import (
	"strings"
	
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

// HelpPropsMsg is a message to update help properties
type HelpPropsMsg struct {
	Width   *int
	ShowAll *bool
	Keys    []key.Binding
	Style   *lipgloss.Style
}

// Update handles help messages
func (h *HelpComponent) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		h.help.Width = msg.Width
	case HelpPropsMsg:
		// Update properties from message
		if msg.Width != nil {
			h.help.Width = *msg.Width
		}
		if msg.ShowAll != nil {
			h.help.ShowAll = *msg.ShowAll
		}
		if msg.Keys != nil {
			h.keys = msg.Keys
		}
		if msg.Style != nil {
			h.style = *msg.Style
		}
	}
	
	return h, nil
}

// View renders the help
func (h *HelpComponent) View() string {
	var view string
	if h.help.ShowAll {
		// FullHelpView expects [][]key.Binding for grouped keys
		// Convert single array to grouped array
		groupedKeys := [][]key.Binding{h.keys}
		view = h.help.FullHelpView(groupedKeys)
	} else {
		view = h.help.ShortHelpView(h.keys)
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

// SetStyle sets the wrapper style for the help component
func (h *HelpComponent) SetStyle(style lipgloss.Style) {
	h.style = style
}

// SetShowAll sets whether to show all help items
func (h *HelpComponent) SetShowAll(showAll bool) {
	h.help.ShowAll = showAll
}

// Common key binding helpers

// NewKeyBinding creates a key binding from a combined key string like "up/k" or "pgup/pgdown".
// It splits on "/", "," and whitespace, normalizes arrow glyphs to Bubble Tea names,
// and sets a compact help string like "up/k".
func NewKeyBinding(keysCombined, helpText string) key.Binding {
	norm := func(s string) string {
		switch s {
		case "↑": return "up"
		case "↓": return "down"
		case "←": return "left"
		case "→": return "right"
		case "pgdn": return "pgdown" // normalize common alias
		default: return s
		}
	}

	// split on / , or whitespace
	seps := func(r rune) bool { return r == '/' || r == ',' || r == ' ' || r == '\t' }
	parts := strings.FieldsFunc(keysCombined, seps)
	keys := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" { continue }
		keys = append(keys, norm(p))
	}
	// Defensive: avoid empty binding
	if len(keys) == 0 {
		// Fallback to a non-matchable key to avoid panics but keep help visible
		keys = []string{"unknown"}
	}
	// Use the original combined string as the help label for clarity
	return key.NewBinding(
		key.WithKeys(keys...),
		key.WithHelp(keysCombined, helpText),
	)
}

// DefaultKeyBindings returns common default key bindings
func DefaultKeyBindings() []key.Binding {
	return []key.Binding{
		NewKeyBinding("up/k", "up"),
		NewKeyBinding("down/j", "down"),
		NewKeyBinding("left/h", "left"),
		NewKeyBinding("right/l", "right"),
		NewKeyBinding("enter", "select"),
		NewKeyBinding("esc", "back"),
		NewKeyBinding("q", "quit"),
		NewKeyBinding("?", "help"),
	}
}

// NavigationKeyBindings returns navigation key bindings
func NavigationKeyBindings() []key.Binding {
	return []key.Binding{
		NewKeyBinding("up/down", "navigate"),
		NewKeyBinding("left/right", "switch tabs"),
		NewKeyBinding("pgup/pgdown", "page up/down"),
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