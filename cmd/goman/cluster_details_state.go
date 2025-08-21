package main

import (
	"sync"

	clusterPkg "github.com/madhouselabs/goman/pkg/cluster"
	"github.com/madhouselabs/goman/pkg/models"
	"github.com/rivo/tview"
)

// ClusterDetailsState holds the state for the cluster details view
type ClusterDetailsState struct {
	mu              sync.RWMutex
	cluster         models.K3sCluster
	metrics         *clusterPkg.ClusterMetrics
	fetchingMetrics bool
	stopRefresh     chan bool
	
	// UI elements that need updating
	clusterInfoTable *tview.Table
	resourcesTable   *tview.Table
	metricsTable     *tview.Table
	nodePoolsTable   *tview.Table
	statusText       *tview.TextView
}

// Global state for the current details view
var detailsState *ClusterDetailsState

// NewClusterDetailsState creates a new state object
func NewClusterDetailsState(cluster models.K3sCluster) *ClusterDetailsState {
	return &ClusterDetailsState{
		cluster:     cluster,
		stopRefresh: make(chan bool, 1),
	}
}

// UpdateCluster updates the cluster data
func (s *ClusterDetailsState) UpdateCluster(cluster models.K3sCluster) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cluster = cluster
}

// GetCluster returns a copy of the cluster
func (s *ClusterDetailsState) GetCluster() models.K3sCluster {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cluster
}

// UpdateMetrics updates the metrics data
func (s *ClusterDetailsState) UpdateMetrics(metrics *clusterPkg.ClusterMetrics) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.metrics = metrics
}

// GetMetrics returns the current metrics
func (s *ClusterDetailsState) GetMetrics() *clusterPkg.ClusterMetrics {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.metrics
}

// SetFetchingMetrics sets the fetching state
func (s *ClusterDetailsState) SetFetchingMetrics(fetching bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fetchingMetrics = fetching
}

// IsFetchingMetrics returns if metrics are being fetched
func (s *ClusterDetailsState) IsFetchingMetrics() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.fetchingMetrics
}

// StopRefresh signals to stop the refresh goroutine
func (s *ClusterDetailsState) StopRefresh() {
	select {
	case s.stopRefresh <- true:
	default:
	}
}