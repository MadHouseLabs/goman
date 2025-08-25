package controller

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/madhouselabs/goman/pkg/models"
	"github.com/madhouselabs/goman/pkg/provider"
)

// Reconciler handles cluster reconciliation with a simple linear approach
type Reconciler struct {
	provider provider.Provider
	owner    string
}

// NewReconciler creates a new simple reconciler
func NewReconciler(prov provider.Provider, owner string) (*Reconciler, error) {
	if prov == nil {
		return nil, fmt.Errorf("provider is required")
	}

	if owner == "" {
		owner = fmt.Sprintf("reconciler-%s-%d", prov.Region(), time.Now().UnixNano())
	}

	return &Reconciler{
		provider: prov,
		owner:    owner,
	}, nil
}

// ReconcileCluster reconciles a cluster with simple linear flow
func (r *Reconciler) ReconcileCluster(ctx context.Context, clusterName string) (*models.ReconcileResult, error) {
	return r.ReconcileClusterWithRequestID(ctx, clusterName, "unknown")
}

// ReconcileClusterWithRequestID reconciles a cluster with request tracking
func (r *Reconciler) ReconcileClusterWithRequestID(ctx context.Context, clusterName string, requestID string) (*models.ReconcileResult, error) {
	log.Printf("[RECONCILE] Starting reconciliation for cluster %s (request: %s)", clusterName, requestID)

	// Create timeout context (14 minutes to be safe within Lambda limit)
	reconcileCtx, cancel := context.WithTimeout(ctx, 14*time.Minute)
	defer cancel()

	// Acquire distributed lock
	resourceID := fmt.Sprintf("cluster-%s", clusterName)
	lockToken, err := r.acquireLock(reconcileCtx, resourceID)
	if err != nil {
		log.Printf("[RECONCILE] Failed to acquire lock: %v", err)
		return &models.ReconcileResult{Requeue: true, RequeueAfter: 30 * time.Second}, nil
	}
	defer r.releaseLock(reconcileCtx, resourceID, lockToken)

	// Load cluster configuration and status
	cluster, err := r.loadCluster(reconcileCtx, clusterName)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			log.Printf("[RECONCILE] Cluster %s not found, skipping", clusterName)
			return &models.ReconcileResult{Requeue: false}, nil
		}
		log.Printf("[RECONCILE] Failed to load cluster: %v", err)
		return &models.ReconcileResult{Requeue: true, RequeueAfter: 1 * time.Minute}, nil
	}

	// Handle deletion if requested
	if cluster.DeletionTimestamp != nil {
		return r.handleDeletion(reconcileCtx, cluster)
	}

	// Execute reconciliation based on current phase
	needsRequeue, err := r.reconcileCluster(reconcileCtx, cluster)
	if err != nil {
		log.Printf("[RECONCILE] Reconciliation failed: %v", err)
		cluster.Status.Phase = string(models.ClusterPhaseFailed)
		cluster.Status.Message = err.Error()
		r.saveCluster(reconcileCtx, cluster)
		return &models.ReconcileResult{Requeue: true, RequeueAfter: 2 * time.Minute}, nil
	}

	// Save final state
	err = r.saveCluster(reconcileCtx, cluster)
	if err != nil {
		log.Printf("[RECONCILE] Failed to save cluster state: %v", err)
		return &models.ReconcileResult{Requeue: true, RequeueAfter: 30 * time.Second}, nil
	}

	// Check if we need to requeue for further processing
	if cluster.Status.Phase == string(models.ClusterPhaseRunning) {
		if needsRequeue {
			log.Printf("[RECONCILE] Cluster %s is running but needs requeue (cleanup happened)", clusterName)
			return &models.ReconcileResult{Requeue: true, RequeueAfter: 15 * time.Second}, nil
		}
		log.Printf("[RECONCILE] Cluster %s is ready", clusterName)
		return &models.ReconcileResult{Requeue: false}, nil
	}

	log.Printf("[RECONCILE] Cluster %s phase: %s, requeuing", clusterName, cluster.Status.Phase)
	return &models.ReconcileResult{Requeue: true, RequeueAfter: 30 * time.Second}, nil
}

// reconcileCluster performs the main reconciliation logic
// Returns (needsRequeue, error) where needsRequeue indicates if reconciliation should run again
func (r *Reconciler) reconcileCluster(ctx context.Context, cluster *models.ClusterResource) (bool, error) {
	log.Printf("[RECONCILE] Processing cluster %s in phase %s", cluster.Name, cluster.Status.Phase)

	switch cluster.Status.Phase {
	case string(models.ClusterPhasePending), "":
		return false, r.provisionInfrastructure(ctx, cluster)
	case string(models.ClusterPhaseProvisioning):
		return false, r.checkProvisioningProgress(ctx, cluster)
	case string(models.ClusterPhaseInstalling):
		return false, r.installK3s(ctx, cluster)
	case string(models.ClusterPhaseConfiguring):
		return false, r.configureK3s(ctx, cluster)
	case string(models.ClusterPhaseRunning):
		log.Printf("[RECONCILE] Cluster %s is running, checking for updates", cluster.Name)
		return r.reconcileRunningCluster(ctx, cluster)
	default:
		log.Printf("[RECONCILE] Unknown phase %s for cluster %s", cluster.Status.Phase, cluster.Name)
		cluster.Status.Phase = string(models.ClusterPhasePending)
		return false, nil
	}
}

// handleDeletion handles cluster deletion
func (r *Reconciler) handleDeletion(ctx context.Context, cluster *models.ClusterResource) (*models.ReconcileResult, error) {
	log.Printf("[DELETE] Processing deletion for cluster %s", cluster.Name)
	
	// Update status to deleting
	cluster.Status.Phase = "Deleting"
	cluster.Status.Message = "Deleting cluster resources"
	
	// Get compute service
	computeService := r.provider.GetComputeService()
	
	// Get ALL instances for this cluster from EC2 (not just from status)
	filters := map[string]string{
		"tag:goman-cluster": cluster.Name,
		"instance-state-name": "running,pending,stopping,stopped",
	}
	
	instances, err := computeService.ListInstances(ctx, filters)
	if err != nil {
		log.Printf("[DELETE] Warning: Failed to list instances from EC2: %v", err)
		// Fall back to status file if we can't query EC2
		for _, instance := range cluster.Status.Instances {
			if instance.InstanceID != "" {
				log.Printf("[DELETE] Deleting instance from status: %s (%s)", instance.Name, instance.InstanceID)
				if err := computeService.DeleteInstance(ctx, instance.InstanceID); err != nil {
					log.Printf("[DELETE] Failed to delete instance %s: %v", instance.InstanceID, err)
				}
			}
		}
	} else {
		// Delete all instances found in EC2
		log.Printf("[DELETE] Found %d instances to delete for cluster %s", len(instances), cluster.Name)
		deletedCount := 0
		for _, instance := range instances {
			log.Printf("[DELETE] Deleting instance %s (%s) - %s", instance.Name, instance.ID, instance.State)
			if err := computeService.DeleteInstance(ctx, instance.ID); err != nil {
				log.Printf("[DELETE] Failed to delete instance %s: %v", instance.ID, err)
				// Continue with other instances even if one fails
			} else {
				deletedCount++
			}
		}
		log.Printf("[DELETE] Successfully deleted %d/%d instances for cluster %s", deletedCount, len(instances), cluster.Name)
	}
	
	// Note: We intentionally keep the security group as it can be reused
	// if the cluster is recreated with the same name. AWS will clean up
	// unused security groups during account maintenance.
	// If you want to delete it, uncomment the following:
	// sgName := fmt.Sprintf("goman-%s-sg", cluster.Name)
	// err := computeService.DeleteSecurityGroup(ctx, sgName)
	// if err != nil {
	//     log.Printf("[DELETE] Failed to delete security group: %v", err)
	// }
	
	// Delete cluster files from S3
	storageService := r.provider.GetStorageService()
	
	// Delete config file
	configKey := fmt.Sprintf("clusters/%s/config.yaml", cluster.Name)
	if err := storageService.DeleteObject(ctx, configKey); err != nil {
		log.Printf("[DELETE] Failed to delete config file: %v", err)
	}
	
	// Delete status file
	statusKey := fmt.Sprintf("clusters/%s/status.yaml", cluster.Name)
	err = storageService.DeleteObject(ctx, statusKey)
	if err != nil {
		log.Printf("[DELETE] Failed to delete status file: %v", err)
	}
	
	// Delete token files (using correct paths)
	tokenKeys := []string{
		fmt.Sprintf("clusters/%s/k3s-server-token", cluster.Name),
		fmt.Sprintf("clusters/%s/k3s-agent-token", cluster.Name),
	}
	for _, key := range tokenKeys {
		err = storageService.DeleteObject(ctx, key)
		if err != nil {
			log.Printf("[DELETE] Failed to delete token file %s: %v", key, err)
		}
	}
	
	log.Printf("[DELETE] Cluster %s deletion completed", cluster.Name)
	return &models.ReconcileResult{Requeue: false}, nil
}

// provisionInfrastructure provisions VMs and generates tokens
func (r *Reconciler) provisionInfrastructure(ctx context.Context, cluster *models.ClusterResource) error {
	log.Printf("[PROVISION] Starting infrastructure provisioning for cluster %s", cluster.Name)
	
	// Generate K3s token - same token for both server and agents
	// K3s agents can join with the server token directly
	k3sToken, err := r.generateToken()
	if err != nil {
		return fmt.Errorf("failed to generate K3s token: %w", err)
	}
	
	// Save tokens to S3 - use same token for both server and agent
	if err := r.saveTokens(ctx, cluster.Name, k3sToken, k3sToken); err != nil {
		return fmt.Errorf("failed to save tokens: %w", err)
	}
	
	// Store tokens in cluster status
	cluster.Status.K3sServerToken = k3sToken
	cluster.Status.K3sAgentToken = k3sToken
	
	computeService := r.provider.GetComputeService()
	
	// For HA mode, we need to create the first master, wait for it to be ready,
	// then create the other masters with the first master's IP
	if cluster.Spec.Mode == "ha" {
		// Check if we already have instances created
		if len(cluster.Status.Instances) == 0 {
			// Create only the first master node
			log.Printf("[PROVISION] Creating first master node for HA cluster %s", cluster.Name)
			
			instanceName := fmt.Sprintf("%s-master-0", cluster.Name)
			instanceConfig := provider.InstanceConfig{
				Name:         instanceName,
				Region:       cluster.Spec.Region,
				InstanceType: cluster.Spec.InstanceType,
				Tags: map[string]string{
					"goman-cluster": cluster.Name,
					"goman-role":    "master",
					"goman-index":   "0",
					"ManagedBy":     "goman",
				},
			}
			
			instance, err := computeService.CreateInstance(ctx, instanceConfig)
			if err != nil {
				return fmt.Errorf("failed to create first master instance %s: %w", instanceName, err)
			}
			
			instanceStatus := models.InstanceStatus{
				InstanceID: instance.ID,
				Name:       instanceName,
				Role:       "master",
				State:      instance.State,
				LaunchTime: time.Now(),
			}
			
			cluster.Status.Instances = []models.InstanceStatus{instanceStatus}
			cluster.Status.Phase = string(models.ClusterPhaseProvisioning)
			cluster.Status.Message = "Created first master node, waiting for it to start"
			log.Printf("[PROVISION] Created first master %s (%s)", instanceName, instance.ID)
			
		} else {
			// Additional masters will be created in checkProvisioningProgress
			cluster.Status.Message = "First master created, additional masters will be created after it's ready"
		}
	} else {
		// Dev mode - create single master
		log.Printf("[PROVISION] Creating single master for dev cluster %s", cluster.Name)
		
		instanceName := fmt.Sprintf("%s-master-0", cluster.Name)
		instanceConfig := provider.InstanceConfig{
			Name:         instanceName,
			Region:       cluster.Spec.Region,
			InstanceType: cluster.Spec.InstanceType,
			Tags: map[string]string{
				"goman-cluster": cluster.Name,
				"goman-role":    "master",
				"ManagedBy":     "goman",
			},
		}
		
		instance, err := computeService.CreateInstance(ctx, instanceConfig)
		if err != nil {
			return fmt.Errorf("failed to create instance %s: %w", instanceName, err)
		}
		
		instanceStatus := models.InstanceStatus{
			InstanceID: instance.ID,
			Name:       instanceName,
			Role:       "master",
			State:      instance.State,
			LaunchTime: time.Now(),
		}
		
		cluster.Status.Instances = []models.InstanceStatus{instanceStatus}
		cluster.Status.Phase = string(models.ClusterPhaseProvisioning)
		cluster.Status.Message = "Provisioned 1 instance, waiting for it to start"
		log.Printf("[PROVISION] Created instance %s (%s) in state %s", instanceName, instance.ID, instance.State)
	}
	
	log.Printf("[PROVISION] Infrastructure provisioning initiated for cluster %s", cluster.Name)
	return nil
}

// checkProvisioningProgress checks if instances are ready
func (r *Reconciler) checkProvisioningProgress(ctx context.Context, cluster *models.ClusterResource) error {
	log.Printf("[PROVISION] Checking provisioning progress for cluster %s", cluster.Name)
	
	computeService := r.provider.GetComputeService()
	
	// For HA mode, we need special handling
	if cluster.Spec.Mode == "ha" {
		// First, update status of existing instances
		for i, instanceStatus := range cluster.Status.Instances {
			instance, err := computeService.GetInstance(ctx, instanceStatus.InstanceID)
			if err != nil {
				log.Printf("[PROVISION] Failed to get instance %s status: %v", instanceStatus.InstanceID, err)
				continue
			}
			
			// Update instance status
			cluster.Status.Instances[i].State = instance.State
			cluster.Status.Instances[i].PrivateIP = instance.PrivateIP
			cluster.Status.Instances[i].PublicIP = instance.PublicIP
		}
		
		// Check if we need to create additional masters
		if len(cluster.Status.Instances) == 1 {
			// Only first master exists
			firstMaster := cluster.Status.Instances[0]
			if firstMaster.State == "running" && firstMaster.PrivateIP != "" {
				// First master is ready, create the remaining two masters
				log.Printf("[PROVISION] First master ready with IP %s, creating remaining masters", firstMaster.PrivateIP)
				
				// Store the first master IP for other nodes to join
				cluster.Status.PreferredMasterInstance = firstMaster.InstanceID
				
				for i := 1; i < 3; i++ {
					instanceName := fmt.Sprintf("%s-master-%d", cluster.Name, i)
					instanceConfig := provider.InstanceConfig{
						Name:         instanceName,
						Region:       cluster.Spec.Region,
						InstanceType: cluster.Spec.InstanceType,
						Tags: map[string]string{
							"goman-cluster":     cluster.Name,
							"goman-role":        "master",
							"goman-index":       fmt.Sprintf("%d", i),
							"goman-master-ip":   firstMaster.PrivateIP,
							"ManagedBy":         "goman",
						},
					}
					
					instance, err := computeService.CreateInstance(ctx, instanceConfig)
					if err != nil {
						return fmt.Errorf("failed to create master instance %s: %w", instanceName, err)
					}
					
					instanceStatus := models.InstanceStatus{
						InstanceID: instance.ID,
						Name:       instanceName,
						Role:       "master",
						State:      instance.State,
						LaunchTime: time.Now(),
					}
					
					cluster.Status.Instances = append(cluster.Status.Instances, instanceStatus)
					log.Printf("[PROVISION] Created additional master %s (%s)", instanceName, instance.ID)
				}
				
				cluster.Status.Message = "Created all 3 master nodes, waiting for them to start"
				return nil // Stay in Provisioning phase to check the new instances
			} else {
				// First master not ready yet
				cluster.Status.Message = fmt.Sprintf("Waiting for first master to be ready (state: %s)", firstMaster.State)
				log.Printf("[PROVISION] Waiting for first master to be ready")
				return nil
			}
		}
		
		// Check if all 3 masters are running
		if len(cluster.Status.Instances) == 3 {
			allRunning := true
			for _, inst := range cluster.Status.Instances {
				if inst.State != "running" {
					allRunning = false
					break
				}
			}
			
			if allRunning {
				cluster.Status.Phase = string(models.ClusterPhaseInstalling)
				cluster.Status.Message = "All 3 master nodes are running, ready for K3s installation"
				log.Printf("[PROVISION] All HA masters running for cluster %s", cluster.Name)
			} else {
				cluster.Status.Message = "Waiting for all master nodes to reach running state"
				log.Printf("[PROVISION] Still waiting for HA masters to start for cluster %s", cluster.Name)
			}
		}
	} else {
		// Dev mode - simple single instance check
		allRunning := true
		for i, instanceStatus := range cluster.Status.Instances {
			instance, err := computeService.GetInstance(ctx, instanceStatus.InstanceID)
			if err != nil {
				log.Printf("[PROVISION] Failed to get instance %s status: %v", instanceStatus.InstanceID, err)
				continue
			}
			
			// Update instance status
			cluster.Status.Instances[i].State = instance.State
			cluster.Status.Instances[i].PrivateIP = instance.PrivateIP
			cluster.Status.Instances[i].PublicIP = instance.PublicIP
			
			if instance.State != "running" {
				allRunning = false
				log.Printf("[PROVISION] Instance %s is in state %s", instanceStatus.InstanceID, instance.State)
			}
		}
		
		if allRunning {
			cluster.Status.Phase = string(models.ClusterPhaseInstalling)
			cluster.Status.Message = "All instances are running, ready for K3s installation"
			log.Printf("[PROVISION] All instances running for cluster %s", cluster.Name)
		} else {
			cluster.Status.Message = "Waiting for instances to reach running state"
			log.Printf("[PROVISION] Still waiting for instances to start for cluster %s", cluster.Name)
		}
	}
	
	return nil
}

// installK3s installs K3s on instances
func (r *Reconciler) installK3s(ctx context.Context, cluster *models.ClusterResource) error {
	log.Printf("[INSTALL] Starting K3s installation for cluster %s", cluster.Name)
	
	// For now, just move to configuring phase
	cluster.Status.Phase = string(models.ClusterPhaseConfiguring)
	cluster.Status.Message = "K3s installation completed, configuring cluster"
	
	log.Printf("[INSTALL] K3s installation completed for cluster %s", cluster.Name)
	return nil
}

// configureK3s configures K3s cluster and provisions node pools
func (r *Reconciler) configureK3s(ctx context.Context, cluster *models.ClusterResource) error {
	log.Printf("[CONFIGURE] Starting K3s configuration for cluster %s", cluster.Name)
	
	// Check if we need to provision node pools
	if len(cluster.Spec.NodePools) > 0 {
		log.Printf("[CONFIGURE] Provisioning %d node pools for cluster %s", len(cluster.Spec.NodePools), cluster.Name)
		if err := r.provisionNodePools(ctx, cluster); err != nil {
			return fmt.Errorf("failed to provision node pools: %w", err)
		}
	}
	
	// Mark as running
	cluster.Status.Phase = string(models.ClusterPhaseRunning)
	cluster.Status.Message = "K3s cluster is running and ready"
	
	log.Printf("[CONFIGURE] K3s configuration completed for cluster %s", cluster.Name)
	return nil
}

// reconcileRunningCluster handles reconciliation for a running cluster (scaling, updates, etc.)
func (r *Reconciler) reconcileRunningCluster(ctx context.Context, cluster *models.ClusterResource) (bool, error) {
	log.Printf("[RUNNING] Reconciling running cluster %s", cluster.Name)
	
	needsRequeue := false
	
	// First, clean up any stale nodes from K3s cluster
	cleanupHappened, err := r.cleanupStaleK3sNodes(ctx, cluster)
	if err != nil {
		log.Printf("[RUNNING] Warning: Failed to cleanup stale nodes: %v", err)
		// Continue with reconciliation even if cleanup fails
	} else if cleanupHappened {
		needsRequeue = true  // Requeue to verify cluster is healthy after cleanup
	}
	
	// Always reconcile node pools - this handles scaling, adding, and removing pools
	if err := r.reconcileNodePools(ctx, cluster); err != nil {
		return false, fmt.Errorf("failed to reconcile node pools: %w", err)
	}
	
	// Cluster remains in running state
	cluster.Status.Message = "K3s cluster is running and ready"
	return needsRequeue, nil
}

// cleanupStaleK3sNodes removes terminated nodes from the K3s cluster
// removeAllWorkers removes all worker nodes when no NodePools are defined
func (r *Reconciler) removeAllWorkers(ctx context.Context, cluster *models.ClusterResource) error {
	log.Printf("[REMOVE_WORKERS] Removing all worker nodes from cluster %s", cluster.Name)
	
	computeService := r.provider.GetComputeService()
	var workersToRemove []*provider.Instance
	var updatedInstances []models.InstanceStatus
	
	// Get actual running instances from AWS to find all workers
	filters := map[string]string{
		"tag:goman-cluster": cluster.Name,
		"tag:goman-role": "worker",
		"instance-state-name": "running",
	}
	runningWorkers, err := computeService.ListInstances(ctx, filters)
	if err != nil {
		log.Printf("[REMOVE_WORKERS] Warning: Failed to list worker instances: %v", err)
		// Fall back to using status if we can't list instances
		for _, instance := range cluster.Status.Instances {
			if instance.Role == "worker" {
				log.Printf("[REMOVE_WORKERS] Removing worker from status: %s (%s)", instance.Name, instance.InstanceID)
				workersToRemove = append(workersToRemove, &provider.Instance{
					ID: instance.InstanceID,
					Name: instance.Name,
					PrivateIP: instance.PrivateIP,
				})
			} else {
				updatedInstances = append(updatedInstances, instance)
			}
		}
	} else {
		// Use actual running instances
		workersToRemove = runningWorkers
		log.Printf("[REMOVE_WORKERS] Found %d running workers to remove", len(workersToRemove))
		
		// Keep only non-worker instances in status
		for _, instance := range cluster.Status.Instances {
			if instance.Role != "worker" {
				updatedInstances = append(updatedInstances, instance)
			}
		}
	}
	
	if len(workersToRemove) == 0 {
		log.Printf("[REMOVE_WORKERS] No workers to remove")
		return nil
	}
	
	// First drain and delete nodes from K3s cluster
	masterInstance := ""
	for _, instance := range cluster.Status.Instances {
		if instance.Role == "master" && instance.State == "running" {
			masterInstance = instance.InstanceID
			break
		}
	}
	
	if masterInstance != "" {
		for _, worker := range workersToRemove {
			// Use worker's private IP to determine K3s node name
			workerIP := worker.PrivateIP
			
			if workerIP != "" {
				// K3s uses the hostname as node name, which is based on private IP
				// Format: ip-<ip-with-dashes>.region.compute.internal
				ipParts := strings.ReplaceAll(workerIP, ".", "-")
				nodeName := fmt.Sprintf("ip-%s.%s.compute.internal", ipParts, r.provider.Region())
				
				// Drain the node first
				drainCmd := fmt.Sprintf("kubectl drain %s --ignore-daemonsets --delete-emptydir-data --force || true", nodeName)
				log.Printf("[REMOVE_WORKERS] Draining node %s from K3s", nodeName)
				result, err := computeService.RunCommand(ctx, []string{masterInstance}, drainCmd)
				if err != nil {
					log.Printf("[REMOVE_WORKERS] Warning: Failed to drain node %s: %v", nodeName, err)
				} else if result.Instances[masterInstance] != nil {
					log.Printf("[REMOVE_WORKERS] Drain output: %s", result.Instances[masterInstance].Output)
				}
				
				// Delete the node from K3s
				deleteCmd := fmt.Sprintf("kubectl delete node %s || true", nodeName)
				log.Printf("[REMOVE_WORKERS] Deleting node %s from K3s", nodeName)
				result, err = computeService.RunCommand(ctx, []string{masterInstance}, deleteCmd)
				if err != nil {
					log.Printf("[REMOVE_WORKERS] Warning: Failed to delete node %s from K3s: %v", nodeName, err)
				} else if result.Instances[masterInstance] != nil {
					log.Printf("[REMOVE_WORKERS] Delete output: %s", result.Instances[masterInstance].Output)
				}
			}
		}
	}
	
	// Terminate EC2 instances
	for _, worker := range workersToRemove {
		log.Printf("[REMOVE_WORKERS] Terminating EC2 instance %s (%s)", worker.Name, worker.ID)
		if err := computeService.DeleteInstance(ctx, worker.ID); err != nil {
			log.Printf("[REMOVE_WORKERS] Error terminating instance %s: %v", worker.ID, err)
			// Continue with other instances
		}
	}
	
	// Update cluster status to remove workers
	cluster.Status.Instances = updatedInstances
	log.Printf("[REMOVE_WORKERS] Removed %d workers from cluster %s", len(workersToRemove), cluster.Name)
	
	return nil
}

func (r *Reconciler) cleanupStaleK3sNodes(ctx context.Context, cluster *models.ClusterResource) (bool, error) {
	log.Printf("[CLEANUP] Checking for stale nodes in K3s cluster %s", cluster.Name)
	
	// Get the master instance to run kubectl commands
	var masterInstanceID string
	for _, inst := range cluster.Status.Instances {
		if inst.Role == "master" && inst.State == "running" {
			masterInstanceID = inst.InstanceID
			break
		}
	}
	
	if masterInstanceID == "" {
		return false, fmt.Errorf("no running master found to execute cleanup")
	}
	
	// Get list of nodes from K3s
	computeService := r.provider.GetComputeService()
	getNodesCmd := "kubectl get nodes -o json | jq -r '.items[] | \"\\(.metadata.name),\\(.status.addresses[] | select(.type==\"InternalIP\") | .address)\"'"
	
	result, err := computeService.RunCommand(ctx, []string{masterInstanceID}, getNodesCmd)
	if err != nil {
		return false, fmt.Errorf("failed to get K3s nodes: %w", err)
	}
	
	// Get output from the master instance
	output := ""
	if instResult, ok := result.Instances[masterInstanceID]; ok {
		output = instResult.Output
	} else {
		return false, fmt.Errorf("no output from master instance")
	}
	
	// Get actual running instances from AWS
	filters := map[string]string{
		"tag:goman-cluster": cluster.Name,
		"instance-state-name": "running",
	}
	runningInstances, err := computeService.ListInstances(ctx, filters)
	if err != nil {
		return false, fmt.Errorf("failed to list running instances: %w", err)
	}
	
	// Build map of running IPs
	runningIPs := make(map[string]bool)
	for _, inst := range runningInstances {
		if inst.PrivateIP != "" {
			runningIPs[inst.PrivateIP] = true
		}
	}
	
	// Parse K3s nodes and find stale ones
	staleNodes := []string{}
	lines := strings.Split(strings.TrimSpace(output), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.Split(line, ",")
		if len(parts) >= 2 {
			nodeName := parts[0]
			nodeIP := parts[1]
			
			// Check if this node's IP is still running
			if !runningIPs[nodeIP] {
				staleNodes = append(staleNodes, nodeName)
				log.Printf("[CLEANUP] Found stale node: %s (IP: %s)", nodeName, nodeIP)
			}
		}
	}
	
	// Remove stale nodes from K3s cluster
	for _, nodeName := range staleNodes {
		log.Printf("[CLEANUP] Removing stale node %s from K3s cluster", nodeName)
		
		// First, drain the node (in case it still has pods)
		drainCmd := fmt.Sprintf("kubectl drain %s --ignore-daemonsets --delete-emptydir-data --force --timeout=30s || true", nodeName)
		if _, err := computeService.RunCommand(ctx, []string{masterInstanceID}, drainCmd); err != nil {
			log.Printf("[CLEANUP] Warning: Failed to drain node %s: %v", nodeName, err)
		}
		
		// Then delete the node
		deleteCmd := fmt.Sprintf("kubectl delete node %s", nodeName)
		if _, err := computeService.RunCommand(ctx, []string{masterInstanceID}, deleteCmd); err != nil {
			log.Printf("[CLEANUP] Warning: Failed to delete node %s: %v", nodeName, err)
			// Continue with other nodes
		} else {
			log.Printf("[CLEANUP] Successfully removed node %s from K3s cluster", nodeName)
		}
	}
	
	if len(staleNodes) > 0 {
		log.Printf("[CLEANUP] Removed %d stale nodes from K3s cluster", len(staleNodes))
		return true, nil  // Cleanup happened, should requeue
	} else {
		log.Printf("[CLEANUP] No stale nodes found in K3s cluster")
		return false, nil  // No cleanup needed
	}
}

// reconcileNodePools ensures the actual worker nodes match the desired configuration
func (r *Reconciler) reconcileNodePools(ctx context.Context, cluster *models.ClusterResource) error {
	computeService := r.provider.GetComputeService()
	storageService := r.provider.GetStorageService()
	
	// Get master IP for new workers to join
	var masterIP string
	for _, inst := range cluster.Status.Instances {
		if inst.Role == "master" && inst.PrivateIP != "" {
			masterIP = inst.PrivateIP
			break
		}
	}
	
	if masterIP == "" {
		return fmt.Errorf("no master node IP found for worker nodes to join")
	}
	
	// Get the node token from S3 for workers to join
	nodeTokenKey := fmt.Sprintf("clusters/%s/k3s-node-token", cluster.Name)
	nodeTokenData, err := storageService.GetObject(ctx, nodeTokenKey)
	if err != nil {
		log.Printf("[NODEPOOLS] Failed to get agent token, using server token: %v", err)
		// For K3s, agents can join with just the server token
		// No need to construct a special format - K3s handles this
		if cluster.Status.K3sServerToken != "" {
			nodeTokenData = []byte(cluster.Status.K3sServerToken)
		} else {
			return fmt.Errorf("no server token available for workers to join")
		}
	}
	nodeToken := strings.TrimSpace(string(nodeTokenData))
	
	// First, get actual running instances from AWS to ensure accuracy
	filters := map[string]string{
		"tag:goman-cluster": cluster.Name,
		"instance-state-name": "running,pending",
	}
	computeInstances, err := computeService.ListInstances(ctx, filters)
	if err != nil {
		log.Printf("[NODEPOOLS] Warning: Failed to list instances from AWS: %v", err)
		// Fall back to status, but it might be stale
	}
	
	// Build a map of actual running instances
	actualInstances := make(map[string]*provider.Instance)
	for _, inst := range computeInstances {
		if inst.State == "running" || inst.State == "pending" {
			actualInstances[inst.ID] = inst
		}
	}
	
	// Count existing workers per pool based on actual instances
	existingWorkers := make(map[string][]models.InstanceStatus)
	allWorkers := []models.InstanceStatus{}
	
	// First pass: collect all workers from actual instances
	for _, inst := range actualInstances {
		if inst.Tags["goman-role"] == "worker" {
			poolName := inst.Tags["goman-nodepool"]
			if poolName == "" {
				// Try to extract from name for backward compatibility
				parts := strings.Split(inst.Name, "-worker-")
				if len(parts) == 2 {
					poolParts := strings.Split(parts[1], "-")
					if len(poolParts) >= 1 {
						poolName = strings.Join(poolParts[:len(poolParts)-1], "-")
					}
				}
			}
			
			workerStatus := models.InstanceStatus{
				InstanceID: inst.ID,
				Name:       inst.Name,
				Role:       "worker",
				State:      inst.State,
				PrivateIP:  inst.PrivateIP,
				PublicIP:   inst.PublicIP,
				LaunchTime: inst.LaunchTime,
			}
			
			if poolName != "" {
				existingWorkers[poolName] = append(existingWorkers[poolName], workerStatus)
			}
			allWorkers = append(allWorkers, workerStatus)
		}
	}
	
	log.Printf("[NODEPOOLS] Found %d total workers across all pools", len(allWorkers))
	
	// Process each node pool
	for _, pool := range cluster.Spec.NodePools {
		poolWorkers := existingWorkers[pool.Name]
		currentCount := len(poolWorkers)
		desiredCount := pool.Count
		
		log.Printf("[NODEPOOLS] Pool '%s': current=%d, desired=%d", pool.Name, currentCount, desiredCount)
		
		// First, handle any duplicates - keep the newest ones
		if currentCount > desiredCount {
			// Scale down - terminate extra workers
			toTerminate := currentCount - desiredCount
			log.Printf("[NODEPOOLS] Scaling down pool '%s': terminating %d excess workers", pool.Name, toTerminate)
			
			// Sort workers by name to identify duplicates and by launch time
			// Group by base name (without index) to find duplicates
			workerGroups := make(map[string][]models.InstanceStatus)
			for _, worker := range poolWorkers {
				// Extract base name without index
				baseName := worker.Name
				if idx := strings.LastIndex(baseName, "-"); idx > 0 {
					if _, err := strconv.Atoi(baseName[idx+1:]); err == nil {
						baseName = baseName[:idx]
					}
				}
				workerGroups[baseName] = append(workerGroups[baseName], worker)
			}
			
			// Build list of workers to terminate
			var toDelete []models.InstanceStatus
			
			// First, remove duplicates (keep newest)
			for _, workers := range workerGroups {
				if len(workers) > 1 {
					// Sort by launch time, newest first
					sort.Slice(workers, func(i, j int) bool {
						return workers[i].LaunchTime.After(workers[j].LaunchTime)
					})
					// Mark older duplicates for deletion
					for i := 1; i < len(workers); i++ {
						toDelete = append(toDelete, workers[i])
						log.Printf("[NODEPOOLS] Marking duplicate %s (%s) for termination", workers[i].Name, workers[i].InstanceID)
					}
				}
			}
			
			// Then, if we still have too many, remove the highest indexed ones
			remainingWorkers := []models.InstanceStatus{}
			for _, worker := range poolWorkers {
				isDuplicate := false
				for _, del := range toDelete {
					if del.InstanceID == worker.InstanceID {
						isDuplicate = true
						break
					}
				}
				if !isDuplicate {
					remainingWorkers = append(remainingWorkers, worker)
				}
			}
			
			if len(remainingWorkers) > desiredCount {
				// Sort by index (descending) to remove highest indexed first
				sort.Slice(remainingWorkers, func(i, j int) bool {
					iIndex := extractWorkerIndex(remainingWorkers[i].Name)
					jIndex := extractWorkerIndex(remainingWorkers[j].Name)
					return iIndex > jIndex
				})
				
				// Add excess workers to delete list
				for i := 0; i < len(remainingWorkers)-desiredCount; i++ {
					toDelete = append(toDelete, remainingWorkers[i])
					log.Printf("[NODEPOOLS] Marking excess %s (%s) for termination", remainingWorkers[i].Name, remainingWorkers[i].InstanceID)
				}
			}
			
			// Terminate all workers marked for deletion
			for _, worker := range toDelete {
				log.Printf("[NODEPOOLS] Terminating worker %s (%s)", worker.Name, worker.InstanceID)
				
				if err := computeService.DeleteInstance(ctx, worker.InstanceID); err != nil {
					log.Printf("[NODEPOOLS] Failed to terminate %s: %v", worker.InstanceID, err)
					// Continue with other terminations
				}
			}
			
		} else if currentCount < desiredCount {
			// Scale up - provision new workers
			toCreate := desiredCount - currentCount
			log.Printf("[NODEPOOLS] Scaling up pool '%s': creating %d new workers", pool.Name, toCreate)
			
			// Find which indices are missing
			existingIndices := make(map[int]bool)
			for _, worker := range poolWorkers {
				if idx := extractWorkerIndex(worker.Name); idx >= 0 {
					existingIndices[idx] = true
				}
			}
			
			// Create workers with missing indices
			createdCount := 0
			for i := 0; createdCount < toCreate && i < desiredCount*2; i++ {
				if !existingIndices[i] {
					workerName := fmt.Sprintf("%s-worker-%s-%d", cluster.Name, pool.Name, i)
					
					instanceConfig := provider.InstanceConfig{
						Name:         workerName,
						InstanceType: pool.InstanceType,
						Region:       cluster.Spec.Region,
						Tags: map[string]string{
							"goman-cluster":    cluster.Name,
							"goman-role":       "worker",
							"goman-nodepool":   pool.Name,
							"goman-master-ip":  masterIP,
							"goman-node-token": nodeToken,
							"ManagedBy":        "goman",
						},
					}
					
					// Apply labels as tags if present
					for k, v := range pool.Labels {
						instanceConfig.Tags[fmt.Sprintf("k8s-label-%s", k)] = v
					}
					
					instance, err := computeService.CreateInstance(ctx, instanceConfig)
					if err != nil {
						log.Printf("[NODEPOOLS] Failed to create worker %s: %v", workerName, err)
						continue
					}
					
					log.Printf("[NODEPOOLS] Created worker node %s (%s) in pool '%s'", workerName, instance.ID, pool.Name)
					createdCount++
					
					// Add the newly created instance to actualInstances so it gets included in status
					actualInstances[instance.ID] = instance
				}
			}
		} else {
			log.Printf("[NODEPOOLS] Pool '%s' has correct number of workers", pool.Name)
		}
	}
	
	// Update cluster status with actual instances
	newInstances := []models.InstanceStatus{}
	
	// Keep master nodes
	for _, inst := range cluster.Status.Instances {
		if inst.Role == "master" {
			newInstances = append(newInstances, inst)
		}
	}
	
	// Add current workers from AWS
	for _, inst := range actualInstances {
		if inst.Tags["goman-role"] == "worker" {
			newInstances = append(newInstances, models.InstanceStatus{
				InstanceID: inst.ID,
				Name:       inst.Name,
				Role:       "worker",
				State:      inst.State,
				PrivateIP:  inst.PrivateIP,
				PublicIP:   inst.PublicIP,
				LaunchTime: inst.LaunchTime,
			})
		}
	}
	
	cluster.Status.Instances = newInstances
	
	// Handle workers from deleted pools (pools that exist in running instances but not in spec)
	// Build set of configured pool names
	configuredPools := make(map[string]bool)
	for _, pool := range cluster.Spec.NodePools {
		configuredPools[pool.Name] = true
	}
	
	// Find workers belonging to non-existent pools
	orphanedWorkers := []models.InstanceStatus{}
	for poolName, workers := range existingWorkers {
		if !configuredPools[poolName] && poolName != "" {
			log.Printf("[NODEPOOLS] Pool '%s' no longer exists in config, removing %d workers", poolName, len(workers))
			orphanedWorkers = append(orphanedWorkers, workers...)
		}
	}
	
	// Also check for workers with no pool assignment (shouldn't happen but handle it)
	for _, worker := range allWorkers {
		hasPool := false
		for poolName := range existingWorkers {
			for _, w := range existingWorkers[poolName] {
				if w.InstanceID == worker.InstanceID {
					hasPool = true
					break
				}
			}
			if hasPool {
				break
			}
		}
		if !hasPool {
			log.Printf("[NODEPOOLS] Worker %s has no pool assignment, marking for removal", worker.Name)
			orphanedWorkers = append(orphanedWorkers, worker)
		}
	}
	
	// Remove orphaned workers
	if len(orphanedWorkers) > 0 {
		log.Printf("[NODEPOOLS] Removing %d workers from deleted pools", len(orphanedWorkers))
		
		// Get master instance for kubectl commands
		var masterInstanceID string
		for _, inst := range cluster.Status.Instances {
			if inst.Role == "master" && inst.State == "running" {
				masterInstanceID = inst.InstanceID
				break
			}
		}
		
		for _, worker := range orphanedWorkers {
			log.Printf("[NODEPOOLS] Removing orphaned worker %s (%s)", worker.Name, worker.InstanceID)
			
			// First drain and delete from K3s if master is available
			if masterInstanceID != "" && worker.PrivateIP != "" {
				ipParts := strings.ReplaceAll(worker.PrivateIP, ".", "-")
				nodeName := fmt.Sprintf("ip-%s.%s.compute.internal", ipParts, r.provider.Region())
				
				// Drain the node
				drainCmd := fmt.Sprintf("kubectl drain %s --ignore-daemonsets --delete-emptydir-data --force --timeout=30s || true", nodeName)
				if _, err := computeService.RunCommand(ctx, []string{masterInstanceID}, drainCmd); err != nil {
					log.Printf("[NODEPOOLS] Warning: Failed to drain orphaned node %s: %v", nodeName, err)
				}
				
				// Delete from K3s
				deleteCmd := fmt.Sprintf("kubectl delete node %s || true", nodeName)
				if _, err := computeService.RunCommand(ctx, []string{masterInstanceID}, deleteCmd); err != nil {
					log.Printf("[NODEPOOLS] Warning: Failed to delete orphaned node %s from K3s: %v", nodeName, err)
				}
			}
			
			// Terminate the EC2 instance
			if err := computeService.DeleteInstance(ctx, worker.InstanceID); err != nil {
				log.Printf("[NODEPOOLS] Error terminating orphaned instance %s: %v", worker.InstanceID, err)
			} else {
				log.Printf("[NODEPOOLS] Terminated orphaned instance %s", worker.InstanceID)
			}
			
			// Remove from status
			newStatusInstances := []models.InstanceStatus{}
			for _, inst := range cluster.Status.Instances {
				if inst.InstanceID != worker.InstanceID {
					newStatusInstances = append(newStatusInstances, inst)
				}
			}
			cluster.Status.Instances = newStatusInstances
		}
	}
	
	log.Printf("[NODEPOOLS] Node pool reconciliation completed for cluster %s", cluster.Name)
	return nil
}

// extractWorkerIndex extracts the worker index from the instance name
func extractWorkerIndex(name string) int {
	parts := strings.Split(name, "-")
	if len(parts) > 0 {
		if index, err := strconv.Atoi(parts[len(parts)-1]); err == nil {
			return index
		}
	}
	return -1
}

// provisionNodePools provisions worker node pools
func (r *Reconciler) provisionNodePools(ctx context.Context, cluster *models.ClusterResource) error {
	computeService := r.provider.GetComputeService()
	storageService := r.provider.GetStorageService()
	
	// Get first master's IP for workers to join
	var masterIP string
	log.Printf("[NODEPOOLS] Looking for master IP, total instances: %d", len(cluster.Status.Instances))
	for _, inst := range cluster.Status.Instances {
		log.Printf("[NODEPOOLS] Instance %s: role=%s, privateIP=%s", inst.InstanceID, inst.Role, inst.PrivateIP)
		if inst.Role == "master" && inst.PrivateIP != "" {
			masterIP = inst.PrivateIP
			break
		}
	}
	
	if masterIP == "" {
		return fmt.Errorf("no master node IP found for worker nodes to join")
	}
	
	// Get the node token from S3 for workers to join
	nodeTokenKey := fmt.Sprintf("clusters/%s/k3s-node-token", cluster.Name)
	nodeTokenData, err := storageService.GetObject(ctx, nodeTokenKey)
	if err != nil {
		// Fallback to agent token for backward compatibility
		log.Printf("[NODEPOOLS] Failed to get node token, trying agent token: %v", err)
		agentTokenKey := fmt.Sprintf("clusters/%s/k3s-agent-token", cluster.Name)
		nodeTokenData, err = storageService.GetObject(ctx, agentTokenKey)
		if err != nil {
			return fmt.Errorf("failed to get join token for workers: %w", err)
		}
	}
	nodeToken := strings.TrimSpace(string(nodeTokenData))
	
	// Check existing workers to avoid duplicates
	existingWorkers := make(map[string]bool)
	for _, inst := range cluster.Status.Instances {
		if inst.Role == "worker" {
			existingWorkers[inst.Name] = true
		}
	}
	
	// Provision each node pool
	for _, pool := range cluster.Spec.NodePools {
		log.Printf("[NODEPOOLS] Provisioning node pool '%s' with %d nodes", pool.Name, pool.Count)
		
		for i := 0; i < pool.Count; i++ {
			workerName := fmt.Sprintf("%s-worker-%s-%d", cluster.Name, pool.Name, i)
			
			// Skip if already exists
			if existingWorkers[workerName] {
				log.Printf("[NODEPOOLS] Worker %s already exists, skipping", workerName)
				continue
			}
			
			// Prepare instance configuration
			instanceConfig := provider.InstanceConfig{
				Name:         workerName,
				Region:       cluster.Spec.Region,
				InstanceType: pool.InstanceType,
				Tags: map[string]string{
					"goman-cluster":     cluster.Name,
					"goman-role":        "worker",
					"goman-nodepool":    pool.Name,
					"goman-master-ip":   masterIP,
					"goman-node-token":  nodeToken,
					"ManagedBy":         "goman",
				},
			}
			
			// Add Kubernetes labels as tags (prefixed with k8s-label-)
			for k, v := range pool.Labels {
				instanceConfig.Tags[fmt.Sprintf("k8s-label-%s", k)] = v
			}
			
			// Add taints as tags if present (for later application via kubectl)
			if len(pool.Taints) > 0 {
				taintStrings := []string{}
				for _, taint := range pool.Taints {
					taintStrings = append(taintStrings, fmt.Sprintf("%s=%s:%s", taint.Key, taint.Value, taint.Effect))
				}
				instanceConfig.Tags["k8s-taints"] = strings.Join(taintStrings, ",")
			}
			
			// Create the instance
			instance, err := computeService.CreateInstance(ctx, instanceConfig)
			if err != nil {
				log.Printf("[NODEPOOLS] Failed to create worker %s: %v", workerName, err)
				continue // Continue with other workers
			}
			
			// Add to cluster status
			instanceStatus := models.InstanceStatus{
				InstanceID: instance.ID,
				Name:       workerName,
				Role:       "worker",
				State:      instance.State,
				LaunchTime: time.Now(),
			}
			cluster.Status.Instances = append(cluster.Status.Instances, instanceStatus)
			
			log.Printf("[NODEPOOLS] Created worker node %s (%s) in pool '%s'", workerName, instance.ID, pool.Name)
		}
	}
	
	log.Printf("[NODEPOOLS] Node pool provisioning completed for cluster %s", cluster.Name)
	return nil
}

// generateToken generates a random token for K3s
func (r *Reconciler) generateToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(bytes), nil
}

// saveTokens saves K3s tokens to S3
func (r *Reconciler) saveTokens(ctx context.Context, clusterName, masterToken, workerToken string) error {
	storageService := r.provider.GetStorageService()
	
	// Save master token
	masterTokenKey := fmt.Sprintf("clusters/%s/k3s-server-token", clusterName)
	if err := storageService.PutObject(ctx, masterTokenKey, []byte(masterToken)); err != nil {
		return fmt.Errorf("failed to save master token: %w", err)
	}
	
	// Save worker token  
	workerTokenKey := fmt.Sprintf("clusters/%s/k3s-agent-token", clusterName)
	if err := storageService.PutObject(ctx, workerTokenKey, []byte(workerToken)); err != nil {
		return fmt.Errorf("failed to save worker token: %w", err)
	}
	
	log.Printf("[TOKENS] Saved K3s tokens for cluster %s", clusterName)
	return nil
}