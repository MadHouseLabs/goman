package components

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/madhouselabs/goman/pkg/models"
	"github.com/madhouselabs/goman/pkg/storage"
)

// ClusterTableComponent is a specialized table for displaying clusters
type ClusterTableComponent struct {
	*BaseComponent
	table         table.Model
	clusters      []models.K3sCluster
	clusterStates map[string]*storage.K3sClusterState
	width         int
	height        int
}

// NewClusterTable creates a new cluster table component
func NewClusterTable(id string) *ClusterTableComponent {
	base := NewBaseComponent(id)
	
	// Create initial table
	t := table.New(
		table.WithColumns([]table.Column{}),
		table.WithRows([]table.Row{}),
		table.WithFocused(true),
		table.WithHeight(10),
	)
	
	// Apply styles
	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		BorderBottom(true).
		Bold(true)
	s.Selected = s.Selected.
		Foreground(lipgloss.Color("255")).
		Background(lipgloss.Color("33")).  // Blue color
		Bold(false)
	t.SetStyles(s)
	
	return &ClusterTableComponent{
		BaseComponent: base,
		table:         t,
		clusters:      []models.K3sCluster{},
		clusterStates: make(map[string]*storage.K3sClusterState),
		width:         80,
		height:        20,
	}
}

// Init initializes the component
func (c *ClusterTableComponent) Init() tea.Cmd {
	return nil
}

// Update handles messages
func (c *ClusterTableComponent) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	
	// Handle window size changes
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		c.SetDimensions(msg.Width, msg.Height-6) // Account for header/footer
	}
	
	// Update the underlying table
	c.table, cmd = c.table.Update(msg)
	
	// Store state
	c.state["selectedRow"] = c.table.SelectedRow()
	c.state["cursor"] = c.table.Cursor()
	c.state["selectedCluster"] = c.GetSelectedCluster()
	
	return c, cmd
}

// View renders the component
func (c *ClusterTableComponent) View() string {
	// Make sure table has rows if we have clusters
	if len(c.clusters) > 0 && len(c.table.Rows()) == 0 {
		c.updateTable()
	}
	return c.table.View()
}

// SetClusters sets the cluster data and updates the table
func (c *ClusterTableComponent) SetClusters(clusters []models.K3sCluster, states map[string]*storage.K3sClusterState) {
	c.clusters = clusters
	c.clusterStates = states
	// Force table update with new data
	c.updateTable()
	// Make sure the table knows it has rows
	if len(clusters) > 0 && len(c.table.Rows()) == 0 {
		// Something went wrong, try again
		c.updateTable()
	}
}

// GetClusters returns the current clusters
func (c *ClusterTableComponent) GetClusters() []models.K3sCluster {
	return c.clusters
}

// GetSelectedCluster returns the currently selected cluster
func (c *ClusterTableComponent) GetSelectedCluster() *models.K3sCluster {
	cursor := c.table.Cursor()
	if cursor >= 0 && cursor < len(c.clusters) {
		return &c.clusters[cursor]
	}
	return nil
}

// GetSelectedIndex returns the selected index
func (c *ClusterTableComponent) GetSelectedIndex() int {
	return c.table.Cursor()
}

// SetDimensions sets the table dimensions
func (c *ClusterTableComponent) SetDimensions(width, height int) {
	c.width = width
	c.height = height
	c.table.SetWidth(width)
	c.table.SetHeight(height)
	c.updateTableLayout()
}

// updateTable rebuilds the table with current data
func (c *ClusterTableComponent) updateTable() {
	// Build columns first
	c.updateTableLayout()
	
	// Build rows
	var rows []table.Row
	for _, cluster := range c.clusters {
		row := c.buildClusterRow(cluster)
		rows = append(rows, row)
	}
	
	// Update the existing table instead of recreating it
	// This preserves keyboard handling and focus state
	c.table.SetRows(rows)
	
	// Ensure height is set
	if c.height > 4 {
		c.table.SetHeight(c.height - 2)
	}
}

// updateTableLayout updates column widths based on current width
func (c *ClusterTableComponent) updateTableLayout() {
	// Calculate column widths - ensure they add up to fill the width
	totalWidth := c.width
	if totalWidth < 100 {
		totalWidth = 100
	}
	
	// Distribute width across columns (total should be close to 100%)
	nameWidth := totalWidth * 20 / 100
	statusWidth := totalWidth * 12 / 100
	regionWidth := totalWidth * 12 / 100
	modeWidth := totalWidth * 8 / 100
	nodesWidth := totalWidth * 10 / 100
	instanceWidth := totalWidth * 14 / 100
	ipWidth := totalWidth * 16 / 100
	ageWidth := totalWidth * 8 / 100
	
	columns := []table.Column{
		{Title: "Name", Width: nameWidth},
		{Title: "Status", Width: statusWidth},
		{Title: "Region", Width: regionWidth},
		{Title: "Mode", Width: modeWidth},
		{Title: "Nodes", Width: nodesWidth},
		{Title: "Instance", Width: instanceWidth},
		{Title: "IP Address", Width: ipWidth},
		{Title: "Age", Width: ageWidth},
	}
	
	c.table.SetColumns(columns)
}

// buildClusterRow builds a table row for a cluster
func (c *ClusterTableComponent) buildClusterRow(cluster models.K3sCluster) table.Row {
	// Status with icon
	statusText := c.getStatusText(cluster.Status)
	
	// Mode
	mode := c.getModeText(cluster.Mode)
	
	// Nodes
	nodes := c.getNodeCount(cluster)
	
	// IP Address from state
	ipAddr := c.getIPAddress(cluster.Name)
	
	// Instance type
	instanceType := cluster.InstanceType
	if instanceType == "" {
		instanceType = "-"
	}
	
	// Age
	age := c.calculateAge(cluster.CreatedAt)
	
	// Ensure no empty strings that might cause rendering issues
	name := cluster.Name
	if name == "" {
		name = "unknown"
	}
	region := cluster.Region
	if region == "" {
		region = "-"
	}
	
	// Return row data without padding - let the table handle it
	return table.Row{
		name,
		statusText,
		region,
		mode,
		nodes,
		instanceType,
		ipAddr,
		age,
	}
}

// padRight pads a string to the right with spaces
func (c *ClusterTableComponent) padRight(s string, width int) string {
	if len(s) >= width {
		return s[:width]
	}
	return s + strings.Repeat(" ", width-len(s))
}

// getColumnWidth gets the width of a column by index
func (c *ClusterTableComponent) getColumnWidth(index int) int {
	totalWidth := c.width
	if totalWidth < 100 {
		totalWidth = 100
	}
	
	widths := []int{
		totalWidth * 20 / 100, // Name
		totalWidth * 12 / 100, // Status
		totalWidth * 12 / 100, // Region
		totalWidth * 8 / 100,  // Mode
		totalWidth * 10 / 100, // Nodes
		totalWidth * 14 / 100, // Instance
		totalWidth * 16 / 100, // IP
		totalWidth * 8 / 100,  // Age
	}
	
	if index >= 0 && index < len(widths) {
		return widths[index]
	}
	return 10
}

// getStatusText returns formatted status text
func (c *ClusterTableComponent) getStatusText(status models.ClusterStatus) string {
	switch status {
	case models.StatusRunning:
		return "● Running"
	case models.StatusCreating:
		return "◐ Creating"
	case models.StatusUpdating:
		return "◐ Updating"
	case models.StatusError:
		return "✗ Error"
	case models.StatusDeleting:
		return "◐ Deleting"
	case models.StatusStopped:
		return "○ Stopped"
	default:
		return "○ " + string(status)
	}
}

// getModeText returns formatted mode text
func (c *ClusterTableComponent) getModeText(mode models.ClusterMode) string {
	switch mode {
	case models.ModeDeveloper:
		return "Dev"
	case models.ModeHA:
		return "HA"
	default:
		return "-"
	}
}

// getNodeCount returns formatted node count
func (c *ClusterTableComponent) getNodeCount(cluster models.K3sCluster) string {
	masterCount := len(cluster.MasterNodes)
	workerCount := len(cluster.WorkerNodes)
	return fmt.Sprintf("%d/%d", masterCount, workerCount)
}

// getIPAddress retrieves IP address from cluster state
func (c *ClusterTableComponent) getIPAddress(clusterName string) string {
	if state, ok := c.clusterStates[clusterName]; ok && state != nil {
		if instances, ok := state.Metadata["instances"].(map[string]interface{}); ok {
			for name, instData := range instances {
				if strings.Contains(name, "master-0") {
					if inst, ok := instData.(map[string]interface{}); ok {
						if ip, ok := inst["public_ip"].(string); ok && ip != "" {
							return ip
						}
					}
				}
			}
		}
	}
	return "-"
}

// calculateAge returns human-readable age
func (c *ClusterTableComponent) calculateAge(createdAt time.Time) string {
	if createdAt.IsZero() {
		return "-"
	}
	
	duration := time.Since(createdAt)
	
	if duration < time.Minute {
		return "now"
	} else if duration < time.Hour {
		return fmt.Sprintf("%dm", int(duration.Minutes()))
	} else if duration < 24*time.Hour {
		return fmt.Sprintf("%dh", int(duration.Hours()))
	} else {
		return fmt.Sprintf("%dd", int(duration.Hours()/24))
	}
}

// Focus sets focus on the table
func (c *ClusterTableComponent) Focus() {
	c.table.Focus()
}

// Blur removes focus from the table
func (c *ClusterTableComponent) Blur() {
	c.table.Blur()
}

// Cursor returns the current cursor position
func (c *ClusterTableComponent) Cursor() int {
	return c.table.Cursor()
}

// SetCursor sets the cursor position
func (c *ClusterTableComponent) SetCursor(cursor int) {
	c.table.SetCursor(cursor)
}