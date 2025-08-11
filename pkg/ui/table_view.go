package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
	"github.com/madhouselabs/goman/pkg/models"
	"github.com/madhouselabs/goman/pkg/storage"
)

// RenderTableView renders a full-height table view with scrolling and anchored footer
func RenderTableView(width, height int, clusters []models.K3sCluster, states map[string]*storage.K3sClusterState, selectedIndex int) string {
	// Ensure minimum dimensions
	if width < 80 {
		width = 80
	}
	if height < 10 {
		height = 10
	}
	
	// Calculate dimensions
	// Title: 1 line, separator: 1 line, footer separator: 1 line, summary: 1 line, help: 1 line, padding: 2
	footerHeight := 5
	headerHeight := 2
	availableHeight := height - footerHeight - headerHeight
	
	if availableHeight < 5 {
		availableHeight = 5
	}
	
	// Build the table content
	tableContent := buildTableContent(clusters, states, selectedIndex, width)
	
	// Create viewport for scrollable content
	vp := viewport.New(width, availableHeight)
	vp.SetContent(tableContent)
	
	// Calculate the position to ensure selected item is visible
	if len(clusters) > 0 {
		// Each cluster takes 1 line, plus headers (2 lines)
		selectedLine := selectedIndex + 2
		
		// Auto-scroll to keep selected item in view
		if selectedLine < vp.YOffset {
			vp.YOffset = selectedLine
		} else if selectedLine >= vp.YOffset+vp.Height {
			vp.YOffset = selectedLine - vp.Height + 1
		}
	}
	
	// Build footer
	footer := buildFooter(clusters, width)
	
	// Title
	titleStyle := lipgloss.NewStyle().
		Foreground(white).
		Bold(true).
		Padding(0, 1)
	
	title := titleStyle.Render("Clusters")
	
	// Separator
	separator := proDim.Render(strings.Repeat("─", width))
	
	// Combine all parts
	return lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		separator,
		vp.View(),
		footer,
	)
}

// buildTableContent builds the scrollable table content
func buildTableContent(clusters []models.K3sCluster, states map[string]*storage.K3sClusterState, selectedIndex int, width int) string {
	var rows []string
	
	// Define column widths based on terminal width
	var nameWidth, statusWidth, regionWidth, modeWidth, nodesWidth, instanceWidth, ipWidth int
	
	if width >= 140 {
		// Wide terminal
		nameWidth = 25
		statusWidth = 15
		regionWidth = 15
		modeWidth = 12
		nodesWidth = 12
		instanceWidth = 20
		ipWidth = 25
	} else if width >= 100 {
		// Medium terminal
		nameWidth = 20
		statusWidth = 12
		regionWidth = 12
		modeWidth = 10
		nodesWidth = 10
		instanceWidth = 15
		ipWidth = 18
	} else {
		// Narrow terminal
		nameWidth = 15
		statusWidth = 10
		regionWidth = 10
		modeWidth = 8
		nodesWidth = 8
		instanceWidth = 12
		ipWidth = 15
	}
	
	// Headers
	headerStyle := lipgloss.NewStyle().
		Foreground(mediumGray).
		Bold(true)
	
	headers := fmt.Sprintf("%-*s %-*s %-*s %-*s %-*s %-*s %-*s",
		nameWidth, "NAME",
		statusWidth, "STATUS",
		regionWidth, "REGION",
		modeWidth, "MODE",
		nodesWidth, "NODES",
		instanceWidth, "INSTANCE",
		ipWidth, "MASTER IP",
	)
	rows = append(rows, headerStyle.Render(headers))
	sepWidth := width - 4
	if sepWidth < 0 {
		sepWidth = 0
	}
	rows = append(rows, proDim.Render(strings.Repeat("─", sepWidth)))
	
	// Cluster rows
	for i, cluster := range clusters {
		// Calculate nodes
		masterCount := len(cluster.MasterNodes)
		workerCount := len(cluster.WorkerNodes)
		nodeInfo := fmt.Sprintf("%dM/%dW", masterCount, workerCount)
		
		// Format status with color
		statusStyle := proDim
		statusIcon := "○"
		switch cluster.Status {
		case models.StatusRunning:
			statusStyle = proStatusGood
			statusIcon = "●"
		case models.StatusCreating, models.StatusUpdating:
			statusStyle = proStatusWarn
			statusIcon = "◐"
		case models.StatusError, models.StatusDeleting:
			statusStyle = proStatusBad
			statusIcon = "✗"
		case models.StatusStopped:
			statusIcon = "○"
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
		
		// Format instance type (shortened)
		instanceType := cluster.InstanceType
		if instanceType == "" {
			instanceType = "-"
		}
		
		// Build row
		rowStyle := proDim
		indicator := "  "
		if i == selectedIndex {
			rowStyle = proHighlight
			indicator = "▸ "
		}
		
		// Format the row with proper column widths
		row := fmt.Sprintf("%s%-*s %s %-*s %-*s %-*s %-*s %-*s %-*s",
			indicator,
			nameWidth-2, cluster.Name,
			statusIcon,
			statusWidth-2, string(cluster.Status),
			regionWidth, cluster.Region,
			modeWidth, modeStr,
			nodesWidth, nodeInfo,
			instanceWidth, instanceType,
			ipWidth, masterIP,
		)
		
		// Apply row styling
		if i == selectedIndex {
			rows = append(rows, proSelected.Render(row))
		} else {
			// Apply status color to the status column only
			parts := strings.SplitN(row, " ", 3)
			if len(parts) >= 3 {
				styledRow := parts[0] + " " + statusStyle.Render(parts[1]) + " " + proDim.Render(parts[2])
				rows = append(rows, styledRow)
			} else {
				rows = append(rows, rowStyle.Render(row))
			}
		}
	}
	
	// If no clusters, show empty message
	if len(clusters) == 0 {
		emptyMsg := proDim.Render("No clusters found. Press 'c' to create a new cluster.")
		rows = append(rows, "")
		rows = append(rows, emptyMsg)
	}
	
	return strings.Join(rows, "\n")
}

// buildFooter builds the anchored footer with summary and help
func buildFooter(clusters []models.K3sCluster, width int) string {
	var footer []string
	
	// Footer separator
	sepWidth := width - 4
	if sepWidth < 0 {
		sepWidth = 0
	}
	footer = append(footer, proDim.Render(strings.Repeat("─", sepWidth)))
	
	// Summary stats
	runningCount := 0
	for _, c := range clusters {
		if c.Status == models.StatusRunning {
			runningCount++
		}
	}
	
	summaryRow := fmt.Sprintf("  %s %d cluster(s)  |  %s %d running",
		proDim.Render("Total:"),
		len(clusters),
		proStatusGood.Render("Active:"),
		runningCount)
	footer = append(footer, summaryRow)
	
	// Help text
	helpText := proHelp.Render("  ↑/↓:navigate  ↵:details  c:create  d:delete  s:sync  q:quit")
	footer = append(footer, helpText)
	
	return strings.Join(footer, "\n")
}