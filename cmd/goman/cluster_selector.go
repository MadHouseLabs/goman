package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/madhouselabs/goman/pkg/models"
	"github.com/madhouselabs/goman/pkg/provider/registry"
	"github.com/madhouselabs/goman/pkg/storage"
	"github.com/rivo/tview"
)

// selectCluster shows an interactive dropdown to select a cluster
func selectCluster(prompt string) (string, error) {
	// Initialize storage to get clusters
	profile := os.Getenv("AWS_PROFILE")
	if profile == "" {
		profile = "default"
	}
	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = "ap-south-1"
	}

	_, err := registry.GetProvider("aws", profile, region)
	if err != nil {
		return "", fmt.Errorf("failed to initialize AWS provider: %w", err)
	}
	
	backend, err := storage.NewS3Backend(profile)
	if err != nil {
		return "", fmt.Errorf("failed to initialize storage backend: %w", err)
	}

	// Load clusters using the same method as the TUI
	clusterStates, err := backend.LoadAllClusterStates()
	if err != nil {
		return "", fmt.Errorf("failed to load clusters: %w", err)
	}
	
	// Extract clusters from states
	var clusters []models.K3sCluster
	for _, state := range clusterStates {
		if state != nil {
			clusters = append(clusters, state.Cluster)
		}
	}

	if len(clusters) == 0 {
		return "", fmt.Errorf("no clusters found")
	}

	// Create a simple TUI for selection
	app := tview.NewApplication()
	selectedCluster := ""
	
	// Create list with cluster information
	list := tview.NewList()
	list.SetBorder(true).SetTitle(prompt)
	list.ShowSecondaryText(true)
	
	// Add clusters to list with details
	for i, cluster := range clusters {
		// Format secondary text with status and region
		status := string(cluster.Status)
		statusColor := "red"
		if status == "running" {
			statusColor = "green"
		} else if status == "creating" || status == "updating" {
			statusColor = "yellow"
		}
		
		secondaryText := fmt.Sprintf("[%s]%s[-] | %s | %s | %d nodes", 
			statusColor, status, string(cluster.Mode), cluster.Region, 
			len(cluster.MasterNodes)+len(cluster.WorkerNodes))
		
		// Create a closure to capture the cluster name
		clusterName := cluster.Name
		list.AddItem(cluster.Name, secondaryText, rune('1'+i), func() {
			selectedCluster = clusterName
			app.Stop()
		})
	}
	
	// Add quit option
	list.AddItem("", "[dim]Press ESC or q to cancel[-]", 0, nil)
	
	// Style the list
	list.SetSelectedBackgroundColor(tcell.ColorDarkCyan)
	list.SetSelectedTextColor(tcell.ColorWhite)
	
	// Handle escape key
	list.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyEscape:
			app.Stop()
			return nil
		case tcell.KeyRune:
			if event.Rune() == 'q' || event.Rune() == 'Q' {
				app.Stop()
				return nil
			}
		}
		return event
	})

	// Center the list on screen
	flex := tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(list, len(clusters)+4, 1, true).
			AddItem(nil, 0, 1, false), 80, 1, true).
		AddItem(nil, 0, 1, false)

	if err := app.SetRoot(flex, true).EnableMouse(true).Run(); err != nil {
		return "", fmt.Errorf("failed to run selector: %w", err)
	}

	if selectedCluster == "" {
		return "", fmt.Errorf("no cluster selected")
	}

	return selectedCluster, nil
}

// selectClusterSimple shows a simple text-based selector (non-TUI)
func selectClusterSimple() (string, error) {
	// Initialize storage to get clusters
	profile := os.Getenv("AWS_PROFILE")
	if profile == "" {
		profile = "default"
	}
	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = "ap-south-1"
	}

	_, err := registry.GetProvider("aws", profile, region)
	if err != nil {
		return "", fmt.Errorf("failed to initialize AWS provider: %w", err)
	}
	
	backend, err := storage.NewS3Backend(profile)
	if err != nil {
		return "", fmt.Errorf("failed to initialize storage backend: %w", err)
	}

	// Load clusters using the same method as the TUI
	clusterStates, err := backend.LoadAllClusterStates()
	if err != nil {
		return "", fmt.Errorf("failed to load clusters: %w", err)
	}
	
	// Extract clusters from states
	var clusters []models.K3sCluster
	for _, state := range clusterStates {
		if state != nil {
			clusters = append(clusters, state.Cluster)
		}
	}

	if len(clusters) == 0 {
		return "", fmt.Errorf("no clusters found")
	}

	// Display clusters
	fmt.Println("\nAvailable clusters:")
	fmt.Println(strings.Repeat("─", 80))
	for i, cluster := range clusters {
		statusColor := "31" // red
		if cluster.Status == "running" {
			statusColor = "32" // green
		} else if cluster.Status == "creating" || cluster.Status == "updating" {
			statusColor = "33" // yellow
		}
		
		fmt.Printf("  %d) %-30s \033[%sm%-10s\033[0m %s %s %d nodes\n", 
			i+1, cluster.Name, statusColor, cluster.Status, 
			cluster.Mode, cluster.Region, 
			len(cluster.MasterNodes)+len(cluster.WorkerNodes))
	}
	fmt.Println(strings.Repeat("─", 80))
	
	// Get user selection
	fmt.Print("Select cluster (1-", len(clusters), ") or q to quit: ")
	var input string
	fmt.Scanln(&input)
	
	if input == "q" || input == "Q" {
		return "", fmt.Errorf("cancelled")
	}
	
	// Parse selection
	var selection int
	if _, err := fmt.Sscanf(input, "%d", &selection); err != nil {
		return "", fmt.Errorf("invalid selection")
	}
	
	if selection < 1 || selection > len(clusters) {
		return "", fmt.Errorf("selection out of range")
	}
	
	return clusters[selection-1].Name, nil
}

// getOrSelectCluster returns the provided cluster name or prompts for selection
func getOrSelectCluster(clusterName string, action string) (string, error) {
	if clusterName != "" {
		return clusterName, nil
	}
	
	// Check if we're in a TTY for interactive selection
	if fileInfo, _ := os.Stdin.Stat(); (fileInfo.Mode() & os.ModeCharDevice) != 0 {
		prompt := fmt.Sprintf(" Select cluster to %s ", action)
		return selectCluster(prompt)
	}
	
	// Fallback to simple selection
	return selectClusterSimple()
}

// ClusterInfo holds basic cluster information for selection
type ClusterInfo struct {
	Name         string
	Status       models.ClusterStatus
	Mode         models.ClusterMode
	Region       string
	NodeCount    int
	InstanceID   string // For SSM connection
}