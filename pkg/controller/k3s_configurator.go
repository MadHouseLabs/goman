package controller

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/madhouselabs/goman/pkg/models"
)

// reconcileConfiguring handles K3s server configuration and startup
func (r *Reconciler) reconcileConfiguring(ctx context.Context, resource *models.ClusterResource) (*models.ReconcileResult, error) {
	log.Printf("[CONFIGURING] Starting K3s server configuration for cluster %s", resource.Name)

	// Check if all instances have K3s installed
	for _, inst := range resource.Status.Instances {
		if !inst.K3sInstalled {
			log.Printf("[CONFIGURING] Instance %s does not have K3s installed yet", inst.Name)
			// Go back to installing phase
			resource.Status.Phase = models.ClusterPhaseInstalling
			return &models.ReconcileResult{
				Requeue:      true,
				RequeueAfter: 10 * time.Second,
			}, nil
		}
	}

	// Configure based on cluster mode
	if resource.Spec.Mode == "ha" {
		return r.configureHACluster(ctx, resource)
	}
	return r.configureDeveloperCluster(ctx, resource)
}

// configureDeveloperCluster configures a single-master developer cluster
func (r *Reconciler) configureDeveloperCluster(ctx context.Context, resource *models.ClusterResource) (*models.ReconcileResult, error) {
	log.Printf("[CONFIGURING] Configuring developer mode cluster %s", resource.Name)

	computeService := r.provider.GetComputeService()
	storageService := r.provider.GetStorageService()

	// Find the master node
	var masterNode *models.InstanceStatus
	for i := range resource.Status.Instances {
		if resource.Status.Instances[i].Role == "master" {
			masterNode = &resource.Status.Instances[i]
			break
		}
	}

	if masterNode == nil {
		return nil, fmt.Errorf("no master node found in cluster")
	}

	// Check if K3s is already running
	if masterNode.K3sRunning {
		log.Printf("[CONFIGURING] K3s already running on master node %s", masterNode.Name)
		return r.extractAndSaveKubeconfig(ctx, resource, masterNode)
	}

	log.Printf("[CONFIGURING] Starting K3s server on %s", masterNode.Name)

	// Generate K3s server startup script for developer mode
	startScript := fmt.Sprintf(`#!/bin/bash
set -e

# Create K3s configuration directory
sudo mkdir -p /etc/rancher/k3s

# Create K3s server configuration
cat <<EOF | sudo tee /etc/rancher/k3s/config.yaml
cluster-init: true
tls-san:
  - %s
node-ip: %s
disable:
  - traefik
write-kubeconfig-mode: "0644"
EOF

# Create systemd service
sudo cat <<'EOSERVICE' | sudo tee /etc/systemd/system/k3s.service
[Unit]
Description=Lightweight Kubernetes
Documentation=https://k3s.io
After=network-online.target

[Service]
Type=notify
ExecStartPre=-/sbin/modprobe br_netfilter
ExecStartPre=-/sbin/modprobe overlay
ExecStart=/usr/local/bin/k3s server
KillMode=process
Delegate=yes
LimitNOFILE=1048576
LimitNPROC=infinity
LimitCORE=infinity
TasksMax=infinity
TimeoutStartSec=0
Restart=always
RestartSec=5s

[Install]
WantedBy=multi-user.target
EOSERVICE

# Start K3s service
sudo systemctl daemon-reload
sudo systemctl enable k3s
sudo systemctl start k3s

# Start K3s and let it initialize
echo "K3s service started"

# Get the server token for future use
sudo cat /var/lib/rancher/k3s/server/node-token
`, masterNode.PrivateIP, masterNode.PrivateIP)

	// Execute the startup script
	result, err := computeService.RunCommand(ctx, []string{masterNode.InstanceID}, startScript)
	if err != nil {
		log.Printf("[CONFIGURING] Failed to start K3s on %s: %v", masterNode.Name, err)
		masterNode.K3sConfigError = fmt.Sprintf("Failed to start K3s: %v", err)
		return &models.ReconcileResult{
			Requeue:      true,
			RequeueAfter: 30 * time.Second,
		}, nil
	}

	if result.Status != "Success" {
		errorMsg := "K3s startup failed"
		if output, ok := result.Instances[masterNode.InstanceID]; ok {
			errorMsg = fmt.Sprintf("Exit code %d: %s", output.ExitCode, output.Error)
		}
		log.Printf("[CONFIGURING] K3s startup failed on %s: %s", masterNode.Name, errorMsg)
		masterNode.K3sConfigError = errorMsg
		return &models.ReconcileResult{
			Requeue:      true,
			RequeueAfter: 30 * time.Second,
		}, nil
	}

	// Extract the server token from output
	if output, ok := result.Instances[masterNode.InstanceID]; ok {
		lines := strings.Split(output.Output, "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "K3S") && strings.Contains(line, "::") {
				resource.Status.K3sServerToken = line
				log.Printf("[CONFIGURING] Got K3s server token for cluster %s", resource.Name)
				
				// Save token to S3 for future reference
				tokenKey := fmt.Sprintf("clusters/%s/k3s-server-token", resource.Name)
				if err := storageService.PutObject(ctx, tokenKey, []byte(line)); err != nil {
					log.Printf("[CONFIGURING] Warning: Failed to save server token to S3: %v", err)
				}
				break
			}
		}
	}

	// Update node status
	now := time.Now()
	masterNode.K3sRunning = true
	masterNode.K3sConfigTime = &now
	masterNode.K3sConfigError = ""

	// Save status
	if err := r.saveClusterResource(ctx, resource); err != nil {
		log.Printf("[CONFIGURING] Warning: failed to save configuration status: %v", err)
	}

	log.Printf("[CONFIGURING] K3s server started successfully on %s", masterNode.Name)

	// Extract and save kubeconfig
	return r.extractAndSaveKubeconfig(ctx, resource, masterNode)
}

// configureHACluster configures a 3-master HA cluster
func (r *Reconciler) configureHACluster(ctx context.Context, resource *models.ClusterResource) (*models.ReconcileResult, error) {
	log.Printf("[CONFIGURING] Configuring HA mode cluster %s", resource.Name)

	computeService := r.provider.GetComputeService()
	storageService := r.provider.GetStorageService()

	// Collect master nodes
	var masterNodes []*models.InstanceStatus
	for i := range resource.Status.Instances {
		if resource.Status.Instances[i].Role == "master" {
			masterNodes = append(masterNodes, &resource.Status.Instances[i])
		}
	}

	if len(masterNodes) != 3 {
		return nil, fmt.Errorf("HA mode requires exactly 3 master nodes, found %d", len(masterNodes))
	}

	// Sort master nodes by name to ensure consistent ordering
	// This is critical so the same node is always treated as the first master
	sort.Slice(masterNodes, func(i, j int) bool {
		return masterNodes[i].Name < masterNodes[j].Name
	})

	// Use the first master's private IP as the cluster endpoint
	// This is simpler and more reliable than using DNS
	firstMasterIP := masterNodes[0].PrivateIP
	log.Printf("[CONFIGURING] Using first master IP %s as cluster endpoint", firstMasterIP)

	// Check if all masters are already running
	allRunning := true
	for _, node := range masterNodes {
		if !node.K3sRunning {
			allRunning = false
			break
		}
	}

	if allRunning {
		log.Printf("[CONFIGURING] All master nodes already running K3s")
		return r.extractAndSaveKubeconfig(ctx, resource, masterNodes[0])
	}

	// Start first master with cluster-init
	firstMaster := masterNodes[0]
	if !firstMaster.K3sRunning {
		log.Printf("[CONFIGURING] Starting first master %s with cluster-init", firstMaster.Name)
		
		startScript := fmt.Sprintf(`#!/bin/bash
set -e

# Complete cleanup of any existing K3s installation
echo "Cleaning up any existing K3s installation..."
sudo systemctl stop k3s || true
sudo systemctl stop k3s-agent || true

# Kill any remaining K3s or containerd processes
sudo pkill -9 -f "k3s" || true
sudo pkill -9 -f "containerd-shim" || true
sleep 2

# Remove all K3s data and configuration
sudo rm -rf /var/lib/rancher/k3s
sudo rm -rf /etc/rancher/k3s
sudo rm -rf /var/lib/kubelet
sudo rm -rf /var/lib/cni
sudo rm -rf /run/k3s
sudo rm -rf /run/containerd

# Create K3s configuration directory
sudo mkdir -p /etc/rancher/k3s

# Create K3s server configuration for first master
cat <<EOF | sudo tee /etc/rancher/k3s/config.yaml
cluster-init: true
tls-san:
  - %s
node-ip: %s
cluster-cidr: "10.42.0.0/16"
service-cidr: "10.43.0.0/16"
cluster-dns: "10.43.0.10"
disable:
  - traefik
write-kubeconfig-mode: "0644"
etcd-expose-metrics: true
EOF

# Create systemd service
sudo cat <<'EOSERVICE' | sudo tee /etc/systemd/system/k3s.service
[Unit]
Description=Lightweight Kubernetes
Documentation=https://k3s.io
After=network-online.target

[Service]
Type=notify
ExecStartPre=-/sbin/modprobe br_netfilter
ExecStartPre=-/sbin/modprobe overlay
ExecStart=/usr/local/bin/k3s server
KillMode=process
Delegate=yes
LimitNOFILE=1048576
LimitNPROC=infinity
LimitCORE=infinity
TasksMax=infinity
TimeoutStartSec=0
Restart=always
RestartSec=5s

[Install]
WantedBy=multi-user.target
EOSERVICE

# Start K3s service
sudo systemctl daemon-reload
sudo systemctl enable k3s

echo "Starting K3s service..."
sudo systemctl start k3s

# Wait for service to be active and verify it starts successfully
echo "Verifying K3s service startup..."
for i in {1..60}; do
  if sudo systemctl is-active k3s >/dev/null 2>&1; then
    echo "K3s service is active"
    break
  fi
  
  if [ $i -eq 60 ]; then
    echo "ERROR: K3s service failed to start within 2 minutes"
    echo "=== Service Status ==="
    sudo systemctl status k3s || true
    echo "=== Service Logs ==="
    sudo journalctl -u k3s --no-pager -n 30 || true
    exit 1
  fi
  
  echo "Waiting for K3s service to start... ($i/60)"
  sleep 2
done

# Wait for K3s API to be ready
echo "Waiting for K3s API server to be ready..."
for i in {1..60}; do
  if curl -k --connect-timeout 5 --max-time 10 https://127.0.0.1:6443/livez >/dev/null 2>&1; then
    echo "K3s API server is ready"
    break
  fi
  
  if [ $i -eq 60 ]; then
    echo "ERROR: K3s API server not ready after 5 minutes"
    echo "=== K3s Logs ==="
    sudo journalctl -u k3s --no-pager -n 50 || true
    exit 1
  fi
  
  echo "Waiting for K3s API server... ($i/60)"
  sleep 5
done

# Wait for etcd to be ready (critical for HA clusters)
echo "Waiting for etcd to be ready..."
for i in {1..30}; do
  if sudo k3s kubectl get nodes >/dev/null 2>&1; then
    echo "etcd and K3s cluster initialized successfully"
    break
  fi
  
  if [ $i -eq 30 ]; then
    echo "ERROR: etcd/cluster not ready after 2.5 minutes"
    echo "=== K3s Logs ==="
    sudo journalctl -u k3s --no-pager -n 30 || true
    exit 1
  fi
  
  echo "Waiting for etcd/cluster to be ready... ($i/30)"
  sleep 5
done

echo "First master fully initialized and ready for additional masters to join"

# Get the server token with clear marker
echo "===TOKEN_START==="
sudo cat /var/lib/rancher/k3s/server/node-token
echo "===TOKEN_END==="
`, firstMaster.PrivateIP, firstMaster.PrivateIP)

		result, err := computeService.RunCommand(ctx, []string{firstMaster.InstanceID}, startScript)
		if err != nil {
			log.Printf("[CONFIGURING] Failed to start first master: %v", err)
			firstMaster.K3sConfigError = fmt.Sprintf("Failed to start K3s: %v", err)
			return &models.ReconcileResult{
				Requeue:      true,
				RequeueAfter: 30 * time.Second,
			}, nil
		}

		if result.Status != "Success" {
			errorMsg := "K3s startup failed"
			if output, ok := result.Instances[firstMaster.InstanceID]; ok {
				errorMsg = fmt.Sprintf("Exit code %d: %s", output.ExitCode, output.Error)
			}
			firstMaster.K3sConfigError = errorMsg
			return &models.ReconcileResult{
				Requeue:      true,
				RequeueAfter: 30 * time.Second,
			}, nil
		}

		// Extract server token
		tokenFound := false
		if output, ok := result.Instances[firstMaster.InstanceID]; ok {
			lines := strings.Split(output.Output, "\n")
			log.Printf("[CONFIGURING] Searching for token in %d lines of output", len(lines))
			
			// First try to find token between markers
			inTokenSection := false
			for _, line := range lines {
				line = strings.TrimSpace(line)
				
				if line == "===TOKEN_START===" {
					inTokenSection = true
					continue
				}
				if line == "===TOKEN_END===" {
					inTokenSection = false
					continue
				}
				
				if inTokenSection && strings.HasPrefix(line, "K10") && strings.Contains(line, "::server:") {
					resource.Status.K3sServerToken = line
					log.Printf("[CONFIGURING] Found K3s server token (from markers) for cluster %s", resource.Name)
					tokenFound = true
					break
				}
			}
			
			// If not found between markers, search all lines
			if !tokenFound {
				for _, line := range lines {
					line = strings.TrimSpace(line)
					// K3s tokens start with K10, not K3S
					if strings.HasPrefix(line, "K10") && strings.Contains(line, "::server:") {
						resource.Status.K3sServerToken = line
						log.Printf("[CONFIGURING] Found K3s server token for cluster %s", resource.Name)
						tokenFound = true
						break
					}
				}
			}
			
			if tokenFound {
				// Save token to S3
				tokenKey := fmt.Sprintf("clusters/%s/k3s-server-token", resource.Name)
				if err := storageService.PutObject(ctx, tokenKey, []byte(resource.Status.K3sServerToken)); err != nil {
					log.Printf("[CONFIGURING] Failed to save server token to S3: %v", err)
					tokenFound = false
				} else {
					log.Printf("[CONFIGURING] Successfully saved server token to S3")
				}
			} else {
				log.Printf("[CONFIGURING] Token not found in script output, will retry with direct command")
			}
		}
		
		// If token not found in output, try to get it directly
		if !tokenFound {
			log.Printf("[CONFIGURING] Attempting to retrieve token with separate command")
			tokenCmd := `sudo cat /var/lib/rancher/k3s/server/node-token 2>/dev/null || echo "NO_TOKEN"`
			tokenResult, err := computeService.RunCommand(ctx, []string{firstMaster.InstanceID}, tokenCmd)
			if err == nil && tokenResult.Status == "Success" {
				if output, ok := tokenResult.Instances[firstMaster.InstanceID]; ok {
					token := strings.TrimSpace(output.Output)
					if strings.HasPrefix(token, "K10") && strings.Contains(token, "::server:") {
						resource.Status.K3sServerToken = token
						log.Printf("[CONFIGURING] Successfully retrieved token with separate command")
						
						// Save token to S3
						tokenKey := fmt.Sprintf("clusters/%s/k3s-server-token", resource.Name)
						if err := storageService.PutObject(ctx, tokenKey, []byte(token)); err != nil {
							log.Printf("[CONFIGURING] Failed to save server token to S3: %v", err)
						} else {
							log.Printf("[CONFIGURING] Successfully saved server token to S3")
							tokenFound = true
						}
					}
				}
			}
		}
		
		if !tokenFound {
			log.Printf("[CONFIGURING] ERROR: Failed to extract K3s server token for cluster %s", resource.Name)
		}

		// Update first master status
		now := time.Now()
		firstMaster.K3sRunning = true
		firstMaster.K3sConfigTime = &now
		firstMaster.K3sConfigError = ""

		// Save status
		if err := r.saveClusterResource(ctx, resource); err != nil {
			log.Printf("[CONFIGURING] Warning: failed to save status: %v", err)
		}

		// Wait longer for first master to fully stabilize with HA etcd
		log.Printf("[CONFIGURING] First master started successfully, waiting for full stabilization...")
		return &models.ReconcileResult{
			Requeue:      true,
			RequeueAfter: 60 * time.Second,
		}, nil
	}

	// Start additional masters in parallel after first master is ready
	// Collect masters that need to be started
	var mastersToStart []*models.InstanceStatus
	for i := 1; i < 3; i++ {
		node := masterNodes[i]
		if !node.K3sRunning {
			mastersToStart = append(mastersToStart, node)
		} else {
			log.Printf("[CONFIGURING] Master %s already running K3s", node.Name)
		}
	}

	// If we have masters to start, start them in parallel
	if len(mastersToStart) > 0 {
		log.Printf("[CONFIGURING] Starting %d additional masters in parallel to join cluster", len(mastersToStart))
		
		// Get token from S3 if not in status
		if resource.Status.K3sServerToken == "" {
			tokenKey := fmt.Sprintf("clusters/%s/k3s-server-token", resource.Name)
			tokenData, err := storageService.GetObject(ctx, tokenKey)
			if err != nil {
				log.Printf("[CONFIGURING] Failed to get server token from S3: %v", err)
				return &models.ReconcileResult{
					Requeue:      true,
					RequeueAfter: 30 * time.Second,
				}, nil
			}
			resource.Status.K3sServerToken = string(tokenData)
			log.Printf("[CONFIGURING] Retrieved server token from S3")
		}
		
		// Start all pending masters in parallel
		var instanceIDs []string
		for _, master := range mastersToStart {
			instanceIDs = append(instanceIDs, master.InstanceID)
			log.Printf("[CONFIGURING] Will join master %s to first master at %s:6443", master.Name, firstMaster.PrivateIP)
		}

		// Create join script that gets the node's IP dynamically
		joinScript := fmt.Sprintf(`#!/bin/bash
set -e

# Get this node's private IP
NODE_IP=$(hostname -I | awk '{print $1}')
echo "This node IP: ${NODE_IP}"

# Complete cleanup of any existing K3s installation
echo "Cleaning up any existing K3s installation on joining master..."
sudo systemctl stop k3s || true
sudo systemctl stop k3s-agent || true
sudo systemctl disable k3s || true

# Graceful process cleanup instead of kill -9
echo "Gracefully stopping K3s processes..."
sudo pkill -TERM k3s || true
sleep 5
sudo pkill -9 k3s || true
sudo pkill -TERM containerd-shim || true
sleep 3
sudo pkill -9 containerd-shim || true
sleep 2

# Remove ALL K3s data and configuration to ensure clean state
sudo rm -rf /var/lib/rancher/k3s
sudo rm -rf /etc/rancher/k3s
sudo rm -rf /var/lib/kubelet
sudo rm -rf /var/lib/cni
sudo rm -rf /run/k3s
sudo rm -rf /run/containerd
sudo rm -f /etc/systemd/system/k3s*.service

# Reload systemd to clear old service files
sudo systemctl daemon-reload

# Pre-flight checks
echo "Performing pre-flight checks..."

# Check connectivity to first master
FIRST_MASTER_IP="%s"
if ! nc -z ${FIRST_MASTER_IP} 6443; then
  echo "ERROR: Cannot reach first master at ${FIRST_MASTER_IP}:6443"
  exit 1
fi

# Wait for first master to be fully ready
echo "Waiting for first master API to be ready..."
for i in {1..60}; do
  if curl -k --connect-timeout 5 --max-time 10 https://${FIRST_MASTER_IP}:6443/livez >/dev/null 2>&1; then
    echo "First master API is ready"
    break
  fi
  if [ $i -eq 60 ]; then
    echo "ERROR: First master API not ready after 5 minutes"
    exit 1
  fi
  echo "Waiting for first master API... ($i/60)"
  sleep 5
done

# Create K3s configuration directory
sudo mkdir -p /etc/rancher/k3s

# Create K3s server configuration for joining master
# CRITICAL: Use server + token configuration to join existing cluster
# This tells K3s to join the existing cluster instead of initializing etcd
cat <<EOF | sudo tee /etc/rancher/k3s/config.yaml
server: https://%s:6443
token: %s
tls-san:
  - ${NODE_IP}
node-ip: ${NODE_IP}
cluster-cidr: "10.42.0.0/16"
service-cidr: "10.43.0.0/16"
cluster-dns: "10.43.0.10"
disable:
  - traefik
write-kubeconfig-mode: "0644"
EOF

# Create systemd service with explicit join parameters
# CRITICAL: Pass server and token as command line args to force join mode
cat <<EOSERVICE | sudo tee /etc/systemd/system/k3s.service
[Unit]
Description=Lightweight Kubernetes
Documentation=https://k3s.io
After=network-online.target

[Service]
Type=notify
ExecStartPre=-/sbin/modprobe br_netfilter
ExecStartPre=-/sbin/modprobe overlay
ExecStart=/usr/local/bin/k3s server --server https://%s:6443 --token %s
KillMode=process
Delegate=yes
LimitNOFILE=1048576
LimitNPROC=infinity
LimitCORE=infinity
TasksMax=infinity
TimeoutStartSec=0
Restart=always
RestartSec=5s

[Install]
WantedBy=multi-user.target
EOSERVICE

# Start K3s service with verification
sudo systemctl daemon-reload
sudo systemctl enable k3s

echo "Starting K3s service..."
sudo systemctl start k3s

# Wait for service to be active and verify it starts successfully
echo "Verifying K3s service startup..."
for i in {1..30}; do
  if sudo systemctl is-active k3s >/dev/null 2>&1; then
    echo "K3s service started successfully"
    
    # Additional verification - check if server is responding
    sleep 5
    if curl -k --connect-timeout 5 https://127.0.0.1:6443/livez >/dev/null 2>&1; then
      echo "K3s server is responding on local interface"
      exit 0
    else
      echo "K3s service active but server not yet responding, will check in next reconciliation"
      exit 0
    fi
  fi
  
  if [ $i -eq 30 ]; then
    echo "ERROR: K3s service failed to start within 60 seconds"
    echo "=== Service Status ==="
    sudo systemctl status k3s || true
    echo "=== Service Logs ==="
    sudo journalctl -u k3s --no-pager -n 30 || true
    echo "=== Process Check ==="
    ps aux | grep k3s | grep -v grep || true
    exit 1
  fi
  
  echo "Waiting for K3s service to start... ($i/30)"
  sleep 2
done
`, firstMaster.PrivateIP, firstMaster.PrivateIP, resource.Status.K3sServerToken, firstMaster.PrivateIP, resource.Status.K3sServerToken)

		// Run the join script on all pending masters in parallel
		result, err := computeService.RunCommand(ctx, instanceIDs, joinScript)
		if err != nil {
			log.Printf("[CONFIGURING] Failed to start masters in parallel: %v", err)
			for _, master := range mastersToStart {
				master.K3sConfigError = fmt.Sprintf("Failed to join cluster: %v", err)
			}
			// Save status and requeue
			if err := r.saveClusterResource(ctx, resource); err != nil {
				log.Printf("[CONFIGURING] Warning: failed to save status: %v", err)
			}
			return &models.ReconcileResult{
				Requeue:      true,
				RequeueAfter: 30 * time.Second,
			}, nil
		}

		// Process results for each master
		successCount := 0
		failureCount := 0
		now := time.Now()
		
		for _, master := range mastersToStart {
			if output, ok := result.Instances[master.InstanceID]; ok {
				if output.Status == "Success" && output.ExitCode == 0 {
					master.K3sRunning = true
					master.K3sConfigTime = &now
					master.K3sConfigError = ""
					successCount++
					log.Printf("[CONFIGURING] Master %s joined cluster successfully", master.Name)
				} else {
					// Detailed error reporting for join failures
					errorMsg := "Unknown join failure"
					if output.Error != "" {
						errorMsg = fmt.Sprintf("Exit code %d: %s", output.ExitCode, output.Error)
					} else {
						errorMsg = fmt.Sprintf("Exit code %d (no error output)", output.ExitCode)
					}
					
					master.K3sConfigError = errorMsg
					failureCount++
					log.Printf("[CONFIGURING] Master %s failed to join: %s", master.Name, errorMsg)
					
					// Log the full output for debugging
					if output.Output != "" {
						log.Printf("[CONFIGURING] Master %s output:\n%s", master.Name, output.Output)
					}
				}
			} else {
				// No output for this instance
				master.K3sConfigError = "No command output received"
				failureCount++
				log.Printf("[CONFIGURING] No output received for master %s", master.Name)
			}
		}
		
		log.Printf("[CONFIGURING] Parallel join results: %d succeeded, %d failed out of %d masters", 
			successCount, failureCount, len(mastersToStart))
		
		// Save status
		if err := r.saveClusterResource(ctx, resource); err != nil {
			log.Printf("[CONFIGURING] Warning: failed to save status: %v", err)
		}
		
		// Requeue to verify cluster formation
		return &models.ReconcileResult{
			Requeue:      true,
			RequeueAfter: 30 * time.Second,
		}, nil
	}

	// Save status
	if err := r.saveClusterResource(ctx, resource); err != nil {
		log.Printf("[CONFIGURING] Warning: failed to save status: %v", err)
	}

	// Check if all masters are running
	allRunning = true
	for _, node := range masterNodes {
		if !node.K3sRunning {
			allRunning = false
			break
		}
	}

	if !allRunning {
		log.Printf("[CONFIGURING] Not all masters are running yet, will retry")
		return &models.ReconcileResult{
			Requeue:      true,
			RequeueAfter: 30 * time.Second,
		}, nil
	}

	log.Printf("[CONFIGURING] All master nodes report K3s running, verifying cluster formation")
	
	// Comprehensive cluster verification
	verifyCmd := `echo "=== Node Count ==="; sudo k3s kubectl get nodes --no-headers | wc -l; echo "=== Node Status ==="; sudo k3s kubectl get nodes; echo "=== Node Details ==="; sudo k3s kubectl get nodes -o wide; echo "=== Etcd Health ==="; sudo k3s kubectl get cs 2>/dev/null || echo "Component status not available"`
	
	result, err := computeService.RunCommand(ctx, []string{firstMaster.InstanceID}, verifyCmd)
	if err != nil {
		log.Printf("[CONFIGURING] Failed to verify cluster: %v", err)
		return &models.ReconcileResult{
			Requeue:      true,
			RequeueAfter: 30 * time.Second,
		}, nil
	}
	
	if result.Status != "Success" {
		log.Printf("[CONFIGURING] Cluster verification command failed")
		return &models.ReconcileResult{
			Requeue:      true,
			RequeueAfter: 30 * time.Second,
		}, nil
	}
	
	// Parse and validate the results
	var nodeCount string
	clusterReady := false
	
	if output, ok := result.Instances[firstMaster.InstanceID]; ok {
		log.Printf("[CONFIGURING] Cluster verification output:\n%s", output.Output)
		
		lines := strings.Split(output.Output, "\n")
		for i, line := range lines {
			line = strings.TrimSpace(line)
			
			// Look for node count after "=== Node Count ===" marker
			if line == "=== Node Count ===" && i+1 < len(lines) {
				nodeCount = strings.TrimSpace(lines[i+1])
				break
			}
		}
		
		// Validate node count
		if nodeCount == "3" {
			// Additional check: ensure all nodes are Ready
			readyNodes := 0
			for _, line := range lines {
				if strings.Contains(line, "Ready") && !strings.Contains(line, "NotReady") {
					readyNodes++
				}
			}
			
			if readyNodes >= 3 {
				clusterReady = true
				log.Printf("[CONFIGURING] Cluster verification successful: %s nodes, %d ready", nodeCount, readyNodes)
			} else {
				log.Printf("[CONFIGURING] Not all nodes are ready: %d ready out of %s total", readyNodes, nodeCount)
			}
		} else {
			log.Printf("[CONFIGURING] Expected 3 nodes but found %s", nodeCount)
		}
	}
	
	if !clusterReady {
		log.Printf("[CONFIGURING] Cluster not fully ready yet, will retry in 30 seconds")
		return &models.ReconcileResult{
			Requeue:      true,
			RequeueAfter: 30 * time.Second,
		}, nil
	}
	
	log.Printf("[CONFIGURING] All master nodes are running K3s and cluster is formed")
	return r.extractAndSaveKubeconfig(ctx, resource, masterNodes[0])
}

// extractAndSaveKubeconfig extracts kubeconfig from master and saves to S3
func (r *Reconciler) extractAndSaveKubeconfig(ctx context.Context, resource *models.ClusterResource, masterNode *models.InstanceStatus) (*models.ReconcileResult, error) {
	computeService := r.provider.GetComputeService()
	storageService := r.provider.GetStorageService()

	log.Printf("[CONFIGURING] Extracting kubeconfig from master %s", masterNode.Name)

	// Get kubeconfig from master
	getKubeconfigCmd := `sudo cat /etc/rancher/k3s/k3s.yaml`
	result, err := computeService.RunCommand(ctx, []string{masterNode.InstanceID}, getKubeconfigCmd)
	if err != nil {
		log.Printf("[CONFIGURING] Failed to get kubeconfig: %v", err)
		return &models.ReconcileResult{
			Requeue:      true,
			RequeueAfter: 30 * time.Second,
		}, nil
	}

	if result.Status != "Success" {
		log.Printf("[CONFIGURING] Failed to extract kubeconfig")
		return &models.ReconcileResult{
			Requeue:      true,
			RequeueAfter: 30 * time.Second,
		}, nil
	}

	// Get the kubeconfig content
	var kubeconfig string
	if output, ok := result.Instances[masterNode.InstanceID]; ok {
		kubeconfig = output.Output
	}

	if kubeconfig == "" {
		log.Printf("[CONFIGURING] Empty kubeconfig received")
		return &models.ReconcileResult{
			Requeue:      true,
			RequeueAfter: 30 * time.Second,
		}, nil
	}

	// Update the server URL in kubeconfig
	// Use public IP for external access (users need to connect from outside the VPC)
	serverURL := fmt.Sprintf("https://%s:6443", masterNode.PublicIP)
	
	// Replace the default server URL with the correct one
	kubeconfig = strings.ReplaceAll(kubeconfig, "https://127.0.0.1:6443", serverURL)
	kubeconfig = strings.ReplaceAll(kubeconfig, "server: https://localhost:6443", fmt.Sprintf("server: %s", serverURL))

	// Save kubeconfig to S3
	kubeconfigKey := fmt.Sprintf("clusters/%s/kubeconfig", resource.Name)
	if err := storageService.PutObject(ctx, kubeconfigKey, []byte(kubeconfig)); err != nil {
		log.Printf("[CONFIGURING] Failed to save kubeconfig to S3: %v", err)
		return &models.ReconcileResult{
			Requeue:      true,
			RequeueAfter: 30 * time.Second,
		}, nil
	}

	// Base64 encode and save in status
	resource.Status.KubeConfig = base64.StdEncoding.EncodeToString([]byte(kubeconfig))
	resource.Status.K3sServerURL = serverURL

	// Update cluster phase to Running
	resource.Status.Phase = models.ClusterPhaseRunning
	resource.Status.Message = "K3s cluster is running"
	now := time.Now()
	resource.Status.LastReconcileTime = &now

	// Save final status
	if err := r.saveClusterResource(ctx, resource); err != nil {
		log.Printf("[CONFIGURING] Warning: failed to save final status: %v", err)
	}

	log.Printf("[CONFIGURING] Cluster %s configuration complete, transitioning to Running phase", resource.Name)
	return &models.ReconcileResult{}, nil
}

// checkK3sServiceStatus checks if K3s service is running on an instance
func (r *Reconciler) checkK3sServiceStatus(ctx context.Context, instanceID string) (bool, error) {
	computeService := r.provider.GetComputeService()
	
	// Check K3s service status
	checkCmd := `sudo systemctl is-active k3s || echo "inactive"`
	result, err := computeService.RunCommand(ctx, []string{instanceID}, checkCmd)
	
	if err != nil {
		return false, fmt.Errorf("failed to check K3s service: %w", err)
	}
	
	if result.Status != "Success" {
		return false, nil
	}
	
	if output, ok := result.Instances[instanceID]; ok {
		status := strings.TrimSpace(output.Output)
		return status == "active", nil
	}
	
	return false, nil
}