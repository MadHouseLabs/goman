package components

import (
	"fmt"
	"time"
	
	"github.com/charmbracelet/bubbles/timer"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// TimerComponent wraps the Bubble Tea timer
type TimerComponent struct {
	*BaseComponent
	timer  timer.Model
	format string // How to format the time display
	label  string
	style  lipgloss.Style
}

// NewTimer creates a new timer component
func NewTimer(id string, duration time.Duration) *TimerComponent {
	base := NewBaseComponent(id)
	t := timer.NewWithInterval(duration, time.Second)
	
	return &TimerComponent{
		BaseComponent: base,
		timer:         t,
		format:        "15:04:05", // Default HH:MM:SS format
		style:         lipgloss.NewStyle(),
	}
}

// Init initializes the timer
func (t *TimerComponent) Init() tea.Cmd {
	return t.timer.Init()
}

// Update handles timer messages
func (t *TimerComponent) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	
	switch msg := msg.(type) {
	case timer.TickMsg:
		t.timer, cmd = t.timer.Update(msg)
		
		// Store timer state
		t.state["running"] = t.timer.Running()
		t.state["timedOut"] = t.timer.Timedout()
		
		// Check if timer expired
		if t.timer.Timedout() {
			// Trigger callback if set
			if callback, ok := t.props["onTimeout"].(func()); ok {
				callback()
			}
		}
		
		return t, cmd
		
	case timer.StartStopMsg:
		t.timer, cmd = t.timer.Update(msg)
		t.state["running"] = t.timer.Running()
		return t, cmd
		
	case timer.TimeoutMsg:
		t.timer, cmd = t.timer.Update(msg)
		t.state["timedOut"] = true
		return t, cmd
	}
	
	return t, nil
}

// View renders the timer
func (t *TimerComponent) View() string {
	// Update properties from props
	if format, ok := t.props["format"].(string); ok {
		t.format = format
	}
	
	if label, ok := t.props["label"].(string); ok {
		t.label = label
	}
	
	// Format the time display
	timeDisplay := ""
	if t.format == "countdown" {
		// Show remaining time
		if t.timer.Timedout() {
			timeDisplay = "00:00:00"
		} else {
			remaining := t.timer.Timeout
			hours := int(remaining.Hours())
			minutes := int(remaining.Minutes()) % 60
			seconds := int(remaining.Seconds()) % 60
			timeDisplay = fmt.Sprintf("%02d:%02d:%02d", hours, minutes, seconds)
		}
	} else if t.format == "elapsed" {
		// Show elapsed time
		elapsed := time.Since(time.Now().Add(-t.timer.Timeout))
		hours := int(elapsed.Hours())
		minutes := int(elapsed.Minutes()) % 60
		seconds := int(elapsed.Seconds()) % 60
		timeDisplay = fmt.Sprintf("%02d:%02d:%02d", hours, minutes, seconds)
	} else {
		// Use custom format
		timeDisplay = time.Now().Format(t.format)
	}
	
	// Build the view
	view := ""
	if t.label != "" {
		labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
		view = labelStyle.Render(t.label) + " "
	}
	
	// Style based on state
	timeStyle := lipgloss.NewStyle()
	if t.timer.Timedout() {
		timeStyle = timeStyle.Foreground(lipgloss.Color("196")) // Red for timeout
	} else if t.timer.Running() {
		timeStyle = timeStyle.Foreground(lipgloss.Color("82")) // Green for running
	} else {
		timeStyle = timeStyle.Foreground(lipgloss.Color("240")) // Gray for stopped
	}
	
	view += timeStyle.Render(timeDisplay)
	
	// Apply custom styling
	if styleProps, ok := t.props["style"].(lipgloss.Style); ok {
		return styleProps.Render(view)
	}
	
	return t.style.Render(view)
}

// Start starts the timer
func (t *TimerComponent) Start() tea.Cmd {
	return t.timer.Start()
}

// Stop stops the timer
func (t *TimerComponent) Stop() tea.Cmd {
	return t.timer.Stop()
}

// Toggle toggles the timer between running and stopped
func (t *TimerComponent) Toggle() tea.Cmd {
	return t.timer.Toggle()
}

// Reset resets the timer
func (t *TimerComponent) Reset(duration time.Duration) {
	t.timer = timer.NewWithInterval(duration, time.Second)
}

// IsRunning returns whether the timer is running
func (t *TimerComponent) IsRunning() bool {
	return t.timer.Running()
}

// IsTimedOut returns whether the timer has timed out
func (t *TimerComponent) IsTimedOut() bool {
	return t.timer.Timedout()
}

// SetFormat sets the time display format
func (t *TimerComponent) SetFormat(format string) {
	t.format = format
}

// SetLabel sets the timer label
func (t *TimerComponent) SetLabel(label string) {
	t.label = label
}

// SetOnTimeout sets a callback for when the timer expires
func (t *TimerComponent) SetOnTimeout(callback func()) {
	t.SetProps(Props{
		"onTimeout": callback,
	})
}

// NewCountdownTimer creates a countdown timer
func NewCountdownTimer(id string, duration time.Duration) *TimerComponent {
	t := NewTimer(id, duration)
	t.SetFormat("countdown")
	t.SetLabel("Time remaining:")
	return t
}

// NewStopwatch creates a stopwatch timer
func NewStopwatch(id string) *TimerComponent {
	t := NewTimer(id, time.Hour*24) // 24 hour max
	t.SetFormat("elapsed")
	t.SetLabel("Elapsed:")
	return t
}

// NewClock creates a clock display
func NewClock(id string) *TimerComponent {
	t := NewTimer(id, time.Second)
	t.SetFormat("15:04:05")
	t.SetLabel("Current time:")
	// Auto-restart to keep updating
	t.SetOnTimeout(func() {
		t.Reset(time.Second)
		t.Start()
	})
	return t
}