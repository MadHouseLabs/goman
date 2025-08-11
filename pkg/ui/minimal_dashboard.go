package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
	"github.com/madhouselabs/goman/pkg/models"
	"github.com/madhouselabs/goman/pkg/storage"
)

// Color palette matching form page for consistency
var (
	// Use exact same colors as form page (from internal/ui/viewport.go)
	ColorGreen   = lipgloss.Color("#10b981")
	ColorYellow  = lipgloss.Color("#f59e0b")
	ColorRed     = lipgloss.Color("#ef4444")
	ColorWhite   = lipgloss.Color("#ffffff")
	ColorGray    = lipgloss.Color("#6b7280")
	ColorBorder  = lipgloss.Color("#27272a")
	ColorPrimary = lipgloss.Color("#3b82f6") // Blue primary color
)

// RenderMinimalDashboard renders a clean, minimal professional dashboard
func RenderMinimalDashboard(width, height int, clusters []models.K3sCluster, states map[string]*storage.K3sClusterState, selectedIndex int) string {
	// Ensure minimum dimensions
	if width < 100 {
		width = 100
	}
	if height < 20 {
		height = 20
	}

	// Calculate layout dimensions
	headerHeight := 4   // Minimal header
	footerHeight := 3   // Minimal footer
	tableHeight := height - headerHeight - footerHeight

	// Build components
	header := renderMinimalHeader(width, len(clusters))
	tableView := renderMinimalTable(clusters, states, selectedIndex, width, tableHeight)
	footer := renderMinimalFooter(clusters, width)

	// Combine all components
	return lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		tableView,
		footer,
	)
}

// renderMinimalHeader creates a clean header matching form page style
func renderMinimalHeader(width int, clusterCount int) string {
	// Title bar - same style as form page
	titleStyle := lipgloss.NewStyle().
		Foreground(ColorWhite).
		Bold(true).
		Padding(0, 1)
	
	title := titleStyle.Render("CLUSTERS")
	
	// Title separator - same as form page
	separator := strings.Repeat("─", width)
	sepStyle := lipgloss.NewStyle().Foreground(ColorBorder)
	
	return lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		sepStyle.Render(separator),
	)
}

// renderMinimalTable creates the table with viewport
func renderMinimalTable(clusters []models.K3sCluster, states map[string]*storage.K3sClusterState, selectedIndex int, width int, height int) string {
	// Create table content
	tableContent := createMinimalTable(clusters, states, selectedIndex, width, height)
	
	// Create viewport for scrolling
	vp := viewport.New(width, height)
	vp.SetContent(tableContent)
	
	// Auto-scroll to selected item
	if len(clusters) > 0 && selectedIndex >= 0 {
		// Each row is 1 line, header is 2 lines
		selectedLine := selectedIndex + 2
		if selectedLine > height - 2 {
			vp.YOffset = selectedLine - (height / 2)
		}
	}
	
	return vp.View()
}

// createMinimalTable builds the table
func createMinimalTable(clusters []models.K3sCluster, states map[string]*storage.K3sClusterState, selectedIndex int, width int, height int) string {
	if len(clusters) == 0 {
		emptyStyle := lipgloss.NewStyle().
			Foreground(ColorGray).
			Italic(true).
			MarginTop(height/2).
			Width(width).
			Align(lipgloss.Center)
		return emptyStyle.Render("No clusters found. Press 'c' to create a new cluster.")
	}

	// Calculate column widths proportionally
	totalWidth := width - 4
	columns := []table.Column{
		{Title: "Name", Width: totalWidth * 22 / 100},
		{Title: "Status", Width: totalWidth * 12 / 100},
		{Title: "Region", Width: totalWidth * 12 / 100},
		{Title: "Mode", Width: totalWidth * 8 / 100},
		{Title: "Nodes", Width: totalWidth * 8 / 100},
		{Title: "Type", Width: totalWidth * 15 / 100},
		{Title: "IP Address", Width: totalWidth * 18 / 100},
		{Title: "Age", Width: totalWidth * 5 / 100},
	}

	// Build rows
	var rows []table.Row
	for _, cluster := range clusters {
		// Status without color codes (table will handle styling)
		var statusText string
		switch cluster.Status {
		case models.StatusRunning:
			statusText = "● Running"
		case models.StatusCreating:
			statusText = "◐ Creating"
		case models.StatusUpdating:
			statusText = "◐ Updating"
		case models.StatusError:
			statusText = "✗ Error"
		case models.StatusDeleting:
			statusText = "◐ Deleting"
		case models.StatusStopped:
			statusText = "○ Stopped"
		default:
			statusText = "○ " + string(cluster.Status)
		}
		// Don't apply color styling - let table handle plain text

		// Mode
		mode := string(cluster.Mode)
		if mode == "developer" {
			mode = "Dev"
		} else if mode == "ha" {
			mode = "HA"
		} else if mode == "" {
			mode = "-"
		}

		// Nodes
		masterCount := len(cluster.MasterNodes)
		workerCount := len(cluster.WorkerNodes)
		nodes := fmt.Sprintf("%d/%d", masterCount, workerCount)

		// IP Address
		ipAddr := "-"
		if state, ok := states[cluster.Name]; ok && state != nil {
			if instances, ok := state.Metadata["instances"].(map[string]interface{}); ok {
				for name, instData := range instances {
					if strings.Contains(name, "master-0") {
						if inst, ok := instData.(map[string]interface{}); ok {
							if ip, ok := inst["public_ip"].(string); ok && ip != "" {
								ipAddr = ip
							}
							break
						}
					}
				}
			}
		}

		// Instance type
		instanceType := cluster.InstanceType
		if instanceType == "" {
			instanceType = "-"
		}

		// Age
		age := calculateAge(cluster.CreatedAt)

		rows = append(rows, table.Row{
			cluster.Name,
			statusText,
			cluster.Region,
			mode,
			nodes,
			instanceType,
			ipAddr,
			age,
		})
	}

	// Create table
	t := table.New(
		table.WithColumns(columns),
		table.WithRows(rows),
		table.WithFocused(true),
		table.WithHeight(height),
		table.WithWidth(width),
	)

	// Table styling matching form page contrast
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
		Foreground(ColorWhite)  // White for visibility on dark terminals
	
	t.SetStyles(s)
	
	if selectedIndex < len(clusters) && selectedIndex >= 0 {
		t.SetCursor(selectedIndex)
	}

	return t.View()
}

// renderMinimalFooter creates a footer matching form page style
func renderMinimalFooter(clusters []models.K3sCluster, width int) string {
	// Help text - centered like form page
	helpStyle := lipgloss.NewStyle().
		Foreground(ColorGray).
		Width(width).
		Align(lipgloss.Center)
	
	helpContent := helpStyle.Render("↑/↓: navigate • ↵: details • c: create • d: delete • s: sync • q: quit")
	
	// Status bar - same style as form page
	var running, total int
	for _, c := range clusters {
		total++
		if c.Status == models.StatusRunning {
			running++
		}
	}
	
	var statusColor lipgloss.Color
	var statusText string
	
	if running > 0 {
		statusColor = ColorGreen
		statusText = fmt.Sprintf("● %d of %d running", running, total)
	} else {
		statusColor = ColorGray
		statusText = fmt.Sprintf("○ No clusters running")
	}
	
	statusStyle := lipgloss.NewStyle().
		Foreground(statusColor).
		Width(width).
		Padding(0, 1)
	
	return lipgloss.JoinVertical(
		lipgloss.Left,
		helpContent,
		statusStyle.Render(statusText),
	)
}

// calculateAge returns a human-readable age string
func calculateAge(createdAt time.Time) string {
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