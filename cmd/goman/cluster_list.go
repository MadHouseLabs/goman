package main

import (
	"fmt"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// Global variables for the cluster list view
var (
	clusterListFlex *tview.Flex
	emptyPlaceholder *tview.TextView
	headerFlex *tview.Flex
	headerDivider *tview.TextView
	footerDivider *tview.TextView
	statusBarFlex *tview.Flex
)

func createClusterListView() {
	// Create a flex layout for the main view
	flex := tview.NewFlex().SetDirection(tview.FlexRow)
	clusterListFlex = flex  // Store reference for updates

	// Create header with title and provider info
	headerFlex = tview.NewFlex().SetDirection(tview.FlexColumn)
	
	titleText := fmt.Sprintf(" [::b]K3s Cluster Manager[::-]")
	title := tview.NewTextView().
		SetText(titleText).
		SetTextAlign(tview.AlignLeft).
		SetDynamicColors(true)
	
	providerText := "[::d]Provider: AWS[::-] "
	providerInfo := tview.NewTextView().
		SetText(providerText).
		SetTextAlign(tview.AlignRight).
		SetDynamicColors(true)
	
	headerFlex.
		AddItem(title, 0, 1, false).
		AddItem(providerInfo, 0, 1, false)

	// Header divider
	headerDivider = tview.NewTextView().
		SetText(string(CharDivider)).
		SetTextColor(ColorMuted)
	headerDivider.SetDrawFunc(func(screen tcell.Screen, x, y, width, height int) (int, int, int, int) {
		for i := x; i < x+width; i++ {
			screen.SetContent(i, y, CharDivider, nil, StyleMuted)
		}
		return 0, 0, 0, 0
	})

	// Create the table
	clusterTable = tview.NewTable().
		SetBorders(false).
		SetSelectable(true, false).
		SetSeparator(' ').
		SetSelectedStyle(StyleHighlight)

	// Set headers with proper spacing
	headers := []string{"  Name", "Mode", "Region", "Status", "Nodes", "Selected", "Created"}
	for col, header := range headers {
		alignment := tview.AlignLeft
		// Center align Status, Nodes, and Connected columns (columns 3, 4, 5)
		if col == 3 || col == 4 || col == 5 {
			alignment = tview.AlignCenter
		}
		cell := tview.NewTableCell(header).
			SetTextColor(ColorPrimary).
			SetAlign(alignment).
			SetSelectable(false).
			SetExpansion(1)
		clusterTable.SetCell(0, col, cell)
	}

	// Create placeholder for empty state
	emptyPlaceholder = tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignCenter).
		SetText(`

[::d]No clusters found[::-]

[::b]Get Started:[::-]

Press [#8be9fd]c[::-] to create your first cluster
Press [#8be9fd]i[::-] to initialize infrastructure

[::d]━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━[::-]

[::d]K3s is a lightweight Kubernetes distribution
perfect for edge, IoT, CI, and development[::-]`)

	// Load clusters and determine which view to show
	refreshClusters()
	
	// Determine initial content area
	var contentArea tview.Primitive
	if len(clusters) == 0 {
		contentArea = emptyPlaceholder
	} else {
		contentArea = clusterTable
	}

	// Set up key handlers for both table and placeholder
	clusterTable.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyEnter:
			row, _ := clusterTable.GetSelection()
			if row > 0 && row <= len(clusters) {
				showClusterDetailsNew(clusters[row-1])
			}
		case tcell.KeyRune:
			switch event.Rune() {
			case 'c', 'C':
				openClusterEditor()
			case 'e', 'E':
				row, _ := clusterTable.GetSelection()
				if row > 0 && row <= len(clusters) {
					editCluster(clusters[row-1])
				}
			case 'd', 'D':
				row, _ := clusterTable.GetSelection()
				if row > 0 && row <= len(clusters) {
					deleteCluster(clusters[row-1])
				}
			case 'k', 'K':
				row, _ := clusterTable.GetSelection()
				if row > 0 && row <= len(clusters) {
					// Switch to the new cluster (handles tunnel management)
					switchToCluster(clusters[row-1].Name)
					// Refresh to update the Selected column
					refreshClusters()
				}
			case 'r', 'R':
				go refreshClustersAsync()
			case 'q', 'Q':
				app.Stop()
			}
		}
		return event
	})
	
	// Set up key handlers for placeholder
	emptyPlaceholder.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyRune:
			switch event.Rune() {
			case 'c', 'C':
				openClusterEditor()
			case 'i', 'I':
				initializeInfrastructure()
			case 'r', 'R':
				go refreshClustersAsync()
			case 'q', 'Q':
				app.Stop()
			}
		}
		return event
	})

	// Footer divider
	footerDivider = tview.NewTextView().
		SetText(string(CharDivider)).
		SetTextColor(ColorMuted)
	footerDivider.SetDrawFunc(func(screen tcell.Screen, x, y, width, height int) (int, int, int, int) {
		for i := x; i < x+width; i++ {
			screen.SetContent(i, y, CharDivider, nil, StyleMuted)
		}
		return 0, 0, 0, 0
	})

	// Status bar with connection status and shortcuts
	statusBarFlex = tview.NewFlex().SetDirection(tview.FlexColumn)
	
	// Connection status (left) - will be updated dynamically
	statusText = tview.NewTextView().
		SetText(" [green]● AWS connected[::-]").
		SetDynamicColors(true).
		SetTextAlign(tview.AlignLeft)
	
	// Shortcuts (right)
	shortcuts := fmt.Sprintf("[#8be9fd]%c%c[::-] Navigate  [#8be9fd]Enter[::-] Details  [#8be9fd]k[::-] Select  [#8be9fd]c[::-] Create  [#8be9fd]e[::-] Edit  [#8be9fd]d[::-] Delete  [#8be9fd]r[::-] Refresh  [#8be9fd]q[::-] Quit ", CharArrowUp, CharArrowDown)
	statusRight := tview.NewTextView().
		SetText(shortcuts).
		SetDynamicColors(true).
		SetTextAlign(tview.AlignRight)
	
	statusBarFlex.
		AddItem(statusText, 0, 1, false).
		AddItem(statusRight, 0, 2, false)

	// Add components to flex
	flex.
		AddItem(headerFlex, 1, 0, false).
		AddItem(headerDivider, 1, 0, false).
		AddItem(contentArea, 0, 1, true).
		AddItem(footerDivider, 1, 0, false).
		AddItem(statusBarFlex, 1, 0, false)

	// Add to pages
	pages.AddPage("clusters", flex, true, true)
}

func refreshClusters() {
	// Save current selection
	selectedRow, _ := clusterTable.GetSelection()
	
	// Load clusters from storage
	newClusters := clusterManager.GetClusters()
	newCount := len(newClusters)
	
	// Update global clusters list
	clusters = newClusters
	
	// Switch between table and placeholder based on cluster count
	if clusterListFlex != nil && clusterListFlex.GetItemCount() >= 5 {
		currentItem := clusterListFlex.GetItem(2)
		
		if newCount == 0 {
			// No clusters, show placeholder
			if _, isTable := currentItem.(*tview.Table); isTable {
				// We need to rebuild the flex to maintain proper order
				clusterListFlex.Clear()
				
				// Re-add all components in the correct order
				clusterListFlex.
					AddItem(headerFlex, 1, 0, false).
					AddItem(headerDivider, 1, 0, false).
					AddItem(emptyPlaceholder, 0, 1, true).
					AddItem(footerDivider, 1, 0, false).
					AddItem(statusBarFlex, 1, 0, false)
			}
			return
		} else {
			// We have clusters, ensure table is shown
			if _, isTable := currentItem.(*tview.Table); !isTable {
				// We need to rebuild the flex to maintain proper order
				clusterListFlex.Clear()
				
				// Re-add all components in the correct order
				clusterListFlex.
					AddItem(headerFlex, 1, 0, false).
					AddItem(headerDivider, 1, 0, false).
					AddItem(clusterTable, 0, 1, true).
					AddItem(footerDivider, 1, 0, false).
					AddItem(statusBarFlex, 1, 0, false)
			}
		}
	}
	
	// Handle row count changes
	currentRows := clusterTable.GetRowCount() - 1 // Exclude header
	
	if newCount < currentRows {
		// Remove extra rows
		for row := clusterTable.GetRowCount() - 1; row > newCount; row-- {
			clusterTable.RemoveRow(row)
		}
	}
	
	// Update or add rows
	for i, cluster := range clusters {
		row := i + 1
		
		// Format creation time
		created := cluster.CreatedAt.Format("2006-01-02 15:04")
		if cluster.CreatedAt.IsZero() {
			created = "-"
		}

		// Count nodes
		nodeCount := len(cluster.MasterNodes) + len(cluster.WorkerNodes)

		// Status color
		statusColor := ColorDanger
		if cluster.Status == "running" {
			statusColor = ColorSuccess
		} else if cluster.Status == "pending" || cluster.Status == "creating" {
			statusColor = ColorWarning
		} else if cluster.Status == "deleting" {
			statusColor = ColorDanger
		}

		// Check if this is the selected cluster
		connectedText := "○"
		connectedColor := ColorMuted
		currentCluster := getCurrentCluster()
		if currentCluster == cluster.Name {
			connectedText = "●"
			connectedColor = ColorSuccess
		}

		// Update cells (this will either update existing or create new)
		clusterTable.SetCell(row, 0, tview.NewTableCell("  "+cluster.Name).SetExpansion(2))
		clusterTable.SetCell(row, 1, tview.NewTableCell(string(cluster.Mode)).SetAlign(tview.AlignLeft).SetExpansion(1))
		clusterTable.SetCell(row, 2, tview.NewTableCell(cluster.Region).SetAlign(tview.AlignLeft).SetExpansion(1))
		
		// Debug: ensure status is not empty
		statusText := string(cluster.Status)
		if statusText == "" {
			statusText = "unknown"
		}
		clusterTable.SetCell(row, 3, tview.NewTableCell(statusText).SetTextColor(statusColor).SetAlign(tview.AlignCenter).SetExpansion(1))
		clusterTable.SetCell(row, 4, tview.NewTableCell(fmt.Sprintf("%d", nodeCount)).SetAlign(tview.AlignCenter).SetExpansion(1))
		clusterTable.SetCell(row, 5, tview.NewTableCell(connectedText).SetTextColor(connectedColor).SetAlign(tview.AlignCenter).SetExpansion(1))
		clusterTable.SetCell(row, 6, tview.NewTableCell(created).SetAlign(tview.AlignLeft).SetExpansion(2))
	}
	
	// Restore selection if valid
	if selectedRow > 0 && selectedRow <= newCount {
		clusterTable.Select(selectedRow, 0)
	} else if newCount > 0 {
		clusterTable.Select(1, 0)
	}
}

// refreshClustersAsync performs a non-blocking refresh of the cluster list
func refreshClustersAsync() {
	// Cancel any existing refresh
	if refreshCancel != nil {
		close(refreshCancel)
	}
	
	// Create new cancellation channel
	refreshCancel = make(chan struct{})
	cancelChan := refreshCancel
	
	// Skip if we just refreshed recently (debounce)
	if time.Since(lastRefreshTime) < 500*time.Millisecond {
		return
	}
	lastRefreshTime = time.Now()
	
	// Update status to show refreshing
	app.QueueUpdateDraw(func() {
		if statusText != nil {
			statusText.SetText(" [yellow]↻ Refreshing...[::-]")
		}
	})

	// Perform refresh in background
	go func() {
		startTime := time.Now()
		
		// Create a context with cancellation
		done := make(chan bool)
		var refreshErr error
		
		go func() {
			defer func() {
				if r := recover(); r != nil {
					refreshErr = fmt.Errorf("refresh failed: %v", r)
				}
				done <- true
			}()
			clusterManager.RefreshClusterStatus()
		}()
		
		// Wait for completion or cancellation
		select {
		case <-done:
			// Refresh completed
		case <-cancelChan:
			// Refresh cancelled, don't update UI
			return
		}
		
		duration := time.Since(startTime)

		// Determine connection status to AWS
		var statusMsg string
		if refreshErr != nil {
			// Connection error
			statusMsg = " [red]● AWS connection error[::-]"
			lastError = refreshErr
		} else if duration > 3*time.Second {
			// Slow connection
			statusMsg = fmt.Sprintf(" [yellow]● Slow AWS connection[::-] (%.1fs)", duration.Seconds())
		} else {
			// Success
			statusMsg = " [green]● AWS connected[::-]"
			lastError = nil
		}

		// Check if this refresh was cancelled
		select {
		case <-cancelChan:
			// Cancelled, don't update UI
			return
		default:
			// Update the UI with fresh data
			app.QueueUpdateDraw(func() {
				if lastError == nil {
					refreshClusters()
				}
				if statusText != nil {
					statusText.SetText(statusMsg)
				}
			})
		}
	}()
}