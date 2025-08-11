package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/lipgloss"
	"github.com/madhouselabs/goman/pkg/models"
	"github.com/madhouselabs/goman/pkg/storage"
)

// RenderClusterTable renders a proper table component with full height
func RenderClusterTable(width, height int, clusters []models.K3sCluster, states map[string]*storage.K3sClusterState, selectedIndex int) string {
	// Ensure minimum dimensions
	if width < 80 {
		width = 80
	}
	if height < 10 {
		height = 10
	}

	// Calculate column widths to use full terminal width
	// Reserve some space for borders and padding (approximately 10 chars)
	availableWidth := width - 10
	
	// Define proportional column widths
	var columns []table.Column
	
	if width >= 140 {
		// Wide terminal - use generous spacing
		nameWidth := availableWidth * 20 / 100     // 20%
		statusWidth := availableWidth * 15 / 100   // 15%
		regionWidth := availableWidth * 15 / 100   // 15%
		modeWidth := availableWidth * 10 / 100     // 10%
		nodesWidth := availableWidth * 10 / 100    // 10%
		instanceWidth := availableWidth * 15 / 100 // 15%
		ipWidth := availableWidth * 15 / 100       // 15%
		
		columns = []table.Column{
			{Title: "Name", Width: nameWidth},
			{Title: "Status", Width: statusWidth},
			{Title: "Region", Width: regionWidth},
			{Title: "Mode", Width: modeWidth},
			{Title: "Nodes", Width: nodesWidth},
			{Title: "Instance", Width: instanceWidth},
			{Title: "Master IP", Width: ipWidth},
		}
	} else if width >= 100 {
		// Medium terminal
		nameWidth := availableWidth * 22 / 100     // 22%
		statusWidth := availableWidth * 14 / 100   // 14%
		regionWidth := availableWidth * 14 / 100   // 14%
		modeWidth := availableWidth * 10 / 100     // 10%
		nodesWidth := availableWidth * 10 / 100    // 10%
		instanceWidth := availableWidth * 15 / 100 // 15%
		ipWidth := availableWidth * 15 / 100       // 15%
		
		columns = []table.Column{
			{Title: "Name", Width: nameWidth},
			{Title: "Status", Width: statusWidth},
			{Title: "Region", Width: regionWidth},
			{Title: "Mode", Width: modeWidth},
			{Title: "Nodes", Width: nodesWidth},
			{Title: "Instance", Width: instanceWidth},
			{Title: "Master IP", Width: ipWidth},
		}
	} else {
		// Narrow terminal - use minimum widths
		nameWidth := availableWidth * 25 / 100     // 25%
		statusWidth := availableWidth * 15 / 100   // 15%
		regionWidth := availableWidth * 15 / 100   // 15%
		modeWidth := availableWidth * 10 / 100     // 10%
		nodesWidth := availableWidth * 10 / 100    // 10%
		instanceWidth := availableWidth * 12 / 100 // 12%
		ipWidth := availableWidth * 13 / 100       // 13%
		
		columns = []table.Column{
			{Title: "Name", Width: nameWidth},
			{Title: "Status", Width: statusWidth},
			{Title: "Region", Width: regionWidth},
			{Title: "Mode", Width: modeWidth},
			{Title: "Nodes", Width: nodesWidth},
			{Title: "Instance", Width: instanceWidth},
			{Title: "Master IP", Width: ipWidth},
		}
	}

	// Build rows
	var rows []table.Row
	for _, cluster := range clusters {
		// Calculate nodes
		masterCount := len(cluster.MasterNodes)
		workerCount := len(cluster.WorkerNodes)
		nodeInfo := fmt.Sprintf("%dM/%dW", masterCount, workerCount)

		// Format status
		statusText := string(cluster.Status)
		switch cluster.Status {
		case models.StatusRunning:
			statusText = "● " + statusText
		case models.StatusCreating, models.StatusUpdating:
			statusText = "◐ " + statusText
		case models.StatusError, models.StatusDeleting:
			statusText = "✗ " + statusText
		case models.StatusStopped:
			statusText = "○ " + statusText
		default:
			statusText = "○ " + statusText
		}

		// Format mode
		modeStr := string(cluster.Mode)
		if modeStr == "" {
			modeStr = "-"
		} else if modeStr == "developer" {
			modeStr = "dev"
		} else if modeStr == "ha" {
			modeStr = "HA"
		}

		// Get IP from state if available
		masterIP := "-"
		if state, ok := states[cluster.Name]; ok && state != nil {
			// Try to get first master's IP from instances metadata
			if instances, ok := state.Metadata["instances"].(map[string]interface{}); ok {
				// Find first master node
				for name, instData := range instances {
					if strings.Contains(name, "master-0") {
						if inst, ok := instData.(map[string]interface{}); ok {
							if ip, ok := inst["public_ip"].(string); ok && ip != "" {
								masterIP = ip
							} else if ip, ok := inst["private_ip"].(string); ok && ip != "" {
								masterIP = ip + " (pvt)"
							}
							break
						}
					}
				}
			}
		}

		// Format instance type
		instanceType := cluster.InstanceType
		if instanceType == "" {
			instanceType = "-"
		}

		// Add row
		rows = append(rows, table.Row{
			cluster.Name,
			statusText,
			cluster.Region,
			modeStr,
			nodeInfo,
			instanceType,
			masterIP,
		})
	}

	// Create table
	t := table.New(
		table.WithColumns(columns),
		table.WithRows(rows),
		table.WithFocused(true),
		table.WithHeight(height - 6), // Leave room for title and footer
		table.WithWidth(width),        // Use full terminal width
	)

	// Style the table
	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("#606060")).
		BorderBottom(true).
		Bold(true).
		Foreground(lipgloss.Color("#909090"))
	
	s.Selected = s.Selected.
		Foreground(lipgloss.Color("#FFFFFF")).
		Background(lipgloss.Color("#303030")).
		Bold(true)
	
	t.SetStyles(s)

	// Set cursor to match selected index
	if selectedIndex < len(clusters) {
		t.SetCursor(selectedIndex)
	}

	// Build the complete view
	var view strings.Builder

	// Title
	titleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FFFFFF")).
		Bold(true).
		Padding(0, 1)
	view.WriteString(titleStyle.Render("Clusters"))
	view.WriteString("\n")

	// Table
	view.WriteString(t.View())
	view.WriteString("\n")

	// Footer with summary and help
	footerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#606060"))
	
	// Summary stats
	runningCount := 0
	for _, c := range clusters {
		if c.Status == models.StatusRunning {
			runningCount++
		}
	}
	
	summaryStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#909090"))
	
	activeStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#52C41A"))
	
	summary := fmt.Sprintf("  %s %d cluster(s)  |  %s %d running",
		summaryStyle.Render("Total:"),
		len(clusters),
		activeStyle.Render("Active:"),
		runningCount)
	
	view.WriteString(footerStyle.Render(strings.Repeat("─", width)))
	view.WriteString("\n")
	view.WriteString(summary)
	view.WriteString("\n")
	
	// Help text
	helpStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#606060"))
	help := helpStyle.Render("  ↑/↓:navigate  ↵:details  c:create  d:delete  s:sync  q:quit")
	view.WriteString(help)

	return view.String()
}