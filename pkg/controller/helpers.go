package controller

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/madhouselabs/goman/pkg/models"
	"github.com/madhouselabs/goman/pkg/storage"
	"gopkg.in/yaml.v3"
)

// acquireLock acquires a distributed lock for a resource
func (r *Reconciler) acquireLock(ctx context.Context, resourceID string) (string, error) {
	lockCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	return r.provider.GetLockService().AcquireLock(lockCtx, resourceID, r.owner, 15*time.Minute)
}

// releaseLock releases a distributed lock
func (r *Reconciler) releaseLock(ctx context.Context, resourceID, token string) {
	lockCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := r.provider.GetLockService().ReleaseLock(lockCtx, resourceID, token)
	if err != nil {
		log.Printf("[LOCK] Failed to release lock %s: %v", resourceID, err)
	}
}

// loadCluster loads cluster configuration and status from S3
func (r *Reconciler) loadCluster(ctx context.Context, clusterName string) (*models.ClusterResource, error) {
	configKey := fmt.Sprintf("clusters/%s/config.yaml", clusterName)
	statusKey := fmt.Sprintf("clusters/%s/status.yaml", clusterName)

	// Load config
	configData, err := r.provider.GetStorageService().GetObject(ctx, configKey)
	if err != nil {
		return nil, fmt.Errorf("failed to load cluster config: %w", err)
	}

	// First parse as storage.ClusterConfig which matches the YAML structure
	var config storage.ClusterConfig
	err = yaml.Unmarshal(configData, &config)
	if err != nil {
		return nil, fmt.Errorf("failed to parse cluster config: %w", err)
	}
	
	// Debug: Log the NodePools
	log.Printf("[DEBUG] Loaded config for %s: NodePools count = %d", clusterName, len(config.Spec.NodePools))
	for i, np := range config.Spec.NodePools {
		log.Printf("[DEBUG]   NodePool %d: name=%s, count=%d, type=%s", i, np.Name, np.Count, np.InstanceType)
	}

	// Convert to models.ClusterResource
	cluster := &models.ClusterResource{
		Name:              clusterName,
		Namespace:         "", // Will be set from provider
		DeletionTimestamp: config.Metadata.DeletionTimestamp,
		Spec: models.ClusterSpec{
			Provider:     "aws",
			Region:       config.Spec.Region,
			InstanceType: config.Spec.InstanceType,
			Mode:         string(config.Spec.Mode),
			K3sVersion:   config.Spec.K3sVersion,
			DesiredState: config.Spec.DesiredState,
		},
		Labels:      config.Metadata.Labels,
		Annotations: config.Metadata.Annotations,
	}
	
	// Convert NodePools if present
	if len(config.Spec.NodePools) > 0 {
		cluster.Spec.NodePools = make([]models.NodePool, len(config.Spec.NodePools))
		for i, np := range config.Spec.NodePools {
			cluster.Spec.NodePools[i] = models.NodePool{
				Name:         np.Name,
				Count:        np.Count,
				InstanceType: np.InstanceType,
				Labels:       np.Labels,
			}
			// Convert taints if present
			if len(np.Taints) > 0 {
				cluster.Spec.NodePools[i].Taints = make([]models.Taint, len(np.Taints))
				for j, t := range np.Taints {
					cluster.Spec.NodePools[i].Taints[j] = models.Taint{
						Key:    t.Key,
						Value:  t.Value,
						Effect: t.Effect,
					}
				}
			}
		}
	}
	
	// Set master count based on mode
	if config.Spec.Mode == "ha" {
		cluster.Spec.MasterCount = 3
	} else {
		cluster.Spec.MasterCount = 1
	}

	// Load status if exists
	statusData, err := r.provider.GetStorageService().GetObject(ctx, statusKey)
	if err == nil {
		log.Printf("[DEBUG] Status YAML for %s:\n%s", clusterName, string(statusData))
		var status models.ClusterResourceStatus
		err = yaml.Unmarshal(statusData, &status)
		if err == nil {
			cluster.Status = status
			log.Printf("[DEBUG] Loaded status: phase=%s, instances=%d", status.Phase, len(status.Instances))
			for i, inst := range status.Instances {
				log.Printf("[DEBUG]   Instance %d: ID=%s, IP=%s", i, inst.InstanceID, inst.PrivateIP)
			}
		} else {
			log.Printf("[DEBUG] Failed to unmarshal status: %v", err)
		}
	} else {
		log.Printf("[DEBUG] No status found: %v", err)
	}

	// Initialize status if empty
	if cluster.Status.Phase == "" {
		cluster.Status.Phase = string(models.ClusterPhasePending)
		cluster.Status.Message = "Starting cluster creation"
		now := time.Now()
		cluster.Status.LastReconcileTime = &now
	}

	return cluster, nil
}

// saveCluster saves cluster status to S3
func (r *Reconciler) saveCluster(ctx context.Context, cluster *models.ClusterResource) error {
	statusKey := fmt.Sprintf("clusters/%s/status.yaml", cluster.Name)
	
	now := time.Now()
	cluster.Status.LastReconcileTime = &now

	statusData, err := yaml.Marshal(cluster.Status)
	if err != nil {
		return fmt.Errorf("failed to marshal cluster status: %w", err)
	}

	err = r.provider.GetStorageService().PutObject(ctx, statusKey, statusData)
	if err != nil {
		return fmt.Errorf("failed to save cluster status: %w", err)
	}

	return nil
}

// TODO: Add step-by-step functions here as we build them