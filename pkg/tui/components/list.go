package components

import (
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ListComponent wraps the Bubble Tea list
type ListComponent struct {
	*BaseComponent
	list  list.Model
	items []list.Item
}

// ListItem implements the list.Item interface
type ListItem struct {
	ItemTitle       string
	ItemDescription string
	ItemValue       interface{}
}

// Title returns the item title (implements list.Item)
func (i ListItem) Title() string       { return i.ItemTitle }

// Description returns the item description (implements list.Item)  
func (i ListItem) Description() string { return i.ItemDescription }

// FilterValue returns the filter value (implements list.Item)
func (i ListItem) FilterValue() string { return i.ItemTitle }

// NewList creates a new list component
func NewList(id string, width, height int) *ListComponent {
	base := NewBaseComponent(id)
	
	// Create list with default delegate
	items := []list.Item{}
	l := list.New(items, list.NewDefaultDelegate(), width, height)
	l.Title = "List"
	
	return &ListComponent{
		BaseComponent: base,
		list:          l,
		items:         items,
	}
}

// Init initializes the list
func (l *ListComponent) Init() tea.Cmd {
	return nil
}

// Update handles list messages
func (l *ListComponent) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	
	// Check for window size changes
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		l.list.SetWidth(msg.Width)
		l.list.SetHeight(msg.Height)
	}
	
	// Update the underlying list
	l.list, cmd = l.list.Update(msg)
	
	// Store selected item in state
	if selected := l.list.SelectedItem(); selected != nil {
		l.state["selectedItem"] = selected
	}
	
	return l, cmd
}

// View renders the list
func (l *ListComponent) View() string {
	// Update title from props
	if title, ok := l.props["title"].(string); ok {
		l.list.Title = title
	}
	
	// Update styles from props
	if styles, ok := l.props["styles"].(list.Styles); ok {
		l.list.Styles = styles
	}
	
	// Check if we should show help
	if showHelp, ok := l.props["showHelp"].(bool); ok {
		l.list.SetShowHelp(showHelp)
	}
	
	// Check if we should show status bar
	if showStatus, ok := l.props["showStatusBar"].(bool); ok {
		l.list.SetShowStatusBar(showStatus)
	}
	
	// Check if filtering is enabled
	if filtering, ok := l.props["filteringEnabled"].(bool); ok {
		l.list.SetFilteringEnabled(filtering)
	}
	
	return l.list.View()
}

// SetItems sets the list items
func (l *ListComponent) SetItems(items []ListItem) {
	listItems := make([]list.Item, len(items))
	for i, item := range items {
		listItems[i] = item
	}
	l.items = listItems
	l.list.SetItems(listItems)
}

// AddItem adds an item to the list
func (l *ListComponent) AddItem(item ListItem) {
	l.items = append(l.items, item)
	l.list.SetItems(l.items)
}

// RemoveItem removes an item at index
func (l *ListComponent) RemoveItem(index int) {
	if index >= 0 && index < len(l.items) {
		l.items = append(l.items[:index], l.items[index+1:]...)
		l.list.SetItems(l.items)
	}
}

// SelectedItem returns the currently selected item
func (l *ListComponent) SelectedItem() ListItem {
	if item := l.list.SelectedItem(); item != nil {
		if listItem, ok := item.(ListItem); ok {
			return listItem
		}
	}
	return ListItem{}
}

// SetDimensions sets the list dimensions
func (l *ListComponent) SetDimensions(width, height int) {
	l.list.SetWidth(width)
	l.list.SetHeight(height)
}

// SetShowTitle sets whether to show the title
func (l *ListComponent) SetShowTitle(show bool) {
	l.list.SetShowTitle(show)
}

// SetStyles sets custom styles for the list
func (l *ListComponent) SetStyles(styles list.Styles) {
	l.list.Styles = styles
}

// NewStyledList creates a list with custom styling
func NewStyledList(id string, width, height int) *ListComponent {
	lc := NewList(id, width, height)
	
	// Apply custom styles
	s := list.DefaultStyles()
	s.Title = s.Title.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240"))
	s.PaginationStyle = list.DefaultStyles().
		PaginationStyle.
		PaddingLeft(4)
	s.HelpStyle = list.DefaultStyles().
		HelpStyle.
		PaddingLeft(4).
		PaddingBottom(1)
	
	lc.list.Styles = s
	
	return lc
}