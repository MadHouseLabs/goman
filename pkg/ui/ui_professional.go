package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/madhouselabs/goman/pkg/models"
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

// RenderProListWithWidth renders list with specific width
func RenderProListWithWidth(clusters []models.K3sCluster, selectedIndex int, width int) string {
	var b strings.Builder
	
	// Simple list without extra styling
	for i, cluster := range clusters {
		indicator := "  "
		if i == selectedIndex {
			indicator = "▸ "
		}
		
		// Calculate total nodes
		nodeCount := len(cluster.MasterNodes) + len(cluster.WorkerNodes)
		
		// Format status with icon
		statusIcon := "○"
		switch cluster.Status {
		case models.StatusRunning:
			statusIcon = "●"
		case models.StatusCreating:
			statusIcon = "◐"
		case models.StatusError:
			statusIcon = "✗"
		}
		
		// Simple line format
		line := fmt.Sprintf("%s%-20s %s %-10s %d nodes", 
			indicator, 
			cluster.Name,
			statusIcon,
			cluster.Status,
			nodeCount)
		
		b.WriteString(line)
		if i < len(clusters)-1 {
			b.WriteString("\n")
		}
	}
	
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

	// Configuration section
	b.WriteString("  " + proDim.Render("CONFIGURATION") + "\n")
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