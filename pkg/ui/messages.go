package ui

import (
	"github.com/madhouselabs/goman/pkg/models"
	"github.com/madhouselabs/goman/pkg/storage"
)

// Message types for UI events
type ClusterCreatedMsg struct{ Cluster *models.K3sCluster }
type ClusterDeletedMsg struct{}
type ClusterStartedMsg struct{}
type ClusterStoppedMsg struct{}
type ClustersSyncedMsg struct{}
type RefreshClustersMsg struct{}
type ErrorMsg struct{ Err error }
type ClearMessageMsg struct{}
type ClearErrorMsg struct{}
type ClusterDetailsLoadedMsg struct{ State *storage.K3sClusterState }