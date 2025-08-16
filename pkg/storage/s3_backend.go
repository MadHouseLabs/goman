package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"gopkg.in/yaml.v3"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	gomanconfig "github.com/madhouselabs/goman/pkg/config"
	"github.com/madhouselabs/goman/pkg/models"
)

// S3Backend implements StorageBackend using AWS S3
type S3Backend struct {
	client     *s3.Client
	bucketName string
	prefix     string
	accountID  string
	region     string
}

// NewS3Backend creates a new S3 storage backend with standardized naming
func NewS3Backend(profile string) (*S3Backend, error) {
	// Use standard region (Mumbai, India)
	region := gomanconfig.GetAWSRegion()

	// Use default profile if not specified
	if profile == "" {
		profile = gomanconfig.GetAWSProfile()
	}

	// Load AWS config with profile (profile is only used for authentication)
	var cfg aws.Config
	var err error

	if profile != "" {
		// Use profile if specified (local development)
		cfg, err = config.LoadDefaultConfig(context.Background(),
			config.WithRegion(region),
			config.WithSharedConfigProfile(profile),
		)
	} else {
		// Use default credentials chain (Lambda IAM role, EC2 instance role, etc.)
		cfg, err = config.LoadDefaultConfig(context.Background(),
			config.WithRegion(region),
		)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Get AWS account ID
	stsClient := sts.NewFromConfig(cfg)
	identity, err := stsClient.GetCallerIdentity(context.Background(), &sts.GetCallerIdentityInput{})
	if err != nil {
		return nil, fmt.Errorf("failed to get AWS account ID: %w", err)
	}

	accountID := *identity.Account

	// Use standardized bucket name: goman-{accountID}
	// Region is always ap-south-1, so no need to include it in bucket name
	bucketName := fmt.Sprintf("goman-%s", accountID)

	// No prefix - files will be directly under bucket root
	// This gives us clean paths like clusters/{name}.json
	prefix := ""

	return &S3Backend{
		client:     s3.NewFromConfig(cfg),
		bucketName: bucketName,
		prefix:     prefix,
		accountID:  accountID,
		region:     region,
	}, nil
}

// Initialize creates the S3 bucket if it doesn't exist
func (s *S3Backend) Initialize() error {
	ctx := context.Background()

	// Check if bucket exists
	_, err := s.client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(s.bucketName),
	})

	if err != nil {
		// Bucket doesn't exist, creating it silently

		// Create bucket if it doesn't exist
		createInput := &s3.CreateBucketInput{
			Bucket: aws.String(s.bucketName),
		}

		// Add location constraint for non-us-east-1 regions
		// ap-south-1 requires location constraint
		if s.region != "us-east-1" {
			createInput.CreateBucketConfiguration = &types.CreateBucketConfiguration{
				LocationConstraint: types.BucketLocationConstraint(s.region),
			}
		}

		_, createErr := s.client.CreateBucket(ctx, createInput)
		if createErr != nil {
			if !strings.Contains(createErr.Error(), "BucketAlreadyExists") &&
				!strings.Contains(createErr.Error(), "BucketAlreadyOwnedByYou") {
				return fmt.Errorf("failed to create S3 bucket %s in region %s: %w", s.bucketName, s.region, createErr)
			}
		}
	}

	// Only download K3s binaries during CLI initialization, not in Lambda
	// Check if we're running in Lambda by looking for AWS_LAMBDA_FUNCTION_NAME
	if os.Getenv("AWS_LAMBDA_FUNCTION_NAME") == "" {
		// Not in Lambda, safe to download binaries
		if err := s.setupK3sBinaries(ctx); err != nil {
			log.Printf("Warning: Failed to setup K3s binaries in S3: %v", err)
			// Don't fail initialization, binaries can be downloaded later
		}
	}

	return nil
}

// setupK3sBinaries downloads K3s binaries from GitHub and stores them in S3
func (s *S3Backend) setupK3sBinaries(ctx context.Context) error {
	// K3s versions to download
	versions := []string{"v1.33.3+k3s1", "v1.32.6+k3s1", "v1.31.4+k3s1"}
	architectures := []string{"amd64", "arm64"}

	for _, version := range versions {
		for _, arch := range architectures {
			key := fmt.Sprintf("binaries/k3s/%s/k3s-%s", version, arch)
			
			// Check if binary already exists in S3
			_, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
				Bucket: aws.String(s.bucketName),
				Key:    aws.String(key),
			})
			
			if err == nil {
				// Binary already exists, skip silently
				continue
			}

			// Download from GitHub
			downloadURL := fmt.Sprintf("https://github.com/k3s-io/k3s/releases/download/%s/k3s", version)
			if arch != "amd64" {
				downloadURL = fmt.Sprintf("https://github.com/k3s-io/k3s/releases/download/%s/k3s-%s", version, arch)
			}

			log.Printf("Downloading K3s %s for %s from GitHub...", version, arch)
			resp, err := http.Get(downloadURL)
			if err != nil {
				log.Printf("Failed to download K3s %s for %s: %v", version, arch, err)
				continue
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				log.Printf("Failed to download K3s %s for %s: HTTP %d", version, arch, resp.StatusCode)
				continue
			}

			// Read the binary
			data, err := io.ReadAll(resp.Body)
			if err != nil {
				log.Printf("Failed to read K3s binary %s for %s: %v", version, arch, err)
				continue
			}

			// Upload to S3
			_, err = s.client.PutObject(ctx, &s3.PutObjectInput{
				Bucket:      aws.String(s.bucketName),
				Key:         aws.String(key),
				Body:        bytes.NewReader(data),
				ContentType: aws.String("application/octet-stream"),
			})

			if err != nil {
				log.Printf("Failed to upload K3s %s for %s to S3: %v", version, arch, err)
				continue
			}

			log.Printf("Successfully stored K3s %s for %s in S3", version, arch)
		}
	}

	return nil
}

// getKey returns the S3 key for a given path
func (s *S3Backend) getKey(path string) string {
	if s.prefix != "" {
		return fmt.Sprintf("%s/%s", s.prefix, path)
	}
	return path
}

// PutObject uploads data to S3 (exported for direct access)
func (s *S3Backend) PutObject(ctx context.Context, key string, data []byte) error {

	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucketName),
		Key:         aws.String(key),
		Body:        bytes.NewReader(data),
		ContentType: aws.String("application/x-yaml"),
	})

	return err
}

// GetObject downloads data from S3 (exported for direct access)
func (s *S3Backend) GetObject(ctx context.Context, key string) ([]byte, error) {

	result, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucketName),
		Key:    aws.String(key),
	})
	if err != nil {
		if strings.Contains(err.Error(), "NoSuchKey") {
			return nil, fmt.Errorf("object not found")
		}
		return nil, err
	}
	defer result.Body.Close()

	buf := new(bytes.Buffer)
	_, err = buf.ReadFrom(result.Body)
	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// DeleteObject deletes an object from S3 (exported for direct access)
func (s *S3Backend) DeleteObject(ctx context.Context, key string) error {
	// Use the key directly - caller should provide the correct path
	// e.g., "clusters/foo/status.yaml"
	fullKey := key
	if s.prefix != "" && !strings.HasPrefix(key, s.prefix) {
		fullKey = s.prefix + "/" + key
	}
	
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucketName),
		Key:    aws.String(fullKey),
	})

	return err
}

// listObjects lists objects with a given prefix
func (s *S3Backend) listObjects(ctx context.Context, prefix string) ([]string, error) {

	fullPrefix := s.getKey(prefix)
	result, err := s.client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket: aws.String(s.bucketName),
		Prefix: aws.String(fullPrefix),
	})
	if err != nil {
		return nil, err
	}

	var keys []string
	for _, obj := range result.Contents {
		if obj.Key != nil {
			// Remove the backend prefix from the key
			key := *obj.Key
			if s.prefix != "" {
				key = strings.TrimPrefix(key, s.prefix+"/")
			}
			keys = append(keys, key)
		}
	}

	return keys, nil
}

// SaveClusters saves k3s clusters to S3
func (s *S3Backend) SaveClusters(clusters []models.K3sCluster) error {
	data, err := json.MarshalIndent(clusters, "", "  ")
	if err != nil {
		return err
	}

	key := s.getKey("clusters/clusters.json")
	return s.PutObject(context.Background(), key, data)
}

// LoadClusters loads k3s clusters from S3
func (s *S3Backend) LoadClusters() ([]models.K3sCluster, error) {
	key := s.getKey("clusters/clusters.json")
	data, err := s.GetObject(context.Background(), key)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return []models.K3sCluster{}, nil
		}
		return nil, err
	}

	var clusters []models.K3sCluster
	if err := json.Unmarshal(data, &clusters); err != nil {
		return nil, err
	}

	return clusters, nil
}

// SaveClusterState saves complete cluster state to S3
// Each cluster is saved in its own file for better S3 event triggering
func (s *S3Backend) SaveClusterState(state *K3sClusterState) error {
	if state == nil || state.Cluster.Name == "" {
		return fmt.Errorf("invalid cluster state")
	}

	// Save individual cluster state file
	// Format: clusters/{cluster-name}.yaml
	key := s.getKey(fmt.Sprintf("clusters/%s.yaml", state.Cluster.Name))

	data, err := yaml.Marshal(state)
	if err != nil {
		return fmt.Errorf("failed to marshal cluster state: %w", err)
	}

	// Save to S3 - this will trigger Lambda via S3 events
	if err := s.PutObject(context.Background(), key, data); err != nil {
		return fmt.Errorf("failed to save cluster state: %w", err)
	}

	return nil
}

// SaveClusterStateToKey saves cluster state to a specific S3 key
func (s *S3Backend) SaveClusterStateToKey(state *K3sClusterState, key string) error {
	if state == nil {
		return fmt.Errorf("invalid cluster state")
	}

	data, err := yaml.Marshal(state)
	if err != nil {
		return fmt.Errorf("failed to marshal cluster state: %w", err)
	}

	// Save to S3 with the specified key
	fullKey := s.getKey(key)
	if err := s.PutObject(context.Background(), fullKey, data); err != nil {
		return fmt.Errorf("failed to save cluster state to %s: %w", key, err)
	}

	return nil
}

// LoadClusterState loads complete cluster state from S3
// Only supports YAML format
func (s *S3Backend) LoadClusterState(clusterName string) (*K3sClusterState, error) {
	ctx := context.Background()
	
	// Load config file (YAML)
	configKey := s.getKey(fmt.Sprintf("clusters/%s/config.yaml", clusterName))
	configData, err := s.GetObject(ctx, configKey)
	if err != nil {
		return nil, fmt.Errorf("cluster %s config not found: %w", clusterName, err)
	}
	
	// Parse config as YAML
	var config ClusterConfig
	if err := yaml.Unmarshal(configData, &config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}
	
	// Validate new format
	if config.APIVersion == "" || config.Kind == "" {
		return nil, fmt.Errorf("invalid config format: missing apiVersion or kind (old format no longer supported)")
	}
	
	// Load status file (YAML)
	var status *ClusterStatus
	statusKey := s.getKey(fmt.Sprintf("clusters/%s/status.yaml", clusterName))
	statusData, err := s.GetObject(ctx, statusKey)
	if err == nil {
		// Try to unmarshal the raw data to see the structure
		var rawStatus map[string]interface{}
		if err := yaml.Unmarshal(statusData, &rawStatus); err == nil {
			// Check if this is Lambda's format (with nested cluster and metadata)
			if clusterData, ok := rawStatus["cluster"].(map[string]interface{}); ok {
				status = &ClusterStatus{}
				
				// Extract status from cluster.status
				if statusStr, ok := clusterData["status"].(string); ok {
					// Map string status to ClusterStatus phase
					switch statusStr {
					case "running":
						status.Phase = models.StatusRunning
					case "creating":
						status.Phase = models.StatusCreating
					case "error":
						status.Phase = models.StatusError
					case "deleting":
						status.Phase = models.StatusDeleting
					case "stopped":
						status.Phase = models.StatusStopped
					default:
						status.Phase = models.StatusCreating
					}
				} else {
					// No status in cluster data, default to creating
					status.Phase = models.StatusCreating
				}
				
				// Extract metadata
				if metadata, ok := rawStatus["metadata"].(map[string]interface{}); ok {
					status.Metadata = metadata
					if msg, ok := metadata["message"].(string); ok {
						status.Message = msg
					}
				}
				
				// Extract instance IDs
				if instanceIDs, ok := rawStatus["instance_ids"].(map[string]interface{}); ok {
					status.InstanceIDs = make(map[string]string)
					for k, v := range instanceIDs {
						if id, ok := v.(string); ok {
							status.InstanceIDs[k] = id
						}
					}
				}
				
				// Extract instance info
				if instances, ok := rawStatus["instances"].(map[string]interface{}); ok {
					status.Instances = make(map[string]InstanceInfo)
					for k, v := range instances {
						if inst, ok := v.(map[string]interface{}); ok {
							info := InstanceInfo{}
							if id, ok := inst["id"].(string); ok {
								info.ID = id
							}
							if ip, ok := inst["private_ip"].(string); ok {
								info.PrivateIP = ip
							}
							if ip, ok := inst["public_ip"].(string); ok {
								info.PublicIP = ip
							}
							if state, ok := inst["state"].(string); ok {
								info.State = state
							}
							if role, ok := inst["role"].(string); ok {
								info.Role = role
							}
							status.Instances[k] = info
						}
					}
				}
			} else {
				// Try direct unmarshal for UI-created format
				status = &ClusterStatus{}
				if err := yaml.Unmarshal(statusData, status); err != nil {
					// Log error but continue without status
					status = nil
				}
			}
		}
	}
	
	// Convert to K3sCluster
	cluster := ConvertFromClusterConfig(&config, status)
	
	// Build K3sClusterState
	state := &K3sClusterState{
		Cluster:     cluster,
		InstanceIDs: make(map[string]string),
		VolumeIDs:   make(map[string][]string),
		Metadata:    make(map[string]interface{}),
	}
	
	// Add status data if available
	if status != nil {
		if status.InstanceIDs != nil {
			state.InstanceIDs = status.InstanceIDs
		}
		if status.VolumeIDs != nil {
			state.VolumeIDs = status.VolumeIDs
		}
		state.SecurityGroups = status.SecurityGroups
		state.VPCID = status.VPCID
		state.SubnetIDs = status.SubnetIDs
		if status.Metadata != nil {
			state.Metadata = status.Metadata
		}
	}
	
	return state, nil
}

// LoadAllClusterStates loads all cluster states from S3
// Only supports YAML format: clusters/{name}/config.yaml + status.yaml
func (s *S3Backend) LoadAllClusterStates() ([]*K3sClusterState, error) {
	// List all cluster files
	keys, err := s.listObjects(context.Background(), "clusters/")
	if err != nil {
		return nil, err
	}

	var states []*K3sClusterState
	processedClusters := make(map[string]bool)
	
	for _, key := range keys {
		// Only process config.yaml files
		if !strings.HasSuffix(key, "/config.yaml") {
			continue
		}
		
		// Extract cluster name from key: clusters/{name}/config.yaml
		parts := strings.Split(key, "/")
		if len(parts) < 3 {
			continue
		}
		
		clusterName := parts[1]
		
		// Skip if already processed
		if processedClusters[clusterName] {
			continue
		}
		
		// Load the cluster state
		state, err := s.LoadClusterState(clusterName)
		if err != nil {
			continue // Skip clusters that can't be loaded
		}
		
		states = append(states, state)
		processedClusters[clusterName] = true
	}

	return states, nil
}

// DeleteClusterState deletes cluster state from S3
// Only deletes YAML format files
func (s *S3Backend) DeleteClusterState(clusterName string) error {
	ctx := context.Background()

	// Delete config and status files
	configKey := s.getKey(fmt.Sprintf("clusters/%s/config.yaml", clusterName))
	s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucketName),
		Key:    aws.String(configKey),
	})
	
	statusKey := s.getKey(fmt.Sprintf("clusters/%s/status.yaml", clusterName))
	s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucketName),
		Key:    aws.String(statusKey),
	})

	return nil
}


// SaveJob saves a job to S3
func (s *S3Backend) SaveJob(job *models.Job) error {
	if job == nil || job.ID == "" {
		return fmt.Errorf("invalid job")
	}

	data, err := json.MarshalIndent(job, "", "  ")
	if err != nil {
		return err
	}

	key := s.getKey(fmt.Sprintf("jobs/%s.json", job.ID))
	return s.PutObject(context.Background(), key, data)
}

// LoadJob loads a job from S3
func (s *S3Backend) LoadJob(jobID string) (*models.Job, error) {
	key := s.getKey(fmt.Sprintf("jobs/%s.json", jobID))
	data, err := s.GetObject(context.Background(), key)
	if err != nil {
		return nil, err
	}

	var job models.Job
	if err := json.Unmarshal(data, &job); err != nil {
		return nil, err
	}

	return &job, nil
}

// LoadAllJobs loads all jobs from S3
func (s *S3Backend) LoadAllJobs() ([]*models.Job, error) {
	keys, err := s.listObjects(context.Background(), "jobs/")
	if err != nil {
		return nil, err
	}

	var jobs []*models.Job
	for _, key := range keys {
		if strings.HasSuffix(key, ".json") {
			data, err := s.GetObject(context.Background(), s.getKey(key))
			if err != nil {
				continue
			}

			var job models.Job
			if err := json.Unmarshal(data, &job); err != nil {
				continue
			}
			jobs = append(jobs, &job)
		}
	}

	return jobs, nil
}

// DeleteJob deletes a job from S3
func (s *S3Backend) DeleteJob(jobID string) error {
	key := fmt.Sprintf("jobs/%s.json", jobID)
	return s.DeleteObject(context.Background(), key)
}

// SaveConfig saves application configuration to S3
func (s *S3Backend) SaveConfig(config map[string]interface{}) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}

	key := s.getKey("config.json")
	return s.PutObject(context.Background(), key, data)
}

// LoadConfig loads application configuration from S3
func (s *S3Backend) LoadConfig() (map[string]interface{}, error) {
	key := s.getKey("config.json")
	data, err := s.GetObject(context.Background(), key)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			// Return default config
			return map[string]interface{}{
				"default_provider": "AWS",
				"default_region":   s.region,
				"theme":            "dark",
			}, nil
		}
		return nil, err
	}

	var config map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	return config, nil
}
