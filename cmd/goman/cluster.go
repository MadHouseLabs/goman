package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/madhouselabs/goman/pkg/cluster"
	"github.com/madhouselabs/goman/pkg/connectivity"
	"github.com/madhouselabs/goman/pkg/provider/aws"
	"github.com/spf13/cobra"
)

// clusterCmd represents the cluster command group
var clusterCmd = &cobra.Command{
	Use:   "cluster",
	Short: "Manage K3s clusters",
	Long:  `Manage K3s cluster operations including create, list, delete, and connect.`,
}

// clusterConnectCmd connects to a cluster
var clusterConnectCmd = &cobra.Command{
	Use:   "connect [cluster-name]",
	Short: "Connect to a K3s cluster via SSM tunnel",
	Long: `Establishes an SSM tunnel to the specified K3s cluster.
This creates a secure connection without requiring public IPs.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var clusterName string
		if len(args) > 0 {
			clusterName = args[0]
		} else {
			// Use interactive selector
			selected, err := getOrSelectCluster("", "connect to")
			if err != nil {
				return err
			}
			clusterName = selected
		}

		// Connect to the cluster
		return connectToClusterCLI(clusterName)
	},
}

// clusterDisconnectCmd disconnects from a cluster
var clusterDisconnectCmd = &cobra.Command{
	Use:   "disconnect [cluster-name]",
	Short: "Disconnect from a K3s cluster",
	Long:  `Closes the SSM tunnel to the specified K3s cluster.`,
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var clusterName string
		if len(args) > 0 {
			clusterName = args[0]
		} else {
			// Use interactive selector
			selected, err := getOrSelectCluster("", "disconnect from")
			if err != nil {
				return err
			}
			clusterName = selected
		}

		// Use SingleTunnelManager
		stm := connectivity.NewSingleTunnelManager()
		
		// Check if tunnel exists
		if !stm.IsConnected(clusterName) {
			return fmt.Errorf("not connected to cluster %s", clusterName)
		}

		// Stop the tunnel
		if err := stm.StopTunnel(clusterName); err != nil {
			return fmt.Errorf("failed to disconnect: %w", err)
		}

		// Clear current cluster if it matches
		if getCurrentCluster() == clusterName {
			clearCurrentCluster()
		}

		fmt.Printf("✅ Disconnected from cluster %s\n", clusterName)
		return nil
	},
}

// clusterStatusCmd shows connection status
var clusterStatusCmd = &cobra.Command{
	Use:   "status [cluster-name]",
	Short: "Show cluster connection status",
	Long:  `Shows the current connection status for K3s clusters.`,
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) > 0 {
			// Check specific cluster
			clusterName := args[0]
			stm := connectivity.NewSingleTunnelManager()
			if stm.IsConnected(clusterName) {
				fmt.Printf("✅ Connected to cluster %s\n", clusterName)
				fmt.Printf("   Kubeconfig: ~/.kube/goman/%s.yaml\n", clusterName)
				fmt.Printf("   You can now use: goman kube kubectl get nodes\n")
			} else {
				fmt.Printf("❌ Not connected to cluster %s\n", clusterName)
				fmt.Printf("   Connect using: goman cluster connect %s\n", clusterName)
			}
		} else {
			// Show all connections
			fmt.Println("Cluster Connection Status:")
			fmt.Println("==========================")
			
			// Initialize cluster manager if needed
			if clusterManager == nil {
				clusterManager = cluster.NewManager()
			}
			
			// Get all clusters
			clusters := clusterManager.GetClusters()
			if len(clusters) == 0 {
				fmt.Println("No clusters found")
				return nil
			}

			for _, cluster := range clusters {
				stm := connectivity.NewSingleTunnelManager()
				if stm.IsConnected(cluster.Name) {
					fmt.Printf("✅ %s - Connected\n", cluster.Name)
				} else {
					fmt.Printf("⭕ %s - Not connected\n", cluster.Name)
				}
			}
		}
		return nil
	},
}

func connectToClusterCLI(clusterName string) error {
	fmt.Printf("🔄 Setting current cluster to %s...\n", clusterName)

	// Initialize AWS provider
	profile := os.Getenv("AWS_PROFILE")
	if profile == "" {
		profile = "default"
	}
	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = "ap-south-1"
	}

	provider, err := aws.GetCachedProvider(profile, region)
	if err != nil {
		return fmt.Errorf("failed to initialize AWS provider: %w", err)
	}

	storageService := provider.GetStorageService()
	ctx := context.Background()

	// Download kubeconfig from S3
	kubeconfigKey := fmt.Sprintf("clusters/%s/kubeconfig", clusterName)
	kubeconfigData, err := storageService.GetObject(ctx, kubeconfigKey)
	if err != nil {
		return fmt.Errorf("failed to download kubeconfig: %w", err)
	}

	// Save kubeconfig to local filesystem
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	kubeconfigDir := filepath.Join(homeDir, ".kube", "goman")
	if err := os.MkdirAll(kubeconfigDir, 0755); err != nil {
		return fmt.Errorf("failed to create kubeconfig directory: %w", err)
	}

	kubeconfigPath := filepath.Join(kubeconfigDir, fmt.Sprintf("%s.yaml", clusterName))
	if err := os.WriteFile(kubeconfigPath, kubeconfigData, 0600); err != nil {
		return fmt.Errorf("failed to save kubeconfig: %w", err)
	}

	// Save as current cluster (simplified - no tunnel management)
	saveCurrentCluster(clusterName)

	fmt.Printf("✅ Selected cluster: %s\n", clusterName)

	return nil
}

func init() {
	clusterCmd.AddCommand(clusterConnectCmd)
	clusterCmd.AddCommand(clusterDisconnectCmd)
	clusterCmd.AddCommand(clusterStatusCmd)
}