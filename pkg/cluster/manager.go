package cluster

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/madhouselabs/goman/pkg/models"
	"github.com/madhouselabs/goman/pkg/setup"
	"github.com/madhouselabs/goman/pkg/storage"
)

var (
	setupOnce      sync.Once
	setupCompleted bool
	setupErr       error
)

// Manager handles k3s cluster operations
type Manager struct {
	mu         sync.RWMutex // Protects clusters slice
	clusters   []models.K3sCluster
	storage    *storage.Storage
	stateCache *StateCache // Cache for cluster states with background updates
	hasSynced  bool       // Track if we've done at least one sync
}

// NewManager creates a new cluster manager
func NewManager() *Manager {
	storage, err := storage.NewStorage()
	if err != nil {
		// Fallback to in-memory only if storage fails
		return &Manager{
			clusters: []models.K3sCluster{},
		}
	}

	// Create state cache for background updates
	stateCache := NewStateCache(storage)
	
	// Skip initial load - let background updater handle it
	// This prevents blocking on startup
	
	// Return manager with cache (will populate in background)
	return &Manager{
		clusters:   []models.K3sCluster{},
		storage:    storage,
		stateCache: stateCache,
	}
}

// GetClusters returns all clusters
func (m *Manager) GetClusters() []models.K3sCluster {
	// If we have a state cache, use it for instant response
	if m.stateCache != nil {
		clusters := m.stateCache.GetClusters()
		// If cache is empty and we haven't synced, force a sync
		if len(clusters) == 0 && !m.HasSyncedOnce() {
			// Do a quick synchronous load on first access
			m.loadClustersFromStorage()
		}
		return m.stateCache.GetClusters()
	}
	
	// Fallback to local clusters list
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Return a copy to prevent external modifications
	result := make([]models.K3sCluster, len(m.clusters))
	copy(result, m.clusters)
	return result
}

// loadClustersFromStorage does a synchronous load from storage
func (m *Manager) loadClustersFromStorage() {
	if m.storage == nil {
		return
	}
	
	states, err := m.storage.LoadAllClusterStates()
	if err != nil {
		// Failed to load, but don't block
		return
	}
	
	m.mu.Lock()
	defer m.mu.Unlock()
	
	// Mark as synced
	m.hasSynced = true
	
	// Update clusters
	m.clusters = []models.K3sCluster{}
	for _, state := range states {
		if state != nil {
			m.clusters = append(m.clusters, state.Cluster)
		}
	}
}

// ensureInfrastructureSetup ensures all required infrastructure is set up
func ensureInfrastructureSetup() error {
	setupOnce.Do(func() {
		ctx := context.Background()

		// Run the full setup to ensure everything is configured
		result, err := setup.EnsureFullSetup(ctx)
		if err != nil {
			setupErr = fmt.Errorf("infrastructure setup failed: %w", err)
			return
		}

		if !result.StorageReady {
			setupErr = fmt.Errorf("storage setup failed")
			return
		}

		if !result.FunctionReady && len(result.Errors) > 0 {
			for _, errMsg := range result.Errors {
				_ = errMsg
			}
		}

		setupCompleted = true
	})

	return setupErr
}

// InitializeInfrastructure initializes required infrastructure
// This should be called once during app startup, not during operations
func (m *Manager) InitializeInfrastructure() error {
	return ensureInfrastructureSetup()
}

// CreateCluster creates a new k3s cluster
func (m *Manager) CreateCluster(cluster models.K3sCluster) (*models.K3sCluster, error) {
	// Infrastructure should already be set up at app initialization
	// No need to check again here as it blocks the UI
	// Generate cluster ID and set initial status
	cluster.ID = fmt.Sprintf("k3s-%d", time.Now().Unix())
	cluster.Status = models.StatusCreating
	cluster.CreatedAt = time.Now()
	cluster.UpdatedAt = time.Now()
	cluster.ClusterToken = fmt.Sprintf("K3S%d::server::%s", time.Now().Unix(), cluster.Name)
	cluster.KubeConfigPath = fmt.Sprintf("~/.kube/k3s-%s.yaml", cluster.Name)

	// Set placeholder API endpoint if we have master nodes
	if len(cluster.MasterNodes) > 0 {
		cluster.APIEndpoint = fmt.Sprintf("https://%s:6443", cluster.MasterNodes[0].IP)
	}

	// Calculate initial totals based on requested configuration
	for _, node := range cluster.MasterNodes {
		cluster.TotalCPU += node.CPU
		cluster.TotalMemoryGB += node.MemoryGB
		cluster.TotalStorageGB += node.StorageGB
	}
	for _, node := range cluster.WorkerNodes {
		cluster.TotalCPU += node.CPU
		cluster.TotalMemoryGB += node.MemoryGB
		cluster.TotalStorageGB += node.StorageGB
	}

	// Estimate cost based on mode
	masterCost := 50
	if cluster.Mode == models.ModeHA {
		masterCost = 150 // 3 masters
	}
	cluster.EstimatedCost = float64(masterCost + len(cluster.WorkerNodes)*30)

	// Save initial state to storage FIRST before adding to memory
	// Use the new separated file structure
	if m.storage != nil {
		// Save config file (user-controlled data)
		if err := m.saveClusterConfig(cluster); err != nil {
			return nil, fmt.Errorf("failed to save cluster config: %w", err)
		}

		// Save initial status file (reconciler-controlled data)
		if err := m.saveClusterStatus(cluster); err != nil {
			return nil, fmt.Errorf("failed to save cluster status: %w", err)
		}
		
		// Request cache update for this cluster
		if m.stateCache != nil {
			m.stateCache.RequestUpdate(cluster.Name)
		}
	}

	// Add to cluster list only after successful save
	m.mu.Lock()
	m.clusters = append(m.clusters, cluster)
	m.mu.Unlock()

	return &cluster, nil
}

// DeleteCluster deletes a cluster
func (m *Manager) DeleteCluster(clusterID string) error {
	// Infrastructure should already be set up at app initialization
	// No need to check again here as it blocks the UI

	m.mu.Lock()
	defer m.mu.Unlock()

	for i := range m.clusters {
		if m.clusters[i].ID == clusterID {
			m.clusters[i].Status = models.StatusDeleting
			m.clusters[i].UpdatedAt = time.Now()

			// Update status to mark as deleting
			if m.storage != nil {
				// Only update the status file for deletion
				if err := m.saveClusterStatus(m.clusters[i]); err != nil {
					return fmt.Errorf("failed to save deletion status: %w", err)
				}
			}

			return nil
		}
	}
	return fmt.Errorf("cluster not found: %s", clusterID)
}

// StartCluster starts a stopped cluster
func (m *Manager) StartCluster(clusterID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i := range m.clusters {
		if m.clusters[i].ID == clusterID {
			if m.clusters[i].Status != models.StatusStopped {
				return fmt.Errorf("cluster is not stopped")
			}
			m.clusters[i].Status = models.StatusRunning
			m.clusters[i].UpdatedAt = time.Now()
			return nil
		}
	}
	return fmt.Errorf("cluster not found: %s", clusterID)
}

// StopCluster stops a running cluster
func (m *Manager) StopCluster(clusterID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i := range m.clusters {
		if m.clusters[i].ID == clusterID {
			if m.clusters[i].Status != models.StatusRunning {
				return fmt.Errorf("cluster is not running")
			}
			m.clusters[i].Status = models.StatusStopped
			m.clusters[i].UpdatedAt = time.Now()
			return nil
		}
	}
	return fmt.Errorf("cluster not found: %s", clusterID)
}

// GetClusterByID returns a cluster by ID
func (m *Manager) GetClusterByID(clusterID string) (*models.K3sCluster, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, cluster := range m.clusters {
		if cluster.ID == clusterID {
			// Return a copy to prevent external modifications
			result := cluster
			return &result, nil
		}
	}
	return nil, fmt.Errorf("cluster not found: %s", clusterID)
}

// GetClusterDetails returns detailed cluster information including instance status
func (m *Manager) GetClusterDetails(clusterName string) (*storage.K3sClusterState, error) {
	if m.storage == nil {
		return nil, fmt.Errorf("storage not available")
	}
	
	// Load the full state from storage
	state, err := m.storage.LoadClusterState(clusterName)
	if err != nil {
		return nil, fmt.Errorf("failed to load cluster state: %w", err)
	}
	
	return state, nil
}

// GetAllClusterStates returns states for all clusters
func (m *Manager) GetAllClusterStates() map[string]*storage.K3sClusterState {
	// Use cached states for instant response (no blocking)
	if m.stateCache != nil {
		return m.stateCache.GetAllStates()
	}
	
	// Fallback to empty if no cache
	return make(map[string]*storage.K3sClusterState)
}

// SyncFromProvider syncs cluster state from the cloud provider
func (m *Manager) SyncFromProvider() error {
	// Infrastructure should already be set up at app initialization
	// This method now only syncs cluster state without blocking on setup
	
	// If we have a state cache, request an immediate refresh
	if m.stateCache != nil {
		m.stateCache.RequestFullRefresh()
		// Return immediately - refresh happens in background
		return nil
	}
	
	// Fallback to direct load if no cache (shouldn't happen normally)
	if m.storage != nil {
		states, err := m.storage.LoadAllClusterStates()
		if err != nil {
			return fmt.Errorf("failed to load cluster states: %w", err)
		}

		m.mu.Lock()
		m.clusters = []models.K3sCluster{}
		for _, state := range states {
			m.clusters = append(m.clusters, state.Cluster)
		}
		m.mu.Unlock()
	}

	return nil
}



// RefreshClusterStatus refreshes cluster status from storage
func (m *Manager) RefreshClusterStatus() {
	// Mark that we've done at least one sync
	m.mu.Lock()
	m.hasSynced = true
	m.mu.Unlock()
	
	// If we have a state cache, it's already refreshing in background
	if m.stateCache != nil {
		// Just trigger a refresh request, don't block
		m.stateCache.RequestFullRefresh()
		return
	}
	
	// Fallback to direct refresh if no cache
	// Reload clusters from storage to get latest state
	if m.storage != nil {
		states, err := m.storage.LoadAllClusterStates()
		if err != nil {
			// Failed to refresh cluster status, continue silently
			return
		}

		m.mu.Lock()
		defer m.mu.Unlock()

		// Simply replace the entire list with what's in storage
		// LoadAllClusterStates returns ALL clusters, so no need to merge
		m.clusters = []models.K3sCluster{}
		for _, state := range states {
			m.clusters = append(m.clusters, state.Cluster)
		}
	}
}

// saveClusterConfig saves cluster configuration (user-controlled data)
func (m *Manager) saveClusterConfig(cluster models.K3sCluster) error {
	if m.storage == nil {
		return nil
	}

	// Create config-only state
	configState := &storage.K3sClusterState{
		Cluster: cluster,
		// Config doesn't include instance IDs or metadata
		InstanceIDs: nil,
		VolumeIDs:   nil,
		Metadata:    nil,
	}

	// Save to config.json file
	configKey := fmt.Sprintf("clusters/%s/config.json", cluster.Name)
	return m.storage.SaveClusterStateToKey(configState, configKey)
}

// saveClusterStatus saves cluster status (reconciler-controlled data)
func (m *Manager) saveClusterStatus(cluster models.K3sCluster) error {
	if m.storage == nil {
		return nil
	}

	// Create status-only state
	statusState := &storage.K3sClusterState{
		Cluster: models.K3sCluster{
			Name:      cluster.Name,
			Status:    cluster.Status,
			UpdatedAt: time.Now(),
		},
		InstanceIDs: make(map[string]string),
		VolumeIDs:   make(map[string][]string),
		Metadata:    make(map[string]interface{}),
	}

	// Add metadata for reconciliation
	statusState.Metadata["updated_at"] = time.Now()
	statusState.Metadata["reconcile_needed"] = true
	if cluster.Status == models.StatusDeleting {
		statusState.Metadata["deletion_requested"] = time.Now()
	}

	// Save to status.json file
	statusKey := fmt.Sprintf("clusters/%s/status.json", cluster.Name)
	return m.storage.SaveClusterStateToKey(statusState, statusKey)
}


// HasSyncedOnce returns true if at least one sync has been performed
func (m *Manager) HasSyncedOnce() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.hasSynced
}