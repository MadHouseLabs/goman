package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/madhouselabs/goman/pkg/cluster"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"
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
		stm := GetGlobalSingleTunnelManager()
		
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

// clusterStatusCmd shows cluster progress and status
var clusterStatusCmd = &cobra.Command{
	Use:   "status [cluster-name]",
	Short: "Show cluster progress and status",
	Long:  `Shows detailed progress and status for K3s clusters including reconciliation progress, instance states, and recent activity.`,
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) > 0 {
			// Show detailed status for specific cluster
			return showClusterProgress(args[0])
		} else {
			// Show all clusters with basic status
			return showAllClustersProgress()
		}
	},
}

func connectToClusterCLI(clusterName string) error {
	fmt.Printf("🔄 Setting current cluster to %s...\n", clusterName)

	// Download kubeconfig if needed (reuse existing function)
	homeDir, _ := os.UserHomeDir()
	kubeconfigPath := filepath.Join(homeDir, ".kube", "goman", fmt.Sprintf("%s.yaml", clusterName))
	
	// Check if kubeconfig already exists locally
	if _, err := os.Stat(kubeconfigPath); os.IsNotExist(err) {
		// Download from S3 if missing
		if err := downloadKubeconfig(clusterName); err != nil {
			return err
		}
	}

	// Save as current cluster
	saveCurrentCluster(clusterName)

	fmt.Printf("✅ Selected cluster: %s\n", clusterName)

	return nil
}

// showClusterProgress displays detailed progress for a specific cluster
func showClusterProgress(clusterName string) error {
	ctx := context.Background()
	
	// First, try to get cluster region from a quick S3 check with default region
	defaultCfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion("ap-south-1"), // Default region for goman clusters
	)
	if err != nil {
		return fmt.Errorf("failed to load AWS config: %w", err)
	}
	
	// Get account ID
	stsClient := sts.NewFromConfig(defaultCfg)
	identity, err := stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return fmt.Errorf("failed to get AWS account ID: %w", err)
	}
	accountID := *identity.Account
	
	// Try to get cluster config to determine actual region
	bucketName := fmt.Sprintf("goman-%s", accountID)
	configKey := fmt.Sprintf("clusters/%s/config.yaml", clusterName)
	statusKey := fmt.Sprintf("clusters/%s/status.yaml", clusterName)
	tokenKey := fmt.Sprintf("clusters/%s/k3s-server-token", clusterName)
	
	s3Client := s3.NewFromConfig(defaultCfg)
	var mode, region, instanceType string
	
	// Get cluster region from config
	configResp, err := s3Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: &bucketName,
		Key:    &configKey,
	})
	if err != nil {
		return fmt.Errorf("❌ Cluster %s not found", clusterName)
	}
	
	defer configResp.Body.Close()
	configData := make([]byte, 2048)
	n, _ := configResp.Body.Read(configData)
	configStr := string(configData[:n])
	
	// Parse YAML content to get the config values
	var configYaml map[string]interface{}
	if err := yaml.Unmarshal(configData[:n], &configYaml); err == nil {
		if spec, ok := configYaml["spec"].(map[interface{}]interface{}); ok {
			if m, ok := spec["mode"]; ok {
				if modeStr, ok := m.(string); ok {
					mode = modeStr
				}
			}
			if r, ok := spec["region"]; ok {
				if regionStr, ok := r.(string); ok {
					region = regionStr
				}
			}
			if it, ok := spec["instanceType"]; ok {
				if instanceTypeStr, ok := it.(string); ok {
					instanceType = instanceTypeStr
				}
			}
		}
	} else {
		// Fallback to simple line parsing if YAML parsing fails
		lines := strings.Split(configStr, "\n")
		for _, line := range lines {
			if strings.Contains(line, "region: ap-south-1") {
				region = "ap-south-1"
			}
			if strings.Contains(line, "mode: ha") {
				mode = "ha"
			} else if strings.Contains(line, "mode: dev") {
				mode = "dev"
			}
			if strings.Contains(line, "instanceType: t3.") {
				parts := strings.Split(line, ":")
				if len(parts) >= 2 {
					instanceType = strings.TrimSpace(parts[1])
				}
			}
		}
	}
	
	
	// Now create config with correct region
	var cfg aws.Config
	if region != "" && region != "ap-south-1" {
		cfg, err = config.LoadDefaultConfig(ctx,
			config.WithRegion(region),
		)
		if err != nil {
			return fmt.Errorf("failed to load AWS config for region %s: %w", region, err)
		}
		s3Client = s3.NewFromConfig(cfg)
	} else {
		cfg = defaultCfg
		region = "ap-south-1" // Default region
	}
	
	logsClient := cloudwatchlogs.NewFromConfig(cfg)
	
	fmt.Printf("=== CLUSTER PROGRESS: %s ===\n\n", clusterName)
	
	// Get status with progress metrics
	statusResp, err := s3Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: &bucketName,
		Key:    &statusKey,
	})
	var statusData []byte
	
	if err != nil {
		fmt.Println("  ❌ No status file found")
	} else {
		defer statusResp.Body.Close()
		statusData = make([]byte, 16384) // Increased buffer size for larger status files
		n, _ = statusResp.Body.Read(statusData)
		statusStr := string(statusData[:n])
		
		// Parse status YAML
		var status map[string]interface{}
		if err := yaml.Unmarshal([]byte(statusStr), &status); err == nil {
			if metadata, ok := status["metadata"].(map[interface{}]interface{}); ok {
				var phase, message, lastReconciled interface{}
				if p, ok := metadata["phase"]; ok {
					phase = p
				}
				if m, ok := metadata["message"]; ok {
					message = m
				}
				if lr, ok := metadata["last_reconciled"]; ok {
					lastReconciled = lr
				}
				
				// Show basic cluster info in header
				fmt.Printf("Cluster: %s [%s] - %s/%s (%s)\n", clusterName, mode, region, instanceType, phase)
				fmt.Printf("Message: %v\n", message)
				fmt.Printf("Last Reconciled: %v\n", lastReconciled)
				fmt.Println()
				
				// Show progress metrics if available
				if progressMetrics, ok := metadata["progress_metrics"].(map[interface{}]interface{}); ok {
					fmt.Println("📈 PROGRESS MATRIX:")
					if currentOp, ok := progressMetrics["currentoperation"]; ok {
						fmt.Printf("%v", currentOp)
					}
					// Count steps directly from the steps array for accurate display
					totalSteps := 0
					completedSteps := 0
					if steps, ok := progressMetrics["steps"].([]interface{}); ok {
						totalSteps = len(steps)
						for _, stepInterface := range steps {
							if step, ok := stepInterface.(map[interface{}]interface{}); ok {
								if status, ok := step["status"].(string); ok && status == "Done" {
									completedSteps++
								}
							}
						}
					}
					fmt.Printf(" (%d/%d steps completed):\n", completedSteps, totalSteps)
					
					if steps, ok := progressMetrics["steps"].([]interface{}); ok {
						// Create ordered steps array for display
						orderedSteps := make([]interface{}, 0, len(steps))
						stepMap := make(map[string]interface{})
						
						// Build step map
						for _, stepInterface := range steps {
							if step, ok := stepInterface.(map[interface{}]interface{}); ok {
								if name, ok := step["name"].(string); ok {
									stepMap[name] = stepInterface
								}
							}
						}
						
						// Add steps in correct order
						stepOrder := []string{"Provisioning", "Installing", "Configuring"}
						for _, stepName := range stepOrder {
							if step, exists := stepMap[stepName]; exists {
								orderedSteps = append(orderedSteps, step)
							}
						}
						
						// Display ordered steps
						for _, stepInterface := range orderedSteps {
							if step, ok := stepInterface.(map[interface{}]interface{}); ok {
								name := step["name"]
								stepStatus := step["status"]
								fmt.Printf("- Step: %v [%v]\n", name, stepStatus)
								
								if description, ok := step["description"]; ok {
									fmt.Printf("  %v\n", description)
								}
								
								if checks, ok := step["checks"].([]interface{}); ok {
									for _, checkInterface := range checks {
										if check, ok := checkInterface.(map[interface{}]interface{}); ok {
											checkName := check["name"]
											checkStatus := check["status"]
											
											// Add failure count if there have been failures
											if failureCount, ok := check["failurecount"]; ok && failureCount != nil {
												var count int
												switch v := failureCount.(type) {
												case float64:
													count = int(v)
												case int:
													count = v
												}
												if count > 0 {
													checkStatus = fmt.Sprintf("%v (Attempt %d/3)", checkStatus, count)
												}
											}
											
											fmt.Printf("    - %v: %v", checkName, checkStatus)
											
											if details, ok := check["details"]; ok && details != "" {
												fmt.Printf(" - %v", details)
											}
											if errorMsg, ok := check["errormessage"]; ok && errorMsg != "" {
												fmt.Printf(" (Error: %v)", errorMsg)
											}
											
											// Add retry timing info if applicable
											if checkStatus == "Failed" {
												if retryAfter, ok := check["retryafter"]; ok && retryAfter != nil {
													if retryStr, ok := retryAfter.(string); ok && retryStr != "" {
														// Parse the time string and show countdown
														fmt.Printf(" [Retry scheduled]")
													}
												}
											}
											
											fmt.Println()
										}
									}
								}
							}
						}
					}
				} else {
					fmt.Println("📈 PROGRESS MATRIX:")
					fmt.Println("❌ No progress metrics available")
					fmt.Println("💡 Progress metrics require the new reconciler version")
					fmt.Printf("💡 Last reconciled: %v (>4 hours ago - Lambda not running)\n", lastReconciled)
					fmt.Println("💡 The cluster is stuck because Lambda reconciliation has stopped")
				}
			}
		}
	}
	
	// Show minimal summary
	fmt.Println("\n💡 SUMMARY:")
	
	// Check K3s token
	_, err = s3Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: &bucketName,
		Key:    &tokenKey,
	})
	if err != nil {
		fmt.Println("🔑 K3s Token: ❌ Missing from S3 (blocking HA cluster formation)")
	} else {
		fmt.Println("🔑 K3s Token: ✅ Available in S3")
	}
	
	// Check recent Lambda activity
	logGroupName := fmt.Sprintf("/aws/lambda/goman-controller-%s", accountID)
	startTime := time.Now().Add(-30 * time.Minute).Unix() * 1000
	
	filterInput := &cloudwatchlogs.FilterLogEventsInput{
		LogGroupName: &logGroupName,
		StartTime:    &startTime,
		FilterPattern: &clusterName,
	}
	
	result, err := logsClient.FilterLogEvents(ctx, filterInput)
	if err == nil && len(result.Events) > 0 {
		fmt.Println("📝 Lambda Activity: ✅ Recent activity detected")
	} else {
		fmt.Println("📝 Lambda Activity: ❌ No recent activity (Lambda not running)")
		fmt.Println("   💡 Cluster is stuck - Lambda reconciliation has stopped")
	}
	
	return nil
}

// showAllClustersProgress displays basic status for all clusters
func showAllClustersProgress() error {
	
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
	
	fmt.Println("=== ALL CLUSTERS STATUS ===")
	fmt.Println()
	
	for _, cluster := range clusters {
		fmt.Printf("🔧 %s:\n", cluster.Name)
		fmt.Printf("  Mode: %s\n", cluster.Mode)
		fmt.Printf("  Region: %s\n", cluster.Region)
		fmt.Printf("  Status: %s\n", cluster.Status)
		
		// Show connection status
		stm := GetGlobalSingleTunnelManager()
		if stm.IsConnected(cluster.Name) {
			fmt.Printf("  Connection: ✅ Connected\n")
		} else {
			fmt.Printf("  Connection: ⭕ Not connected\n")
		}
		fmt.Println()
	}
	
	fmt.Println("💡 Use 'goman cluster status <cluster-name>' for detailed progress")
	return nil
}

func init() {
	clusterCmd.AddCommand(clusterConnectCmd)
	clusterCmd.AddCommand(clusterDisconnectCmd)
	clusterCmd.AddCommand(clusterStatusCmd)
}