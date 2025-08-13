package cluster

import (
	"context"
	"sync"
	"time"

	"github.com/madhouselabs/goman/pkg/models"
	"github.com/madhouselabs/goman/pkg/storage"
)

// Type alias for easier use
type ClusterState = storage.K3sClusterState

// StateCache manages cluster state caching with background updates
type StateCache struct {
	storage       *storage.Storage
	states        map[string]*ClusterState
	clusters      map[string]*models.K3sCluster
	mu            sync.RWMutex
	updateChan    chan updateRequest
	stopChan      chan struct{}
	lastUpdate    time.Time
	updatePeriod  time.Duration
}

type updateRequest struct {
	clusterName string
	immediate   bool
}

// NewStateCache creates a new state cache manager
func NewStateCache(storage *storage.Storage) *StateCache {
	cache := &StateCache{
		storage:      storage,
		states:       make(map[string]*ClusterState),
		clusters:     make(map[string]*models.K3sCluster),
		updateChan:   make(chan updateRequest, 100),
		stopChan:     make(chan struct{}),
		updatePeriod: 15 * time.Second, // Default refresh period
	}
	
	// Start background update goroutine
	go cache.backgroundUpdater()
	
	// Request immediate initial load
	cache.RequestFullRefresh()
	
	return cache
}

// backgroundUpdater continuously updates the cache in the background
func (c *StateCache) backgroundUpdater() {
	ticker := time.NewTicker(c.updatePeriod)
	defer ticker.Stop()
	
	for {
		select {
		case <-c.stopChan:
			return
			
		case req := <-c.updateChan:
			// Handle immediate update requests
			if req.clusterName != "" {
				c.updateClusterState(req.clusterName)
			} else {
				// Update all clusters
				c.refreshAllStates()
			}
			
		case <-ticker.C:
			// Periodic refresh of all states
			c.refreshAllStates()
		}
	}
}

// refreshAllStates refreshes all cluster states from storage
func (c *StateCache) refreshAllStates() {
	if c.storage == nil {
		return
	}
	
	// Load all states from storage
	states, err := c.storage.LoadAllClusterStates()
	if err != nil {
		// Log error but don't crash - keep using cached data
		return
	}
	
	// Update cache atomically
	c.mu.Lock()
	defer c.mu.Unlock()
	
	// Clear and rebuild cache
	c.states = make(map[string]*ClusterState)
	c.clusters = make(map[string]*models.K3sCluster)
	
	for _, state := range states {
		if state != nil {
			c.states[state.Cluster.Name] = state
			c.clusters[state.Cluster.Name] = &state.Cluster
		}
	}
	
	c.lastUpdate = time.Now()
}

// updateClusterState updates a single cluster's state
func (c *StateCache) updateClusterState(clusterName string) {
	if c.storage == nil {
		return
	}
	
	state, err := c.storage.LoadClusterState(clusterName)
	if err != nil {
		// If error, remove from cache
		c.mu.Lock()
		delete(c.states, clusterName)
		delete(c.clusters, clusterName)
		c.mu.Unlock()
		return
	}
	
	// Update cache
	c.mu.Lock()
	if state != nil {
		c.states[clusterName] = state
		c.clusters[clusterName] = &state.Cluster
	} else {
		delete(c.states, clusterName)
		delete(c.clusters, clusterName)
	}
	c.mu.Unlock()
}

// GetState returns cached state for a cluster (instant, no blocking)
func (c *StateCache) GetState(clusterName string) *ClusterState {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.states[clusterName]
}

// GetAllStates returns all cached states (instant, no blocking)
func (c *StateCache) GetAllStates() map[string]*ClusterState {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	// Return a copy to prevent external modifications
	result := make(map[string]*ClusterState)
	for k, v := range c.states {
		result[k] = v
	}
	return result
}

// GetClusters returns all cached clusters (instant, no blocking)
func (c *StateCache) GetClusters() []models.K3sCluster {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	result := make([]models.K3sCluster, 0, len(c.clusters))
	for _, cluster := range c.clusters {
		if cluster != nil {
			result = append(result, *cluster)
		}
	}
	return result
}

// RequestUpdate requests an immediate update for a specific cluster or all clusters
func (c *StateCache) RequestUpdate(clusterName string) {
	select {
	case c.updateChan <- updateRequest{clusterName: clusterName, immediate: true}:
	default:
		// Channel full, skip request (background update will catch it)
	}
}

// RequestFullRefresh requests a full refresh of all clusters
func (c *StateCache) RequestFullRefresh() {
	select {
	case c.updateChan <- updateRequest{immediate: true}:
	default:
		// Channel full, skip request (background update will catch it)
	}
}

// InitialLoad performs the initial load of all states
func (c *StateCache) InitialLoad(ctx context.Context) error {
	if c.storage == nil {
		return nil
	}
	
	// Do initial load synchronously to populate cache
	states, err := c.storage.LoadAllClusterStates()
	if err != nil {
		return err
	}
	
	c.mu.Lock()
	defer c.mu.Unlock()
	
	for _, state := range states {
		if state != nil {
			c.states[state.Cluster.Name] = state
			c.clusters[state.Cluster.Name] = &state.Cluster
		}
	}
	
	c.lastUpdate = time.Now()
	return nil
}

// Stop stops the background updater
func (c *StateCache) Stop() {
	close(c.stopChan)
}

// GetLastUpdateTime returns when the cache was last updated
func (c *StateCache) GetLastUpdateTime() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastUpdate
}

// SetUpdatePeriod sets how often the cache refreshes
func (c *StateCache) SetUpdatePeriod(period time.Duration) {
	c.updatePeriod = period
}