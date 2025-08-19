package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/madhouselabs/goman/pkg/provider/registry"
	"github.com/madhouselabs/goman/pkg/storage"
	"github.com/spf13/cobra"
)

// Use global single tunnel manager
var singleTunnelManager = GetGlobalSingleTunnelManager()

// kubectlCmd represents the kubectl command group
var kubectlCmd = &cobra.Command{
	Use:   "kubectl",
	Short: "Manage kubectl access to K3s clusters",
	Long: `Connect to K3s clusters using secure SSM port forwarding.
No public IPs or open security groups required.

When run without subcommands, shows an interactive cluster selector.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// If no subcommand, show interactive selector
		selected, err := getOrSelectCluster("", "manage")
		if err != nil {
			return err
		}
		
		fmt.Printf("\nSelected cluster: %s\n\n", selected)
		fmt.Println("Available commands:")
		fmt.Printf("  goman kubectl connect %s         # Connect to cluster\n", selected)
		fmt.Printf("  goman kubectl exec %s -- get nodes  # Execute kubectl command\n", selected)
		fmt.Printf("  goman kubectl disconnect %s      # Disconnect from cluster\n", selected)
		fmt.Printf("  goman kubectl status %s          # Show connection status\n", selected)
		
		return nil
	},
}

// connectCmd connects to a cluster
var connectCmd = &cobra.Command{
	Use:   "connect [cluster-name]",
	Short: "Connect to a K3s cluster via SSM tunnel",
	Long: `Establishes a secure SSM port forwarding session to the K3s API server.
Downloads the kubeconfig and sets up kubectl context.
If no cluster name is provided, shows an interactive selector.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var clusterName string
		if len(args) > 0 {
			clusterName = args[0]
		} else {
			// Show cluster selector
			selected, err := getOrSelectCluster("", "connect to")
			if err != nil {
				return err
			}
			clusterName = selected
			fmt.Printf("\nConnecting to cluster: %s\n", clusterName)
		}
		return connectToClusterCLI(clusterName)
	},
}

// disconnectCmd disconnects from a cluster
var disconnectCmd = &cobra.Command{
	Use:   "disconnect [cluster-name]",
	Short: "Disconnect from a K3s cluster",
	Long:  "Stops the SSM tunnel for a connected cluster.\nIf no cluster name is provided, shows an interactive selector.",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var clusterName string
		if len(args) > 0 {
			clusterName = args[0]
		} else {
			// Show cluster selector
			selected, err := getOrSelectCluster("", "disconnect from")
			if err != nil {
				return err
			}
			clusterName = selected
			fmt.Printf("\nDisconnecting from cluster: %s\n", clusterName)
		}
		return disconnectFromCluster(clusterName)
	},
}

// execCmd executes kubectl commands with automatic connection
var execCmd = &cobra.Command{
	Use:                "exec [cluster-name] -- [kubectl args]",
	Short:              "Execute kubectl commands on a cluster",
	Long:               "Execute kubectl commands on a cluster with automatic SSM tunnel setup.\nIf no cluster name is provided, shows an interactive selector.",
	DisableFlagParsing: true,
	Args:               cobra.MinimumNArgs(0),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Find the -- separator
		dashIndex := -1
		for i, arg := range args {
			if arg == "--" {
				dashIndex = i
				break
			}
		}
		
		var clusterName string
		var kubectlArgs []string
		
		if dashIndex == -1 {
			// No -- separator, might be just "goman kubectl exec"
			if len(args) == 0 {
				// Show selector
				selected, err := getOrSelectCluster("", "execute on")
				if err != nil {
					return err
				}
				clusterName = selected
				fmt.Printf("\nSelected cluster: %s\n", clusterName)
				fmt.Println("Now enter kubectl command (e.g., 'get nodes', 'get pods -A'):")
				return fmt.Errorf("please run: goman kubectl exec %s -- <kubectl command>", clusterName)
			}
			return fmt.Errorf("usage: goman kubectl exec [cluster-name] -- [kubectl commands]")
		}
		
		if dashIndex == 0 {
			// Format: "goman kubectl exec -- get nodes"
			// Need to select cluster
			selected, err := getOrSelectCluster("", "execute on")
			if err != nil {
				return err
			}
			clusterName = selected
			kubectlArgs = args[1:]
			fmt.Printf("\nExecuting on cluster: %s\n", clusterName)
		} else {
			// Format: "goman kubectl exec cluster-name -- get nodes"
			clusterName = args[0]
			kubectlArgs = args[dashIndex+1:]
		}
		
		if len(kubectlArgs) == 0 {
			return fmt.Errorf("no kubectl command provided after --")
		}
		
		return executeKubectlCommand(clusterName, kubectlArgs)
	},
}

// statusCmd shows connection status
var statusCmd = &cobra.Command{
	Use:   "status [cluster-name]",
	Short: "Show connection status for a cluster",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			// Show all connections
			return showAllConnectionStatus()
		}
		return showConnectionStatus(args[0])
	},
}

func init() {
	kubectlCmd.AddCommand(connectCmd)
	kubectlCmd.AddCommand(disconnectCmd)
	kubectlCmd.AddCommand(execCmd)
	kubectlCmd.AddCommand(statusCmd)
}


func disconnectFromCluster(clusterName string) error {
	if !singleTunnelManager.IsConnected(clusterName) {
		fmt.Printf("Not connected to cluster %s\n", clusterName)
		return nil
	}

	if err := singleTunnelManager.StopTunnel(clusterName); err != nil {
		return fmt.Errorf("failed to stop tunnel: %w", err)
	}

	fmt.Printf("Disconnected from cluster %s\n", clusterName)
	return nil
}

func executeKubectlCommand(clusterName string, kubectlArgs []string) error {
	// Ensure connected
	if !singleTunnelManager.IsConnected(clusterName) {
		fmt.Printf("Connecting to cluster %s...\n", clusterName)
		if err := connectToClusterCLI(clusterName); err != nil {
			return err
		}
		// Add a small delay for tunnel to stabilize
		time.Sleep(2 * time.Second)
	}

	// Get kubeconfig path
	homeDir, _ := os.UserHomeDir()
	kubeconfigPath := filepath.Join(homeDir, ".kube", "goman", fmt.Sprintf("%s.yaml", clusterName))

	// Check if kubeconfig exists
	if _, err := os.Stat(kubeconfigPath); os.IsNotExist(err) {
		return fmt.Errorf("kubeconfig not found. Please run 'goman kubectl connect %s' first", clusterName)
	}

	// Execute kubectl command
	cmd := exec.Command("kubectl", kubectlArgs...)
	cmd.Env = append(os.Environ(), fmt.Sprintf("KUBECONFIG=%s", kubeconfigPath))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	return cmd.Run()
}

func showConnectionStatus(clusterName string) error {
	if singleTunnelManager.IsConnected(clusterName) {
		tunnel := singleTunnelManager.GetTunnelInfo(clusterName)
		if tunnel != nil {
		fmt.Printf("Cluster: %s\n", clusterName)
		fmt.Printf("Status: Connected\n")
		fmt.Printf("Instance: %s\n", tunnel.InstanceID)
			fmt.Printf("Port Forwarding: localhost:%d -> %d\n", tunnel.LocalPort, tunnel.RemotePort)
		} else {
			fmt.Printf("Cluster: %s\n", clusterName)
			fmt.Printf("Status: Not connected\n")
		}
	} else {
		fmt.Printf("Cluster: %s\n", clusterName)
		fmt.Printf("Status: Not connected\n")
	}
	return nil
}

func showAllConnectionStatus() error {
	// Get all clusters from storage
	profile := os.Getenv("AWS_PROFILE")
	if profile == "" {
		profile = "default"
	}
	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = "ap-south-1"  // Default region for goman
	}
	
	_, err := registry.GetProvider("aws", profile, region)
	if err != nil {
		return fmt.Errorf("failed to initialize AWS provider: %w", err)
	}
	backend, err := storage.NewS3Backend(profile)
	if err != nil {
		return fmt.Errorf("failed to initialize storage backend: %w", err)
	}
	
	clusters, err := backend.LoadClusters()
	if err != nil {
		return fmt.Errorf("failed to list clusters: %w", err)
	}

	if len(clusters) == 0 {
		fmt.Println("No clusters found")
		return nil
	}

	fmt.Println("Cluster Connection Status:")
	fmt.Println("─────────────────────────")
	
	for _, cluster := range clusters {
		status := "Not connected"
		if singleTunnelManager.IsConnected(cluster.Name) {
			status = "Connected"
		}
		fmt.Printf("%-20s %s\n", cluster.Name, status)
	}

	return nil
}