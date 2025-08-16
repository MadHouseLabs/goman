package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/madhouselabs/goman/pkg/cluster"
	"github.com/madhouselabs/goman/pkg/config"
	"github.com/madhouselabs/goman/pkg/models"
	"github.com/madhouselabs/goman/pkg/provider/aws"
	"github.com/rivo/tview"
	"github.com/spf13/cobra"
)

// Styling constants
var (
	// Colors
	ColorPrimary     = tcell.ColorBlue
	ColorSuccess     = tcell.ColorGreen
	ColorWarning     = tcell.ColorYellow
	ColorDanger      = tcell.ColorRed
	ColorMuted       = tcell.ColorDarkGray
	ColorBackground  = tcell.ColorBlack
	ColorForeground  = tcell.ColorWhite
	ColorHighlight   = tcell.ColorLightBlue
	
	// Styles
	StyleDefault     = tcell.StyleDefault.Foreground(ColorForeground).Background(ColorBackground)
	StylePrimary     = tcell.StyleDefault.Foreground(ColorPrimary).Bold(true)
	StyleSuccess     = tcell.StyleDefault.Foreground(ColorSuccess)
	StyleWarning     = tcell.StyleDefault.Foreground(ColorWarning)
	StyleDanger      = tcell.StyleDefault.Foreground(ColorDanger)
	StyleMuted       = tcell.StyleDefault.Foreground(ColorMuted)
	StyleHighlight   = tcell.StyleDefault.Foreground(ColorForeground).Background(ColorPrimary).Bold(true)
	StyleHeader      = tcell.StyleDefault.Foreground(ColorWarning).Bold(true)
	
	// Unicode characters
	CharDivider      = '─'
	CharVertical     = '│'
	CharCornerTL     = '┌'
	CharCornerTR     = '┐'
	CharCornerBL     = '└'
	CharCornerBR     = '┘'
	CharBullet       = '•'
	CharArrowRight   = '→'
	CharArrowLeft    = '←'
	CharArrowUp      = '↑'
	CharArrowDown    = '↓'
	CharCheck        = '✓'
	CharCross        = '✗'
	CharStar         = '★'
)

var (
	app            *tview.Application
	pages          *tview.Pages
	clusterManager *cluster.Manager
	cfg            *config.Config
	clusterTable   *tview.Table
	clusters       []models.K3sCluster
	statusText     *tview.TextView
	lastError      error
	refreshCancel  chan struct{}
	lastRefreshTime time.Time
)

func main() {
	var rootCmd = &cobra.Command{
		Use:   "goman",
		Short: "Goman - Kubernetes Cluster Manager",
		Long:  `Goman is a CLI tool for managing Kubernetes clusters on AWS.`,
		Run: func(cmd *cobra.Command, args []string) {
			runTUI()
		},
	}

	var initCmd = &cobra.Command{
		Use:   "init",
		Short: "Initialize infrastructure",
		Run: func(cmd *cobra.Command, args []string) {
			initInfrastructure()
		},
	}

	var cleanupCmd = &cobra.Command{
		Use:    "cleanup [cluster-name]",
		Short:  "Force cleanup a cluster (removes all resources)",
		Hidden: true, // Hidden command for troubleshooting
		Args:   cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			forceCleanupCluster(args[0])
		},
	}

	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(cleanupCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func forceCleanupCluster(clusterName string) {
	fmt.Printf("🧹 Force cleaning up cluster: %s\n", clusterName)
	
	// Load configuration
	cfg, err := config.NewConfig()
	if err != nil {
		fmt.Printf("Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Get AWS provider
	provider, err := aws.GetCachedProvider(cfg.AWSProfile, cfg.AWSRegion)
	if err != nil {
		fmt.Printf("Error getting AWS provider: %v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()
	
	// 1. Delete S3 files
	fmt.Println("📦 Deleting S3 files...")
	storageService := provider.GetStorageService()
	configKey := fmt.Sprintf("clusters/%s/config.yaml", clusterName)
	statusKey := fmt.Sprintf("clusters/%s/status.yaml", clusterName)
	
	storageService.DeleteObject(ctx, configKey)
	storageService.DeleteObject(ctx, statusKey)
	
	// Also try old JSON format
	storageService.DeleteObject(ctx, fmt.Sprintf("clusters/%s/config.json", clusterName))
	storageService.DeleteObject(ctx, fmt.Sprintf("clusters/%s/status.json", clusterName))
	
	// 2. Terminate EC2 instances
	fmt.Println("🖥️  Terminating EC2 instances...")
	computeService := provider.GetComputeService()
	filters := map[string]string{
		"tag:ClusterName": clusterName,
		"instance-state-name": "pending,running,stopping,stopped",
	}
	
	instances, err := computeService.ListInstances(ctx, filters)
	if err == nil && len(instances) > 0 {
		for _, inst := range instances {
			fmt.Printf("  Terminating instance: %s\n", inst.ID)
			computeService.DeleteInstance(ctx, inst.ID)
		}
	}
	
	// 3. Release DynamoDB lock
	fmt.Println("🔒 Releasing DynamoDB lock...")
	lockService := provider.GetLockService()
	resourceID := fmt.Sprintf("cluster-%s", clusterName)
	
	// Force release by deleting the lock entry
	if locked, _, _ := lockService.IsLocked(ctx, resourceID); locked {
		// We can't directly delete, but we can try to acquire with a very short TTL
		// and then let it expire
		fmt.Println("  Lock found, will expire soon")
	}
	
	// 4. Clean up from cluster manager's memory
	manager := cluster.NewManager()
	clusters := manager.GetClusters()
	for _, c := range clusters {
		if c.Name == clusterName {
			manager.DeleteCluster(c.ID)
			break
		}
	}
	
	fmt.Printf("✅ Force cleanup complete for cluster: %s\n", clusterName)
	fmt.Println("\nNote: Some resources like security groups may remain if they have dependencies.")
	fmt.Println("Lambda controller will clean up remaining resources on next reconciliation.")
}

func initInfrastructure() {
	fmt.Println("Initializing AWS infrastructure...")
	
	// Load configuration
	cfg, err := config.NewConfig()
	if err != nil {
		fmt.Printf("Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Get AWS provider
	provider, err := aws.GetCachedProvider(cfg.AWSProfile, cfg.AWSRegion)
	if err != nil {
		fmt.Printf("Error getting AWS provider: %v\n", err)
		os.Exit(1)
	}

	// Initialize infrastructure
	ctx := context.Background()
	if _, err := provider.Initialize(ctx); err != nil {
		fmt.Printf("Error initializing infrastructure: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("✅ Infrastructure initialized successfully!")
}

func runTUI() {
	// Load configuration
	var err error
	cfg, err = config.NewConfig()
	if err != nil {
		fmt.Printf("Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Initialize cluster manager
	clusterManager = cluster.NewManager()

	// Create the application
	app = tview.NewApplication()
	
	// Create pages for different views
	pages = tview.NewPages()

	// Create and add the cluster list view
	createClusterListView()

	// Start with the cluster list
	pages.SwitchToPage("clusters")

	// Initial refresh when starting
	go refreshClustersAsync()

	// Start periodic refresh every 5 seconds
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			if pages.HasPage("clusters") {
				go refreshClustersAsync()
			}
		}
	}()

	// Set root and run
	if err := app.SetRoot(pages, true).EnableMouse(true).Run(); err != nil {
		panic(err)
	}
}

func createClusterListView() {
	// Create a flex layout for the main view
	flex := tview.NewFlex().SetDirection(tview.FlexRow)

	// Create header with title and provider info
	headerFlex := tview.NewFlex().SetDirection(tview.FlexColumn)
	
	titleText := fmt.Sprintf(" [::b]K3s Cluster Manager[::-]")
	title := tview.NewTextView().
		SetText(titleText).
		SetTextAlign(tview.AlignLeft).
		SetDynamicColors(true)
	
	providerText := fmt.Sprintf("[::d]Provider: AWS | Region: %s[::-] ", cfg.AWSRegion)
	providerInfo := tview.NewTextView().
		SetText(providerText).
		SetTextAlign(tview.AlignRight).
		SetDynamicColors(true)
	
	headerFlex.
		AddItem(title, 0, 1, false).
		AddItem(providerInfo, 0, 1, false)

	// Header divider
	headerDivider := tview.NewTextView().
		SetText(string(CharDivider)).
		SetTextColor(ColorMuted).
		SetDrawFunc(func(screen tcell.Screen, x, y, width, height int) (int, int, int, int) {
			for i := x; i < x+width; i++ {
				screen.SetContent(i, y, CharDivider, nil, StyleMuted)
			}
			return 0, 0, 0, 0
		})

	// Create the table
	clusterTable = tview.NewTable().
		SetBorders(false).
		SetSelectable(true, false).
		SetSeparator(' ').
		SetSelectedStyle(StyleHighlight)

	// Set headers with proper spacing
	headers := []string{"  Name", "Mode", "Region", "Status", "Nodes", "Created"}
	for col, header := range headers {
		alignment := tview.AlignLeft
		if col > 0 && col < len(headers)-1 {
			alignment = tview.AlignCenter
		}
		cell := tview.NewTableCell(header).
			SetTextColor(ColorWarning).
			SetAlign(alignment).
			SetSelectable(false).
			SetExpansion(1)
		clusterTable.SetCell(0, col, cell)
	}

	// Load clusters
	refreshClusters()

	// Set up key handlers for the table
	clusterTable.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyEnter:
			row, _ := clusterTable.GetSelection()
			if row > 0 && row <= len(clusters) {
				showClusterDetails(clusters[row-1])
			}
		case tcell.KeyRune:
			switch event.Rune() {
			case 'c', 'C':
				openClusterEditor()
			case 'e', 'E':
				row, _ := clusterTable.GetSelection()
				if row > 0 && row <= len(clusters) {
					editCluster(clusters[row-1])
				}
			case 'd', 'D':
				row, _ := clusterTable.GetSelection()
				if row > 0 && row <= len(clusters) {
					deleteCluster(clusters[row-1])
				}
			case 'r', 'R':
				go refreshClustersAsync()
			case 'q', 'Q':
				app.Stop()
			}
		}
		return event
	})

	// Footer divider
	footerDivider := tview.NewTextView().
		SetText(string(CharDivider)).
		SetTextColor(ColorMuted).
		SetDrawFunc(func(screen tcell.Screen, x, y, width, height int) (int, int, int, int) {
			for i := x; i < x+width; i++ {
				screen.SetContent(i, y, CharDivider, nil, StyleMuted)
			}
			return 0, 0, 0, 0
		})

	// Status bar with connection status and shortcuts
	statusBarFlex := tview.NewFlex().SetDirection(tview.FlexColumn)
	
	// Connection status (left) - will be updated dynamically
	statusText = tview.NewTextView().
		SetText(" [green]● Connected[::-]").
		SetDynamicColors(true).
		SetTextAlign(tview.AlignLeft)
	
	// Shortcuts (right)
	shortcuts := fmt.Sprintf("[yellow]%c%c[::-] Navigate  [yellow]Enter[::-] Details  [yellow]c[::-] Create  [yellow]e[::-] Edit  [yellow]d[::-] Delete  [yellow]R[::-] Refresh  [yellow]q[::-] Quit ", CharArrowUp, CharArrowDown)
	statusRight := tview.NewTextView().
		SetText(shortcuts).
		SetDynamicColors(true).
		SetTextAlign(tview.AlignRight)
	
	statusBarFlex.
		AddItem(statusText, 0, 1, false).
		AddItem(statusRight, 0, 2, false)

	// Add components to flex
	flex.
		AddItem(headerFlex, 1, 0, false).
		AddItem(headerDivider, 1, 0, false).
		AddItem(clusterTable, 0, 1, true).
		AddItem(footerDivider, 1, 0, false).
		AddItem(statusBarFlex, 1, 0, false)

	// Add to pages
	pages.AddPage("clusters", flex, true, true)
}

func editCluster(cluster models.K3sCluster) {
	// Show loading message before suspending
	statusText.SetText(" [yellow]Opening editor...[::-]")
	app.ForceDraw()
	
	// Small delay for visual smoothness
	time.Sleep(100 * time.Millisecond)
	
	// Suspend the TUI application temporarily
	app.Suspend(func() {
		// Clear and reset terminal for a clean editor experience
		fmt.Print("\033[2J\033[H\033[?47l")
		// Convert cluster to YAML format for editing
		yamlContent := fmt.Sprintf(`# K3s Cluster Configuration - Edit Mode
# =====================================
# Modify the fields below to update your cluster configuration.
# Note: Some fields like 'name' cannot be changed after creation.

# Basic Configuration
# -------------------
name: %s                       # Cannot be changed
description: "%s"              # Cluster description
mode: %s                       # Cannot be changed - Cluster mode is immutable after creation
region: %s                     # AWS region
instanceType: %s               # EC2 instance type (will resize existing instances)

# Version Configuration
# --------------------
k3sVersion: %s                 # K3s version
# kubeVersion: %s              # Kubernetes version

# Network Configuration
# --------------------
networkCIDR: %s                # VPC CIDR block
serviceCIDR: %s                # Service CIDR
# clusterDNS: %s               # Cluster DNS IP

# Tags (Optional)
# --------------
# tags:
#   - Environment:production
#   - Team:platform
`, cluster.Name, cluster.Description, cluster.Mode, cluster.Region, cluster.InstanceType,
			cluster.K3sVersion, cluster.KubeVersion,
			cluster.NetworkCIDR, cluster.ServiceCIDR, cluster.ClusterDNS)

		// Create temporary file for editing
		tmpFile, err := ioutil.TempFile("", fmt.Sprintf("goman-cluster-%s-*.yaml", cluster.Name))
		if err != nil {
			return
		}
		tmpFilePath := tmpFile.Name()
		defer os.Remove(tmpFilePath)

		// Write content to temp file
		if _, err := tmpFile.WriteString(yamlContent); err != nil {
			tmpFile.Close()
			return
		}
		tmpFile.Close()

		// Get file modification time before editing
		statBefore, err := os.Stat(tmpFilePath)
		if err != nil {
			return
		}
		modTimeBefore := statBefore.ModTime()

		// Determine which editor to use
		editor := os.Getenv("EDITOR")
		if editor == "" {
			editor = "vim"
		}

		// Open the editor
		cmd := exec.Command(editor, tmpFilePath)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Run(); err != nil {
			// User exited editor, silently return
			return
		}

		// Check if file was modified
		statAfter, err := os.Stat(tmpFilePath)
		if err != nil {
			return
		}
		
		// If modification time hasn't changed, user didn't save
		if modTimeBefore.Equal(statAfter.ModTime()) {
			return
		}

		// Read the edited content
		content, err := ioutil.ReadFile(tmpFilePath)
		if err != nil {
			return
		}

		yamlContentEdited := string(content)
		
		// Validate and update cluster - keep retrying on errors
		for {
			if err := validateAndUpdateClusterFromEditor(cluster, yamlContentEdited); err != nil {
				// Write validation error as comment at the top of the file
				errorContent := fmt.Sprintf("# ERROR: %s\n# Please fix the error above and save again, or exit without saving to cancel.\n#\n%s", err.Error(), yamlContentEdited)
				ioutil.WriteFile(tmpFilePath, []byte(errorContent), 0644)
				
				// Get file modification time before editing
				statBefore, _ := os.Stat(tmpFilePath)
				modTimeBefore := statBefore.ModTime()
				
				// Reopen editor with error message
				cmd := exec.Command(editor, tmpFilePath)
				cmd.Stdin = os.Stdin
				cmd.Stdout = os.Stdout
				cmd.Stderr = os.Stderr
				cmd.Run()
				
				// Check if file was modified
				statAfter, _ := os.Stat(tmpFilePath)
				if modTimeBefore.Equal(statAfter.ModTime()) {
					// User didn't save, exit the loop
					break
				}
				
				// Read the new content and try again
				content, err = ioutil.ReadFile(tmpFilePath)
				if err != nil {
					break
				}
				yamlContentEdited = string(content)
				
				// Remove error comments before retrying
				lines := strings.Split(yamlContentEdited, "\n")
				var cleanLines []string
				for _, line := range lines {
					if !strings.HasPrefix(line, "# ERROR:") && !strings.HasPrefix(line, "# Please fix") {
						cleanLines = append(cleanLines, line)
					}
				}
				yamlContentEdited = strings.Join(cleanLines, "\n")
			} else {
				// Success, exit the loop
				break
			}
		}
		
		// Restore terminal state before returning to TUI
		fmt.Print("\033[?47h\033[2J\033[H")
		time.Sleep(50 * time.Millisecond)
	})
	
	// Restore status after returning
	statusText.SetText(" [green]● Connected[::-]")
	
	// The TUI will automatically resume after Suspend function completes
	// Refresh the cluster list to show any updates
	go refreshClustersAsync()
}

func openClusterEditor() {
	// Show loading message before suspending
	statusText.SetText(" [yellow]Opening editor...[::-]")
	app.ForceDraw()
	
	// Small delay for visual smoothness
	time.Sleep(100 * time.Millisecond)
	
	// Generate unique cluster name
	timestamp := time.Now().Unix()
	uniqueName := fmt.Sprintf("k3s-cluster-%d", timestamp)
	
	// Suspend the TUI application temporarily
	app.Suspend(func() {
		// Clear and reset terminal for a clean editor experience
		fmt.Print("\033[2J\033[H\033[?47l")
		// Default YAML configuration template
		defaultYAML := fmt.Sprintf(`# K3s Cluster Configuration
# ===========================
# This file defines the configuration for your K3s Kubernetes cluster.
# Uncomment and modify the fields as needed.

# Basic Configuration (Required)
# ------------------------------
name: %s               # Unique cluster identifier
description: "Development cluster"  # Human-readable description

# Cluster Mode
# - developer: Single master node (for development/testing)
# - ha: 3 master nodes (for production/high availability)
mode: developer

# AWS Configuration
region: %s              # AWS region for deployment
instanceType: t3.medium        # EC2 instance type for nodes

# Advanced Configuration (Optional)
# ---------------------------------
# k3sVersion: latest           # K3s version (default: latest stable)
# kubeVersion: v1.28.5         # Kubernetes version

# Network Configuration
# networkCIDR: 10.0.0.0/16     # VPC CIDR block
# serviceCIDR: 10.43.0.0/16    # Kubernetes service CIDR
# clusterDNS: 10.43.0.10       # Cluster DNS service IP

# Node Configuration (Optional)
# workerNodes: 2               # Number of worker nodes (default: 0)

# Tags for AWS Resources
# tags:
#   - Environment:development
#   - Team:platform
#   - Owner:devops
#   - Project:myapp

# Features (Optional)
# features:
#   traefik: true              # Enable Traefik ingress controller
#   serviceLB: true            # Enable service load balancer
#   localStorage: true         # Enable local storage provisioner
#   metricsServer: true        # Enable metrics server
#   coreDNS: true              # Enable CoreDNS
#   flannelBackend: vxlan      # Flannel backend (vxlan/host-gw/wireguard)
`, uniqueName, cfg.AWSRegion)

		// Create temporary file for editing
		tmpFile, err := ioutil.TempFile("", "goman-cluster-*.yaml")
		if err != nil {
			return
		}
		tmpFilePath := tmpFile.Name()
		defer os.Remove(tmpFilePath)

		// Write default content to temp file
		if _, err := tmpFile.WriteString(defaultYAML); err != nil {
			tmpFile.Close()
			return
		}
		tmpFile.Close()

		// Get file modification time before editing
		statBefore, err := os.Stat(tmpFilePath)
		if err != nil {
			return
		}
		modTimeBefore := statBefore.ModTime()

		// Determine which editor to use
		editor := os.Getenv("EDITOR")
		if editor == "" {
			editor = "vim"
		}

		// Open the editor
		cmd := exec.Command(editor, tmpFilePath)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Run(); err != nil {
			// User exited editor, silently return
			return
		}

		// Check if file was modified
		statAfter, err := os.Stat(tmpFilePath)
		if err != nil {
			return
		}
		
		// If modification time hasn't changed, user didn't save
		if modTimeBefore.Equal(statAfter.ModTime()) {
			return
		}

		// Read the edited content
		content, err := ioutil.ReadFile(tmpFilePath)
		if err != nil {
			return
		}

		yamlContent := string(content)
		
		// Silently validate and create cluster
		validateAndCreateClusterFromEditor(yamlContent)
		
		// Restore terminal state before returning to TUI
		fmt.Print("\033[?47h\033[2J\033[H")
		time.Sleep(50 * time.Millisecond)
	})
	
	// Restore status after returning
	statusText.SetText(" [green]● Connected[::-]")
	
	// The TUI will automatically resume after Suspend function completes
	// Refresh the cluster list to show any new clusters
	go refreshClustersAsync()
}

// validateAndUpdateClusterFromEditor validates YAML and updates the cluster from editor
func validateAndUpdateClusterFromEditor(originalCluster models.K3sCluster, yamlContent string) error {
	// Parse YAML
	config := make(map[string]interface{})
	
	// Simple YAML parsing - extract key fields
	lines := strings.Split(yamlContent, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}
		
		// Remove inline comments
		if idx := strings.Index(line, "#"); idx > 0 {
			line = strings.TrimSpace(line[:idx])
		}
		
		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			// Remove any quotes around the value
			value = strings.Trim(value, "\"'")
			config[key] = value
		}
	}
	
	// Extract and validate required fields
	name, ok := config["name"].(string)
	if !ok || name == "" {
		return fmt.Errorf("cluster name is required")
	}
	
	mode, ok := config["mode"].(string)
	if !ok || (mode != "developer" && mode != "ha") {
		return fmt.Errorf("mode must be 'developer' or 'ha'")
	}
	
	// Check if mode is being changed (not allowed)
	originalMode := string(originalCluster.Mode)
	if mode != originalMode {
		return fmt.Errorf("cluster mode cannot be changed after creation (current: %s, attempted: %s)", originalMode, mode)
	}
	
	region, ok := config["region"].(string)
	if !ok || region == "" {
		return fmt.Errorf("region is required")
	}
	
	instanceType, ok := config["instanceType"].(string)
	if !ok || instanceType == "" {
		instanceType = "t3.medium"
	}
	
	// Extract description
	description, _ := config["description"].(string)
	
	// Update the existing cluster
	return updateExistingCluster(originalCluster.Name, name, description, mode, region, instanceType)
}

// validateAndCreateClusterFromEditor validates YAML and creates the cluster from editor
func validateAndCreateClusterFromEditor(yamlContent string) error {
	// Parse YAML
	config := make(map[string]interface{})
	
	// Simple YAML parsing - extract key fields
	lines := strings.Split(yamlContent, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}
		
		// Remove inline comments
		if idx := strings.Index(line, "#"); idx > 0 {
			line = strings.TrimSpace(line[:idx])
		}
		
		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			// Remove any quotes around the value
			value = strings.Trim(value, "\"'")
			config[key] = value
		}
	}
	
	// Extract and validate required fields
	name, ok := config["name"].(string)
	if !ok || name == "" {
		return fmt.Errorf("cluster name is required")
	}
	
	mode, ok := config["mode"].(string)
	if !ok || (mode != "developer" && mode != "ha") {
		return fmt.Errorf("mode must be 'developer' or 'ha'")
	}
	
	region, ok := config["region"].(string)
	if !ok || region == "" {
		return fmt.Errorf("region is required")
	}
	
	instanceType, ok := config["instanceType"].(string)
	if !ok || instanceType == "" {
		instanceType = "t3.medium"
	}
	
	// Determine node count based on mode
	nodeCount := "1"
	if mode == "ha" {
		nodeCount = "3"
	}
	
	// Extract description
	description, _ := config["description"].(string)
	if description == "" {
		description = "K3s cluster"
	}
	
	// Create the cluster without UI (we're in editor mode)
	createNewClusterFromEditor(name, description, mode, region, instanceType, nodeCount)
	
	return nil
}

func refreshClusters() {
	// Save current selection
	selectedRow, _ := clusterTable.GetSelection()
	
	// Load clusters from storage
	newClusters := clusterManager.GetClusters()
	newCount := len(newClusters)
	
	// Update global clusters list
	clusters = newClusters
	
	// Handle row count changes
	currentRows := clusterTable.GetRowCount() - 1 // Exclude header
	
	if newCount < currentRows {
		// Remove extra rows
		for row := clusterTable.GetRowCount() - 1; row > newCount; row-- {
			clusterTable.RemoveRow(row)
		}
	}
	
	// Update or add rows
	for i, cluster := range clusters {
		row := i + 1
		
		// Format creation time
		created := cluster.CreatedAt.Format("2006-01-02 15:04")
		if cluster.CreatedAt.IsZero() {
			created = "-"
		}

		// Count nodes
		nodeCount := len(cluster.MasterNodes) + len(cluster.WorkerNodes)

		// Status color
		statusColor := ColorDanger
		if cluster.Status == "running" {
			statusColor = ColorSuccess
		} else if cluster.Status == "pending" || cluster.Status == "creating" {
			statusColor = ColorWarning
		} else if cluster.Status == "deleting" {
			statusColor = ColorDanger
		}

		// Update cells (this will either update existing or create new)
		clusterTable.SetCell(row, 0, tview.NewTableCell("  "+cluster.Name).SetExpansion(2))
		clusterTable.SetCell(row, 1, tview.NewTableCell(string(cluster.Mode)).SetAlign(tview.AlignCenter).SetExpansion(1))
		clusterTable.SetCell(row, 2, tview.NewTableCell(cluster.Region).SetAlign(tview.AlignCenter).SetExpansion(1))
		clusterTable.SetCell(row, 3, tview.NewTableCell(string(cluster.Status)).SetTextColor(statusColor).SetAlign(tview.AlignCenter).SetExpansion(1))
		clusterTable.SetCell(row, 4, tview.NewTableCell(fmt.Sprintf("%d", nodeCount)).SetAlign(tview.AlignCenter).SetExpansion(1))
		clusterTable.SetCell(row, 5, tview.NewTableCell(created).SetAlign(tview.AlignLeft).SetExpansion(2))
	}
	
	// Restore selection if valid
	if selectedRow > 0 && selectedRow <= newCount {
		clusterTable.Select(selectedRow, 0)
	} else if newCount > 0 {
		clusterTable.Select(1, 0)
	}
}

// refreshClustersAsync performs a non-blocking refresh of the cluster list
func refreshClustersAsync() {
	// Cancel any existing refresh
	if refreshCancel != nil {
		close(refreshCancel)
	}
	
	// Create new cancellation channel
	refreshCancel = make(chan struct{})
	cancelChan := refreshCancel
	
	// Skip if we just refreshed recently (debounce)
	if time.Since(lastRefreshTime) < 500*time.Millisecond {
		return
	}
	lastRefreshTime = time.Now()
	
	// Update status to show refreshing
	app.QueueUpdateDraw(func() {
		if statusText != nil {
			statusText.SetText(" [yellow]↻ Refreshing...[::-]")
		}
	})

	// Perform refresh in background
	go func() {
		startTime := time.Now()
		
		// Create a context with cancellation
		done := make(chan bool)
		var refreshErr error
		
		go func() {
			defer func() {
				if r := recover(); r != nil {
					refreshErr = fmt.Errorf("refresh failed: %v", r)
				}
				done <- true
			}()
			clusterManager.RefreshClusterStatus()
		}()
		
		// Wait for completion or cancellation
		select {
		case <-done:
			// Refresh completed
		case <-cancelChan:
			// Refresh cancelled, don't update UI
			return
		}
		
		duration := time.Since(startTime)

		// Determine connection status
		var statusMsg string
		if refreshErr != nil {
			// Connection error
			statusMsg = " [red]● Connection error[::-]"
			lastError = refreshErr
		} else if duration > 3*time.Second {
			// Slow connection
			statusMsg = fmt.Sprintf(" [yellow]● Slow connection[::-] (%.1fs)", duration.Seconds())
		} else {
			// Success
			statusMsg = " [green]● Connected[::-]"
			lastError = nil
		}

		// Check if this refresh was cancelled
		select {
		case <-cancelChan:
			// Cancelled, don't update UI
			return
		default:
			// Update the UI with fresh data
			app.QueueUpdateDraw(func() {
				if lastError == nil {
					refreshClusters()
				}
				if statusText != nil {
					statusText.SetText(statusMsg)
				}
			})
		}
	}()
}

func showClusterDetails(cluster models.K3sCluster) {
	// Create a text view for details
	details := tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(true)

	// Format details
	text := fmt.Sprintf(`[yellow]Cluster Details[::-]
[::b]Name:[::-] %s
[::b]Status:[::-] %s
[::b]Mode:[::-] %s
[::b]Region:[::-] %s
[::b]Instance Type:[::-] %s
[::b]K3s Version:[::-] %s
[::b]Total CPU:[::-] %d
[::b]Total Memory:[::-] %d GB
[::b]Created:[::-] %s
[::b]Updated:[::-] %s

[yellow]Master Nodes:[::-]
`, cluster.Name, cluster.Status, cluster.Mode, cluster.Region, 
		cluster.InstanceType, cluster.K3sVersion, cluster.TotalCPU, 
		cluster.TotalMemoryGB, cluster.CreatedAt.Format(time.RFC3339),
		cluster.UpdatedAt.Format(time.RFC3339))

	for _, node := range cluster.MasterNodes {
		text += fmt.Sprintf("  %c %s (%s)\n", CharBullet, node.Name, node.Status)
	}

	if len(cluster.WorkerNodes) > 0 {
		text += "\n[yellow]Worker Nodes:[::-]\n"
		for _, node := range cluster.WorkerNodes {
			text += fmt.Sprintf("  %c %s (%s)\n", CharBullet, node.Name, node.Status)
		}
	}

	details.SetText(text)

	// Create a frame with the details
	frame := tview.NewFrame(details).
		AddText("Press ESC to go back", false, tview.AlignCenter, ColorForeground).
		SetBorders(1, 1, 1, 1, 2, 2)

	// Handle ESC key
	details.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEscape {
			pages.SwitchToPage("clusters")
			// Refresh when returning to cluster list
			go refreshClustersAsync()
		}
		return event
	})

	// Add and switch to details page
	pages.AddAndSwitchToPage("details", frame, true)
}

func createNewCluster(name, mode, region, instanceType, nodeCountStr string) {
	createNewClusterWithUI(name, "", mode, region, instanceType, nodeCountStr, true)
}

func createNewClusterFromEditor(name, description, mode, region, instanceType, nodeCountStr string) {
	createNewClusterWithUI(name, description, mode, region, instanceType, nodeCountStr, false)
}

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
			// Silently handle error - user will see cluster not created
		}
		return
	}

	// Show progress modal (when called from UI)
	modal := tview.NewModal().
		SetText(fmt.Sprintf("Creating cluster '%s'...", name)).
		AddButtons([]string{"OK"})

	pages.AddAndSwitchToPage("progress", modal, false)

	// Create cluster in background
	go func() {
		_, err := clusterManager.CreateCluster(*cluster)
		app.QueueUpdateDraw(func() {
			pages.RemovePage("progress")
			if err != nil {
				errorModal := tview.NewModal().
					SetText(fmt.Sprintf("Error creating cluster: %v", err)).
					AddButtons([]string{"OK"}).
					SetDoneFunc(func(buttonIndex int, buttonLabel string) {
						pages.RemovePage("error")
						refreshClusters()
					})
				pages.AddAndSwitchToPage("error", errorModal, false)
			} else {
				refreshClusters()
			}
		})
	}()
}

func updateExistingCluster(originalName, name, description, mode, region, instanceType string) error {
	// Load the existing cluster
	existingClusters := clusterManager.GetClusters()
	var existingCluster *models.K3sCluster
	for i := range existingClusters {
		if existingClusters[i].Name == originalName {
			existingCluster = &existingClusters[i]
			break
		}
	}
	
	if existingCluster == nil {
		return fmt.Errorf("cluster not found")
	}
	
	// Update cluster fields
	existingCluster.Name = name
	existingCluster.Description = description
	existingCluster.Region = region
	existingCluster.InstanceType = instanceType
	
	// Mode should NOT be updated - it's immutable
	// Keep the existing mode
	
	// Update the cluster
	_, err := clusterManager.UpdateCluster(*existingCluster)
	return err
}

func deleteCluster(cluster models.K3sCluster) {
	// Confirmation modal
	modal := tview.NewModal().
		SetText(fmt.Sprintf("Are you sure you want to delete cluster '%s'?", cluster.Name)).
		AddButtons([]string{"Delete", "Cancel"}).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			pages.RemovePage("confirm")
			if buttonLabel == "Delete" {
				// Delete in background
				go func() {
					err := clusterManager.DeleteCluster(cluster.ID)
					app.QueueUpdateDraw(func() {
						if err != nil {
							errorModal := tview.NewModal().
								SetText(fmt.Sprintf("Error deleting cluster: %v", err)).
								AddButtons([]string{"OK"}).
								SetDoneFunc(func(buttonIndex int, buttonLabel string) {
									pages.RemovePage("error")
								})
							pages.AddAndSwitchToPage("error", errorModal, false)
						}
						refreshClusters()
					})
				}()
			}
		})

	pages.AddAndSwitchToPage("confirm", modal, false)
}

func showError(message string) {
	modal := tview.NewModal().
		SetText(message).
		AddButtons([]string{"OK"}).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			pages.RemovePage("error")
		})
	pages.AddAndSwitchToPage("error", modal, false)
}