package storage

import (
	"time"
	"github.com/madhouselabs/goman/pkg/models"
)

// ClusterConfig represents the desired state (spec) stored in config.yaml
type ClusterConfig struct {
	APIVersion string                   `json:"apiVersion" yaml:"apiVersion"`
	Kind       string                   `json:"kind" yaml:"kind"`
	Metadata   ClusterMetadata          `json:"metadata" yaml:"metadata"`
	Spec       ClusterSpec              `json:"spec" yaml:"spec"`
}

// ClusterMetadata contains cluster metadata
type ClusterMetadata struct {
	Name               string            `json:"name" yaml:"name"`
	ID                 string            `json:"id" yaml:"id"`
	Labels             map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`
	Annotations        map[string]string `json:"annotations,omitempty" yaml:"annotations,omitempty"`
	CreatedAt          time.Time         `json:"created_at" yaml:"createdAt"`
	UpdatedAt          time.Time         `json:"updated_at" yaml:"updatedAt"`
	DeletionTimestamp  *time.Time        `json:"deletionTimestamp,omitempty" yaml:"deletionTimestamp,omitempty"`
}

// ClusterSpec contains the desired cluster specification
type ClusterSpec struct {
	Description    string             `json:"description" yaml:"description"`
	Mode           models.ClusterMode `json:"mode" yaml:"mode"`
	Region         string             `json:"region" yaml:"region"`
	InstanceType   string             `json:"instance_type" yaml:"instanceType"`
	K3sVersion     string             `json:"k3s_version" yaml:"k3sVersion"`
	KubeVersion    string             `json:"kube_version" yaml:"kubeVersion"`
	MasterNodes    []models.Node   `json:"master_nodes" yaml:"masterNodes"`
	WorkerNodes    []models.Node   `json:"worker_nodes" yaml:"workerNodes"`
	NetworkCIDR    string             `json:"network_cidr" yaml:"networkCIDR"`
	ServiceCIDR    string             `json:"service_cidr" yaml:"serviceCIDR"`
	ClusterDNS     string             `json:"cluster_dns" yaml:"clusterDNS"`
	Features       models.K3sFeatures    `json:"features" yaml:"features"`
	SSHKeyPath     string             `json:"ssh_key_path" yaml:"sshKeyPath"`
	KubeConfigPath string             `json:"kubeconfig_path" yaml:"kubeConfigPath"`
	Tags           []string           `json:"tags,omitempty" yaml:"tags,omitempty"`
}

// ClusterStatus represents the observed state stored in status.yaml
type ClusterStatus struct {
	Phase         models.ClusterStatus   `json:"phase" yaml:"phase"`
	Message       string                 `json:"message,omitempty" yaml:"message,omitempty"`
	Reason        string                 `json:"reason,omitempty" yaml:"reason,omitempty"`
	UpdatedAt     time.Time              `json:"updated_at" yaml:"updatedAt"`
	Conditions    []ClusterCondition     `json:"conditions,omitempty" yaml:"conditions,omitempty"`
	InstanceIDs   map[string]string      `json:"instance_ids,omitempty" yaml:"instanceIDs,omitempty"`
	Instances     map[string]InstanceInfo `json:"instances,omitempty" yaml:"instances,omitempty"`
	VolumeIDs     map[string][]string    `json:"volume_ids,omitempty" yaml:"volumeIDs,omitempty"`
	SecurityGroups []string              `json:"security_groups,omitempty" yaml:"securityGroups,omitempty"`
	VPCID         string                 `json:"vpc_id,omitempty" yaml:"vpcID,omitempty"`
	SubnetIDs     []string               `json:"subnet_ids,omitempty" yaml:"subnetIDs,omitempty"`
	APIEndpoint   string                 `json:"api_endpoint,omitempty" yaml:"apiEndpoint,omitempty"`
	ClusterToken  string                 `json:"cluster_token,omitempty" yaml:"clusterToken,omitempty"`
	Metadata      map[string]interface{} `json:"metadata,omitempty" yaml:"metadata,omitempty"`
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
			Description:    cluster.Description,
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
		Description:    config.Spec.Description,
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

	// Check if cluster is marked for deletion
	if config.Metadata.DeletionTimestamp != nil {
		// Cluster has been marked for deletion
		cluster.Status = models.StatusDeleting
	} else if status != nil {
		// Check if this status is from an old deletion that's still being processed
		// If the status has deletion_requested in metadata but config doesn't have DeletionTimestamp,
		// this is a stale status from a previous cluster with the same name
		if status.Metadata != nil {
			if _, hasDeletionRequested := status.Metadata["deletion_requested"]; hasDeletionRequested {
				// This is a stale status from a deleted cluster, ignore it
				cluster.Status = models.StatusCreating
			} else {
				// Add status if available and valid
				cluster.Status = status.Phase
				cluster.APIEndpoint = status.APIEndpoint
				cluster.ClusterToken = status.ClusterToken
			}
		} else {
			// Status exists but no metadata - use the phase if it's valid
			if status.Phase != "" {
				cluster.Status = status.Phase
				cluster.APIEndpoint = status.APIEndpoint
				cluster.ClusterToken = status.ClusterToken
			} else {
				// Status exists but phase is empty, default to creating
				cluster.Status = models.StatusCreating
			}
		}
		
		// Update nodes with instance information from status
		if status.Instances != nil {
			// If no nodes in config, create them from instances
			if len(cluster.MasterNodes) == 0 && len(cluster.WorkerNodes) == 0 {
				// Create nodes from instances
				masterNodes := []models.Node{}
				workerNodes := []models.Node{}
				
				for name, instInfo := range status.Instances {
					node := models.Node{
						Name:   name,
						ID:     instInfo.ID,
						IP:     instInfo.PrivateIP,
						Status: instInfo.State,
						Role:   models.NodeRole(instInfo.Role),
					}
					
					if instInfo.Role == "master" {
						masterNodes = append(masterNodes, node)
					} else if instInfo.Role == "worker" {
						workerNodes = append(workerNodes, node)
					}
				}
				
				cluster.MasterNodes = masterNodes
				cluster.WorkerNodes = workerNodes
			} else {
				// Update existing nodes
				for i, node := range cluster.MasterNodes {
					if instInfo, ok := status.Instances[node.Name]; ok {
						cluster.MasterNodes[i].ID = instInfo.ID
						cluster.MasterNodes[i].IP = instInfo.PrivateIP
						cluster.MasterNodes[i].Status = instInfo.State
					}
				}
				// Update worker nodes
				for i, node := range cluster.WorkerNodes {
					if instInfo, ok := status.Instances[node.Name]; ok {
						cluster.WorkerNodes[i].ID = instInfo.ID
						cluster.WorkerNodes[i].IP = instInfo.PrivateIP
						cluster.WorkerNodes[i].Status = instInfo.State
					}
				}
			}
		}
	} else {
		// Default status when no status file exists yet
		cluster.Status = models.StatusCreating
	}

	// Calculate totals from nodes (will be 0 for now, replaced by live metrics)
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