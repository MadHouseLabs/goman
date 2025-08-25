package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/madhouselabs/goman/pkg/config"
	"github.com/madhouselabs/goman/pkg/controller"
	"github.com/madhouselabs/goman/pkg/provider/aws"
)

func main() {
	// Set up detailed logging
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

	// Get cluster name from args or use default
	clusterName := "k3s-cluster-1756046400"
	if len(os.Args) > 1 {
		clusterName = os.Args[1]
	}

	log.Printf("Starting local reconciler test for cluster: %s", clusterName)

	// Set up AWS provider
	profile := config.GetAWSProfile()
	region := config.GetAWSRegion()
	log.Printf("Using AWS profile: %s, region: %s", profile, region)

	provider, err := aws.GetCachedProvider(profile, region)
	if err != nil {
		log.Fatalf("Failed to get AWS provider: %v", err)
	}

	// Create reconciler
	reconciler, err := controller.NewReconciler(provider, "local-test")
	if err != nil {
		log.Fatalf("Failed to create reconciler: %v", err)
	}

	// Create context
	ctx := context.Background()

	// First, let's check what's in the config
	storageService := provider.GetStorageService()
	configData, err := storageService.GetObject(ctx, fmt.Sprintf("clusters/%s/config.yaml", clusterName))
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	log.Printf("=== RAW CONFIG FROM S3 ===\n%s\n=== END CONFIG ===", string(configData))
	
	// Run reconciliation
	log.Printf("Starting reconciliation for cluster: %s", clusterName)
	result, err := reconciler.ReconcileCluster(ctx, clusterName)
	
	if err != nil {
		log.Printf("Reconciliation error: %v", err)
	} else {
		log.Printf("Reconciliation completed successfully")
		log.Printf("Result: Requeue=%v, RequeueAfter=%v", result.Requeue, result.RequeueAfter)
	}

	// Check the final status
	statusData, err := storageService.GetObject(ctx, fmt.Sprintf("clusters/%s/status.yaml", clusterName))
	if err == nil {
		log.Printf("Final status:\n%s", string(statusData))
	}
}