package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	zone "github.com/lrstanley/bubblezone"
	"github.com/madhouselabs/goman/pkg/setup"
)

// CLI handles command-line interface operations
type CLI struct {
	initCmd    *flag.FlagSet
	statusCmd  *flag.FlagSet
	clusterCmd *flag.FlagSet
}

// NewCLI creates a new CLI handler
func NewCLI() *CLI {
	cli := &CLI{
		initCmd:    flag.NewFlagSet("init", flag.ExitOnError),
		statusCmd:  flag.NewFlagSet("status", flag.ExitOnError),
		clusterCmd: flag.NewFlagSet("cluster", flag.ExitOnError),
	}

	// Add flags for init command
	cli.initCmd.Bool("force", false, "Force re-initialization even if already initialized")

	return cli
}

// Run processes CLI arguments and executes appropriate command
func (cli *CLI) Run() {
	// If no arguments, run TUI
	if len(os.Args) < 2 {
		cli.runTUI()
		return
	}

	switch os.Args[1] {
	case "init", "--init":
		cli.handleInit()
	case "status", "--status":
		cli.handleStatus()
	case "cluster":
		cli.handleCluster()
	case "resources":
		cli.handleResources()
	case "uninit":
		// Hidden command - not shown in help
		cli.handleUninit()
	case "help", "--help", "-h":
		cli.printHelp()
	default:
		// Unknown command, run TUI
		cli.runTUI()
	}
}

// handleInit handles the initialization command
func (cli *CLI) handleInit() {
	// Check for --non-interactive flag
	for _, arg := range os.Args {
		if arg == "--non-interactive" || arg == "-n" {
			handleInitNonInteractive()
			return
		}
	}
	// When called directly via 'goman init', show the TUI initialization screen
	// This provides a better user experience than CLI-only mode
	cli.showInitPrompt()
}

// handleUninit removes all Goman infrastructure (hidden command)
func (cli *CLI) handleUninit() {
	fmt.Println("WARNING: This will remove all Goman infrastructure from your cloud provider")
	fmt.Println("This includes:")
	fmt.Println("  • Storage backend and all stored data")
	fmt.Println("  • Serverless functions")
	fmt.Println("  • Lock service tables")
	fmt.Println("  • IAM roles and policies")
	fmt.Println("  • All other provider-specific resources")
	fmt.Println()
	fmt.Print("Are you sure? (yes/no): ")

	var response string
	fmt.Scanln(&response)

	if response != "yes" {
		fmt.Println("Aborted.")
		return
	}

	fmt.Println("\nRemoving Goman infrastructure...")

	// Run cleanup
	ctx := context.Background()
	if err := cli.cleanupInfrastructure(ctx); err != nil {
		fmt.Printf("Error during cleanup: %v\n", err)
		fmt.Println("Some resources may need manual cleanup in AWS console")
		os.Exit(1)
	}

	// Remove initialization marker
	home, _ := os.UserHomeDir()
	initFile := filepath.Join(home, ".goman", "initialized.json")
	os.Remove(initFile)

	fmt.Println("✓ Goman infrastructure removed successfully")
	fmt.Println("Run 'goman init' to reinitialize when needed")
}

// handleStatus shows initialization status
func (cli *CLI) handleStatus() {
	if !cli.isInitialized() {
		fmt.Println("✗ Goman is not initialized")
		fmt.Println("Run 'goman init' to set up the infrastructure")
		os.Exit(1)
	}

	status := cli.getInitStatus()
	fmt.Println("Goman Infrastructure Status:")
	fmt.Println("============================")
	if status.S3Bucket {
		fmt.Println("✓ S3 Bucket: Configured")
	} else {
		fmt.Println("✗ S3 Bucket: Not configured")
	}
	if status.Lambda {
		fmt.Println("✓ Lambda Function: Deployed")
	} else {
		fmt.Println("✗ Lambda Function: Not deployed")
	}
	if status.DynamoDB {
		fmt.Println("✓ DynamoDB Table: Created")
	} else {
		fmt.Println("✗ DynamoDB Table: Not created")
	}
	if status.IAMRoles {
		fmt.Println("✓ IAM Roles: Configured")
	} else {
		fmt.Println("✗ IAM Roles: Not configured")
	}
}

// handleCluster handles cluster subcommands
func (cli *CLI) handleCluster() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: goman cluster <command>")
		fmt.Println("Commands: create, delete, list, status")
		os.Exit(1)
	}

	// Check initialization first
	if !cli.isInitialized() {
		fmt.Println("✗ Goman is not initialized")
		fmt.Println("Run 'goman init' first to set up the infrastructure")
		os.Exit(1)
	}

	// Handle cluster subcommands
	switch os.Args[2] {
	case "create":
		handleClusterCreate(os.Args[3:])
	case "delete":
		handleClusterDelete(os.Args[3:])
	case "list":
		handleClusterList(os.Args[3:])
	case "status":
		handleClusterStatus(os.Args[3:])
	default:
		fmt.Printf("Unknown cluster command: %s\n", os.Args[2])
		os.Exit(1)
	}
}

// handleResources handles resources subcommands
func (cli *CLI) handleResources() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: goman resources <command>")
		fmt.Println("Commands: list")
		os.Exit(1)
	}

	// Check initialization first
	if !cli.isInitialized() {
		fmt.Println("✗ Goman is not initialized")
		fmt.Println("Run 'goman init' first to set up the infrastructure")
		os.Exit(1)
	}

	switch os.Args[2] {
	case "list":
		handleResourcesList(os.Args[3:])
	default:
		fmt.Printf("Unknown resources command: %s\n", os.Args[2])
		os.Exit(1)
	}
}

// runTUI runs the interactive TUI
func (cli *CLI) runTUI() {
	// Check initialization status
	if !cli.isInitialized() {
		// Show initialization prompt
		cli.showInitPrompt()
		return
	}

	// Run the normal TUI
	runMainTUI()
}

// showInitPrompt shows initialization prompt in TUI
func (cli *CLI) showInitPrompt() {
	// Initialize bubblezone manager for mouse support
	zone.NewGlobal()

	p := tea.NewProgram(newInitPromptModel(), tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	// After initialization completes, check if we should run the main TUI
	if cli.isInitialized() {
		runMainTUI()
	}
}

// isInitialized checks if Goman is initialized
func (cli *CLI) isInitialized() bool {
	// Check for initialization marker file
	home, _ := os.UserHomeDir()
	initFile := filepath.Join(home, ".goman", "initialized.json")

	if _, err := os.Stat(initFile); os.IsNotExist(err) {
		return false
	}

	// Verify the status
	status := cli.getInitStatus()
	return status.S3Bucket && status.Lambda && status.DynamoDB && status.IAMRoles
}

// InitStatus represents initialization status
type InitStatus struct {
	S3Bucket  bool   `json:"s3_bucket"`
	Lambda    bool   `json:"lambda"`
	DynamoDB  bool   `json:"dynamodb"`
	IAMRoles  bool   `json:"iam_roles"`
	Timestamp string `json:"timestamp"`
}

// getInitStatus reads initialization status
func (cli *CLI) getInitStatus() InitStatus {
	home, _ := os.UserHomeDir()
	initFile := filepath.Join(home, ".goman", "initialized.json")

	data, err := os.ReadFile(initFile)
	if err != nil {
		return InitStatus{}
	}

	var status InitStatus
	json.Unmarshal(data, &status)
	return status
}

// saveInitStatus saves initialization status
func saveInitStatus(result *setup.InitializeResult) error {
	home, _ := os.UserHomeDir()
	gomanDir := filepath.Join(home, ".goman")

	// Create directory if it doesn't exist
	if err := os.MkdirAll(gomanDir, 0755); err != nil {
		return err
	}

	status := InitStatus{
		S3Bucket:  result.StorageReady,
		Lambda:    result.FunctionReady,
		DynamoDB:  result.LockServiceReady,
		IAMRoles:  result.AuthReady,
		Timestamp: fmt.Sprintf("%v", result),
	}

	data, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return err
	}

	initFile := filepath.Join(gomanDir, "initialized.json")
	return os.WriteFile(initFile, data, 0644)
}

// cleanupInfrastructure removes all provider-specific infrastructure
func (cli *CLI) cleanupInfrastructure(ctx context.Context) error {
	// Use the provider-agnostic cleanup from setup package
	return setup.CleanupInfrastructure(ctx)
}

// printHelp prints help information
func (cli *CLI) printHelp() {
	fmt.Println("Goman - Kubernetes Cluster Manager")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  goman              Start interactive TUI")
	fmt.Println("  goman init         Initialize infrastructure")
	fmt.Println("  goman status       Show initialization status")
	fmt.Println("  goman cluster      Manage clusters (use TUI)")
	fmt.Println()
	fmt.Println("Options:")
	fmt.Println("  --help, -h         Show this help message")
	fmt.Println()
	fmt.Println("First-time setup:")
	fmt.Println("  Run 'goman init' to set up AWS infrastructure before using the TUI")
}
