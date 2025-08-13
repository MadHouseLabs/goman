package main

import (
	"fmt"
	"os"
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

	// Initialize with empty states - will be populated by DataManager
	return model{
		state:          viewList,
		clusters:       []models.K3sCluster{},
		clusterStates:  make(map[string]*storage.K3sClusterState),
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
		initialLoading: true, // Start with loading state
	}, nil
}

func (m model) Init() tea.Cmd {
	// Subscribe to data updates with 30 second refresh rate
	subscribeCmd := datamanager.SubscribeCmd(m.dataManager, datamanager.SubscriptionConfig{
		SubscriberID: "main-tui",
		DataTypes: []datamanager.DataType{
			datamanager.DataTypeClusters,
			datamanager.DataTypeClusterStates,
		},
		RefreshRate: 30 * time.Second,
		Priority:    datamanager.PriorityNormal,
	})
	
	// Request initial data immediately
	initialDataCmd := datamanager.RequestDataCmd(m.dataManager, datamanager.DataRequest{
		ID:       "initial-load",
		Type:     datamanager.DataTypeClusters,
		Policy:   datamanager.PolicyFreshIfStale,
		Priority: datamanager.PriorityHigh,
	})
	
	// Also trigger immediate refresh via the old polling mechanism
	immediateRefreshCmd := func() tea.Msg {
		return ui.RefreshClustersMsg{}
	}
	
	return tea.Batch(
		m.spinner.Tick,
		tea.EnterAltScreen,
		subscribeCmd,
		initialDataCmd,
		immediateRefreshCmd,        // Trigger immediate refresh
		m.listenForUpdates(), // Start listening for data updates
	)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.viewport.Width = m.width - 4
		m.viewport.Height = m.height - 10
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
				m.clusters = clusterData
				// Only clear initial loading if we've actually loaded data or confirmed empty
				// Don't clear on first empty response as it might be cached empty state
				if len(clusterData) > 0 || m.clusterManager.HasSyncedOnce() {
					m.initialLoading = false
				}
			}
		case datamanager.DataTypeClusterStates:
			if states, ok := msg.Data.(map[string]*storage.K3sClusterState); ok {
				m.clusterStates = states
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
			switch {
			case key.Matches(msg, m.keys.Quit):
				return m, tea.Quit

			case key.Matches(msg, m.keys.Up):
				if m.selectedIndex > 0 {
					m.selectedIndex--
				}

			case key.Matches(msg, m.keys.Down):
				if m.selectedIndex < len(m.clusters)-1 {
					m.selectedIndex++
				}

			case key.Matches(msg, m.keys.Create):
				m.state = viewCreate
				m.form = ui.NewProClusterFormWithConfig(m.config)
				return m, nil

			case key.Matches(msg, m.keys.Enter):
				if len(m.clusters) > 0 && m.selectedIndex < len(m.clusters) {
					m.selectedCluster = &m.clusters[m.selectedIndex]
					m.selectedClusterState = nil // Clear previous state
					m.detailLoading = true        // Set loading state
					m.state = viewDetail
					// Fetch details asynchronously and start spinner
					return m, tea.Batch(
						m.fetchClusterDetails(m.selectedCluster.Name),
						m.spinner.Tick,
					)
				}

			case key.Matches(msg, m.keys.Delete):
				if len(m.clusters) > 0 && m.selectedIndex < len(m.clusters) {
					m.deleteTarget = &m.clusters[m.selectedIndex]
					m.state = viewConfirmDelete
					return m, nil
				}

			case key.Matches(msg, m.keys.Sync):
				// Request fresh data from DataManager
				return m, datamanager.RefreshDataCmd(m.dataManager, datamanager.DataTypeClusters)
			}

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
					// Mark cluster as deleting locally before backend operation
					for i, c := range m.clusters {
						if c.ID == m.deleteTarget.ID {
							m.clusters[i].Status = models.StatusDeleting
							break
						}
					}
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
		m.clusters = m.clusterManager.GetClusters()
		// Update cached states after cluster creation
		m.clusterStates = m.clusterManager.GetAllClusterStates()
		m.state = viewList
		m.form = nil
		m.message = ""
		// Only clear message, don't trigger refresh as polling is already running
		return m, m.clearMessage()

	case ui.ClusterDeletedMsg:
		m.loading = false
		m.clusters = m.clusterManager.GetClusters()
		// Update cached states after cluster deletion
		m.clusterStates = m.clusterManager.GetAllClusterStates()
		if m.selectedIndex >= len(m.clusters) && m.selectedIndex > 0 {
			m.selectedIndex--
		}
		m.message = ""
		return m, m.clearMessage()

	case ui.ClustersSyncedMsg:
		m.loading = false
		m.clusters = m.clusterManager.GetClusters()
		// Update cached states after sync
		m.clusterStates = m.clusterManager.GetAllClusterStates()
		m.message = ""
		return m, m.clearMessage()

	case ui.RefreshClustersMsg:
		// Silently refresh cluster list to update statuses
		m.clusterManager.RefreshClusterStatus()
		m.clusters = m.clusterManager.GetClusters()
		// Cache cluster states for rendering
		m.clusterStates = m.clusterManager.GetAllClusterStates()
		// Clear initial loading since we've done a real refresh
		m.initialLoading = false
		// Continue polling for updates
		return m, m.startPolling()

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
		// Use cached cluster states for rendering (no API calls)
		// Use RenderClusterListWithStates to get proper viewport wrapping
		return ui.RenderClusterListWithStates(m.width, m.height, m.clusters, m.clusterStates, m.selectedIndex)
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

func (m *model) refreshClusters() tea.Cmd {
	return tea.Tick(3*time.Second, func(time.Time) tea.Msg {
		return ui.RefreshClustersMsg{}
	})
}

func (m *model) clearMessage() tea.Cmd {
	return tea.Tick(3*time.Second, func(time.Time) tea.Msg {
		return ui.ClearMessageMsg{}
	})
}

func (m *model) clearError() tea.Cmd {
	return tea.Tick(5*time.Second, func(time.Time) tea.Msg {
		return ui.ClearErrorMsg{}
	})
}

// startPolling starts continuous polling for cluster updates
func (m *model) startPolling() tea.Cmd {
	return tea.Tick(15*time.Second, func(time.Time) tea.Msg {
		return ui.RefreshClustersMsg{}
	})
}

// fetchInitialStates fetches cluster states immediately for initial render
func (m *model) fetchInitialStates() tea.Cmd {
	return func() tea.Msg {
		// This will be handled by RefreshClustersMsg handler
		return ui.RefreshClustersMsg{}
	}
}

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
