package wrappers

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/madhouselabs/goman/pkg/tui/components"
)

// ButtonDemoWrapper manages button demonstrations with keyboard navigation
type ButtonDemoWrapper struct {
	buttons      []*components.ButtonComponent
	focusedIndex int
	logFunc      func(string)
}

// NewButtonDemoWrapper creates a new button demo wrapper
func NewButtonDemoWrapper(buttons []*components.ButtonComponent, logFunc func(string)) *ButtonDemoWrapper {
	wrapper := &ButtonDemoWrapper{
		buttons:      buttons,
		focusedIndex: 0,
		logFunc:      logFunc,
	}

	// Set up click handlers
	for _, btn := range buttons {
		btnLabel := btn.GetProps()["label"]
		btn.SetOnClick(func() {
			if wrapper.logFunc != nil {
				wrapper.logFunc(fmt.Sprintf("Button clicked: %v", btnLabel))
			}
		})
	}

	// Focus first button
	if len(buttons) > 0 {
		buttons[0].Focus()
	}

	return wrapper
}

// ID returns the wrapper ID
func (w *ButtonDemoWrapper) ID() string {
	return "button-demo"
}

// Init initializes all buttons
func (w *ButtonDemoWrapper) Init() tea.Cmd {
	var cmds []tea.Cmd
	for _, btn := range w.buttons {
		cmds = append(cmds, btn.Init())
	}
	return tea.Batch(cmds...)
}

// Update handles button navigation and interactions
func (w *ButtonDemoWrapper) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "tab", "right", "l":
			// Move to next button
			if len(w.buttons) > 0 {
				w.buttons[w.focusedIndex].Blur()
				w.focusedIndex = (w.focusedIndex + 1) % len(w.buttons)
				w.buttons[w.focusedIndex].Focus()
			}
			return w, nil

		case "shift+tab", "left", "h":
			// Move to previous button
			if len(w.buttons) > 0 {
				w.buttons[w.focusedIndex].Blur()
				w.focusedIndex--
				if w.focusedIndex < 0 {
					w.focusedIndex = len(w.buttons) - 1
				}
				w.buttons[w.focusedIndex].Focus()
			}
			return w, nil

		case "d":
			// Toggle disable state for focused button
			if w.focusedIndex < len(w.buttons) {
				btn := w.buttons[w.focusedIndex]
				if btn.IsDisabled() {
					btn.Enable()
					if w.logFunc != nil {
						w.logFunc("Button enabled")
					}
				} else {
					btn.Disable()
					if w.logFunc != nil {
						w.logFunc("Button disabled")
					}
				}
			}
			return w, nil
		}
	}

	// Update focused button
	if w.focusedIndex < len(w.buttons) {
		model, cmd := w.buttons[w.focusedIndex].Update(msg)
		if btn, ok := model.(*components.ButtonComponent); ok {
			w.buttons[w.focusedIndex] = btn
		}
		cmds = append(cmds, cmd)
	}

	return w, tea.Batch(cmds...)
}

// View renders all buttons
func (w *ButtonDemoWrapper) View() string {
	containerStyle := lipgloss.NewStyle().
		Padding(2).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240"))

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("86")).
		MarginBottom(1)

	buttonContainer := lipgloss.NewStyle().
		MarginBottom(2)

	helpStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		Italic(true)

	// Render buttons horizontally
	var buttonViews []string
	for _, btn := range w.buttons {
		buttonViews = append(buttonViews, btn.View())
	}

	title := titleStyle.Render("Button Demo")
	buttons := buttonContainer.Render(
		lipgloss.JoinHorizontal(lipgloss.Left, buttonViews...),
	)
	help := helpStyle.Render("Use ←/→ or tab to navigate, Enter to click, 'd' to toggle disable")

	content := lipgloss.JoinVertical(lipgloss.Left, title, buttons, help)
	return containerStyle.Render(content)
}

// Component interface methods
func (w *ButtonDemoWrapper) SetProps(props components.Props) {}
func (w *ButtonDemoWrapper) GetProps() components.Props       { return components.Props{} }
func (w *ButtonDemoWrapper) SetState(state components.State)  {}
func (w *ButtonDemoWrapper) GetState() components.State       { return components.State{} }
func (w *ButtonDemoWrapper) SetContext(ctx context.Context)   {}
func (w *ButtonDemoWrapper) GetContext() context.Context      { return context.Background() }