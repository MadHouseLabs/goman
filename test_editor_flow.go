package main

import (
	"fmt"
	"log"

	"github.com/madhouselabs/goman/pkg/cluster"
	"github.com/madhouselabs/goman/pkg/models"
	"gopkg.in/yaml.v2"
)

func main() {
	// Initialize cluster manager
	clusterManager := cluster.NewManager()
	
	// Get the cluster to edit
	clusters := clusterManager.GetClusters()
	var targetCluster *models.K3sCluster
	for i := range clusters {
		if clusters[i].Name == "k3s-cluster-1756100915" {
			targetCluster = &clusters[i]
			break
		}
	}
	
	if targetCluster == nil {
		log.Fatal("Cluster not found")
	}
	
	fmt.Printf("Original cluster NodePools: %+v\n", targetCluster.NodePools)
	
	// Simulate YAML edit
	yamlContent := `
description: "Development cluster with workers"
region: ap-south-1
instanceType: t3.medium

nodePools:
  - name: workers
    count: 3
    instanceType: t3.large
    labels:
      role: worker
`
	
	// Parse YAML like the editor does
	var config map[string]interface{}
	if err := yaml.Unmarshal([]byte(yamlContent), &config); err != nil {
		log.Fatal(err)
	}
	
	// Extract nodePools
	var nodePools []models.NodePool
	if nodePoolsRaw, ok := config["nodePools"]; ok {
		fmt.Printf("Found nodePools: %+v\n", nodePoolsRaw)
		if npList, ok := nodePoolsRaw.([]interface{}); ok {
			for _, np := range npList {
				if npMap, ok := np.(map[interface{}]interface{}); ok {
					nodePool := models.NodePool{}
					
					if name, ok := npMap["name"].(string); ok {
						nodePool.Name = name
					}
					if count, ok := npMap["count"].(int); ok {
						nodePool.Count = count
					}
					if instanceType, ok := npMap["instanceType"].(string); ok {
						nodePool.InstanceType = instanceType
					}
					
					// Parse labels
					if labelsRaw, ok := npMap["labels"]; ok {
						if labelsMap, ok := labelsRaw.(map[interface{}]interface{}); ok {
							nodePool.Labels = make(map[string]string)
							for k, v := range labelsMap {
								if ks, ok := k.(string); ok {
									if vs, ok := v.(string); ok {
										nodePool.Labels[ks] = vs
									}
								}
							}
						}
					}
					
					nodePools = append(nodePools, nodePool)
				}
			}
		}
	}
	
	fmt.Printf("Parsed nodePools: %+v\n", nodePools)
	
	// Update the cluster
	targetCluster.NodePools = nodePools
	targetCluster.Description = "Development cluster with workers"
	
	fmt.Printf("Cluster before UpdateCluster: %+v\n", targetCluster.NodePools)
	
	updatedCluster, err := clusterManager.UpdateCluster(*targetCluster)
	if err != nil {
		log.Fatal(err)
	}
	
	fmt.Printf("Updated cluster NodePools: %+v\n", updatedCluster.NodePools)
	
}