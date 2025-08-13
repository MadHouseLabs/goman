// Package wrappers provides wrapper components for interactive storybook demos
package wrappers

import (
	"context"
	"fmt"
	
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/madhouselabs/goman/pkg/tui/components"
)

// InputDemoWrapper manages multiple text inputs with focus
type InputDemoWrapper struct {
	inputs       []*components.TextInputComponent
	inputNames   []string
	focusedIndex int
}

// ID returns the wrapper ID
func (w *InputDemoWrapper) ID() string {
	return "input-demo"
}

// Init initializes all inputs
func (w *InputDemoWrapper) Init() tea.Cmd {
	var cmds []tea.Cmd
	for _, input := range w.inputs {
		cmds = append(cmds, input.Init())
	}
	return tea.Batch(cmds...)
}

// Update handles input messages and focus management
func (w *InputDemoWrapper) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	
	// Handle tab navigation
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "tab":
			// Move to next input
			w.inputs[w.focusedIndex].Blur()
			w.focusedIndex = (w.focusedIndex + 1) % len(w.inputs)
			w.inputs[w.focusedIndex].Focus()
			return w, nil
		case "shift+tab":
			// Move to previous input
			w.inputs[w.focusedIndex].Blur()
			w.focusedIndex--
			if w.focusedIndex < 0 {
				w.focusedIndex = len(w.inputs) - 1
			}
			w.inputs[w.focusedIndex].Focus()
			return w, nil
		}
	}
	
	// Update only the focused input
	model, cmd := w.inputs[w.focusedIndex].Update(msg)
	if input, ok := model.(*components.TextInputComponent); ok {
		w.inputs[w.focusedIndex] = input
	}
	cmds = append(cmds, cmd)
	
	return w, tea.Batch(cmds...)
}

// View renders all inputs
func (w *InputDemoWrapper) View() string {
	containerStyle := lipgloss.NewStyle().
		Padding(1).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240"))
	
	var views []string
	for i, input := range w.inputs {
		// Add label with value display
		labelStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("86")).
			Bold(true)
		
		valueStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			Italic(true)
		
		header := fmt.Sprintf("%s: %s",
			labelStyle.Render(w.inputNames[i]),
			valueStyle.Render(input.Value()))
		
		views = append(views, header)
		views = append(views, input.View())
		views = append(views, "") // Add spacing
	}
	
	content := lipgloss.JoinVertical(lipgloss.Left, views...)
	return containerStyle.Render(content)
}

// Component interface methods
func (w *InputDemoWrapper) SetProps(props components.Props) {}
func (w *InputDemoWrapper) GetProps() components.Props { return components.Props{} }
func (w *InputDemoWrapper) SetState(state components.State) {}
func (w *InputDemoWrapper) GetState() components.State { return components.State{} }
func (w *InputDemoWrapper) SetContext(ctx context.Context) {}
func (w *InputDemoWrapper) GetContext() context.Context { return context.Background() }

// NewInputDemoWrapper creates a new input demo wrapper
func NewInputDemoWrapper(inputs []*components.TextInputComponent, names []string) *InputDemoWrapper {
	wrapper := &InputDemoWrapper{
		inputs:       inputs,
		inputNames:   names,
		focusedIndex: 0,
	}
	
	// Focus first input, blur others
	if len(inputs) > 0 {
		inputs[0].Focus()
		for i := 1; i < len(inputs); i++ {
			inputs[i].Blur()
		}
	}
	
	return wrapper
}