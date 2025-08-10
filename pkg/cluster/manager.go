package cluster

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/madhouselabs/goman/pkg/models"
	"github.com/madhouselabs/goman/pkg/provider/registry"
	"github.com/madhouselabs/goman/pkg/setup"
	"github.com/madhouselabs/goman/pkg/storage"
)

var (
	setupOnce sync.Once
	setupCompleted bool
	setupErr error
)

// Manager handles k3s cluster operations
type Manager struct {
	mu       sync.RWMutex // Protects clusters slice
	clusters []models.K3sCluster
	storage  *storage.Storage
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

	// Load clusters from storage
	states, err := storage.LoadAllClusterStates()
	if err != nil {
		// If error loading states, start with empty list
		return &Manager{
			clusters: []models.K3sCluster{},
			storage:  storage,
		}
	}

	// Extract clusters from states
	var clusters []models.K3sCluster
	for _, state := range states {
		clusters = append(clusters, state.Cluster)
	}

	return &Manager{
		clusters: clusters,
		storage:  storage,
	}
}

// GetClusters returns all clusters
func (m *Manager) GetClusters() []models.K3sCluster {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	// Return a copy to prevent external modifications
	result := make([]models.K3sCluster, len(m.clusters))
	copy(result, m.clusters)
	return result
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
		
		// Check if critical components are set up
		if !result.S3BucketCreated {
			setupErr = fmt.Errorf("S3 bucket setup failed")
			return
		}
		
		if !result.LambdaDeployed && len(result.Errors) > 0 {
			// Lambda deployment failed, but we can continue
			// The clusters will be created but won't be reconciled
			// Log the errors but don't fail
			for _, errMsg := range result.Errors {
				// Silently log errors (in production, use proper logging)
				_ = errMsg
			}
		}
		
		setupCompleted = true
	})
	
	return setupErr
}

// CreateCluster creates a new k3s cluster
func (m *Manager) CreateCluster(cluster models.K3sCluster) (*models.K3sCluster, error) {
	// Ensure infrastructure is set up before creating cluster
	if err := ensureInfrastructureSetup(); err != nil {
		return nil, fmt.Errorf("failed to ensure infrastructure setup: %w", err)
	}
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
	
	// Estimate cost
	cluster.EstimatedCost = float64(len(cluster.MasterNodes)*50 + len(cluster.WorkerNodes)*30)
	
	// Save initial state to storage FIRST before adding to memory
	if m.storage != nil {
		k3sState := &storage.K3sClusterState{
			Cluster:        cluster,
			InstanceIDs:    make(map[string]string),
			VolumeIDs:      make(map[string][]string),
			Metadata:       make(map[string]interface{}),
		}
		k3sState.Metadata["created_at"] = time.Now()
		k3sState.Metadata["reconcile_needed"] = true
		
		// Save state - this will trigger Lambda via S3 events
		err := m.storage.SaveClusterState(k3sState)
		if err != nil {
			// Failed to save cluster state, return error
			return nil, fmt.Errorf("failed to save cluster state: %w", err)
		}
		
		// Also manually invoke Lambda to ensure it processes the cluster
		// This is a backup in case S3 notifications aren't working
		go m.invokeLambdaForCluster(cluster.Name)
	}
	
	// Add to cluster list only after successful save
	m.mu.Lock()
	m.clusters = append(m.clusters, cluster)
	m.mu.Unlock()
	
	return &cluster, nil
}

// DeleteCluster deletes a cluster
func (m *Manager) DeleteCluster(clusterID string) error {
	// Ensure infrastructure is set up before deleting cluster
	if err := ensureInfrastructureSetup(); err != nil {
		return fmt.Errorf("failed to ensure infrastructure setup: %w", err)
	}
	
	m.mu.Lock()
	defer m.mu.Unlock()
	
	for i := range m.clusters {
		if m.clusters[i].ID == clusterID {
			m.clusters[i].Status = models.StatusDeleting
			m.clusters[i].UpdatedAt = time.Now()
			
			// Update state to mark as deleting
			if m.storage != nil {
				if state, err := m.storage.LoadClusterState(m.clusters[i].Name); err == nil {
					state.Cluster.Status = models.StatusDeleting
					state.Cluster.UpdatedAt = time.Now()
					if state.Metadata == nil {
						state.Metadata = make(map[string]interface{})
					}
					state.Metadata["deletion_requested"] = time.Now()
					state.Metadata["reconcile_needed"] = true
					
					// Save state - this will trigger Lambda via S3 events
					err := m.storage.SaveClusterState(state)
					if err != nil {
						// Failed to save deletion state, return error
						return fmt.Errorf("failed to save deletion state: %w", err)
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

// SyncFromAWS syncs cluster state from AWS
func (m *Manager) SyncFromAWS(profile, region string) error {
	// Ensure infrastructure is set up before syncing
	if err := ensureInfrastructureSetup(); err != nil {
		return fmt.Errorf("failed to ensure infrastructure setup: %w", err)
	}
	
	// In the Lambda architecture, syncing happens automatically
	// through S3 state updates. This method could trigger a manual sync.
	// Sync requested silently
	
	// Reload state from storage
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

// invokeLambdaForCluster manually invokes the Lambda function for a cluster
func (m *Manager) invokeLambdaForCluster(clusterName string) {
	ctx := context.Background()
	
	// Get the provider
	prov, err := registry.GetDefaultProvider()
	if err != nil {
		// Silently continue
		return
	}
	
	// Get function service
	functionService := prov.GetFunctionService()
	
	// Create payload
	payload := fmt.Sprintf(`{"clusterName": "%s"}`, clusterName)
	
	// Invoke function - best effort, ignore errors
	_, _ = functionService.InvokeFunction(ctx, "goman-cluster-controller", []byte(payload))
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
		
		m.mu.Lock()
		defer m.mu.Unlock()
		
		// Replace entire clusters slice with latest state
		m.clusters = []models.K3sCluster{}
		for _, state := range states {
			m.clusters = append(m.clusters, state.Cluster)
		}
	}
}