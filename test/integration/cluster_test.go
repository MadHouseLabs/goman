package integration

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/madhouselabs/goman/pkg/cluster"
	"github.com/madhouselabs/goman/pkg/models"
	"github.com/madhouselabs/goman/pkg/provider/registry"
)

func TestClusterCreation(t *testing.T) {
	// Skip if not in integration test mode
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run")
	}
	
	// Ensure AWS credentials are configured
	if os.Getenv("AWS_PROFILE") == "" && os.Getenv("AWS_ACCESS_KEY_ID") == "" {
		t.Fatal("AWS credentials not configured. Set AWS_PROFILE or AWS_ACCESS_KEY_ID")
	}
	
	// Create cluster manager
	manager := cluster.NewManager()
	
	// Create a test cluster
	testCluster := models.K3sCluster{
		Name:        fmt.Sprintf("test-%d", time.Now().Unix()),
		Provider:    "AWS",
		Region:      "ap-south-1",
		K3sVersion:  "v1.28.5+k3s1",
		NetworkCIDR: "10.0.0.0/16",
		MasterNodes: []models.Node{
			{
				Name:         "master-1",
				Role:         models.NodeRoleMaster,
				InstanceType: "t3.micro", // Use smallest instance for testing
				CPU:          1,
				MemoryGB:     1,
				StorageGB:    8,
				State:        models.NodeStatePending,
			},
		},
		WorkerNodes: []models.Node{},
		Tags: map[string]string{
			"Environment": "test",
			"TestRun":     "integration",
		},
	}
	
	// Create the cluster
	createdCluster, err := manager.CreateCluster(testCluster)
	if err != nil {
		t.Fatalf("Failed to create cluster: %v", err)
	}
	
	t.Logf("Cluster created with ID: %s", createdCluster.ID)
	
	// Wait for cluster to be ready (with timeout)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			t.Fatal("Timeout waiting for cluster to be ready")
		case <-ticker.C:
			manager.RefreshClusterStatus()
			clusters := manager.GetClusters()
			
			for _, c := range clusters {
				if c.ID == createdCluster.ID {
					t.Logf("Cluster status: %s", c.Status)
					
					if c.Status == models.StatusRunning {
						t.Log("Cluster is running!")
						
						// Clean up - delete the cluster
						t.Log("Cleaning up - deleting cluster")
						err := manager.DeleteCluster(c.ID)
						if err != nil {
							t.Errorf("Failed to delete cluster: %v", err)
						}
						return
					} else if c.Status == models.StatusError {
						t.Fatal("Cluster creation failed")
					}
					break
				}
			}
		}
	}
}

func TestProviderInitialization(t *testing.T) {
	// Test that provider can be initialized
	provider, err := registry.GetDefaultProvider()
	if err != nil {
		t.Fatalf("Failed to get default provider: %v", err)
	}
	
	if provider == nil {
		t.Fatal("Provider is nil")
	}
	
	t.Logf("Provider initialized: %s", provider.Name())
	t.Logf("Region: %s", provider.Region())
	
	// Test that services are available
	if provider.GetStorageService() == nil {
		t.Error("Storage service is nil")
	}
	
	if provider.GetLockService() == nil {
		t.Error("Lock service is nil")
	}
	
	if provider.GetComputeService() == nil {
		t.Error("Compute service is nil")
	}
	
	if provider.GetFunctionService() == nil {
		t.Error("Function service is nil")
	}
	
	if provider.GetNotificationService() == nil {
		t.Error("Notification service is nil")
	}
}

func TestStorageService(t *testing.T) {
	// Skip if not in integration test mode
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run")
	}
	
	provider, err := registry.GetDefaultProvider()
	if err != nil {
		t.Fatalf("Failed to get provider: %v", err)
	}
	
	ctx := context.Background()
	storageService := provider.GetStorageService()
	
	// Initialize storage service
	err = storageService.Initialize(ctx)
	if err != nil {
		t.Fatalf("Failed to initialize storage service: %v", err)
	}
	
	// Test write and read
	testKey := fmt.Sprintf("test/integration-%d.txt", time.Now().Unix())
	testData := []byte("Hello from integration test")
	
	// Write data
	err = storageService.PutObject(ctx, testKey, testData)
	if err != nil {
		t.Fatalf("Failed to write object: %v", err)
	}
	
	// Read data back
	readData, err := storageService.GetObject(ctx, testKey)
	if err != nil {
		t.Fatalf("Failed to read object: %v", err)
	}
	
	if string(readData) != string(testData) {
		t.Errorf("Data mismatch. Expected: %s, Got: %s", testData, readData)
	}
	
	// Clean up
	err = storageService.DeleteObject(ctx, testKey)
	if err != nil {
		t.Errorf("Failed to delete test object: %v", err)
	}
	
	t.Log("Storage service test passed")
}

func TestLockService(t *testing.T) {
	// Skip if not in integration test mode
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run")
	}
	
	provider, err := registry.GetDefaultProvider()
	if err != nil {
		t.Fatalf("Failed to get provider: %v", err)
	}
	
	ctx := context.Background()
	lockService := provider.GetLockService()
	
	// Initialize lock service
	err = lockService.Initialize(ctx)
	if err != nil {
		t.Fatalf("Failed to initialize lock service: %v", err)
	}
	
	resourceID := fmt.Sprintf("test-resource-%d", time.Now().Unix())
	owner := "test-owner"
	
	// Acquire lock
	token, err := lockService.AcquireLock(ctx, resourceID, owner, 1*time.Minute)
	if err != nil {
		t.Fatalf("Failed to acquire lock: %v", err)
	}
	
	t.Logf("Lock acquired with token: %s", token)
	
	// Check if locked
	locked, lockOwner, err := lockService.IsLocked(ctx, resourceID)
	if err != nil {
		t.Fatalf("Failed to check lock status: %v", err)
	}
	
	if !locked {
		t.Error("Resource should be locked")
	}
	
	if lockOwner != owner {
		t.Errorf("Lock owner mismatch. Expected: %s, Got: %s", owner, lockOwner)
	}
	
	// Try to acquire same lock (should fail)
	_, err = lockService.AcquireLock(ctx, resourceID, "another-owner", 1*time.Minute)
	if err == nil {
		t.Error("Should not be able to acquire already locked resource")
	}
	
	// Release lock
	err = lockService.ReleaseLock(ctx, resourceID, token)
	if err != nil {
		t.Fatalf("Failed to release lock: %v", err)
	}
	
	// Check if unlocked
	locked, _, err = lockService.IsLocked(ctx, resourceID)
	if err != nil {
		t.Fatalf("Failed to check lock status after release: %v", err)
	}
	
	if locked {
		t.Error("Resource should be unlocked after release")
	}
	
	t.Log("Lock service test passed")
}