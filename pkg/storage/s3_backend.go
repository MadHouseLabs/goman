package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

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
		cfg, err = config.LoadDefaultConfig(context.TODO(),
			config.WithRegion(region),
			config.WithSharedConfigProfile(profile),
		)
	} else {
		// Use default credentials chain (Lambda IAM role, EC2 instance role, etc.)
		cfg, err = config.LoadDefaultConfig(context.TODO(),
			config.WithRegion(region),
		)
	}
	
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Get AWS account ID
	stsClient := sts.NewFromConfig(cfg)
	identity, err := stsClient.GetCallerIdentity(context.TODO(), &sts.GetCallerIdentityInput{})
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

// Initialize creates the S3 bucket if it doesn't exist and migrates old data
func (s *S3Backend) Initialize() error {
	ctx := context.TODO()

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
			// Check if bucket already exists (race condition or cross-region)
			if !strings.Contains(createErr.Error(), "BucketAlreadyExists") && 
			   !strings.Contains(createErr.Error(), "BucketAlreadyOwnedByYou") {
				return fmt.Errorf("failed to create S3 bucket %s in region %s: %w", s.bucketName, s.region, createErr)
			}
			// Bucket exists, that's fine
			// Bucket already exists, continue silently
		} else {
			// Successfully created bucket without versioning
		}
	} else {
		// Bucket exists, continue silently
	}
	
	// No migration needed - we're using individual files per cluster
	
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
func (s *S3Backend) putObject(key string, data []byte) error {
	ctx := context.TODO()
	
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucketName),
		Key:         aws.String(key),
		Body:        bytes.NewReader(data),
		ContentType: aws.String("application/json"),
	})
	
	return err
}

// getObject downloads data from S3
func (s *S3Backend) getObject(key string) ([]byte, error) {
	ctx := context.TODO()
	
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
func (s *S3Backend) deleteObject(key string) error {
	ctx := context.TODO()
	
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucketName),
		Key:    aws.String(key),
	})
	
	return err
}

// listObjects lists objects with a given prefix
func (s *S3Backend) listObjects(prefix string) ([]string, error) {
	ctx := context.TODO()
	
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
	return s.putObject(key, data)
}

// LoadClusters loads k3s clusters from S3
func (s *S3Backend) LoadClusters() ([]models.K3sCluster, error) {
	key := s.getKey("clusters/clusters.json")
	data, err := s.getObject(key)
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
	if err := s.putObject(key, data); err != nil {
		return fmt.Errorf("failed to save cluster state: %w", err)
	}
	
	return nil
}

// LoadClusterState loads complete cluster state from S3
func (s *S3Backend) LoadClusterState(clusterName string) (*K3sClusterState, error) {
	// Load cluster state by name: clusters/{cluster-name}.json
	key := s.getKey(fmt.Sprintf("clusters/%s.json", clusterName))
	data, err := s.getObject(key)
	if err != nil {
		return nil, err
	}
	
	var state K3sClusterState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	
	return &state, nil
}

// LoadAllClusterStates loads all cluster states from S3
// Loads from individual cluster files
func (s *S3Backend) LoadAllClusterStates() ([]*K3sClusterState, error) {
	// List all cluster files in the new location
	// Note: prefix is already "state/default", so we just need "clusters/"
	keys, err := s.listObjects("clusters/")
	if err != nil {
		return nil, err
	}
	
	var states []*K3sClusterState
	for _, key := range keys {
		// Only process JSON files
		if !strings.HasSuffix(key, ".json") {
			continue
		}
		
		// Load individual cluster state
		// Note: listObjects returns keys without prefix, but getObject needs the full key
		fullKey := s.getKey(key)
		data, err := s.getObject(fullKey)
		if err != nil {
			continue // Skip files that can't be read
		}
		
		var state K3sClusterState
		if err := json.Unmarshal(data, &state); err != nil {
			continue // Skip malformed files
		}
		
		states = append(states, &state)
	}
	
	return states, nil
}

// DeleteClusterState deletes cluster state from S3
// Deletes the individual cluster file
func (s *S3Backend) DeleteClusterState(clusterName string) error {
	ctx := context.TODO()
	
	// Delete the individual cluster file
	key := s.getKey(fmt.Sprintf("clusters/%s.json", clusterName))
	
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucketName),
		Key:    aws.String(key),
	})
	
	return err
}

// saveAllClusterStates is a helper to save all states to single file
func (s *S3Backend) saveAllClusterStates(states []*K3sClusterState) error {
	data, err := json.MarshalIndent(states, "", "  ")
	if err != nil {
		return err
	}
	
	key := s.getKey("clusters.json")
	return s.putObject(key, data)
}

// migrateToSingleFile migrates old individual cluster files to single file
func (s *S3Backend) migrateToSingleFile() error {
	// Check if single file already exists
	_, err := s.getObject(s.getKey("clusters.json"))
	if err == nil {
		// Single file exists, no migration needed
		return nil
	}
	
	// Check for old individual files
	keys, err := s.listObjects("clusters/")
	if err != nil {
		return nil // No old files, nothing to migrate
	}
	
	var states []*K3sClusterState
	var hasOldFiles bool
	
	for _, key := range keys {
		if strings.HasSuffix(key, ".state.json") {
			hasOldFiles = true
			data, err := s.getObject(s.getKey(key))
			if err != nil {
				continue
			}
			
			var state K3sClusterState
			if err := json.Unmarshal(data, &state); err != nil {
				continue
			}
			states = append(states, &state)
		}
	}
	
	// If we found old files, save them to the new single file
	if hasOldFiles && len(states) > 0 {
		if err := s.saveAllClusterStates(states); err != nil {
			return fmt.Errorf("failed to save migrated data: %w", err)
		}
		
		// Optionally delete old files after successful migration
		for _, key := range keys {
			if strings.HasSuffix(key, ".state.json") {
				_ = s.deleteObject(s.getKey(key)) // Ignore delete errors
			}
		}
	}
	
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
	return s.putObject(key, data)
}

// LoadJob loads a job from S3
func (s *S3Backend) LoadJob(jobID string) (*models.Job, error) {
	key := s.getKey(fmt.Sprintf("jobs/%s.json", jobID))
	data, err := s.getObject(key)
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
	keys, err := s.listObjects("jobs/")
	if err != nil {
		return nil, err
	}
	
	var jobs []*models.Job
	for _, key := range keys {
		if strings.HasSuffix(key, ".json") {
			data, err := s.getObject(s.getKey(key))
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
	return s.deleteObject(key)
}

// SaveConfig saves application configuration to S3
func (s *S3Backend) SaveConfig(config map[string]interface{}) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	
	key := s.getKey("config.json")
	return s.putObject(key, data)
}

// LoadConfig loads application configuration from S3
func (s *S3Backend) LoadConfig() (map[string]interface{}, error) {
	key := s.getKey("config.json")
	data, err := s.getObject(key)
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