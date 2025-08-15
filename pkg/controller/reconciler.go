package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
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
	log.Printf("[RECONCILE] Starting reconciliation for cluster %s", clusterName)
	
	// Create overall timeout context for the entire reconciliation
	reconcileCtx, reconcileCancel := context.WithTimeout(ctx, ReconcileTimeout)
	defer reconcileCancel()

	resourceID := fmt.Sprintf("cluster-%s", clusterName)
	log.Printf("[LOCK] Attempting to acquire lock for %s", resourceID)

	// Try to acquire lock
	lockCtx, lockCancel := context.WithTimeout(reconcileCtx, LockAcquireTimeout)
	defer lockCancel()

	lockToken, err := r.provider.GetLockService().AcquireLock(lockCtx, resourceID, r.owner, LockTTL)
	if err != nil {
		// Another controller is working on this cluster
		log.Printf("[LOCK] Failed to acquire lock for %s: %v", resourceID, err)

		// Check if cluster is locked
		locked, owner, _ := r.provider.GetLockService().IsLocked(ctx, resourceID)
		if locked {
			log.Printf("[LOCK] Cluster %s is currently locked by %s", clusterName, owner)
			
			// Check if cluster still exists before requeueing
			log.Printf("[LOAD] Checking if cluster %s still exists before requeue", clusterName)
			configKey := fmt.Sprintf("clusters/%s/config.json", clusterName)
			_, configErr := r.provider.GetStorageService().GetObject(ctx, configKey)
			if configErr != nil {
				if strings.Contains(configErr.Error(), "not found") || strings.Contains(configErr.Error(), "NoSuchKey") {
					log.Printf("[LOAD] Cluster %s no longer exists (was locked by %s), skipping reconciliation", clusterName, owner)
					return &models.ReconcileResult{
						Requeue: false,
					}, nil
				}
			}
			
			// Cluster exists but is locked, requeue for later
			log.Printf("[REQUEUE] Cluster %s exists but is locked, requeueing in %v", clusterName, LockedClusterRetryInterval)
			return &models.ReconcileResult{
				Requeue:      true,
				RequeueAfter: LockedClusterRetryInterval,
			}, nil
		}

		return nil, fmt.Errorf("failed to acquire lock: %w", err)
	}

	// Ensure we release the lock when done
	defer func() {
		// Use a fresh context for lock release to ensure it happens even if main context is cancelled
		releaseCtx, releaseCancel := context.WithTimeout(context.Background(), LockReleaseTimeout)
		defer releaseCancel()

		if err := r.provider.GetLockService().ReleaseLock(releaseCtx, resourceID, lockToken); err != nil {
			log.Printf("[LOCK] Failed to release lock for %s: %v", resourceID, err)
		} else {
			log.Printf("[LOCK] Released lock for %s", resourceID)
		}
	}()

	log.Printf("[LOCK] Successfully acquired lock for cluster %s (token: %s)", clusterName, lockToken)

	// Load cluster resource from storage
	log.Printf("[LOAD] Loading cluster resource for %s", clusterName)
	resource, err := r.loadClusterResource(reconcileCtx, clusterName)
	if err != nil {
		// Check if the error indicates the cluster doesn't exist
		if strings.Contains(err.Error(), "not found") {
			log.Printf("[LOAD] Cluster %s no longer exists, skipping reconciliation", clusterName)
			// Return success with no requeue - cluster is gone
			return &models.ReconcileResult{
				Requeue: false,
			}, nil
		}
		return nil, fmt.Errorf("failed to load cluster resource: %w", err)
	}

	// For long-running operations, periodically renew the lock
	renewCtx, renewCancel := context.WithCancel(reconcileCtx)
	defer renewCancel() // Cancel the renewal goroutine when done

	renewTicker := time.NewTicker(LockRenewInterval)
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
				renewLockCtx, renewLockCancel := context.WithTimeout(reconcileCtx, LockRenewTimeout)
				if err := r.provider.GetLockService().RenewLock(renewLockCtx, resourceID, lockToken, LockTTL); err != nil {
					log.Printf("[LOCK] Failed to renew lock for %s: %v", resourceID, err)
					renewLockCancel()
					return
				}
				renewLockCancel()
				log.Printf("[LOCK] Renewed lock for cluster %s", clusterName)
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

// Phase reconciliation functions moved to phases.go and deletion.go

// reconcileProvisioning creates infrastructure
func (r *Reconciler) reconcileProvisioning(ctx context.Context, resource *models.ClusterResource) (*models.ReconcileResult, error) {
	log.Printf("[PROVISIONING] Starting infrastructure provisioning for cluster %s", resource.Name)
	
	// Get provider configuration
	providerConfig := config.GetProviderConfig()

	// Use compute service to create instances
	computeService := r.provider.GetComputeService()

	// First, check for existing instances for this cluster
	filters := map[string]string{
		"tag:ClusterName":     resource.Name,
		"instance-state-name": "pending,running,stopping,stopped",
	}
	
	// Add region filter if specified in the resource
	if resource.Spec.Region != "" {
		filters["region"] = resource.Spec.Region
		log.Printf("Querying instances for cluster %s in region %s", resource.Name, resource.Spec.Region)
	}
	
	existingInstances, err := computeService.ListInstances(ctx, filters)
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
			// Determine role from name
			role := "worker"
			if strings.Contains(inst.Name, "master") {
				role = "master"
			}
			resource.Status.Instances = append(resource.Status.Instances, models.InstanceStatus{
				InstanceID: inst.ID,
				Name:       inst.Name,
				Role:       role,
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
					// Determine role from name
					role := "worker"
					if strings.Contains(nodeName, "master") {
						role = "master"
					}
					resource.Status.Instances = append(resource.Status.Instances, models.InstanceStatus{
						InstanceID: awsInst.ID,
						Name:       awsInst.Name,
						Role:       role,
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
				// Determine role from name
				role := "worker"
				if strings.Contains(nodeName, "master") {
					role = "master"
				}
				resource.Status.Instances = append(resource.Status.Instances, models.InstanceStatus{
					InstanceID: "", // Will be filled when creation completes
					Name:       nodeName,
					Role:       role,
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
			var creationErrors []string

			for _, nodeName := range nodesToCreate {
				log.Printf("Creating instance %s in region %s", nodeName, resource.Spec.Region)

				instanceConfig := provider.InstanceConfig{
					Name:         nodeName,
					Region:       resource.Spec.Region, // Honor the region from cluster spec
					InstanceType: resource.Spec.InstanceType,
					ImageID:      providerConfig.GetProviderImageID(r.provider.Name()),
					// No KeyName needed - using Systems Manager or equivalent
					Tags: map[string]string{
						"ClusterName": resource.Name,
						"ManagedBy":   "goman",
						"Provider":    r.provider.Name(),
					},
				}

				// Use normal context with reasonable timeout (30s is enough for RunInstances)
				instanceCtx, instanceCancel := context.WithTimeout(ctx, 30*time.Second)

				instance, err := computeService.CreateInstance(instanceCtx, instanceConfig)
				instanceCancel()

				if err != nil {
					log.Printf("Failed to create instance %s in region %s: %v", nodeName, resource.Spec.Region, err)
					creationErrors = append(creationErrors, fmt.Sprintf("%s: %v", nodeName, err))
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

			// If ALL instance creations failed, move to failed state
			if !instancesCreated && len(creationErrors) > 0 {
				log.Printf("All instance creation attempts failed for cluster %s", resource.Name)
				resource.Status.Phase = models.ClusterPhaseFailed
				resource.Status.Message = fmt.Sprintf("Failed to create instances: %s", strings.Join(creationErrors, "; "))
				// Save the failed state
				if err := r.saveClusterResource(ctx, resource); err != nil {
					log.Printf("Warning: failed to save failed state: %v", err)
				}
				return &models.ReconcileResult{}, nil
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

	// Check instance states for errors or readiness
	allRunning := true
	hasErrors := false
	instanceStates := []string{}
	errorMessages := []string{}
	
	for _, inst := range resource.Status.Instances {
		// Check for error states that indicate actual failures
		if inst.State == "terminated" || inst.State == "stopped" || inst.State == "stopping" || inst.State == "shutting-down" {
			// These states indicate instance failure or unexpected termination
			hasErrors = true
			errorMessages = append(errorMessages, fmt.Sprintf("%s is in error state: %s", inst.Name, inst.State))
			log.Printf("[PROVISIONING] ERROR: Instance %s is in unexpected state: %s", inst.Name, inst.State)
		} else if inst.State != "running" {
			// Instance is still pending, this is normal
			allRunning = false
			instanceStates = append(instanceStates, fmt.Sprintf("%s is %s", inst.Name, inst.State))
			log.Printf("[PROVISIONING] Instance %s is %s (waiting for running state)", inst.Name, inst.State)
		} else {
			// Instance is running - IPs are optional, not required
			if inst.PrivateIP != "" {
				log.Printf("[PROVISIONING] Instance %s is running with IP %s", inst.Name, inst.PrivateIP)
			} else {
				log.Printf("[PROVISIONING] Instance %s is running (IP not yet assigned)", inst.Name)
			}
		}
	}

	// If any instances are in error state, mark cluster as failed
	if hasErrors {
		log.Printf("[PROVISIONING] Cluster %s has instances in error state", resource.Name)
		resource.Status.Phase = models.ClusterPhaseFailed
		resource.Status.Message = fmt.Sprintf("Instance errors: %s", strings.Join(errorMessages, "; "))
		return &models.ReconcileResult{}, nil
	}

	// If not all instances are running yet, keep waiting
	if !allRunning {
		resource.Status.Message = fmt.Sprintf("Waiting for instances: %s", strings.Join(instanceStates, ", "))
		log.Printf("[PROVISIONING] Cluster %s waiting for instances to reach running state", resource.Name)
		return &models.ReconcileResult{
			Requeue:      true,
			RequeueAfter: 10 * time.Second,
		}, nil
	}

	// All instances are running (IPs are optional), move to running phase
	log.Printf("[PROVISIONING] All instances for cluster %s are running, transitioning to running phase", resource.Name)
	resource.Status.Phase = models.ClusterPhaseRunning
	resource.Status.Message = "Cluster is running"
	resource.Status.ObservedGeneration = resource.Generation
	now := time.Now()
	resource.Status.LastReconcileTime = &now

	return &models.ReconcileResult{}, nil
}

// reconcileRunning checks health and handles updates
func (r *Reconciler) reconcileRunning(ctx context.Context, resource *models.ClusterResource) (*models.ReconcileResult, error) {
	log.Printf("[RUNNING] Checking health of running cluster %s", resource.Name)

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

	// Update instance states from cloud provider to keep IP addresses current
	computeService := r.provider.GetComputeService()
	filters := map[string]string{
		"tag:ClusterName":     resource.Name,
		"instance-state-name": "pending,running,stopping,stopped",
	}
	
	// Add region filter if specified in the resource
	if resource.Spec.Region != "" {
		filters["region"] = resource.Spec.Region
		log.Printf("Updating instance states for cluster %s in region %s", resource.Name, resource.Spec.Region)
	}
	
	cloudInstances, err := computeService.ListInstances(ctx, filters)
	if err != nil {
		log.Printf("Warning: failed to list instances for health check: %v", err)
	} else if len(cloudInstances) > 0 {
		// Update Status.Instances with current state from cloud
		instanceMap := make(map[string]*provider.Instance)
		for _, inst := range cloudInstances {
			instanceMap[inst.Name] = inst
		}
		
		// Update existing entries in Status.Instances
		for i := range resource.Status.Instances {
			if cloudInst, ok := instanceMap[resource.Status.Instances[i].Name]; ok {
				// Update with current state from cloud
				resource.Status.Instances[i].State = cloudInst.State
				resource.Status.Instances[i].PrivateIP = cloudInst.PrivateIP
				resource.Status.Instances[i].PublicIP = cloudInst.PublicIP
				log.Printf("Updated instance %s: PrivateIP=%s, PublicIP=%s", 
					resource.Status.Instances[i].Name, 
					cloudInst.PrivateIP, 
					cloudInst.PublicIP)
			}
		}
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

// reconcileFailed moved to phases.go

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

	// Check if this is the new format (with apiVersion and kind)
	if apiVersion, ok := config["apiVersion"].(string); ok && apiVersion != "" {
		// New format - convert to old format for compatibility
		clusterState := make(map[string]interface{})
		
		// Build the cluster object from new format
		cluster := make(map[string]interface{})
		
		// Extract metadata
		if metadata, ok := config["metadata"].(map[string]interface{}); ok {
			cluster["name"] = metadata["name"]
			cluster["id"] = metadata["id"]
			cluster["created_at"] = metadata["created_at"]
			cluster["updated_at"] = metadata["updated_at"]
			
			// Check for deletion timestamp in config metadata
			if deletionTimestamp, ok := metadata["deletionTimestamp"]; ok && deletionTimestamp != nil {
				// Mark this cluster for deletion
				if statusMeta, ok := status["metadata"].(map[string]interface{}); ok {
					statusMeta["deletion_requested"] = deletionTimestamp
				} else {
					status["metadata"] = map[string]interface{}{
						"deletion_requested": deletionTimestamp,
					}
				}
			}
		}
		
		// Extract spec
		if spec, ok := config["spec"].(map[string]interface{}); ok {
			cluster["mode"] = spec["mode"]
			cluster["region"] = spec["region"]
			cluster["instance_type"] = spec["instance_type"]
			cluster["k3s_version"] = spec["k3s_version"]
			cluster["master_nodes"] = spec["master_nodes"]
			cluster["worker_nodes"] = spec["worker_nodes"]
			cluster["network_cidr"] = spec["network_cidr"]
			cluster["service_cidr"] = spec["service_cidr"]
		}
		
		// Add status from status file
		if phase, ok := status["phase"].(string); ok {
			cluster["status"] = phase
		}
		
		clusterState["cluster"] = cluster
		clusterState["instance_ids"] = status["instance_ids"]
		clusterState["metadata"] = status["metadata"]
		
		return r.convertStateToResource(clusterState)
	} else {
		// Old format - keep existing logic for backward compatibility
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

	// Check metadata for phase and other reconciler-controlled state
	if metadata, ok := state["metadata"].(map[string]interface{}); ok {
		// FIRST: Check for deletion timestamp - this takes precedence over everything
		if deletionRequested, ok := metadata["deletion_requested"]; ok {
			// Cluster is marked for deletion
			resource.Status.Phase = models.ClusterPhaseDeleting
			resource.Status.Message = fmt.Sprintf("Deletion requested at %v", deletionRequested)

			// Set DeletionTimestamp if available
			// Handle both string format and time.Time format (from Manager)
			switch v := deletionRequested.(type) {
			case string:
				if t, err := time.Parse(time.RFC3339, v); err == nil {
					resource.DeletionTimestamp = &t
				}
			case float64:
				// JSON number (Unix timestamp)
				t := time.Unix(int64(v), 0)
				resource.DeletionTimestamp = &t
			default:
				// Could be a time.Time object marshaled as string like "2024-01-01T00:00:00Z"
				if timeStr, ok := v.(string); ok {
					if t, err := time.Parse(time.RFC3339, timeStr); err == nil {
						resource.DeletionTimestamp = &t
					}
				}
			}
		} else {
			// Only read phase from metadata if not deleting
			if phase, ok := metadata["phase"].(string); ok && phase != "" {
				switch phase {
				case models.ClusterPhaseProvisioning:
					resource.Status.Phase = models.ClusterPhaseProvisioning
				case models.ClusterPhaseRunning:
					resource.Status.Phase = models.ClusterPhaseRunning
				case models.ClusterPhaseFailed:
					resource.Status.Phase = models.ClusterPhaseFailed
				case models.ClusterPhaseDeleting:
					resource.Status.Phase = models.ClusterPhaseDeleting
				default:
					// Keep the phase from cluster status if metadata phase is unknown
				}
			}
		}
		
		// Read message from metadata
		if message, ok := metadata["message"].(string); ok {
			resource.Status.Message = message
		}
		
		// Check retry count (for logging/debugging)
		if retryCount, ok := metadata["provision_retry_count"].(float64); ok && retryCount > 0 {
			log.Printf("Cluster %s has been retried %v times", resource.Name, retryCount)
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
	
	// First, load existing status to preserve metadata like retry counts
	var existingMetadata map[string]interface{}
	existingStatusData, err := r.provider.GetStorageService().GetObject(ctx, statusKey)
	if err == nil {
		var existingStatus map[string]interface{}
		if json.Unmarshal(existingStatusData, &existingStatus) == nil {
			if metadata, ok := existingStatus["metadata"].(map[string]interface{}); ok {
				existingMetadata = metadata
			}
		}
	}

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
	metadata := map[string]interface{}{
		"last_reconciled":    time.Now().Format(time.RFC3339),
		"phase":              resource.Status.Phase,
		"message":            resource.Status.Message,
		"observed_generation": resource.Status.ObservedGeneration,
	}
	
	// Track provisioning retry count for monitoring purposes only
	// Not used for failure decisions - failures are based on actual instance/API errors
	if resource.Status.Phase == models.ClusterPhaseProvisioning {
		retryCount := 0
		if existingMetadata != nil {
			if count, ok := existingMetadata["provision_retry_count"].(float64); ok {
				retryCount = int(count)
			}
		}
		// Increment for monitoring
		metadata["provision_retry_count"] = retryCount + 1
		log.Printf("[SAVE] Cluster %s provisioning attempt #%d", resource.Name, retryCount+1)
	} else {
		// Reset retry count when phase changes
		metadata["provision_retry_count"] = 0
	}
	
	// Set deletion_requested if DeletionTimestamp is set on resource
	// This comes from the config.json metadata.deletionTimestamp field
	if resource.DeletionTimestamp != nil {
		metadata["deletion_requested"] = resource.DeletionTimestamp.Format(time.RFC3339)
	}
	
	statusState["metadata"] = metadata

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
		// Only log if it's not a "topic not found" error - SNS topics are optional
		if !strings.Contains(err.Error(), "NotFound") && !strings.Contains(err.Error(), "Topic does not exist") {
			log.Printf("Failed to send notification: %v", err)
		}
		// Silently ignore "topic not found" errors as notifications are optional
	}
}
