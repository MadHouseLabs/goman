package models

import (
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
}

// ClusterResourceStatus represents the observed state of a cluster
type ClusterResourceStatus struct {
	Phase              string      `json:"phase"`
	ObservedGeneration int         `json:"observedGeneration"`
	Conditions         []Condition `json:"conditions,omitempty"`
	LastReconcileTime  *time.Time  `json:"lastReconcileTime,omitempty"`

	// Actual infrastructure state
	ClusterID      string           `json:"clusterId,omitempty"`
	VpcID          string           `json:"vpcId,omitempty"`
	SubnetIDs      []string         `json:"subnetIds,omitempty"`
	SecurityGroups []string         `json:"securityGroups,omitempty"`
	Instances      []InstanceStatus `json:"instances,omitempty"`
	APIEndpoint    string           `json:"apiEndpoint,omitempty"`
	
	// K3s cluster status (will be populated after installation)
	K3sServerURL       string `json:"k3sServerUrl,omitempty"`       // K3s API server URL
	KubeConfig         string `json:"kubeConfig,omitempty"`         // Base64 encoded kubeconfig
	K3sServerToken     string `json:"k3sServerToken,omitempty"`     // Token for joining additional masters
	K3sAgentToken      string `json:"k3sAgentToken,omitempty"`      // Token for joining worker nodes
	InternalDNS        string `json:"internalDns,omitempty"`        // Internal DNS name for API server (HA mode)

	// Progress tracking
	Message string `json:"message,omitempty"`
	Reason  string `json:"reason,omitempty"`
}

// InstanceStatus represents the status of an EC2 instance
type InstanceStatus struct {
	InstanceID string    `json:"instanceId"`
	Name       string    `json:"name"`
	Role       string    `json:"role"` // master or worker
	State      string    `json:"state"`
	PrivateIP  string    `json:"privateIp,omitempty"`
	PublicIP   string    `json:"publicIp,omitempty"`
	LaunchTime time.Time `json:"launchTime"`
	
	// K3s installation status
	K3sInstalled       bool      `json:"k3sInstalled"`
	K3sVersion         string    `json:"k3sVersion,omitempty"`
	K3sInstallTime     *time.Time `json:"k3sInstallTime,omitempty"`
	K3sInstallError    string    `json:"k3sInstallError,omitempty"`
	
	// K3s configuration status
	K3sRunning         bool      `json:"k3sRunning"`
	K3sConfigTime      *time.Time `json:"k3sConfigTime,omitempty"`
	K3sConfigError     string    `json:"k3sConfigError,omitempty"`
}

// Condition represents a condition of a resource
type Condition struct {
	Type               string    `json:"type"`
	Status             string    `json:"status"` // True, False, Unknown
	LastTransitionTime time.Time `json:"lastTransitionTime"`
	Reason             string    `json:"reason,omitempty"`
	Message            string    `json:"message,omitempty"`
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
