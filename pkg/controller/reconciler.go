package controller

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"
	"gopkg.in/yaml.v3"

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

	// All instances are running (IPs are optional), move to installing phase
	log.Printf("[PROVISIONING] All instances for cluster %s are running, transitioning to installing phase", resource.Name)
	resource.Status.Phase = models.ClusterPhaseInstalling
	resource.Status.Message = "Installing K3s on instances"
	resource.Status.ObservedGeneration = resource.Generation
	now := time.Now()
	resource.Status.LastReconcileTime = &now

	return &models.ReconcileResult{
		Requeue:      true,
		RequeueAfter: 5 * time.Second,
	}, nil
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

	// Check if instance type needs to be changed
	needsResize, err := r.checkInstanceTypeChanges(ctx, resource)
	if err != nil {
		log.Printf("Error checking instance type changes: %v", err)
	} else if needsResize {
		log.Printf("[RESIZE] Instance type change detected for cluster %s", resource.Name)
		return r.resizeInstances(ctx, resource)
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
				// Update with current state from cloud - but preserve K3s status fields
				resource.Status.Instances[i].State = cloudInst.State
				resource.Status.Instances[i].PrivateIP = cloudInst.PrivateIP
				resource.Status.Instances[i].PublicIP = cloudInst.PublicIP
				// NOTE: We intentionally DO NOT update K3s status fields here
				// Those are managed by the Configuring phase and K3s health checks
				log.Printf("Updated instance %s: PrivateIP=%s, PublicIP=%s, K3sRunning=%v", 
					resource.Status.Instances[i].Name, 
					cloudInst.PrivateIP, 
					cloudInst.PublicIP,
					resource.Status.Instances[i].K3sRunning)
			}
		}
	}
	
	// Check K3s service health on master nodes
	// We check all master nodes regardless of K3sRunning status to detect issues
	needsConfiguration := false
	for i := range resource.Status.Instances {
		inst := &resource.Status.Instances[i]
		if inst.Role == "master" {
			// Check if K3s was supposed to be installed and running
			if inst.K3sInstalled {
				// Check if K3s service is actually running
				isRunning, err := r.checkK3sServiceStatus(ctx, inst.InstanceID)
				if err != nil {
					log.Printf("[RUNNING] Failed to check K3s service on %s: %v", inst.Name, err)
				} else {
					if isRunning && !inst.K3sRunning {
						// Service is running but status says it's not - update status
						log.Printf("[RUNNING] K3s service is running on %s but status was false, updating", inst.Name)
						inst.K3sRunning = true
					} else if !isRunning && inst.K3sRunning {
						// Service stopped but status says it's running
						log.Printf("[RUNNING] K3s service stopped on %s, marking as not running", inst.Name)
						inst.K3sRunning = false
						inst.K3sConfigError = "K3s service stopped unexpectedly"
						needsConfiguration = true
					} else if !isRunning && !inst.K3sRunning {
						// Service is not running and status reflects that - needs configuration
						log.Printf("[RUNNING] K3s service not running on %s, needs configuration", inst.Name)
						needsConfiguration = true
					}
				}
			} else if !inst.K3sInstalled {
				// K3s not installed at all - needs installation
				log.Printf("[RUNNING] K3s not installed on %s, needs installation", inst.Name)
				resource.Status.Phase = models.ClusterPhaseInstalling
				resource.Status.Message = "K3s installation required"
				return &models.ReconcileResult{
					Requeue:      true,
					RequeueAfter: 10 * time.Second,
				}, nil
			}
		}
	}
	
	// If any master needs configuration, transition to Configuring phase
	if needsConfiguration {
		log.Printf("[RUNNING] Detected K3s services need configuration, transitioning to Configuring phase")
		resource.Status.Phase = models.ClusterPhaseConfiguring
		resource.Status.Message = "Configuring K3s services"
		return &models.ReconcileResult{
			Requeue:      true,
			RequeueAfter: 10 * time.Second,
		}, nil
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

// checkInstanceTypeChanges checks if any instances need resizing
func (r *Reconciler) checkInstanceTypeChanges(ctx context.Context, resource *models.ClusterResource) (bool, error) {
	computeService := r.provider.GetComputeService()
	
	// Get current instances
	filters := map[string]string{
		"tag:ClusterName":     resource.Name,
		"instance-state-name": "running,stopped",
	}
	
	if resource.Spec.Region != "" {
		filters["region"] = resource.Spec.Region
	}
	
	instances, err := computeService.ListInstances(ctx, filters)
	if err != nil {
		return false, fmt.Errorf("failed to list instances: %w", err)
	}
	
	// Check if any instance has different type than desired
	desiredType := resource.Spec.InstanceType
	if desiredType == "" {
		desiredType = "t3.medium" // default
	}
	
	for _, inst := range instances {
		if inst.InstanceType != desiredType {
			log.Printf("[RESIZE] Instance %s has type %s, but desired is %s", inst.Name, inst.InstanceType, desiredType)
			return true, nil
		}
	}
	
	return false, nil
}

// resizeInstances handles resizing instances to new instance type
func (r *Reconciler) resizeInstances(ctx context.Context, resource *models.ClusterResource) (*models.ReconcileResult, error) {
	computeService := r.provider.GetComputeService()
	
	// Update status
	resource.Status.Phase = models.ClusterPhaseUpdating
	resource.Status.Message = fmt.Sprintf("Resizing instances to %s", resource.Spec.InstanceType)
	
	// Save status to notify UI
	if err := r.saveClusterResource(ctx, resource); err != nil {
		log.Printf("Failed to save status: %v", err)
	}
	
	// Get instances that need resizing
	filters := map[string]string{
		"tag:ClusterName":     resource.Name,
		"instance-state-name": "running,stopped",
	}
	
	if resource.Spec.Region != "" {
		filters["region"] = resource.Spec.Region
	}
	
	instances, err := computeService.ListInstances(ctx, filters)
	if err != nil {
		return nil, fmt.Errorf("failed to list instances: %w", err)
	}
	
	desiredType := resource.Spec.InstanceType
	if desiredType == "" {
		desiredType = "t3.medium"
	}
	
	// Process each instance that needs resizing
	var resizeErrors []string
	resizedCount := 0
	
	for _, inst := range instances {
		if inst.InstanceType == desiredType {
			continue // Already correct type
		}
		
		log.Printf("[RESIZE] Resizing instance %s from %s to %s", inst.Name, inst.InstanceType, desiredType)
		
		// Stop instance if running
		if inst.State == "running" {
			log.Printf("[RESIZE] Stopping instance %s", inst.Name)
			if err := computeService.StopInstance(ctx, inst.ID); err != nil {
				resizeErrors = append(resizeErrors, fmt.Sprintf("%s: failed to stop: %v", inst.Name, err))
				continue
			}
			
			// Wait for instance to stop (with timeout)
			stopCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
			err := r.waitForInstanceState(stopCtx, inst.ID, "stopped")
			cancel()
			
			if err != nil {
				resizeErrors = append(resizeErrors, fmt.Sprintf("%s: timeout waiting for stop: %v", inst.Name, err))
				continue
			}
		}
		
		// Modify instance type
		log.Printf("[RESIZE] Modifying instance %s type to %s", inst.Name, desiredType)
		if err := computeService.ModifyInstanceType(ctx, inst.ID, desiredType); err != nil {
			resizeErrors = append(resizeErrors, fmt.Sprintf("%s: failed to modify type: %v", inst.Name, err))
			continue
		}
		
		// Start instance
		log.Printf("[RESIZE] Starting instance %s", inst.Name)
		if err := computeService.StartInstance(ctx, inst.ID); err != nil {
			resizeErrors = append(resizeErrors, fmt.Sprintf("%s: failed to start: %v", inst.Name, err))
			continue
		}
		
		resizedCount++
		
		// Update status with progress
		resource.Status.Message = fmt.Sprintf("Resized %d/%d instances to %s", resizedCount, len(instances), desiredType)
		if err := r.saveClusterResource(ctx, resource); err != nil {
			log.Printf("Failed to save status: %v", err)
		}
	}
	
	// Check results
	if len(resizeErrors) > 0 {
		resource.Status.Phase = models.ClusterPhaseFailed
		resource.Status.Message = fmt.Sprintf("Resize failed: %s", strings.Join(resizeErrors, "; "))
		return &models.ReconcileResult{
			Requeue:      true,
			RequeueAfter: 30 * time.Second,
		}, fmt.Errorf("resize errors: %v", resizeErrors)
	}
	
	// All instances resized successfully
	log.Printf("[RESIZE] Successfully resized %d instances for cluster %s", resizedCount, resource.Name)
	resource.Status.Phase = models.ClusterPhaseRunning
	resource.Status.Message = fmt.Sprintf("All instances resized to %s", desiredType)
	
	return &models.ReconcileResult{
		Requeue:      true,
		RequeueAfter: 10 * time.Second,
	}, nil
}

// waitForInstanceState waits for an instance to reach the desired state
func (r *Reconciler) waitForInstanceState(ctx context.Context, instanceID, desiredState string) error {
	computeService := r.provider.GetComputeService()
	
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for instance %s to reach state %s", instanceID, desiredState)
		case <-time.After(5 * time.Second):
			// Check instance state
			filters := map[string]string{
				"instance-id": instanceID,
			}
			instances, err := computeService.ListInstances(ctx, filters)
			if err != nil {
				return fmt.Errorf("failed to get instance state: %w", err)
			}
			
			if len(instances) > 0 && instances[0].State == desiredState {
				return nil // Reached desired state
			}
		}
	}
}

// loadClusterResource loads cluster resource from storage with timeout
func (r *Reconciler) loadClusterResource(ctx context.Context, name string) (*models.ClusterResource, error) {
	// Create a timeout context for loading (30 seconds)
	loadCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Load from new format only: separate config and status files
	configKey := fmt.Sprintf("clusters/%s/config.yaml", name)
	statusKey := fmt.Sprintf("clusters/%s/status.yaml", name)

	// Load config (required)
	configData, err := r.provider.GetStorageService().GetObject(loadCtx, configKey)
	if err != nil {
		return nil, fmt.Errorf("cluster config %s not found: %w", name, err)
	}

	var config map[string]interface{}
	if err := yaml.Unmarshal(configData, &config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal cluster config: %w", err)
	}

	// Load status (optional - might not exist for new clusters)
	var status map[string]interface{}
	statusData, err := r.provider.GetStorageService().GetObject(loadCtx, statusKey)
	if err == nil {
		if err := yaml.Unmarshal(statusData, &status); err != nil {
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
		
		// Extract spec (using camelCase from YAML)
		if spec, ok := config["spec"].(map[string]interface{}); ok {
			cluster["mode"] = spec["mode"]
			cluster["region"] = spec["region"]
			cluster["instance_type"] = spec["instanceType"]
			cluster["k3s_version"] = spec["k3sVersion"]
			cluster["master_nodes"] = spec["masterNodes"]
			cluster["worker_nodes"] = spec["workerNodes"]
			cluster["network_cidr"] = spec["networkCIDR"]
			cluster["service_cidr"] = spec["serviceCIDR"]
			cluster["cluster_dns"] = spec["clusterDNS"]
			cluster["description"] = spec["description"]
			
			// Debug logging
			log.Printf("[CONFIG] Loaded instanceType: %v", spec["instanceType"])
		}
		
		// Add status from status file
		if phase, ok := status["phase"].(string); ok {
			cluster["status"] = phase
		}
		
		clusterState["cluster"] = cluster
		clusterState["instance_ids"] = status["instance_ids"]
		clusterState["instances"] = status["instances"]  // Add detailed instance data
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
		clusterState["instances"] = status["instances"]  // Add detailed instance data
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
		log.Printf("[CONFIG] Set resource InstanceType to: %s", instanceType)
	} else {
		log.Printf("[CONFIG] No instance_type found in cluster config, using default: %s", resource.Spec.InstanceType)
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
				instance := models.InstanceStatus{
					InstanceID: id,
					Name:       nodeName,
					State:      "running", // Default to running if ID exists
				}
				
				// Load detailed instance data if available
				if instances, ok := state["instances"].(map[string]interface{}); ok {
					if instData, ok := instances[nodeName].(map[string]interface{}); ok {
						// Load basic fields
						if state, ok := instData["state"].(string); ok {
							instance.State = state
						}
						if privateIP, ok := instData["private_ip"].(string); ok {
							instance.PrivateIP = privateIP
						}
						if publicIP, ok := instData["public_ip"].(string); ok {
							instance.PublicIP = publicIP
						}
						if role, ok := instData["role"].(string); ok {
							instance.Role = role
						}
						
						// Load K3s installation status fields
						if k3sInstalled, ok := instData["k3s_installed"].(bool); ok {
							instance.K3sInstalled = k3sInstalled
						}
						if k3sVersion, ok := instData["k3s_version"].(string); ok {
							instance.K3sVersion = k3sVersion
						}
						if k3sInstallTime, ok := instData["k3s_install_time"].(string); ok {
							if t, err := time.Parse(time.RFC3339, k3sInstallTime); err == nil {
								instance.K3sInstallTime = &t
							}
						}
						if k3sInstallError, ok := instData["k3s_install_error"].(string); ok {
							instance.K3sInstallError = k3sInstallError
						}
						
						// Load K3s configuration status fields
						if k3sRunning, ok := instData["k3s_running"].(bool); ok {
							instance.K3sRunning = k3sRunning
						}
						if k3sConfigTime, ok := instData["k3s_config_time"].(string); ok {
							if t, err := time.Parse(time.RFC3339, k3sConfigTime); err == nil {
								instance.K3sConfigTime = &t
							}
						}
						if k3sConfigError, ok := instData["k3s_config_error"].(string); ok {
							instance.K3sConfigError = k3sConfigError
						}
					}
				}
				
				resource.Status.Instances = append(resource.Status.Instances, instance)
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
				case models.ClusterPhaseInstalling:
					resource.Status.Phase = models.ClusterPhaseInstalling
				case models.ClusterPhaseConfiguring:
					resource.Status.Phase = models.ClusterPhaseConfiguring
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
	statusKey := fmt.Sprintf("clusters/%s/status.yaml", resource.Name)
	
	// First, load existing status to preserve metadata like retry counts
	var existingMetadata map[string]interface{}
	existingStatusData, err := r.provider.GetStorageService().GetObject(ctx, statusKey)
	if err == nil {
		var existingStatus map[string]interface{}
		if yaml.Unmarshal(existingStatusData, &existingStatus) == nil {
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
		instanceState := map[string]interface{}{
			"id":         inst.InstanceID,
			"state":      inst.State,
			"private_ip": inst.PrivateIP,
			"public_ip":  inst.PublicIP,
			"role":       inst.Role,
		}
		
		// Add K3s installation status fields
		if inst.K3sInstalled {
			instanceState["k3s_installed"] = inst.K3sInstalled
			instanceState["k3s_version"] = inst.K3sVersion
			if inst.K3sInstallTime != nil {
				instanceState["k3s_install_time"] = inst.K3sInstallTime.Format(time.RFC3339)
			}
		}
		if inst.K3sInstallError != "" {
			instanceState["k3s_install_error"] = inst.K3sInstallError
		}
		
		// Add K3s configuration status fields
		if inst.K3sRunning {
			instanceState["k3s_running"] = inst.K3sRunning
			if inst.K3sConfigTime != nil {
				instanceState["k3s_config_time"] = inst.K3sConfigTime.Format(time.RFC3339)
			}
		}
		if inst.K3sConfigError != "" {
			instanceState["k3s_config_error"] = inst.K3sConfigError
		}
		
		instanceStates[inst.Name] = instanceState
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
	statusData, err := yaml.Marshal(statusState)
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
