package storage

import (
	"github.com/madhouselabs/goman/pkg/models"
)

// StorageBackend defines the interface for different storage implementations
type StorageBackend interface {
	// Cluster operations
	SaveClusters(clusters []models.K3sCluster) error
	LoadClusters() ([]models.K3sCluster, error)

	// Cluster state operations
	SaveClusterState(state *K3sClusterState) error
	LoadClusterState(clusterName string) (*K3sClusterState, error)
	LoadAllClusterStates() ([]*K3sClusterState, error)
	DeleteClusterState(clusterName string) error

	// Job operations
	SaveJob(job *models.Job) error
	LoadJob(jobID string) (*models.Job, error)
	LoadAllJobs() ([]*models.Job, error)
	DeleteJob(jobID string) error

	// Config operations
	SaveConfig(config map[string]interface{}) error
	LoadConfig() (map[string]interface{}, error)

	// Initialize storage backend
	Initialize() error
}
