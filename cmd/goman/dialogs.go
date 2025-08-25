package main

import (
	"context"
	"fmt"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/madhouselabs/goman/pkg/config"
	"github.com/madhouselabs/goman/pkg/models"
	"github.com/madhouselabs/goman/pkg/provider/aws"
	"github.com/rivo/tview"
	"gopkg.in/yaml.v3"
)

// deleteCluster shows a confirmation dialog for cluster deletion
// This preserves the exact original design from main.go
func deleteCluster(cluster models.K3sCluster) {
	// Confirmation modal with proper dark theme styling
	modal := tview.NewModal().
		SetText(fmt.Sprintf("[::b]Confirm Delete[::-]\n\nAre you sure you want to delete cluster '%s'?", cluster.Name)).
		AddButtons([]string{"Delete", "Cancel"}).
		SetBackgroundColor(ColorBackground).
		SetTextColor(ColorForeground).
		SetButtonBackgroundColor(ColorBackground).
		SetButtonTextColor(ColorForeground).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			// First switch back to clusters page, then remove the modal
			pages.SwitchToPage("clusters")
			pages.RemovePage("confirm")
			
			if buttonLabel == "Delete" {
				// Delete in background
				go func() {
					err := clusterManager.DeleteCluster(cluster.ID)
					app.QueueUpdateDraw(func() {
						if err != nil {
							errorModal := tview.NewModal().
								SetText(fmt.Sprintf("[red][::b]Error[::-][white]\n\nError deleting cluster: %v", err)).
								AddButtons([]string{"OK"}).
								SetBackgroundColor(ColorBackground).
								SetTextColor(ColorForeground).
								SetButtonBackgroundColor(ColorBackground).
								SetButtonTextColor(ColorForeground).
								SetDoneFunc(func(buttonIndex int, buttonLabel string) {
									pages.SwitchToPage("clusters")
									pages.RemovePage("error")
								})
							errorModal.SetBorder(false)
							pages.AddAndSwitchToPage("error", errorModal, true)
						}
						refreshClusters()
					})
				}()
			}
		})
	
	// Remove border to avoid purple background
	modal.SetBorder(false)
	
	pages.AddAndSwitchToPage("confirm", modal, true)
}

// stopCluster stops a running cluster
func stopCluster(cluster models.K3sCluster) {
	if cluster.Status != "running" {
		showError(fmt.Sprintf("Cannot stop cluster '%s' - it is not running (status: %s)", cluster.Name, cluster.Status))
		return
	}
	
	// Confirmation modal
	modal := tview.NewModal().
		SetText(fmt.Sprintf("[::b]Confirm Stop[::-]\n\nStop cluster '%s'?\nThis will stop all EC2 instances.", cluster.Name)).
		AddButtons([]string{"Stop", "Cancel"}).
		SetBackgroundColor(ColorBackground).
		SetTextColor(ColorForeground).
		SetButtonBackgroundColor(ColorBackground).
		SetButtonTextColor(ColorForeground).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			pages.SwitchToPage("clusters")
			pages.RemovePage("confirm")
			
			if buttonLabel == "Stop" {
				// Stop in background
				go func() {
					err := clusterManager.StopCluster(cluster.Name)
					app.QueueUpdateDraw(func() {
						if err != nil {
							showError(fmt.Sprintf("Error stopping cluster: %v", err))
						}
						refreshClusters()
					})
				}()
			}
		})
	
	modal.SetBorder(false)
	pages.AddAndSwitchToPage("confirm", modal, true)
}

// startCluster starts a stopped cluster
func startCluster(cluster models.K3sCluster) {
	if cluster.Status != "stopped" {
		showError(fmt.Sprintf("Cannot start cluster '%s' - it is not stopped (status: %s)", cluster.Name, cluster.Status))
		return
	}
	
	// Confirmation modal
	modal := tview.NewModal().
		SetText(fmt.Sprintf("[::b]Confirm Start[::-]\n\nStart cluster '%s'?\nThis will start all EC2 instances.", cluster.Name)).
		AddButtons([]string{"Start", "Cancel"}).
		SetBackgroundColor(ColorBackground).
		SetTextColor(ColorForeground).
		SetButtonBackgroundColor(ColorBackground).
		SetButtonTextColor(ColorForeground).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			pages.SwitchToPage("clusters")
			pages.RemovePage("confirm")
			
			if buttonLabel == "Start" {
				// Start in background
				go func() {
					err := clusterManager.StartCluster(cluster.Name)
					app.QueueUpdateDraw(func() {
						if err != nil {
							showError(fmt.Sprintf("Error starting cluster: %v", err))
						}
						refreshClusters()
					})
				}()
			}
		})
	
	modal.SetBorder(false)
	pages.AddAndSwitchToPage("confirm", modal, true)
}

// showError displays an error modal
func showError(message string) {
	modal := tview.NewModal().
		SetText(fmt.Sprintf("[red][::b]Error[::-][white]\n\n%s", message)).
		AddButtons([]string{"OK"}).
		SetBackgroundColor(ColorBackground).
		SetTextColor(ColorForeground).
		SetButtonBackgroundColor(ColorBackground).
		SetButtonTextColor(ColorForeground).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			pages.SwitchToPage("clusters")
			pages.RemovePage("error")
		})
	modal.SetBorder(false)
	pages.AddAndSwitchToPage("error", modal, false)
}

// showProgressModal displays a progress modal with a message
func showProgressModal(message string) {
	modal := tview.NewModal().
		SetText(fmt.Sprintf("[::b]Progress[::-]\n\n%s", message)).
		AddButtons([]string{"OK"}).
		SetBackgroundColor(ColorBackground).
		SetTextColor(ColorForeground).
		SetButtonBackgroundColor(ColorBackground).
		SetButtonTextColor(ColorForeground)
	
	modal.SetBorder(false)
	pages.AddAndSwitchToPage("progress", modal, false)
}

// createNewClusterWithUI handles the UI flow for cluster creation
func createNewClusterWithUI(name, description, mode, region, instanceType, nodeCountStr string, showUI bool) {
	// Parse node count
	nodeCount := 1
	fmt.Sscanf(nodeCountStr, "%d", &nodeCount)

	// Determine cluster mode
	clusterMode := models.ModeDev
	if mode == "ha" {
		clusterMode = models.ModeHA
	}

	// Create cluster object
	cluster := &models.K3sCluster{
		Name:         name,
		Description:  description,
		Mode:         clusterMode,
		Region:       region,
		InstanceType: instanceType,
		Status:       "pending",
	}

	// Set nodes based on mode
	if clusterMode == models.ModeHA {
		for i := 0; i < nodeCount; i++ {
			cluster.MasterNodes = append(cluster.MasterNodes, models.Node{
				Name: fmt.Sprintf("%s-master-%d", name, i+1),
			})
		}
	} else {
		cluster.MasterNodes = []models.Node{{
			Name: fmt.Sprintf("%s-master", name),
		}}
	}

	if !showUI {
		// Called from editor, create synchronously
		_, err := clusterManager.CreateCluster(*cluster)
		if err != nil {
			// Log the error so we can debug
			fmt.Printf("\n[ERROR] Failed to create cluster: %v\n", err)
			fmt.Printf("Press Enter to continue...")
			fmt.Scanln()
		}
		return
	}

	// Show progress modal (when called from UI)
	modal := tview.NewModal().
		SetText(fmt.Sprintf("[::b]Creating Cluster[::-]\n\nCreating cluster '%s'...", name)).
		AddButtons([]string{"OK"}).
		SetBackgroundColor(ColorBackground).
		SetTextColor(ColorForeground).
		SetButtonBackgroundColor(ColorBackground).
		SetButtonTextColor(ColorForeground)

	modal.SetBorder(false)
	pages.AddAndSwitchToPage("progress", modal, false)

	// Create cluster in background
	go func() {
		_, err := clusterManager.CreateCluster(*cluster)
		app.QueueUpdateDraw(func() {
			pages.RemovePage("progress")
			if err != nil {
				errorModal := tview.NewModal().
					SetText(fmt.Sprintf("[red][::b]Error[::-][white]\n\nError creating cluster: %v", err)).
					AddButtons([]string{"OK"}).
					SetBackgroundColor(ColorBackground).
					SetTextColor(ColorForeground).
					SetButtonBackgroundColor(ColorBackground).
					SetButtonTextColor(ColorForeground).
					SetDoneFunc(func(buttonIndex int, buttonLabel string) {
						pages.RemovePage("error")
						refreshClusters()
					})
				errorModal.SetBorder(false)
				pages.AddAndSwitchToPage("error", errorModal, false)
			} else {
				refreshClusters()
			}
		})
	}()
}

// triggerReconciliation manually triggers reconciliation for a cluster
func triggerReconciliation(cluster models.K3sCluster) {
	// Show confirmation modal
	modal := tview.NewModal().
		SetText(fmt.Sprintf("[::b]Trigger Reconciliation[::-]\n\nTrigger reconciliation for cluster '%s'?\n\nThis will force the Lambda controller to re-process the cluster.", cluster.Name)).
		AddButtons([]string{"Trigger", "Cancel"}).
		SetBackgroundColor(ColorBackground).
		SetTextColor(ColorForeground).
		SetButtonBackgroundColor(ColorBackground).
		SetButtonTextColor(ColorForeground).
		SetButtonActivatedStyle(tcell.StyleDefault.
			Background(ColorPrimary).
			Foreground(ColorBackground))

	modal.SetDoneFunc(func(buttonIndex int, buttonLabel string) {
		pages.RemovePage("reconcile")
		if buttonLabel == "Trigger" {
			go triggerClusterReconciliation(cluster.Name)
		}
	})

	pages.AddAndSwitchToPage("reconcile", modal, false)
}

// triggerClusterReconciliation performs the actual reconciliation trigger
func triggerClusterReconciliation(clusterName string) {
	// Show progress modal
	modal := tview.NewModal().
		SetText(fmt.Sprintf("[::b]Triggering Reconciliation[::-]\n\nTriggering reconciliation for cluster '%s'...", clusterName)).
		AddButtons([]string{"OK"}).
		SetBackgroundColor(ColorBackground).
		SetTextColor(ColorForeground).
		SetButtonBackgroundColor(ColorBackground).
		SetButtonTextColor(ColorForeground).
		SetButtonActivatedStyle(tcell.StyleDefault.
			Background(ColorPrimary).
			Foreground(ColorBackground))

	pages.AddAndSwitchToPage("progress", modal, false)

	// Trigger reconciliation by updating the cluster config with a timestamp
	// This will cause an S3 event to trigger the Lambda
	err := updateClusterConfigForReconciliation(clusterName)
	
	app.QueueUpdateDraw(func() {
		pages.RemovePage("progress")
		if err != nil {
			errorModal := tview.NewModal().
				SetText(fmt.Sprintf("[red][::b]Error[::-][white]\n\nFailed to trigger reconciliation: %v", err)).
				AddButtons([]string{"OK"}).
				SetBackgroundColor(ColorBackground).
				SetTextColor(ColorForeground).
				SetButtonBackgroundColor(ColorBackground).
				SetButtonTextColor(ColorForeground).
				SetButtonActivatedStyle(tcell.StyleDefault.
					Background(ColorDanger).
					Foreground(ColorBackground))

			errorModal.SetDoneFunc(func(buttonIndex int, buttonLabel string) {
				pages.RemovePage("error")
			})

			pages.AddAndSwitchToPage("error", errorModal, false)
		} else {
			successModal := tview.NewModal().
				SetText(fmt.Sprintf("[green][::b]Success[::-][white]\n\nReconciliation triggered for cluster '%s'.\n\nCheck the cluster status for progress.", clusterName)).
				AddButtons([]string{"OK"}).
				SetBackgroundColor(ColorBackground).
				SetTextColor(ColorForeground).
				SetButtonBackgroundColor(ColorBackground).
				SetButtonTextColor(ColorForeground).
				SetButtonActivatedStyle(tcell.StyleDefault.
					Background(ColorSuccess).
					Foreground(ColorBackground))

			successModal.SetDoneFunc(func(buttonIndex int, buttonLabel string) {
				pages.RemovePage("success")
				refreshClusters()
			})

			pages.AddAndSwitchToPage("success", successModal, false)
		}
	})
}

// updateClusterConfigForReconciliation triggers reconciliation by updating the cluster config
func updateClusterConfigForReconciliation(clusterName string) error {
	ctx := context.Background()
	
	// Get AWS provider
	profile := config.GetAWSProfile()
	region := config.GetAWSRegion()
	provider, err := aws.GetCachedProvider(profile, region)
	if err != nil {
		return fmt.Errorf("failed to get AWS provider: %w", err)
	}
	
	// Get storage service
	storageService := provider.GetStorageService()
	
	// Read current config
	configKey := fmt.Sprintf("clusters/%s/config.yaml", clusterName)
	configData, err := storageService.GetObject(ctx, configKey)
	if err != nil {
		return fmt.Errorf("failed to read cluster config: %w", err)
	}
	
	// Parse config as generic map to preserve structure
	var configMap map[string]interface{}
	if err := yaml.Unmarshal(configData, &configMap); err != nil {
		return fmt.Errorf("failed to parse cluster config: %w", err)
	}
	
	// Add/update reconcile trigger annotation in metadata
	metadata, ok := configMap["metadata"].(map[string]interface{})
	if !ok {
		metadata = make(map[string]interface{})
		configMap["metadata"] = metadata
	}
	
	annotations, ok := metadata["annotations"].(map[string]interface{})
	if !ok {
		annotations = make(map[string]interface{})
		metadata["annotations"] = annotations
	}
	
	// Add trigger timestamp to force reconciliation
	annotations["goman.io/reconcile-trigger"] = time.Now().Format(time.RFC3339)
	
	// Marshal back to YAML
	updatedConfigData, err := yaml.Marshal(configMap)
	if err != nil {
		return fmt.Errorf("failed to marshal updated config: %w", err)
	}
	
	// Save updated config - this will trigger S3 event → Lambda
	if err := storageService.PutObject(ctx, configKey, updatedConfigData); err != nil {
		return fmt.Errorf("failed to save updated config: %w", err)
	}
	
	return nil
}