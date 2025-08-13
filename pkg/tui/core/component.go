// Package core provides the foundational component system for the TUI
package core

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"
)

// Component is the base interface for all UI components
type Component interface {
	// Lifecycle
	Init() tea.Cmd
	Update(msg tea.Msg) (Component, tea.Cmd)
	View() string

	// Props management
	Props() Props
	SetProps(props Props) error
	
	// State management  
	State() State
	SetState(state State) error
	UpdateState(updates State) error
	
	// Context management
	Context() context.Context
	SetContext(ctx context.Context)
	
	// Component identification
	ID() string
	Type() string
	
	// Parent-child relationships
	Parent() Component
	SetParent(parent Component)
	Children() []Component
	AddChild(child Component) error
	RemoveChild(id string) error
	
	// Event handling
	OnMount()
	OnUnmount()
	OnPropsChange(oldProps, newProps Props)
	OnStateChange(oldState, newState State)
}

// Props represents immutable properties passed to a component
type Props map[string]interface{}

// State represents mutable component state
type State map[string]interface{}

// Get retrieves a value from Props
func (p Props) Get(key string) (interface{}, bool) {
	val, ok := p[key]
	return val, ok
}

// GetString retrieves a string value from Props
func (p Props) GetString(key string) (string, bool) {
	val, ok := p[key]
	if !ok {
		return "", false
	}
	str, ok := val.(string)
	return str, ok
}

// GetInt retrieves an int value from Props
func (p Props) GetInt(key string) (int, bool) {
	val, ok := p[key]
	if !ok {
		return 0, false
	}
	i, ok := val.(int)
	return i, ok
}

// GetBool retrieves a bool value from Props
func (p Props) GetBool(key string) (bool, bool) {
	val, ok := p[key]
	if !ok {
		return false, false
	}
	b, ok := val.(bool)
	return b, ok
}

// Clone creates a deep copy of Props
func (p Props) Clone() Props {
	clone := make(Props)
	for k, v := range p {
		clone[k] = v
	}
	return clone
}

// Merge merges another Props into this one
func (p Props) Merge(other Props) Props {
	result := p.Clone()
	for k, v := range other {
		result[k] = v
	}
	return result
}

// State methods mirror Props methods
func (s State) Get(key string) (interface{}, bool) {
	val, ok := s[key]
	return val, ok
}

func (s State) GetString(key string) (string, bool) {
	val, ok := s[key]
	if !ok {
		return "", false
	}
	str, ok := val.(string)
	return str, ok
}

func (s State) GetInt(key string) (int, bool) {
	val, ok := s[key]
	if !ok {
		return 0, false
	}
	i, ok := val.(int)
	return i, ok
}

func (s State) GetBool(key string) (bool, bool) {
	val, ok := s[key]
	if !ok {
		return false, false
	}
	b, ok := val.(bool)
	return b, ok
}

func (s State) Clone() State {
	clone := make(State)
	for k, v := range s {
		clone[k] = v
	}
	return clone
}

func (s State) Merge(other State) State {
	result := s.Clone()
	for k, v := range other {
		result[k] = v
	}
	return result
}