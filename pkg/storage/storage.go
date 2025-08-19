package storage

import (
	"fmt"
	"os"

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

// K3sClusterState represents the complete state of a k3s cluster with AWS resources
type K3sClusterState struct {
	Cluster        models.K3sCluster      `json:"cluster"`
	InstanceIDs    map[string]string      `json:"instance_ids"`
	VolumeIDs      map[string][]string    `json:"volume_ids"`
	SecurityGroups []string               `json:"security_groups"`
	VPCID          string                 `json:"vpc_id"`
	SubnetIDs      []string               `json:"subnet_ids"`
	Metadata       map[string]interface{} `json:"metadata"`
}

// NewStorage creates a new storage instance with S3 backend
// TODO: Refactor to use provider's storage service for multi-cloud support
func NewStorage() (*Storage, error) {
	profile := os.Getenv("AWS_PROFILE")

	// For now, continue using S3Backend directly for backward compatibility
	// In future, this should use: provider.GetStorageService() -> NewProviderBackend()
	backend, err := NewS3Backend(profile)
	if err != nil {
		return nil, fmt.Errorf("failed to create S3 storage backend: %w", err)
	}

	if err := backend.Initialize(); err != nil {
		return nil, fmt.Errorf("failed to initialize S3 storage: %w", err)
	}

	return &Storage{
		backend: backend,
	}, nil
}

// NewStorageWithBackend creates a new storage instance with specified backend
func NewStorageWithBackend(backend StorageBackend) (*Storage, error) {
	if err := backend.Initialize(); err != nil {
		return nil, err
	}

	return &Storage{
		backend: backend,
	}, nil
}

// NewStorageFromProvider creates a new storage instance using a provider's storage service
// This enables multi-cloud support by using any S3-compatible storage
func NewStorageFromProvider(storageService provider.StorageService, prefix string) (*Storage, error) {
	backend := NewProviderBackend(storageService, prefix)
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
