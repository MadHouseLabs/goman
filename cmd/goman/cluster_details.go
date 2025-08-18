package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/madhouselabs/goman/pkg/models"
	"github.com/rivo/tview"
)

func showClusterDetails(cluster models.K3sCluster) {
	// Create main flex container (same structure as listing page)
	flex := tview.NewFlex().SetDirection(tview.FlexRow)
	
	// Create header with title (same style as listing page)
	headerFlex := tview.NewFlex().SetDirection(tview.FlexColumn)
	
	titleText := fmt.Sprintf(" %s%sCluster Details%s%s", TagBold, TagPrimary, TagReset, TagReset)
	title := tview.NewTextView().
		SetText(titleText).
		SetTextAlign(tview.AlignLeft).
		SetDynamicColors(true)
	
	// Cluster name and status on the right
	statusColor := TagSuccess
	statusIcon := "●"
	if cluster.Status == "creating" {
		statusColor = TagWarning
		statusIcon = "◐"
	} else if cluster.Status == "stopped" {
		statusColor = TagMuted
		statusIcon = "○"
	}
	
	clusterInfoText := fmt.Sprintf("%s%s %s %s%s %s", TagBold, cluster.Name, TagReset, statusColor, statusIcon, strings.ToUpper(string(cluster.Status)))
	clusterInfo := tview.NewTextView().
		SetText(clusterInfoText).
		SetTextAlign(tview.AlignRight).
		SetDynamicColors(true)
	
	headerFlex.
		AddItem(title, 0, 1, false).
		AddItem(clusterInfo, 0, 1, false)
	
	// Header divider
	headerDivider := tview.NewTextView().
		SetText(string(CharDivider)).
		SetTextColor(ColorMuted).
		SetDrawFunc(func(screen tcell.Screen, x, y, width, height int) (int, int, int, int) {
			for i := x; i < x+width; i++ {
				screen.SetContent(i, y, CharDivider, nil, StyleMuted)
			}
			return 0, 0, 0, 0
		})
	
	// Calculate totals
	totalNodes := len(cluster.MasterNodes) + len(cluster.WorkerNodes)
	totalCPU := 0
	totalMemory := 0
	totalStorage := 0
	for _, node := range cluster.MasterNodes {
		if node.CPU > 0 {
			totalCPU += node.CPU
		} else {
			totalCPU += 2 // default
		}
		if node.MemoryGB > 0 {
			totalMemory += node.MemoryGB
		} else {
			totalMemory += 4 // default
		}
		if node.StorageGB > 0 {
			totalStorage += node.StorageGB
		} else {
			totalStorage += 20 // default
		}
	}
	for _, node := range cluster.WorkerNodes {
		if node.CPU > 0 {
			totalCPU += node.CPU
		} else {
			totalCPU += 2
		}
		if node.MemoryGB > 0 {
			totalMemory += node.MemoryGB
		} else {
			totalMemory += 4
		}
		if node.StorageGB > 0 {
			totalStorage += node.StorageGB
		} else {
			totalStorage += 20
		}
	}
	
	// Create three sections: Cluster Info, Resources, Metrics
	
	// Section 1: Cluster Info
	clusterInfoTable := tview.NewTable().
		SetBorders(false).
		SetSelectable(false, false).
		SetFixed(1, 2)
	
	// Title spans both columns with padding
	clusterInfoTable.SetCell(0, 0, tview.NewTableCell(" [::b]Cluster Information").
		SetTextColor(ColorPrimary).
		SetAlign(tview.AlignLeft))
	clusterInfoTable.SetCell(0, 1, tview.NewTableCell(""))
	
	// Cluster info data with proper alignment
	infoRow := 1
	clusterInfoTable.SetCell(infoRow, 0, tview.NewTableCell(" Name:").
		SetTextColor(ColorMuted).
		SetAlign(tview.AlignLeft))
	clusterInfoTable.SetCell(infoRow, 1, tview.NewTableCell(cluster.Name).
		SetTextColor(ColorForeground).
		SetAlign(tview.AlignLeft))
	infoRow++
	
	statusColor2 := getStatusColor(string(cluster.Status))
	clusterInfoTable.SetCell(infoRow, 0, tview.NewTableCell(" Status:").
		SetTextColor(ColorMuted).
		SetAlign(tview.AlignLeft))
	clusterInfoTable.SetCell(infoRow, 1, tview.NewTableCell(string(cluster.Status)).
		SetTextColor(statusColor2).
		SetAlign(tview.AlignLeft))
	infoRow++
	
	clusterInfoTable.SetCell(infoRow, 0, tview.NewTableCell(" Mode:").
		SetTextColor(ColorMuted).
		SetAlign(tview.AlignLeft))
	clusterInfoTable.SetCell(infoRow, 1, tview.NewTableCell(strings.ToUpper(string(cluster.Mode))).
		SetTextColor(ColorForeground).
		SetAlign(tview.AlignLeft))
	infoRow++
	
	clusterInfoTable.SetCell(infoRow, 0, tview.NewTableCell(" Region:").
		SetTextColor(ColorMuted).
		SetAlign(tview.AlignLeft))
	clusterInfoTable.SetCell(infoRow, 1, tview.NewTableCell(cluster.Region).
		SetTextColor(ColorForeground).
		SetAlign(tview.AlignLeft))
	infoRow++
	
	clusterInfoTable.SetCell(infoRow, 0, tview.NewTableCell(" Created:").
		SetTextColor(ColorMuted).
		SetAlign(tview.AlignLeft))
	clusterInfoTable.SetCell(infoRow, 1, tview.NewTableCell(cluster.CreatedAt.Format("2006-01-02 15:04")).
		SetTextColor(ColorForeground).
		SetAlign(tview.AlignLeft))
	infoRow++
	
	clusterInfoTable.SetCell(infoRow, 0, tview.NewTableCell(" Updated:").
		SetTextColor(ColorMuted).
		SetAlign(tview.AlignLeft))
	clusterInfoTable.SetCell(infoRow, 1, tview.NewTableCell(cluster.UpdatedAt.Format("2006-01-02 15:04")).
		SetTextColor(ColorForeground).
		SetAlign(tview.AlignLeft))
	
	// Section 2: Resources  
	resourcesTable := tview.NewTable().
		SetBorders(false).
		SetSelectable(false, false).
		SetFixed(1, 2)
	
	// Title spans both columns with padding
	resourcesTable.SetCell(0, 0, tview.NewTableCell(" [::b]Resources").
		SetTextColor(ColorPrimary).
		SetAlign(tview.AlignLeft))
	resourcesTable.SetCell(0, 1, tview.NewTableCell(""))
	
	// Resources data with proper alignment
	resourceRow := 1
	resourcesTable.SetCell(resourceRow, 0, tview.NewTableCell(" Master Nodes:").
		SetTextColor(ColorMuted).
		SetAlign(tview.AlignLeft))
	resourcesTable.SetCell(resourceRow, 1, tview.NewTableCell(fmt.Sprintf("%d", len(cluster.MasterNodes))).
		SetTextColor(ColorAccent).
		SetAlign(tview.AlignLeft))
	resourceRow++
	
	resourcesTable.SetCell(resourceRow, 0, tview.NewTableCell(" Worker Nodes:").
		SetTextColor(ColorMuted).
		SetAlign(tview.AlignLeft))
	resourcesTable.SetCell(resourceRow, 1, tview.NewTableCell(fmt.Sprintf("%d", len(cluster.WorkerNodes))).
		SetTextColor(ColorAccent).
		SetAlign(tview.AlignLeft))
	resourceRow++
	
	resourcesTable.SetCell(resourceRow, 0, tview.NewTableCell(" Total Nodes:").
		SetTextColor(ColorMuted).
		SetAlign(tview.AlignLeft))
	resourcesTable.SetCell(resourceRow, 1, tview.NewTableCell(fmt.Sprintf("%d", totalNodes)).
		SetTextColor(ColorSuccess).
		SetAlign(tview.AlignLeft))
	resourceRow++
	
	resourcesTable.SetCell(resourceRow, 0, tview.NewTableCell(" Total CPU:").
		SetTextColor(ColorMuted).
		SetAlign(tview.AlignLeft))
	resourcesTable.SetCell(resourceRow, 1, tview.NewTableCell(fmt.Sprintf("%d vCPUs", totalCPU)).
		SetTextColor(ColorForeground).
		SetAlign(tview.AlignLeft))
	resourceRow++
	
	resourcesTable.SetCell(resourceRow, 0, tview.NewTableCell(" Total Memory:").
		SetTextColor(ColorMuted).
		SetAlign(tview.AlignLeft))
	resourcesTable.SetCell(resourceRow, 1, tview.NewTableCell(fmt.Sprintf("%d GB", totalMemory)).
		SetTextColor(ColorForeground).
		SetAlign(tview.AlignLeft))
	resourceRow++
	
	resourcesTable.SetCell(resourceRow, 0, tview.NewTableCell(" Total Storage:").
		SetTextColor(ColorMuted).
		SetAlign(tview.AlignLeft))
	resourcesTable.SetCell(resourceRow, 1, tview.NewTableCell(fmt.Sprintf("%d GB", totalStorage)).
		SetTextColor(ColorForeground).
		SetAlign(tview.AlignLeft))
	resourceRow++
	
	// Section 3: Metrics
	metricsTable := tview.NewTable().
		SetBorders(false).
		SetSelectable(false, false).
		SetFixed(1, 2)
	
	// Title spans both columns with padding
	metricsTable.SetCell(0, 0, tview.NewTableCell(" [::b]Metrics").
		SetTextColor(ColorPrimary).
		SetAlign(tview.AlignLeft))
	metricsTable.SetCell(0, 1, tview.NewTableCell(""))
	
	// Metrics data
	metricsRow := 1
	
	metricsTable.SetCell(metricsRow, 0, tview.NewTableCell(" Uptime:").
		SetTextColor(ColorMuted).
		SetAlign(tview.AlignLeft))
	uptime := time.Since(cluster.CreatedAt)
	uptimeStr := fmt.Sprintf("%d days, %d hours", int(uptime.Hours()/24), int(uptime.Hours())%24)
	metricsTable.SetCell(metricsRow, 1, tview.NewTableCell(uptimeStr).
		SetTextColor(ColorForeground).
		SetAlign(tview.AlignLeft))
	metricsRow++
	
	metricsTable.SetCell(metricsRow, 0, tview.NewTableCell(" Nodes Health:").
		SetTextColor(ColorMuted).
		SetAlign(tview.AlignLeft))
	runningNodes := 0
	for _, node := range cluster.MasterNodes {
		if node.Status == "running" {
			runningNodes++
		}
	}
	for _, node := range cluster.WorkerNodes {
		if node.Status == "running" {
			runningNodes++
		}
	}
	healthStr := fmt.Sprintf("%d/%d Running", runningNodes, totalNodes)
	healthColor := ColorSuccess
	if runningNodes < totalNodes {
		healthColor = ColorWarning
	}
	metricsTable.SetCell(metricsRow, 1, tview.NewTableCell(healthStr).
		SetTextColor(healthColor).
		SetAlign(tview.AlignLeft))
	metricsRow++
	
	metricsTable.SetCell(metricsRow, 0, tview.NewTableCell(" CPU Usage:").
		SetTextColor(ColorMuted).
		SetAlign(tview.AlignLeft))
	// Calculate current CPU usage (simulated for now)
	cpuUsage := "45%" // This would come from actual metrics
	cpuColor := ColorSuccess
	if strings.TrimSuffix(cpuUsage, "%") > "70" {
		cpuColor = ColorWarning
	}
	metricsTable.SetCell(metricsRow, 1, tview.NewTableCell(cpuUsage).
		SetTextColor(cpuColor).
		SetAlign(tview.AlignLeft))
	metricsRow++
	
	metricsTable.SetCell(metricsRow, 0, tview.NewTableCell(" Memory Usage:").
		SetTextColor(ColorMuted).
		SetAlign(tview.AlignLeft))
	// Calculate current memory usage (simulated for now)
	memUsage := "62%" // This would come from actual metrics
	memColor := ColorSuccess
	if strings.TrimSuffix(memUsage, "%") > "70" {
		memColor = ColorWarning
	}
	metricsTable.SetCell(metricsRow, 1, tview.NewTableCell(memUsage).
		SetTextColor(memColor).
		SetAlign(tview.AlignLeft))
	
	// Create vertical dividers using TextView
	divider1 := tview.NewTextView().
		SetText("").
		SetDrawFunc(func(screen tcell.Screen, x, y, width, height int) (int, int, int, int) {
			for i := y; i < y+height; i++ {
				screen.SetContent(x+1, i, '│', nil, StyleMuted)
			}
			return 0, 0, 0, 0
		})
	
	divider2 := tview.NewTextView().
		SetText("").
		SetDrawFunc(func(screen tcell.Screen, x, y, width, height int) (int, int, int, int) {
			for i := y; i < y+height; i++ {
				screen.SetContent(x+1, i, '│', nil, StyleMuted)
			}
			return 0, 0, 0, 0
		})
	
	// Section 4: Connection Info
	connectionTable := tview.NewTable().
		SetBorders(false).
		SetSelectable(false, false).
		SetFixed(1, 2)
	
	// Title
	connectionTable.SetCell(0, 0, tview.NewTableCell(" [::b]Connection").
		SetTextColor(ColorPrimary).
		SetAlign(tview.AlignLeft))
	connectionTable.SetCell(0, 1, tview.NewTableCell(""))
	
	// Connection data
	connRow := 1
	
	// Get master instance for connection
	masterInstanceID := ""
	if len(cluster.MasterNodes) > 0 {
		masterInstanceID = cluster.MasterNodes[0].ID
	}
	
	connectionTable.SetCell(connRow, 0, tview.NewTableCell(" Connect:").
		SetTextColor(ColorMuted).
		SetAlign(tview.AlignLeft))
	connectionTable.SetCell(connRow, 1, tview.NewTableCell(fmt.Sprintf("goman kubectl connect %s", cluster.Name)).
		SetTextColor(ColorAccent).
		SetAlign(tview.AlignLeft))
	connRow++
	
	connectionTable.SetCell(connRow, 0, tview.NewTableCell(" Master:").
		SetTextColor(ColorMuted).
		SetAlign(tview.AlignLeft))
	connectionTable.SetCell(connRow, 1, tview.NewTableCell(masterInstanceID).
		SetTextColor(ColorForeground).
		SetAlign(tview.AlignLeft))
	connRow++
	
	connectionTable.SetCell(connRow, 0, tview.NewTableCell(" Method:").
		SetTextColor(ColorMuted).
		SetAlign(tview.AlignLeft))
	connectionTable.SetCell(connRow, 1, tview.NewTableCell("SSM Port Forward").
		SetTextColor(ColorForeground).
		SetAlign(tview.AlignLeft))
	
	// Create a horizontal flex to arrange the three sections with dividers
	sectionsContainer := tview.NewFlex().SetDirection(tview.FlexColumn)
	sectionsContainer.AddItem(clusterInfoTable, 0, 1, false)
	sectionsContainer.AddItem(divider1, 3, 0, false)
	sectionsContainer.AddItem(resourcesTable, 0, 1, false)
	sectionsContainer.AddItem(divider2, 3, 0, false)
	sectionsContainer.AddItem(metricsTable, 0, 1, false)
	
	// Calculate node statistics first
	totalNodes = len(cluster.MasterNodes) + len(cluster.WorkerNodes)
	runningCount := 0
	stoppedCount := 0
	provisioningCount := 0
	
	// Count master node states
	for _, node := range cluster.MasterNodes {
		switch node.Status {
		case "running":
			runningCount++
		case "stopped":
			stoppedCount++
		default:
			provisioningCount++
		}
	}
	
	// Count worker node states
	for _, node := range cluster.WorkerNodes {
		switch node.Status {
		case "running":
			runningCount++
		case "stopped":
			stoppedCount++
		default:
			provisioningCount++
		}
	}
	
	// Calculate expected nodes (min/max)
	minNodes := 1 // At least 1 master
	maxNodes := totalNodes // Current total (can be expanded)
	if string(cluster.Mode) == "ha" || cluster.Mode == models.ModeHA {
		minNodes = 3 // HA mode requires 3 masters minimum
	}
	
	// Create node pools table with same design as cluster list
	nodePoolsTable := tview.NewTable().
		SetBorders(false).
		SetSelectable(true, false).
		SetSeparator(' ').
		SetSelectedStyle(StyleHighlight)
	
	// Add headers - same style as cluster list with proper spacing
	headers := []string{"  Pool Name", "Role", "Nodes", "Instance Type", "CPU", "Memory", "Status"}
	for col, header := range headers {
		alignment := tview.AlignLeft
		if col > 1 && col < 6 { // Center align numeric columns
			alignment = tview.AlignCenter
		}
		cell := tview.NewTableCell(header).
			SetTextColor(ColorPrimary).
			SetAlign(alignment).
			SetSelectable(false).
			SetExpansion(1)
		nodePoolsTable.SetCell(0, col, cell)
	}
	
	// Add control plane pool (always shown as it's a default pool)
	poolRow := 1
	
	// Always add control plane pool - this is mandatory for every cluster
	controlPlaneCount := len(cluster.MasterNodes)
	expectedControlPlaneCount := 1 // Default for developer mode
	if string(cluster.Mode) == "ha" || cluster.Mode == models.ModeHA {
		expectedControlPlaneCount = 3
	}
	
	controlPlaneStatus := "Running"
	controlPlaneColor := ColorSuccess
	
	if controlPlaneCount == 0 {
		controlPlaneStatus = "Not Provisioned"
		controlPlaneColor = ColorMuted
	} else if controlPlaneCount < expectedControlPlaneCount {
		controlPlaneStatus = fmt.Sprintf("Scaling (%d/%d)", controlPlaneCount, expectedControlPlaneCount)
		controlPlaneColor = ColorWarning
	} else {
		// Check if any master is not running
		for _, node := range cluster.MasterNodes {
			if node.Status != "running" {
				controlPlaneStatus = "Provisioning"
				controlPlaneColor = ColorWarning
				break
			}
		}
	}
	
	// Get control plane instance type
	controlPlaneInstanceType := cluster.InstanceType
	if controlPlaneInstanceType == "" {
		controlPlaneInstanceType = "t3.medium"
	}
	if len(cluster.MasterNodes) > 0 && cluster.MasterNodes[0].InstanceType != "" {
		controlPlaneInstanceType = cluster.MasterNodes[0].InstanceType
	}
	
	// Calculate control plane resources (show expected if no nodes yet)
	controlPlaneCPU := controlPlaneCount * 2
	controlPlaneMemory := controlPlaneCount * 4
	
	// If no nodes yet, show expected resources
	if controlPlaneCount == 0 {
		// Show expected resources based on instance type
		if controlPlaneInstanceType == "t3.medium" {
			controlPlaneCPU = expectedControlPlaneCount * 2
			controlPlaneMemory = expectedControlPlaneCount * 4
		} else if controlPlaneInstanceType == "t3.large" {
			controlPlaneCPU = expectedControlPlaneCount * 2
			controlPlaneMemory = expectedControlPlaneCount * 8
		} else if controlPlaneInstanceType == "t3.xlarge" {
			controlPlaneCPU = expectedControlPlaneCount * 4
			controlPlaneMemory = expectedControlPlaneCount * 16
		}
	} else if len(cluster.MasterNodes) > 0 {
		if cluster.MasterNodes[0].CPU > 0 {
			controlPlaneCPU = controlPlaneCount * cluster.MasterNodes[0].CPU
		}
		if cluster.MasterNodes[0].MemoryGB > 0 {
			controlPlaneMemory = controlPlaneCount * cluster.MasterNodes[0].MemoryGB
		}
	}
	
	nodePoolsTable.SetCell(poolRow, 0, tview.NewTableCell("  control-plane").
		SetTextColor(ColorForeground).
		SetAlign(tview.AlignLeft).
		SetExpansion(1))
	nodePoolsTable.SetCell(poolRow, 1, tview.NewTableCell("Control Plane").
		SetTextColor(ColorPrimary).
		SetAlign(tview.AlignLeft).
		SetExpansion(1))
	
	// Show expected count if different from actual
	nodeCountText := fmt.Sprintf("%d", controlPlaneCount)
	if controlPlaneCount != expectedControlPlaneCount {
		nodeCountText = fmt.Sprintf("%d/%d", controlPlaneCount, expectedControlPlaneCount)
	}
	nodePoolsTable.SetCell(poolRow, 2, tview.NewTableCell(nodeCountText).
		SetTextColor(ColorForeground).
		SetAlign(tview.AlignCenter).
		SetExpansion(1))
	nodePoolsTable.SetCell(poolRow, 3, tview.NewTableCell(controlPlaneInstanceType).
		SetTextColor(ColorForeground).
		SetAlign(tview.AlignCenter).
		SetExpansion(1))
	
	// Show CPU and Memory (even if 0)
	cpuText := fmt.Sprintf("%d", controlPlaneCPU)
	if controlPlaneCPU == 0 {
		cpuText = fmt.Sprintf("%d", expectedControlPlaneCount*2) // Default 2 vCPUs per node
	}
	memText := fmt.Sprintf("%d GB", controlPlaneMemory)
	if controlPlaneMemory == 0 {
		memText = fmt.Sprintf("%d GB", expectedControlPlaneCount*4) // Default 4 GB per node
	}
	
	nodePoolsTable.SetCell(poolRow, 4, tview.NewTableCell(cpuText).
		SetTextColor(ColorForeground).
		SetAlign(tview.AlignCenter).
		SetExpansion(1))
	nodePoolsTable.SetCell(poolRow, 5, tview.NewTableCell(memText).
		SetTextColor(ColorForeground).
		SetAlign(tview.AlignCenter).
		SetExpansion(1))
	nodePoolsTable.SetCell(poolRow, 6, tview.NewTableCell(controlPlaneStatus).
		SetTextColor(controlPlaneColor).
		SetAlign(tview.AlignLeft).
		SetExpansion(1))
	poolRow++
	
	// Add worker pool if there are worker nodes
	if len(cluster.WorkerNodes) > 0 {
		workerCount := len(cluster.WorkerNodes)
		workerStatus := "Running"
		workerColor := ColorSuccess
		
		// Check worker status
		for _, node := range cluster.WorkerNodes {
			if node.Status != "running" {
				workerStatus = "Scaling"
				workerColor = ColorWarning
				break
			}
		}
		
		// Get worker instance type
		workerInstanceType := cluster.InstanceType
		if workerInstanceType == "" {
			workerInstanceType = "t3.medium"
		}
		if cluster.WorkerNodes[0].InstanceType != "" {
			workerInstanceType = cluster.WorkerNodes[0].InstanceType
		}
		
		// Calculate worker resources
		workerCPU := workerCount * 2
		workerMemory := workerCount * 4
		if cluster.WorkerNodes[0].CPU > 0 {
			workerCPU = workerCount * cluster.WorkerNodes[0].CPU
		}
		if cluster.WorkerNodes[0].MemoryGB > 0 {
			workerMemory = workerCount * cluster.WorkerNodes[0].MemoryGB
		}
		
		nodePoolsTable.SetCell(poolRow, 0, tview.NewTableCell("  worker-pool-1").
			SetTextColor(ColorForeground).
			SetAlign(tview.AlignLeft).
			SetExpansion(1))
		nodePoolsTable.SetCell(poolRow, 1, tview.NewTableCell("Worker").
			SetTextColor(ColorAccent).
			SetAlign(tview.AlignLeft).
			SetExpansion(1))
		nodePoolsTable.SetCell(poolRow, 2, tview.NewTableCell(fmt.Sprintf("%d", workerCount)).
			SetTextColor(ColorForeground).
			SetAlign(tview.AlignCenter).
			SetExpansion(1))
		nodePoolsTable.SetCell(poolRow, 3, tview.NewTableCell(workerInstanceType).
			SetTextColor(ColorForeground).
			SetAlign(tview.AlignCenter).
			SetExpansion(1))
		nodePoolsTable.SetCell(poolRow, 4, tview.NewTableCell(fmt.Sprintf("%d", workerCPU)).
			SetTextColor(ColorForeground).
			SetAlign(tview.AlignCenter).
			SetExpansion(1))
		nodePoolsTable.SetCell(poolRow, 5, tview.NewTableCell(fmt.Sprintf("%d GB", workerMemory)).
			SetTextColor(ColorForeground).
			SetAlign(tview.AlignCenter).
			SetExpansion(1))
		nodePoolsTable.SetCell(poolRow, 6, tview.NewTableCell(workerStatus).
			SetTextColor(workerColor).
			SetAlign(tview.AlignLeft).
			SetExpansion(1))
		poolRow++
	}
	
	// Footer divider
	footerDivider := tview.NewTextView().
		SetText(string(CharDivider)).
		SetTextColor(ColorMuted).
		SetDrawFunc(func(screen tcell.Screen, x, y, width, height int) (int, int, int, int) {
			for i := x; i < x+width; i++ {
				screen.SetContent(i, y, CharDivider, nil, StyleMuted)
			}
			return 0, 0, 0, 0
		})
	
	// Status bar with shortcuts (same style as listing page)
	statusBarFlex := tview.NewFlex().SetDirection(tview.FlexColumn)
	
	// Connection status (left)
	connectionStatus := tview.NewTextView().
		SetText(fmt.Sprintf(" %s● Connected to %s%s", TagSuccess, cluster.Name, TagReset)).
		SetDynamicColors(true).
		SetTextAlign(tview.AlignLeft)
	
	// Shortcuts (right)
	shortcuts := fmt.Sprintf("%s%c%s Back  %sEnter%s Select  %sk%s Select  %se%s Edit  %sd%s Delete  %sr%s Refresh ", 
		TagPrimary, CharArrowLeft, TagReset, 
		TagPrimary, TagReset, 
		TagPrimary, TagReset, 
		TagPrimary, TagReset, 
		TagPrimary, TagReset,
		TagPrimary, TagReset)
	statusRight := tview.NewTextView().
		SetText(shortcuts).
		SetDynamicColors(true).
		SetTextAlign(tview.AlignRight)
	
	statusBarFlex.
		AddItem(connectionStatus, 0, 1, false).
		AddItem(statusRight, 0, 2, false)
	
	// Create content area with better organization
	contentFlex := tview.NewFlex().SetDirection(tview.FlexRow)
	
	// Add sections container
	contentFlex.AddItem(sectionsContainer, 10, 0, false)
	
	// Add a divider
	contentDivider := tview.NewTextView().
		SetText("").
		SetDrawFunc(func(screen tcell.Screen, x, y, width, height int) (int, int, int, int) {
			for i := x; i < x+width; i++ {
				screen.SetContent(i, y, CharDivider, nil, StyleMuted)
			}
			return 0, 0, 0, 0
		})
	contentFlex.AddItem(contentDivider, 1, 0, false)
	
	// Add node pools title with statistics
	poolsTitleFlex := tview.NewFlex().SetDirection(tview.FlexColumn)
	
	poolsTitle := tview.NewTextView().
		SetDynamicColors(true).
		SetText(fmt.Sprintf("  %sNode Pools%s", TagPrimary, TagReset))
	
	// Node statistics on the right
	nodeStats := fmt.Sprintf("Nodes: [::b]%d[::-] | Min: [::d]%d[::-] | Max: [::d]%d[::-] | Running: [green]%d[::-] | Stopped: [red]%d[::-] | Provisioning: [yellow]%d[::-] ",
		totalNodes, minNodes, maxNodes, runningCount, stoppedCount, provisioningCount)
	
	poolsStats := tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignRight).
		SetText(nodeStats)
	
	poolsTitleFlex.
		AddItem(poolsTitle, 0, 1, false).
		AddItem(poolsStats, 0, 1, false)
		
	contentFlex.AddItem(poolsTitleFlex, 1, 0, false)
	
	// Add divider below title
	poolsDivider := tview.NewTextView().
		SetText("").
		SetDrawFunc(func(screen tcell.Screen, x, y, width, height int) (int, int, int, int) {
			for i := x; i < x+width; i++ {
				screen.SetContent(i, y, CharDivider, nil, StyleMuted)
			}
			return 0, 0, 0, 0
		})
	contentFlex.AddItem(poolsDivider, 1, 0, false)
	contentFlex.AddItem(nodePoolsTable, 0, 1, true)
	
	// Build main layout (same structure as listing page)
	flex.
		AddItem(headerFlex, 1, 0, false).
		AddItem(headerDivider, 1, 0, false).
		AddItem(contentFlex, 0, 1, true).
		AddItem(footerDivider, 1, 0, false).
		AddItem(statusBarFlex, 1, 0, false)
	
	// Handle keyboard input
	flex.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyEscape:
			pages.SwitchToPage("clusters")
			go refreshClustersAsync()
			return nil
		case tcell.KeyRune:
			switch event.Rune() {
			case 'e', 'E':
				editCluster(cluster)
				return nil
			case 'd', 'D':
				deleteCluster(cluster)
				return nil
			case 'r', 'R':
				go refreshClustersAsync()
				return nil
			case 'k', 'K':
				// Switch to this cluster (handles tunnel management)
				switchToCluster(cluster.Name)
				return nil
			}
		}
		return event
	})
	
	// Add and switch to details page
	pages.AddAndSwitchToPage("details", flex, true)
}