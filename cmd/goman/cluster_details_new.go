package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	clusterPkg "github.com/madhouselabs/goman/pkg/cluster"
	"github.com/madhouselabs/goman/pkg/models"
	"github.com/rivo/tview"
)

// Global UI elements for the details page
var (
	detailsFlex      *tview.Flex
	detailsInitialized = false
)

func showClusterDetailsNew(cluster models.K3sCluster) {
	// Stop any existing refresh
	if detailsState != nil {
		detailsState.StopRefresh()
	}
	
	// Create or update the UI
	if !detailsInitialized {
		// Create new state only when creating UI for the first time
		detailsState = NewClusterDetailsState(cluster)
		createDetailsUI()
		detailsInitialized = true
	} else {
		// If UI already exists, just update the cluster in existing state
		if detailsState == nil {
			detailsState = NewClusterDetailsState(cluster)
		} else {
			detailsState.UpdateCluster(cluster)
		}
	}
	
	// Update UI with cluster data (only after UI is created)
	updateDetailsUI(cluster)
	
	// Start metrics refresh if cluster is running
	if cluster.Status == models.StatusRunning {
		startMetricsRefresh()
	}
	
	// Switch to details page
	pages.RemovePage("details")
	pages.AddAndSwitchToPage("details", detailsFlex, true)
}

func createDetailsUI() {
	// Create main flex container
	detailsFlex = tview.NewFlex().SetDirection(tview.FlexRow)
	
	// Create header with title
	headerFlex := tview.NewFlex().SetDirection(tview.FlexColumn)
	
	titleText := fmt.Sprintf(" %s%sCluster Details%s%s", TagBold, TagPrimary, TagReset, TagReset)
	titleView := tview.NewTextView().
		SetText(titleText).
		SetTextAlign(tview.AlignLeft).
		SetDynamicColors(true)
	
	// Status will be updated dynamically
	detailsState.statusText = tview.NewTextView().
		SetTextAlign(tview.AlignRight).
		SetDynamicColors(true)
	
	headerFlex.
		AddItem(titleView, 0, 1, false).
		AddItem(detailsState.statusText, 0, 1, false)
	
	// Header divider
	headerDivider := createDivider()
	
	// Create the three info sections
	sectionsContainer := tview.NewFlex().SetDirection(tview.FlexColumn)
	
	// Section 1: Cluster Info
	detailsState.clusterInfoTable = createClusterInfoTable()
	
	// Section 2: Resources
	detailsState.resourcesTable = createResourcesTable()
	
	// Section 3: Metrics
	detailsState.metricsTable = createMetricsTable()
	
	// Add sections with dividers
	divider1 := createVerticalDivider()
	divider2 := createVerticalDivider()
	
	sectionsContainer.
		AddItem(detailsState.clusterInfoTable, 0, 1, false).
		AddItem(divider1, 3, 0, false).
		AddItem(detailsState.resourcesTable, 0, 1, false).
		AddItem(divider2, 3, 0, false).
		AddItem(detailsState.metricsTable, 0, 1, false)
	
	// Create content area
	contentFlex := tview.NewFlex().SetDirection(tview.FlexRow)
	contentFlex.AddItem(sectionsContainer, 12, 0, false)
	
	// Add divider
	contentDivider := createDivider()
	contentFlex.AddItem(contentDivider, 1, 0, false)
	
	// Node pools section
	poolsTitleFlex := tview.NewFlex().SetDirection(tview.FlexColumn)
	
	poolsTitle := tview.NewTextView().
		SetDynamicColors(true).
		SetText(fmt.Sprintf("  %sNode Pools%s", TagPrimary, TagReset))
	
	poolsStats := tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignRight)
	
	poolsTitleFlex.
		AddItem(poolsTitle, 0, 1, false).
		AddItem(poolsStats, 0, 1, false)
	
	contentFlex.AddItem(poolsTitleFlex, 1, 0, false)
	
	// Node pools table
	detailsState.nodePoolsTable = createNodePoolsTable()
	contentFlex.AddItem(detailsState.nodePoolsTable, 0, 1, true)
	
	// Footer divider
	footerDivider := createDivider()
	
	// Status bar
	statusBarFlex := createStatusBar()
	
	// Build main layout
	detailsFlex.
		AddItem(headerFlex, 1, 0, false).
		AddItem(headerDivider, 1, 0, false).
		AddItem(contentFlex, 0, 1, true).
		AddItem(footerDivider, 1, 0, false).
		AddItem(statusBarFlex, 1, 0, false)
	
	// Handle keyboard input
	detailsFlex.SetInputCapture(handleDetailsInput)
}

func createClusterInfoTable() *tview.Table {
	table := tview.NewTable().
		SetBorders(false).
		SetSelectable(false, false).
		SetFixed(1, 2)
	
	// Title
	table.SetCell(0, 0, tview.NewTableCell(" [::b]Cluster Information").
		SetTextColor(ColorPrimary).
		SetAlign(tview.AlignLeft))
	table.SetCell(0, 1, tview.NewTableCell(""))
	
	// Add rows for each field
	row := 1
	fields := []string{"Name:", "Status:", "Mode:", "Region:", "Created:", "Updated:"}
	for _, field := range fields {
		table.SetCell(row, 0, tview.NewTableCell(" "+field).
			SetTextColor(ColorMuted).
			SetAlign(tview.AlignLeft))
		table.SetCell(row, 1, tview.NewTableCell("").
			SetTextColor(ColorForeground).
			SetAlign(tview.AlignLeft))
		row++
	}
	
	return table
}

func createResourcesTable() *tview.Table {
	table := tview.NewTable().
		SetBorders(false).
		SetSelectable(false, false).
		SetFixed(1, 2)
	
	// Title
	table.SetCell(0, 0, tview.NewTableCell(" [::b]Resources").
		SetTextColor(ColorPrimary).
		SetAlign(tview.AlignLeft))
	table.SetCell(0, 1, tview.NewTableCell(""))
	
	// Add rows for each field
	row := 1
	fields := []string{"Master Nodes:", "Worker Nodes:", "Total Nodes:", "Total CPU:", "Total Memory:", "Total Storage:"}
	for _, field := range fields {
		table.SetCell(row, 0, tview.NewTableCell(" "+field).
			SetTextColor(ColorMuted).
			SetAlign(tview.AlignLeft))
		table.SetCell(row, 1, tview.NewTableCell("").
			SetTextColor(ColorForeground).
			SetAlign(tview.AlignLeft))
		row++
	}
	
	return table
}

func createMetricsTable() *tview.Table {
	table := tview.NewTable().
		SetBorders(false).
		SetSelectable(false, false).
		SetFixed(1, 2)
	
	// Title
	table.SetCell(0, 0, tview.NewTableCell(" [::b]Metrics").
		SetTextColor(ColorPrimary).
		SetAlign(tview.AlignLeft))
	table.SetCell(0, 1, tview.NewTableCell(""))
	
	// Add rows for each field
	row := 1
	fields := []string{"Uptime:", "Nodes Health:", "CPU Usage:", "Memory Usage:", "Pod Count:", "Updated:"}
	for _, field := range fields {
		table.SetCell(row, 0, tview.NewTableCell(" "+field).
			SetTextColor(ColorMuted).
			SetAlign(tview.AlignLeft))
		table.SetCell(row, 1, tview.NewTableCell("").
			SetTextColor(ColorForeground).
			SetAlign(tview.AlignLeft))
		row++
	}
	
	return table
}

func createNodePoolsTable() *tview.Table {
	table := tview.NewTable().
		SetBorders(false).
		SetSelectable(true, false).
		SetSeparator(' ').
		SetSelectedStyle(StyleHighlight)
	
	// Add headers
	headers := []string{"  Pool Name", "Role", "Nodes", "Instance Type", "CPU", "Memory", "Status"}
	for col, header := range headers {
		alignment := tview.AlignLeft
		if col > 1 && col < 6 {
			alignment = tview.AlignCenter
		}
		cell := tview.NewTableCell(header).
			SetTextColor(ColorPrimary).
			SetAlign(alignment).
			SetSelectable(false).
			SetExpansion(1)
		table.SetCell(0, col, cell)
	}
	
	return table
}

func createDivider() *tview.TextView {
	divider := tview.NewTextView().
		SetText(string(CharDivider)).
		SetTextColor(ColorMuted)
	divider.SetDrawFunc(func(screen tcell.Screen, x, y, width, height int) (int, int, int, int) {
		for i := x; i < x+width; i++ {
			screen.SetContent(i, y, CharDivider, nil, StyleMuted)
		}
		return 0, 0, 0, 0
	})
	return divider
}

func createVerticalDivider() *tview.TextView {
	divider := tview.NewTextView().
		SetText("")
	divider.SetDrawFunc(func(screen tcell.Screen, x, y, width, height int) (int, int, int, int) {
		for i := y; i < y+height; i++ {
			screen.SetContent(x+1, i, '│', nil, StyleMuted)
		}
		return 0, 0, 0, 0
	})
	return divider
}

func createStatusBar() *tview.Flex {
	statusBarFlex := tview.NewFlex().SetDirection(tview.FlexColumn)
	
	connectionStatus := tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignLeft)
	
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
	
	return statusBarFlex
}

func updateDetailsUI(cluster models.K3sCluster) {
	if detailsState == nil {
		return
	}
	
	// Update status text
	if detailsState.statusText != nil {
		statusColor := TagSuccess
		statusIcon := "●"
		if cluster.Status == "creating" {
			statusColor = TagWarning
			statusIcon = "◐"
		} else if cluster.Status == "stopped" {
			statusColor = TagMuted
			statusIcon = "○"
		}
		
		statusText := fmt.Sprintf("%s%s %s %s%s %s", TagBold, cluster.Name, TagReset, statusColor, statusIcon, strings.ToUpper(string(cluster.Status)))
		detailsState.statusText.SetText(statusText)
	}
	
	// Update cluster info table
	updateClusterInfoTable(cluster)
	
	// Update resources table
	updateResourcesTableData(cluster)
	
	// Update metrics table (initial static values)
	updateMetricsTableData(cluster)
	
	// Update node pools table
	updateNodePoolsTableData(cluster)
}

func updateClusterInfoTable(cluster models.K3sCluster) {
	table := detailsState.clusterInfoTable
	if table == nil {
		return
	}
	
	table.SetCell(1, 1, tview.NewTableCell(cluster.Name))
	
	statusColor := getStatusColor(string(cluster.Status))
	table.SetCell(2, 1, tview.NewTableCell(string(cluster.Status)).SetTextColor(statusColor))
	
	table.SetCell(3, 1, tview.NewTableCell(strings.ToUpper(string(cluster.Mode))))
	table.SetCell(4, 1, tview.NewTableCell(cluster.Region))
	table.SetCell(5, 1, tview.NewTableCell(cluster.CreatedAt.Format("2006-01-02 15:04")))
	table.SetCell(6, 1, tview.NewTableCell(cluster.UpdatedAt.Format("2006-01-02 15:04")))
}

func updateResourcesTableData(cluster models.K3sCluster) {
	table := detailsState.resourcesTable
	if table == nil {
		return
	}
	
	// Calculate totals
	totalNodes := len(cluster.MasterNodes) + len(cluster.WorkerNodes)
	totalCPU := 0
	totalMemory := 0
	totalStorage := 0
	
	for _, node := range cluster.MasterNodes {
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
	
	table.SetCell(1, 1, tview.NewTableCell(fmt.Sprintf("%d", len(cluster.MasterNodes))).SetTextColor(ColorAccent))
	table.SetCell(2, 1, tview.NewTableCell(fmt.Sprintf("%d", len(cluster.WorkerNodes))).SetTextColor(ColorAccent))
	table.SetCell(3, 1, tview.NewTableCell(fmt.Sprintf("%d", totalNodes)).SetTextColor(ColorSuccess))
	table.SetCell(4, 1, tview.NewTableCell(fmt.Sprintf("%d vCPUs", totalCPU)))
	table.SetCell(5, 1, tview.NewTableCell(fmt.Sprintf("%d GB", totalMemory)))
	table.SetCell(6, 1, tview.NewTableCell(fmt.Sprintf("%d GB", totalStorage)))
}

func updateMetricsTableData(cluster models.K3sCluster) {
	table := detailsState.metricsTable
	if table == nil {
		return
	}
	
	// Uptime
	uptime := time.Since(cluster.CreatedAt)
	uptimeStr := fmt.Sprintf("%d days, %d hours", int(uptime.Hours()/24), int(uptime.Hours())%24)
	table.SetCell(1, 1, tview.NewTableCell(uptimeStr))
	
	// Nodes Health
	metrics := detailsState.GetMetrics()
	if metrics != nil && metrics.NodesTotal > 0 {
		healthStr := fmt.Sprintf("%d/%d Running", metrics.NodesReady, metrics.NodesTotal)
		healthColor := ColorSuccess
		if metrics.NodesReady < metrics.NodesTotal {
			healthColor = ColorWarning
		}
		table.SetCell(2, 1, tview.NewTableCell(healthStr).SetTextColor(healthColor))
	} else {
		totalNodes := len(cluster.MasterNodes) + len(cluster.WorkerNodes)
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
		table.SetCell(2, 1, tview.NewTableCell(healthStr).SetTextColor(healthColor))
	}
	
	// CPU Usage
	if metrics != nil && metrics.TotalCPU > 0 {
		cpuPercent := (metrics.UsedCPU / metrics.TotalCPU) * 100
		cpuStr := fmt.Sprintf("%.1f%% (%.1f/%.1f vCPUs)", cpuPercent, metrics.UsedCPU, metrics.TotalCPU)
		cpuColor := ColorSuccess
		if cpuPercent > 70 {
			cpuColor = ColorWarning
		}
		if cpuPercent > 90 {
			cpuColor = ColorDanger
		}
		table.SetCell(3, 1, tview.NewTableCell(cpuStr).SetTextColor(cpuColor))
	} else {
		table.SetCell(3, 1, tview.NewTableCell("N/A").SetTextColor(ColorMuted))
	}
	
	// Memory Usage
	if metrics != nil && metrics.TotalMemoryGB > 0 {
		memPercent := (metrics.UsedMemoryGB / metrics.TotalMemoryGB) * 100
		memStr := fmt.Sprintf("%.1f%% (%.1f/%.1f GB)", memPercent, metrics.UsedMemoryGB, metrics.TotalMemoryGB)
		memColor := ColorSuccess
		if memPercent > 70 {
			memColor = ColorWarning
		}
		if memPercent > 90 {
			memColor = ColorDanger
		}
		table.SetCell(4, 1, tview.NewTableCell(memStr).SetTextColor(memColor))
	} else {
		table.SetCell(4, 1, tview.NewTableCell("N/A").SetTextColor(ColorMuted))
	}
	
	// Pod Count
	if metrics != nil && metrics.PodCount > 0 {
		table.SetCell(5, 1, tview.NewTableCell(fmt.Sprintf("%d", metrics.PodCount)).SetTextColor(ColorAccent))
	} else {
		table.SetCell(5, 1, tview.NewTableCell("N/A").SetTextColor(ColorMuted))
	}
	
	// Updated timestamp
	if metrics != nil && !metrics.LastUpdated.IsZero() {
		table.SetCell(6, 1, tview.NewTableCell(metrics.LastUpdated.Format("15:04:05")).SetTextColor(ColorMuted))
	} else {
		table.SetCell(6, 1, tview.NewTableCell("--").SetTextColor(ColorMuted))
	}
}

func updateNodePoolsTableData(cluster models.K3sCluster) {
	table := detailsState.nodePoolsTable
	if table == nil {
		return
	}
	
	// Clear existing rows (except header)
	for row := 1; row < table.GetRowCount(); row++ {
		for col := 0; col < 7; col++ {
			table.SetCell(row, col, nil)
		}
	}
	
	poolRow := 1
	
	// Control plane pool
	controlPlaneCount := len(cluster.MasterNodes)
	expectedControlPlaneCount := 1
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
	}
	
	instanceType := cluster.InstanceType
	if instanceType == "" {
		instanceType = "t3.medium"
	}
	
	table.SetCell(poolRow, 0, tview.NewTableCell("  control-plane").SetTextColor(ColorForeground))
	table.SetCell(poolRow, 1, tview.NewTableCell("Control Plane").SetTextColor(ColorPrimary))
	table.SetCell(poolRow, 2, tview.NewTableCell(fmt.Sprintf("%d/%d", controlPlaneCount, expectedControlPlaneCount)).SetAlign(tview.AlignCenter))
	table.SetCell(poolRow, 3, tview.NewTableCell(instanceType).SetAlign(tview.AlignCenter))
	table.SetCell(poolRow, 4, tview.NewTableCell(fmt.Sprintf("%d", controlPlaneCount*2)).SetAlign(tview.AlignCenter))
	table.SetCell(poolRow, 5, tview.NewTableCell(fmt.Sprintf("%d GB", controlPlaneCount*4)).SetAlign(tview.AlignCenter))
	table.SetCell(poolRow, 6, tview.NewTableCell(controlPlaneStatus).SetTextColor(controlPlaneColor))
	poolRow++
	
	// Worker pool if exists
	if len(cluster.WorkerNodes) > 0 {
		workerCount := len(cluster.WorkerNodes)
		workerStatus := "Running"
		workerColor := ColorSuccess
		
		for _, node := range cluster.WorkerNodes {
			if node.Status != "running" {
				workerStatus = "Scaling"
				workerColor = ColorWarning
				break
			}
		}
		
		table.SetCell(poolRow, 0, tview.NewTableCell("  worker-pool-1").SetTextColor(ColorForeground))
		table.SetCell(poolRow, 1, tview.NewTableCell("Worker").SetTextColor(ColorAccent))
		table.SetCell(poolRow, 2, tview.NewTableCell(fmt.Sprintf("%d", workerCount)).SetAlign(tview.AlignCenter))
		table.SetCell(poolRow, 3, tview.NewTableCell(instanceType).SetAlign(tview.AlignCenter))
		table.SetCell(poolRow, 4, tview.NewTableCell(fmt.Sprintf("%d", workerCount*2)).SetAlign(tview.AlignCenter))
		table.SetCell(poolRow, 5, tview.NewTableCell(fmt.Sprintf("%d GB", workerCount*4)).SetAlign(tview.AlignCenter))
		table.SetCell(poolRow, 6, tview.NewTableCell(workerStatus).SetTextColor(workerColor))
	}
}

func handleDetailsInput(event *tcell.EventKey) *tcell.EventKey {
	switch event.Key() {
	case tcell.KeyEscape:
		// Stop metrics refresh
		if detailsState != nil {
			detailsState.StopRefresh()
		}
		// Go back to clusters
		pages.SwitchToPage("clusters")
		go refreshClustersAsync()
		return nil
	case tcell.KeyRune:
		switch event.Rune() {
		case 'r', 'R':
			// Refresh cluster data
			if detailsState != nil {
				cluster := detailsState.GetCluster()
				manager := clusterPkg.NewManager()
				clusters := manager.GetClusters()
				for _, c := range clusters {
					if c.Name == cluster.Name {
						detailsState.UpdateCluster(c)
						updateDetailsUI(c)
						// Also refresh metrics immediately
						go fetchMetricsOnce()
						break
					}
				}
			}
			return nil
		case 'e', 'E':
			if detailsState != nil {
				editCluster(detailsState.GetCluster())
			}
			return nil
		case 'd', 'D':
			if detailsState != nil {
				deleteCluster(detailsState.GetCluster())
			}
			return nil
		case 'k', 'K':
			if detailsState != nil {
				switchToCluster(detailsState.GetCluster().Name)
			}
			return nil
		}
	}
	return event
}

// startMetricsRefresh starts the background goroutine to refresh metrics
func startMetricsRefresh() {
	go func() {
		// Initial fetch
		fetchMetricsOnce()
		
		// Set up refresh timer
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		
		for {
			select {
			case <-ticker.C:
				fetchMetricsOnce()
			case <-detailsState.stopRefresh:
				return
			}
		}
	}()
}

// fetchMetricsOnce fetches metrics once and updates the UI
func fetchMetricsOnce() {
	if detailsState == nil {
		return
	}
	
	cluster := detailsState.GetCluster()
	if cluster.Status != models.StatusRunning {
		return
	}
	
	// Fetch metrics in background
	go func() {
		metrics, err := clusterPkg.FetchClusterMetrics(cluster.Name)
		
		if err == nil {
			// Update metrics
			detailsState.UpdateMetrics(metrics)
			
			// Update UI only if we got new metrics
			app.QueueUpdateDraw(func() {
				updateMetricsTableData(cluster)
			})
		}
	}()
}