package connectivity

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

// ActiveTunnelState represents the state of the single active tunnel
type ActiveTunnelState struct {
	ClusterName string    `json:"cluster_name"`
	InstanceID  string    `json:"instance_id"`
	Region      string    `json:"region"`
	PID         int       `json:"pid"`
	StartedAt   time.Time `json:"started_at"`
	LocalPort   int       `json:"local_port"`
	RemotePort  int       `json:"remote_port"`
}

// SingleTunnelManager manages a single active SSM tunnel
type SingleTunnelManager struct {
	stateFile string
	mu        sync.Mutex
}

// NewSingleTunnelManager creates a new single tunnel manager
func NewSingleTunnelManager() *SingleTunnelManager {
	homeDir, _ := os.UserHomeDir()
	stateFile := filepath.Join(homeDir, ".goman", "active-tunnel.json")
	
	// Ensure directory exists
	os.MkdirAll(filepath.Dir(stateFile), 0755)
	
	return &SingleTunnelManager{
		stateFile: stateFile,
	}
}

// LoadActiveTunnel loads the active tunnel state from disk
func (stm *SingleTunnelManager) LoadActiveTunnel() (*ActiveTunnelState, error) {
	data, err := os.ReadFile(stm.stateFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // No active tunnel
		}
		return nil, err
	}
	
	var state ActiveTunnelState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	
	return &state, nil
}

// SaveActiveTunnel saves the active tunnel state to disk
func (stm *SingleTunnelManager) SaveActiveTunnel(state *ActiveTunnelState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	
	return os.WriteFile(stm.stateFile, data, 0644)
}

// DeleteActiveTunnel removes the active tunnel state file
func (stm *SingleTunnelManager) DeleteActiveTunnel() error {
	err := os.Remove(stm.stateFile)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// IsProcessAlive checks if a process is still running
func (stm *SingleTunnelManager) IsProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	
	// Check if process exists
	err = process.Signal(syscall.Signal(0))
	return err == nil
}

// IsPortListening checks if port 6443 is listening
func (stm *SingleTunnelManager) IsPortListening(port int) bool {
	timeout := time.Millisecond * 100
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("localhost:%d", port), timeout)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// KillProcess kills a process and all its children by PID
func (stm *SingleTunnelManager) KillProcess(pid int) error {
	if pid <= 0 {
		return nil
	}
	
	// First kill the main process
	process, err := os.FindProcess(pid)
	if err == nil {
		process.Signal(syscall.SIGTERM)
		time.Sleep(200 * time.Millisecond)
		
		// Force kill if still alive
		if stm.IsProcessAlive(pid) {
			process.Signal(syscall.SIGKILL)
		}
	}
	
	// Kill any child processes using pkill
	// This gets both the aws ssm command and session-manager-plugin
	cmd := exec.Command("sh", "-c", fmt.Sprintf("pkill -P %d", pid))
	cmd.Run()
	
	// Also explicitly kill any session-manager-plugin processes
	// This handles cases where the child process might have detached
	cmd = exec.Command("sh", "-c", "pkill -f session-manager-plugin")
	cmd.Run()
	
	// Kill any remaining aws ssm processes
	cmd = exec.Command("sh", "-c", "pkill -f 'aws.*ssm.*start-session'")
	cmd.Run()
	
	// Give processes time to fully terminate
	time.Sleep(500 * time.Millisecond)
	
	return nil
}

// EnsureTunnel ensures a tunnel is running for the specified cluster
func (stm *SingleTunnelManager) EnsureTunnel(clusterName, instanceID, region string) error {
	stm.mu.Lock()
	defer stm.mu.Unlock()
	
	// Load current active tunnel state
	activeTunnel, err := stm.LoadActiveTunnel()
	if err != nil {
		fmt.Printf("Warning: Error loading active tunnel state: %v\n", err)
	}
	
	// Check if we have an active tunnel for the same cluster
	if activeTunnel != nil && activeTunnel.ClusterName == clusterName {
		// Check if the process is still alive and port is listening
		if stm.IsProcessAlive(activeTunnel.PID) && stm.IsPortListening(6443) {
			// Tunnel is healthy and for the same cluster, reuse it
			fmt.Printf("Reusing existing tunnel for cluster %s (PID: %d)\n", clusterName, activeTunnel.PID)
			return nil
		}
		// Tunnel is dead or unhealthy, clean it up
		fmt.Printf("Existing tunnel for cluster %s is dead or unhealthy, cleaning up\n", clusterName)
		stm.KillProcess(activeTunnel.PID)
		stm.DeleteActiveTunnel()
	}
	
	// If we have a tunnel for a different cluster, stop it completely
	if activeTunnel != nil && activeTunnel.ClusterName != clusterName {
		fmt.Printf("Stopping tunnel for cluster %s to switch to %s\n", activeTunnel.ClusterName, clusterName)
		stm.KillProcess(activeTunnel.PID)
		stm.DeleteActiveTunnel()
		
		// Ensure port 6443 is completely free
		// This will clean up any orphaned processes
		stm.cleanupPortWithoutLock(6443)
		
		// Wait for port to be fully freed
		time.Sleep(1 * time.Second)
	}
	
	// Start new tunnel in background
	fmt.Printf("Starting new SSM tunnel for cluster %s...\n", clusterName)
	pid, err := stm.StartBackgroundTunnel(instanceID, region)
	if err != nil {
		return fmt.Errorf("failed to start tunnel: %w", err)
	}
	
	// Save the new active tunnel state
	newState := &ActiveTunnelState{
		ClusterName: clusterName,
		InstanceID:  instanceID,
		Region:      region,
		PID:         pid,
		StartedAt:   time.Now(),
		LocalPort:   6443,
		RemotePort:  6443,
	}
	
	if err := stm.SaveActiveTunnel(newState); err != nil {
		fmt.Printf("Warning: Failed to save tunnel state: %v\n", err)
	}
	
	// Wait for tunnel to be established
	fmt.Printf("Waiting for tunnel to establish...")
	for i := 0; i < 50; i++ {
		if stm.IsPortListening(6443) {
			fmt.Printf("\n✅ SSM tunnel established for cluster %s (PID: %d)\n", clusterName, pid)
			return nil
		}
		if i%5 == 0 {
			fmt.Printf(".")
		}
		time.Sleep(200 * time.Millisecond)
	}
	
	// Tunnel failed to establish
	fmt.Printf("\n")
	stm.KillProcess(pid)
	stm.DeleteActiveTunnel()
	return fmt.Errorf("tunnel failed to establish after 10 seconds")
}

// StartBackgroundTunnel starts an SSM tunnel as a background process
func (stm *SingleTunnelManager) StartBackgroundTunnel(instanceID, region string) (int, error) {
	// Build SSM command
	args := []string{
		"ssm", "start-session",
		"--target", instanceID,
		"--document-name", "AWS-StartPortForwardingSession",
		"--parameters", `{"portNumber":["6443"],"localPortNumber":["6443"]}`,
	}
	
	if region != "" {
		args = append(args, "--region", region)
	}
	
	// Create command
	cmd := exec.Command("aws", args...)
	
	// Set up process to run in background
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true, // Create new session (detach from terminal)
	}
	
	// Redirect output to /dev/null to prevent blocking
	devNull, err := os.OpenFile("/dev/null", os.O_WRONLY, 0)
	if err != nil {
		return 0, fmt.Errorf("failed to open /dev/null: %w", err)
	}
	defer devNull.Close()
	
	cmd.Stdout = devNull
	cmd.Stderr = devNull
	cmd.Stdin = nil
	
	// Start the process
	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("failed to start SSM session: %w", err)
	}
	
	// Return the PID
	return cmd.Process.Pid, nil
}

// StopActiveTunnel stops the currently active tunnel
func (stm *SingleTunnelManager) StopActiveTunnel() error {
	stm.mu.Lock()
	defer stm.mu.Unlock()
	
	activeTunnel, err := stm.LoadActiveTunnel()
	if err != nil {
		return err
	}
	
	if activeTunnel == nil {
		return nil // No active tunnel
	}
	
	// Kill the process
	if err := stm.KillProcess(activeTunnel.PID); err != nil {
		return err
	}
	
	// Delete state file
	return stm.DeleteActiveTunnel()
}

// GetActiveTunnel returns the current active tunnel state
func (stm *SingleTunnelManager) GetActiveTunnel() (*ActiveTunnelState, error) {
	activeTunnel, err := stm.LoadActiveTunnel()
	if err != nil {
		return nil, err
	}
	
	// Verify the tunnel is still alive
	if activeTunnel != nil {
		if !stm.IsProcessAlive(activeTunnel.PID) {
			// Tunnel is dead, clean up
			stm.DeleteActiveTunnel()
			return nil, nil
		}
	}
	
	return activeTunnel, nil
}

// cleanupPortWithoutLock cleans up processes on a port (must be called with lock held)
func (stm *SingleTunnelManager) cleanupPortWithoutLock(port int) error {
	// Kill any remaining processes on the port
	cmd := exec.Command("sh", "-c", fmt.Sprintf("lsof -ti:%d", port))
	output, err := cmd.Output()
	if err != nil {
		return nil // No processes found
	}
	
	// Kill each process
	pids := string(output)
	if pids != "" {
		for _, pidStr := range splitLines(pids) {
			if pidStr != "" {
				var pid int
				fmt.Sscanf(pidStr, "%d", &pid)
				if pid > 0 {
					// Kill the process and its group
					syscall.Kill(-pid, syscall.SIGKILL)
				}
			}
		}
	}
	
	// Also explicitly cleanup session-manager-plugin
	exec.Command("sh", "-c", "pkill -f session-manager-plugin").Run()
	
	return nil
}

// CleanupPort forcefully cleans up any process using the specified port
func (stm *SingleTunnelManager) CleanupPort(port int) error {
	// First try to stop active tunnel normally
	stm.StopActiveTunnel()
	
	// Then kill any remaining processes on the port
	cmd := exec.Command("sh", "-c", fmt.Sprintf("lsof -ti:%d", port))
	output, err := cmd.Output()
	if err != nil {
		return nil // No processes found
	}
	
	// Kill each process
	pids := string(output)
	if pids != "" {
		for _, pidStr := range splitLines(pids) {
			if pidStr != "" {
				var pid int
				fmt.Sscanf(pidStr, "%d", &pid)
				if pid > 0 {
					stm.KillProcess(pid)
				}
			}
		}
	}
	
	return nil
}

// IsConnected checks if there's an active tunnel for the specified cluster
func (stm *SingleTunnelManager) IsConnected(clusterName string) bool {
	activeTunnel, err := stm.GetActiveTunnel()
	if err != nil || activeTunnel == nil {
		return false
	}
	
	// Check if it's for the same cluster and process is alive
	if activeTunnel.ClusterName == clusterName {
		return stm.IsProcessAlive(activeTunnel.PID) && stm.IsPortListening(6443)
	}
	
	return false
}

// GetTunnelInfo returns information about the tunnel if it's for the specified cluster
func (stm *SingleTunnelManager) GetTunnelInfo(clusterName string) *ActiveTunnelState {
	activeTunnel, err := stm.GetActiveTunnel()
	if err != nil || activeTunnel == nil {
		return nil
	}
	
	// Only return if it's for the requested cluster and alive
	if activeTunnel.ClusterName == clusterName && stm.IsProcessAlive(activeTunnel.PID) {
		return activeTunnel
	}
	
	return nil
}

// StopTunnel stops the tunnel if it's for the specified cluster
func (stm *SingleTunnelManager) StopTunnel(clusterName string) error {
	stm.mu.Lock()
	defer stm.mu.Unlock()
	
	activeTunnel, err := stm.LoadActiveTunnel()
	if err != nil {
		return err
	}
	
	if activeTunnel == nil {
		return nil // No active tunnel
	}
	
	// Only stop if it's for the specified cluster
	if activeTunnel.ClusterName != clusterName {
		return fmt.Errorf("tunnel is for cluster %s, not %s", activeTunnel.ClusterName, clusterName)
	}
	
	// Kill the process
	if err := stm.KillProcess(activeTunnel.PID); err != nil {
		return err
	}
	
	// Delete state file
	return stm.DeleteActiveTunnel()
}

// Helper function to split lines
func splitLines(s string) []string {
	var lines []string
	line := ""
	for _, c := range s {
		if c == '\n' {
			if line != "" {
				lines = append(lines, line)
				line = ""
			}
		} else if c != '\r' {
			line += string(c)
		}
	}
	if line != "" {
		lines = append(lines, line)
	}
	return lines
}