package components

import (
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// TableComponent wraps the Bubble Tea table
type TableComponent struct {
	*BaseComponent
	table table.Model
}

// NewTable creates a new table component
func NewTable(id string) *TableComponent {
	base := NewBaseComponent(id)
	
	// Create default columns
	columns := []table.Column{
		{Title: "Column 1", Width: 20},
		{Title: "Column 2", Width: 20},
	}
	
	// Create default rows
	rows := []table.Row{}
	
	// Create table
	t := table.New(
		table.WithColumns(columns),
		table.WithRows(rows),
		table.WithFocused(true),
		table.WithHeight(10),
	)
	
	// Apply default styles
	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		BorderBottom(true).
		Bold(false)
	s.Selected = s.Selected.
		Foreground(lipgloss.Color("229")).
		Background(lipgloss.Color("57")).
		Bold(false)
	t.SetStyles(s)
	
	return &TableComponent{
		BaseComponent: base,
		table:         t,
	}
}

// Init initializes the table
func (t *TableComponent) Init() tea.Cmd {
	return nil
}

// Update handles table messages
func (t *TableComponent) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	
	// Update the underlying table
	t.table, cmd = t.table.Update(msg)
	
	// Store selected row in state
	t.state["selectedRow"] = t.table.SelectedRow()
	t.state["cursor"] = t.table.Cursor()
	
	return t, cmd
}

// View renders the table
func (t *TableComponent) View() string {
	// Update dimensions from props
	if width, ok := t.props["width"].(int); ok {
		t.table.SetWidth(width)
	}
	
	if height, ok := t.props["height"].(int); ok {
		t.table.SetHeight(height)
	}
	
	// Update focus from props
	if focused, ok := t.props["focused"].(bool); ok {
		t.table.Focus()
		if !focused {
			t.table.Blur()
		}
	}
	
	return t.table.View()
}

// SetColumns sets the table columns
func (t *TableComponent) SetColumns(columns []table.Column) {
	t.table.SetColumns(columns)
}

// SetRows sets the table rows
func (t *TableComponent) SetRows(rows []table.Row) {
	t.table.SetRows(rows)
}

// AddRow adds a row to the table
func (t *TableComponent) AddRow(row table.Row) {
	rows := t.table.Rows()
	rows = append(rows, row)
	t.table.SetRows(rows)
}

// RemoveRow removes a row at index
func (t *TableComponent) RemoveRow(index int) {
	rows := t.table.Rows()
	if index >= 0 && index < len(rows) {
		rows = append(rows[:index], rows[index+1:]...)
		t.table.SetRows(rows)
	}
}

// SelectedRow returns the currently selected row
func (t *TableComponent) SelectedRow() table.Row {
	return t.table.SelectedRow()
}

// SetDimensions sets the table dimensions
func (t *TableComponent) SetDimensions(width, height int) {
	t.table.SetWidth(width)
	t.table.SetHeight(height)
}

// SetStyles sets custom styles for the table
func (t *TableComponent) SetStyles(styles table.Styles) {
	t.table.SetStyles(styles)
}

// Focus focuses the table
func (t *TableComponent) Focus() {
	t.table.Focus()
}

// Blur unfocuses the table
func (t *TableComponent) Blur() {
	t.table.Blur()
}

// GotoTop moves cursor to the first row
func (t *TableComponent) GotoTop() {
	t.table.GotoTop()
}

// GotoBottom moves cursor to the last row
func (t *TableComponent) GotoBottom() {
	t.table.GotoBottom()
}

// Cursor returns the current cursor position
func (t *TableComponent) Cursor() int {
	return t.table.Cursor()
}

// SetCursor sets the cursor position
func (t *TableComponent) SetCursor(cursor int) {
	t.table.SetCursor(cursor)
}

// NewStyledTable creates a table with custom styling
func NewStyledTable(id string, columns []table.Column, rows []table.Row) *TableComponent {
	tc := NewTable(id)
	tc.SetColumns(columns)
	tc.SetRows(rows)
	
	// Apply custom styles
	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.ThickBorder()).
		BorderForeground(lipgloss.Color("240")).
		BorderBottom(true).
		Bold(true).
		Foreground(lipgloss.Color("229"))
	s.Selected = s.Selected.
		Foreground(lipgloss.Color("229")).
		Background(lipgloss.Color("57"))
	s.Cell = s.Cell.
		Padding(0, 1)
	
	tc.table.SetStyles(s)
	
	return tc
}