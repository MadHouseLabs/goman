package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/madhouselabs/goman/pkg/models"
	"github.com/madhouselabs/goman/pkg/provider"
	"gopkg.in/yaml.v3"
)

// ProviderBackend implements StorageBackend using a provider's StorageService
// This allows any cloud provider with S3-compatible storage to be used
type ProviderBackend struct {
	storageService provider.StorageService
	prefix         string
}

// NewProviderBackend creates a new provider-based storage backend
func NewProviderBackend(storageService provider.StorageService, prefix string) *ProviderBackend {
	return &ProviderBackend{
		storageService: storageService,
		prefix:         prefix,
	}
}

// Initialize creates necessary storage structure
func (pb *ProviderBackend) Initialize() error {
	return pb.storageService.Initialize(context.Background())
}

// getKey returns the full key with prefix
func (pb *ProviderBackend) getKey(path string) string {
	if pb.prefix != "" {
		return fmt.Sprintf("%s/%s", pb.prefix, path)
	}
	return path
}

// SaveClusters saves k3s clusters
func (pb *ProviderBackend) SaveClusters(clusters []models.K3sCluster) error {
	data, err := json.MarshalIndent(clusters, "", "  ")
	if err != nil {
		return err
	}

	key := pb.getKey("clusters/clusters.json")
	return pb.storageService.PutObject(context.Background(), key, data)
}

// LoadClusters loads k3s clusters
func (pb *ProviderBackend) LoadClusters() ([]models.K3sCluster, error) {
	key := pb.getKey("clusters/clusters.json")
	data, err := pb.storageService.GetObject(context.Background(), key)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return []models.K3sCluster{}, nil
		}
		return nil, err
	}

	var clusters []models.K3sCluster
	if err := json.Unmarshal(data, &clusters); err != nil {
		return nil, err
	}

	return clusters, nil
}

// SaveClusterState saves complete cluster state
func (pb *ProviderBackend) SaveClusterState(state *K3sClusterState) error {
	if state == nil || state.Cluster.Name == "" {
		return fmt.Errorf("invalid cluster state")
	}

	// Save as YAML for consistency with existing format
	key := pb.getKey(fmt.Sprintf("clusters/%s.yaml", state.Cluster.Name))
	
	data, err := yaml.Marshal(state)
	if err != nil {
		return fmt.Errorf("failed to marshal cluster state: %w", err)
	}

	return pb.storageService.PutObject(context.Background(), key, data)
}

// LoadClusterState loads complete cluster state
func (pb *ProviderBackend) LoadClusterState(clusterName string) (*K3sClusterState, error) {
	ctx := context.Background()
	
	// Try to load the new YAML format first
	configKey := pb.getKey(fmt.Sprintf("clusters/%s/config.yaml", clusterName))
	configData, err := pb.storageService.GetObject(ctx, configKey)
	if err != nil {
		return nil, fmt.Errorf("cluster %s config not found: %w", clusterName, err)
	}
	
	// Parse config as YAML
	var config ClusterConfig
	if err := yaml.Unmarshal(configData, &config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}
	
	// Load status file
	var status *ClusterStatus
	statusKey := pb.getKey(fmt.Sprintf("clusters/%s/status.yaml", clusterName))
	statusData, err := pb.storageService.GetObject(ctx, statusKey)
	if err == nil {
		status = &ClusterStatus{}
		if err := yaml.Unmarshal(statusData, status); err != nil {
			status = nil
		}
	}
	
	// Convert to K3sCluster
	cluster := ConvertFromClusterConfig(&config, status)
	
	// Build K3sClusterState
	state := &K3sClusterState{
		Cluster:     cluster,
		InstanceIDs: make(map[string]string),
		VolumeIDs:   make(map[string][]string),
		Metadata:    make(map[string]interface{}),
	}
	
	// Add status data if available
	if status != nil {
		if status.InstanceIDs != nil {
			state.InstanceIDs = status.InstanceIDs
		}
		if status.VolumeIDs != nil {
			state.VolumeIDs = status.VolumeIDs
		}
		state.SecurityGroups = status.SecurityGroups
		state.VPCID = status.VPCID
		state.SubnetIDs = status.SubnetIDs
		if status.Metadata != nil {
			state.Metadata = status.Metadata
		}
	}
	
	return state, nil
}

// LoadAllClusterStates loads all cluster states
func (pb *ProviderBackend) LoadAllClusterStates() ([]*K3sClusterState, error) {
	// List all cluster files
	keys, err := pb.storageService.ListObjects(context.Background(), pb.getKey("clusters/"))
	if err != nil {
		return nil, err
	}

	var states []*K3sClusterState
	processedClusters := make(map[string]bool)
	
	for _, key := range keys {
		// Remove prefix if present
		cleanKey := strings.TrimPrefix(key, pb.prefix+"/")
		
		// Only process config.yaml files
		if !strings.HasSuffix(cleanKey, "/config.yaml") {
			continue
		}
		
		// Extract cluster name from key: clusters/{name}/config.yaml
		parts := strings.Split(cleanKey, "/")
		if len(parts) < 3 {
			continue
		}
		
		clusterName := parts[1]
		
		// Skip if already processed
		if processedClusters[clusterName] {
			continue
		}
		
		// Load the cluster state
		state, err := pb.LoadClusterState(clusterName)
		if err != nil {
			continue // Skip clusters that can't be loaded
		}
		
		states = append(states, state)
		processedClusters[clusterName] = true
	}

	return states, nil
}

// DeleteClusterState deletes cluster state
func (pb *ProviderBackend) DeleteClusterState(clusterName string) error {
	ctx := context.Background()

	// Delete config and status files
	configKey := pb.getKey(fmt.Sprintf("clusters/%s/config.yaml", clusterName))
	pb.storageService.DeleteObject(ctx, configKey)
	
	statusKey := pb.getKey(fmt.Sprintf("clusters/%s/status.yaml", clusterName))
	pb.storageService.DeleteObject(ctx, statusKey)

	return nil
}

// SaveJob saves a job
func (pb *ProviderBackend) SaveJob(job *models.Job) error {
	if job == nil || job.ID == "" {
		return fmt.Errorf("invalid job")
	}

	data, err := json.MarshalIndent(job, "", "  ")
	if err != nil {
		return err
	}

	key := pb.getKey(fmt.Sprintf("jobs/%s.json", job.ID))
	return pb.storageService.PutObject(context.Background(), key, data)
}

// LoadJob loads a job
func (pb *ProviderBackend) LoadJob(jobID string) (*models.Job, error) {
	key := pb.getKey(fmt.Sprintf("jobs/%s.json", jobID))
	data, err := pb.storageService.GetObject(context.Background(), key)
	if err != nil {
		return nil, err
	}

	var job models.Job
	if err := json.Unmarshal(data, &job); err != nil {
		return nil, err
	}

	return &job, nil
}

// LoadAllJobs loads all jobs
func (pb *ProviderBackend) LoadAllJobs() ([]*models.Job, error) {
	keys, err := pb.storageService.ListObjects(context.Background(), pb.getKey("jobs/"))
	if err != nil {
		return nil, err
	}

	var jobs []*models.Job
	for _, key := range keys {
		if strings.HasSuffix(key, ".json") {
			data, err := pb.storageService.GetObject(context.Background(), key)
			if err != nil {
				continue
			}

			var job models.Job
			if err := json.Unmarshal(data, &job); err != nil {
				continue
			}
			jobs = append(jobs, &job)
		}
	}

	return jobs, nil
}

// DeleteJob deletes a job
func (pb *ProviderBackend) DeleteJob(jobID string) error {
	key := pb.getKey(fmt.Sprintf("jobs/%s.json", jobID))
	return pb.storageService.DeleteObject(context.Background(), key)
}

// SaveConfig saves application configuration
func (pb *ProviderBackend) SaveConfig(config map[string]interface{}) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}

	key := pb.getKey("config.json")
	return pb.storageService.PutObject(context.Background(), key, data)
}

// LoadConfig loads application configuration
func (pb *ProviderBackend) LoadConfig() (map[string]interface{}, error) {
	key := pb.getKey("config.json")
	data, err := pb.storageService.GetObject(context.Background(), key)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			// Return default config
			return map[string]interface{}{
				"default_provider": "aws",
				"theme":            "dark",
			}, nil
		}
		return nil, err
	}

	var config map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	return config, nil
}