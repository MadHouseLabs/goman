package models

import (
	"time"
)

// ClusterStatus represents the status of a k3s cluster
type ClusterStatus string

const (
	StatusRunning   ClusterStatus = "running"
	StatusStopped   ClusterStatus = "stopped"
	StatusCreating  ClusterStatus = "creating"
	StatusDeleting  ClusterStatus = "deleting"
	StatusUpdating  ClusterStatus = "updating"
	StatusError     ClusterStatus = "error"
)

// NodeRole represents the role of a node in the cluster
type NodeRole string

const (
	RoleMaster NodeRole = "master"
	RoleWorker NodeRole = "worker"
)

// Node represents a single node in the k3s cluster
type Node struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Role         NodeRole  `json:"role"`
	IP           string    `json:"ip"`
	Status       string    `json:"status"`
	CPU          int       `json:"cpu"`
	MemoryGB     int       `json:"memory_gb"`
	StorageGB    int       `json:"storage_gb"`
	Provider     string    `json:"provider"`
	InstanceType string    `json:"instance_type"`
	Region       string    `json:"region"`
	CreatedAt    time.Time `json:"created_at"`
}

// K3sCluster represents a k3s Kubernetes cluster
type K3sCluster struct {
	ID               string        `json:"id"`
	Name             string        `json:"name"`
	Status           ClusterStatus `json:"status"`
	K3sVersion       string        `json:"k3s_version"`
	KubeVersion      string        `json:"kube_version"`
	MasterNodes      []Node        `json:"master_nodes"`
	WorkerNodes      []Node        `json:"worker_nodes"`
	APIEndpoint      string        `json:"api_endpoint"`
	ClusterToken     string        `json:"cluster_token"`
	CreatedAt        time.Time     `json:"created_at"`
	UpdatedAt        time.Time     `json:"updated_at"`
	TotalCPU         int           `json:"total_cpu"`
	TotalMemoryGB    int           `json:"total_memory_gb"`
	TotalStorageGB   int           `json:"total_storage_gb"`
	EstimatedCost    float64       `json:"estimated_cost"`
	Tags             []string      `json:"tags"`
	SSHKeyPath       string        `json:"ssh_key_path"`
	KubeConfigPath   string        `json:"kubeconfig_path"`
	NetworkCIDR      string        `json:"network_cidr"`
	ServiceCIDR      string        `json:"service_cidr"`
	ClusterDNS       string        `json:"cluster_dns"`
	Features         K3sFeatures   `json:"features"`
}

// K3sFeatures represents optional k3s features
type K3sFeatures struct {
	Traefik          bool   `json:"traefik"`
	ServiceLB        bool   `json:"servicelb"`
	LocalStorage     bool   `json:"local_storage"`
	MetricsServer    bool   `json:"metrics_server"`
	CoreDNS          bool   `json:"coredns"`
	FlannelBackend   string `json:"flannel_backend"`
}

// ClusterConfig represents the user's input configuration for a cluster
type ClusterConfig struct {
	Name         string            `yaml:"name"`
	Region       string            `yaml:"region"`
	Provider     string            `yaml:"provider"`
	Version      string            `yaml:"k3s_version"`
	NodeCount    int               `yaml:"node_count"`
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