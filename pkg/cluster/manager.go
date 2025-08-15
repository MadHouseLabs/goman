package cluster

import (
	"context"
	"encoding/json"
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

	// Load initial clusters from storage
	manager := &Manager{
		clusters:   []models.K3sCluster{},
		storage:    storage,
	}
	
	// Do initial load synchronously
	manager.loadClustersFromStorage()
	
	return manager
}

// GetClusters returns all clusters
func (m *Manager) GetClusters() []models.K3sCluster {
	// Always return from in-memory list
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
		
		// The controller will create the status.json file when it reconciles
		// We only manage config.json from the UI side
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

			// Update config to mark as deleting - this is the source of truth
			// The controller will see this and update status accordingly
			if m.storage != nil {
				// Load existing config to preserve all fields
				configKey := fmt.Sprintf("clusters/%s/config.json", m.clusters[i].Name)
				if s3Backend, ok := m.storage.GetBackend().(*storage.S3Backend); ok {
					configData, err := s3Backend.GetObject(context.Background(), configKey)
					if err != nil {
						return fmt.Errorf("failed to load cluster config: %w", err)
					}
					
					var config storage.ClusterConfig
					if err := json.Unmarshal(configData, &config); err != nil {
						return fmt.Errorf("failed to unmarshal config: %w", err)
					}
					
					// Set deletion timestamp in metadata
					now := time.Now()
					config.Metadata.DeletionTimestamp = &now
					config.Metadata.UpdatedAt = now
					
					// Save updated config
					updatedData, err := json.MarshalIndent(config, "", "  ")
					if err != nil {
						return fmt.Errorf("failed to marshal updated config: %w", err)
					}
					
					if err := s3Backend.PutObject(context.Background(), configKey, updatedData); err != nil {
						return fmt.Errorf("failed to save deletion config: %w", err)
					}
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
	// Load directly from storage
	if m.storage != nil {
		states, err := m.storage.LoadAllClusterStates()
		if err != nil {
			// Return empty map on error
			return make(map[string]*storage.K3sClusterState)
		}
		
		// Convert to map
		result := make(map[string]*storage.K3sClusterState)
		for _, state := range states {
			if state != nil && state.Cluster.Name != "" {
				result[state.Cluster.Name] = state
			}
		}
		return result
	}
	
	// Fallback to empty if no storage
	return make(map[string]*storage.K3sClusterState)
}

// SyncFromProvider syncs cluster state from the cloud provider
func (m *Manager) SyncFromProvider() error {
	// Load directly from storage
	if m.storage != nil {
		states, err := m.storage.LoadAllClusterStates()
		if err != nil {
			return fmt.Errorf("failed to load cluster states: %w", err)
		}

		// Build new cluster list first, then atomically swap
		newClusters := []models.K3sCluster{}
		for _, state := range states {
			newClusters = append(newClusters, state.Cluster)
		}
		
		m.mu.Lock()
		// Only update if we got valid data (preserve existing state on error/empty)
		if len(newClusters) > 0 || len(states) == 0 {
			m.clusters = newClusters
		}
		m.hasSynced = true
		m.mu.Unlock()
	}

	return nil
}



// RefreshClusterStatus refreshes cluster status from storage
func (m *Manager) RefreshClusterStatus() {
	// Reload clusters from storage to get latest state
	if m.storage != nil {
		states, err := m.storage.LoadAllClusterStates()
		if err != nil {
			// Failed to refresh cluster status, continue silently
			return
		}

		// Build new cluster list first, then atomically swap
		newClusters := []models.K3sCluster{}
		for _, state := range states {
			newClusters = append(newClusters, state.Cluster)
		}

		m.mu.Lock()
		defer m.mu.Unlock()

		// Only update if we got valid data (preserve existing state on error/empty)
		// If S3 returns empty but we have existing clusters, keep them temporarily
		if len(newClusters) > 0 || (len(states) == 0 && m.hasSynced) {
			m.clusters = newClusters
		}
		m.hasSynced = true
	}
}

// saveClusterConfig saves cluster configuration (user-controlled data)
func (m *Manager) saveClusterConfig(cluster models.K3sCluster) error {
	if m.storage == nil {
		return nil
	}

	// Convert to proper config structure (without status)
	config := storage.ConvertToClusterConfig(cluster)

	// Save to config.json file
	configKey := fmt.Sprintf("clusters/%s/config.json", cluster.Name)
	
	// We need to marshal this ourselves since SaveClusterStateToKey expects K3sClusterState
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	
	// Use the S3 backend directly to save the raw data
	if s3Backend, ok := m.storage.GetBackend().(*storage.S3Backend); ok {
		return s3Backend.PutObject(context.Background(), configKey, data)
	}
	
	return fmt.Errorf("storage backend does not support direct object put")
}



// HasSyncedOnce returns true if at least one sync has been performed
func (m *Manager) HasSyncedOnce() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.hasSynced
}