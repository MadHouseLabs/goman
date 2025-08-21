package main

import (
	"context"
	"fmt"
	"os"

	"github.com/madhouselabs/goman/pkg/config"
	"github.com/madhouselabs/goman/pkg/provider/registry"
)

// initializeInfrastructure initializes AWS infrastructure
func initializeInfrastructure() {
	fmt.Println("Initializing AWS infrastructure...")
	
	// Load configuration
	cfg, err := config.NewConfig()
	if err != nil {
		fmt.Printf("Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Get AWS provider
	provider, err := registry.GetProvider("aws", cfg.AWSProfile, cfg.AWSRegion)
	if err != nil {
		fmt.Printf("Error getting AWS provider: %v\n", err)
		os.Exit(1)
	}

	// Initialize infrastructure
	ctx := context.Background()
	result, err := provider.Initialize(ctx)
	if err != nil {
		fmt.Printf("❌ Error initializing infrastructure: %v\n", err)
		os.Exit(1)
	}

	// Show initialization result details
	if result != nil {
		fmt.Println("\n📋 Initialization Status:")
		fmt.Printf("  Storage: %v\n", result.StorageReady)
		fmt.Printf("  Lock Service: %v\n", result.LockServiceReady)
		fmt.Printf("  Lambda Function: %v\n", result.FunctionReady)
		fmt.Printf("  Notifications: %v\n", result.NotificationsReady)
		
		if len(result.Resources) > 0 {
			fmt.Println("\n📦 Resources Created:")
			for key, value := range result.Resources {
				fmt.Printf("  %s: %s\n", key, value)
			}
		}
		
		if len(result.Errors) > 0 {
			fmt.Println("\n⚠️  Warnings:")
			for _, err := range result.Errors {
				fmt.Printf("  - %s\n", err)
			}
		}
	}

	fmt.Println("\n✅ Infrastructure initialized successfully!")
}

// forceCleanupCluster removes all AWS resources for a cluster
func forceCleanupCluster(clusterName string) {
	fmt.Printf("🗑️  Force cleaning up cluster '%s'...\n", clusterName)
	
	// Load configuration
	cfg, err := config.NewConfig()
	if err != nil {
		fmt.Printf("Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Get AWS provider
	provider, err := registry.GetProvider("aws", cfg.AWSProfile, cfg.AWSRegion)
	if err != nil {
		fmt.Printf("Error getting AWS provider: %v\n", err)
		os.Exit(1)
	}

	// Force cleanup all cluster resources
	ctx := context.Background()
	if err := provider.Cleanup(ctx); err != nil {
		fmt.Printf("Error during force cleanup: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✅ Cluster '%s' forcefully cleaned up!\n", clusterName)
}