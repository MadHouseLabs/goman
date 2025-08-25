package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/madhouselabs/goman/pkg/provider/aws"
	"github.com/madhouselabs/goman/pkg/storage"
	"gopkg.in/yaml.v3"
)

func main() {
	ctx := context.Background()
	
	// Get provider
	provider, err := aws.GetCachedProvider("default", "ap-south-1")
	if err != nil {
		log.Fatal(err)
	}
	
	storageService := provider.GetStorageService()
	
	// Read existing config
	configKey := "clusters/k3s-cluster-1756100915/config.yaml"
	configData, err := storageService.GetObject(ctx, configKey)
	if err != nil {
		log.Fatal(err)
	}
	
	fmt.Println("=== ORIGINAL CONFIG ===")
	fmt.Println(string(configData))
	
	// Parse as storage.ClusterConfig
	var config storage.ClusterConfig
	if err := yaml.Unmarshal(configData, &config); err != nil {
		log.Fatal(err)
	}
	
	// Add nodepools
	config.Spec.NodePools = []storage.NodePool{
		{
			Name:         "general",
			Count:        2,
			InstanceType: "t3.medium",
			Labels: map[string]string{
				"workload": "general",
				"tier":     "application",
			},
		},
	}
	
	// Update timestamp
	config.Metadata.UpdatedAt = time.Now()
	
	// Marshal back
	updatedData, err := yaml.Marshal(config)
	if err != nil {
		log.Fatal(err)
	}
	
	fmt.Println("\n=== UPDATED CONFIG ===")
	fmt.Println(string(updatedData))
	
	// Save back
	if err := storageService.PutObject(ctx, configKey, updatedData); err != nil {
		log.Fatal(err)
	}
	
	fmt.Println("\n=== SAVED SUCCESSFULLY ===")
	
	// Read back to verify
	verifyData, err := storageService.GetObject(ctx, configKey)
	if err != nil {
		log.Fatal(err)
	}
	
	fmt.Println("\n=== VERIFICATION READ ===")
	fmt.Println(string(verifyData))
}