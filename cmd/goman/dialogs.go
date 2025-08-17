package main

import (
	"fmt"

	"github.com/madhouselabs/goman/pkg/models"
	"github.com/rivo/tview"
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
	clusterMode := models.ModeDeveloper
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