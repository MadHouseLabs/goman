package main

import (
	"context"
	"flag"
	"log"
	"os"
	"time"

	"github.com/madhouselabs/goman/pkg/config"
	"github.com/madhouselabs/goman/pkg/controller"
	"github.com/madhouselabs/goman/pkg/provider/aws"
	"github.com/madhouselabs/goman/pkg/storage"
	"gopkg.in/yaml.v3"
)

func main() {
	// Parse command line flags
	var clusterName string
	var statusFile string
	flag.StringVar(&clusterName, "cluster", "k3s-cluster-1756100915", "Cluster name to reconcile")
	flag.StringVar(&statusFile, "status-file", "", "Optional status file to load")
	flag.Parse()

	ctx := context.Background()
	
	// Get provider
	profile := config.GetAWSProfile()
	region := config.GetAWSRegion()
	provider, err := aws.GetCachedProvider(profile, region)
	if err != nil {
		log.Fatalf("Failed to get AWS provider: %v", err)
	}

	// Create reconciler
	reconciler, err := controller.NewReconciler(provider, "debug-test")
	if err != nil {
		log.Fatalf("Failed to create reconciler: %v", err)
	}

	// If status file is provided, update the status in storage first
	if statusFile != "" {
		log.Printf("Loading status from file: %s", statusFile)
		statusData, err := os.ReadFile(statusFile)
		if err != nil {
			log.Fatalf("Failed to read status file: %v", err)
		}
		
		var status storage.ClusterStatus
		if err := yaml.Unmarshal(statusData, &status); err != nil {
			log.Fatalf("Failed to parse status file: %v", err)
		}
		
		// Write the status to storage
		storageService := provider.GetStorageService()
		statusKey := "clusters/" + clusterName + "/status.yaml"
		if err := storageService.PutObject(ctx, statusKey, statusData); err != nil {
			log.Fatalf("Failed to write status to storage: %v", err)
		}
		log.Printf("Updated status in storage")
	}

	// Run reconciliation for the cluster
	log.Printf("Starting reconciliation for cluster: %s", clusterName)
	
	// Set a timeout for reconciliation
	ctxWithTimeout, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	
	result, err := reconciler.ReconcileCluster(ctxWithTimeout, clusterName)
	if err != nil {
		log.Printf("Reconciliation failed: %v", err)
	} else {
		log.Printf("Reconciliation completed successfully: %v", result)
	}
}