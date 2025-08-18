package main

import (
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/madhouselabs/goman/pkg/connectivity"
)

var (
	globalSingleTunnelManager *connectivity.SingleTunnelManager
	tunnelManagerOnce         sync.Once
)

// GetGlobalSingleTunnelManager returns the singleton single tunnel manager
func GetGlobalSingleTunnelManager() *connectivity.SingleTunnelManager {
	tunnelManagerOnce.Do(func() {
		globalSingleTunnelManager = connectivity.NewSingleTunnelManager()
	})
	return globalSingleTunnelManager
}

// getCurrentClusterFile returns the path to the current cluster file
func getCurrentClusterFile() string {
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, ".goman", "current-cluster")
}

// getCurrentCluster returns the current cluster name
func getCurrentCluster() string {
	data, err := os.ReadFile(getCurrentClusterFile())
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// saveCurrentCluster saves the current cluster name
func saveCurrentCluster(clusterName string) error {
	homeDir, _ := os.UserHomeDir()
	gomanDir := filepath.Join(homeDir, ".goman")
	if err := os.MkdirAll(gomanDir, 0755); err != nil {
		return err
	}
	return os.WriteFile(getCurrentClusterFile(), []byte(clusterName), 0644)
}

// clearCurrentCluster removes the current cluster file
func clearCurrentCluster() error {
	err := os.Remove(getCurrentClusterFile())
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// switchToCluster switches to a different cluster
func switchToCluster(newClusterName string) error {
	// Simply save the new cluster as current
	// Tunnel management is handled on-demand when actually connecting
	return saveCurrentCluster(newClusterName)
}