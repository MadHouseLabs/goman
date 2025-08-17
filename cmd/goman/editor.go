package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/madhouselabs/goman/pkg/models"
	"gopkg.in/yaml.v2"
)

// editCluster opens vim editor to edit a cluster configuration
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
		// Convert cluster to YAML format for editing - only show editable fields
		yamlContent := fmt.Sprintf(`# Editing: %s
# Mode: %s | Status: %s | Created: %s
# 
# Read-only: name, mode, k3s version, network settings
# Editable: description, region, instanceType

description: "%s"
region: %s
instanceType: %s
`, cluster.Name, cluster.Mode, cluster.Status, cluster.CreatedAt.Format("2006-01-02"),
			cluster.Description, 
			cluster.Region,
			cluster.InstanceType)

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

// openClusterEditor opens vim editor to create a new cluster
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
		defaultYAML := fmt.Sprintf(`# New K3s Cluster

name: %s
description: "Development cluster"
mode: developer          # developer (1 master) or ha (3 masters)
region: ap-south-1
instanceType: t3.medium
k3sVersion: latest
`, uniqueName)

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

		// Read the edited content
		content, err := ioutil.ReadFile(tmpFilePath)
		if err != nil {
			return
		}

		yamlContent := string(content)
		
		// Validate and create cluster - keep retrying on errors
		for {
			if err := validateAndCreateClusterFromEditor(yamlContent); err != nil {
				// Write validation error as comment at the top of the file
				errorContent := fmt.Sprintf("# ERROR: %s\n# Please fix the error above and save again, or exit without saving to cancel.\n#\n%s", err.Error(), yamlContent)
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
				yamlContent = string(content)
				
				// Remove error comments before retrying
				lines := strings.Split(yamlContent, "\n")
				var cleanLines []string
				for _, line := range lines {
					if !strings.HasPrefix(line, "# ERROR:") && !strings.HasPrefix(line, "# Please fix") {
						cleanLines = append(cleanLines, line)
					}
				}
				yamlContent = strings.Join(cleanLines, "\n")
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

// validateAndCreateClusterFromEditor parses YAML and creates a new cluster
func validateAndCreateClusterFromEditor(yamlContent string) error {
	// Parse YAML
	var config map[string]interface{}
	if err := yaml.Unmarshal([]byte(yamlContent), &config); err != nil {
		return fmt.Errorf("invalid YAML: %v", err)
	}
	
	// Validate required fields
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

// validateAndUpdateClusterFromEditor parses YAML and updates an existing cluster
func validateAndUpdateClusterFromEditor(originalCluster models.K3sCluster, yamlContent string) error {
	// Parse YAML
	var config map[string]interface{}
	if err := yaml.Unmarshal([]byte(yamlContent), &config); err != nil {
		return fmt.Errorf("invalid YAML: %v", err)
	}
	
	// Immutable fields - use original values
	name := originalCluster.Name
	mode := string(originalCluster.Mode)
	// K3s version and network settings are immutable after creation
	
	// Editable fields
	region, ok := config["region"].(string)
	if !ok || region == "" {
		region = originalCluster.Region
	}
	
	instanceType, ok := config["instanceType"].(string)
	if !ok || instanceType == "" {
		instanceType = originalCluster.InstanceType
	}
	
	// Extract description
	description, _ := config["description"].(string)
	if description == "" {
		description = originalCluster.Description
	}
	
	// Update the cluster (description, region, and instanceType can change)
	return updateExistingCluster(originalCluster.Name, name, description, mode, region, instanceType)
}

// createNewClusterFromEditor creates a cluster from editor without UI
func createNewClusterFromEditor(name, description, mode, region, instanceType, nodeCountStr string) {
	createNewClusterWithUI(name, description, mode, region, instanceType, nodeCountStr, false)
}

// updateExistingCluster updates an existing cluster configuration
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