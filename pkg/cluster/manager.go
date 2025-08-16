package cluster

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
	"gopkg.in/yaml.v3"

	"github.com/madhouselabs/goman/pkg/config"
	"github.com/madhouselabs/goman/pkg/models"
	"github.com/madhouselabs/goman/pkg/provider/aws"
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
	
	// Remember which clusters are marked as deleting locally
	// Only preserve this status if the cluster still exists in storage
	deletingClusters := make(map[string]bool)
	existingClusterNames := make(map[string]bool)
	
	// First pass: identify which clusters exist in storage
	for _, state := range states {
		if state != nil && state.Cluster.Name != "" {
			existingClusterNames[state.Cluster.Name] = true
		}
	}
	
	// Only remember deleting status for clusters that still exist
	for _, c := range m.clusters {
		if c.Status == models.StatusDeleting && existingClusterNames[c.Name] {
			deletingClusters[c.Name] = true
		}
	}
	
	// Update clusters
	m.clusters = []models.K3sCluster{}
	for _, state := range states {
		if state != nil {
			cluster := state.Cluster
			
			// If we marked it as deleting locally AND it still exists, keep that status
			// This preserves the deleting status during Lambda reconciliation
			if deletingClusters[cluster.Name] {
				cluster.Status = models.StatusDeleting
			} else if cluster.Status == models.StatusCreating {
				// Try to get real status from AWS if status file doesn't exist
				m.updateClusterStatusFromAWS(&cluster)
			}
			// Don't override status if it's already set properly from storage
			
			m.clusters = append(m.clusters, cluster)
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
		// Clean up any leftover files from previous cluster with same name
		// This prevents new clusters from picking up old "deleting" status
		if s3Backend, ok := m.storage.GetBackend().(*storage.S3Backend); ok {
			ctx := context.Background()
			
			// Delete any existing config and status files
			configKey := fmt.Sprintf("clusters/%s/config.yaml", cluster.Name)
			statusKey := fmt.Sprintf("clusters/%s/status.yaml", cluster.Name)
			
			// Delete both files
			s3Backend.DeleteObject(ctx, configKey)
			s3Backend.DeleteObject(ctx, statusKey)
			
			// Wait a bit longer to ensure:
			// 1. S3 eventual consistency catches up
			// 2. Any in-flight Lambda processing of the old cluster completes
			// This prevents the new cluster from picking up stale status
			time.Sleep(500 * time.Millisecond)
		}
		
		// Save config file (user-controlled data)
		if err := m.saveClusterConfig(cluster); err != nil {
			return nil, fmt.Errorf("failed to save cluster config: %w", err)
		}
		
		// The controller will create the status.yaml file when it reconciles
		// We only manage config.yaml from the UI side
	}

	// Add to cluster list only after successful save
	m.mu.Lock()
	// Remove any existing cluster with the same name (e.g., one marked as deleting)
	for i := len(m.clusters) - 1; i >= 0; i-- {
		if m.clusters[i].Name == cluster.Name {
			// Remove the old cluster entry
			m.clusters = append(m.clusters[:i], m.clusters[i+1:]...)
		}
	}
	// Now add the new cluster
	m.clusters = append(m.clusters, cluster)
	m.mu.Unlock()

	return &cluster, nil
}

// UpdateCluster updates an existing cluster
func (m *Manager) UpdateCluster(cluster models.K3sCluster) (*models.K3sCluster, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Find and update the cluster in memory
	found := false
	for i := range m.clusters {
		if m.clusters[i].ID == cluster.ID || m.clusters[i].Name == cluster.Name {
			// Validate that mode is not being changed
			if m.clusters[i].Mode != cluster.Mode {
				return nil, fmt.Errorf("cluster mode cannot be changed after creation (current: %s, attempted: %s)", 
					m.clusters[i].Mode, cluster.Mode)
			}
			
			// Update fields
			m.clusters[i].Name = cluster.Name
			m.clusters[i].Description = cluster.Description
			m.clusters[i].Region = cluster.Region
			m.clusters[i].InstanceType = cluster.InstanceType
			m.clusters[i].UpdatedAt = time.Now()
			found = true
			cluster = m.clusters[i]
			break
		}
	}

	if !found {
		return nil, fmt.Errorf("cluster not found")
	}

	// Save updated config to storage
	if m.storage != nil {
		if err := m.saveClusterConfig(cluster); err != nil {
			return nil, fmt.Errorf("failed to save updated cluster config: %w", err)
		}
	}

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
			clusterName := m.clusters[i].Name
			
			// Mark for deletion in memory - KEEP IN LIST
			m.clusters[i].Status = models.StatusDeleting
			m.clusters[i].UpdatedAt = time.Now()

			// Try to update config with deletion timestamp if it exists
			if m.storage != nil {
				configKey := fmt.Sprintf("clusters/%s/config.yaml", clusterName)
				if s3Backend, ok := m.storage.GetBackend().(*storage.S3Backend); ok {
					// Try to load existing config
					configData, err := s3Backend.GetObject(context.Background(), configKey)
					if err != nil {
						// If config doesn't exist, it's already deleted
						if strings.Contains(err.Error(), "not found") {
							// Clean up any remaining state files
							m.storage.DeleteClusterState(clusterName)
							// Remove from in-memory list since it's already gone
							m.clusters = append(m.clusters[:i], m.clusters[i+1:]...)
							return nil
						}
						// For other errors, log but keep showing as deleting
						fmt.Printf("Warning: Could not load config for deletion: %v\n", err)
						// Keep in list with deleting status
						return nil
					}
					
					// Config exists, update it with deletion timestamp
					var config storage.ClusterConfig
					if err := yaml.Unmarshal(configData, &config); err != nil {
						fmt.Printf("Warning: Could not unmarshal config: %v\n", err)
						// Keep in list with deleting status
						return nil
					}
					
					// Set deletion timestamp
					now := time.Now()
					config.Metadata.DeletionTimestamp = &now
					config.Metadata.UpdatedAt = now
					
					// Save updated config - this will trigger Lambda
					updatedData, err := yaml.Marshal(config)
					if err != nil {
						fmt.Printf("Warning: Could not marshal config: %v\n", err)
						// Keep in list with deleting status
						return nil
					}
					
					// Save the updated config with deletion timestamp
					// This will trigger Lambda to process the deletion
					if err := s3Backend.PutObject(context.Background(), configKey, updatedData); err != nil {
						fmt.Printf("Warning: Could not save deletion config: %v\n", err)
						// Still keep in list with deleting status
					}
					
					// DON'T remove from list - let it show as "deleting" until Lambda removes files
					// The loadClustersFromStorage() will remove it when files are gone
				}
			}
			
			// Keep cluster in list with "deleting" status
			// It will be removed when Lambda deletes the S3 files and refresh happens
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

// updateClusterStatusFromAWS queries AWS to get real instance status
func (m *Manager) updateClusterStatusFromAWS(cluster *models.K3sCluster) {
	// Get AWS provider
	cfg, err := config.NewConfig()
	if err != nil {
		return
	}
	
	provider, err := aws.GetCachedProvider(cfg.AWSProfile, cfg.AWSRegion)
	if err != nil {
		return
	}
	
	// Query instances for this cluster
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	
	// Don't include terminated instances - they might be from a previous cluster with the same name
	filters := map[string]string{
		"tag:ClusterName": cluster.Name,
		"instance-state-name": "pending,running,stopping,stopped",
	}
	
	instances, err := provider.GetComputeService().ListInstances(ctx, filters)
	if err != nil {
		return
	}
	
	// Determine cluster status based on instances
	if len(instances) == 0 {
		// No instances found, cluster doesn't exist yet or was deleted
		// Don't change status - keep it as creating
		return
	}
	
	// Check instance states
	allRunning := true
	hasStopped := false
	
	for _, inst := range instances {
		switch inst.State {
		case "running":
			// Good state
		case "pending":
			allRunning = false
		case "stopped", "stopping":
			hasStopped = true
			allRunning = false
		default:
			allRunning = false
		}
	}
	
	// Set status based on instance states
	// Note: We don't check for terminated instances since we filter them out
	if hasStopped {
		cluster.Status = models.StatusStopped
	} else if allRunning {
		cluster.Status = models.StatusRunning
	} else {
		cluster.Status = models.StatusCreating
	}
	
	// Update node count
	masterCount := 0
	workerCount := 0
	for _, inst := range instances {
		if inst.State == "running" || inst.State == "pending" {
			// Simple heuristic: if name contains "master", it's a master node
			if strings.Contains(inst.Name, "master") {
				masterCount++
			} else {
				workerCount++
			}
		}
	}
	
	// Update master/worker nodes if we found instances
	if masterCount > 0 || workerCount > 0 {
		// Clear and rebuild node lists based on actual instances
		cluster.MasterNodes = []models.Node{}
		cluster.WorkerNodes = []models.Node{}
		
		for _, inst := range instances {
			if inst.State == "running" || inst.State == "pending" {
				node := models.Node{
					ID:     inst.ID,
					Name:   inst.Name,
					IP:     inst.PrivateIP,
					Status: inst.State,
				}
				
				if strings.Contains(inst.Name, "master") {
					node.Role = models.RoleMaster
					cluster.MasterNodes = append(cluster.MasterNodes, node)
				} else {
					node.Role = models.RoleWorker
					cluster.WorkerNodes = append(cluster.WorkerNodes, node)
				}
			}
		}
	}
}

// saveClusterConfig saves cluster configuration (user-controlled data)
func (m *Manager) saveClusterConfig(cluster models.K3sCluster) error {
	if m.storage == nil {
		return nil
	}

	// Convert to proper config structure (without status)
	config := storage.ConvertToClusterConfig(cluster)

	// Save to config.yaml file
	configKey := fmt.Sprintf("clusters/%s/config.yaml", cluster.Name)
	
	// Marshal as YAML
	data, err := yaml.Marshal(config)
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