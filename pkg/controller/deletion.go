package controller

import (
	"context"
	"fmt"
	"log"

	"github.com/madhouselabs/goman/pkg/models"
	"github.com/madhouselabs/goman/pkg/provider"
)

// reconcileDeleting handles cluster deletion
func (r *Reconciler) reconcileDeleting(ctx context.Context, resource *models.ClusterResource) (*models.ReconcileResult, error) {
	log.Printf("[DELETE] Starting deletion process for cluster %s", resource.Name)

	computeService := r.provider.GetComputeService()

	// First, check if any instances still exist in the cloud provider
	filters := map[string]string{
		"tag:ClusterName":     resource.Name,
		"instance-state-name": "pending,running,stopping,stopped", // Don't include "terminated"
	}

	// Add region filter if the cluster has a region specified
	if resource.Spec.Region != "" {
		filters["region"] = resource.Spec.Region
		log.Printf("[DELETE] Checking for instances to delete in region: %s", resource.Spec.Region)
	}

	actualInstances, err := computeService.ListInstances(ctx, filters)
	if err != nil {
		log.Printf("[DELETE] Warning: failed to list instances for deletion check: %v", err)
		// Continue with deletion anyway
	}

	log.Printf("[DELETE] Found %d active instances for cluster %s", len(actualInstances), resource.Name)

	// Check if all instances are terminated
	if len(actualInstances) == 0 {
		return r.finalizeClusterDeletion(ctx, resource)
	}

	// Some instances still exist, initiate deletion for those not already terminating
	return r.deleteInstances(ctx, resource, actualInstances)
}

// deleteInstances requests deletion for all instances
func (r *Reconciler) deleteInstances(ctx context.Context, resource *models.ClusterResource, instances []*provider.Instance) (*models.ReconcileResult, error) {
	log.Printf("[DELETE] Found %d instances still active for cluster %s", len(instances), resource.Name)

	// Mark instances as terminating in our state
	for i, inst := range resource.Status.Instances {
		if inst.State != "terminating" && inst.State != "terminated" {
			resource.Status.Instances[i].State = "terminating"
			log.Printf("[DELETE] Marking instance %s as terminating in state", inst.InstanceID)
		}
	}

	// Save state to reflect terminating status
	if err := r.saveClusterResource(ctx, resource); err != nil {
		log.Printf("[DELETE] Warning: failed to save state during deletion: %v", err)
	}

	computeService := r.provider.GetComputeService()
	instancesDeleted := 0
	
	// Request deletion for all instances (non-blocking)
	for _, inst := range instances {
		// Skip if already terminating
		if inst.State == "shutting-down" || inst.State == "terminating" || inst.State == "terminated" {
			log.Printf("[DELETE] Instance %s is already %s, skipping deletion request", inst.ID, inst.State)
			continue
		}

		log.Printf("[DELETE] Requesting termination for instance %s", inst.ID)
		
		// Use a short timeout for the API call itself (not waiting for deletion to complete)
		// This is just to send the termination request to AWS
		deleteCtx, deleteCancel := context.WithTimeout(ctx, DeleteInstanceTimeout)
		err := computeService.DeleteInstance(deleteCtx, inst.ID)
		deleteCancel()
		
		if err != nil {
			log.Printf("[DELETE] Failed to request termination for instance %s: %v", inst.ID, err)
			// Continue with other instances even if one fails
		} else {
			log.Printf("[DELETE] Successfully requested termination for instance %s", inst.ID)
			instancesDeleted++
		}
	}

	if instancesDeleted > 0 {
		log.Printf("[DELETE] Requested termination for %d instances of cluster %s", instancesDeleted, resource.Name)
	} else if len(instances) > 0 {
		log.Printf("[DELETE] All %d instances are already terminating for cluster %s", len(instances), resource.Name)
	}

	// Always requeue to check deletion status
	log.Printf("[DELETE] Requeueing to check deletion status in %v", DeletingRecheckInterval)
	return &models.ReconcileResult{
		Requeue:      true,
		RequeueAfter: DeletingRecheckInterval,
	}, nil
}

// finalizeClusterDeletion removes all cluster resources after instances are terminated
func (r *Reconciler) finalizeClusterDeletion(ctx context.Context, resource *models.ClusterResource) (*models.ReconcileResult, error) {
	log.Printf("[DELETE] All instances for cluster %s are terminated, cleaning up resources", resource.Name)

	// Clean up remaining cloud resources (security groups, etc.)
	if cleaner, ok := r.provider.(ClusterCleaner); ok {
		log.Printf("[DELETE] Cleaning up cloud resources for cluster %s", resource.Name)
		if err := cleaner.CleanupClusterResources(ctx, resource.Name); err != nil {
			log.Printf("[DELETE] Warning: failed to cleanup cluster resources: %v", err)
			// Continue with deletion even if cleanup fails
		}
	}

	// Delete the resource from storage (both config and status files)
	configKey := fmt.Sprintf("clusters/%s/config.yaml", resource.Name)
	statusKey := fmt.Sprintf("clusters/%s/status.yaml", resource.Name)
	
	// Delete config file
	log.Printf("[DELETE] Deleting config file: %s", configKey)
	if err := r.provider.GetStorageService().DeleteObject(ctx, configKey); err != nil {
		log.Printf("[DELETE] Warning: failed to delete config file: %v", err)
	}
	
	// Delete status file
	log.Printf("[DELETE] Deleting status file: %s", statusKey)
	if err := r.provider.GetStorageService().DeleteObject(ctx, statusKey); err != nil {
		log.Printf("[DELETE] Warning: failed to delete status file: %v", err)
	}
	
	log.Printf("[DELETE] Cluster %s deleted successfully", resource.Name)
	return &models.ReconcileResult{}, nil // No requeue, deletion complete
}