package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/madhouselabs/goman/pkg/models"
	"github.com/madhouselabs/goman/pkg/storage"
)

// Professional color palette - improved contrast
var (
	// Monochrome base with better contrast
	white       = lipgloss.Color("#FFFFFF")
	lightGray   = lipgloss.Color("#D0D0D0")  // Brightened for better readability
	mediumGray  = lipgloss.Color("#909090")  // Increased contrast
	darkGray    = lipgloss.Color("#505050")  // Slightly lighter
	black       = lipgloss.Color("#000000")
	
	// Accent colors with better visibility
	subtleGreen  = lipgloss.Color("#52C41A")  // Brighter green
	subtleRed    = lipgloss.Color("#FF4D4F")  // More visible red
	subtleYellow = lipgloss.Color("#FAAD14")  // Warmer yellow
	subtleBlue   = lipgloss.Color("#1890FF")  // Clearer blue
	
	// Background
	bgDark = lipgloss.Color("#0A0A0A")
	bgCard = lipgloss.Color("#1A1A1A")  // Slightly lighter for contrast
)

// Professional styles - minimal and clean
var (
	proBase = lipgloss.NewStyle().
		Background(bgDark).
		Foreground(lightGray)

	proTitle = lipgloss.NewStyle().
		Foreground(white).
		Bold(true)

	proSubtitle = lipgloss.NewStyle().
		Foreground(mediumGray)

	proDim = lipgloss.NewStyle().
		Foreground(mediumGray)

	proHighlight = lipgloss.NewStyle().
		Foreground(white)

	proSelected = lipgloss.NewStyle().
		Foreground(white).
		Bold(true)

	proStatusGood = lipgloss.NewStyle().
		Foreground(subtleGreen)

	proStatusBad = lipgloss.NewStyle().
		Foreground(subtleRed)

	proStatusWarn = lipgloss.NewStyle().
		Foreground(subtleYellow)

	proStatusInfo = lipgloss.NewStyle().
		Foreground(subtleBlue)

	proLabel = lipgloss.NewStyle().
		Foreground(lightGray)  // Brighter labels

	proValue = lipgloss.NewStyle().
		Foreground(white)  // White for values

	proHelp = lipgloss.NewStyle().
		Foreground(mediumGray)  // More visible help text

	proDivider = lipgloss.NewStyle().
		Foreground(darkGray)
)

// formatAge formats a timestamp into a human-readable age
func formatAge(timestamp string) string {
	// For now, return a simple format
	// You can enhance this to show "2h ago", "3d ago", etc.
	if timestamp == "" {
		return "-"
	}
	return timestamp[:10] // Just show date for now
}

// RenderProList renders a professional list view with proper alignment
func RenderProList(clusters []models.K3sCluster, selectedIndex int) string {
	return RenderProListWithWidth(clusters, selectedIndex, 120)
}

// RenderProListWithStatesAndWidth renders list with full state information including IPs and responsive width
func RenderProListWithStatesAndWidth(clusters []models.K3sCluster, states map[string]*storage.K3sClusterState, selectedIndex int, termWidth int) string {
	// Calculate table width (leave some padding)
	tableWidth := termWidth - 4
	if tableWidth < 80 {
		tableWidth = 80
	} else if tableWidth > 160 {
		tableWidth = 160
	}
	// Define column widths
	const (
		nameWidth     = 20
		statusWidth   = 15
		regionWidth   = 15
		modeWidth     = 10
		nodesWidth    = 10
		instanceWidth = 15
		ipWidth       = 20
	)
	
	// Create styles for table components
	tableStyle := lipgloss.NewStyle().
		Width(tableWidth).
		Padding(1, 2)
	
	headerStyle := lipgloss.NewStyle().
		Foreground(mediumGray).
		Bold(true)
	
	rowStyle := lipgloss.NewStyle()
	
	selectedRowStyle := lipgloss.NewStyle().
		Foreground(white).
		Bold(true)
	
	// Build the table
	var rows []string
	
	// Calculate separator width (table width minus padding)
	sepWidth := tableWidth - 4
	
	// Title
	titleRow := proTitle.Render("KUBERNETES CLUSTERS")
	rows = append(rows, titleRow)
	rows = append(rows, proDim.Render(strings.Repeat("═", sepWidth)))
	rows = append(rows, "") // Empty line
	
	// Headers
	headers := lipgloss.JoinHorizontal(
		lipgloss.Top,
		headerStyle.Width(nameWidth).Render("NAME"),
		headerStyle.Width(statusWidth).Render("STATUS"),
		headerStyle.Width(regionWidth).Render("REGION"),
		headerStyle.Width(modeWidth).Render("MODE"),
		headerStyle.Width(nodesWidth).Render("NODES"),
		headerStyle.Width(instanceWidth).Render("INSTANCE"),
		headerStyle.Width(ipWidth).Render("MASTER IP"),
	)
	rows = append(rows, headers)
	rows = append(rows, proDim.Render(strings.Repeat("─", sepWidth)))
	
	// List clusters
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
								masterIP = ip + " (private)"
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
		
		// Build row with lipgloss
		currentRowStyle := rowStyle
		indicator := "  "
		if i == selectedIndex {
			currentRowStyle = selectedRowStyle
			indicator = "▸ "
		}
		
		// Create cells with proper widths
		nameCell := currentRowStyle.Width(nameWidth).Render(indicator + cluster.Name)
		statusCell := lipgloss.NewStyle().Width(statusWidth).Render(statusIcon + " " + statusStyle.Render(string(cluster.Status)))
		regionCell := proDim.Width(regionWidth).Render(cluster.Region)
		modeCell := proDim.Width(modeWidth).Render(modeStr)
		nodesCell := proDim.Width(nodesWidth).Render(nodeInfo)
		instanceCell := proDim.Width(instanceWidth).Render(instanceType)
		ipCell := proValue.Width(ipWidth).Render(masterIP)
		
		// Join cells horizontally
		row := lipgloss.JoinHorizontal(
			lipgloss.Top,
			nameCell,
			statusCell,
			regionCell,
			modeCell,
			nodesCell,
			instanceCell,
			ipCell,
		)
		
		rows = append(rows, row)
	}
	
	// Add empty line if there are clusters
	if len(clusters) > 0 {
		rows = append(rows, "")
	}
	
	// Footer
	rows = append(rows, proDim.Render(strings.Repeat("─", sepWidth)))
	
	// Summary stats
	runningCount := 0
	for _, c := range clusters {
		if c.Status == models.StatusRunning {
			runningCount++
		}
	}
	summaryRow := fmt.Sprintf("%s %d cluster(s)  |  %s %d running", 
		proDim.Render("Total:"),
		len(clusters),
		proStatusGood.Render("Active:"),
		runningCount)
	rows = append(rows, summaryRow)
	
	// Help text
	rows = append(rows, "")
	rows = append(rows, proHelp.Render("↑/↓:navigate  ↵:details  c:create  d:delete  s:sync  q:quit"))
	
	// Join all rows vertically and apply table style
	content := lipgloss.JoinVertical(lipgloss.Left, rows...)
	return tableStyle.Render(content)
}

// RenderProListWithStates renders list with full state information including IPs
func RenderProListWithStates(clusters []models.K3sCluster, states map[string]*storage.K3sClusterState, selectedIndex int) string {
	return RenderProListWithStatesAndWidth(clusters, states, selectedIndex, 120)
}

// RenderProListWithWidth renders list with specific width
func RenderProListWithWidth(clusters []models.K3sCluster, selectedIndex int, width int) string {
	var b strings.Builder
	
	// Header
	b.WriteString("\n")
	b.WriteString("  " + proDim.Render("CLUSTERS") + "\n")
	b.WriteString("  " + proDim.Render(strings.Repeat("─", 80)) + "\n\n")
	
	// Column headers
	b.WriteString(fmt.Sprintf("  %-20s %-12s %-15s %-10s %-8s %-15s\n",
		proDim.Render("NAME"),
		proDim.Render("STATUS"),
		proDim.Render("REGION"),
		proDim.Render("MODE"),
		proDim.Render("NODES"),
		proDim.Render("INSTANCE")))
	b.WriteString("  " + proDim.Render(strings.Repeat("─", 80)) + "\n")
	
	// Simple list without extra styling
	for i, cluster := range clusters {
		indicator := "  "
		if i == selectedIndex {
			indicator = "▸ "
		}
		
		// Calculate total nodes
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
			modeStr = "unknown"
		}
		
		// Simple line format with more info
		line := fmt.Sprintf("%s%-20s %s %-11s %-15s %-10s %-8s %-15s",
			indicator,
			proValue.Render(cluster.Name),
			statusIcon,
			statusStyle.Render(fmt.Sprintf("%-10s", cluster.Status)),
			proDim.Render(cluster.Region),
			proDim.Render(modeStr),
			proDim.Render(nodeInfo),
			proDim.Render(cluster.InstanceType))
		
		b.WriteString(line)
		if i < len(clusters)-1 {
			b.WriteString("\n")
		}
	}
	
	// Footer with summary
	b.WriteString("\n\n  " + proDim.Render(strings.Repeat("─", 80)) + "\n")
	b.WriteString(fmt.Sprintf("  %s %d cluster(s)\n", 
		proDim.Render("Total:"),
		len(clusters)))
	
	// Help text
	b.WriteString("\n  " + proHelp.Render("↑/↓:navigate  ↵:details  c:create  d:delete  s:sync  q:quit") + "\n")
	
	return b.String()
}

// RenderProDetail renders professional detail view
func RenderProDetail(cluster models.K3sCluster) string {
	var b strings.Builder

	// Title with status
	b.WriteString("\n  " + proTitle.Render(strings.ToUpper(cluster.Name)) + "\n")
	
	// Status line
	statusStyle := proDim
	switch cluster.Status {
	case models.StatusRunning:
		statusStyle = proStatusGood
	case models.StatusError, models.StatusDeleting:
		statusStyle = proStatusBad
	case models.StatusCreating, models.StatusUpdating:
		statusStyle = proStatusWarn
	}
	b.WriteString("  " + statusStyle.Render(string(cluster.Status)) + "\n\n")

	// Basic Information section
	b.WriteString("  " + proDim.Render("CLUSTER INFORMATION") + "\n")
	b.WriteString("  " + proDim.Render(strings.Repeat("─", 40)) + "\n")
	
	basicInfo := [][]string{
		{"id", cluster.ID},
		{"region", cluster.Region},
		{"mode", string(cluster.Mode)},
		{"instance", cluster.InstanceType},
		{"created", cluster.CreatedAt.Format("2006-01-02 15:04")},
		{"updated", cluster.UpdatedAt.Format("2006-01-02 15:04")},
	}
	
	for _, info := range basicInfo {
		b.WriteString(fmt.Sprintf("  %-12s %s\n", 
			proLabel.Render(info[0]),
			proValue.Render(info[1])))
	}

	// Configuration section
	b.WriteString("\n  " + proDim.Render("CONFIGURATION") + "\n")
	b.WriteString("  " + proDim.Render(strings.Repeat("─", 40)) + "\n")
	
	configs := [][]string{
		{"version", cluster.K3sVersion},
		{"endpoint", cluster.APIEndpoint},
		{"network", cluster.NetworkCIDR},
		{"service", cluster.ServiceCIDR},
	}
	
	for _, cfg := range configs {
		b.WriteString(fmt.Sprintf("  %-12s %s\n", 
			proLabel.Render(cfg[0]),
			proValue.Render(cfg[1])))
	}

	// Nodes section
	b.WriteString("\n  " + proDim.Render("NODES") + "\n")
	b.WriteString("  " + proDim.Render(strings.Repeat("─", 40)) + "\n")
	
	for _, node := range cluster.MasterNodes {
		b.WriteString(fmt.Sprintf("  %-8s %-15s %-15s %dc/%dg\n",
			proLabel.Render("master"),
			proValue.Render(node.Name),
			proDim.Render(node.IP),
			node.CPU, node.MemoryGB))
	}
	
	for _, node := range cluster.WorkerNodes {
		b.WriteString(fmt.Sprintf("  %-8s %-15s %-15s %dc/%dg\n",
			proLabel.Render("worker"),
			proValue.Render(node.Name),
			proDim.Render(node.IP),
			node.CPU, node.MemoryGB))
	}

	// Summary
	b.WriteString("\n  " + proDim.Render("SUMMARY") + "\n")
	b.WriteString("  " + proDim.Render(strings.Repeat("─", 40)) + "\n")
	
	b.WriteString(fmt.Sprintf("  %-12s %d cores, %d GB memory, %d GB storage\n",
		proLabel.Render("resources"),
		cluster.TotalCPU,
		cluster.TotalMemoryGB,
		cluster.TotalStorageGB))
	
	if cluster.EstimatedCost > 0 {
		b.WriteString(fmt.Sprintf("  %-12s $%.2f/month\n",
			proLabel.Render("estimated"),
			cluster.EstimatedCost))
	}

	// Features (if any enabled)
	enabledFeatures := []string{}
	if cluster.Features.Traefik {
		enabledFeatures = append(enabledFeatures, "traefik")
	}
	if cluster.Features.MetricsServer {
		enabledFeatures = append(enabledFeatures, "metrics")
	}
	if cluster.Features.ServiceLB {
		enabledFeatures = append(enabledFeatures, "servicelb")
	}
	
	if len(enabledFeatures) > 0 {
		b.WriteString(fmt.Sprintf("  %-12s %s\n",
			proLabel.Render("features"),
			proValue.Render(strings.Join(enabledFeatures, ", "))))
	}

	return b.String()
}

// RenderProDetailWithState renders cluster detail view with full state information
func RenderProDetailWithState(cluster models.K3sCluster, state *storage.K3sClusterState) string {
	var b strings.Builder

	// Title with status
	b.WriteString("\n  " + proTitle.Render(strings.ToUpper(cluster.Name)) + "\n")
	
	// Status line
	statusStyle := proDim
	switch cluster.Status {
	case models.StatusRunning:
		statusStyle = proStatusGood
	case models.StatusError, models.StatusDeleting:
		statusStyle = proStatusBad
	case models.StatusCreating, models.StatusUpdating:
		statusStyle = proStatusWarn
	}
	b.WriteString("  " + statusStyle.Render(string(cluster.Status)) + "\n")
	
	// Show metadata message if available
	if state != nil && state.Metadata != nil {
		if msg, ok := state.Metadata["message"].(string); ok && msg != "" {
			b.WriteString("  " + proDim.Render(msg) + "\n")
		}
		if phase, ok := state.Metadata["phase"].(string); ok && phase != "" {
			b.WriteString("  " + proDim.Render("Phase: "+phase) + "\n")
		}
	}
	b.WriteString("\n")

	// Basic Information section
	b.WriteString("  " + proDim.Render("CLUSTER INFORMATION") + "\n")
	b.WriteString("  " + proDim.Render(strings.Repeat("─", 60)) + "\n")
	
	basicInfo := [][]string{
		{"id", cluster.ID},
		{"region", cluster.Region},
		{"mode", string(cluster.Mode)},
		{"instance", cluster.InstanceType},
		{"created", cluster.CreatedAt.Format("2006-01-02 15:04")},
		{"updated", cluster.UpdatedAt.Format("2006-01-02 15:04")},
	}
	
	for _, info := range basicInfo {
		b.WriteString(fmt.Sprintf("  %-12s %s\n", 
			proLabel.Render(info[0]),
			proValue.Render(info[1])))
	}

	// Instance Details section
	if state != nil && len(state.InstanceIDs) > 0 {
		b.WriteString("\n  " + proDim.Render("INSTANCES") + "\n")
		b.WriteString("  " + proDim.Render(strings.Repeat("─", 60)) + "\n")
		
		// Try to get instance details from metadata
		if instances, ok := state.Metadata["instances"].(map[string]interface{}); ok {
			for name, instData := range instances {
				if inst, ok := instData.(map[string]interface{}); ok {
					instanceID := ""
					if id, ok := inst["id"].(string); ok {
						instanceID = id
					}
					privateIP := "pending"
					if ip, ok := inst["private_ip"].(string); ok && ip != "" {
						privateIP = ip
					}
					publicIP := "pending"
					if ip, ok := inst["public_ip"].(string); ok && ip != "" {
						publicIP = ip
					}
					instanceState := "unknown"
					if st, ok := inst["state"].(string); ok {
						instanceState = st
					}
					role := "unknown"
					if r, ok := inst["role"].(string); ok && r != "" {
						role = r
					}
					
					// Format instance line
					stateStyle := proDim
					if instanceState == "running" {
						stateStyle = proStatusGood
					} else if instanceState == "pending" || instanceState == "initiating" {
						stateStyle = proStatusWarn
					} else if instanceState == "terminated" || instanceState == "terminating" {
						stateStyle = proStatusBad
					}
					
					b.WriteString(fmt.Sprintf("  %-20s %-12s %s\n",
						proLabel.Render(name),
						stateStyle.Render(instanceState),
						proDim.Render(role)))
					b.WriteString(fmt.Sprintf("    %-10s %s\n", 
						proDim.Render("id:"), 
						proValue.Render(instanceID)))
					b.WriteString(fmt.Sprintf("    %-10s %s\n", 
						proDim.Render("private:"), 
						proValue.Render(privateIP)))
					b.WriteString(fmt.Sprintf("    %-10s %s\n", 
						proDim.Render("public:"), 
						proValue.Render(publicIP)))
					b.WriteString("\n")
				}
			}
		} else {
			// Fallback to simple instance ID list
			for name, id := range state.InstanceIDs {
				b.WriteString(fmt.Sprintf("  %-20s %s\n", 
					proLabel.Render(name),
					proValue.Render(id)))
			}
		}
	}

	// Configuration section
	b.WriteString("  " + proDim.Render("CONFIGURATION") + "\n")
	b.WriteString("  " + proDim.Render(strings.Repeat("─", 60)) + "\n")
	
	configs := [][]string{
		{"version", cluster.K3sVersion},
		{"endpoint", cluster.APIEndpoint},
		{"network", cluster.NetworkCIDR},
		{"service", cluster.ServiceCIDR},
		{"dns", cluster.ClusterDNS},
	}
	
	for _, cfg := range configs {
		if cfg[1] != "" {
			b.WriteString(fmt.Sprintf("  %-12s %s\n", 
				proLabel.Render(cfg[0]),
				proValue.Render(cfg[1])))
		}
	}

	// Summary
	b.WriteString("\n  " + proDim.Render("SUMMARY") + "\n")
	b.WriteString("  " + proDim.Render(strings.Repeat("─", 60)) + "\n")
	
	b.WriteString(fmt.Sprintf("  %-12s %d cores, %d GB memory, %d GB storage\n",
		proLabel.Render("resources"),
		cluster.TotalCPU,
		cluster.TotalMemoryGB,
		cluster.TotalStorageGB))
	
	if cluster.EstimatedCost > 0 {
		b.WriteString(fmt.Sprintf("  %-12s $%.2f/month\n",
			proLabel.Render("estimated"),
			cluster.EstimatedCost))
	}

	// Features (if any enabled)
	enabledFeatures := []string{}
	if cluster.Features.Traefik {
		enabledFeatures = append(enabledFeatures, "traefik")
	}
	if cluster.Features.MetricsServer {
		enabledFeatures = append(enabledFeatures, "metrics")
	}
	if cluster.Features.ServiceLB {
		enabledFeatures = append(enabledFeatures, "servicelb")
	}
	
	if len(enabledFeatures) > 0 {
		b.WriteString(fmt.Sprintf("  %-12s %s\n",
			proLabel.Render("features"),
			proValue.Render(strings.Join(enabledFeatures, ", "))))
	}

	return b.String()
}

// Note: RenderProForm is removed as the ProForm struct handles its own rendering

// RenderProHelp renders minimal help text
func RenderProHelp(context string) string {
	helps := map[string]string{
		"list": "c:create  d:delete  s:start  x:stop  ↵:details  q:quit",
		"detail": "esc:back  e:edit  d:delete",
		"form": "tab:next  ↵:submit  esc:cancel",
		"loading": "",
	}

	help, ok := helps[context]
	if !ok || help == "" {
		return ""
	}

	return "\n  " + proHelp.Render(help)
}

// RenderProLoading renders minimal loading state
func RenderProLoading(message string) string {
	return "\n  " + proDim.Render("• "+message)
}

// RenderProMessage renders status messages
func RenderProMessage(message string, isError bool) string {
	style := proStatusGood
	if isError {
		style = proStatusBad
	}
	return "  " + style.Render(message) + "\n"
}

// Note: CreateProFormField is removed as ProForm handles its own fields

// RenderProFormMinimal renders an even more minimal form
func RenderProFormMinimal(title string, fields [][]string, focusIndex int) string {
	var b strings.Builder

	b.WriteString("\n  " + proTitle.Render(strings.ToUpper(title)) + "\n\n")

	for i, field := range fields {
		label := field[0]
		value := field[1]
		
		style := proDim
		cursor := " "
		if i == focusIndex {
			style = proHighlight
			cursor = ">"
		}

		b.WriteString(fmt.Sprintf("  %s %-18s %s\n",
			cursor,
			proLabel.Render(label),
			style.Render(value)))
	}

	return b.String()
}