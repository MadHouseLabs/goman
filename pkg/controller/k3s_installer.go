package controller

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/madhouselabs/goman/pkg/models"
)

// reconcileInstalling handles K3s installation on all instances
func (r *Reconciler) reconcileInstalling(ctx context.Context, resource *models.ClusterResource) (*models.ReconcileResult, error) {
	log.Printf("[INSTALLING] Starting K3s installation for cluster %s", resource.Name)

	computeService := r.provider.GetComputeService()
	
	// Check SSM agent status for all instances first
	log.Printf("[INSTALLING] Checking SSM agent status for all instances...")
	allSSMReady := true
	ssmNotReadyInstances := []string{}
	
	for _, inst := range resource.Status.Instances {
		if inst.State != "running" {
			continue
		}
		
		// Try a simple echo command to check if SSM is ready
		testCmd := "echo 'SSM Ready'"
		result, err := computeService.RunCommand(ctx, []string{inst.InstanceID}, testCmd)
		
		if err != nil || result == nil || result.Status != "Success" {
			allSSMReady = false
			ssmNotReadyInstances = append(ssmNotReadyInstances, inst.Name)
			log.Printf("[INSTALLING] SSM agent not ready on instance %s (ID: %s)", inst.Name, inst.InstanceID)
		}
	}
	
	if !allSSMReady {
		log.Printf("[INSTALLING] SSM agent not ready on instances: %v. Waiting...", ssmNotReadyInstances)
		return &models.ReconcileResult{
			Requeue:      true,
			RequeueAfter: 30 * time.Second, // Wait 30 seconds for SSM agent
		}, nil
	}
	
	log.Printf("[INSTALLING] SSM agent is ready on all instances")

	// Check if all instances have K3s installed
	allInstalled := true
	hasErrors := false
	installationStatus := []string{}
	
	for i, inst := range resource.Status.Instances {
		if inst.K3sInstalled {
			log.Printf("[INSTALLING] Instance %s already has K3s installed (version: %s)", inst.Name, inst.K3sVersion)
			continue
		}

		// Check if instance is running before attempting installation
		if inst.State != "running" {
			log.Printf("[INSTALLING] Instance %s is not running (state: %s), cannot install K3s", inst.Name, inst.State)
			allInstalled = false
			installationStatus = append(installationStatus, fmt.Sprintf("%s is %s", inst.Name, inst.State))
			continue
		}

		// Check if we have already attempted installation and it failed
		if inst.K3sInstallError != "" {
			log.Printf("[INSTALLING] Instance %s has previous K3s installation error: %s", inst.Name, inst.K3sInstallError)
			hasErrors = true
			continue
		}

		allInstalled = false
		
		// Check K3s installation status on this instance
		log.Printf("[INSTALLING] Checking K3s installation status on instance %s", inst.Name)
		
		// First, check if K3s binary is already installed (idempotency check)
		checkCmd := "if [ -f /usr/local/bin/k3s ] && /usr/local/bin/k3s --version &>/dev/null; then /usr/local/bin/k3s --version 2>&1 | head -n1; else echo 'NOT_INSTALLED'; fi"
		result, err := computeService.RunCommand(ctx, []string{inst.InstanceID}, checkCmd)
		
		if err == nil && result != nil && result.Status == "Success" {
			// Check if K3s binary exists and get version
			if output, ok := result.Instances[inst.InstanceID]; ok {
				outputStr := strings.TrimSpace(output.Output)
				if outputStr != "NOT_INSTALLED" && strings.Contains(outputStr, "k3s version") {
					// K3s binary is already installed
					log.Printf("[INSTALLING] K3s binary is already installed on %s", inst.Name)
					
					// Update status with version if not already set
					if !resource.Status.Instances[i].K3sInstalled {
						now := time.Now()
						resource.Status.Instances[i].K3sInstalled = true
						resource.Status.Instances[i].K3sVersion = outputStr
						resource.Status.Instances[i].K3sInstallTime = &now
						log.Printf("[INSTALLING] Updated instance %s status as K3s installed (version: %s)", inst.Name, outputStr)
					}
					continue
				}
			}
		}

		// K3s is not installed, prepare installation
		log.Printf("[INSTALLING] K3s not found on instance %s, preparing installation", inst.Name)
		
		// Generate K3s installation script
		installScript := r.generateK3sInstallScript(resource, &inst)
		
		// Execute installation script
		log.Printf("[INSTALLING] Installing K3s on instance %s", inst.Name)
		installResult, err := computeService.RunCommand(ctx, []string{inst.InstanceID}, installScript)
		
		if err != nil {
			log.Printf("[INSTALLING] Failed to execute K3s installation on %s: %v", inst.Name, err)
			resource.Status.Instances[i].K3sInstallError = fmt.Sprintf("Failed to execute install command: %v", err)
			hasErrors = true
			continue
		}
		
		// Check installation result
		if installResult.Status != "Success" {
			errorMsg := "Installation failed"
			if output, ok := installResult.Instances[inst.InstanceID]; ok {
				errorMsg = fmt.Sprintf("Exit code %d: %s", output.ExitCode, output.Error)
			}
			log.Printf("[INSTALLING] K3s installation failed on %s: %s", inst.Name, errorMsg)
			resource.Status.Instances[i].K3sInstallError = errorMsg
			hasErrors = true
			continue
		}
		
		log.Printf("[INSTALLING] K3s installation initiated on instance %s", inst.Name)
		installationStatus = append(installationStatus, fmt.Sprintf("%s installing", inst.Name))
	}

	// Save state after checking/updating installation status
	if err := r.saveClusterResource(ctx, resource); err != nil {
		log.Printf("[INSTALLING] Warning: failed to save installation status: %v", err)
	}

	// Handle different installation states
	if hasErrors {
		log.Printf("[INSTALLING] K3s installation failed on some instances for cluster %s", resource.Name)
		resource.Status.Phase = models.ClusterPhaseFailed
		resource.Status.Message = "K3s installation failed on some instances"
		return &models.ReconcileResult{}, nil
	}

	if !allInstalled {
		// Some instances still need K3s installation
		status := "Installing K3s"
		if len(installationStatus) > 0 {
			status = fmt.Sprintf("Installing K3s: %s", strings.Join(installationStatus, ", "))
		}
		resource.Status.Message = status
		log.Printf("[INSTALLING] K3s installation in progress for cluster %s", resource.Name)
		return &models.ReconcileResult{
			Requeue:      true,
			RequeueAfter: 30 * time.Second, // Check every 30 seconds
		}, nil
	}

	// All instances have K3s binary installed
	log.Printf("[INSTALLING] K3s binary installed on all instances for cluster %s", resource.Name)
	
	// Note: We're only installing the binary, not configuring or starting K3s services yet
	// Cluster formation and service configuration will be handled in the Configuring phase
	
	// Transition to Configuring phase to start K3s services
	resource.Status.Phase = models.ClusterPhaseConfiguring
	resource.Status.Message = "K3s binary installed, starting configuration"
	now := time.Now()
	resource.Status.LastReconcileTime = &now
	
	log.Printf("[INSTALLING] Cluster %s transitioned to configuring phase", resource.Name)
	return &models.ReconcileResult{
		Requeue:      true,
		RequeueAfter: 5 * time.Second,
	}, nil
}

// generateK3sInstallScript generates the installation script for K3s
func (r *Reconciler) generateK3sInstallScript(resource *models.ClusterResource, instance *models.InstanceStatus) string {
	// Determine K3s version to install
	k3sVersion := resource.Spec.K3sVersion
	if k3sVersion == "" {
		k3sVersion = "v1.33.3+k3s1" // Default to a specific stable version
	}

	// For now, just download directly from GitHub since AWS CLI is not available on instances
	// In the future, we can generate pre-signed URLs or install AWS CLI first
	script := fmt.Sprintf(`#!/bin/bash
set -e

# Check if K3s binary is already installed
if [ -f /usr/local/bin/k3s ] && /usr/local/bin/k3s --version &> /dev/null; then
    echo "K3s binary is already installed"
    exit 0
fi

echo "Installing K3s binary version: %s"

# Detect architecture
ARCH=$(uname -m)
case $ARCH in
    x86_64)
        ARCH="amd64"
        ;;
    aarch64)
        ARCH="arm64"
        ;;
    armv7l)
        ARCH="armhf"
        ;;
    *)
        echo "Unsupported architecture: $ARCH"
        exit 1
        ;;
esac

# Download K3s binary from GitHub releases
echo "Downloading K3s from GitHub..."
DOWNLOAD_URL="https://github.com/k3s-io/k3s/releases/download/%s/k3s"
if [ "$ARCH" != "amd64" ]; then
    DOWNLOAD_URL="https://github.com/k3s-io/k3s/releases/download/%s/k3s-${ARCH}"
fi

echo "Downloading from: $DOWNLOAD_URL"
curl -sfL -o /tmp/k3s "$DOWNLOAD_URL"

if [ ! -f /tmp/k3s ]; then
    echo "Failed to download K3s binary"
    exit 1
fi

# Make it executable and move to /usr/local/bin
chmod +x /tmp/k3s
sudo mv /tmp/k3s /usr/local/bin/k3s

# Create symbolic links for kubectl, crictl, ctr
sudo ln -sf /usr/local/bin/k3s /usr/local/bin/kubectl
sudo ln -sf /usr/local/bin/k3s /usr/local/bin/crictl
sudo ln -sf /usr/local/bin/k3s /usr/local/bin/ctr

# Verify installation
/usr/local/bin/k3s --version
echo "K3s binary installation completed successfully"
`, k3sVersion, k3sVersion, k3sVersion)

	return script
}

// checkK3sHealth checks if K3s binary is installed on an instance
func (r *Reconciler) checkK3sHealth(ctx context.Context, instanceID string) (bool, error) {
	computeService := r.provider.GetComputeService()
	
	// Check if K3s binary exists and is executable
	healthCmd := "[ -f /usr/local/bin/k3s ] && /usr/local/bin/k3s --version &> /dev/null && echo 'installed' || echo 'not installed'"
	result, err := computeService.RunCommand(ctx, []string{instanceID}, healthCmd)
	
	if err != nil {
		return false, fmt.Errorf("failed to check K3s binary: %w", err)
	}
	
	if result.Status != "Success" {
		return false, nil
	}
	
	if output, ok := result.Instances[instanceID]; ok {
		// Check if output indicates K3s is installed
		return strings.Contains(output.Output, "installed") && !strings.Contains(output.Output, "not installed"), nil
	}
	
	return false, nil
}