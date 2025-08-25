package models

import (
	"time"
)

// ClusterStatus represents the status of a k3s cluster
type ClusterStatus string

const (
	StatusRunning  ClusterStatus = "running"
	StatusStopped  ClusterStatus = "stopped"
	StatusCreating ClusterStatus = "creating"
	StatusDeleting ClusterStatus = "deleting"
	StatusUpdating ClusterStatus = "updating"
	StatusError    ClusterStatus = "error"
	StatusStarting ClusterStatus = "starting"
	StatusStopping ClusterStatus = "stopping"
)

// NodeRole represents the role of a node in the cluster
type NodeRole string

const (
	RoleMaster NodeRole = "master"
	RoleWorker NodeRole = "worker"
)

// Node represents a single node in the k3s cluster
type Node struct {
	ID           string    `json:"id" yaml:"id"`
	Name         string    `json:"name" yaml:"name"`
	Role         NodeRole  `json:"role" yaml:"role"`
	IP           string    `json:"ip" yaml:"ip"`
	Status       string    `json:"status" yaml:"status"`
	CPU          int       `json:"cpu" yaml:"cpu"`
	MemoryGB     int       `json:"memory_gb" yaml:"memoryGB"`
	StorageGB    int       `json:"storage_gb" yaml:"storageGB"`
	Provider     string    `json:"provider" yaml:"provider"`
	InstanceType string    `json:"instance_type" yaml:"instanceType"`
	Region       string    `json:"region" yaml:"region"`
	CreatedAt    time.Time `json:"created_at" yaml:"createdAt"`
}

// ClusterMode represents the deployment mode of the cluster
type ClusterMode string

const (
	ModeDev ClusterMode = "dev" // Single master node for development
	ModeHA  ClusterMode = "ha"  // 3 master nodes for high availability
)

// K3sCluster represents a k3s Kubernetes cluster
type K3sCluster struct {
	ID             string        `json:"id"`
	Name           string        `json:"name"`
	Description    string        `json:"description"`
	Status         ClusterStatus `json:"status"`
	Mode           ClusterMode   `json:"mode"`
	Region         string        `json:"region"`
	InstanceType   string        `json:"instance_type"`
	K3sVersion     string        `json:"k3s_version"`
	KubeVersion    string        `json:"kube_version"`
	MasterNodes    []Node        `json:"master_nodes"`
	WorkerNodes    []Node        `json:"worker_nodes"`
	APIEndpoint    string        `json:"api_endpoint"`
	ClusterToken   string        `json:"cluster_token"`
	CreatedAt      time.Time     `json:"created_at"`
	UpdatedAt      time.Time     `json:"updated_at"`
	TotalCPU       int           `json:"total_cpu"`
	TotalMemoryGB  int           `json:"total_memory_gb"`
	TotalStorageGB int           `json:"total_storage_gb"`
	EstimatedCost  float64       `json:"estimated_cost"`
	Tags           []string      `json:"tags"`
	SSHKeyPath     string        `json:"ssh_key_path"`
	KubeConfigPath string        `json:"kubeconfig_path"`
	NetworkCIDR    string        `json:"network_cidr"`
	ServiceCIDR    string        `json:"service_cidr"`
	ClusterDNS     string        `json:"cluster_dns"`
	Features       K3sFeatures   `json:"features"`
	DesiredState   string        `json:"desired_state"` // "running" or "stopped"
	NodePools      []NodePool    `json:"node_pools,omitempty"` // Worker node pools
}

// GetMasterCount returns the number of master nodes based on the cluster mode
func (c *K3sCluster) GetMasterCount() int {
	switch c.Mode {
	case ModeHA:
		return 3
	case ModeDev:
		return 1
	default:
		return 1
	}
}

// K3sFeatures represents optional k3s features
type K3sFeatures struct {
	Traefik        bool   `json:"traefik" yaml:"traefik"`
	ServiceLB      bool   `json:"servicelb" yaml:"serviceLB"`
	LocalStorage   bool   `json:"local_storage" yaml:"localStorage"`
	MetricsServer  bool   `json:"metrics_server" yaml:"metricsServer"`
	CoreDNS        bool   `json:"coredns" yaml:"coreDNS"`
	FlannelBackend string `json:"flannel_backend" yaml:"flannelBackend"`
}

// ClusterConfig represents the user's input configuration for a cluster
type ClusterConfig struct {
	Name         string            `yaml:"name"`
	Region       string            `yaml:"region"`
	Provider     string            `yaml:"provider"`
	Version      string            `yaml:"k3s_version"`
	Mode         string            `yaml:"mode"`
	InstanceType string            `yaml:"instance_type"`
	Tags         map[string]string `yaml:"tags,omitempty"`
	SSHKeyPath   string            `yaml:"ssh_key_path,omitempty"`
	Network      NetworkConfig     `yaml:"network"`
}

// NetworkConfig represents network configuration
type NetworkConfig struct {
	VpcCIDR     string `yaml:"vpc_cidr"`
	SubnetCIDR  string `yaml:"subnet_cidr"`
	ServiceCIDR string `yaml:"service_cidr"`
	PodCIDR     string `yaml:"pod_cidr"`
}

// ClusterState represents the actual infrastructure state
type ClusterState struct {
	ClusterID       string          `yaml:"cluster_id"`
	Name            string          `yaml:"name"`
	Provider        string          `yaml:"provider"`
	Region          string          `yaml:"region"`
	Status          string          `yaml:"status"`
	Version         string          `yaml:"k3s_version"`
	CreatedAt       time.Time       `yaml:"created_at"`
	UpdatedAt       time.Time       `yaml:"updated_at"`
	VpcID           string          `yaml:"vpc_id,omitempty"`
	SubnetID        string          `yaml:"subnet_id,omitempty"`
	SecurityGroupID string          `yaml:"security_group_id,omitempty"`
	KeyPairName     string          `yaml:"key_pair_name,omitempty"`
	MasterEndpoint  string          `yaml:"master_endpoint,omitempty"`
	ClusterToken    string          `yaml:"cluster_token,omitempty"`
	Instances       []InstanceState `yaml:"instances"`
	Config          ClusterConfig   `yaml:"config"`
}

// InstanceState represents an EC2 instance state
type InstanceState struct {
	InstanceID   string    `yaml:"instance_id"`
	Name         string    `yaml:"name"`
	Role         string    `yaml:"role"`
	InstanceType string    `yaml:"instance_type"`
	PrivateIP    string    `yaml:"private_ip"`
	PublicIP     string    `yaml:"public_ip,omitempty"`
	State        string    `yaml:"state"`
	LaunchedAt   time.Time `yaml:"launched_at"`
	UserData     string    `yaml:"user_data,omitempty"`
}
