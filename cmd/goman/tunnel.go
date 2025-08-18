package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// tunnelCmd represents the tunnel command group
var tunnelCmd = &cobra.Command{
	Use:   "tunnel",
	Short: "Manage SSM tunnels",
	Long:  `Manage SSM tunnel operations including status, cleanup, and diagnostics.`,
}

// tunnelStatusCmd shows tunnel status
var tunnelStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show tunnel status and diagnostics",
	Long:  `Shows detailed status of the active SSM tunnel.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		stm := GetGlobalSingleTunnelManager()
		
		fmt.Println("🔍 SSM Tunnel Status")
		fmt.Println("=" + strings.Repeat("=", 60))
		
		// Check active tunnel
		activeTunnel, err := stm.GetActiveTunnel()
		if err != nil {
			fmt.Printf("Error loading active tunnel: %v\n", err)
		}
		
		currentCluster := getCurrentCluster()
		
		fmt.Println("\n📋 Active Tunnel:")
		if activeTunnel != nil {
			status := "❌ Dead"
			if stm.IsProcessAlive(activeTunnel.PID) {
				if stm.IsPortListening(6443) {
					status = "✅ Healthy"
				} else {
					status = "⚠️  Process alive but port not listening"
				}
			}
			
			current := ""
			if activeTunnel.ClusterName == currentCluster {
				current = " (current cluster)"
			}
			
			fmt.Printf("  • Cluster: %s%s\n", activeTunnel.ClusterName, current)
			fmt.Printf("  • Status: %s\n", status)
			fmt.Printf("  • PID: %d\n", activeTunnel.PID)
			fmt.Printf("  • Instance: %s\n", activeTunnel.InstanceID)
			fmt.Printf("  • Region: %s\n", activeTunnel.Region)
			fmt.Printf("  • Port: %d -> %d\n", activeTunnel.LocalPort, activeTunnel.RemotePort)
			fmt.Printf("  • Started: %s ago\n", formatDuration(time.Since(activeTunnel.StartedAt)))
		} else {
			fmt.Println("  No active tunnel")
		}
		
		// Check for orphaned processes
		fmt.Println("\n🔎 Orphaned Processes:")
		orphanCount := checkOrphanedProcesses()
		
		// Check port status
		fmt.Println("\n🔌 Port 6443 Status:")
		checkPortStatus(6443)
		
		// Summary
		fmt.Println("\n📊 Summary:")
		trackedCount := 0
		healthyCount := 0
		if activeTunnel != nil {
			trackedCount = 1
			if stm.IsProcessAlive(activeTunnel.PID) && stm.IsPortListening(6443) {
				healthyCount = 1
			}
		}
		fmt.Printf("  • Active tunnels: %d\n", trackedCount)
		fmt.Printf("  • Healthy tunnels: %d\n", healthyCount)
		fmt.Printf("  • Orphaned processes: %d\n", orphanCount)
		
		if orphanCount > 0 {
			fmt.Println("\n⚠️  Found orphaned processes. Run 'goman tunnel cleanup' to clean them up.")
		}
		
		return nil
	},
}

// tunnelCleanupCmd cleans up tunnels
var tunnelCleanupCmd = &cobra.Command{
	Use:   "cleanup",
	Short: "Clean up the active SSM tunnel and orphaned processes",
	Long:  `Forcefully cleans up the active SSM tunnel, orphaned processes, and state files.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("🧹 Cleaning up SSM tunnels...")
		
		stm := GetGlobalSingleTunnelManager()
		
		// Stop active tunnel
		if err := stm.StopActiveTunnel(); err != nil {
			fmt.Printf("Warning: Error stopping active tunnel: %v\n", err)
		}
		
		// Clean up port 6443
		if err := stm.CleanupPort(6443); err != nil {
			fmt.Printf("Warning: Error cleaning port 6443: %v\n", err)
		}
		
		// Kill all SSM processes
		killAllSSMProcesses()
		
		fmt.Println("✅ Cleanup complete")
		return nil
	},
}

// tunnelHealthCmd checks tunnel health
var tunnelHealthCmd = &cobra.Command{
	Use:   "health [cluster-name]",
	Short: "Check health of a specific tunnel",
	Long:  `Performs a health check on a specific SSM tunnel.`,
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var clusterName string
		if len(args) > 0 {
			clusterName = args[0]
		} else {
			clusterName = getCurrentCluster()
			if clusterName == "" {
				return fmt.Errorf("no cluster specified and no current cluster set")
			}
		}
		
		stm := GetGlobalSingleTunnelManager()
		
		fmt.Printf("🏥 Checking health of tunnel for cluster: %s\n", clusterName)
		
		if stm.IsConnected(clusterName) {
			fmt.Println("✅ Tunnel is healthy")
			return nil
		}
		
		fmt.Println("❌ Tunnel is unhealthy or not found")
		fmt.Println("\nDiagnostics:")
		
		// Check if tunnel exists
		activeTunnel, _ := stm.GetActiveTunnel()
		if activeTunnel == nil {
			fmt.Println("  • No active tunnel")
		} else if activeTunnel.ClusterName != clusterName {
			fmt.Printf("  • Active tunnel is for cluster %s, not %s\n", activeTunnel.ClusterName, clusterName)
		} else if !stm.IsProcessAlive(activeTunnel.PID) {
			fmt.Println("  • Tunnel process is dead")
		}
		
		// Check port
		if !isPortOpen(6443) {
			fmt.Println("  • Port 6443 is not open")
		}
		
		// Check for processes
		checkSSMProcesses()
		
		fmt.Println("\n💡 Try running: goman tunnel cleanup && goman kube kubectl get nodes")
		
		return nil
	},
}

// Helper functions

func checkOrphanedProcesses() int {
	count := 0
	
	// Check for aws ssm processes
	cmd := exec.Command("sh", "-c", "ps aux | grep 'aws.*ssm.*start-session' | grep -v grep")
	if output, err := cmd.Output(); err == nil {
		lines := strings.Split(strings.TrimSpace(string(output)), "\n")
		for _, line := range lines {
			if line != "" {
				count++
				fmt.Printf("  • AWS SSM process: %s\n", strings.Fields(line)[1])
			}
		}
	}
	
	// Check for session-manager-plugin processes
	cmd = exec.Command("sh", "-c", "ps aux | grep 'session-manager-plugin' | grep -v grep")
	if output, err := cmd.Output(); err == nil {
		lines := strings.Split(strings.TrimSpace(string(output)), "\n")
		for _, line := range lines {
			if line != "" {
				count++
				fmt.Printf("  • Session Manager plugin: %s\n", strings.Fields(line)[1])
			}
		}
	}
	
	if count == 0 {
		fmt.Println("  None found")
	}
	
	return count
}

func checkPortStatus(port int) {
	// Check what's using the port
	cmd := exec.Command("sh", "-c", fmt.Sprintf("lsof -i:%d", port))
	if output, err := cmd.Output(); err == nil {
		lines := strings.Split(strings.TrimSpace(string(output)), "\n")
		if len(lines) > 1 { // Skip header
			fmt.Printf("  Port %d is in use by:\n", port)
			for i, line := range lines {
				if i > 0 && line != "" { // Skip header
					fields := strings.Fields(line)
					if len(fields) > 1 {
						fmt.Printf("    • Process: %s (PID: %s)\n", fields[0], fields[1])
					}
				}
			}
		} else {
			fmt.Printf("  Port %d is free\n", port)
		}
	} else {
		fmt.Printf("  Port %d is free\n", port)
	}
}

func checkSSMProcesses() {
	// Check for SSM processes
	cmd := exec.Command("sh", "-c", "ps aux | grep -E '(aws.*ssm|session-manager)' | grep -v grep")
	if output, err := cmd.Output(); err == nil {
		lines := strings.Split(strings.TrimSpace(string(output)), "\n")
		if len(lines) > 0 && lines[0] != "" {
			fmt.Println("  • Found SSM processes:")
			for _, line := range lines {
				if line != "" {
					fields := strings.Fields(line)
					if len(fields) > 10 {
						fmt.Printf("    PID %s: %s\n", fields[1], strings.Join(fields[10:], " "))
					}
				}
			}
		} else {
			fmt.Println("  • No SSM processes found")
		}
	} else {
		fmt.Println("  • No SSM processes found")
	}
}

func killAllSSMProcesses() {
	// Kill aws ssm processes
	exec.Command("sh", "-c", "pkill -f 'aws.*ssm.*start-session'").Run()
	
	// Kill session-manager-plugin
	exec.Command("sh", "-c", "pkill -f 'session-manager-plugin'").Run()
	
	fmt.Println("  • Killed all SSM processes")
}

func cleanupStateFiles() {
	homeDir, _ := os.UserHomeDir()
	stateDir := fmt.Sprintf("%s/.goman/tunnels", homeDir)
	
	// Remove all state files
	exec.Command("sh", "-c", fmt.Sprintf("rm -f %s/*.json", stateDir)).Run()
	
	fmt.Println("  • Cleaned up state files")
}

func isPortOpen(port int) bool {
	cmd := exec.Command("sh", "-c", fmt.Sprintf("nc -z localhost %d", port))
	return cmd.Run() == nil
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.0f seconds", d.Seconds())
	} else if d < time.Hour {
		return fmt.Sprintf("%.0f minutes", d.Minutes())
	} else {
		return fmt.Sprintf("%.1f hours", d.Hours())
	}
}

func init() {
	tunnelCmd.AddCommand(tunnelStatusCmd)
	tunnelCmd.AddCommand(tunnelCleanupCmd)
	tunnelCmd.AddCommand(tunnelHealthCmd)
}