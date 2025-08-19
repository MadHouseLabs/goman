package storage

import (
	"fmt"

	"github.com/madhouselabs/goman/pkg/models"
	"github.com/madhouselabs/goman/pkg/provider"
)

// Storage wraps a StorageBackend
type Storage struct {
	backend StorageBackend
}

// GetBackend returns the underlying storage backend
func (s *Storage) GetBackend() StorageBackend {
	return s.backend
}

// K3sClusterState represents the complete state of a k3s cluster with cloud resources
type K3sClusterState struct {
	Cluster        models.K3sCluster      `json:"cluster"`
	InstanceIDs    map[string]string      `json:"instance_ids"`
	VolumeIDs      map[string][]string    `json:"volume_ids"`
	SecurityGroups []string               `json:"security_groups"`
	VPCID          string                 `json:"vpc_id"`
	SubnetIDs      []string               `json:"subnet_ids"`
	Metadata       map[string]interface{} `json:"metadata"`
}

// NewStorage is deprecated - use NewStorageWithProvider instead
// This function is kept for backward compatibility but requires the caller
// to have already initialized a provider
func NewStorage() (*Storage, error) {
	return nil, fmt.Errorf("NewStorage() requires a provider. Use NewStorageWithProvider() instead")
}

// NewStorageWithProvider creates a new storage instance with a specific provider
func NewStorageWithProvider(p provider.Provider) (*Storage, error) {
	storageService := p.GetStorageService()
	
	backend := NewProviderBackend(storageService, "")
	if err := backend.Initialize(); err != nil {
		return nil, fmt.Errorf("failed to initialize provider storage: %w", err)
	}

	return &Storage{
		backend: backend,
	}, nil
}

// SaveClusters saves k3s clusters using the backend
func (s *Storage) SaveClusters(clusters []models.K3sCluster) error {
	return s.backend.SaveClusters(clusters)
}

// LoadClusters loads k3s clusters using the backend
func (s *Storage) LoadClusters() ([]models.K3sCluster, error) {
	return s.backend.LoadClusters()
}

// SaveClusterState saves complete cluster state using the backend
func (s *Storage) SaveClusterState(state *K3sClusterState) error {
	return s.backend.SaveClusterState(state)
}

// LoadClusterState loads complete cluster state using the backend
func (s *Storage) LoadClusterState(clusterName string) (*K3sClusterState, error) {
	return s.backend.LoadClusterState(clusterName)
}

// LoadAllClusterStates loads all cluster states using the backend
func (s *Storage) LoadAllClusterStates() ([]*K3sClusterState, error) {
	return s.backend.LoadAllClusterStates()
}

// DeleteClusterState deletes cluster state using the backend
func (s *Storage) DeleteClusterState(clusterName string) error {
	return s.backend.DeleteClusterState(clusterName)
}

// SaveConfig saves application configuration using the backend
func (s *Storage) SaveConfig(config map[string]interface{}) error {
	return s.backend.SaveConfig(config)
}

// LoadConfig loads application configuration using the backend
func (s *Storage) LoadConfig() (map[string]interface{}, error) {
	return s.backend.LoadConfig()
}

// SaveClusterStateToKey saves cluster state to a specific key
func (s *Storage) SaveClusterStateToKey(state *K3sClusterState, key string) error {
	// Use the backend's SaveClusterStateToKey if available
	if backend, ok := s.backend.(interface {
		SaveClusterStateToKey(*K3sClusterState, string) error
	}); ok {
		return backend.SaveClusterStateToKey(state, key)
	}
	// Fallback to regular SaveClusterState
	return s.backend.SaveClusterState(state)
}
