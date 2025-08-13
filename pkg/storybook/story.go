package storybook

import (
	"fmt"
	"reflect"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/madhouselabs/goman/pkg/tui/components"
)

// Story represents a single component story with examples and documentation
type Story struct {
	ID              string
	Name            string
	Description     string
	Component       components.Component
	Examples        []Example
	currentExample  int
	width           int
	height          int
	properties      map[string]interface{}
	sourceCode      string
	documentation   string
}

// Example represents a specific example of a component with different props
type Example struct {
	Name        string
	Description string
	Props       components.Props
	State       components.State
	Code        string
}

// NewStory creates a new component story
func NewStory(id, name, description string, component components.Component) *Story {
	return &Story{
		ID:            id,
		Name:          name,
		Description:   description,
		Component:     component,
		Examples:      make([]Example, 0),
		currentExample: 0,
		properties:    make(map[string]interface{}),
	}
}

// AddExample adds an example to the story
func (s *Story) AddExample(name, description string, props components.Props, state components.State, code string) {
	example := Example{
		Name:        name,
		Description: description,
		Props:       props,
		State:       state,
		Code:        code,
	}
	s.Examples = append(s.Examples, example)
}

// SetDimensions sets the story display dimensions
func (s *Story) SetDimensions(width, height int) {
	s.width = width
	s.height = height
}

// Init initializes the story
func (s *Story) Init() tea.Cmd {
	if s.Component != nil {
		return s.Component.Init()
	}
	return nil
}

// Update handles story updates
func (s *Story) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "n":
			s.nextExample()
		case "p":
			s.previousExample()
		case "r":
			s.resetComponent()
		}
	}

	// Update the component with current example props and state
	if s.Component != nil {
		if len(s.Examples) > 0 && s.currentExample < len(s.Examples) {
			example := s.Examples[s.currentExample]
			s.Component.SetProps(example.Props)
			s.Component.SetState(example.State)
		}
		
		model, componentCmd := s.Component.Update(msg)
		if comp, ok := model.(components.Component); ok {
			s.Component = comp
		}
		cmd = tea.Batch(cmd, componentCmd)
	}

	return s, cmd
}

// View renders the story
func (s *Story) View() string {
	if s.width == 0 || s.height == 0 {
		return "Loading story..."
	}

	// Story header
	header := s.renderHeader()
	
	// Component showcase area
	showcaseHeight := s.height - 8 // Account for header and controls
	showcase := s.renderShowcase(showcaseHeight)
	
	// Example controls
	controls := s.renderControls()
	
	// Example info
	info := s.renderExampleInfo()

	return lipgloss.JoinVertical(lipgloss.Left, header, showcase, controls, info)
}

// renderHeader renders the story header
func (s *Story) renderHeader() string {
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("69")).
		MarginBottom(1)
	
	descStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("245")).
		MarginBottom(1)

	title := titleStyle.Render(s.Name)
	description := descStyle.Render(s.Description)

	return lipgloss.JoinVertical(lipgloss.Left, title, description)
}

// renderShowcase renders the component showcase area
func (s *Story) renderShowcase(height int) string {
	showcaseStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(2).
		Width(s.width - 4).
		Height(height).
		Align(lipgloss.Center, lipgloss.Center)

	if s.Component == nil {
		return showcaseStyle.Render("No component available")
	}

	return showcaseStyle.Render(s.Component.View())
}

// renderControls renders the example navigation controls
func (s *Story) renderControls() string {
	if len(s.Examples) <= 1 {
		return ""
	}

	controlsStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("245")).
		MarginTop(1).
		MarginBottom(1)

	current := s.currentExample + 1
	total := len(s.Examples)
	controls := fmt.Sprintf("Example %d of %d | Press 'n' for next, 'p' for previous, 'r' to reset", current, total)

	return controlsStyle.Render(controls)
}

// renderExampleInfo renders information about the current example
func (s *Story) renderExampleInfo() string {
	if len(s.Examples) == 0 {
		return ""
	}

	example := s.Examples[s.currentExample]
	
	infoStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(1).
		Width(s.width - 4)

	nameStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("69"))

	descStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("245")).
		MarginTop(1)

	content := nameStyle.Render(example.Name)
	if example.Description != "" {
		content += "\n" + descStyle.Render(example.Description)
	}

	// Show props if any
	if len(example.Props) > 0 {
		propsStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("208")).
			MarginTop(1)
		
		var propLines []string
		for key, value := range example.Props {
			propLines = append(propLines, fmt.Sprintf("  %s: %v", key, value))
		}
		
		content += "\n" + propsStyle.Render("Props:\n"+strings.Join(propLines, "\n"))
	}

	return infoStyle.Render(content)
}

// GetProperties returns the current properties of the story component
func (s *Story) GetProperties() map[string]interface{} {
	properties := make(map[string]interface{})
	
	if s.Component != nil {
		// Get component props
		props := s.Component.GetProps()
		for key, value := range props {
			properties[key] = value
		}
		
		// Get component state
		state := s.Component.GetState()
		for key, value := range state {
			properties["state_"+key] = value
		}
		
		// Add component-specific properties using reflection
		s.addReflectionProperties(properties)
	}
	
	// Add story metadata
	properties["story_name"] = s.Name
	properties["story_description"] = s.Description
	properties["current_example"] = s.currentExample + 1
	properties["total_examples"] = len(s.Examples)
	
	return properties
}

// addReflectionProperties adds properties discovered through reflection
func (s *Story) addReflectionProperties(properties map[string]interface{}) {
	if s.Component == nil {
		return
	}
	
	value := reflect.ValueOf(s.Component)
	if value.Kind() == reflect.Ptr {
		value = value.Elem()
	}
	
	if value.Kind() != reflect.Struct {
		return
	}
	
	structType := value.Type()
	for i := 0; i < value.NumField(); i++ {
		field := value.Field(i)
		fieldType := structType.Field(i)
		
		// Skip unexported fields
		if !field.CanInterface() {
			continue
		}
		
		// Skip embedded BaseComponent
		if fieldType.Name == "BaseComponent" {
			continue
		}
		
		// Add simple types
		switch field.Kind() {
		case reflect.String, reflect.Int, reflect.Bool, reflect.Float64:
			properties[strings.ToLower(fieldType.Name)] = field.Interface()
		}
	}
}

// GetCode returns the source code for the current example
func (s *Story) GetCode() string {
	if len(s.Examples) > 0 && s.currentExample < len(s.Examples) {
		example := s.Examples[s.currentExample]
		if example.Code != "" {
			return example.Code
		}
	}
	
	// Generate default code based on component
	return s.generateDefaultCode()
}

// generateDefaultCode generates basic code example for the component
func (s *Story) generateDefaultCode() string {
	if s.Component == nil {
		return "// No component available"
	}
	
	// Use reflection to determine component type
	componentType := reflect.TypeOf(s.Component)
	if componentType.Kind() == reflect.Ptr {
		componentType = componentType.Elem()
	}
	
	componentName := componentType.Name()
	
	code := fmt.Sprintf(`// %s Example
%s := components.New%s("example-id")

// Set properties
`, s.Name, strings.ToLower(componentName), componentName)

	// Add props if available
	if len(s.Examples) > 0 && s.currentExample < len(s.Examples) {
		example := s.Examples[s.currentExample]
		for key, value := range example.Props {
			code += fmt.Sprintf(`%s.SetProps(components.Props{"%s": %v})
`, strings.ToLower(componentName), key, value)
		}
	}
	
	code += fmt.Sprintf(`
// Use in Bubble Tea app
model, cmd := %s.Update(msg)
view := %s.View()`, strings.ToLower(componentName), strings.ToLower(componentName))
	
	return code
}

// Navigation methods

func (s *Story) nextExample() {
	if len(s.Examples) > 0 {
		s.currentExample = (s.currentExample + 1) % len(s.Examples)
		s.resetComponent()
	}
}

func (s *Story) previousExample() {
	if len(s.Examples) > 0 {
		s.currentExample = (s.currentExample - 1 + len(s.Examples)) % len(s.Examples)
		s.resetComponent()
	}
}

func (s *Story) resetComponent() {
	if s.Component != nil && len(s.Examples) > 0 && s.currentExample < len(s.Examples) {
		example := s.Examples[s.currentExample]
		s.Component.SetProps(example.Props)
		s.Component.SetState(example.State)
	}
}