package wrappers

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/madhouselabs/goman/pkg/tui/components"
)

// PaginatorWrapper wraps paginator with keyboard controls
type PaginatorWrapper struct {
	paginator *components.PaginatorComponent
	logFunc   func(string)
	items     []string
}

// NewPaginatorWrapper creates a new paginator wrapper
func NewPaginatorWrapper(paginator *components.PaginatorComponent, items []string, logFunc func(string)) *PaginatorWrapper {
	return &PaginatorWrapper{
		paginator: paginator,
		logFunc:   logFunc,
		items:     items,
	}
}

// ID returns the wrapper ID
func (w *PaginatorWrapper) ID() string {
	return "paginator-wrapper"
}

// Init initializes the paginator
func (w *PaginatorWrapper) Init() tea.Cmd {
	return w.paginator.Init()
}

// Update handles paginator controls
func (w *PaginatorWrapper) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	// Handle keyboard controls
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "n", "right":
			w.paginator.NextPage()
			if w.logFunc != nil {
				w.logFunc(fmt.Sprintf("Page %d of %d", w.paginator.Page()+1, w.paginator.TotalPages()))
			}
			return w, nil

		case "p", "left", "h":
			w.paginator.PrevPage()
			if w.logFunc != nil {
				w.logFunc(fmt.Sprintf("Page %d of %d", w.paginator.Page()+1, w.paginator.TotalPages()))
			}
			return w, nil

		case "f":
			w.paginator.FirstPage()
			if w.logFunc != nil {
				w.logFunc("First page")
			}
			return w, nil

		case "L":
			w.paginator.LastPage()
			if w.logFunc != nil {
				w.logFunc("Last page")
			}
			return w, nil
		}
	}

	// Update paginator
	model, cmd := w.paginator.Update(msg)
	if pag, ok := model.(*components.PaginatorComponent); ok {
		w.paginator = pag
	}
	cmds = append(cmds, cmd)

	return w, tea.Batch(cmds...)
}

// View renders the paginator with items
func (w *PaginatorWrapper) View() string {
	containerStyle := lipgloss.NewStyle().
		Padding(2).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240"))

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("86")).
		MarginBottom(1)

	itemsStyle := lipgloss.NewStyle().
		Padding(1).
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("245")).
		MarginBottom(1).
		Height(8)

	helpStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		Italic(true).
		MarginTop(1)

	// Get current page items
	itemsPerPage := 10
	start := w.paginator.Page() * itemsPerPage
	end := start + itemsPerPage
	if end > len(w.items) {
		end = len(w.items)
	}

	var itemViews []string
	for i := start; i < end; i++ {
		itemViews = append(itemViews, fmt.Sprintf("• %s", w.items[i]))
	}

	title := titleStyle.Render("Paginator Demo")
	items := itemsStyle.Render(lipgloss.JoinVertical(lipgloss.Left, itemViews...))
	paginator := w.paginator.View()
	help := helpStyle.Render("Use ←/→ or 'p'/'n' to navigate, 'f' for first, 'L' for last")

	content := lipgloss.JoinVertical(lipgloss.Left, title, items, paginator, help)
	return containerStyle.Render(content)
}

// Component interface methods
func (w *PaginatorWrapper) SetProps(props components.Props) {}
func (w *PaginatorWrapper) GetProps() components.Props       { return components.Props{} }
func (w *PaginatorWrapper) SetState(state components.State)  {}
func (w *PaginatorWrapper) GetState() components.State       { return components.State{} }
func (w *PaginatorWrapper) SetContext(ctx context.Context)   {}
func (w *PaginatorWrapper) GetContext() context.Context      { return context.Background() }