package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"

	"github.com/madhouselabs/goman/pkg/cluster"
	"github.com/madhouselabs/goman/pkg/config"
	"github.com/madhouselabs/goman/pkg/datamanager"
	"github.com/madhouselabs/goman/pkg/models"
	"github.com/madhouselabs/goman/pkg/storage"
	"github.com/madhouselabs/goman/pkg/tui/components"
	"github.com/madhouselabs/goman/pkg/ui"
)

// View states
type viewState int

const (
	viewList viewState = iota
	viewCreate
	viewDetail
	viewLoading
	viewConfirmDelete
)

// Model represents the application state
type model struct {
	state           viewState
	clusters        []models.K3sCluster
	clusterStates   map[string]*storage.K3sClusterState // Cached cluster states
	dataManager     *datamanager.DataManager            // Central data manager
	subscriber      *datamanager.TUISubscriber          // Data subscriber
	clusterManager  *cluster.Manager
	storage         *storage.Storage
	config          *config.Config
	selectedIndex   int
	selectedCluster *models.K3sCluster
	selectedClusterState *storage.K3sClusterState // Detailed state for selected cluster
	detailLoading   bool                        // Loading state for detail view
	initialLoading  bool                        // Initial data loading state
	form            *ui.ProForm
	viewport        viewport.Model
	spinner         spinner.Model
	help            help.Model
	keys            ui.KeyMap
	width           int
	height          int
	err             error
	message         string
	loading         bool
	loadingMsg      string
	deleteTarget    *models.K3sCluster // Cluster pending deletion
	clusterListView *ui.ClusterListView // New cluster list view
	clusterTable    *components.ClusterTableComponent  // Cluster table component that manages its own state
	
	// Optimistic update tracking
	pendingCreates  map[string]models.K3sCluster // Clusters being created (key: cluster ID)
	pendingDeletes  map[string]bool              // Clusters being deleted (key: cluster ID)
}

func initialModel() (model, error) {
	stor, err := storage.NewStorage()
	if err != nil {
		return model{}, err
	}

	config, err := config.NewConfig()
	if err != nil {
		return model{}, err
	}

	// Create DataManager for centralized data management
	dataManager, err := datamanager.NewDataManager()
	if err != nil {
		return model{}, err
	}

	// Create TUI subscriber
	subscriber := datamanager.NewTUISubscriber(
		"main-tui",
		[]datamanager.DataType{
			datamanager.DataTypeClusters,
			datamanager.DataTypeClusterStates,
		},
	)

	clusterManager := cluster.NewManager()

	// Setup spinner
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#606060"))

	// Setup viewport
	vp := viewport.New(80, 20)

	// Initialize cluster table component that manages its own state
	clusterTable := components.NewClusterTable("cluster-table")
	clusterTable.SetDimensions(80, 20)
	clusterTable.Focus() // Set focus for keyboard navigation

	// Initialize with empty states - will be populated by DataManager
	// Load initial clusters synchronously
	initialClusters := clusterManager.GetClusters()
	initialStates := clusterManager.GetAllClusterStates()
	
	m := model{
		state:          viewList,
		clusters:       initialClusters,
		clusterStates:  initialStates,
		dataManager:    dataManager,
		subscriber:     subscriber,
		clusterManager: clusterManager,
		storage:        stor,
		config:         config,
		spinner:        s,
		viewport:       vp,
		help:           help.New(),
		keys:           ui.DefaultKeyMap(),
		selectedIndex:  0,
		width:          80,  // Default width until window size is known
		height:         24,  // Default height until window size is known
		initialLoading: len(initialClusters) == 0, // Only show loading if no clusters yet
		clusterListView: nil, // Not needed anymore
		clusterTable:   clusterTable,    // Persistent table component
		pendingCreates: make(map[string]models.K3sCluster),
		pendingDeletes: make(map[string]bool),
	}
	
	// Set initial data in table
	clusterTable.SetClusters(initialClusters, initialStates)
	
	// Initialize the table component
	clusterTable.Init()
	
	return m, nil
}

func (m model) Init() tea.Cmd {
	// Subscribe to data updates with 5 second refresh rate
	subscribeCmd := datamanager.SubscribeCmd(m.dataManager, datamanager.SubscriptionConfig{
		SubscriberID: "main-tui",
		DataTypes: []datamanager.DataType{
			datamanager.DataTypeClusters,
			datamanager.DataTypeClusterStates,
		},
		RefreshRate: 5 * time.Second,
		Priority:    datamanager.PriorityNormal,
	})
	
	// Request initial data immediately
	initialDataCmd := datamanager.RequestDataCmd(m.dataManager, datamanager.DataRequest{
		ID:       "initial-load",
		Type:     datamanager.DataTypeClusters,
		Policy:   datamanager.PolicyFreshIfStale,
		Priority: datamanager.PriorityHigh,
	})
	
	return tea.Batch(
		m.spinner.Tick,
		tea.EnterAltScreen,
		subscribeCmd,
		initialDataCmd,
		m.listenForUpdates(), // Start listening for data updates
	)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	// Update the table component if in list view
	if m.state == viewList && m.clusterTable != nil {
		_, tableCmd := m.clusterTable.Update(msg)
		if tableCmd != nil {
			cmds = append(cmds, tableCmd)
		}
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.viewport.Width = m.width - 4
		m.viewport.Height = m.height - 10
		// Update table dimensions
		m.updateClusterTable()
		return m, nil

	case tea.MouseMsg:
		// Handle mouse events
		switch m.state {
		case viewList:
			if msg.Type == tea.MouseRelease && msg.Button == tea.MouseButtonLeft {
				// Check if any list item was clicked
				for i := range m.clusters {
					zoneID := fmt.Sprintf("list_item_%d", i)
					if zone.Get(zoneID).InBounds(msg) {
						m.selectedIndex = i
						// Double-click or just select - for now just select
						// Could add double-click detection to open details
						return m, nil
					}
				}
			}
		case viewCreate:
			// Pass mouse events to the form
			if m.form != nil {
				formCmd := m.form.Update(msg)

				// Check if form is submitted
				if m.form.IsSubmitted() {
					cluster := m.form.GetCluster()
					m.loading = true
					m.loadingMsg = fmt.Sprintf("Creating cluster %s...", cluster.Name)
					
					// Track as pending creation for optimistic update
					cluster.Status = "pending"
					m.pendingCreates[cluster.ID] = cluster
					
					// Don't directly modify clusters - let mergeWithPending handle it
					
					return m, m.createCluster(cluster)
				}

				return m, formCmd
			}
		}

	case spinner.TickMsg:
		// Update spinner for both general loading and detail loading
		if m.loading || m.detailLoading {
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}

	case datamanager.DataUpdate:
		// Handle data updates from DataManager
		switch msg.Type {
		case datamanager.DataTypeClusters:
			if clusterData, ok := msg.Data.([]models.K3sCluster); ok {
				// Merge with pending operations instead of direct replacement
				m.clusters = m.mergeWithPending(clusterData)
				// Only clear initial loading if we've actually loaded data or confirmed empty
				// Don't clear on first empty response as it might be cached empty state
				if len(clusterData) > 0 || m.clusterManager.HasSyncedOnce() {
					m.initialLoading = false
				}
				// Update table data
				m.updateClusterTable()
			}
		case datamanager.DataTypeClusterStates:
			if states, ok := msg.Data.(map[string]*storage.K3sClusterState); ok {
				m.clusterStates = states
				// Update table data
				m.updateClusterTable()
			}
		}
		// Continue listening for updates
		return m, m.listenForUpdates()
		
	case datamanager.DataManagerMsg:
		// Handle DataManager responses
		if msg.Response != nil && msg.Response.Data != nil {
			switch msg.Response.Type {
			case datamanager.DataTypeClusters:
				if clusters, ok := msg.Response.Data.([]models.K3sCluster); ok {
					m.clusters = clusters
					// Only clear initial loading if we've actually loaded data or confirmed empty
					if len(clusters) > 0 || m.clusterManager.HasSyncedOnce() {
						m.initialLoading = false
					}
				}
			case datamanager.DataTypeClusterStates:
				if states, ok := msg.Response.Data.(map[string]*storage.K3sClusterState); ok {
					m.clusterStates = states
				}
			}
		}
		return m, nil

	case tea.KeyMsg:
		switch m.state {
		case viewList:
			// Handle special keys first
			switch {
			case key.Matches(msg, m.keys.Quit):
				return m, tea.Quit

			case key.Matches(msg, m.keys.Create):
				m.state = viewCreate
				m.form = ui.NewProClusterFormWithConfig(m.config)
				return m, nil

			case key.Matches(msg, m.keys.Enter):
				// Get selected cluster directly from the component
				if m.clusterTable != nil {
					if selected := m.clusterTable.GetSelectedCluster(); selected != nil {
						m.selectedCluster = selected
						m.selectedIndex = m.clusterTable.GetSelectedIndex()
						m.selectedClusterState = nil // Clear previous state
						m.detailLoading = true        // Set loading state
						m.state = viewDetail
						// Fetch details asynchronously and start spinner
						return m, tea.Batch(
							m.fetchClusterDetails(m.selectedCluster.Name),
							m.spinner.Tick,
						)
					}
				}

			case key.Matches(msg, m.keys.Delete):
				// Get selected cluster from component
				if m.clusterTable != nil {
					if selected := m.clusterTable.GetSelectedCluster(); selected != nil {
						m.deleteTarget = selected
						m.state = viewConfirmDelete
						return m, nil
					}
				}

			case key.Matches(msg, m.keys.Sync):
				// Request fresh data from DataManager
				return m, datamanager.RefreshDataCmd(m.dataManager, datamanager.DataTypeClusters)
			}
			
			// Forward all keys to table component for navigation
			if m.clusterTable != nil {
				var tableCmd tea.Cmd
				var updatedModel tea.Model
				updatedModel, tableCmd = m.clusterTable.Update(msg)
				// The ClusterTableComponent returns itself as tea.Model
				if tc, ok := updatedModel.(*components.ClusterTableComponent); ok {
					m.clusterTable = tc
				}
				
				// Sync selected index from component
				m.selectedIndex = m.clusterTable.GetSelectedIndex()
				
				return m, tableCmd
			}
			
			return m, nil

		case viewCreate:
			// Handle escape key to go back
			switch msg.String() {
			case "esc":
				m.state = viewList
				m.form = nil
				return m, nil
			}

			// Let the form handle the key event too
			if m.form != nil {
				formCmd := m.form.Update(msg)

				// Check if form is submitted
				if m.form.IsSubmitted() {
					cluster := m.form.GetCluster()
					m.loading = true
					m.loadingMsg = fmt.Sprintf("Creating cluster %s...", cluster.Name)
					
					// Track as pending creation for optimistic update
					cluster.Status = "pending"
					m.pendingCreates[cluster.ID] = cluster
					
					// Don't directly modify clusters - let mergeWithPending handle it
					
					return m, m.createCluster(cluster)
				}

				return m, formCmd
			}

		case viewDetail:
			switch msg.String() {
			case "esc", "q":
				m.state = viewList
				m.selectedCluster = nil
				return m, nil
			}

		case viewConfirmDelete:
			switch msg.String() {
			case "y", "Y":
				if m.deleteTarget != nil {
					// Track as pending deletion for optimistic update
					m.pendingDeletes[m.deleteTarget.ID] = true
					
					// Update the status visually
					for i, c := range m.clusters {
						if c.ID == m.deleteTarget.ID {
							m.clusters[i].Status = models.StatusDeleting
							break
						}
					}
					m.updateClusterTable()
					
					m.state = viewList
					m.message = fmt.Sprintf("Deleting cluster %s...", m.deleteTarget.Name)
					cmd := m.deleteCluster(m.deleteTarget.ID)
					m.deleteTarget = nil
					return m, cmd
				}
			case "n", "N", "esc":
				m.state = viewList
				m.deleteTarget = nil
				return m, nil
			}
		}

	case ui.ClusterCreatedMsg:
		m.loading = false
		m.state = viewList
		m.form = nil
		m.message = "Cluster created, refreshing..."
		
		// Remove from pending creates since it's now created
		if msg.Cluster != nil {
			delete(m.pendingCreates, msg.Cluster.ID)
		}
		
		// Trigger immediate refresh from S3 to get latest status
		refreshCmd := datamanager.RequestDataCmd(m.dataManager, datamanager.DataRequest{
			ID:       "cluster-created-refresh",
			Type:     datamanager.DataTypeClusters,
			Policy:   datamanager.PolicyForceRefresh,
			Priority: datamanager.PriorityHigh,
		})
		return m, tea.Batch(m.clearMessage(), refreshCmd)

	case ui.ClusterDeletedMsg:
		m.loading = false
		m.message = "Cluster deleted, refreshing..."
		
		// Clear all pending deletes (we don't have specific ID here)
		// The next refresh will show the correct state
		m.pendingDeletes = make(map[string]bool)
		
		if m.selectedIndex >= len(m.clusters) && m.selectedIndex > 0 {
			m.selectedIndex--
		}
		
		// Trigger immediate refresh from S3 to get latest status
		refreshCmd := datamanager.RequestDataCmd(m.dataManager, datamanager.DataRequest{
			ID:       "cluster-deleted-refresh",
			Type:     datamanager.DataTypeClusters,
			Policy:   datamanager.PolicyForceRefresh,
			Priority: datamanager.PriorityHigh,
		})
		return m, tea.Batch(m.clearMessage(), refreshCmd)

	case ui.ClustersSyncedMsg:
		m.loading = false
		// Use mergeWithPending to properly handle optimistic updates
		m.clusters = m.mergeWithPending(m.clusterManager.GetClusters())
		// Update cached states after sync
		m.clusterStates = m.clusterManager.GetAllClusterStates()
		m.updateClusterTable()
		m.message = ""
		return m, m.clearMessage()


	case ui.ClusterDetailsLoadedMsg:
		// Details loaded for selected cluster
		m.selectedClusterState = msg.State
		m.detailLoading = false
		return m, nil

	case ui.ErrorMsg:
		m.loading = false
		m.err = msg.Err
		m.state = viewList
		return m, m.clearError()

	case ui.ClearMessageMsg:
		m.message = ""
		return m, nil

	case ui.ClearErrorMsg:
		m.err = nil
		return m, nil
	}

	// Update viewport if in detail view
	if m.state == viewDetail {
		m.viewport, cmd = m.viewport.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m model) View() string {
	// Show loading state during initial data fetch
	if m.initialLoading && len(m.clusters) == 0 {
		return ui.RenderSimpleLoadingWithSpinner("Loading clusters...", m.spinner, m.width, m.height)
	}
	
	if m.loading {
		return ui.RenderSimpleLoadingWithSpinner(m.loadingMsg, m.spinner, m.width, m.height)
	}

	switch m.state {
	case viewList:
		// Use the persistent table for proper interactivity
		return m.renderClusterListWithTable()
	case viewCreate:
		if m.form != nil {
			return m.form.RenderViewport(m.width, m.height)
		}
		return "Form not initialized"
	case viewDetail:
		if m.selectedCluster != nil {
			// Show loading state while fetching details
			if m.detailLoading {
				return ui.RenderProDetailLoading(*m.selectedCluster, m.spinner, m.width, m.height)
			}
			// Show details with state if available
			if m.selectedClusterState != nil {
				return ui.RenderProDetailWithState(*m.selectedCluster, m.selectedClusterState)
			}
			// Fallback to basic view
			return ui.RenderProDetail(*m.selectedCluster)
		}
		return "No cluster selected"
	case viewConfirmDelete:
		if m.deleteTarget != nil {
			return ui.RenderDeleteConfirmation(m.deleteTarget.Name, m.width, m.height)
		}
		return "No cluster selected for deletion"
	default:
		return "Unknown view"
	}
}

// Commands
func (m *model) createCluster(cluster models.K3sCluster) tea.Cmd {
	return func() tea.Msg {
		newCluster, err := m.clusterManager.CreateCluster(cluster)
		if err != nil {
			return ui.ErrorMsg{Err: err}
		}
		return ui.ClusterCreatedMsg{Cluster: newCluster}
	}
}

func (m *model) deleteCluster(clusterID string) tea.Cmd {
	return func() tea.Msg {
		err := m.clusterManager.DeleteCluster(clusterID)
		if err != nil {
			return ui.ErrorMsg{Err: err}
		}
		return ui.ClusterDeletedMsg{}
	}
}

func (m *model) syncClusters() tea.Cmd {
	return func() tea.Msg {
		err := m.clusterManager.SyncFromProvider()
		if err != nil {
			return ui.ErrorMsg{Err: err}
		}
		return ui.ClustersSyncedMsg{}
	}
}


func (m *model) clearMessage() tea.Cmd {
	return tea.Tick(3*time.Second, func(time.Time) tea.Msg {
		return ui.ClearMessageMsg{}
	})
}

// mergeWithPending merges incoming cluster data with pending operations
func (m *model) mergeWithPending(clusters []models.K3sCluster) []models.K3sCluster {
	// Create a map of existing clusters for quick lookup
	clusterMap := make(map[string]models.K3sCluster)
	for _, c := range clusters {
		// Skip if this cluster is marked for deletion
		if m.pendingDeletes[c.ID] {
			continue
		}
		clusterMap[c.ID] = c
	}
	
	// Add pending creates that aren't in the backend yet
	for id, pendingCluster := range m.pendingCreates {
		if _, exists := clusterMap[id]; !exists {
			// This cluster is being created but not in backend yet
			clusterMap[id] = pendingCluster
		}
	}
	
	// Convert map back to slice
	result := make([]models.K3sCluster, 0, len(clusterMap))
	for _, cluster := range clusterMap {
		result = append(result, cluster)
	}
	
	return result
}

func (m *model) clearError() tea.Cmd {
	return tea.Tick(5*time.Second, func(time.Time) tea.Msg {
		return ui.ClearErrorMsg{}
	})
}

// startPolling starts continuous polling for cluster updates


// fetchClusterDetails fetches detailed state for a specific cluster
func (m *model) fetchClusterDetails(clusterName string) tea.Cmd {
	return func() tea.Msg {
		state, err := m.clusterManager.GetClusterDetails(clusterName)
		if err != nil {
			return ui.ErrorMsg{Err: err}
		}
		return ui.ClusterDetailsLoadedMsg{State: state}
	}
}

// listenForUpdates listens for data updates from DataManager
func (m *model) listenForUpdates() tea.Cmd {
	return func() tea.Msg {
		// Get update channel from DataManager
		updateChan, exists := m.dataManager.GetUpdateChannel("main-tui")
		if !exists {
			return nil
		}
		
		// Listen for updates
		update := <-updateChan
		return update
	}
}

// updateClusterTable updates the table with current cluster data
func (m *model) updateClusterTable() {
	if m.clusterTable == nil {
		return
	}
	
	// Pass the data to the component
	m.clusterTable.SetClusters(m.clusters, m.clusterStates)
	// Set table height to fill the content area (total height - 4 for header/footer)
	m.clusterTable.SetDimensions(m.width, m.height - 4)
}


// renderClusterListWithTable renders the cluster list view with persistent table
func (m model) renderClusterListWithTable() string {
	// Build header
	header := m.renderHeader()
	
	// Build table content
	var tableContent string
	contentHeight := m.height - 4 // Account for header (2 lines) and footer (2 lines)
	
	
	if m.clusterTable != nil && len(m.clusters) > 0 {
		// Show the table when we have clusters
		tableContent = m.clusterTable.View()
	} else if len(m.clusters) == 0 {
		// Empty state
		emptyStyle := lipgloss.NewStyle().
			Foreground(ui.ColorGray).
			Italic(true).
			Width(m.width).
			Height(contentHeight).
			Align(lipgloss.Center, lipgloss.Center)
		tableContent = emptyStyle.Render("No clusters found. Press 'c' to create a new cluster.")
	} else {
		// Table not initialized
		tableContent = "Loading table..."
	}
	
	// Build footer
	footer := m.renderFooter()
	
	// Combine all sections
	return lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		tableContent,
		footer,
	)
}

// renderHeader renders the header with title and connection status
func (m model) renderHeader() string {
	// Title
	titleStyle := lipgloss.NewStyle().
		Foreground(ui.ColorWhite).
		Bold(true).
		Padding(0, 1)
	
	title := titleStyle.Render("GOMAN CLUSTERS")
	
	// Connection status
	statusStyle := lipgloss.NewStyle().Foreground(ui.ColorGreen)
	region := m.config.AWSRegion
	statusText := statusStyle.Render(fmt.Sprintf("● Connected to AWS (%s) • Just synced", region))
	
	// Calculate padding for right alignment
	titleWidth := lipgloss.Width(title)
	statusWidth := lipgloss.Width(statusText)
	padding := m.width - titleWidth - statusWidth - 2
	if padding < 0 {
		padding = 1
	}
	
	// Combine title and status
	headerLine := lipgloss.JoinHorizontal(
		lipgloss.Top,
		title,
		strings.Repeat(" ", padding),
		statusText,
		" ",
	)
	
	// Separator
	separator := strings.Repeat("─", m.width)
	sepStyle := lipgloss.NewStyle().Foreground(ui.ColorBorder)
	
	return lipgloss.JoinVertical(
		lipgloss.Left,
		headerLine,
		sepStyle.Render(separator),
	)
}

// renderFooter renders the footer with stats and keyboard shortcuts
func (m model) renderFooter() string {
	// Separator
	separator := strings.Repeat("─", m.width)
	sepStyle := lipgloss.NewStyle().Foreground(ui.ColorBorder)
	
	// Calculate status
	var running, total int
	for _, c := range m.clusters {
		total++
		if c.Status == models.StatusRunning {
			running++
		}
	}
	
	var statusColor lipgloss.Color
	var statusText string
	
	if total == 0 {
		statusColor = ui.ColorGray
		statusText = "○ No clusters"
	} else if running > 0 {
		statusColor = ui.ColorGreen
		statusText = fmt.Sprintf("● %d of %d running", running, total)
	} else {
		statusColor = ui.ColorGray
		statusText = fmt.Sprintf("○ No clusters running")
	}
	
	// Status on the left
	statusStyle := lipgloss.NewStyle().
		Foreground(statusColor)
	
	// Navigation help on the right
	navStyle := lipgloss.NewStyle().
		Foreground(ui.ColorGray)
	
	navText := "↑↓/jk: navigate • ↵: details • c: create • d: delete • s: sync • r: refresh • q: quit"
	
	// Calculate padding for alignment
	statusWidth := lipgloss.Width(statusText)
	navWidth := lipgloss.Width(navText)
	paddingWidth := m.width - statusWidth - navWidth - 4 // 4 for margins
	
	if paddingWidth < 0 {
		paddingWidth = 1
	}
	
	// Create the footer line with proper spacing
	footerLine := lipgloss.JoinHorizontal(
		lipgloss.Top,
		" ", // Left margin
		statusStyle.Render(statusText),
		strings.Repeat(" ", paddingWidth), // Dynamic spacing
		navStyle.Render(navText),
		" ", // Right margin
	)
	
	return lipgloss.JoinVertical(
		lipgloss.Left,
		sepStyle.Render(separator),
		footerLine,
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

func main() {
	// Handle CLI commands
	cli := NewCLI()
	cli.Run()
}

// runMainTUI runs the main TUI interface (called from CLI)
func runMainTUI() {
	zone.NewGlobal()

	m, err := initialModel()
	if err != nil {
		fmt.Printf("Error initializing: %v\n", err)
		os.Exit(1)
	}

	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %v", err)
		os.Exit(1)
	}
}
