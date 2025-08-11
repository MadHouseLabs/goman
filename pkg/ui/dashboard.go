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

// Color palette for professional look
var (
	// Primary colors
	primaryColor   = lipgloss.Color("#00D9FF") // Cyan
	secondaryColor = lipgloss.Color("#FF00FF") // Magenta
	accentColor    = lipgloss.Color("#00FF88") // Green
	
	// Status colors
	successColor = lipgloss.Color("#00FF00")
	warningColor = lipgloss.Color("#FFB000")
	errorColor   = lipgloss.Color("#FF3333")
	
	// UI colors
	bgColor       = lipgloss.Color("#0A0E27") // Dark blue background
	bgAltColor    = lipgloss.Color("#1C1E33") // Slightly lighter
	borderColor   = lipgloss.Color("#383B5B") // Border
	textColor     = lipgloss.Color("#E0E0E0") // Light gray text
	dimTextColor  = lipgloss.Color("#808080") // Dimmed text
	headerBgColor = lipgloss.Color("#1A1D3A") // Header background
)

// RenderDashboard renders the main cluster dashboard with professional styling
func RenderDashboard(width, height int, clusters []models.K3sCluster, states map[string]*storage.K3sClusterState, selectedIndex int) string {
	// Ensure minimum dimensions
	if width < 100 {
		width = 100
	}
	if height < 20 {
		height = 20
	}

	// Calculate layout dimensions
	headerHeight := 6  // Header with title and info
	footerHeight := 5  // Footer with stats and controls
	availableHeight := height - headerHeight - footerHeight

	// Build components
	header := renderHeader(width)
	tableContent := renderTableContent(clusters, states, selectedIndex, width, availableHeight)
	footer := renderFooter(clusters, width)

	// Combine all components
	return lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		tableContent,
		footer,
	)
}

// renderHeader creates a professional header with title and system info
func renderHeader(width int) string {
	// Main container style
	headerStyle := lipgloss.NewStyle().
		Width(width).
		Background(headerBgColor).
		Padding(0, 2)

	// Title with ASCII styling
	titleText := `╔═╗╔═╗╔╦╗╔═╗╔╗╔
║ ╦║ ║║║║╠═╣║║║
╚═╝╚═╝╩ ╩╩ ╩╝╚╝ CLUSTER MANAGER`
	
	titleStyle := lipgloss.NewStyle().
		Foreground(primaryColor).
		Bold(true)

	// System info bar
	timestamp := time.Now().Format("15:04:05 MST")
	systemInfo := fmt.Sprintf("│ AWS Provider │ Auto-Sync ON │ %s │", timestamp)
	
	infoStyle := lipgloss.NewStyle().
		Foreground(dimTextColor).
		Align(lipgloss.Right).
		Width(width - 4)

	// Status indicator
	statusDot := lipgloss.NewStyle().
		Foreground(successColor).
		Render("●")
	
	statusText := lipgloss.NewStyle().
		Foreground(textColor).
		Render(" System Online")

	statusLine := statusDot + statusText

	// Separator
	separator := lipgloss.NewStyle().
		Foreground(borderColor).
		Render(strings.Repeat("═", width-4))

	// Combine header elements
	content := lipgloss.JoinVertical(
		lipgloss.Left,
		titleStyle.Render(titleText),
		infoStyle.Render(systemInfo),
		statusLine,
		separator,
	)

	return headerStyle.Render(content)
}

// renderTableContent creates the scrollable table section
func renderTableContent(clusters []models.K3sCluster, states map[string]*storage.K3sClusterState, selectedIndex int, width int, height int) string {
	// Container with border
	containerStyle := lipgloss.NewStyle().
		Width(width).
		Height(height).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Background(bgColor)

	// Calculate available space inside container
	innerWidth := width - 2  // Account for borders
	innerHeight := height - 2

	// Create table
	table := createClusterTable(clusters, states, selectedIndex, innerWidth, innerHeight)
	
	// Create viewport for scrolling
	vp := viewport.New(innerWidth, innerHeight)
	vp.SetContent(table)

	// Auto-scroll to selected item
	if len(clusters) > 0 && selectedIndex >= 0 {
		// Each row is approximately 1 line
		if selectedIndex > innerHeight/2 {
			vp.LineDown(selectedIndex - innerHeight/2)
		}
	}

	return containerStyle.Render(vp.View())
}

// createClusterTable builds the table content
func createClusterTable(clusters []models.K3sCluster, states map[string]*storage.K3sClusterState, selectedIndex int, width int, height int) string {
	// Calculate column widths (proportional to available width)
	totalWidth := width - 2
	nameWidth := totalWidth * 25 / 100
	statusWidth := totalWidth * 15 / 100
	regionWidth := totalWidth * 12 / 100
	modeWidth := totalWidth * 10 / 100
	nodesWidth := totalWidth * 10 / 100
	instanceWidth := totalWidth * 13 / 100
	ipWidth := totalWidth * 15 / 100

	columns := []table.Column{
		{Title: "CLUSTER", Width: nameWidth},
		{Title: "STATUS", Width: statusWidth},
		{Title: "REGION", Width: regionWidth},
		{Title: "MODE", Width: modeWidth},
		{Title: "NODES", Width: nodesWidth},
		{Title: "INSTANCE", Width: instanceWidth},
		{Title: "IP ADDRESS", Width: ipWidth},
	}

	// Build rows with enhanced styling
	var rows []table.Row
	for _, cluster := range clusters {
		// Node count
		masterCount := len(cluster.MasterNodes)
		workerCount := len(cluster.WorkerNodes)
		nodeInfo := fmt.Sprintf("%d/%d", masterCount, workerCount)

		// Status with icon
		var statusIcon, statusText string
		switch cluster.Status {
		case models.StatusRunning:
			statusIcon = "▣"
			statusText = "Running"
		case models.StatusCreating:
			statusIcon = "◈"
			statusText = "Creating"
		case models.StatusUpdating:
			statusIcon = "◉"
			statusText = "Updating"
		case models.StatusError:
			statusIcon = "▥"
			statusText = "Error"
		case models.StatusDeleting:
			statusIcon = "◈"
			statusText = "Deleting"
		case models.StatusStopped:
			statusIcon = "□"
			statusText = "Stopped"
		default:
			statusIcon = "○"
			statusText = string(cluster.Status)
		}
		status := fmt.Sprintf("%s %s", statusIcon, statusText)

		// Mode formatting
		mode := string(cluster.Mode)
		if mode == "developer" {
			mode = "DEV"
		} else if mode == "ha" {
			mode = "HA"
		} else if mode == "" {
			mode = "—"
		}

		// Get IP address
		ipAddr := "—"
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
		instance := cluster.InstanceType
		if instance == "" {
			instance = "—"
		}

		rows = append(rows, table.Row{
			cluster.Name,
			status,
			cluster.Region,
			mode,
			nodeInfo,
			instance,
			ipAddr,
		})
	}

	// Create and style the table
	t := table.New(
		table.WithColumns(columns),
		table.WithRows(rows),
		table.WithFocused(true),
		table.WithHeight(height),
		table.WithWidth(width),
	)

	// Professional table styling
	s := table.DefaultStyles()
	
	// Header style
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(borderColor).
		BorderBottom(true).
		Bold(true).
		Foreground(primaryColor).
		Background(bgAltColor)
	
	// Selected row style
	s.Selected = s.Selected.
		Foreground(bgColor).
		Background(accentColor).
		Bold(true)
	
	// Cell style
	s.Cell = s.Cell.
		Foreground(textColor)
	
	t.SetStyles(s)
	
	// Set cursor position
	if selectedIndex < len(clusters) && selectedIndex >= 0 {
		t.SetCursor(selectedIndex)
	}

	return t.View()
}

// renderFooter creates the footer with statistics and controls
func renderFooter(clusters []models.K3sCluster, width int) string {
	// Footer container
	footerStyle := lipgloss.NewStyle().
		Width(width).
		Background(headerBgColor).
		Padding(0, 2)

	// Calculate statistics
	var running, stopped, creating, errors int
	totalCPU := 0
	totalMemory := 0
	
	for _, c := range clusters {
		switch c.Status {
		case models.StatusRunning:
			running++
		case models.StatusStopped:
			stopped++
		case models.StatusCreating, models.StatusUpdating:
			creating++
		case models.StatusError:
			errors++
		}
		totalCPU += c.TotalCPU
		totalMemory += c.TotalMemoryGB
	}

	// Stats bar with colored indicators
	statsLine := fmt.Sprintf(
		"│ %s %d │ %s %d │ %s %d │ %s %d │ CPU: %d cores │ RAM: %d GB │",
		lipgloss.NewStyle().Foreground(successColor).Render("●"),
		running,
		lipgloss.NewStyle().Foreground(dimTextColor).Render("○"),
		stopped,
		lipgloss.NewStyle().Foreground(warningColor).Render("◈"),
		creating,
		lipgloss.NewStyle().Foreground(errorColor).Render("▥"),
		errors,
		totalCPU,
		totalMemory,
	)

	statsStyle := lipgloss.NewStyle().
		Foreground(textColor).
		Width(width - 4).
		Align(lipgloss.Center)

	// Separator
	separator := lipgloss.NewStyle().
		Foreground(borderColor).
		Render(strings.Repeat("═", width-4))

	// Control hints with key highlighting
	keyStyle := lipgloss.NewStyle().
		Foreground(primaryColor).
		Bold(true)
	
	textStyle := lipgloss.NewStyle().
		Foreground(dimTextColor)

	controls := fmt.Sprintf("%s %s  %s %s  %s %s  %s %s  %s %s  %s %s  %s %s",
		keyStyle.Render("[↑↓]"),
		textStyle.Render("Navigate"),
		keyStyle.Render("[↵]"),
		textStyle.Render("Details"),
		keyStyle.Render("[c]"),
		textStyle.Render("Create"),
		keyStyle.Render("[d]"),
		textStyle.Render("Delete"),
		keyStyle.Render("[s]"),
		textStyle.Render("Sync"),
		keyStyle.Render("[r]"),
		textStyle.Render("Refresh"),
		keyStyle.Render("[q]"),
		textStyle.Render("Quit"),
	)

	controlsStyle := lipgloss.NewStyle().
		Width(width - 4).
		Align(lipgloss.Center)

	// Version/Mode indicator
	modeIndicator := lipgloss.NewStyle().
		Foreground(secondaryColor).
		Italic(true).
		Render("◆ Serverless Mode • v1.0.0")

	// Combine footer elements
	content := lipgloss.JoinVertical(
		lipgloss.Left,
		separator,
		statsStyle.Render(statsLine),
		controlsStyle.Render(controls),
		modeIndicator,
	)

	return footerStyle.Render(content)
}