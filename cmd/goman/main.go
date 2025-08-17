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
	// Colors - Dracula theme (modified with blue primary)
	ColorPrimary     = tcell.NewRGBColor(139, 233, 253)  // Blue (dracula cyan/blue)
	ColorSuccess     = tcell.NewRGBColor(80, 250, 123)   // Green (dracula green)
	ColorWarning     = tcell.NewRGBColor(241, 250, 140)  // Yellow (dracula yellow)
	ColorDanger      = tcell.NewRGBColor(255, 85, 85)    // Red (dracula red)
	ColorMuted       = tcell.NewRGBColor(98, 114, 164)   // Comment (dracula comment)
	ColorBackground  = tcell.NewRGBColor(40, 42, 54)     // Background (dracula bg)
	ColorForeground  = tcell.NewRGBColor(248, 248, 242)  // Foreground (dracula fg)
	ColorSelection   = tcell.NewRGBColor(68, 71, 90)     // Current Line (dracula current line)
	ColorAccent      = tcell.NewRGBColor(189, 147, 249)  // Purple (dracula purple)
	ColorOrange      = tcell.NewRGBColor(255, 184, 108)  // Orange (dracula orange)
	ColorPink        = tcell.NewRGBColor(255, 121, 198)  // Pink (dracula pink)
	
	// Styles
	StyleDefault     = tcell.StyleDefault.Foreground(ColorForeground).Background(ColorBackground)
	StylePrimary     = tcell.StyleDefault.Foreground(ColorPrimary).Bold(true)
	StyleSuccess     = tcell.StyleDefault.Foreground(ColorSuccess)
	StyleWarning     = tcell.StyleDefault.Foreground(ColorWarning)
	StyleDanger      = tcell.StyleDefault.Foreground(ColorDanger)
	StyleMuted       = tcell.StyleDefault.Foreground(ColorMuted)
	StyleHighlight   = tcell.StyleDefault.Foreground(ColorPrimary).Background(ColorSelection).Bold(true)
	StyleHeader      = tcell.StyleDefault.Foreground(ColorPrimary).Bold(true)
	StyleAccent      = tcell.StyleDefault.Foreground(ColorAccent)
	
	// Color tags for tview dynamic colors (Dracula hex values)
	TagPrimary       = "[#8be9fd]"  // Dracula cyan/blue
	TagSuccess       = "[#50fa7b]"  // Dracula green
	TagWarning       = "[#f1fa8c]"  // Dracula yellow
	TagDanger        = "[#ff5555]"  // Dracula red
	TagMuted         = "[#6272a4]"  // Dracula comment
	TagAccent        = "[#bd93f9]"  // Dracula purple
	TagReset         = "[::-]"
	TagBold          = "[::b]"
	
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
		// Center align Status and Nodes columns (columns 3, 4)
		if col == 3 || col == 4 {
			alignment = tview.AlignCenter
		}
		cell := tview.NewTableCell(header).
			SetTextColor(ColorPrimary).
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
	shortcuts := fmt.Sprintf("[#8be9fd]%c%c[::-] Navigate  [#8be9fd]Enter[::-] Details  [#8be9fd]c[::-] Create  [#8be9fd]e[::-] Edit  [#8be9fd]d[::-] Delete  [#8be9fd]R[::-] Refresh  [#8be9fd]q[::-] Quit ", CharArrowUp, CharArrowDown)
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
	statusText.SetText(fmt.Sprintf(" %sOpening editor...%s", TagWarning, TagReset))
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
	statusText.SetText(fmt.Sprintf(" %sOpening editor...%s", TagWarning, TagReset))
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
		clusterTable.SetCell(row, 1, tview.NewTableCell(string(cluster.Mode)).SetAlign(tview.AlignLeft).SetExpansion(1))
		clusterTable.SetCell(row, 2, tview.NewTableCell(cluster.Region).SetAlign(tview.AlignLeft).SetExpansion(1))
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

// getStatusColor returns the appropriate color for a status
func getStatusColor(status string) tcell.Color {
	switch status {
	case "running":
		return ColorSuccess
	case "creating", "updating":
		return ColorWarning
	case "error", "failed":
		return ColorDanger
	default:
		return ColorMuted
	}
}

func showClusterDetails(cluster models.K3sCluster) {
	// Create main flex container (same structure as listing page)
	flex := tview.NewFlex().SetDirection(tview.FlexRow)
	
	// Create header with title (same style as listing page)
	headerFlex := tview.NewFlex().SetDirection(tview.FlexColumn)
	
	titleText := fmt.Sprintf(" %s%sCluster Details%s%s", TagBold, TagPrimary, TagReset, TagReset)
	title := tview.NewTextView().
		SetText(titleText).
		SetTextAlign(tview.AlignLeft).
		SetDynamicColors(true)
	
	// Cluster name and status on the right
	statusColor := TagSuccess
	statusIcon := "●"
	if cluster.Status == "creating" {
		statusColor = TagWarning
		statusIcon = "◐"
	} else if cluster.Status == "stopped" {
		statusColor = TagMuted
		statusIcon = "○"
	}
	
	clusterInfoText := fmt.Sprintf("%s%s %s %s%s %s", TagBold, cluster.Name, TagReset, statusColor, statusIcon, strings.ToUpper(string(cluster.Status)))
	clusterInfo := tview.NewTextView().
		SetText(clusterInfoText).
		SetTextAlign(tview.AlignRight).
		SetDynamicColors(true)
	
	headerFlex.
		AddItem(title, 0, 1, false).
		AddItem(clusterInfo, 0, 1, false)
	
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
	
	// Calculate totals
	totalNodes := len(cluster.MasterNodes) + len(cluster.WorkerNodes)
	totalCPU := 0
	totalMemory := 0
	totalStorage := 0
	for _, node := range cluster.MasterNodes {
		if node.CPU > 0 {
			totalCPU += node.CPU
		} else {
			totalCPU += 2 // default
		}
		if node.MemoryGB > 0 {
			totalMemory += node.MemoryGB
		} else {
			totalMemory += 4 // default
		}
		if node.StorageGB > 0 {
			totalStorage += node.StorageGB
		} else {
			totalStorage += 20 // default
		}
	}
	for _, node := range cluster.WorkerNodes {
		if node.CPU > 0 {
			totalCPU += node.CPU
		} else {
			totalCPU += 2
		}
		if node.MemoryGB > 0 {
			totalMemory += node.MemoryGB
		} else {
			totalMemory += 4
		}
		if node.StorageGB > 0 {
			totalStorage += node.StorageGB
		} else {
			totalStorage += 20
		}
	}
	
	// Create three sections: Cluster Info, Resources, Metrics
	
	// Section 1: Cluster Info
	clusterInfoTable := tview.NewTable().
		SetBorders(false).
		SetSelectable(false, false).
		SetFixed(1, 2)
	
	// Title spans both columns with padding
	clusterInfoTable.SetCell(0, 0, tview.NewTableCell(" [::b]Cluster Information").
		SetTextColor(ColorPrimary).
		SetAlign(tview.AlignLeft))
	clusterInfoTable.SetCell(0, 1, tview.NewTableCell(""))
	
	// Cluster info data with proper alignment
	infoRow := 1
	clusterInfoTable.SetCell(infoRow, 0, tview.NewTableCell(" Name:").
		SetTextColor(ColorMuted).
		SetAlign(tview.AlignLeft))
	clusterInfoTable.SetCell(infoRow, 1, tview.NewTableCell(cluster.Name).
		SetTextColor(ColorForeground).
		SetAlign(tview.AlignLeft))
	infoRow++
	
	statusColor2 := getStatusColor(string(cluster.Status))
	clusterInfoTable.SetCell(infoRow, 0, tview.NewTableCell(" Status:").
		SetTextColor(ColorMuted).
		SetAlign(tview.AlignLeft))
	clusterInfoTable.SetCell(infoRow, 1, tview.NewTableCell(string(cluster.Status)).
		SetTextColor(statusColor2).
		SetAlign(tview.AlignLeft))
	infoRow++
	
	clusterInfoTable.SetCell(infoRow, 0, tview.NewTableCell(" Mode:").
		SetTextColor(ColorMuted).
		SetAlign(tview.AlignLeft))
	clusterInfoTable.SetCell(infoRow, 1, tview.NewTableCell(strings.ToUpper(string(cluster.Mode))).
		SetTextColor(ColorForeground).
		SetAlign(tview.AlignLeft))
	infoRow++
	
	clusterInfoTable.SetCell(infoRow, 0, tview.NewTableCell(" Region:").
		SetTextColor(ColorMuted).
		SetAlign(tview.AlignLeft))
	clusterInfoTable.SetCell(infoRow, 1, tview.NewTableCell(cluster.Region).
		SetTextColor(ColorForeground).
		SetAlign(tview.AlignLeft))
	infoRow++
	
	clusterInfoTable.SetCell(infoRow, 0, tview.NewTableCell(" Created:").
		SetTextColor(ColorMuted).
		SetAlign(tview.AlignLeft))
	clusterInfoTable.SetCell(infoRow, 1, tview.NewTableCell(cluster.CreatedAt.Format("2006-01-02 15:04")).
		SetTextColor(ColorForeground).
		SetAlign(tview.AlignLeft))
	infoRow++
	
	clusterInfoTable.SetCell(infoRow, 0, tview.NewTableCell(" Updated:").
		SetTextColor(ColorMuted).
		SetAlign(tview.AlignLeft))
	clusterInfoTable.SetCell(infoRow, 1, tview.NewTableCell(cluster.UpdatedAt.Format("2006-01-02 15:04")).
		SetTextColor(ColorForeground).
		SetAlign(tview.AlignLeft))
	
	// Section 2: Resources  
	resourcesTable := tview.NewTable().
		SetBorders(false).
		SetSelectable(false, false).
		SetFixed(1, 2)
	
	// Title spans both columns with padding
	resourcesTable.SetCell(0, 0, tview.NewTableCell(" [::b]Resources").
		SetTextColor(ColorPrimary).
		SetAlign(tview.AlignLeft))
	resourcesTable.SetCell(0, 1, tview.NewTableCell(""))
	
	// Resources data with proper alignment
	resourceRow := 1
	resourcesTable.SetCell(resourceRow, 0, tview.NewTableCell(" Master Nodes:").
		SetTextColor(ColorMuted).
		SetAlign(tview.AlignLeft))
	resourcesTable.SetCell(resourceRow, 1, tview.NewTableCell(fmt.Sprintf("%d", len(cluster.MasterNodes))).
		SetTextColor(ColorAccent).
		SetAlign(tview.AlignLeft))
	resourceRow++
	
	resourcesTable.SetCell(resourceRow, 0, tview.NewTableCell(" Worker Nodes:").
		SetTextColor(ColorMuted).
		SetAlign(tview.AlignLeft))
	resourcesTable.SetCell(resourceRow, 1, tview.NewTableCell(fmt.Sprintf("%d", len(cluster.WorkerNodes))).
		SetTextColor(ColorAccent).
		SetAlign(tview.AlignLeft))
	resourceRow++
	
	resourcesTable.SetCell(resourceRow, 0, tview.NewTableCell(" Total Nodes:").
		SetTextColor(ColorMuted).
		SetAlign(tview.AlignLeft))
	resourcesTable.SetCell(resourceRow, 1, tview.NewTableCell(fmt.Sprintf("%d", totalNodes)).
		SetTextColor(ColorSuccess).
		SetAlign(tview.AlignLeft))
	resourceRow++
	
	resourcesTable.SetCell(resourceRow, 0, tview.NewTableCell(" Total CPU:").
		SetTextColor(ColorMuted).
		SetAlign(tview.AlignLeft))
	resourcesTable.SetCell(resourceRow, 1, tview.NewTableCell(fmt.Sprintf("%d vCPUs", totalCPU)).
		SetTextColor(ColorForeground).
		SetAlign(tview.AlignLeft))
	resourceRow++
	
	resourcesTable.SetCell(resourceRow, 0, tview.NewTableCell(" Total Memory:").
		SetTextColor(ColorMuted).
		SetAlign(tview.AlignLeft))
	resourcesTable.SetCell(resourceRow, 1, tview.NewTableCell(fmt.Sprintf("%d GB", totalMemory)).
		SetTextColor(ColorForeground).
		SetAlign(tview.AlignLeft))
	resourceRow++
	
	resourcesTable.SetCell(resourceRow, 0, tview.NewTableCell(" Total Storage:").
		SetTextColor(ColorMuted).
		SetAlign(tview.AlignLeft))
	resourcesTable.SetCell(resourceRow, 1, tview.NewTableCell(fmt.Sprintf("%d GB", totalStorage)).
		SetTextColor(ColorForeground).
		SetAlign(tview.AlignLeft))
	resourceRow++
	
	// Section 3: Metrics
	metricsTable := tview.NewTable().
		SetBorders(false).
		SetSelectable(false, false).
		SetFixed(1, 2)
	
	// Title spans both columns with padding
	metricsTable.SetCell(0, 0, tview.NewTableCell(" [::b]Metrics").
		SetTextColor(ColorPrimary).
		SetAlign(tview.AlignLeft))
	metricsTable.SetCell(0, 1, tview.NewTableCell(""))
	
	// Metrics data
	metricsRow := 1
	
	metricsTable.SetCell(metricsRow, 0, tview.NewTableCell(" Uptime:").
		SetTextColor(ColorMuted).
		SetAlign(tview.AlignLeft))
	uptime := time.Since(cluster.CreatedAt)
	uptimeStr := fmt.Sprintf("%d days, %d hours", int(uptime.Hours()/24), int(uptime.Hours())%24)
	metricsTable.SetCell(metricsRow, 1, tview.NewTableCell(uptimeStr).
		SetTextColor(ColorForeground).
		SetAlign(tview.AlignLeft))
	metricsRow++
	
	metricsTable.SetCell(metricsRow, 0, tview.NewTableCell(" Nodes Health:").
		SetTextColor(ColorMuted).
		SetAlign(tview.AlignLeft))
	runningNodes := 0
	for _, node := range cluster.MasterNodes {
		if node.Status == "running" {
			runningNodes++
		}
	}
	for _, node := range cluster.WorkerNodes {
		if node.Status == "running" {
			runningNodes++
		}
	}
	healthStr := fmt.Sprintf("%d/%d Running", runningNodes, totalNodes)
	healthColor := ColorSuccess
	if runningNodes < totalNodes {
		healthColor = ColorWarning
	}
	metricsTable.SetCell(metricsRow, 1, tview.NewTableCell(healthStr).
		SetTextColor(healthColor).
		SetAlign(tview.AlignLeft))
	metricsRow++
	
	metricsTable.SetCell(metricsRow, 0, tview.NewTableCell(" CPU Usage:").
		SetTextColor(ColorMuted).
		SetAlign(tview.AlignLeft))
	// Calculate current CPU usage (simulated for now)
	cpuUsage := "45%" // This would come from actual metrics
	cpuColor := ColorSuccess
	if strings.TrimSuffix(cpuUsage, "%") > "70" {
		cpuColor = ColorWarning
	}
	metricsTable.SetCell(metricsRow, 1, tview.NewTableCell(cpuUsage).
		SetTextColor(cpuColor).
		SetAlign(tview.AlignLeft))
	metricsRow++
	
	metricsTable.SetCell(metricsRow, 0, tview.NewTableCell(" Memory Usage:").
		SetTextColor(ColorMuted).
		SetAlign(tview.AlignLeft))
	// Calculate current memory usage (simulated for now)
	memUsage := "62%" // This would come from actual metrics
	memColor := ColorSuccess
	if strings.TrimSuffix(memUsage, "%") > "70" {
		memColor = ColorWarning
	}
	metricsTable.SetCell(metricsRow, 1, tview.NewTableCell(memUsage).
		SetTextColor(memColor).
		SetAlign(tview.AlignLeft))
	
	// Create vertical dividers using TextView
	divider1 := tview.NewTextView().
		SetText("").
		SetDrawFunc(func(screen tcell.Screen, x, y, width, height int) (int, int, int, int) {
			for i := y; i < y+height; i++ {
				screen.SetContent(x+1, i, '│', nil, StyleMuted)
			}
			return 0, 0, 0, 0
		})
	
	divider2 := tview.NewTextView().
		SetText("").
		SetDrawFunc(func(screen tcell.Screen, x, y, width, height int) (int, int, int, int) {
			for i := y; i < y+height; i++ {
				screen.SetContent(x+1, i, '│', nil, StyleMuted)
			}
			return 0, 0, 0, 0
		})
	
	// Create a horizontal flex to arrange the three sections with dividers
	sectionsContainer := tview.NewFlex().SetDirection(tview.FlexColumn)
	sectionsContainer.AddItem(clusterInfoTable, 0, 1, false)
	sectionsContainer.AddItem(divider1, 3, 0, false)
	sectionsContainer.AddItem(resourcesTable, 0, 1, false)
	sectionsContainer.AddItem(divider2, 3, 0, false)
	sectionsContainer.AddItem(metricsTable, 0, 1, false)
	
	// Calculate node statistics first
	totalNodes = len(cluster.MasterNodes) + len(cluster.WorkerNodes)
	runningCount := 0
	stoppedCount := 0
	provisioningCount := 0
	
	// Count master node states
	for _, node := range cluster.MasterNodes {
		switch node.Status {
		case "running":
			runningCount++
		case "stopped":
			stoppedCount++
		default:
			provisioningCount++
		}
	}
	
	// Count worker node states
	for _, node := range cluster.WorkerNodes {
		switch node.Status {
		case "running":
			runningCount++
		case "stopped":
			stoppedCount++
		default:
			provisioningCount++
		}
	}
	
	// Calculate expected nodes (min/max)
	minNodes := 1 // At least 1 master
	maxNodes := totalNodes // Current total (can be expanded)
	if string(cluster.Mode) == "ha" || cluster.Mode == models.ModeHA {
		minNodes = 3 // HA mode requires 3 masters minimum
	}
	
	// Create node pools table with same design as cluster list
	nodePoolsTable := tview.NewTable().
		SetBorders(false).
		SetSelectable(true, false).
		SetSeparator(' ').
		SetSelectedStyle(StyleHighlight)
	
	// Add headers - same style as cluster list with proper spacing
	headers := []string{"  Pool Name", "Role", "Nodes", "Instance Type", "CPU", "Memory", "Status"}
	for col, header := range headers {
		alignment := tview.AlignLeft
		if col > 1 && col < 6 { // Center align numeric columns
			alignment = tview.AlignCenter
		}
		cell := tview.NewTableCell(header).
			SetTextColor(ColorPrimary).
			SetAlign(alignment).
			SetSelectable(false).
			SetExpansion(1)
		nodePoolsTable.SetCell(0, col, cell)
	}
	
	// Add control plane pool (always shown as it's a default pool)
	poolRow := 1
	
	// Always add control plane pool - this is mandatory for every cluster
	controlPlaneCount := len(cluster.MasterNodes)
	expectedControlPlaneCount := 1 // Default for developer mode
	if string(cluster.Mode) == "ha" || cluster.Mode == models.ModeHA {
		expectedControlPlaneCount = 3
	}
	
	controlPlaneStatus := "Running"
	controlPlaneColor := ColorSuccess
	
	if controlPlaneCount == 0 {
		controlPlaneStatus = "Not Provisioned"
		controlPlaneColor = ColorMuted
	} else if controlPlaneCount < expectedControlPlaneCount {
		controlPlaneStatus = fmt.Sprintf("Scaling (%d/%d)", controlPlaneCount, expectedControlPlaneCount)
		controlPlaneColor = ColorWarning
	} else {
		// Check if any master is not running
		for _, node := range cluster.MasterNodes {
			if node.Status != "running" {
				controlPlaneStatus = "Provisioning"
				controlPlaneColor = ColorWarning
				break
			}
		}
	}
	
	// Get control plane instance type
	controlPlaneInstanceType := cluster.InstanceType
	if controlPlaneInstanceType == "" {
		controlPlaneInstanceType = "t3.medium"
	}
	if len(cluster.MasterNodes) > 0 && cluster.MasterNodes[0].InstanceType != "" {
		controlPlaneInstanceType = cluster.MasterNodes[0].InstanceType
	}
	
	// Calculate control plane resources (show expected if no nodes yet)
	controlPlaneCPU := controlPlaneCount * 2
	controlPlaneMemory := controlPlaneCount * 4
	
	// If no nodes yet, show expected resources
	if controlPlaneCount == 0 {
		// Show expected resources based on instance type
		if controlPlaneInstanceType == "t3.medium" {
			controlPlaneCPU = expectedControlPlaneCount * 2
			controlPlaneMemory = expectedControlPlaneCount * 4
		} else if controlPlaneInstanceType == "t3.large" {
			controlPlaneCPU = expectedControlPlaneCount * 2
			controlPlaneMemory = expectedControlPlaneCount * 8
		} else if controlPlaneInstanceType == "t3.xlarge" {
			controlPlaneCPU = expectedControlPlaneCount * 4
			controlPlaneMemory = expectedControlPlaneCount * 16
		}
	} else if len(cluster.MasterNodes) > 0 {
		if cluster.MasterNodes[0].CPU > 0 {
			controlPlaneCPU = controlPlaneCount * cluster.MasterNodes[0].CPU
		}
		if cluster.MasterNodes[0].MemoryGB > 0 {
			controlPlaneMemory = controlPlaneCount * cluster.MasterNodes[0].MemoryGB
		}
	}
	
	nodePoolsTable.SetCell(poolRow, 0, tview.NewTableCell("  control-plane").
		SetTextColor(ColorForeground).
		SetAlign(tview.AlignLeft).
		SetExpansion(1))
	nodePoolsTable.SetCell(poolRow, 1, tview.NewTableCell("Control Plane").
		SetTextColor(ColorPrimary).
		SetAlign(tview.AlignLeft).
		SetExpansion(1))
	
	// Show expected count if different from actual
	nodeCountText := fmt.Sprintf("%d", controlPlaneCount)
	if controlPlaneCount != expectedControlPlaneCount {
		nodeCountText = fmt.Sprintf("%d/%d", controlPlaneCount, expectedControlPlaneCount)
	}
	nodePoolsTable.SetCell(poolRow, 2, tview.NewTableCell(nodeCountText).
		SetTextColor(ColorForeground).
		SetAlign(tview.AlignCenter).
		SetExpansion(1))
	nodePoolsTable.SetCell(poolRow, 3, tview.NewTableCell(controlPlaneInstanceType).
		SetTextColor(ColorForeground).
		SetAlign(tview.AlignCenter).
		SetExpansion(1))
	
	// Show CPU and Memory (even if 0)
	cpuText := fmt.Sprintf("%d", controlPlaneCPU)
	if controlPlaneCPU == 0 {
		cpuText = fmt.Sprintf("%d", expectedControlPlaneCount*2) // Default 2 vCPUs per node
	}
	memText := fmt.Sprintf("%d GB", controlPlaneMemory)
	if controlPlaneMemory == 0 {
		memText = fmt.Sprintf("%d GB", expectedControlPlaneCount*4) // Default 4 GB per node
	}
	
	nodePoolsTable.SetCell(poolRow, 4, tview.NewTableCell(cpuText).
		SetTextColor(ColorForeground).
		SetAlign(tview.AlignCenter).
		SetExpansion(1))
	nodePoolsTable.SetCell(poolRow, 5, tview.NewTableCell(memText).
		SetTextColor(ColorForeground).
		SetAlign(tview.AlignCenter).
		SetExpansion(1))
	nodePoolsTable.SetCell(poolRow, 6, tview.NewTableCell(controlPlaneStatus).
		SetTextColor(controlPlaneColor).
		SetAlign(tview.AlignLeft).
		SetExpansion(1))
	poolRow++
	
	// Add worker pool if there are worker nodes
	if len(cluster.WorkerNodes) > 0 {
		workerCount := len(cluster.WorkerNodes)
		workerStatus := "Running"
		workerColor := ColorSuccess
		
		// Check worker status
		for _, node := range cluster.WorkerNodes {
			if node.Status != "running" {
				workerStatus = "Scaling"
				workerColor = ColorWarning
				break
			}
		}
		
		// Get worker instance type
		workerInstanceType := cluster.InstanceType
		if workerInstanceType == "" {
			workerInstanceType = "t3.medium"
		}
		if cluster.WorkerNodes[0].InstanceType != "" {
			workerInstanceType = cluster.WorkerNodes[0].InstanceType
		}
		
		// Calculate worker resources
		workerCPU := workerCount * 2
		workerMemory := workerCount * 4
		if cluster.WorkerNodes[0].CPU > 0 {
			workerCPU = workerCount * cluster.WorkerNodes[0].CPU
		}
		if cluster.WorkerNodes[0].MemoryGB > 0 {
			workerMemory = workerCount * cluster.WorkerNodes[0].MemoryGB
		}
		
		nodePoolsTable.SetCell(poolRow, 0, tview.NewTableCell("  worker-pool-1").
			SetTextColor(ColorForeground).
			SetAlign(tview.AlignLeft).
			SetExpansion(1))
		nodePoolsTable.SetCell(poolRow, 1, tview.NewTableCell("Worker").
			SetTextColor(ColorAccent).
			SetAlign(tview.AlignLeft).
			SetExpansion(1))
		nodePoolsTable.SetCell(poolRow, 2, tview.NewTableCell(fmt.Sprintf("%d", workerCount)).
			SetTextColor(ColorForeground).
			SetAlign(tview.AlignCenter).
			SetExpansion(1))
		nodePoolsTable.SetCell(poolRow, 3, tview.NewTableCell(workerInstanceType).
			SetTextColor(ColorForeground).
			SetAlign(tview.AlignCenter).
			SetExpansion(1))
		nodePoolsTable.SetCell(poolRow, 4, tview.NewTableCell(fmt.Sprintf("%d", workerCPU)).
			SetTextColor(ColorForeground).
			SetAlign(tview.AlignCenter).
			SetExpansion(1))
		nodePoolsTable.SetCell(poolRow, 5, tview.NewTableCell(fmt.Sprintf("%d GB", workerMemory)).
			SetTextColor(ColorForeground).
			SetAlign(tview.AlignCenter).
			SetExpansion(1))
		nodePoolsTable.SetCell(poolRow, 6, tview.NewTableCell(workerStatus).
			SetTextColor(workerColor).
			SetAlign(tview.AlignLeft).
			SetExpansion(1))
		poolRow++
	}
	
	// Note: Alternating row backgrounds removed temporarily for debugging
	
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
	
	// Status bar with shortcuts (same style as listing page)
	statusBarFlex := tview.NewFlex().SetDirection(tview.FlexColumn)
	
	// Connection status (left)
	connectionStatus := tview.NewTextView().
		SetText(fmt.Sprintf(" %s● Connected to %s%s", TagSuccess, cluster.Name, TagReset)).
		SetDynamicColors(true).
		SetTextAlign(tview.AlignLeft)
	
	// Shortcuts (right)
	shortcuts := fmt.Sprintf("%s%c%s Back  %sEnter%s Select  %sk%s Kubeconfig  %se%s Edit  %sd%s Delete  %sr%s Refresh ", 
		TagPrimary, CharArrowLeft, TagReset, 
		TagPrimary, TagReset, 
		TagPrimary, TagReset, 
		TagPrimary, TagReset, 
		TagPrimary, TagReset,
		TagPrimary, TagReset)
	statusRight := tview.NewTextView().
		SetText(shortcuts).
		SetDynamicColors(true).
		SetTextAlign(tview.AlignRight)
	
	statusBarFlex.
		AddItem(connectionStatus, 0, 1, false).
		AddItem(statusRight, 0, 2, false)
	
	// Create content area with better organization
	contentFlex := tview.NewFlex().SetDirection(tview.FlexRow)
	
	// Add sections container
	contentFlex.AddItem(sectionsContainer, 10, 0, false)
	
	// Add a divider
	contentDivider := tview.NewTextView().
		SetText("").
		SetDrawFunc(func(screen tcell.Screen, x, y, width, height int) (int, int, int, int) {
			for i := x; i < x+width; i++ {
				screen.SetContent(i, y, CharDivider, nil, StyleMuted)
			}
			return 0, 0, 0, 0
		})
	contentFlex.AddItem(contentDivider, 1, 0, false)
	
	// Add node pools title with statistics
	poolsTitleFlex := tview.NewFlex().SetDirection(tview.FlexColumn)
	
	poolsTitle := tview.NewTextView().
		SetDynamicColors(true).
		SetText(fmt.Sprintf("  %sNode Pools%s", TagPrimary, TagReset))
	
	// Node statistics on the right
	nodeStats := fmt.Sprintf("Nodes: [::b]%d[::-] | Min: [::d]%d[::-] | Max: [::d]%d[::-] | Running: [green]%d[::-] | Stopped: [red]%d[::-] | Provisioning: [yellow]%d[::-] ",
		totalNodes, minNodes, maxNodes, runningCount, stoppedCount, provisioningCount)
	
	poolsStats := tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignRight).
		SetText(nodeStats)
	
	poolsTitleFlex.
		AddItem(poolsTitle, 0, 1, false).
		AddItem(poolsStats, 0, 1, false)
		
	contentFlex.AddItem(poolsTitleFlex, 1, 0, false)
	
	// Add divider below title
	poolsDivider := tview.NewTextView().
		SetText("").
		SetDrawFunc(func(screen tcell.Screen, x, y, width, height int) (int, int, int, int) {
			for i := x; i < x+width; i++ {
				screen.SetContent(i, y, CharDivider, nil, StyleMuted)
			}
			return 0, 0, 0, 0
		})
	contentFlex.AddItem(poolsDivider, 1, 0, false)
	contentFlex.AddItem(nodePoolsTable, 0, 1, true)
	
	// Build main layout (same structure as listing page)
	flex.
		AddItem(headerFlex, 1, 0, false).
		AddItem(headerDivider, 1, 0, false).
		AddItem(contentFlex, 0, 1, true).
		AddItem(footerDivider, 1, 0, false).
		AddItem(statusBarFlex, 1, 0, false)
	
	// Handle keyboard input
	flex.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyEscape:
			pages.SwitchToPage("clusters")
			go refreshClustersAsync()
			return nil
		case tcell.KeyRune:
			switch event.Rune() {
			case 'e', 'E':
				editCluster(cluster)
				return nil
			case 'd', 'D':
				deleteCluster(cluster)
				return nil
			case 'r', 'R':
				go refreshClustersAsync()
				return nil
			case 'k', 'K':
				// TODO: Download kubeconfig
				return nil
			}
		}
		return event
	})
	
	// Add and switch to details page
	pages.AddAndSwitchToPage("details", flex, true)
}

func showClusterDetailsOld(cluster models.K3sCluster) {
	// Create a text view for details
	details := tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(true)

	// Calculate cluster statistics
	totalNodes := len(cluster.MasterNodes) + len(cluster.WorkerNodes)
	runningNodes := 0
	totalCPU := 0
	totalMemory := 0
	totalStorage := 0
	
	for _, node := range cluster.MasterNodes {
		if node.Status == "running" {
			runningNodes++
		}
		totalCPU += node.CPU
		totalMemory += node.MemoryGB
		totalStorage += node.StorageGB
	}
	for _, node := range cluster.WorkerNodes {
		if node.Status == "running" {
			runningNodes++
		}
		totalCPU += node.CPU
		totalMemory += node.MemoryGB
		totalStorage += node.StorageGB
	}

	// Get status color
	statusColor := TagDanger
	statusIcon := "●"
	if cluster.Status == "running" {
		statusColor = TagSuccess
		statusIcon = "●"
	} else if cluster.Status == "creating" || cluster.Status == "updating" {
		statusColor = TagWarning
		statusIcon = "◐"
	} else if cluster.Status == "stopped" {
		statusColor = TagMuted
		statusIcon = "○"
	}

	// Format uptime
	uptime := time.Since(cluster.CreatedAt)
	uptimeStr := fmt.Sprintf("%dd %dh", int(uptime.Hours())/24, int(uptime.Hours())%24)
	
	// Set default values for missing data
	if cluster.APIEndpoint == "" {
		cluster.APIEndpoint = cluster.Name
	}
	if cluster.K3sVersion == "" {
		cluster.K3sVersion = "latest"
	}
	if cluster.KubeVersion == "" {
		cluster.KubeVersion = "1.28.3"
	}
	if cluster.NetworkCIDR == "" {
		cluster.NetworkCIDR = "10.42.0.0/16"
	}
	if cluster.ServiceCIDR == "" {
		cluster.ServiceCIDR = "10.43.0.0/16"
	}
	if cluster.ClusterDNS == "" {
		cluster.ClusterDNS = "10.43.0.10"
	}
	if cluster.KubeConfigPath == "" {
		cluster.KubeConfigPath = fmt.Sprintf("~/.kube/config-%s", cluster.Name)
	}
	if cluster.SSHKeyPath == "" {
		cluster.SSHKeyPath = "Managed by AWS Systems Manager"
	}
	
	// Set default CPU/Memory if not set
	if totalCPU == 0 {
		// Estimate based on instance type and node count
		cpuPerNode := 2 // default for t3.medium
		switch cluster.InstanceType {
		case "t3.micro":
			cpuPerNode = 2
		case "t3.small":
			cpuPerNode = 2
		case "t3.medium":
			cpuPerNode = 2
		case "t3.large":
			cpuPerNode = 2
		case "t3.xlarge":
			cpuPerNode = 4
		}
		totalCPU = cpuPerNode * totalNodes
	}
	if totalMemory == 0 {
		// Estimate based on instance type and node count
		memPerNode := 4 // default for t3.medium
		switch cluster.InstanceType {
		case "t3.micro":
			memPerNode = 1
		case "t3.small":
			memPerNode = 2
		case "t3.medium":
			memPerNode = 4
		case "t3.large":
			memPerNode = 8
		case "t3.xlarge":
			memPerNode = 16
		}
		totalMemory = memPerNode * totalNodes
	}
	if totalStorage == 0 {
		totalStorage = 20 * totalNodes // Default 20GB per node
	}

	// Calculate estimated monthly cost
	costPerHour := 0.0
	switch cluster.InstanceType {
	case "t3.micro":
		costPerHour = 0.0104
	case "t3.small":
		costPerHour = 0.0208
	case "t3.medium":
		costPerHour = 0.0416
	case "t3.large":
		costPerHour = 0.0832
	default:
		costPerHour = 0.0416 // default to medium
	}
	monthlyCost := costPerHour * 24 * 30 * float64(totalNodes)

	// Build the professional dashboard-style details
	text := fmt.Sprintf(`
%s╔════════════════════════════════════════════════════════════════════════════╗%s
%s║                           KUBERNETES CLUSTER DASHBOARD                        ║%s
%s╚════════════════════════════════════════════════════════════════════════════╝%s

%s┌─ CLUSTER IDENTITY ─────────────────────────────────────────────────────────┐%s
%s│%s                                                                            %s│%s
%s│%s  %sName%s        %s%-30s%s %sRegion%s      %s%-15s%s %s│%s
%s│%s  %sMode%s        %s%-30s%s %sProvider%s    AWS EC2         %s│%s
%s│%s  %sStatus%s      %s%s%-29s%s %sType%s        %s%-15s%s %s│%s
%s│%s                                                                            %s│%s
%s└────────────────────────────────────────────────────────────────────────────┘%s

%s┌─ INFRASTRUCTURE METRICS ───────────────────────────────────────────────────┐%s
%s│%s                                                                            %s│%s
%s│%s  %s▣ COMPUTE%s                    %s▣ STORAGE%s                 %s▣ NETWORK%s        %s│%s
%s│%s  ├─ vCPUs:      %s%3d cores%s     ├─ Total:   %s%3d GB%s        ├─ Pod CIDR:   %s%-14s%s %s│%s
%s│%s  ├─ Memory:     %s%3d GB%s        ├─ Type:    EBS           ├─ Svc CIDR:   %s%-14s%s %s│%s
%s│%s  └─ Nodes:      %s%d/%d active%s    └─ IOPS:    3000          └─ DNS:        %s%-14s%s %s│%s
%s│%s                                                                            %s│%s
%s│%s  %s▣ AVAILABILITY%s                                          %s▣ COST ANALYSIS%s  %s│%s
%s│%s  ├─ Uptime:     %s%-12s%s                            ├─ Hourly:  $%s%.4f%s    %s│%s
%s│%s  ├─ SLA:        99.95%%                                  ├─ Daily:   $%s%.2f%s    %s│%s
%s│%s  └─ Health:     %s%s%s                                   └─ Monthly: $%s%.2f%s   %s│%s
%s│%s                                                                            %s│%s
%s└────────────────────────────────────────────────────────────────────────────┘%s

%s┌─ NODE TOPOLOGY ─────────────────────────────────────────────────────────────┐%s
%s│%s                                                                            %s│%s
%s│%s  %s◆ CONTROL PLANE%s [%d nodes]                                                 %s│%s`,
		TagMuted, TagReset,
		TagPrimary, TagReset,
		TagMuted, TagReset,
		
		TagMuted, TagReset,
		TagMuted, TagReset, TagMuted, TagReset,
		TagMuted, TagReset, TagBold, TagReset, cluster.Name, TagReset, TagBold, TagReset, cluster.Region, TagReset, TagMuted, TagReset,
		TagMuted, TagReset, TagBold, TagReset, strings.ToUpper(string(cluster.Mode)), TagReset, TagBold, TagReset, TagMuted, TagReset,
		TagMuted, TagReset, TagBold, TagReset, statusColor, statusIcon, strings.ToUpper(string(cluster.Status)), TagReset, TagBold, TagReset, cluster.InstanceType, TagReset, TagMuted, TagReset,
		TagMuted, TagReset, TagMuted, TagReset,
		TagMuted, TagReset,
		
		TagMuted, TagReset,
		TagMuted, TagReset, TagMuted, TagReset,
		TagMuted, TagReset, TagPrimary, TagReset, TagPrimary, TagReset, TagPrimary, TagReset, TagMuted, TagReset,
		TagMuted, TagReset, TagAccent, totalCPU, TagReset, TagAccent, totalStorage, TagReset, cluster.NetworkCIDR, TagReset, TagMuted, TagReset,
		TagMuted, TagReset, TagAccent, totalMemory, TagReset, cluster.ServiceCIDR, TagReset, TagMuted, TagReset,
		TagMuted, TagReset, TagSuccess, runningNodes, totalNodes, TagReset, cluster.ClusterDNS, TagReset, TagMuted, TagReset,
		TagMuted, TagReset, TagMuted, TagReset,
		TagMuted, TagReset, TagPrimary, TagReset, TagPrimary, TagReset, TagMuted, TagReset,
		TagMuted, TagReset, uptimeStr, TagReset, TagWarning, costPerHour*float64(totalNodes), TagReset, TagMuted, TagReset,
		TagMuted, TagReset, TagWarning, costPerHour*24*float64(totalNodes), TagReset, TagMuted, TagReset,
		TagMuted, TagReset, statusColor, "HEALTHY", TagReset, TagWarning, monthlyCost, TagReset, TagMuted, TagReset,
		TagMuted, TagReset, TagMuted, TagReset,
		TagMuted, TagReset,
		
		TagMuted, TagReset,
		TagMuted, TagReset, TagMuted, TagReset,
		TagMuted, TagReset, TagPrimary, TagReset, len(cluster.MasterNodes), TagMuted, TagReset)

	// Add master nodes in professional table format
	for i, node := range cluster.MasterNodes {
		nodeStatusColor := TagSuccess
		nodeStatusIcon := "◉"
		nodeStatus := "READY"
		if node.Status != "running" {
			nodeStatusColor = TagWarning
			nodeStatusIcon = "◎"
			nodeStatus = "PENDING"
		}
		
		// Set defaults for missing node data
		if node.IP == "" {
			node.IP = fmt.Sprintf("10.0.1.%d", 10+i)
		}
		if node.CPU == 0 {
			node.CPU = 2
		}
		if node.MemoryGB == 0 {
			node.MemoryGB = 4
		}
		if node.StorageGB == 0 {
			node.StorageGB = 20
		}
		
		text += fmt.Sprintf(`%s│%s  ├─ [master-%d]  %s%-15s%s  %s%s %-8s%s  %s│%s %d vCPU %s│%s %dG RAM %s│%s %dG SSD  %s│%s
`, 
			TagMuted, TagReset,
			i,
			TagBold, node.IP, TagReset,
			nodeStatusColor, nodeStatusIcon, nodeStatus, TagReset,
			TagMuted, TagReset, node.CPU,
			TagMuted, TagReset, node.MemoryGB,
			TagMuted, TagReset, node.StorageGB,
			TagMuted, TagReset)
	}

	text += fmt.Sprintf(`%s│%s                                                                            %s│%s
%s└────────────────────────────────────────────────────────────────────────────┘%s

`, TagMuted, TagReset, TagMuted, TagReset,
		TagMuted, TagReset)

	// Add worker nodes section if present
	if len(cluster.WorkerNodes) > 0 {
		text += fmt.Sprintf(`%s┌─ WORKER POOL ──────────────────────────────────────────────────────────────┐%s
%s│%s                                                                            %s│%s
%s│%s  %s◆ DATA PLANE%s [%d nodes]                                                   %s│%s
`, TagMuted, TagReset,
			TagMuted, TagReset, TagMuted, TagReset,
			TagMuted, TagReset, TagPrimary, TagReset, len(cluster.WorkerNodes), TagMuted, TagReset)
		
		for i, node := range cluster.WorkerNodes {
			nodeStatusColor := TagSuccess
			nodeStatusIcon := "◉"
			nodeStatus := "READY"
			if node.Status != "running" {
				nodeStatusColor = TagWarning
				nodeStatusIcon = "◎"
				nodeStatus = "PENDING"
			}
			
			// Set defaults
			if node.IP == "" {
				node.IP = fmt.Sprintf("10.0.2.%d", 10+i)
			}
			if node.CPU == 0 {
				node.CPU = 2
			}
			if node.MemoryGB == 0 {
				node.MemoryGB = 4
			}
			if node.StorageGB == 0 {
				node.StorageGB = 20
			}
			
			text += fmt.Sprintf(`%s│%s  ├─ [worker-%d]  %s%-15s%s  %s%s %-8s%s  %s│%s %d vCPU %s│%s %dG RAM %s│%s %dG SSD  %s│%s
`, 
				TagMuted, TagReset,
				i,
				TagBold, node.IP, TagReset,
				nodeStatusColor, nodeStatusIcon, nodeStatus, TagReset,
				TagMuted, TagReset, node.CPU,
				TagMuted, TagReset, node.MemoryGB,
				TagMuted, TagReset, node.StorageGB,
				TagMuted, TagReset)
		}
		
		text += fmt.Sprintf(`%s│%s                                                                            %s│%s
%s└────────────────────────────────────────────────────────────────────────────┘%s

`, TagMuted, TagReset, TagMuted, TagReset,
			TagMuted, TagReset)
	}

	// Add platform services section
	text += fmt.Sprintf(`%s┌─ PLATFORM SERVICES ────────────────────────────────────────────────────────┐%s
%s│%s                                                                            %s│%s
`, TagMuted, TagReset,
		TagMuted, TagReset, TagMuted, TagReset)
	
	// Service status with better layout
	services := []struct{
		name string
		enabled bool
		status string
		port string
	}{
		{"Kubernetes API Server", true, "ACTIVE", ":6443"},
		{"etcd Database", cluster.Mode == "ha", "ACTIVE", ":2379-2380"},
		{"CoreDNS", cluster.Features.CoreDNS, "ACTIVE", ":53"},
		{"Metrics Server", cluster.Features.MetricsServer, "ACTIVE", ":443"},
		{"Traefik Ingress", cluster.Features.Traefik, "ACTIVE", ":80,:443"},
		{"Service LoadBalancer", cluster.Features.ServiceLB, "ACTIVE", "dynamic"},
		{"Local Path Storage", cluster.Features.LocalStorage, "ACTIVE", "N/A"},
		{"Flannel CNI", true, "ACTIVE", ":8472"},
	}
	
	text += fmt.Sprintf(`%s│%s  %-30s %-12s %-10s %-15s %s│%s
%s│%s  %-30s %-12s %-10s %-15s %s│%s
`, 
		TagMuted, TagReset, "SERVICE", "STATUS", "STATE", "PORTS", TagMuted, TagReset,
		TagMuted, TagReset, strings.Repeat("─", 30), strings.Repeat("─", 12), strings.Repeat("─", 10), strings.Repeat("─", 15), TagMuted, TagReset)
	
	for _, svc := range services {
		statusIcon := "○"
		statusColor := TagMuted
		state := "DISABLED"
		
		if svc.enabled {
			statusIcon = "●"
			statusColor = TagSuccess
			state = svc.status
		}
		
		text += fmt.Sprintf(`%s│%s  %s%s%s %-28s %-12s %-10s %-15s %s│%s
`,
			TagMuted, TagReset,
			statusColor, statusIcon, TagReset,
			svc.name, state, "Healthy", svc.port,
			TagMuted, TagReset)
	}

	text += fmt.Sprintf(`%s│%s                                                                            %s│%s
%s└────────────────────────────────────────────────────────────────────────────┘%s

`, TagMuted, TagReset, TagMuted, TagReset,
		TagMuted, TagReset)

	// Add operational commands section
	text += fmt.Sprintf(`%s┌─ QUICK ACTIONS ────────────────────────────────────────────────────────────┐%s
%s│%s                                                                            %s│%s
%s│%s  %s◉ Connect to cluster:%s                                                     %s│%s
%s│%s    kubectl --kubeconfig=%s get nodes                         %s│%s
%s│%s                                                                            %s│%s
%s│%s  %s◉ Access Dashboard:%s                                                        %s│%s
%s│%s    kubectl proxy & open http://localhost:8001/api/v1/...                  %s│%s
%s│%s                                                                            %s│%s
%s│%s  %s◉ View Logs:%s                                                               %s│%s
%s│%s    kubectl logs -n kube-system -l component=k3s                           %s│%s
%s│%s                                                                            %s│%s
%s└────────────────────────────────────────────────────────────────────────────┘%s
`, TagMuted, TagReset,
		TagMuted, TagReset, TagMuted, TagReset,
		TagMuted, TagReset, TagPrimary, TagReset, TagMuted, TagReset,
		TagMuted, TagReset, cluster.KubeConfigPath, TagMuted, TagReset,
		TagMuted, TagReset, TagMuted, TagReset,
		TagMuted, TagReset, TagPrimary, TagReset, TagMuted, TagReset,
		TagMuted, TagReset, TagMuted, TagReset,
		TagMuted, TagReset, TagMuted, TagReset,
		TagMuted, TagReset, TagPrimary, TagReset, TagMuted, TagReset,
		TagMuted, TagReset, TagMuted, TagReset,
		TagMuted, TagReset, TagMuted, TagReset,
		TagMuted, TagReset)

	details.SetText(text)

	// Create a frame with the details and shortcuts
	frame := tview.NewFrame(details).
		AddText(fmt.Sprintf(" %s━━━ %s CLUSTER DETAILS %s━━━%s ", TagPrimary, cluster.Name, TagPrimary, TagReset), true, tview.AlignCenter, ColorForeground).
		AddText(fmt.Sprintf("%sESC%s Back  %sk%s Download Kubeconfig  %se%s Edit  %sr%s Restart  %sd%s Delete", TagPrimary, TagReset, TagPrimary, TagReset, TagPrimary, TagReset, TagPrimary, TagReset, TagPrimary, TagReset), false, tview.AlignCenter, ColorForeground).
		SetBorders(0, 0, 1, 1, 0, 0)

	// Handle ESC key and prevent 'd' key from bubbling
	details.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEscape {
			pages.SwitchToPage("clusters")
			// Refresh when returning to cluster list
			go refreshClustersAsync()
			return nil // Consume the event
		}
		// Prevent 'd' key from triggering delete when in details view
		if event.Key() == tcell.KeyRune && (event.Rune() == 'd' || event.Rune() == 'D') {
			return nil // Consume the event
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
			// Log the error so we can debug
			fmt.Printf("\n[ERROR] Failed to create cluster: %v\n", err)
			fmt.Printf("Press Enter to continue...")
			fmt.Scanln()
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
							pages.AddAndSwitchToPage("error", errorModal, true)
						}
						refreshClusters()
					})
				}()
			}
		})

	pages.AddAndSwitchToPage("confirm", modal, true)
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