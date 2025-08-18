package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/madhouselabs/goman/pkg/cluster"
	"github.com/madhouselabs/goman/pkg/connectivity"
	"github.com/madhouselabs/goman/pkg/models"
	"github.com/madhouselabs/goman/pkg/provider/aws"
	"github.com/madhouselabs/goman/pkg/storage"
	"github.com/spf13/cobra"
)

// kubeCmd represents the kube command
var kubeCmd = &cobra.Command{
	Use:   "kube [command]",
	Short: "Run commands with KUBECONFIG configured",
	Long: `Run any command with the KUBECONFIG environment variable set to the connected cluster.

Examples:
  goman kube kubectl get nodes       # Run kubectl commands
  goman kube k9s                     # Launch k9s
  goman kube bash                    # Open shell with KUBECONFIG set
  goman kube helm install myapp .    # Run helm commands
  goman kube --force-clean kubectl get nodes  # Force clean all tunnels before connecting`,
	Args:                  cobra.MinimumNArgs(1),
	DisableFlagParsing:    true,
	DisableFlagsInUseLine: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Get current cluster
		clusterName := getCurrentCluster()
		if clusterName == "" {
			// Try to find any connected cluster
			clusterName = findConnectedCluster()
			if clusterName == "" {
				return fmt.Errorf("no cluster is selected. Run: goman cluster connect [cluster-name]")
			}
		}
		
		// Ensure SSM tunnel is established (connect on demand if needed)
		fmt.Printf("🔄 Ensuring SSM tunnel to cluster %s...\n", clusterName)
		if err := establishSSMTunnel(clusterName); err != nil {
			return fmt.Errorf("failed to establish tunnel: %w", err)
		}

		// Get kubeconfig path
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get home directory: %w", err)
		}

		kubeconfigPath := filepath.Join(homeDir, ".kube", "goman", fmt.Sprintf("%s.yaml", clusterName))
		
		// Check if kubeconfig exists, download if missing
		if _, err := os.Stat(kubeconfigPath); os.IsNotExist(err) {
			fmt.Printf("📥 Downloading kubeconfig for cluster %s...\n", clusterName)
			if err := downloadKubeconfig(clusterName); err != nil {
				return fmt.Errorf("failed to download kubeconfig: %w", err)
			}
		}

		// Prepare the command
		cmdName := args[0]
		cmdArgs := args[1:]

		// Special handling for shell commands
		if cmdName == "bash" || cmdName == "sh" || cmdName == "zsh" {
			shell := cmdName
			if cmdName == "bash" && os.Getenv("SHELL") != "" {
				// Use user's preferred shell if they just typed "bash"
				shell = os.Getenv("SHELL")
			}
			
			fmt.Printf("🚀 Opening shell with KUBECONFIG for cluster: %s\n", clusterName)
			fmt.Println("Type 'exit' to return")
			fmt.Println()

			// Create shell with custom prompt
			shellCmd := exec.Command(shell)
			shellCmd.Env = append(os.Environ(), 
				fmt.Sprintf("KUBECONFIG=%s", kubeconfigPath),
				fmt.Sprintf("PS1=[goman:%s] \\w $ ", clusterName))
			shellCmd.Stdin = os.Stdin
			shellCmd.Stdout = os.Stdout
			shellCmd.Stderr = os.Stderr
			
			return shellCmd.Run()
		}

		// Check if command exists
		if _, err := exec.LookPath(cmdName); err != nil {
			// Special message for common tools
			switch cmdName {
			case "k9s":
				return fmt.Errorf("k9s is not installed. Install it from: https://k9scli.io")
			case "kubectl":
				return fmt.Errorf("kubectl is not installed. Install it from: https://kubernetes.io/docs/tasks/tools/")
			case "helm":
				return fmt.Errorf("helm is not installed. Install it from: https://helm.sh/docs/intro/install/")
			default:
				return fmt.Errorf("command '%s' not found", cmdName)
			}
		}

		// Run the command with KUBECONFIG set
		runCmd := exec.Command(cmdName, cmdArgs...)
		runCmd.Env = append(os.Environ(), fmt.Sprintf("KUBECONFIG=%s", kubeconfigPath))
		runCmd.Stdin = os.Stdin
		runCmd.Stdout = os.Stdout
		runCmd.Stderr = os.Stderr

		// Show what we're running (except for interactive tools)
		if cmdName != "k9s" && cmdName != "kubectl" {
			fmt.Printf("🚀 Running: %s %s\n", cmdName, strings.Join(cmdArgs, " "))
			fmt.Printf("   Cluster: %s\n\n", clusterName)
		}

		return runCmd.Run()
	},
}

// findConnectedCluster finds the first connected cluster
func findConnectedCluster() string {
	// Initialize cluster manager if needed
	if clusterManager == nil {
		clusterManager = cluster.NewManager()
		if clusterManager == nil {
			return ""
		}
	}
	
	clusters := clusterManager.GetClusters()
	stm := connectivity.NewSingleTunnelManager()
	for _, cluster := range clusters {
		if stm.IsConnected(cluster.Name) {
			return cluster.Name
		}
	}
	return ""
}

// findAllConnectedClusters returns all connected cluster names
func findAllConnectedClusters() []string {
	var connected []string
	
	// Initialize cluster manager if needed
	if clusterManager == nil {
		clusterManager = cluster.NewManager()
		if clusterManager == nil {
			return connected
		}
	}
	
	clusters := clusterManager.GetClusters()
	stm := connectivity.NewSingleTunnelManager()
	for _, cluster := range clusters {
		if stm.IsConnected(cluster.Name) {
			connected = append(connected, cluster.Name)
		}
	}
	return connected
}

// downloadKubeconfig downloads the kubeconfig from S3 if it doesn't exist locally
func downloadKubeconfig(clusterName string) error {
	// Initialize AWS provider and storage
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
		return fmt.Errorf("failed to download kubeconfig from S3: %w", err)
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

	fmt.Printf("✅ Downloaded kubeconfig for cluster %s\n", clusterName)
	return nil
}

// establishSSMTunnel establishes an SSM tunnel to the cluster using SingleTunnelManager
func establishSSMTunnel(clusterName string) error {
	// Initialize cluster manager if needed
	if clusterManager == nil {
		clusterManager = cluster.NewManager()
	}
	
	// Get cluster details
	clusters := clusterManager.GetClusters()
	var targetCluster *models.K3sCluster
	for _, cluster := range clusters {
		if cluster.Name == clusterName {
			targetCluster = &cluster
			break
		}
	}

	if targetCluster == nil {
		return fmt.Errorf("cluster %s not found", clusterName)
	}

	// Get master instance ID
	masterInstanceID := ""
	if len(targetCluster.MasterNodes) > 0 {
		masterInstanceID = targetCluster.MasterNodes[0].ID
	}

	if masterInstanceID == "" {
		// Try to get from S3 state
		profile := os.Getenv("AWS_PROFILE")
		if profile == "" {
			profile = "default"
		}
		
		backend, err := storage.NewS3Backend(profile)
		if err != nil {
			return fmt.Errorf("failed to initialize storage: %w", err)
		}

		clusterState, err := backend.LoadClusterState(clusterName)
		if err != nil {
			return fmt.Errorf("failed to load cluster state: %w", err)
		}

		// For HA clusters, connect to master-0 specifically
		for nodeName, instanceID := range clusterState.InstanceIDs {
			if strings.Contains(nodeName, "master-0") {
				masterInstanceID = instanceID
				break
			}
		}
		// Fallback to any master if master-0 not found
		if masterInstanceID == "" {
			for nodeName, instanceID := range clusterState.InstanceIDs {
				if strings.Contains(nodeName, "master") {
					masterInstanceID = instanceID
					break
				}
			}
		}
	}

	if masterInstanceID == "" {
		return fmt.Errorf("no master instance found for cluster %s", clusterName)
	}

	// Get cluster region
	region := targetCluster.Region
	if region == "" {
		region = os.Getenv("AWS_REGION")
		if region == "" {
			region = "ap-south-1" // Default region
		}
	}
	
	// Use SingleTunnelManager to ensure tunnel
	stm := connectivity.NewSingleTunnelManager()
	if err := stm.EnsureTunnel(clusterName, masterInstanceID, region); err != nil {
		return fmt.Errorf("failed to ensure SSM tunnel: %w", err)
	}
	
	return nil
}