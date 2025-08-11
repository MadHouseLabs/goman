package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

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

	return nil
}

// getKey returns the S3 key for a given path
func (s *S3Backend) getKey(path string) string {
	if s.prefix != "" {
		return fmt.Sprintf("%s/%s", s.prefix, path)
	}
	return path
}

// putObject uploads data to S3
func (s *S3Backend) putObject(ctx context.Context, key string, data []byte) error {

	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucketName),
		Key:         aws.String(key),
		Body:        bytes.NewReader(data),
		ContentType: aws.String("application/json"),
	})

	return err
}

// getObject downloads data from S3
func (s *S3Backend) getObject(ctx context.Context, key string) ([]byte, error) {

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

// deleteObject deletes an object from S3
func (s *S3Backend) deleteObject(ctx context.Context, key string) error {

	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucketName),
		Key:    aws.String(key),
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
	return s.putObject(context.Background(), key, data)
}

// LoadClusters loads k3s clusters from S3
func (s *S3Backend) LoadClusters() ([]models.K3sCluster, error) {
	key := s.getKey("clusters/clusters.json")
	data, err := s.getObject(context.Background(), key)
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
	// Format: clusters/{cluster-name}.json
	key := s.getKey(fmt.Sprintf("clusters/%s.json", state.Cluster.Name))

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal cluster state: %w", err)
	}

	// Save to S3 - this will trigger Lambda via S3 events
	if err := s.putObject(context.Background(), key, data); err != nil {
		return fmt.Errorf("failed to save cluster state: %w", err)
	}

	return nil
}

// SaveClusterStateToKey saves cluster state to a specific S3 key
func (s *S3Backend) SaveClusterStateToKey(state *K3sClusterState, key string) error {
	if state == nil {
		return fmt.Errorf("invalid cluster state")
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal cluster state: %w", err)
	}

	// Save to S3 with the specified key
	fullKey := s.getKey(key)
	if err := s.putObject(context.Background(), fullKey, data); err != nil {
		return fmt.Errorf("failed to save cluster state to %s: %w", key, err)
	}

	return nil
}

// LoadClusterState loads complete cluster state from S3
func (s *S3Backend) LoadClusterState(clusterName string) (*K3sClusterState, error) {
	ctx := context.Background()
	
	// Load config file
	configKey := s.getKey(fmt.Sprintf("clusters/%s/config.json", clusterName))
	configData, err := s.getObject(ctx, configKey)
	if err != nil {
		return nil, fmt.Errorf("cluster %s config not found: %w", clusterName, err)
	}
	
	var state K3sClusterState
	
	// Load config
	var configState map[string]interface{}
	if err := json.Unmarshal(configData, &configState); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}
	
	// Extract cluster from config
	if clusterData, ok := configState["cluster"].(map[string]interface{}); ok {
		clusterJSON, _ := json.Marshal(clusterData)
		json.Unmarshal(clusterJSON, &state.Cluster)
	}
	
	// Try to load status (optional - may not exist for new clusters)
	statusKey := s.getKey(fmt.Sprintf("clusters/%s/status.json", clusterName))
	statusData, statusErr := s.getObject(ctx, statusKey)
	
	if statusErr == nil {
		var statusState map[string]interface{}
		if err := json.Unmarshal(statusData, &statusState); err == nil {
			// Merge status into cluster
			if statusCluster, ok := statusState["cluster"].(map[string]interface{}); ok {
				if status, ok := statusCluster["status"].(string); ok {
					state.Cluster.Status = models.ClusterStatus(status)
				}
				if updatedAt, ok := statusCluster["updated_at"].(string); ok {
					if t, err := time.Parse(time.RFC3339, updatedAt); err == nil {
						state.Cluster.UpdatedAt = t
					}
				}
			}
			
			// Extract instance IDs and metadata
			if instanceIDs, ok := statusState["instance_ids"].(map[string]interface{}); ok {
				state.InstanceIDs = make(map[string]string)
				for k, v := range instanceIDs {
					if id, ok := v.(string); ok {
						state.InstanceIDs[k] = id
					}
				}
			}
			
			if metadata, ok := statusState["metadata"].(map[string]interface{}); ok {
				state.Metadata = metadata
			}
		}
	}
	
	return &state, nil
}

// LoadAllClusterStates loads all cluster states from S3
// Only supports new format: clusters/{name}/config.json + status.json
func (s *S3Backend) LoadAllClusterStates() ([]*K3sClusterState, error) {
	// List all cluster files
	keys, err := s.listObjects(context.Background(), "clusters/")
	if err != nil {
		return nil, err
	}

	var states []*K3sClusterState
	processedClusters := make(map[string]bool)
	
	for _, key := range keys {
		// Only process config.json files
		if !strings.HasSuffix(key, "/config.json") {
			continue
		}
		
		// Extract cluster name from key: clusters/{name}/config.json
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
// Only deletes new format files
func (s *S3Backend) DeleteClusterState(clusterName string) error {
	ctx := context.Background()

	// Delete config and status files
	configKey := s.getKey(fmt.Sprintf("clusters/%s/config.json", clusterName))
	s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucketName),
		Key:    aws.String(configKey),
	})
	
	statusKey := s.getKey(fmt.Sprintf("clusters/%s/status.json", clusterName))
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
	return s.putObject(context.Background(), key, data)
}

// LoadJob loads a job from S3
func (s *S3Backend) LoadJob(jobID string) (*models.Job, error) {
	key := s.getKey(fmt.Sprintf("jobs/%s.json", jobID))
	data, err := s.getObject(context.Background(), key)
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
			data, err := s.getObject(context.Background(), s.getKey(key))
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
	key := s.getKey(fmt.Sprintf("jobs/%s.json", jobID))
	return s.deleteObject(context.Background(), key)
}

// SaveConfig saves application configuration to S3
func (s *S3Backend) SaveConfig(config map[string]interface{}) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}

	key := s.getKey("config.json")
	return s.putObject(context.Background(), key, data)
}

// LoadConfig loads application configuration from S3
func (s *S3Backend) LoadConfig() (map[string]interface{}, error) {
	key := s.getKey("config.json")
	data, err := s.getObject(context.Background(), key)
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
