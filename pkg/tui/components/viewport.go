package components

import (
	"fmt"
	
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ViewportComponent wraps the Bubble Tea viewport
type ViewportComponent struct {
	*BaseComponent
	viewport viewport.Model
	style    lipgloss.Style
}

// NewViewport creates a new viewport component
func NewViewport(id string, width, height int) *ViewportComponent {
	base := NewBaseComponent(id)
	vp := viewport.New(width, height)
	
	return &ViewportComponent{
		BaseComponent: base,
		viewport:      vp,
		style:         lipgloss.NewStyle(),
	}
}

// Init initializes the viewport
func (v *ViewportComponent) Init() tea.Cmd {
	return nil
}

// Update handles viewport messages
func (v *ViewportComponent) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// Update viewport dimensions from props if set
		if w, ok := v.props["width"].(int); ok {
			v.viewport.Width = w
		} else {
			v.viewport.Width = msg.Width
		}
		
		if h, ok := v.props["height"].(int); ok {
			v.viewport.Height = h
		} else {
			v.viewport.Height = msg.Height
		}
		
	case tea.KeyMsg:
		// Handle scrolling
		switch msg.String() {
		case "up", "k":
			v.viewport.LineUp(1)
		case "down", "j":
			v.viewport.LineDown(1)
		case "pgup":
			v.viewport.HalfViewUp()
		case "pgdown":
			v.viewport.HalfViewDown()
		case "home", "g":
			v.viewport.GotoTop()
		case "end", "G":
			v.viewport.GotoBottom()
		}
	}
	
	// Update the underlying viewport
	v.viewport, cmd = v.viewport.Update(msg)
	
	return v, cmd
}

// View renders the viewport
func (v *ViewportComponent) View() string {
	// Apply any custom styling from props
	if styleProps, ok := v.props["style"].(lipgloss.Style); ok {
		v.style = styleProps
	}
	
	// Check if we should show scroll percentage
	if showScroll, ok := v.props["showScrollBar"].(bool); ok && showScroll {
		return v.style.Render(v.viewport.View()) + v.scrollBar()
	}
	
	return v.style.Render(v.viewport.View())
}

// SetContent sets the viewport content
func (v *ViewportComponent) SetContent(content string) {
	v.viewport.SetContent(content)
}

// SetDimensions sets the viewport dimensions
func (v *ViewportComponent) SetDimensions(width, height int) {
	v.viewport.Width = width
	v.viewport.Height = height
}

// scrollBar renders a simple scroll indicator
func (v *ViewportComponent) scrollBar() string {
	percent := v.viewport.ScrollPercent()
	if percent == 0 {
		return " TOP"
	} else if percent >= 0.99 {
		return " BOTTOM"
	}
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Render(" " + lipgloss.NewStyle().Render("█") + " " + 
			lipgloss.NewStyle().Render(fmt.Sprintf("%.0f%%", percent*100)))
}

// ScrollToTop scrolls to the top
func (v *ViewportComponent) ScrollToTop() {
	v.viewport.GotoTop()
}

// ScrollToBottom scrolls to the bottom
func (v *ViewportComponent) ScrollToBottom() {
	v.viewport.GotoBottom()
}