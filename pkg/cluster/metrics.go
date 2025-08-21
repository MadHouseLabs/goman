package cluster

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/madhouselabs/goman/pkg/logger"
	"github.com/madhouselabs/goman/pkg/models"
	"github.com/madhouselabs/goman/pkg/provider/registry"
	"github.com/madhouselabs/goman/pkg/storage"
)

// ClusterMetrics holds the metrics for a cluster
type ClusterMetrics struct {
	TotalCPU      float64
	UsedCPU       float64
	TotalMemoryGB float64
	UsedMemoryGB  float64
	PodCount      int
	NodesReady    int
	NodesTotal    int
	LastUpdated   time.Time
}

// FetchClusterMetrics fetches live metrics from a K3s cluster using SSM
func FetchClusterMetrics(clusterName string) (*ClusterMetrics, error) {
	ctx := context.Background()
	
	// Get provider first
	provider, err := registry.GetDefaultProvider()
	if err != nil {
		return nil, fmt.Errorf("failed to get provider: %w", err)
	}
	
	// Get cluster configuration
	store, err := storage.NewStorageWithProvider(provider)
	if err != nil {
		return nil, fmt.Errorf("failed to create storage: %w", err)
	}
	
	clusterState, err := store.LoadClusterState(clusterName)
	if err != nil {
		return nil, fmt.Errorf("failed to load cluster state: %w", err)
	}
	
	cluster := clusterState.Cluster

	if cluster.Status != models.StatusRunning {
		return nil, fmt.Errorf("cluster is not running")
	}

	// Get the master node instance ID
	var masterInstanceID string
	for _, node := range cluster.MasterNodes {
		if node.ID != "" {
			masterInstanceID = node.ID
			break
		}
	}

	if masterInstanceID == "" {
		return nil, fmt.Errorf("no master node found")
	}

	// Prepare kubectl commands to fetch metrics
	// Combine multiple commands into a single script for efficiency
	command := `#!/bin/bash
set -e

# Export kubeconfig
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml

# Get node metrics
echo "=== NODE_METRICS ==="
kubectl top nodes --no-headers 2>/dev/null || echo "METRICS_NOT_AVAILABLE"

echo "=== POD_COUNT ==="
kubectl get pods --all-namespaces --no-headers 2>/dev/null | wc -l

echo "=== NODE_STATUS ==="
kubectl get nodes --no-headers 2>/dev/null | awk '{print $2}'

echo "=== END ==="
`

	// Execute command via SSM
	logger.Printf("Fetching metrics for cluster %s from instance %s", clusterName, masterInstanceID)
	result, err := provider.GetComputeService().RunCommand(ctx, []string{masterInstanceID}, command)
	if err != nil {
		return nil, fmt.Errorf("failed to run command: %w", err)
	}

	// Check if command succeeded
	if result.Status != "Success" {
		return nil, fmt.Errorf("command failed with status: %s", result.Status)
	}

	// Get the output from the master instance
	instanceResult, ok := result.Instances[masterInstanceID]
	if !ok || instanceResult.ExitCode != 0 {
		if instanceResult != nil && instanceResult.Error != "" {
			return nil, fmt.Errorf("command failed: %s", instanceResult.Error)
		}
		return nil, fmt.Errorf("command failed with exit code: %d", instanceResult.ExitCode)
	}

	// Parse the output
	metrics := &ClusterMetrics{
		LastUpdated: time.Now(),
	}

	output := instanceResult.Output
	sections := strings.Split(output, "===")

	for i, section := range sections {
		section = strings.TrimSpace(section)
		if i+1 < len(sections) {
			nextSection := strings.TrimSpace(sections[i+1])
			
			switch section {
			case "NODE_METRICS":
				if !strings.Contains(nextSection, "METRICS_NOT_AVAILABLE") {
					parseNodeMetrics(nextSection, metrics)
				}
			case "POD_COUNT":
				if count, err := strconv.Atoi(strings.TrimSpace(nextSection)); err == nil {
					metrics.PodCount = count
				}
			case "NODE_STATUS":
				parseNodeStatus(nextSection, metrics)
			}
		}
	}

	// If we couldn't get metrics from kubectl top, estimate based on instance types
	if metrics.TotalCPU == 0 {
		// Estimate based on nodes (t3.medium = 2 vCPUs, 4GB RAM)
		metrics.TotalCPU = float64(len(cluster.MasterNodes)+len(cluster.WorkerNodes)) * 2
		metrics.TotalMemoryGB = float64(len(cluster.MasterNodes)+len(cluster.WorkerNodes)) * 4
		// Assume 20% usage if we can't get actual metrics
		metrics.UsedCPU = metrics.TotalCPU * 0.2
		metrics.UsedMemoryGB = metrics.TotalMemoryGB * 0.2
	}

	metrics.NodesTotal = len(cluster.MasterNodes) + len(cluster.WorkerNodes)

	return metrics, nil
}

// parseNodeMetrics parses the output of kubectl top nodes
func parseNodeMetrics(output string, metrics *ClusterMetrics) {
	lines := strings.Split(output, "\n")
	
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.Contains(line, "METRICS_NOT_AVAILABLE") {
			continue
		}

		// Example line: node-name   100m   10%   500Mi   25%
		fields := strings.Fields(line)
		if len(fields) >= 5 {
			// Parse CPU (convert millicores to cores)
			cpuStr := fields[1]
			if strings.HasSuffix(cpuStr, "m") {
				cpuStr = strings.TrimSuffix(cpuStr, "m")
				if cpu, err := strconv.ParseFloat(cpuStr, 64); err == nil {
					metrics.UsedCPU += cpu / 1000.0 // Convert millicores to cores
				}
			} else {
				if cpu, err := strconv.ParseFloat(cpuStr, 64); err == nil {
					metrics.UsedCPU += cpu
				}
			}

			// Parse CPU percentage to estimate total
			cpuPercentStr := strings.TrimSuffix(fields[2], "%")
			if cpuPercent, err := strconv.ParseFloat(cpuPercentStr, 64); err == nil && cpuPercent > 0 {
				// Calculate total CPU from used and percentage
				nodeTotalCPU := (metrics.UsedCPU / cpuPercent) * 100
				metrics.TotalCPU += nodeTotalCPU
			}

			// Parse Memory (convert Mi/Gi to GB)
			memStr := fields[3]
			var memGB float64
			if strings.HasSuffix(memStr, "Mi") {
				memStr = strings.TrimSuffix(memStr, "Mi")
				if mem, err := strconv.ParseFloat(memStr, 64); err == nil {
					memGB = mem / 1024.0 // Convert MiB to GiB
				}
			} else if strings.HasSuffix(memStr, "Gi") {
				memStr = strings.TrimSuffix(memStr, "Gi")
				if mem, err := strconv.ParseFloat(memStr, 64); err == nil {
					memGB = mem
				}
			}
			metrics.UsedMemoryGB += memGB

			// Parse Memory percentage to estimate total
			memPercentStr := strings.TrimSuffix(fields[4], "%")
			if memPercent, err := strconv.ParseFloat(memPercentStr, 64); err == nil && memPercent > 0 && memGB > 0 {
				// Calculate total memory from used and percentage
				nodeTotalMem := (memGB / memPercent) * 100
				metrics.TotalMemoryGB += nodeTotalMem
			}
		}
	}
}

// parseNodeStatus parses the output of kubectl get nodes status column
func parseNodeStatus(output string, metrics *ClusterMetrics) {
	lines := strings.Split(output, "\n")
	
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		
		// Status is typically "Ready" or "NotReady"
		if strings.Contains(line, "Ready") && !strings.Contains(line, "NotReady") {
			metrics.NodesReady++
		}
	}
}