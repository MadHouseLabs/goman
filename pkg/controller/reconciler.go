package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/madhouselabs/goman/pkg/config"
	"github.com/madhouselabs/goman/pkg/models"
	"github.com/madhouselabs/goman/pkg/provider"
)

// ClusterCleaner interface for providers that can clean up cluster resources
type ClusterCleaner interface {
	CleanupClusterResources(ctx context.Context, clusterName string) error
}

// Reconciler handles cluster reconciliation with distributed locking
type Reconciler struct {
	provider provider.Provider
	owner    string // Unique identifier for this reconciler instance
}

// NewReconciler creates a new reconciler
func NewReconciler(prov provider.Provider, owner string) (*Reconciler, error) {
	if prov == nil {
		return nil, fmt.Errorf("provider is required")
	}

	if owner == "" {
		// Generate unique owner ID
		owner = fmt.Sprintf("reconciler-%s-%d", prov.Region(), time.Now().UnixNano())
	}

	return &Reconciler{
		provider: prov,
		owner:    owner,
	}, nil
}

// ReconcileCluster reconciles a cluster with distributed locking
func (r *Reconciler) ReconcileCluster(ctx context.Context, clusterName string) (*models.ReconcileResult, error) {
	// Create overall timeout context for the entire reconciliation (10 minutes)
	reconcileCtx, reconcileCancel := context.WithTimeout(ctx, 10*time.Minute)
	defer reconcileCancel()

	resourceID := fmt.Sprintf("cluster-%s", clusterName)

	// Try to acquire lock with 5 minute TTL
	lockCtx, lockCancel := context.WithTimeout(reconcileCtx, 30*time.Second)
	defer lockCancel()

	lockToken, err := r.provider.GetLockService().AcquireLock(lockCtx, resourceID, r.owner, 5*time.Minute)
	if err != nil {
		// Another controller is working on this cluster
		log.Printf("Failed to acquire lock for %s: %v", resourceID, err)

		// Check if cluster is locked
		locked, owner, _ := r.provider.GetLockService().IsLocked(ctx, resourceID)
		if locked {
			log.Printf("Cluster %s is being reconciled by %s", clusterName, owner)
			// Requeue for later
			return &models.ReconcileResult{
				Requeue:      true,
				RequeueAfter: 30 * time.Second,
			}, nil
		}

		return nil, fmt.Errorf("failed to acquire lock: %w", err)
	}

	// Ensure we release the lock when done
	defer func() {
		// Use a fresh context for lock release to ensure it happens even if main context is cancelled
		releaseCtx, releaseCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer releaseCancel()

		if err := r.provider.GetLockService().ReleaseLock(releaseCtx, resourceID, lockToken); err != nil {
			log.Printf("Failed to release lock for %s: %v", resourceID, err)
		}
	}()

	log.Printf("Acquired lock for cluster %s", clusterName)

	// Load cluster resource from storage
	resource, err := r.loadClusterResource(reconcileCtx, clusterName)
	if err != nil {
		return nil, fmt.Errorf("failed to load cluster resource: %w", err)
	}

	// For long-running operations, periodically renew the lock
	renewCtx, renewCancel := context.WithCancel(reconcileCtx)
	defer renewCancel() // Cancel the renewal goroutine when done

	renewTicker := time.NewTicker(2 * time.Minute)
	defer renewTicker.Stop()

	// Start lock renewal in background
	renewDone := make(chan struct{})
	go func() {
		defer close(renewDone)
		for {
			select {
			case <-renewCtx.Done():
				return
			case <-renewTicker.C:
				// Use a timeout context for lock renewal
				renewLockCtx, renewLockCancel := context.WithTimeout(reconcileCtx, 10*time.Second)
				if err := r.provider.GetLockService().RenewLock(renewLockCtx, resourceID, lockToken, 5*time.Minute); err != nil {
					log.Printf("Failed to renew lock for %s: %v", resourceID, err)
					renewLockCancel()
					return
				}
				renewLockCancel()
				log.Printf("Renewed lock for cluster %s", clusterName)
			}
		}
	}()

	// Perform reconciliation based on phase
	result, err := r.reconcileBasedOnPhase(reconcileCtx, resource)
	if err != nil {
		// Send error notification
		r.sendNotification(reconcileCtx, "error", fmt.Sprintf("Reconciliation failed for cluster %s: %v", clusterName, err))
		return nil, err
	}

	// Check if cluster was deleted (reconcileDeleting returns a special result)
	// If deletion is complete, the files have already been removed by reconcileDeleting
	// so we should NOT save the resource back
	if resource.Status.Phase == models.ClusterPhaseDeleting && result != nil && !result.Requeue {
		// Deletion completed successfully, don't save the resource back
		log.Printf("Cluster %s deletion completed, not saving resource", clusterName)
		return result, nil
	}

	// Save updated resource for all other phases
	if err := r.saveClusterResource(reconcileCtx, resource); err != nil {
		return nil, fmt.Errorf("failed to save cluster resource: %w", err)
	}

	// Send success notification if cluster became ready
	if resource.Status.Phase == models.ClusterPhaseRunning {
		r.sendNotification(reconcileCtx, "success", fmt.Sprintf("Cluster %s is now running", clusterName))
	}

	return result, nil
}

// reconcileBasedOnPhase performs reconciliation based on current phase
func (r *Reconciler) reconcileBasedOnPhase(ctx context.Context, resource *models.ClusterResource) (*models.ReconcileResult, error) {
	switch resource.Status.Phase {
	case "", models.ClusterPhasePending:
		return r.reconcilePending(ctx, resource)
	case models.ClusterPhaseProvisioning:
		return r.reconcileProvisioning(ctx, resource)
	case models.ClusterPhaseRunning:
		return r.reconcileRunning(ctx, resource)
	case models.ClusterPhaseFailed:
		return r.reconcileFailed(ctx, resource)
	case models.ClusterPhaseDeleting:
		return r.reconcileDeleting(ctx, resource)
	default:
		return &models.ReconcileResult{}, nil
	}
}

// reconcilePending starts provisioning
func (r *Reconciler) reconcilePending(ctx context.Context, resource *models.ClusterResource) (*models.ReconcileResult, error) {
	log.Printf("Starting provisioning for cluster %s", resource.Name)

	resource.Status.Phase = models.ClusterPhaseProvisioning
	resource.Status.Message = "Starting infrastructure provisioning"

	return &models.ReconcileResult{
		Requeue:      true,
		RequeueAfter: 5 * time.Second,
	}, nil
}

// reconcileProvisioning creates infrastructure
func (r *Reconciler) reconcileProvisioning(ctx context.Context, resource *models.ClusterResource) (*models.ReconcileResult, error) {
	log.Printf("Provisioning infrastructure for cluster %s", resource.Name)

	// Get provider configuration
	providerConfig := config.GetProviderConfig()

	// Use compute service to create instances
	computeService := r.provider.GetComputeService()

	// First, check for existing instances for this cluster
	existingInstances, err := computeService.ListInstances(ctx, map[string]string{
		"tag:Cluster":         resource.Name,
		"instance-state-name": "pending,running,stopping,stopped",
	})
	if err != nil {
		log.Printf("Warning: failed to list existing instances: %v", err)
		existingInstances = nil
	}

	// Update Status.Instances with current state from cloud provider
	if len(existingInstances) > 0 {
		log.Printf("Found %d existing instances for cluster %s", len(existingInstances), resource.Name)
		
		// Build a map of existing instances for quick lookup
		instanceMap := make(map[string]*provider.Instance)
		for _, inst := range existingInstances {
			instanceMap[inst.Name] = inst
		}
		
		// Update existing entries in Status.Instances
		for i := range resource.Status.Instances {
			if cloudInst, ok := instanceMap[resource.Status.Instances[i].Name]; ok {
				// Update with current state from cloud
				resource.Status.Instances[i].InstanceID = cloudInst.ID
				resource.Status.Instances[i].State = cloudInst.State
				resource.Status.Instances[i].PrivateIP = cloudInst.PrivateIP
				resource.Status.Instances[i].PublicIP = cloudInst.PublicIP
				if cloudInst.LaunchTime.After(resource.Status.Instances[i].LaunchTime) {
					resource.Status.Instances[i].LaunchTime = cloudInst.LaunchTime
				}
				delete(instanceMap, resource.Status.Instances[i].Name)
			}
		}
		
		// Add any new instances not in Status.Instances
		for _, inst := range instanceMap {
			resource.Status.Instances = append(resource.Status.Instances, models.InstanceStatus{
				InstanceID: inst.ID,
				Name:       inst.Name,
				State:      inst.State,
				PrivateIP:  inst.PrivateIP,
				PublicIP:   inst.PublicIP,
				LaunchTime: inst.LaunchTime,
			})
		}
	}

	// Ensure we have the right number of instances based on mode
	// Get master count from the spec (mode-based)
	masterCount := resource.Spec.MasterCount
	if masterCount == 0 {
		// Default based on mode if not specified
		masterCount = 1 // Default to developer mode
	}

	// Check each expected node individually to handle placeholders correctly
	nodesToCreate := []string{}
	for i := 0; i < masterCount; i++ {
		nodeName := fmt.Sprintf("%s-master-%d", resource.Name, i)

		// Check if this node already exists in our state (including placeholders)
		nodeInState := false
		for _, inst := range resource.Status.Instances {
			if inst.Name == nodeName {
				nodeInState = true
				// If it's a placeholder without instance ID, check if it now exists in the cloud provider
				if inst.InstanceID == "" || inst.State == "initiating" {
					for _, awsInst := range existingInstances {
						if awsInst.Name == nodeName {
							log.Printf("Updating placeholder for %s with real instance ID %s", nodeName, awsInst.ID)
							// Update the placeholder with real data
							for j, stateInst := range resource.Status.Instances {
								if stateInst.Name == nodeName {
									resource.Status.Instances[j].InstanceID = awsInst.ID
									resource.Status.Instances[j].State = awsInst.State
									resource.Status.Instances[j].PrivateIP = awsInst.PrivateIP
									resource.Status.Instances[j].PublicIP = awsInst.PublicIP
									break
								}
							}
							break
						}
					}
				}
				break
			}
		}

		if !nodeInState {
			// Check if this node exists in the cloud provider
			nodeInCloud := false
			for _, awsInst := range existingInstances {
				if awsInst.Name == nodeName {
					log.Printf("Instance %s already exists in cloud provider, adding to state", nodeName)
					resource.Status.Instances = append(resource.Status.Instances, models.InstanceStatus{
						InstanceID: awsInst.ID,
						Name:       awsInst.Name,
						State:      awsInst.State,
						PrivateIP:  awsInst.PrivateIP,
						PublicIP:   awsInst.PublicIP,
						LaunchTime: awsInst.LaunchTime,
					})
					nodeInCloud = true
					break
				}
			}

			if !nodeInCloud {
				// Need to create this node - add placeholder first
				log.Printf("Adding placeholder for instance %s", nodeName)
				resource.Status.Instances = append(resource.Status.Instances, models.InstanceStatus{
					InstanceID: "", // Will be filled when creation completes
					Name:       nodeName,
					State:      "initiating",
					PrivateIP:  "",
					PublicIP:   "",
					LaunchTime: time.Now(),
				})
				nodesToCreate = append(nodesToCreate, nodeName)
			}
		}
	}

	if len(nodesToCreate) > 0 {

		// Save state with placeholders to prevent duplicate creation
		if len(nodesToCreate) > 0 {
			log.Printf("Saving state with %d placeholder nodes", len(nodesToCreate))
			if err := r.saveClusterResource(ctx, resource); err != nil {
				log.Printf("Failed to save state with placeholders: %v", err)
				return nil, err
			}

			// Now create instances synchronously (no goroutines needed)
			// Cloud provider typically returns immediately with instance in "pending" state
			log.Printf("Creating %d instances", len(nodesToCreate))
			instancesCreated := false

			for _, nodeName := range nodesToCreate {
				log.Printf("Creating instance %s", nodeName)

				instanceConfig := provider.InstanceConfig{
					Name:         nodeName,
					Region:       resource.Spec.Region, // Honor the region from cluster spec
					InstanceType: resource.Spec.InstanceType,
					ImageID:      providerConfig.GetProviderImageID(r.provider.Name()),
					// No KeyName needed - using Systems Manager or equivalent
					Tags: map[string]string{
						"Cluster":   resource.Name,
						"ManagedBy": "goman",
						"Provider":  r.provider.Name(),
					},
				}

				// Use normal context with reasonable timeout (30s is enough for RunInstances)
				instanceCtx, instanceCancel := context.WithTimeout(ctx, 30*time.Second)

				instance, err := computeService.CreateInstance(instanceCtx, instanceConfig)
				instanceCancel()

				if err != nil {
					log.Printf("Failed to create instance %s: %v", nodeName, err)
					// Continue trying other instances
				} else {
					log.Printf("Successfully initiated creation of instance %s (ID: %s)", nodeName, instance.ID)

					// Update the placeholder with the real instance ID
					for i, inst := range resource.Status.Instances {
						if inst.Name == nodeName {
							resource.Status.Instances[i].InstanceID = instance.ID
							resource.Status.Instances[i].State = provider.InstanceStatePending
							break
						}
					}
					instancesCreated = true
				}
			}

			// Save the updated state with real instance IDs
			if instancesCreated {
				log.Printf("Saving state with real instance IDs")
				if err := r.saveClusterResource(ctx, resource); err != nil {
					log.Printf("Warning: failed to save state with instance IDs: %v", err)
				}
			}
		}

		// Always requeue to check instance status
		return &models.ReconcileResult{
			Requeue:      true,
			RequeueAfter: 15 * time.Second, // Check frequently during provisioning
		}, nil
	}

	// Check if all instances are running
	allRunning := true
	for _, inst := range resource.Status.Instances {
		if inst.State != "running" {
			allRunning = false
			break
		}
	}

	if !allRunning {
		// Wait for instances to be ready
		return &models.ReconcileResult{
			Requeue:      true,
			RequeueAfter: 10 * time.Second,
		}, nil
	}

	// All instances ready, move to running
	resource.Status.Phase = models.ClusterPhaseRunning
	resource.Status.Message = "Cluster is running"
	resource.Status.ObservedGeneration = resource.Generation
	now := time.Now()
	resource.Status.LastReconcileTime = &now

	return &models.ReconcileResult{}, nil
}

// reconcileRunning checks health and handles updates
func (r *Reconciler) reconcileRunning(ctx context.Context, resource *models.ClusterResource) (*models.ReconcileResult, error) {
	log.Printf("Checking running cluster %s", resource.Name)

	// Check if spec changed
	if resource.Generation != resource.Status.ObservedGeneration {
		// Handle updates (e.g., need more/fewer instances)
		currentCount := len(resource.Status.Instances)
		if currentCount != resource.Spec.MasterCount {
			resource.Status.Phase = models.ClusterPhaseProvisioning
			resource.Status.Message = "Updating cluster configuration"
			return &models.ReconcileResult{
				Requeue:      true,
				RequeueAfter: 5 * time.Second,
			}, nil
		}
		// Update observed generation since we're in sync
		resource.Status.ObservedGeneration = resource.Generation
	}

	// Update last reconcile time
	now := time.Now()
	resource.Status.LastReconcileTime = &now

	// Periodic health check
	return &models.ReconcileResult{
		Requeue:      true,
		RequeueAfter: 60 * time.Second,
	}, nil
}

// reconcileFailed handles failed clusters
func (r *Reconciler) reconcileFailed(ctx context.Context, resource *models.ClusterResource) (*models.ReconcileResult, error) {
	log.Printf("Cluster %s is in failed state: %s", resource.Name, resource.Status.Message)
	// Don't requeue failed clusters automatically
	return &models.ReconcileResult{}, nil
}

// reconcileDeleting handles cluster deletion
func (r *Reconciler) reconcileDeleting(ctx context.Context, resource *models.ClusterResource) (*models.ReconcileResult, error) {
	log.Printf("Deleting cluster %s", resource.Name)

	computeService := r.provider.GetComputeService()

	// First, check if any instances still exist in the cloud provider
	// Include the region in the filters if available
	filters := map[string]string{
		"tag:Cluster":         resource.Name,
		"instance-state-name": "pending,running,stopping,stopped", // Don't include "terminated"
	}

	// Add region filter if the cluster has a region specified
	if resource.Spec.Region != "" {
		filters["region"] = resource.Spec.Region
		log.Printf("Checking for instances in region: %s", resource.Spec.Region)
	}

	actualInstances, err := computeService.ListInstances(ctx, filters)
	if err != nil {
		log.Printf("Warning: failed to list instances for deletion check: %v", err)
		// Continue with deletion anyway
	}

	// Check if all instances are terminated
	if len(actualInstances) == 0 {
		log.Printf("All instances for cluster %s are terminated, cleaning up resources", resource.Name)

		// All instances are gone, clean up remaining resources
		if cleaner, ok := r.provider.(ClusterCleaner); ok {
			if err := cleaner.CleanupClusterResources(ctx, resource.Name); err != nil {
				log.Printf("Warning: failed to cleanup cluster resources: %v", err)
			}
		}

		// Delete the resource from storage (both config and status files)
		configKey := fmt.Sprintf("clusters/%s/config.json", resource.Name)
		statusKey := fmt.Sprintf("clusters/%s/status.json", resource.Name)
		
		// Delete config file
		if err := r.provider.GetStorageService().DeleteObject(ctx, configKey); err != nil {
			log.Printf("Failed to delete config file: %v", err)
		}
		
		// Delete status file
		if err := r.provider.GetStorageService().DeleteObject(ctx, statusKey); err != nil {
			log.Printf("Failed to delete status file: %v", err)
		}
		
		log.Printf("Deleted cluster files for %s", resource.Name)

		log.Printf("Cluster %s deleted successfully", resource.Name)
		return &models.ReconcileResult{}, nil // No requeue, deletion complete
	}

	// Some instances still exist, initiate deletion for those not already terminating
	log.Printf("Found %d instances still running for cluster %s", len(actualInstances), resource.Name)

	// Mark instances as terminating in our state
	for i, inst := range resource.Status.Instances {
		if inst.State != "terminating" && inst.State != "terminated" {
			resource.Status.Instances[i].State = "terminating"
		}
	}

	// Save state to reflect terminating status
	if err := r.saveClusterResource(ctx, resource); err != nil {
		log.Printf("Warning: failed to save state during deletion: %v", err)
	}

	// Fire off deletion requests for all instances (don't wait)
	instancesDeleted := 0
	for _, inst := range actualInstances {
		// Skip if already terminating
		if inst.State == "shutting-down" || inst.State == "terminating" || inst.State == "terminated" {
			log.Printf("Instance %s is already %s, skipping deletion", inst.ID, inst.State)
			continue
		}

		log.Printf("Initiating deletion of instance %s", inst.ID)
		instancesDeleted++

		// Fire and forget - don't wait for completion
		go func(instanceID string) {
			// Use background context so deletion continues even after function returns
			deleteCtx, deleteCancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer deleteCancel()

			if err := computeService.DeleteInstance(deleteCtx, instanceID); err != nil {
				log.Printf("Failed to delete instance %s: %v", instanceID, err)
			} else {
				log.Printf("Successfully initiated deletion of instance %s", instanceID)
			}
		}(inst.ID)
	}

	if instancesDeleted > 0 {
		log.Printf("Initiated deletion of %d instances for cluster %s", instancesDeleted, resource.Name)
	}

	// Requeue to check deletion status
	return &models.ReconcileResult{
		Requeue:      true,
		RequeueAfter: 20 * time.Second, // Check every 20 seconds during deletion
	}, nil
}

// loadClusterResource loads cluster resource from storage with timeout
func (r *Reconciler) loadClusterResource(ctx context.Context, name string) (*models.ClusterResource, error) {
	// Create a timeout context for loading (30 seconds)
	loadCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Load from new format only: separate config and status files
	configKey := fmt.Sprintf("clusters/%s/config.json", name)
	statusKey := fmt.Sprintf("clusters/%s/status.json", name)

	// Load config (required)
	configData, err := r.provider.GetStorageService().GetObject(loadCtx, configKey)
	if err != nil {
		return nil, fmt.Errorf("cluster config %s not found: %w", name, err)
	}

	var config map[string]interface{}
	if err := json.Unmarshal(configData, &config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal cluster config: %w", err)
	}

	// Load status (optional - might not exist for new clusters)
	var status map[string]interface{}
	statusData, err := r.provider.GetStorageService().GetObject(loadCtx, statusKey)
	if err == nil {
		if err := json.Unmarshal(statusData, &status); err != nil {
			log.Printf("Warning: Failed to unmarshal status for %s: %v", name, err)
			status = make(map[string]interface{})
		}
	} else {
		// No status file yet, initialize empty
		status = make(map[string]interface{})
	}

	// Merge config and status to create full state
	clusterState := make(map[string]interface{})
	clusterState["cluster"] = config["cluster"]
	if statusCluster, ok := status["cluster"].(map[string]interface{}); ok {
		// Merge status into cluster
		if cluster, ok := clusterState["cluster"].(map[string]interface{}); ok {
			cluster["status"] = statusCluster["status"]
		}
	}
	clusterState["instance_ids"] = status["instance_ids"]
	clusterState["metadata"] = status["metadata"]

	return r.convertStateToResource(clusterState)
}

// convertStateToResource converts a state JSON to ClusterResource
func (r *Reconciler) convertStateToResource(state map[string]interface{}) (*models.ClusterResource, error) {
	cluster, ok := state["cluster"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid state format: missing cluster")
	}

	resource := &models.ClusterResource{
		Name:      getStringFromMap(cluster, "name"),
		Namespace: "default",
		ClusterID: getStringFromMap(cluster, "id"),
		Spec: models.ClusterSpec{
			MasterCount:  1,           // Default, will be updated from nodes or mode
			Mode:         "developer", // Default mode
			InstanceType: "t3.medium",
		},
		Status: models.ClusterResourceStatus{
			Phase:   models.ClusterPhasePending,
			Message: "Loaded from state",
		},
		Generation: 1,
	}

	// Extract mode from cluster
	if mode := getStringFromMap(cluster, "mode"); mode != "" {
		resource.Spec.Mode = mode
		// Set master count based on mode
		if mode == "ha" {
			resource.Spec.MasterCount = 3
		} else {
			resource.Spec.MasterCount = 1
		}
	}

	// Always check for region in cluster config first (it's a cluster-level property)
	if region := getStringFromMap(cluster, "region"); region != "" {
		resource.Spec.Region = region
		log.Printf("Setting region from cluster config: %s", region)
	}

	// Check for instance_type in cluster config
	if instanceType := getStringFromMap(cluster, "instance_type"); instanceType != "" {
		resource.Spec.InstanceType = instanceType
	}

	// Extract master nodes to get node count and possibly override instance type
	if masterNodes, ok := cluster["master_nodes"].([]interface{}); ok && len(masterNodes) > 0 {
		resource.Spec.MasterCount = len(masterNodes)
		if node, ok := masterNodes[0].(map[string]interface{}); ok {
			if instanceType := getStringFromMap(node, "instance_type"); instanceType != "" {
				resource.Spec.InstanceType = instanceType
			}
		}
	}

	// Set status based on cluster status
	if status := getStringFromMap(cluster, "status"); status != "" {
		switch status {
		case "creating":
			resource.Status.Phase = models.ClusterPhaseProvisioning
		case "running":
			resource.Status.Phase = models.ClusterPhaseRunning
		case "error":
			resource.Status.Phase = models.ClusterPhaseFailed
		case "deleting":
			resource.Status.Phase = models.ClusterPhaseDeleting
		default:
			resource.Status.Phase = models.ClusterPhasePending
		}
	}

	// Extract instance IDs from state
	if instanceIDs, ok := state["instance_ids"].(map[string]interface{}); ok {
		for nodeName, instanceID := range instanceIDs {
			if id, ok := instanceID.(string); ok && id != "" {
				resource.Status.Instances = append(resource.Status.Instances, models.InstanceStatus{
					InstanceID: id,
					Name:       nodeName,
					State:      "running", // Assume running if ID exists
				})
			}
		}
	}

	// Check for deletion timestamp in metadata
	if metadata, ok := state["metadata"].(map[string]interface{}); ok {
		if deletionRequested, ok := metadata["deletion_requested"]; ok {
			// Cluster is marked for deletion
			resource.Status.Phase = models.ClusterPhaseDeleting
			resource.Status.Message = fmt.Sprintf("Deletion requested at %v", deletionRequested)

			// Set DeletionTimestamp if available
			if deletionTime, ok := deletionRequested.(string); ok {
				if t, err := time.Parse(time.RFC3339, deletionTime); err == nil {
					resource.DeletionTimestamp = &t
				}
			}
		}
	}

	return resource, nil
}

// getStringFromMap safely gets a string from a map
func getStringFromMap(m map[string]interface{}, key string) string {
	if val, ok := m[key].(string); ok {
		return val
	}
	return ""
}

// mapPhaseToStatus maps ClusterPhase to status string
func mapPhaseToStatus(phase string) string {
	switch phase {
	case models.ClusterPhaseRunning:
		return "running"
	case models.ClusterPhaseProvisioning:
		return "creating"
	case models.ClusterPhaseFailed:
		return "error"
	case models.ClusterPhaseDeleting:
		return "deleting"
	default:
		return "pending"
	}
}

// saveClusterResource saves ONLY the status to storage (never modifies config)
func (r *Reconciler) saveClusterResource(ctx context.Context, resource *models.ClusterResource) error {
	// Create a timeout context for saving (30 seconds)
	saveCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// IMPORTANT: Only save status, never modify config
	statusKey := fmt.Sprintf("clusters/%s/status.json", resource.Name)

	// Create status structure
	statusState := make(map[string]interface{})

	// Status cluster info (reconciler-controlled fields only)
	statusState["cluster"] = map[string]interface{}{
		"status":     mapPhaseToStatus(resource.Status.Phase),
		"updated_at": time.Now().Format(time.RFC3339),
	}

	// Instance IDs and their states
	instanceIDs := make(map[string]string)
	instanceStates := make(map[string]interface{})
	for _, inst := range resource.Status.Instances {
		instanceIDs[inst.Name] = inst.InstanceID
		instanceStates[inst.Name] = map[string]interface{}{
			"id":         inst.InstanceID,
			"state":      inst.State,
			"private_ip": inst.PrivateIP,
			"public_ip":  inst.PublicIP,
			"role":       inst.Role,
		}
	}
	statusState["instance_ids"] = instanceIDs
	statusState["instances"] = instanceStates

	// Metadata
	statusState["metadata"] = map[string]interface{}{
		"last_reconciled":    time.Now().Format(time.RFC3339),
		"phase":              resource.Status.Phase,
		"message":            resource.Status.Message,
		"observed_generation": resource.Status.ObservedGeneration,
	}

	// Save status file
	statusData, err := json.MarshalIndent(statusState, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal status: %w", err)
	}

	if err := r.provider.GetStorageService().PutObject(saveCtx, statusKey, statusData); err != nil {
		return fmt.Errorf("failed to save cluster status: %w", err)
	}

	log.Printf("Saved cluster status for %s with phase %s", resource.Name, resource.Status.Phase)
	return nil
}


// sendNotification sends a notification
func (r *Reconciler) sendNotification(ctx context.Context, notificationType, message string) {
	topic := "goman-cluster-events"
	if notificationType == "error" {
		topic = "goman-error-events"
	}

	if err := r.provider.GetNotificationService().Publish(ctx, topic, message); err != nil {
		log.Printf("Failed to send notification: %v", err)
	}
}
