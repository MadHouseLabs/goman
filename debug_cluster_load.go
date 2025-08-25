package main

import (
	"fmt"

	"github.com/madhouselabs/goman/pkg/cluster"
)

func main() {
	// Initialize cluster manager (same as UI does)
	clusterManager := cluster.NewManager()
	
	// Get clusters (this is what the UI sees)
	clusters := clusterManager.GetClusters()
	
	fmt.Printf("Total clusters loaded: %d\n\n", len(clusters))
	
	for _, c := range clusters {
		fmt.Printf("Cluster: %s\n", c.Name)
		fmt.Printf("  Description: %s\n", c.Description)
		fmt.Printf("  NodePools: %d\n", len(c.NodePools))
		if len(c.NodePools) > 0 {
			for i, np := range c.NodePools {
				fmt.Printf("    [%d] Name: %s, Count: %d, InstanceType: %s\n", 
					i, np.Name, np.Count, np.InstanceType)
			}
		}
		fmt.Println()
	}
}