package models

import (
	"fmt"
	"strings"
	"time"
)

// ClusterResource represents the desired state of a K3s cluster (like a K8s CRD)
type ClusterResource struct {
	// Metadata
	Name              string            `json:"name"`
	Namespace         string            `json:"namespace"` // AWS profile
	UID               string            `json:"uid"`
	ClusterID         string            `json:"clusterId"` // The actual cluster ID
	ResourceVersion   string            `json:"resourceVersion"`
	Generation        int               `json:"generation"`
	CreationTimestamp time.Time         `json:"creationTimestamp"`
	DeletionTimestamp *time.Time        `json:"deletionTimestamp,omitempty"`
	Labels            map[string]string `json:"labels,omitempty"`
	Annotations       map[string]string `json:"annotations,omitempty"`

	// Spec - Desired State
	Spec ClusterSpec `json:"spec"`

	// Status - Actual State
	Status ClusterResourceStatus `json:"status"`
}

// ClusterSpec defines the desired state of a cluster
type ClusterSpec struct {
	Provider     string            `json:"provider"`
	Region       string            `json:"region"`
	InstanceType string            `json:"instanceType"`
	MasterCount  int               `json:"masterCount"` // Number of master nodes (1 for dev, 3 for HA)
	Mode         string            `json:"mode"`        // "dev" or "ha"
	K3sVersion   string            `json:"k3sVersion"`
	Network      NetworkConfig     `json:"network"`
	Tags         map[string]string `json:"tags,omitempty"`
	DesiredState string            `json:"desiredState,omitempty"` // "running" or "stopped"
	NodePools    []NodePool        `json:"nodePools,omitempty"`    // Worker node pools
}

// NodePool defines a group of worker nodes with similar configuration
type NodePool struct {
	Name         string            `json:"name"`
	Count        int               `json:"count"`
	InstanceType string            `json:"instanceType"`
	Labels       map[string]string `json:"labels,omitempty"`
	Taints       []Taint           `json:"taints,omitempty"`
}

// Taint represents a Kubernetes taint on nodes
type Taint struct {
	Key    string `json:"key"`
	Value  string `json:"value"`
	Effect string `json:"effect"` // NoSchedule, PreferNoSchedule, NoExecute
}

// ClusterResourceStatus represents the observed state of a cluster
type ClusterResourceStatus struct {
	Phase              string      `json:"phase" yaml:"phase"`
	ObservedGeneration int         `json:"observedGeneration" yaml:"observedGeneration"`
	Conditions         []Condition `json:"conditions,omitempty" yaml:"conditions,omitempty"`
	LastReconcileTime  *time.Time  `json:"lastReconcileTime,omitempty" yaml:"lastReconcileTime,omitempty"`

	// Actual infrastructure state
	ClusterID      string           `json:"clusterId,omitempty" yaml:"clusterId,omitempty"`
	VpcID          string           `json:"vpcId,omitempty" yaml:"vpcId,omitempty"`
	SubnetIDs      []string         `json:"subnetIds,omitempty" yaml:"subnetIds,omitempty"`
	SecurityGroups []string         `json:"securityGroups,omitempty" yaml:"securityGroups,omitempty"`
	Instances      []InstanceStatus `json:"instances,omitempty" yaml:"instances,omitempty"`
	APIEndpoint    string           `json:"apiEndpoint,omitempty" yaml:"apiEndpoint,omitempty"`
	
	// K3s cluster status (will be populated after installation)
	K3sServerURL       string `json:"k3sServerUrl,omitempty" yaml:"k3sServerUrl,omitempty"`       // K3s API server URL
	KubeConfig         string `json:"kubeConfig,omitempty" yaml:"kubeConfig,omitempty"`         // Base64 encoded kubeconfig
	K3sServerToken     string `json:"k3sServerToken,omitempty" yaml:"k3sServerToken,omitempty"`     // Token for joining additional masters
	K3sAgentToken      string `json:"k3sAgentToken,omitempty" yaml:"k3sAgentToken,omitempty"`      // Token for joining worker nodes
	InternalDNS        string `json:"internalDns,omitempty" yaml:"internalDns,omitempty"`        // Internal DNS name for API server (HA mode)
	
	// Connection information for SSM access
	MasterInstanceIDs       []string `json:"masterInstanceIds,omitempty" yaml:"masterInstanceIds,omitempty"`       // Instance IDs of master nodes
	PreferredMasterInstance string   `json:"preferredMasterInstance,omitempty" yaml:"preferredMasterInstance,omitempty"` // Preferred master for connections

	// Progress tracking
	Message         string               `json:"message,omitempty" yaml:"message,omitempty"`
	Reason          string               `json:"reason,omitempty" yaml:"reason,omitempty"`
	ProgressMetrics *ProgressMetrics     `json:"progressMetrics,omitempty" yaml:"progressMetrics,omitempty"`
	
	// Pending operations tracking (for non-blocking execution)
	PendingOperations *PendingOperations `json:"pendingOperations,omitempty" yaml:"pendingOperations,omitempty"`
}

// InstanceStatus represents the status of an EC2 instance
type InstanceStatus struct {
	InstanceID string    `json:"instanceId" yaml:"instanceId"`
	Name       string    `json:"name" yaml:"name"`
	Role       string    `json:"role" yaml:"role"` // master or worker
	State      string    `json:"state" yaml:"state"`
	PrivateIP  string    `json:"privateIp,omitempty" yaml:"privateIp,omitempty"`
	PublicIP   string    `json:"publicIp,omitempty" yaml:"publicIp,omitempty"`
	LaunchTime time.Time `json:"launchTime" yaml:"launchTime"`
	
	// K3s installation status
	K3sInstalled       bool      `json:"k3sInstalled" yaml:"k3sInstalled"`
	K3sVersion         string    `json:"k3sVersion,omitempty" yaml:"k3sVersion,omitempty"`
	K3sInstallTime     *time.Time `json:"k3sInstallTime,omitempty" yaml:"k3sInstallTime,omitempty"`
	K3sInstallError    string    `json:"k3sInstallError,omitempty" yaml:"k3sInstallError,omitempty"`
	
	// K3s configuration status
	K3sRunning         bool      `json:"k3sRunning" yaml:"k3sRunning"`
	K3sConfigTime      *time.Time `json:"k3sConfigTime,omitempty" yaml:"k3sConfigTime,omitempty"`
	K3sConfigError     string    `json:"k3sConfigError,omitempty" yaml:"k3sConfigError,omitempty"`
	
	// Instance lifecycle tracking
	LastStartTime      *time.Time `json:"lastStartTime,omitempty" yaml:"lastStartTime,omitempty"`      // When instance was last started
}

// ProgressMetrics tracks detailed progress through reconciliation operations
type ProgressMetrics struct {
	CurrentOperation string                `json:"currentOperation,omitempty"` // e.g., "Creation", "Deletion", "Update", "Scaling"
	Steps            []StepProgress        `json:"steps,omitempty"`
	StartTime        *time.Time            `json:"startTime,omitempty"`
	LastUpdateTime   *time.Time            `json:"lastUpdateTime,omitempty"`
	TotalSteps       int                   `json:"totalSteps,omitempty"`
	CompletedSteps   int                   `json:"completedSteps,omitempty"`
}

// StepProgress tracks a single step in the reconciliation process
type StepProgress struct {
	Name          string         `json:"name"`          // e.g., "Provisioning Infrastructure", "Installing K3s", "Configuring Cluster"
	Description   string         `json:"description,omitempty"` // Human-readable description
	Status        string         `json:"status"`        // "Pending", "InProgress", "Done", "Failed", "Skipped"
	StartTime     *time.Time     `json:"startTime,omitempty"`
	EndTime       *time.Time     `json:"endTime,omitempty"`
	Checks        []CheckProgress `json:"checks,omitempty"`
	ErrorMessage  string         `json:"errorMessage,omitempty"`
	SkipReason    string         `json:"skipReason,omitempty"`
}

// CheckProgress tracks individual checks within a step
type CheckProgress struct {
	Name         string     `json:"name"`         // e.g., "Create security group", "Wait for instances", "Extract K3s token"
	Description  string     `json:"description,omitempty"` // Human-readable description
	Status       string     `json:"status"`       // "Pending", "InProgress", "Done", "Failed", "Skipped"
	StartTime    *time.Time `json:"startTime,omitempty"`
	EndTime      *time.Time `json:"endTime,omitempty"`
	ErrorMessage string     `json:"errorMessage,omitempty"`
	Details      string     `json:"details,omitempty"`     // Additional context (e.g., instance IDs, token status)
	FailureCount int        `json:"failureCount,omitempty"` // Number of times this check has failed
	RetryAfter   *time.Time `json:"retryAfter,omitempty"`   // When to retry this check after failure
}

// Condition represents a condition of a resource
type Condition struct {
	Type               string    `json:"type"`
	Status             string    `json:"status"` // True, False, Unknown
	LastTransitionTime time.Time `json:"lastTransitionTime"`
	Reason             string    `json:"reason,omitempty"`
	Message            string    `json:"message,omitempty"`
}

// PendingOperations tracks long-running operations that don't block reconciliation
type PendingOperations struct {
	Commands             map[string]*PendingCommand    `json:"commands,omitempty"`             // CommandID -> Command details
	InstanceStateChanges map[string]string             `json:"instanceStateChanges,omitempty"` // InstanceID -> Expected state
	BackgroundProcesses  map[string]*BackgroundProcess `json:"backgroundProcesses,omitempty"`  // ProcessKey -> Background process details
}

// PendingCommand represents a command that was started but not yet completed
type PendingCommand struct {
	CommandID string        `json:"commandId"`
	StartedAt time.Time     `json:"startedAt"`
	Purpose   string        `json:"purpose"`     // Description of what this command does
	Timeout   time.Duration `json:"timeout"`     // How long to wait before considering it failed
	StepName  string        `json:"stepName"`    // Which step this command belongs to
	CheckName string        `json:"checkName"`   // Which check this command belongs to
}

// BackgroundProcess represents a long-running process that executes in the background on an instance
type BackgroundProcess struct {
	ProcessKey   string        `json:"processKey"`   // Unique identifier for this background process
	InstanceID   string        `json:"instanceId"`   // Instance where the process is running
	StartedAt    time.Time     `json:"startedAt"`    // When the process was started
	Purpose      string        `json:"purpose"`      // Description of what this process does
	Timeout      time.Duration `json:"timeout"`      // How long to wait before considering it failed
	StepName     string        `json:"stepName"`     // Which step this process belongs to
	CheckName    string        `json:"checkName"`    // Which check this process belongs to
	
	// Process management details
	PIDFile      string `json:"pidFile"`      // Path to PID file on the instance
	LogFile      string `json:"logFile"`      // Path to log file on the instance
	ScriptPath   string `json:"scriptPath"`   // Path to the script being executed
	WorkingDir   string `json:"workingDir"`   // Working directory for the process
	
	// Status tracking
	Status         string        `json:"status"`         // "running", "completed", "failed", "timeout"
	LastChecked    time.Time     `json:"lastChecked"`    // Last time we checked process status
	LastCheckedAt  time.Time     `json:"lastCheckedAt"`  // Alias for compatibility
	CheckInterval  time.Duration `json:"checkInterval"`  // How often to check process status
	ExitCode       *int          `json:"exitCode,omitempty"`     // Process exit code (if completed)
	ErrorMessage   string        `json:"errorMessage,omitempty"` // Error message if failed
	Output         string        `json:"output,omitempty"`       // Process output (if completed)
	LogContent     string        `json:"logContent,omitempty"`   // Log file content (if completed)
	Script         string        `json:"script,omitempty"`       // The script being executed
	StartCommandID string        `json:"startCommandId,omitempty"` // SSM command ID used to start process
}

// GetCommand retrieves a pending command by key
func (p *PendingOperations) GetCommand(key string) *PendingCommand {
	if p == nil || p.Commands == nil {
		return nil
	}
	return p.Commands[key]
}

// GetBackgroundProcess retrieves a background process by key
func (p *PendingOperations) GetBackgroundProcess(key string) *BackgroundProcess {
	if p == nil || p.BackgroundProcesses == nil {
		return nil
	}
	return p.BackgroundProcesses[key]
}

// AddBackgroundProcess adds a new background process
func (p *PendingOperations) AddBackgroundProcess(key string, instanceID, purpose, stepName, checkName string, timeout time.Duration) *BackgroundProcess {
	if p == nil {
		return nil
	}
	if p.BackgroundProcesses == nil {
		p.BackgroundProcesses = make(map[string]*BackgroundProcess)
	}
	
	process := &BackgroundProcess{
		ProcessKey: key,
		InstanceID: instanceID,
		StartedAt:  time.Now(),
		Purpose:    purpose,
		Timeout:    timeout,
		StepName:   stepName,
		CheckName:  checkName,
		Status:     "running",
		PIDFile:    fmt.Sprintf("/tmp/goman_%s.pid", key),
		LogFile:    fmt.Sprintf("/tmp/goman_%s.log", key),
		WorkingDir: "/tmp",
	}
	
	p.BackgroundProcesses[key] = process
	return process
}

// RemoveBackgroundProcess removes a background process
func (p *PendingOperations) RemoveBackgroundProcess(key string) {
	if p == nil || p.BackgroundProcesses == nil {
		return
	}
	delete(p.BackgroundProcesses, key)
}

// IsTimedOut checks if a background process has timed out
func (bp *BackgroundProcess) IsTimedOut() bool {
	if bp == nil {
		return false
	}
	return time.Since(bp.StartedAt) > bp.Timeout
}

// AddCommand adds a new pending command
func (p *PendingOperations) AddCommand(key, commandID, purpose string, timeout time.Duration, stepName, checkName string) {
	if p == nil {
		return // Cannot add to nil PendingOperations
	}
	if p.Commands == nil {
		p.Commands = make(map[string]*PendingCommand)
	}
	p.Commands[key] = &PendingCommand{
		CommandID: commandID,
		StartedAt: time.Now(),
		Purpose:   purpose,
		Timeout:   timeout,
		StepName:  stepName,
		CheckName: checkName,
	}
}

// RemoveCommand removes a pending command by key
func (p *PendingOperations) RemoveCommand(key string) {
	if p == nil || p.Commands == nil {
		return
	}
	delete(p.Commands, key)
}

// InitializeProgress initializes progress tracking for an operation
func (r *ClusterResource) InitializeProgress(operation string) {
	now := time.Now()
	r.Status.ProgressMetrics = &ProgressMetrics{
		CurrentOperation: operation,
		StartTime:        &now,
		LastUpdateTime:   &now,
		Steps:            []StepProgress{},
		TotalSteps:       0,
		CompletedSteps:   0,
	}
}

// InitializeClusterLifecycleProgress initializes progress tracking with all cluster lifecycle steps
func (r *ClusterResource) InitializeClusterLifecycleProgress(operation string) {
	now := time.Now()
	r.Status.ProgressMetrics = &ProgressMetrics{
		CurrentOperation: operation,
		StartTime:        &now,
		LastUpdateTime:   &now,
		Steps:            []StepProgress{},
		TotalSteps:       0,
		CompletedSteps:   0,
	}
	
	r.addAllLifecycleSteps()
}

// EnsureAllLifecycleSteps ensures all lifecycle steps are present in correct order
func (r *ClusterResource) EnsureAllLifecycleSteps() {
	if r.Status.ProgressMetrics == nil {
		return
	}
	
	// Create a map of existing steps for easy lookup
	existingSteps := make(map[string]StepProgress)
	for _, step := range r.Status.ProgressMetrics.Steps {
		existingSteps[step.Name] = step
	}
	
	// Create a new ordered steps slice
	orderedSteps := []StepProgress{}
	
	// 1. Provisioning step
	if step, exists := existingSteps["Provisioning"]; exists {
		orderedSteps = append(orderedSteps, step)
	} else {
		// Create missing Provisioning step
		if r.Status.Phase != ClusterPhasePending && r.Status.Phase != ClusterPhaseProvisioning {
			orderedSteps = append(orderedSteps, r.createCompletedStep("Provisioning", "Create and configure AWS infrastructure", []string{
				"Query existing instances", "Create missing instances", "Wait for instances to be running",
				"Assign IP addresses", "Verify instance health",
			}))
		} else {
			orderedSteps = append(orderedSteps, r.createPendingStep("Provisioning", "Create and configure AWS infrastructure", []string{
				"Query existing instances", "Create missing instances", "Wait for instances to be running",
				"Assign IP addresses", "Verify instance health",
			}))
		}
	}
	
	// 2. Installing step
	if step, exists := existingSteps["Installing"]; exists {
		orderedSteps = append(orderedSteps, step)
	} else {
		// Create missing Installing step
		if r.Status.Phase == ClusterPhaseConfiguring || r.Status.Phase == ClusterPhaseRunning {
			orderedSteps = append(orderedSteps, r.createCompletedStep("Installing", "Install K3s on cluster instances", []string{
				"Download K3s binary", "Install K3s service", "Verify installation",
			}))
		} else if r.Status.Phase != ClusterPhasePending && r.Status.Phase != ClusterPhaseProvisioning {
			orderedSteps = append(orderedSteps, r.createPendingStep("Installing", "Install K3s on cluster instances", []string{
				"Download K3s binary", "Install K3s service", "Verify installation",
			}))
		}
	}
	
	// 3. Configuring step
	if step, exists := existingSteps["Configuring"]; exists {
		orderedSteps = append(orderedSteps, step)
	} else {
		orderedSteps = append(orderedSteps, r.createPendingStep("Configuring", "Configure and start K3s cluster", []string{
			"Check K3s installation status", "Start first master with cluster-init",
			"Extract server token from first master", "Save token to S3 for joining masters",
			"Start additional masters to join cluster", "Verify all masters are running",
			"Extract and save kubeconfig",
		}))
	}
	
	// Replace the steps with the ordered ones
	r.Status.ProgressMetrics.Steps = orderedSteps
	r.Status.ProgressMetrics.TotalSteps = len(orderedSteps)
	
	// Update completed steps count
	completed := 0
	for _, step := range orderedSteps {
		if step.Status == "Done" {
			completed++
		}
	}
	r.Status.ProgressMetrics.CompletedSteps = completed
}

// addAllLifecycleSteps adds all lifecycle steps to progress tracking
func (r *ClusterResource) addAllLifecycleSteps() {
	// Add all lifecycle steps upfront so users can see the complete journey
	r.AddStep("Provisioning", "Create and configure AWS infrastructure", []string{
		"Query existing instances",
		"Create missing instances", 
		"Wait for instances to be running",
		"Assign IP addresses",
		"Verify instance health",
	})
	
	r.AddStep("Installing", "Install K3s on cluster instances", []string{
		"Download K3s binary",
		"Install K3s service",
		"Verify installation",
	})
	
	r.AddStep("Configuring", "Configure and start K3s cluster", []string{
		"Check K3s installation status",
		"Start first master with cluster-init",
		"Extract server token from first master",
		"Save token to S3 for joining masters",
		"Start additional masters to join cluster",
		"Verify all masters are running",
		"Extract and save kubeconfig",
	})
}

// createCompletedStep creates a completed step with all checks done
func (r *ClusterResource) createCompletedStep(name, description string, checks []string) StepProgress {
	now := time.Now()
	
	// Create all checks as completed
	checkProgresses := make([]CheckProgress, len(checks))
	for i, checkName := range checks {
		checkProgresses[i] = CheckProgress{
			Name:         checkName,
			Status:       "Done",
			StartTime:    &now,
			EndTime:      &now,
			Details:      "Completed in previous phase",
			FailureCount: 0,
		}
	}
	
	return StepProgress{
		Name:        name,
		Description: description,
		Status:      "Done",
		StartTime:   &now,
		EndTime:     &now,
		Checks:      checkProgresses,
	}
}

// createPendingStep creates a pending step with all checks pending
func (r *ClusterResource) createPendingStep(name, description string, checks []string) StepProgress {
	// Create all checks as pending
	checkProgresses := make([]CheckProgress, len(checks))
	for i, checkName := range checks {
		checkProgresses[i] = CheckProgress{
			Name:         checkName,
			Status:       "Pending",
			FailureCount: 0,
		}
	}
	
	return StepProgress{
		Name:        name,
		Description: description,
		Status:      "Pending",
		Checks:      checkProgresses,
	}
}

// addCompletedStep adds a step and marks it as completed with all checks done
func (r *ClusterResource) addCompletedStep(name, description string, checks []string) {
	r.AddStep(name, description, checks)
	
	// Mark the step as completed
	for i := range r.Status.ProgressMetrics.Steps {
		if r.Status.ProgressMetrics.Steps[i].Name == name {
			now := time.Now()
			r.Status.ProgressMetrics.Steps[i].Status = "Done"
			r.Status.ProgressMetrics.Steps[i].StartTime = &now
			r.Status.ProgressMetrics.Steps[i].EndTime = &now
			
			// Mark all checks as done
			for j := range r.Status.ProgressMetrics.Steps[i].Checks {
				r.Status.ProgressMetrics.Steps[i].Checks[j].Status = "Done"
				r.Status.ProgressMetrics.Steps[i].Checks[j].StartTime = &now
				r.Status.ProgressMetrics.Steps[i].Checks[j].EndTime = &now
				r.Status.ProgressMetrics.Steps[i].Checks[j].Details = "Completed in previous phase"
			}
			break
		}
	}
}

// AddStep adds a new step to progress tracking
func (r *ClusterResource) AddStep(name, description string, checks []string) {
	if r.Status.ProgressMetrics == nil {
		r.InitializeProgress("Creation")
	}
	
	checkProgresses := make([]CheckProgress, len(checks))
	for i, checkName := range checks {
		checkProgresses[i] = CheckProgress{
			Name:   checkName,
			Status: "Pending",
		}
	}
	
	step := StepProgress{
		Name:        name,
		Description: description,
		Status:      "Pending",
		Checks:      checkProgresses,
	}
	
	r.Status.ProgressMetrics.Steps = append(r.Status.ProgressMetrics.Steps, step)
	r.Status.ProgressMetrics.TotalSteps = len(r.Status.ProgressMetrics.Steps)
	now := time.Now()
	r.Status.ProgressMetrics.LastUpdateTime = &now
}

// StartStep marks a step as in progress
func (r *ClusterResource) StartStep(stepName string) {
	if r.Status.ProgressMetrics == nil {
		return
	}
	
	for i := range r.Status.ProgressMetrics.Steps {
		if r.Status.ProgressMetrics.Steps[i].Name == stepName {
			now := time.Now()
			r.Status.ProgressMetrics.Steps[i].Status = "InProgress"
			r.Status.ProgressMetrics.Steps[i].StartTime = &now
			r.Status.ProgressMetrics.LastUpdateTime = &now
			break
		}
	}
}

// CompleteStep marks a step as done
func (r *ClusterResource) CompleteStep(stepName string) {
	if r.Status.ProgressMetrics == nil {
		return
	}
	
	for i := range r.Status.ProgressMetrics.Steps {
		if r.Status.ProgressMetrics.Steps[i].Name == stepName {
			now := time.Now()
			r.Status.ProgressMetrics.Steps[i].Status = "Done"
			r.Status.ProgressMetrics.Steps[i].EndTime = &now
			r.Status.ProgressMetrics.LastUpdateTime = &now
			
			// Update completed steps count
			completed := 0
			for _, step := range r.Status.ProgressMetrics.Steps {
				if step.Status == "Done" {
					completed++
				}
			}
			r.Status.ProgressMetrics.CompletedSteps = completed
			break
		}
	}
}

// FailStep marks a step as failed
func (r *ClusterResource) FailStep(stepName, errorMsg string) {
	if r.Status.ProgressMetrics == nil {
		return
	}
	
	for i := range r.Status.ProgressMetrics.Steps {
		if r.Status.ProgressMetrics.Steps[i].Name == stepName {
			now := time.Now()
			r.Status.ProgressMetrics.Steps[i].Status = "Failed"
			r.Status.ProgressMetrics.Steps[i].EndTime = &now
			r.Status.ProgressMetrics.Steps[i].ErrorMessage = errorMsg
			r.Status.ProgressMetrics.LastUpdateTime = &now
			break
		}
	}
}

// ShouldRetryCheck checks if a check should be retried based on timeout, failure count, and retry delay
func (r *ClusterResource) ShouldRetryCheck(stepName, checkName string, timeoutDuration time.Duration) bool {
	if r.Status.ProgressMetrics == nil {
		return true // No progress tracking, allow retry
	}
	
	for i := range r.Status.ProgressMetrics.Steps {
		if r.Status.ProgressMetrics.Steps[i].Name == stepName {
			for j := range r.Status.ProgressMetrics.Steps[i].Checks {
				if r.Status.ProgressMetrics.Steps[i].Checks[j].Name == checkName {
					check := &r.Status.ProgressMetrics.Steps[i].Checks[j]
					
					// If check has failed too many times (3+ failures), don't retry
					if check.FailureCount >= 3 {
						return false
					}
					
					// If check is failed and has a retry delay, check if enough time has passed
					if check.Status == "Failed" && check.RetryAfter != nil {
						return time.Now().After(*check.RetryAfter)
					}
					
					// If check is not in progress, allow retry
					if check.Status != "InProgress" {
						return true
					}
					
					// If no start time, allow retry
					if check.StartTime == nil {
						return true
					}
					
					// Check if timeout has elapsed for in-progress checks
					elapsed := time.Since(*check.StartTime)
					return elapsed >= timeoutDuration
				}
			}
		}
	}
	
	// Check not found, allow retry
	return true
}

// UpdateCheck updates the status of a specific check within a step
func (r *ClusterResource) UpdateCheck(stepName, checkName, status string, details string, errorMsg ...string) {
	if r.Status.ProgressMetrics == nil {
		return
	}
	
	for i := range r.Status.ProgressMetrics.Steps {
		if r.Status.ProgressMetrics.Steps[i].Name == stepName {
			for j := range r.Status.ProgressMetrics.Steps[i].Checks {
				if r.Status.ProgressMetrics.Steps[i].Checks[j].Name == checkName {
					now := time.Now()
					check := &r.Status.ProgressMetrics.Steps[i].Checks[j]
					
					if status == "InProgress" && check.StartTime == nil {
						check.StartTime = &now
					}
					if (status == "Done" || status == "Failed") && check.EndTime == nil {
						check.EndTime = &now
					}
					
					check.Status = status
					check.Details = details
					if len(errorMsg) > 0 {
						check.ErrorMessage = errorMsg[0]
					}
					
					r.Status.ProgressMetrics.LastUpdateTime = &now
					return
				}
			}
		}
	}
}

// FailCheckWithRetry marks a check as failed and sets up retry logic
func (r *ClusterResource) FailCheckWithRetry(stepName, checkName, errorMsg string, retryDelay time.Duration) {
	if r.Status.ProgressMetrics == nil {
		return
	}
	
	for i := range r.Status.ProgressMetrics.Steps {
		if r.Status.ProgressMetrics.Steps[i].Name == stepName {
			for j := range r.Status.ProgressMetrics.Steps[i].Checks {
				if r.Status.ProgressMetrics.Steps[i].Checks[j].Name == checkName {
					now := time.Now()
					check := &r.Status.ProgressMetrics.Steps[i].Checks[j]
					
					// Increment failure count
					check.FailureCount++
					
					// Set status and timing
					check.Status = "Failed"
					check.ErrorMessage = errorMsg
					check.EndTime = &now
					
					// If we haven't exceeded max failures, set retry time
					if check.FailureCount < 3 {
						retryTime := now.Add(retryDelay)
						check.RetryAfter = &retryTime
						check.Details = fmt.Sprintf("Attempt %d/3 - will retry in %v", check.FailureCount, retryDelay)
					} else {
						// Max failures reached, permanently failed
						check.Details = fmt.Sprintf("Max retries (3) exceeded - check permanently failed")
						check.RetryAfter = nil
					}
					
					r.Status.ProgressMetrics.LastUpdateTime = &now
					return
				}
			}
		}
	}
}

// HasPermanentFailures checks if any checks in a step have permanently failed (3+ failures)
func (r *ClusterResource) HasPermanentFailures(stepName string) bool {
	if r.Status.ProgressMetrics == nil {
		return false
	}
	
	for i := range r.Status.ProgressMetrics.Steps {
		if r.Status.ProgressMetrics.Steps[i].Name == stepName {
			for _, check := range r.Status.ProgressMetrics.Steps[i].Checks {
				if check.FailureCount >= 3 {
					return true
				}
			}
		}
	}
	return false
}

// PrintProgress prints the current progress in a readable format
func (r *ClusterResource) PrintProgress() string {
	if r.Status.ProgressMetrics == nil {
		return "No progress tracking available"
	}
	
	var output strings.Builder
	output.WriteString(fmt.Sprintf("%s (%d/%d steps completed):\n", 
		r.Status.ProgressMetrics.CurrentOperation,
		r.Status.ProgressMetrics.CompletedSteps,
		r.Status.ProgressMetrics.TotalSteps))
	
	for _, step := range r.Status.ProgressMetrics.Steps {
		stepStatus := step.Status
		if step.ErrorMessage != "" {
			stepStatus += fmt.Sprintf(" (%s)", step.ErrorMessage)
		}
		
		output.WriteString(fmt.Sprintf("- Step: %s [%s]\n", step.Name, stepStatus))
		if step.Description != "" {
			output.WriteString(fmt.Sprintf("  %s\n", step.Description))
		}
		
		for _, check := range step.Checks {
			checkStatus := check.Status
			
			// Add failure count if there have been failures
			if check.FailureCount > 0 {
				checkStatus += fmt.Sprintf(" (Attempt %d/3)", check.FailureCount)
			}
			
			if check.ErrorMessage != "" {
				checkStatus += fmt.Sprintf(" (Error: %s)", check.ErrorMessage)
			}
			if check.Details != "" {
				checkStatus += fmt.Sprintf(" - %s", check.Details)
			}
			
			// Add retry timing info if applicable
			if check.Status == "Failed" && check.RetryAfter != nil && time.Now().Before(*check.RetryAfter) {
				timeUntilRetry := time.Until(*check.RetryAfter)
				checkStatus += fmt.Sprintf(" [Retry in %v]", timeUntilRetry.Truncate(time.Second))
			}
			
			output.WriteString(fmt.Sprintf("    - %s: %s\n", check.Name, checkStatus))
		}
	}
	
	return output.String()
}

// Phases for cluster lifecycle
const (
	ClusterPhasePending      = "Pending"
	ClusterPhaseProvisioning = "Provisioning"
	ClusterPhaseInstalling   = "Installing"   // K3s binary installation
	ClusterPhaseConfiguring  = "Configuring"   // K3s server configuration and startup
	ClusterPhaseRunning      = "Running"
	ClusterPhaseUpdating     = "Updating"
	ClusterPhaseTerminating  = "Terminating"
	ClusterPhaseFailed       = "Failed"
	ClusterPhaseDeleting     = "Deleting"
	ClusterPhaseStopped      = "Stopped"
	ClusterPhaseStopping     = "Stopping"
	ClusterPhaseStarting     = "Starting"
)

// Condition types
const (
	ConditionReady       = "Ready"
	ConditionProgressing = "Progressing"
	ConditionDegraded    = "Degraded"
	ConditionAvailable   = "Available"
)

// ReconcileResult represents the result of a reconciliation
type ReconcileResult struct {
	Requeue      bool          // Should reconcile again
	RequeueAfter time.Duration // Wait before reconciling again
}

// Event types for recording
type EventType string

const (
	EventTypeNormal  EventType = "Normal"
	EventTypeWarning EventType = "Warning"
)

// Event represents a cluster event
type Event struct {
	Type      EventType `json:"type"`
	Reason    string    `json:"reason"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
	Source    string    `json:"source"`
}

// AddPendingCommand adds a pending command to track
func (r *ClusterResource) AddPendingCommand(commandID, purpose, stepName, checkName string, timeout time.Duration) {
	if r.Status.PendingOperations == nil {
		r.Status.PendingOperations = &PendingOperations{
			Commands: make(map[string]*PendingCommand),
			InstanceStateChanges: make(map[string]string),
		}
	}
	
	r.Status.PendingOperations.Commands[commandID] = &PendingCommand{
		CommandID: commandID,
		StartedAt: time.Now(),
		Purpose:   purpose,
		Timeout:   timeout,
		StepName:  stepName,
		CheckName: checkName,
	}
}

// RemovePendingCommand removes a completed command from tracking
func (r *ClusterResource) RemovePendingCommand(commandID string) {
	if r.Status.PendingOperations != nil && r.Status.PendingOperations.Commands != nil {
		delete(r.Status.PendingOperations.Commands, commandID)
		
		// Clean up empty maps
		if len(r.Status.PendingOperations.Commands) == 0 && 
		   len(r.Status.PendingOperations.InstanceStateChanges) == 0 {
			r.Status.PendingOperations = nil
		}
	}
}

// GetPendingCommands returns all pending commands
func (r *ClusterResource) GetPendingCommands() map[string]*PendingCommand {
	if r.Status.PendingOperations == nil || r.Status.PendingOperations.Commands == nil {
		return make(map[string]*PendingCommand)
	}
	return r.Status.PendingOperations.Commands
}

// AddPendingInstanceStateChange tracks expected instance state changes
func (r *ClusterResource) AddPendingInstanceStateChange(instanceID, expectedState string) {
	if r.Status.PendingOperations == nil {
		r.Status.PendingOperations = &PendingOperations{
			Commands: make(map[string]*PendingCommand),
			InstanceStateChanges: make(map[string]string),
		}
	}
	
	r.Status.PendingOperations.InstanceStateChanges[instanceID] = expectedState
}

// RemovePendingInstanceStateChange removes completed state change tracking
func (r *ClusterResource) RemovePendingInstanceStateChange(instanceID string) {
	if r.Status.PendingOperations != nil && r.Status.PendingOperations.InstanceStateChanges != nil {
		delete(r.Status.PendingOperations.InstanceStateChanges, instanceID)
		
		// Clean up empty maps
		if len(r.Status.PendingOperations.Commands) == 0 && 
		   len(r.Status.PendingOperations.InstanceStateChanges) == 0 {
			r.Status.PendingOperations = nil
		}
	}
}

// HasTimedOutCommands checks for commands that have exceeded their timeout
func (r *ClusterResource) HasTimedOutCommands() []string {
	var timedOut []string
	now := time.Now()
	
	for commandID, cmd := range r.GetPendingCommands() {
		if now.Sub(cmd.StartedAt) > cmd.Timeout {
			timedOut = append(timedOut, commandID)
		}
	}
	
	return timedOut
}
