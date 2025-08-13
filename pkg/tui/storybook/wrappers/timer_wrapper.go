package wrappers

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/madhouselabs/goman/pkg/tui/components"
)

// TimerWrapper wraps countdown timer with keyboard controls
type TimerWrapper struct {
	timer   *components.TimerComponent
	logFunc func(string)
}

// NewTimerWrapper creates a new timer wrapper
func NewTimerWrapper(timer *components.TimerComponent, logFunc func(string)) *TimerWrapper {
	return &TimerWrapper{
		timer:   timer,
		logFunc: logFunc,
	}
}

// ID returns the wrapper ID
func (w *TimerWrapper) ID() string {
	return "timer-wrapper"
}

// Init initializes the timer
func (w *TimerWrapper) Init() tea.Cmd {
	return w.timer.Init()
}

// Update handles timer controls
func (w *TimerWrapper) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	// Handle keyboard controls
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "s", "S":
			cmd := w.timer.Start()
			if cmd != nil {
				cmds = append(cmds, cmd)
				if w.logFunc != nil {
					w.logFunc("Timer started")
				}
			}
			return w, tea.Batch(cmds...)

		case "p", "P":
			w.timer.Stop()
			if w.logFunc != nil {
				w.logFunc("Timer paused")
			}
			return w, nil

		case "r", "R":
			w.timer.Reset(60 * time.Second)
			if w.logFunc != nil {
				w.logFunc("Timer reset")
			}
			return w, nil
		}
	}

	// Update timer
	model, cmd := w.timer.Update(msg)
	if timer, ok := model.(*components.TimerComponent); ok {
		w.timer = timer
	}
	cmds = append(cmds, cmd)

	return w, tea.Batch(cmds...)
}

// View renders the timer with controls
func (w *TimerWrapper) View() string {
	containerStyle := lipgloss.NewStyle().
		Padding(2).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240"))

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("86")).
		MarginBottom(1)

	helpStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		Italic(true).
		MarginTop(1)

	title := titleStyle.Render("Timer Demo")
	timer := w.timer.View()
	help := helpStyle.Render("Press 's' to start, 'p' to pause, 'r' to reset")

	content := lipgloss.JoinVertical(lipgloss.Left, title, timer, help)
	return containerStyle.Render(content)
}

// Component interface methods
func (w *TimerWrapper) SetProps(props components.Props) {}
func (w *TimerWrapper) GetProps() components.Props       { return components.Props{} }
func (w *TimerWrapper) SetState(state components.State)  {}
func (w *TimerWrapper) GetState() components.State       { return components.State{} }
func (w *TimerWrapper) SetContext(ctx context.Context)   {}
func (w *TimerWrapper) GetContext() context.Context      { return context.Background() }