package components

import (
	"context"
	"sync"
	
	tea "github.com/charmbracelet/bubbletea"
)

// ContextKey is the type for context keys
type ContextKey string

// Common context keys
const (
	ContextKeyTheme    ContextKey = "theme"
	ContextKeyUser     ContextKey = "user"
	ContextKeySettings ContextKey = "settings"
	ContextKeyData     ContextKey = "data"
	ContextKeyState    ContextKey = "state"
)

// Context represents the application context that flows through components
type Context struct {
	ctx      context.Context
	mu       sync.RWMutex
	values   map[ContextKey]interface{}
	parent   *Context
	children []*Context
	onChange map[ContextKey][]func(interface{})
}

// NewContext creates a new context
func NewContext() *Context {
	return &Context{
		ctx:      context.Background(),
		values:   make(map[ContextKey]interface{}),
		children: []*Context{},
		onChange: make(map[ContextKey][]func(interface{})),
	}
}

// NewContextWithParent creates a new context with a parent
func NewContextWithParent(parent *Context) *Context {
	c := &Context{
		ctx:      parent.ctx,
		values:   make(map[ContextKey]interface{}),
		parent:   parent,
		children: []*Context{},
		onChange: make(map[ContextKey][]func(interface{})),
	}
	parent.addChild(c)
	return c
}

// Get retrieves a value from the context
func (c *Context) Get(key ContextKey) (interface{}, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	// Check current context
	if val, ok := c.values[key]; ok {
		return val, true
	}
	
	// Check parent context
	if c.parent != nil {
		return c.parent.Get(key)
	}
	
	return nil, false
}

// Set sets a value in the context
func (c *Context) Set(key ContextKey, value interface{}) {
	c.mu.Lock()
	oldValue := c.values[key]
	c.values[key] = value
	handlers := c.onChange[key]
	c.mu.Unlock()
	
	// Notify handlers if value changed
	if oldValue != value {
		for _, handler := range handlers {
			handler(value)
		}
	}
}

// Delete removes a value from the context
func (c *Context) Delete(key ContextKey) {
	c.mu.Lock()
	delete(c.values, key)
	c.mu.Unlock()
}

// OnChange registers a handler for context value changes
func (c *Context) OnChange(key ContextKey, handler func(interface{})) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	if c.onChange[key] == nil {
		c.onChange[key] = []func(interface{}){}
	}
	c.onChange[key] = append(c.onChange[key], handler)
}

// addChild adds a child context
func (c *Context) addChild(child *Context) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.children = append(c.children, child)
}

// Provider wraps a component with context
type Provider struct {
	*BaseComponent
	context *Context
	child   Component
}

// NewProvider creates a new context provider
func NewProvider(id string, context *Context) *Provider {
	return &Provider{
		BaseComponent: NewBaseComponent(id),
		context:       context,
	}
}

// SetChild sets the child component
func (p *Provider) SetChild(child Component) {
	p.child = child
	// Inject context into child
	if contextAware, ok := child.(ContextAware); ok {
		contextAware.SetContext(p.context)
	}
}

// Update handles messages
func (p *Provider) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if p.child != nil {
		model, cmd := p.child.Update(msg)
		if comp, ok := model.(Component); ok {
			p.child = comp
		}
		return p, cmd
	}
	return p, nil
}

// View renders the provider
func (p *Provider) View() string {
	if p.child != nil {
		return p.child.View()
	}
	return ""
}

// ContextAware interface for components that can receive context
type ContextAware interface {
	SetContext(*Context)
	GetContext() *Context
}

// Consumer is a component that consumes context values
type Consumer struct {
	*BaseComponent
	context  *Context
	renderer func(*Context) string
}

// NewConsumer creates a new context consumer
func NewConsumer(id string, renderer func(*Context) string) *Consumer {
	return &Consumer{
		BaseComponent: NewBaseComponent(id),
		renderer:      renderer,
	}
}

// SetContext sets the context
func (c *Consumer) SetContext(ctx *Context) {
	c.context = ctx
}

// GetContext gets the context
func (c *Consumer) GetContext() *Context {
	return c.context
}

// View renders using the context
func (c *Consumer) View() string {
	if c.renderer != nil && c.context != nil {
		return c.renderer(c.context)
	}
	return ""
}

// ThemeContext represents theme configuration
type ThemeContext struct {
	Primary   string
	Secondary string
	Success   string
	Error     string
	Warning   string
	Info      string
	Dark      bool
}

// DefaultTheme returns the default theme
func DefaultTheme() *ThemeContext {
	return &ThemeContext{
		Primary:   "69",
		Secondary: "205",
		Success:   "82",
		Error:     "196",
		Warning:   "214",
		Info:      "86",
		Dark:      false,
	}
}

// DarkTheme returns a dark theme
func DarkTheme() *ThemeContext {
	return &ThemeContext{
		Primary:   "141",
		Secondary: "98",
		Success:   "48",
		Error:     "160",
		Warning:   "178",
		Info:      "117",
		Dark:      true,
	}
}

// StateContext represents global application state
type StateContext struct {
	mu     sync.RWMutex
	values map[string]interface{}
}

// NewStateContext creates a new state context
func NewStateContext() *StateContext {
	return &StateContext{
		values: make(map[string]interface{}),
	}
}

// Get retrieves a state value
func (s *StateContext) Get(key string) (interface{}, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	val, ok := s.values[key]
	return val, ok
}

// Set sets a state value
func (s *StateContext) Set(key string, value interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[key] = value
}

// Delete removes a state value
func (s *StateContext) Delete(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.values, key)
}

// UseContext is a helper to get a value from context
func UseContext(ctx *Context, key ContextKey) interface{} {
	val, _ := ctx.Get(key)
	return val
}

// UseTheme is a helper to get the theme from context
func UseTheme(ctx *Context) *ThemeContext {
	if val, ok := ctx.Get(ContextKeyTheme); ok {
		if theme, ok := val.(*ThemeContext); ok {
			return theme
		}
	}
	return DefaultTheme()
}

// UseState is a helper to get the state from context
func UseState(ctx *Context) *StateContext {
	if val, ok := ctx.Get(ContextKeyState); ok {
		if state, ok := val.(*StateContext); ok {
			return state
		}
	}
	return NewStateContext()
}