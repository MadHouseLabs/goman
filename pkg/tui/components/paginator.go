package components

import (
	"fmt"
	
	"github.com/charmbracelet/bubbles/paginator"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// PaginatorComponent wraps the Bubble Tea paginator
type PaginatorComponent struct {
	*BaseComponent
	paginator paginator.Model
	style     lipgloss.Style
}

// NewPaginator creates a new paginator component
func NewPaginator(id string, totalPages int) *PaginatorComponent {
	base := NewBaseComponent(id)
	p := paginator.New()
	p.TotalPages = totalPages
	p.PerPage = 1
	
	return &PaginatorComponent{
		BaseComponent: base,
		paginator:     p,
		style:         lipgloss.NewStyle(),
	}
}

// Init initializes the paginator
func (p *PaginatorComponent) Init() tea.Cmd {
	return nil
}

// Update handles paginator messages
func (p *PaginatorComponent) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	p.paginator, cmd = p.paginator.Update(msg)
	
	// Store current page in state
	p.state["currentPage"] = p.paginator.Page
	p.state["totalPages"] = p.paginator.TotalPages
	
	return p, cmd
}

// View renders the paginator
func (p *PaginatorComponent) View() string {
	// Update properties from props
	if totalPages, ok := p.props["totalPages"].(int); ok {
		p.paginator.TotalPages = totalPages
	}
	
	if activeDot, ok := p.props["activeDot"].(string); ok {
		p.paginator.ActiveDot = activeDot
	}
	
	if inactiveDot, ok := p.props["inactiveDot"].(string); ok {
		p.paginator.InactiveDot = inactiveDot
	}
	
	if showNumbers, ok := p.props["showNumbers"].(bool); ok && showNumbers {
		// Custom numbered pagination
		current := p.paginator.Page + 1
		total := p.paginator.TotalPages
		
		pageInfo := fmt.Sprintf("Page %d of %d", current, total)
		dots := p.paginator.View()
		
		return lipgloss.JoinHorizontal(
			lipgloss.Center,
			pageInfo,
			"  ",
			dots,
		)
	}
	
	// Apply custom styling
	if styleProps, ok := p.props["style"].(lipgloss.Style); ok {
		return styleProps.Render(p.paginator.View())
	}
	
	return p.style.Render(p.paginator.View())
}

// SetPage sets the current page
func (p *PaginatorComponent) SetPage(page int) {
	p.paginator.Page = page
}

// Page returns the current page
func (p *PaginatorComponent) Page() int {
	return p.paginator.Page
}

// SetTotalPages sets the total number of pages
func (p *PaginatorComponent) SetTotalPages(total int) {
	p.paginator.TotalPages = total
}

// TotalPages returns the total number of pages
func (p *PaginatorComponent) TotalPages() int {
	return p.paginator.TotalPages
}

// NextPage moves to the next page
func (p *PaginatorComponent) NextPage() {
	if p.paginator.Page < p.paginator.TotalPages-1 {
		p.paginator.Page++
	}
}

// PrevPage moves to the previous page
func (p *PaginatorComponent) PrevPage() {
	if p.paginator.Page > 0 {
		p.paginator.Page--
	}
}

// FirstPage moves to the first page
func (p *PaginatorComponent) FirstPage() {
	p.paginator.Page = 0
}

// LastPage moves to the last page
func (p *PaginatorComponent) LastPage() {
	p.paginator.Page = p.paginator.TotalPages - 1
}

// SetType sets the paginator type (dots or arabic)
func (p *PaginatorComponent) SetType(pType paginator.Type) {
	p.paginator.Type = pType
}

// SetActiveDot sets the active dot character
func (p *PaginatorComponent) SetActiveDot(dot string) {
	p.paginator.ActiveDot = dot
}

// SetInactiveDot sets the inactive dot character
func (p *PaginatorComponent) SetInactiveDot(dot string) {
	p.paginator.InactiveDot = dot
}

// NewNumberedPaginator creates a paginator with page numbers
func NewNumberedPaginator(id string, totalPages int) *PaginatorComponent {
	p := NewPaginator(id, totalPages)
	p.SetProps(Props{
		"showNumbers": true,
	})
	return p
}

// NewDotPaginator creates a paginator with custom dots
func NewDotPaginator(id string, totalPages int) *PaginatorComponent {
	p := NewPaginator(id, totalPages)
	p.SetActiveDot("●")
	p.SetInactiveDot("○")
	return p
}