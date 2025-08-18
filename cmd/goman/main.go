package main

import (
	"fmt"
	"os"
	"time"

	"github.com/madhouselabs/goman/pkg/cluster"
	"github.com/madhouselabs/goman/pkg/config"
	"github.com/madhouselabs/goman/pkg/models"
	"github.com/rivo/tview"
	"github.com/spf13/cobra"
)

// Global variables
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
			initializeInfrastructure()
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
	rootCmd.AddCommand(kubectlCmd)
	rootCmd.AddCommand(clusterCmd)
	rootCmd.AddCommand(kubeCmd)
	rootCmd.AddCommand(tunnelCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
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