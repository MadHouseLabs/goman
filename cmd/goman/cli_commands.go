package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/madhouselabs/goman/pkg/cluster"
	"github.com/madhouselabs/goman/pkg/models"
	"github.com/madhouselabs/goman/pkg/provider/aws"
	"github.com/madhouselabs/goman/pkg/setup"
)

// ClusterCreateOptions represents options for cluster creation
type ClusterCreateOptions struct {
	Name         string `json:"name"`
	Region       string `json:"region"`
	Mode         string `json:"mode"`
	InstanceType string `json:"instance_type"`
	Wait         bool   `json:"wait"`
	JSONOutput   bool   `json:"json_output"`
}

// handleClusterCreate handles cluster creation via CLI
func handleClusterCreate(args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: goman cluster create <name> [--region=<region>] [--mode=<mode>] [--instance-type=<type>] [--wait] [--json]")
		os.Exit(1)
	}

	opts := ClusterCreateOptions{
		Name:         args[0],
		Region:       "ap-south-1",
		Mode:         "developer",
		InstanceType: "t3.medium",
	}

	// Parse flags
	for i := 1; i < len(args); i++ {
		if strings.HasPrefix(args[i], "--region=") {
			opts.Region = strings.TrimPrefix(args[i], "--region=")
		} else if strings.HasPrefix(args[i], "--mode=") {
			opts.Mode = strings.TrimPrefix(args[i], "--mode=")
		} else if strings.HasPrefix(args[i], "--instance-type=") {
			opts.InstanceType = strings.TrimPrefix(args[i], "--instance-type=")
		} else if args[i] == "--wait" {
			opts.Wait = true
		} else if args[i] == "--json" {
			opts.JSONOutput = true
		}
	}

	// Create cluster
	manager := cluster.NewManager()

	clusterMode := models.ModeDeveloper
	if opts.Mode == "ha" {
		clusterMode = models.ModeHA
	}

	newCluster := models.K3sCluster{
		Name:         opts.Name,
		Region:       opts.Region,
		Mode:         clusterMode,
		InstanceType: opts.InstanceType,
		K3sVersion:   "v1.28.5+k3s1",
		NetworkCIDR:  "10.0.0.0/16",
		ServiceCIDR:  "10.43.0.0/16",
	}

	// Set master nodes based on mode
	masterCount := newCluster.GetMasterCount()
	for i := 0; i < masterCount; i++ {
		newCluster.MasterNodes = append(newCluster.MasterNodes, models.Node{
			Name:         fmt.Sprintf("master-%d", i+1),
			InstanceType: opts.InstanceType,
			CPU:          2,
			MemoryGB:     4,
			StorageGB:    20,
		})
	}

	if !opts.JSONOutput {
		fmt.Printf("Creating cluster '%s' in region %s...\n", opts.Name, opts.Region)
	}

	createdCluster, err := manager.CreateCluster(newCluster)
	if err != nil {
		if opts.JSONOutput {
			result := map[string]interface{}{
				"error": err.Error(),
			}
			json.NewEncoder(os.Stdout).Encode(result)
		} else {
			fmt.Printf("Error creating cluster: %v\n", err)
		}
		os.Exit(1)
	}

	if opts.Wait {
		// Wait for cluster to be ready
		if !opts.JSONOutput {
			fmt.Println("Waiting for cluster to be ready...")
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()

		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				if opts.JSONOutput {
					result := map[string]interface{}{
						"error": "timeout waiting for cluster",
					}
					json.NewEncoder(os.Stdout).Encode(result)
				} else {
					fmt.Println("Timeout waiting for cluster to be ready")
				}
				os.Exit(1)
			case <-ticker.C:
				manager.RefreshClusterStatus()
				clusters := manager.GetClusters()
				for _, c := range clusters {
					if c.ID == createdCluster.ID {
						if c.Status == models.StatusRunning {
							createdCluster = &c
							goto done
						} else if c.Status == models.StatusError {
							if opts.JSONOutput {
								result := map[string]interface{}{
									"error": "cluster creation failed",
								}
								json.NewEncoder(os.Stdout).Encode(result)
							} else {
								fmt.Println("Cluster creation failed")
							}
							os.Exit(1)
						}
						if !opts.JSONOutput {
							fmt.Printf("  Status: %s\n", c.Status)
						}
						break
					}
				}
			}
		}
	}
done:

	if opts.JSONOutput {
		result := map[string]interface{}{
			"cluster_id":   createdCluster.ID,
			"name":         createdCluster.Name,
			"region":       createdCluster.Region,
			"status":       createdCluster.Status,
			"mode":         createdCluster.Mode,
			"master_count": len(createdCluster.MasterNodes),
		}
		json.NewEncoder(os.Stdout).Encode(result)
	} else {
		fmt.Printf("Cluster '%s' created successfully (ID: %s)\n", createdCluster.Name, createdCluster.ID)
	}
}

// handleClusterList handles listing clusters
func handleClusterList(args []string) {
	var region string
	var jsonOutput bool

	// Parse flags
	for _, arg := range args {
		if strings.HasPrefix(arg, "--region=") {
			region = strings.TrimPrefix(arg, "--region=")
		} else if arg == "--json" {
			jsonOutput = true
		}
	}

	manager := cluster.NewManager()
	manager.RefreshClusterStatus()
	clusters := manager.GetClusters()

	// Filter by region if specified
	var filtered []models.K3sCluster
	for _, c := range clusters {
		if region == "" || c.Region == region {
			filtered = append(filtered, c)
		}
	}

	if jsonOutput {
		result := make([]map[string]interface{}, 0)
		for _, c := range filtered {
			result = append(result, map[string]interface{}{
				"cluster_id":   c.ID,
				"name":         c.Name,
				"region":       c.Region,
				"status":       c.Status,
				"mode":         c.Mode,
				"master_count": len(c.MasterNodes),
				"worker_count": len(c.WorkerNodes),
				"created_at":   c.CreatedAt,
			})
		}
		json.NewEncoder(os.Stdout).Encode(result)
	} else {
		if len(filtered) == 0 {
			fmt.Println("No clusters found")
			return
		}

		fmt.Println("Clusters:")
		fmt.Println("ID\t\t\tName\t\tRegion\t\tStatus\t\tMode")
		fmt.Println(strings.Repeat("-", 80))
		for _, c := range filtered {
			fmt.Printf("%s\t%s\t\t%s\t%s\t\t%s\n",
				c.ID, c.Name, c.Region, c.Status, c.Mode)
		}
	}
}

// handleClusterStatus handles getting cluster status
func handleClusterStatus(args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: goman cluster status <name> [--json]")
		os.Exit(1)
	}

	name := args[0]
	var jsonOutput bool

	for i := 1; i < len(args); i++ {
		if args[i] == "--json" {
			jsonOutput = true
		}
	}

	manager := cluster.NewManager()
	manager.RefreshClusterStatus()
	clusters := manager.GetClusters()

	var found *models.K3sCluster
	for _, c := range clusters {
		if c.Name == name {
			found = &c
			break
		}
	}

	if found == nil {
		if jsonOutput {
			result := map[string]interface{}{
				"error": "cluster not found",
			}
			json.NewEncoder(os.Stdout).Encode(result)
		} else {
			fmt.Printf("Cluster '%s' not found\n", name)
		}
		os.Exit(1)
	}

	if jsonOutput {
		result := map[string]interface{}{
			"cluster_id":    found.ID,
			"name":          found.Name,
			"region":        found.Region,
			"status":        found.Status,
			"mode":          found.Mode,
			"instance_type": found.InstanceType,
			"k3s_version":   found.K3sVersion,
			"master_nodes":  found.MasterNodes,
			"worker_nodes":  found.WorkerNodes,
			"created_at":    found.CreatedAt,
			"updated_at":    found.UpdatedAt,
		}
		json.NewEncoder(os.Stdout).Encode(result)
	} else {
		fmt.Printf("Cluster: %s\n", found.Name)
		fmt.Printf("ID: %s\n", found.ID)
		fmt.Printf("Status: %s\n", found.Status)
		fmt.Printf("Region: %s\n", found.Region)
		fmt.Printf("Mode: %s\n", found.Mode)
		fmt.Printf("Instance Type: %s\n", found.InstanceType)
		fmt.Printf("K3s Version: %s\n", found.K3sVersion)
		fmt.Printf("Masters: %d\n", len(found.MasterNodes))
		fmt.Printf("Workers: %d\n", len(found.WorkerNodes))
		fmt.Printf("Created: %s\n", found.CreatedAt.Format(time.RFC3339))
		fmt.Printf("Updated: %s\n", found.UpdatedAt.Format(time.RFC3339))

		if len(found.MasterNodes) > 0 {
			fmt.Println("\nMaster Nodes:")
			for _, node := range found.MasterNodes {
				fmt.Printf("  - %s: %s (IP: %s)\n", node.Name, node.Status, node.IP)
			}
		}

		if len(found.WorkerNodes) > 0 {
			fmt.Println("\nWorker Nodes:")
			for _, node := range found.WorkerNodes {
				fmt.Printf("  - %s: %s (IP: %s)\n", node.Name, node.Status, node.IP)
			}
		}
	}
}

// handleClusterDelete handles cluster deletion
func handleClusterDelete(args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: goman cluster delete <name> [--region=<region>] [--json]")
		os.Exit(1)
	}

	name := args[0]
	var region string
	var jsonOutput bool

	for i := 1; i < len(args); i++ {
		if strings.HasPrefix(args[i], "--region=") {
			region = strings.TrimPrefix(args[i], "--region=")
		} else if args[i] == "--json" {
			jsonOutput = true
		}
	}

	manager := cluster.NewManager()
	clusters := manager.GetClusters()

	var toDelete *models.K3sCluster
	for _, c := range clusters {
		if c.Name == name && (region == "" || c.Region == region) {
			toDelete = &c
			break
		}
	}

	if toDelete == nil {
		if jsonOutput {
			result := map[string]interface{}{
				"error": "cluster not found",
			}
			json.NewEncoder(os.Stdout).Encode(result)
		} else {
			fmt.Printf("Cluster '%s' not found", name)
			if region != "" {
				fmt.Printf(" in region %s", region)
			}
			fmt.Println()
		}
		os.Exit(1)
	}

	if !jsonOutput {
		fmt.Printf("Deleting cluster '%s' (ID: %s)...\n", toDelete.Name, toDelete.ID)
	}

	err := manager.DeleteCluster(toDelete.ID)
	if err != nil {
		if jsonOutput {
			result := map[string]interface{}{
				"error": err.Error(),
			}
			json.NewEncoder(os.Stdout).Encode(result)
		} else {
			fmt.Printf("Error deleting cluster: %v\n", err)
		}
		os.Exit(1)
	}

	if jsonOutput {
		result := map[string]interface{}{
			"deleted":    true,
			"cluster_id": toDelete.ID,
			"name":       toDelete.Name,
		}
		json.NewEncoder(os.Stdout).Encode(result)
	} else {
		fmt.Printf("Cluster '%s' marked for deletion\n", toDelete.Name)
	}
}

// handleResourcesList handles listing AWS resources
func handleResourcesList(args []string) {
	var region string
	var jsonOutput bool

	// Parse flags
	for _, arg := range args {
		if strings.HasPrefix(arg, "--region=") {
			region = strings.TrimPrefix(arg, "--region=")
		} else if arg == "--json" {
			jsonOutput = true
		}
	}

	if region == "" {
		region = os.Getenv("AWS_REGION")
		if region == "" {
			region = "ap-south-1"
		}
	}

	ctx := context.Background()

	// Initialize AWS provider
	profile := os.Getenv("AWS_PROFILE")
	if profile == "" {
		profile = "default"
	}
	provider, err := aws.NewProvider(profile, region)
	if err != nil {
		if jsonOutput {
			result := map[string]interface{}{
				"error": err.Error(),
			}
			json.NewEncoder(os.Stdout).Encode(result)
		} else {
			fmt.Printf("Error initializing provider: %v\n", err)
		}
		os.Exit(1)
	}

	// Get compute service to list instances
	computeService := provider.GetComputeService()

	// List instances with goman tags
	filters := map[string]string{
		"tag:ManagedBy": "goman",
	}

	instances, err := computeService.ListInstances(ctx, filters)
	if err != nil {
		if jsonOutput {
			result := map[string]interface{}{
				"error": err.Error(),
			}
			json.NewEncoder(os.Stdout).Encode(result)
		} else {
			fmt.Printf("Error listing instances: %v\n", err)
		}
		os.Exit(1)
	}

	if jsonOutput {
		result := map[string]interface{}{
			"region":    region,
			"instances": instances,
		}
		json.NewEncoder(os.Stdout).Encode(result)
	} else {
		fmt.Printf("AWS Resources in region %s:\n", region)
		fmt.Println("\nEC2 Instances:")
		if len(instances) == 0 {
			fmt.Println("  No instances found")
		} else {
			for _, inst := range instances {
				fmt.Printf("  - %s: %s (%s) - %s\n",
					inst.ID, inst.Name, inst.InstanceType, inst.State)
			}
		}

		// Check for S3 bucket
		storageService := provider.GetStorageService()
		if storageService != nil {
			fmt.Println("\nS3 Storage:")
			// The bucket name is account-specific
			fmt.Printf("  - Bucket: goman-state-<account-id>\n")
		}

		// Check for Lambda function
		functionService := provider.GetFunctionService()
		if functionService != nil {
			fmt.Println("\nLambda Functions:")
			fmt.Printf("  - Function: goman-controller-<account-id>\n")
		}

		// Check for DynamoDB table
		lockService := provider.GetLockService()
		if lockService != nil {
			fmt.Println("\nDynamoDB Tables:")
			fmt.Printf("  - Table: goman-locks\n")
		}
	}
}

// handleInitNonInteractive handles non-interactive initialization
func handleInitNonInteractive() {
	fmt.Println("Initializing Goman infrastructure...")

	ctx := context.Background()
	result, err := setup.EnsureFullSetup(ctx)
	if err != nil {
		fmt.Printf("Error during initialization: %v\n", err)
		os.Exit(1)
	}

	if !result.StorageReady || !result.FunctionReady || !result.LockServiceReady {
		fmt.Println("Initialization incomplete:")
		if !result.StorageReady {
			fmt.Println("  - Storage setup failed")
		}
		if !result.FunctionReady {
			fmt.Println("  - Function setup failed")
		}
		if !result.LockServiceReady {
			fmt.Println("  - Lock service setup failed")
		}
		for _, errMsg := range result.Errors {
			fmt.Printf("  Error: %s\n", errMsg)
		}
		os.Exit(1)
	}

	// Save initialization status
	err = saveInitStatus(result)
	if err != nil {
		fmt.Printf("Warning: Failed to save init status: %v\n", err)
	}

	fmt.Println("✓ Goman infrastructure initialized successfully")
	fmt.Println("  - S3 Bucket: Ready")
	fmt.Println("  - Lambda Function: Deployed")
	fmt.Println("  - DynamoDB Table: Created")
	fmt.Println("  - IAM Roles: Configured")
}
