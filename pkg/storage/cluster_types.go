package storage

import (
	"time"
	"github.com/madhouselabs/goman/pkg/models"
)

// ClusterConfig represents the desired state (spec) stored in config.json
type ClusterConfig struct {
	APIVersion string                   `json:"apiVersion"`
	Kind       string                   `json:"kind"`
	Metadata   ClusterMetadata          `json:"metadata"`
	Spec       ClusterSpec              `json:"spec"`
}

// ClusterMetadata contains cluster metadata
type ClusterMetadata struct {
	Name               string            `json:"name"`
	ID                 string            `json:"id"`
	Labels             map[string]string `json:"labels,omitempty"`
	Annotations        map[string]string `json:"annotations,omitempty"`
	CreatedAt          time.Time         `json:"created_at"`
	UpdatedAt          time.Time         `json:"updated_at"`
	DeletionTimestamp  *time.Time        `json:"deletionTimestamp,omitempty"`
}

// ClusterSpec contains the desired cluster specification
type ClusterSpec struct {
	Mode           models.ClusterMode `json:"mode"`
	Region         string             `json:"region"`
	InstanceType   string             `json:"instance_type"`
	K3sVersion     string             `json:"k3s_version"`
	KubeVersion    string             `json:"kube_version"`
	MasterNodes    []models.Node   `json:"master_nodes"`
	WorkerNodes    []models.Node   `json:"worker_nodes"`
	NetworkCIDR    string             `json:"network_cidr"`
	ServiceCIDR    string             `json:"service_cidr"`
	ClusterDNS     string             `json:"cluster_dns"`
	Features       models.K3sFeatures    `json:"features"`
	SSHKeyPath     string             `json:"ssh_key_path"`
	KubeConfigPath string             `json:"kubeconfig_path"`
	Tags           []string           `json:"tags,omitempty"`
}

// ClusterStatus represents the observed state stored in status.json
type ClusterStatus struct {
	Phase         models.ClusterStatus   `json:"phase"`
	Message       string                 `json:"message,omitempty"`
	Reason        string                 `json:"reason,omitempty"`
	UpdatedAt     time.Time              `json:"updated_at"`
	Conditions    []ClusterCondition     `json:"conditions,omitempty"`
	InstanceIDs   map[string]string      `json:"instance_ids,omitempty"`
	Instances     map[string]InstanceInfo `json:"instances,omitempty"`
	VolumeIDs     map[string][]string    `json:"volume_ids,omitempty"`
	SecurityGroups []string              `json:"security_groups,omitempty"`
	VPCID         string                 `json:"vpc_id,omitempty"`
	SubnetIDs     []string               `json:"subnet_ids,omitempty"`
	APIEndpoint   string                 `json:"api_endpoint,omitempty"`
	ClusterToken  string                 `json:"cluster_token,omitempty"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
}

// InstanceInfo contains EC2 instance information
type InstanceInfo struct {
	ID        string `json:"id"`
	PrivateIP string `json:"private_ip"`
	PublicIP  string `json:"public_ip"`
	State     string `json:"state"`
	Role      string `json:"role"`
}

// ClusterCondition represents a condition of a cluster
type ClusterCondition struct {
	Type               string    `json:"type"`
	Status             string    `json:"status"`
	LastTransitionTime time.Time `json:"last_transition_time"`
	Reason             string    `json:"reason,omitempty"`
	Message            string    `json:"message,omitempty"`
}

// ConvertToClusterConfig converts K3sCluster to ClusterConfig (for config.json)
func ConvertToClusterConfig(cluster models.K3sCluster) *ClusterConfig {
	return &ClusterConfig{
		APIVersion: "goman.io/v1",
		Kind:       "K3sCluster",
		Metadata: ClusterMetadata{
			Name:      cluster.Name,
			ID:        cluster.ID,
			CreatedAt: cluster.CreatedAt,
			UpdatedAt: cluster.UpdatedAt,
			Labels: map[string]string{
				"mode":   string(cluster.Mode),
				"region": cluster.Region,
			},
		},
		Spec: ClusterSpec{
			Mode:           cluster.Mode,
			Region:         cluster.Region,
			InstanceType:   cluster.InstanceType,
			K3sVersion:     cluster.K3sVersion,
			KubeVersion:    cluster.KubeVersion,
			MasterNodes:    cluster.MasterNodes,
			WorkerNodes:    cluster.WorkerNodes,
			NetworkCIDR:    cluster.NetworkCIDR,
			ServiceCIDR:    cluster.ServiceCIDR,
			ClusterDNS:     cluster.ClusterDNS,
			Features:       cluster.Features,
			SSHKeyPath:     cluster.SSHKeyPath,
			KubeConfigPath: cluster.KubeConfigPath,
			Tags:           cluster.Tags,
		},
	}
}

// ConvertFromClusterConfig converts ClusterConfig back to K3sCluster
func ConvertFromClusterConfig(config *ClusterConfig, status *ClusterStatus) models.K3sCluster {
	cluster := models.K3sCluster{
		ID:             config.Metadata.ID,
		Name:           config.Metadata.Name,
		Mode:           config.Spec.Mode,
		Region:         config.Spec.Region,
		InstanceType:   config.Spec.InstanceType,
		K3sVersion:     config.Spec.K3sVersion,
		KubeVersion:    config.Spec.KubeVersion,
		MasterNodes:    config.Spec.MasterNodes,
		WorkerNodes:    config.Spec.WorkerNodes,
		NetworkCIDR:    config.Spec.NetworkCIDR,
		ServiceCIDR:    config.Spec.ServiceCIDR,
		ClusterDNS:     config.Spec.ClusterDNS,
		Features:       config.Spec.Features,
		SSHKeyPath:     config.Spec.SSHKeyPath,
		KubeConfigPath: config.Spec.KubeConfigPath,
		Tags:           config.Spec.Tags,
		CreatedAt:      config.Metadata.CreatedAt,
		UpdatedAt:      config.Metadata.UpdatedAt,
	}

	// Add status if available
	if status != nil {
		cluster.Status = status.Phase
		cluster.APIEndpoint = status.APIEndpoint
		cluster.ClusterToken = status.ClusterToken
	}

	// Calculate totals
	for _, node := range cluster.MasterNodes {
		cluster.TotalCPU += node.CPU
		cluster.TotalMemoryGB += node.MemoryGB
		cluster.TotalStorageGB += node.StorageGB
	}
	for _, node := range cluster.WorkerNodes {
		cluster.TotalCPU += node.CPU
		cluster.TotalMemoryGB += node.MemoryGB
		cluster.TotalStorageGB += node.StorageGB
	}

	// Estimate cost based on mode
	masterCost := 50
	if cluster.Mode == models.ModeHA {
		masterCost = 150 // 3 masters
	}
	cluster.EstimatedCost = float64(masterCost + len(cluster.WorkerNodes)*30)

	return cluster
}