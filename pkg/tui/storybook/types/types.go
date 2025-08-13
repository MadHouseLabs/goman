// Package types defines common types for the storybook
package types

import (
	"github.com/charmbracelet/bubbles/key"
	"github.com/madhouselabs/goman/pkg/tui/components"
)

// InteractiveMode represents the mode of the storybook
type InteractiveMode int

const (
	ModeNavigation InteractiveMode = iota
	ModeInteractive
)

// Story represents a single component story
type Story struct {
	Name        string
	Description string
	Component   func() components.Component
}

// Category represents a category of stories
type Category struct {
	Name    string
	Stories []Story
}

// KeyMap defines key bindings for the storybook
type KeyMap struct {
	NextStory        key.Binding
	PrevStory        key.Binding
	EnterInteractive key.Binding
	ExitInteractive  key.Binding
	ToggleCode       key.Binding
	ToggleProps      key.Binding
	Quit             key.Binding
	Help             key.Binding
	Enter            key.Binding
	Up               key.Binding
	Down             key.Binding
	Left             key.Binding
	Right            key.Binding
}

// DefaultKeyMap returns default key bindings
func DefaultKeyMap() KeyMap {
	return KeyMap{
		NextStory: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "next story"),
		),
		PrevStory: key.NewBinding(
			key.WithKeys("shift+tab"),
			key.WithHelp("shift+tab", "prev story"),
		),
		EnterInteractive: key.NewBinding(
			key.WithKeys("i"),
			key.WithHelp("i", "enter interactive mode"),
		),
		ExitInteractive: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "exit interactive mode"),
		),
		ToggleCode: key.NewBinding(
			key.WithKeys("c"),
			key.WithHelp("c", "toggle code"),
		),
		ToggleProps: key.NewBinding(
			key.WithKeys("p"),
			key.WithHelp("p", "toggle props"),
		),
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "help"),
		),
		Enter: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "select"),
		),
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "down"),
		),
		Left: key.NewBinding(
			key.WithKeys("left", "h"),
			key.WithHelp("←/h", "left"),
		),
		Right: key.NewBinding(
			key.WithKeys("right", "l"),
			key.WithHelp("→/l", "right"),
		),
	}
}