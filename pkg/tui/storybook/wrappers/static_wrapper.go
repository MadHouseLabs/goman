package wrappers

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/madhouselabs/goman/pkg/tui/components"
)

// StaticTextWrapper wraps a static text display
type StaticTextWrapper struct {
	text  string
	style lipgloss.Style
}

// NewStaticTextWrapper creates a new static text wrapper
func NewStaticTextWrapper(text string, style lipgloss.Style) *StaticTextWrapper {
	return &StaticTextWrapper{
		text:  text,
		style: style,
	}
}

// ID returns the wrapper ID
func (w *StaticTextWrapper) ID() string {
	return "static-text"
}

// Init initializes the wrapper
func (w *StaticTextWrapper) Init() tea.Cmd {
	return nil
}

// Update handles messages
func (w *StaticTextWrapper) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	return w, nil
}

// View renders the static text
func (w *StaticTextWrapper) View() string {
	return w.style.Render(w.text)
}

// Component interface methods
func (w *StaticTextWrapper) SetProps(props components.Props) {}
func (w *StaticTextWrapper) GetProps() components.Props       { return components.Props{} }
func (w *StaticTextWrapper) SetState(state components.State)  {}
func (w *StaticTextWrapper) GetState() components.State       { return components.State{} }
func (w *StaticTextWrapper) SetContext(ctx context.Context)   {}
func (w *StaticTextWrapper) GetContext() context.Context      { return context.Background() }