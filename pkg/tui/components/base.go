// Package components provides TUI components built on Bubble Tea
package components

import (
	"context"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
)

// Props represents component properties
type Props map[string]interface{}

// State represents component state
type State map[string]interface{}

// BaseComponent provides common functionality for all components
type BaseComponent struct {
	mu       sync.RWMutex
	id       string
	props    Props
	state    State
	ctx      context.Context
	parent   Component
	children []Component
}

// Component interface that all components must implement
type Component interface {
	// Bubble Tea Model interface
	Init() tea.Cmd
	Update(msg tea.Msg) (tea.Model, tea.Cmd)
	View() string

	// Component specific
	ID() string
	SetProps(props Props)
	GetProps() Props
	SetState(state State)
	GetState() State
	SetContext(ctx context.Context)
	GetContext() context.Context
}

// NewBaseComponent creates a new base component
func NewBaseComponent(id string) *BaseComponent {
	return &BaseComponent{
		id:       id,
		props:    make(Props),
		state:    make(State),
		children: []Component{},
		ctx:      context.Background(),
	}
}

// ID returns the component ID
func (b *BaseComponent) ID() string {
	return b.id
}

// SetProps sets the component props
func (b *BaseComponent) SetProps(props Props) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.props = props
}

// GetProps returns the component props
func (b *BaseComponent) GetProps() Props {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.props
}

// SetState sets the component state
func (b *BaseComponent) SetState(state State) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.state = state
}

// GetState returns the component state
func (b *BaseComponent) GetState() State {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.state
}

// SetContext sets the component context
func (b *BaseComponent) SetContext(ctx context.Context) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.ctx = ctx
}

// GetContext returns the component context
func (b *BaseComponent) GetContext() context.Context {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.ctx
}

// Init initializes the component (default implementation)
func (b *BaseComponent) Init() tea.Cmd {
	return nil
}

// Update handles messages (default implementation)
func (b *BaseComponent) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	return b, nil
}

// View renders the component (default implementation)
func (b *BaseComponent) View() string {
	return ""
}