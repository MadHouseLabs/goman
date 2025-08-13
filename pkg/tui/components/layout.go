package components

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// BoxComponent is a container with padding, margin, and border
type BoxComponent struct {
	*BaseComponent
	content  Component
	style    lipgloss.Style
	width    int
	height   int
}

// NewBox creates a new box component
func NewBox(id string) *BoxComponent {
	return &BoxComponent{
		BaseComponent: NewBaseComponent(id),
		style:         lipgloss.NewStyle(),
	}
}

// SetContent sets the box content
func (b *BoxComponent) SetContent(content Component) {
	b.content = content
}

// Init initializes the box component and its content
func (b *BoxComponent) Init() tea.Cmd {
	if b.content != nil {
		return b.content.Init()
	}
	return nil
}

// Update handles messages
func (b *BoxComponent) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if b.content != nil {
		model, cmd := b.content.Update(msg)
		if comp, ok := model.(Component); ok {
			b.content = comp
		}
		return b, cmd
	}
	return b, nil
}

// View renders the box
func (b *BoxComponent) View() string {
	// Apply style from props
	if styleProps, ok := b.props["style"].(lipgloss.Style); ok {
		b.style = styleProps
	}
	
	// Apply dimensions
	if width, ok := b.props["width"].(int); ok {
		b.style = b.style.Width(width)
	}
	if height, ok := b.props["height"].(int); ok {
		b.style = b.style.Height(height)
	}
	
	// Apply padding
	if padding, ok := b.props["padding"].(int); ok {
		b.style = b.style.Padding(padding)
	}
	
	// Apply margin
	if margin, ok := b.props["margin"].(int); ok {
		b.style = b.style.Margin(margin)
	}
	
	// Apply border
	if border, ok := b.props["border"].(bool); ok && border {
		b.style = b.style.Border(lipgloss.NormalBorder())
		if borderColor, ok := b.props["borderColor"].(string); ok {
			b.style = b.style.BorderForeground(lipgloss.Color(borderColor))
		}
	}
	
	content := ""
	if b.content != nil {
		content = b.content.View()
	}
	
	return b.style.Render(content)
}

// FlexComponent arranges children in a row or column
type FlexComponent struct {
	*BaseComponent
	children  []Component
	direction string // "row" or "column"
	gap       int
	style     lipgloss.Style
}

// NewFlex creates a new flex container
func NewFlex(id string, direction string) *FlexComponent {
	return &FlexComponent{
		BaseComponent: NewBaseComponent(id),
		direction:     direction,
		children:      []Component{},
		style:         lipgloss.NewStyle(),
	}
}

// AddChild adds a child component
func (f *FlexComponent) AddChild(child Component) {
	f.children = append(f.children, child)
}

// Init initializes the flex component and all its children
func (f *FlexComponent) Init() tea.Cmd {
	var cmds []tea.Cmd
	for _, child := range f.children {
		if child != nil {
			cmds = append(cmds, child.Init())
		}
	}
	return tea.Batch(cmds...)
}

// Update handles messages
func (f *FlexComponent) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	
	for i, child := range f.children {
		model, cmd := child.Update(msg)
		if comp, ok := model.(Component); ok {
			f.children[i] = comp
		}
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	
	return f, tea.Batch(cmds...)
}

// View renders the flex container
func (f *FlexComponent) View() string {
	// Update gap from props
	if gap, ok := f.props["gap"].(int); ok {
		f.gap = gap
	}
	
	// Collect child views
	var views []string
	for _, child := range f.children {
		views = append(views, child.View())
	}
	
	// Join based on direction
	if f.direction == "column" {
		// Vertical layout
		if f.gap > 0 {
			for i := range views {
				if i < len(views)-1 {
					views[i] += strings.Repeat("\n", f.gap)
				}
			}
		}
		return lipgloss.JoinVertical(lipgloss.Left, views...)
	} else {
		// Horizontal layout (default)
		if f.gap > 0 {
			for i := range views {
				if i < len(views)-1 {
					views[i] += strings.Repeat(" ", f.gap)
				}
			}
		}
		return lipgloss.JoinHorizontal(lipgloss.Top, views...)
	}
}

// GridComponent arranges children in a grid
type GridComponent struct {
	*BaseComponent
	children [][]Component
	rows     int
	cols     int
	gap      int
	style    lipgloss.Style
}

// NewGrid creates a new grid container
func NewGrid(id string, rows, cols int) *GridComponent {
	grid := make([][]Component, rows)
	for i := range grid {
		grid[i] = make([]Component, cols)
	}
	
	return &GridComponent{
		BaseComponent: NewBaseComponent(id),
		children:      grid,
		rows:          rows,
		cols:          cols,
		style:         lipgloss.NewStyle(),
	}
}

// SetCell sets a component at a specific grid position
func (g *GridComponent) SetCell(row, col int, component Component) {
	if row >= 0 && row < g.rows && col >= 0 && col < g.cols {
		g.children[row][col] = component
	}
}

// Init initializes the grid component and all its children
func (g *GridComponent) Init() tea.Cmd {
	var cmds []tea.Cmd
	for row := range g.children {
		for col := range g.children[row] {
			if g.children[row][col] != nil {
				cmds = append(cmds, g.children[row][col].Init())
			}
		}
	}
	return tea.Batch(cmds...)
}

// Update handles messages
func (g *GridComponent) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	
	for row := range g.children {
		for col := range g.children[row] {
			if g.children[row][col] != nil {
				model, cmd := g.children[row][col].Update(msg)
				if comp, ok := model.(Component); ok {
					g.children[row][col] = comp
				}
				if cmd != nil {
					cmds = append(cmds, cmd)
				}
			}
		}
	}
	
	return g, tea.Batch(cmds...)
}

// View renders the grid
func (g *GridComponent) View() string {
	// Update gap from props
	if gap, ok := g.props["gap"].(int); ok {
		g.gap = gap
	}
	
	var rows []string
	
	for _, row := range g.children {
		var cells []string
		for _, cell := range row {
			cellView := ""
			if cell != nil {
				cellView = cell.View()
			}
			cells = append(cells, cellView)
		}
		// Join cells horizontally
		rowView := lipgloss.JoinHorizontal(lipgloss.Top, cells...)
		rows = append(rows, rowView)
	}
	
	// Join rows vertically
	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

// Helper functions for common layouts

// NewVBox creates a vertical box (column flex)
func NewVBox(id string) *FlexComponent {
	return NewFlex(id, "column")
}

// NewHBox creates a horizontal box (row flex)
func NewHBox(id string) *FlexComponent {
	return NewFlex(id, "row")
}

// NewBorderedBox creates a box with a border
func NewBorderedBox(id string, content Component) *BoxComponent {
	box := NewBox(id)
	box.SetContent(content)
	box.SetProps(Props{
		"border":      true,
		"borderColor": "240",
		"padding":     1,
	})
	return box
}