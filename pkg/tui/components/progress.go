package components

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/progress"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ProgressComponent wraps the Bubble Tea progress bar
type ProgressComponent struct {
	*BaseComponent
	progress progress.Model
	percent  float64
	label    string
}

// NewProgress creates a new progress bar component
func NewProgress(id string) *ProgressComponent {
	base := NewBaseComponent(id)
	p := progress.New(progress.WithDefaultGradient())
	
	return &ProgressComponent{
		BaseComponent: base,
		progress:      p,
		percent:       0.0,
	}
}

// Init initializes the progress bar
func (p *ProgressComponent) Init() tea.Cmd {
	return nil
}

// Update handles progress bar messages
func (p *ProgressComponent) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Check for window size changes
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		p.progress.Width = msg.Width - 4
	case progress.FrameMsg:
		progressModel, cmd := p.progress.Update(msg)
		p.progress = progressModel.(progress.Model)
		return p, cmd
	}
	
	// Update percent from props
	if percent, ok := p.props["percent"].(float64); ok {
		p.percent = percent
	}
	
	return p, p.progress.SetPercent(p.percent)
}

// View renders the progress bar
func (p *ProgressComponent) View() string {
	// Update label from props
	if label, ok := p.props["label"].(string); ok {
		p.label = label
	}
	
	// Update width from props
	if width, ok := p.props["width"].(int); ok {
		p.progress.Width = width
	}
	
	// Build the view
	var b strings.Builder
	
	if p.label != "" {
		labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
		b.WriteString(labelStyle.Render(p.label))
		b.WriteString("\n")
	}
	
	b.WriteString(p.progress.View())
	
	// Add percentage text
	percentStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		MarginLeft(2)
	b.WriteString(percentStyle.Render(fmt.Sprintf("%.0f%%", p.percent*100)))
	
	return b.String()
}

// SetPercent sets the progress percentage (0.0 to 1.0)
func (p *ProgressComponent) SetPercent(percent float64) tea.Cmd {
	p.percent = percent
	return p.progress.SetPercent(percent)
}

// SetLabel sets the progress label
func (p *ProgressComponent) SetLabel(label string) {
	p.label = label
}

// SetWidth sets the progress bar width
func (p *ProgressComponent) SetWidth(width int) {
	p.progress.Width = width
}

// SetColors sets custom colors for the progress bar
func (p *ProgressComponent) SetColors(gradient []string) {
	if len(gradient) >= 2 {
		p.progress = progress.New(
			progress.WithGradient(gradient[0], gradient[1]),
			progress.WithWidth(p.progress.Width),
		)
	}
}

// Increment increments the progress by a given amount
func (p *ProgressComponent) Increment(amount float64) tea.Cmd {
	p.percent += amount
	if p.percent > 1.0 {
		p.percent = 1.0
	}
	return p.SetPercent(p.percent)
}

// Reset resets the progress to 0
func (p *ProgressComponent) Reset() tea.Cmd {
	p.percent = 0.0
	return p.SetPercent(0.0)
}

// NewStyledProgress creates a progress bar with custom styling
func NewStyledProgress(id string, width int) *ProgressComponent {
	p := NewProgress(id)
	p.SetWidth(width)
	p.SetColors([]string{"#FF00FF", "#00FFFF"})
	return p
}