package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/madhouselabs/goman/pkg/models"
	"github.com/madhouselabs/goman/pkg/storage"
	"github.com/charmbracelet/bubbles/table"
	"github.com/madhouselabs/goman/pkg/tui/components"
)

// ClusterListView is an improved cluster list UI with table, viewport, and status indicators
type ClusterListView struct {
	width         int
	height        int
	clusters      []models.K3sCluster
	states        map[string]*storage.K3sClusterState
	selectedIndex int
	table         *components.TableComponent
	viewport      *components.ViewportComponent
	connectionStatus ConnectionStatus
}

// ConnectionStatus represents the connection state
type ConnectionStatus struct {
	IsConnected bool
	Provider    string
	Region      string
	LastSync    time.Time
	Error       error
}

// NewClusterListView creates a new cluster list view
func NewClusterListView(width, height int) *ClusterListView {
	// Create table component
	table := components.NewTable("cluster-table")
	
	// Create viewport for scrolling
	viewport := components.NewViewport("cluster-viewport", width-2, height-6) // Account for header and footer
	
	return &ClusterListView{
		width:    width,
		height:   height,
		table:    table,
		viewport: viewport,
		states:   make(map[string]*storage.K3sClusterState),
	}
}

// SetClusters updates the cluster list
func (v *ClusterListView) SetClusters(clusters []models.K3sCluster, states map[string]*storage.K3sClusterState) {
	v.clusters = clusters
	v.states = states
	v.updateTable()
}

// SetSelectedIndex sets the selected cluster index
func (v *ClusterListView) SetSelectedIndex(index int) {
	v.selectedIndex = index
	// Table cursor is set when updating the table
}

// SetConnectionStatus updates the connection status
func (v *ClusterListView) SetConnectionStatus(status ConnectionStatus) {
	v.connectionStatus = status
}

// SetDimensions updates the view dimensions
func (v *ClusterListView) SetDimensions(width, height int) {
	v.width = width
	v.height = height
	if v.viewport != nil {
		v.viewport.SetDimensions(width-2, height-6)
	}
	v.updateTable()
}

// updateTable updates the table content
func (v *ClusterListView) updateTable() {
	if v.table == nil {
		return
	}

	// Calculate column widths based on available width
	totalWidth := v.width - 4
	
	// Define columns with proportional widths
	columns := []table.Column{
		{Title: "Name", Width: totalWidth * 20 / 100},
		{Title: "Status", Width: totalWidth * 12 / 100},
		{Title: "Region", Width: totalWidth * 12 / 100},
		{Title: "Mode", Width: totalWidth * 8 / 100},
		{Title: "Nodes", Width: totalWidth * 10 / 100},
		{Title: "Instance", Width: totalWidth * 12 / 100},
		{Title: "IP Address", Width: totalWidth * 15 / 100},
		{Title: "Age", Width: totalWidth * 8 / 100},
		{Title: "Cost/hr", Width: totalWidth * 8 / 100},
	}

	// Build rows (table.Row is []string)
	var rows []table.Row
	for _, cluster := range v.clusters {
		row := v.buildClusterRow(cluster)
		rows = append(rows, row)
	}

	// Update table
	v.table.SetColumns(columns)
	v.table.SetRows(rows)
	
	// Update viewport with table content
	if v.viewport != nil {
		v.viewport.SetContent(v.table.View())
	}
}

// buildClusterRow builds a table row for a cluster
func (v *ClusterListView) buildClusterRow(cluster models.K3sCluster) []string {
	// Status with icon
	statusText := v.getStatusText(cluster.Status)
	
	// Mode (shortened)
	mode := v.getModeText(cluster.Mode)
	
	// Node count
	nodeCount := v.getNodeCount(cluster)
	
	// IP Address from state
	ipAddr := v.getIPAddress(cluster.Name)
	
	// Instance type
	instanceType := cluster.InstanceType
	if instanceType == "" {
		instanceType = "-"
	}
	
	// Age
	age := v.calculateAge(cluster.CreatedAt)
	
	// Estimated cost
	cost := v.estimateCost(cluster)
	
	return []string{
		cluster.Name,
		statusText,
		cluster.Region,
		mode,
		nodeCount,
		instanceType,
		ipAddr,
		age,
		cost,
	}
}

// getStatusText returns formatted status text with icon
func (v *ClusterListView) getStatusText(status models.ClusterStatus) string {
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

// getModeText returns shortened mode text
func (v *ClusterListView) getModeText(mode models.ClusterMode) string {
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
func (v *ClusterListView) getNodeCount(cluster models.K3sCluster) string {
	masterCount := len(cluster.MasterNodes)
	workerCount := len(cluster.WorkerNodes)
	total := masterCount + workerCount
	return fmt.Sprintf("%d (%dM/%dW)", total, masterCount, workerCount)
}

// getIPAddress retrieves IP address from cluster state
func (v *ClusterListView) getIPAddress(clusterName string) string {
	if state, ok := v.states[clusterName]; ok && state != nil {
		if instances, ok := state.Metadata["instances"].(map[string]interface{}); ok {
			for name, instData := range instances {
				if strings.Contains(name, "master-0") {
					if inst, ok := instData.(map[string]interface{}); ok {
						if ip, ok := inst["public_ip"].(string); ok && ip != "" {
							return ip
						}
					}
					break
				}
			}
		}
	}
	return "-"
}

// calculateAge returns human-readable age
func (v *ClusterListView) calculateAge(createdAt time.Time) string {
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
	} else if duration < 7*24*time.Hour {
		return fmt.Sprintf("%dd", int(duration.Hours()/24))
	} else if duration < 30*24*time.Hour {
		return fmt.Sprintf("%dw", int(duration.Hours()/(24*7)))
	} else {
		return fmt.Sprintf("%dmo", int(duration.Hours()/(24*30)))
	}
}

// estimateCost estimates hourly cost based on instance type and count
func (v *ClusterListView) estimateCost(cluster models.K3sCluster) string {
	// Simple cost estimation based on instance type
	// These are rough estimates and should be replaced with actual pricing data
	costPerHour := 0.0
	
	switch cluster.InstanceType {
	case "t3.micro":
		costPerHour = 0.0104
	case "t3.small":
		costPerHour = 0.0208
	case "t3.medium":
		costPerHour = 0.0416
	case "t3.large":
		costPerHour = 0.0832
	case "t3.xlarge":
		costPerHour = 0.1664
	case "t3.2xlarge":
		costPerHour = 0.3328
	default:
		return "-"
	}
	
	// Calculate total cost
	totalNodes := len(cluster.MasterNodes) + len(cluster.WorkerNodes)
	if totalNodes == 0 {
		return "-"
	}
	
	totalCost := costPerHour * float64(totalNodes)
	return fmt.Sprintf("$%.2f", totalCost)
}

// Render renders the complete cluster list view
func (v *ClusterListView) Render() string {
	// For now, use the working minimal dashboard with enhanced header/footer
	return v.renderWithBubbleTable()
}

// renderWithBubbleTable uses bubble tea table directly for proper interactivity
func (v *ClusterListView) renderWithBubbleTable() string {
	// Build header
	header := v.renderHeader()
	
	// Build table content with bubble tea table
	tableContent := v.renderBubbleTable()
	
	// Build footer
	footer := v.renderFooter()
	
	// Combine all sections
	return lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		tableContent,
		footer,
	)
}

// renderBubbleTable creates an interactive bubble tea table
func (v *ClusterListView) renderBubbleTable() string {
	height := v.height - 4 // Account for header and footer
	
	if len(v.clusters) == 0 {
		return v.renderEmptyState()
	}
	
	// Calculate column widths
	totalWidth := v.width - 4
	
	columns := []table.Column{
		{Title: "Name", Width: totalWidth * 20 / 100},
		{Title: "Status", Width: totalWidth * 12 / 100},
		{Title: "Region", Width: totalWidth * 12 / 100},
		{Title: "Mode", Width: totalWidth * 8 / 100},
		{Title: "Nodes", Width: totalWidth * 10 / 100},
		{Title: "Instance", Width: totalWidth * 12 / 100},
		{Title: "IP Address", Width: totalWidth * 15 / 100},
		{Title: "Age", Width: totalWidth * 8 / 100},
		{Title: "Cost/hr", Width: totalWidth * 8 / 100},
	}
	
	// Build rows
	var rows []table.Row
	for _, cluster := range v.clusters {
		row := v.buildClusterRow(cluster)
		rows = append(rows, row)
	}
	
	// Create table
	t := table.New(
		table.WithColumns(columns),
		table.WithRows(rows),
		table.WithFocused(true),
		table.WithHeight(height),
		table.WithWidth(v.width),
	)
	
	// Apply styling
	s := table.DefaultStyles()
	
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(ColorBorder).
		BorderBottom(true).
		Bold(true).
		Foreground(ColorWhite)
	
	s.Selected = s.Selected.
		Foreground(ColorWhite).
		Background(ColorPrimary).
		Bold(true)
	
	s.Cell = s.Cell.
		Foreground(ColorWhite)
	
	t.SetStyles(s)
	
	// Set cursor to selected index
	if v.selectedIndex >= 0 && v.selectedIndex < len(v.clusters) {
		t.SetCursor(v.selectedIndex)
	}
	
	return t.View()
}

// renderHeader renders the header with title and connection status
func (v *ClusterListView) renderHeader() string {
	// Title
	titleStyle := lipgloss.NewStyle().
		Foreground(ColorWhite).
		Bold(true).
		Padding(0, 1)
	
	title := titleStyle.Render("GOMAN CLUSTERS")
	
	// Connection status on the same line
	statusStr := v.renderConnectionStatus()
	
	// Calculate padding for right alignment
	titleWidth := lipgloss.Width(title)
	statusWidth := lipgloss.Width(statusStr)
	padding := v.width - titleWidth - statusWidth - 2
	if padding < 0 {
		padding = 1
	}
	
	// Combine title and status
	headerLine := lipgloss.JoinHorizontal(
		lipgloss.Top,
		title,
		strings.Repeat(" ", padding),
		statusStr,
		" ",
	)
	
	// Separator
	separator := strings.Repeat("─", v.width)
	sepStyle := lipgloss.NewStyle().Foreground(ColorBorder)
	
	return lipgloss.JoinVertical(
		lipgloss.Left,
		headerLine,
		sepStyle.Render(separator),
	)
}

// renderConnectionStatus renders the connection status indicator
func (v *ClusterListView) renderConnectionStatus() string {
	var statusIcon string
	var statusColor lipgloss.Color
	var statusText string
	
	if v.connectionStatus.Error != nil {
		statusIcon = "✗"
		statusColor = ColorRed
		statusText = "Connection Error"
	} else if v.connectionStatus.IsConnected {
		statusIcon = "●"
		statusColor = ColorGreen
		if v.connectionStatus.Provider != "" {
			statusText = fmt.Sprintf("Connected to %s (%s)", v.connectionStatus.Provider, v.connectionStatus.Region)
		} else {
			statusText = "Connected"
		}
	} else {
		statusIcon = "○"
		statusColor = ColorGray
		statusText = "Disconnected"
	}
	
	// Add last sync time if available
	if !v.connectionStatus.LastSync.IsZero() && v.connectionStatus.IsConnected {
		syncAge := time.Since(v.connectionStatus.LastSync)
		if syncAge < time.Minute {
			statusText += " • Just synced"
		} else if syncAge < time.Hour {
			statusText += fmt.Sprintf(" • Synced %dm ago", int(syncAge.Minutes()))
		} else {
			statusText += fmt.Sprintf(" • Synced %dh ago", int(syncAge.Hours()))
		}
	}
	
	style := lipgloss.NewStyle().Foreground(statusColor)
	return style.Render(fmt.Sprintf("%s %s", statusIcon, statusText))
}

// renderContent renders the main content area with table
func (v *ClusterListView) renderContent() string {
	if len(v.clusters) == 0 {
		return v.renderEmptyState()
	}
	
	// Use viewport for scrollable content
	if v.viewport != nil {
		return v.viewport.View()
	}
	
	// Fallback to table view
	if v.table != nil {
		return v.table.View()
	}
	
	return ""
}

// renderEmptyState renders the empty state message
func (v *ClusterListView) renderEmptyState() string {
	height := v.height - 4 // Account for header and footer
	
	emptyStyle := lipgloss.NewStyle().
		Foreground(ColorGray).
		Italic(true).
		Width(v.width).
		Height(height).
		Align(lipgloss.Center, lipgloss.Center)
	
	message := "No clusters found\n\nPress 'c' to create your first cluster"
	return emptyStyle.Render(message)
}

// renderFooter renders the footer with stats and keyboard shortcuts
func (v *ClusterListView) renderFooter() string {
	// Separator
	separator := strings.Repeat("─", v.width)
	sepStyle := lipgloss.NewStyle().Foreground(ColorBorder)
	
	// Left side: cluster statistics
	statsStr := v.renderStatistics()
	
	// Right side: keyboard shortcuts
	shortcutsStr := v.renderKeyboardShortcuts()
	
	// Calculate padding
	statsWidth := lipgloss.Width(statsStr)
	shortcutsWidth := lipgloss.Width(shortcutsStr)
	padding := v.width - statsWidth - shortcutsWidth - 2
	if padding < 0 {
		padding = 1
	}
	
	// Combine stats and shortcuts
	footerLine := lipgloss.JoinHorizontal(
		lipgloss.Top,
		" ",
		statsStr,
		strings.Repeat(" ", padding),
		shortcutsStr,
		" ",
	)
	
	return lipgloss.JoinVertical(
		lipgloss.Left,
		sepStyle.Render(separator),
		footerLine,
	)
}

// renderStatistics renders cluster statistics
func (v *ClusterListView) renderStatistics() string {
	var running, creating, error, total int
	var totalCost float64
	
	for _, cluster := range v.clusters {
		total++
		switch cluster.Status {
		case models.StatusRunning:
			running++
		case models.StatusCreating, models.StatusUpdating:
			creating++
		case models.StatusError:
			error++
		}
		
		// Add to cost calculation
		if cost := v.estimateCost(cluster); cost != "-" {
			var costVal float64
			fmt.Sscanf(cost, "$%f", &costVal)
			totalCost += costVal
		}
	}
	
	// Build status text
	var parts []string
	
	if total == 0 {
		parts = append(parts, "No clusters")
	} else {
		parts = append(parts, fmt.Sprintf("%d cluster%s", total, pluralize(total)))
		
		if running > 0 {
			style := lipgloss.NewStyle().Foreground(ColorGreen)
			parts = append(parts, style.Render(fmt.Sprintf("%d running", running)))
		}
		
		if creating > 0 {
			style := lipgloss.NewStyle().Foreground(ColorYellow)
			parts = append(parts, style.Render(fmt.Sprintf("%d creating", creating)))
		}
		
		if error > 0 {
			style := lipgloss.NewStyle().Foreground(ColorRed)
			parts = append(parts, style.Render(fmt.Sprintf("%d error", error)))
		}
		
		if totalCost > 0 {
			costStyle := lipgloss.NewStyle().Foreground(ColorPrimary)
			parts = append(parts, costStyle.Render(fmt.Sprintf("~$%.2f/hr", totalCost)))
		}
	}
	
	return strings.Join(parts, " • ")
}

// renderKeyboardShortcuts renders keyboard shortcuts
func (v *ClusterListView) renderKeyboardShortcuts() string {
	shortcuts := []string{
		"↑↓/jk: navigate",
		"↵: details",
		"c: create",
		"d: delete",
		"s: sync",
		"r: refresh",
		"q: quit",
	}
	
	style := lipgloss.NewStyle().Foreground(ColorGray)
	return style.Render(strings.Join(shortcuts, " • "))
}

// pluralize returns "s" for plural
func pluralize(count int) string {
	if count == 1 {
		return ""
	}
	return "s"
}