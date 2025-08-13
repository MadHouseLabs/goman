package components

import (
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// SpinnerComponent wraps the Bubble Tea spinner
type SpinnerComponent struct {
	*BaseComponent
	spinner spinner.Model
	message string
	style   lipgloss.Style
}

// NewSpinner creates a new spinner component
func NewSpinner(id string) *SpinnerComponent {
	base := NewBaseComponent(id)
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	
	return &SpinnerComponent{
		BaseComponent: base,
		spinner:       s,
		style:         lipgloss.NewStyle(),
	}
}

// Init initializes the spinner
func (s *SpinnerComponent) Init() tea.Cmd {
	return s.spinner.Tick
}

// Update handles spinner messages
func (s *SpinnerComponent) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	s.spinner, cmd = s.spinner.Update(msg)
	return s, cmd
}

// View renders the spinner
func (s *SpinnerComponent) View() string {
	// Update spinner type from props
	if spinnerType, ok := s.props["type"].(spinner.Spinner); ok {
		s.spinner.Spinner = spinnerType
	}
	
	// Update message from props
	if msg, ok := s.props["message"].(string); ok {
		s.message = msg
	}
	
	// Update style from props
	if styleProps, ok := s.props["style"].(lipgloss.Style); ok {
		s.spinner.Style = styleProps
	}
	
	// Build the view
	view := s.spinner.View()
	if s.message != "" {
		view += " " + s.message
	}
	
	return s.style.Render(view)
}

// SetMessage sets the spinner message
func (s *SpinnerComponent) SetMessage(message string) {
	s.message = message
}

// SetSpinnerType sets the spinner animation type
func (s *SpinnerComponent) SetSpinnerType(spinnerType spinner.Spinner) {
	s.spinner.Spinner = spinnerType
}

// SetStyle sets the spinner style
func (s *SpinnerComponent) SetStyle(style lipgloss.Style) {
	s.spinner.Style = style
}

// Common spinner presets
func NewLoadingSpinner(id string, message string) *SpinnerComponent {
	s := NewSpinner(id)
	s.SetMessage(message)
	s.SetSpinnerType(spinner.Dot)
	s.SetStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("69")))
	return s
}

func NewProgressSpinner(id string, message string) *SpinnerComponent {
	s := NewSpinner(id)
	s.SetMessage(message)
	s.SetSpinnerType(spinner.Line)
	s.SetStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("170")))
	return s
}

func NewPulseSpinner(id string, message string) *SpinnerComponent {
	s := NewSpinner(id)
	s.SetMessage(message)
	s.SetSpinnerType(spinner.Pulse)
	s.SetStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("205")))
	return s
}