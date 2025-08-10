package aws

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/madhouselabs/goman/pkg/controller"
	"github.com/madhouselabs/goman/pkg/models"
	"github.com/madhouselabs/goman/pkg/storage"
)

// LambdaEvent represents the incoming Lambda event
type LambdaEvent struct {
	ClusterName string `json:"cluster_name"`
	Action      string `json:"action"`
}

// LambdaHandler wraps the reconciler for AWS Lambda
type LambdaHandler struct {
	reconciler *controller.Reconciler
	storage    *storage.Storage
	provider   *AWSProvider
}

// NewLambdaHandler creates a new Lambda handler
func NewLambdaHandler() (*LambdaHandler, error) {
	log.Println("Creating Lambda handler...")

	// Create AWS provider directly (we're in AWS Lambda environment)
	log.Println("Creating AWS provider...")
	prov, err := NewProvider("", "") // Will use defaults from environment
	if err != nil {
		log.Printf("Failed to create provider: %v", err)
		return nil, fmt.Errorf("failed to create provider: %w", err)
	}
	log.Println("Provider created successfully")

	// Initialize provider services
	ctx := context.Background()

	// The lock service table should already exist (created during setup)
	// Initialize will check if table exists and return early without trying to create it
	if err := prov.GetLockService().Initialize(ctx); err != nil {
		// Log the error but continue - the table should exist
		log.Printf("Warning: Lock service initialization had an issue (table should exist): %v", err)
	}

	// Initialize storage service
	if err := prov.GetStorageService().Initialize(ctx); err != nil {
		return nil, fmt.Errorf("failed to initialize storage service: %w", err)
	}

	// Initialize notification service
	if err := prov.GetNotificationService().Initialize(ctx); err != nil {
		log.Printf("Warning: failed to initialize notification service: %v", err)
	}

	// Note: Compute service initialization (SSM instance profile creation) is done
	// during local setup, not in Lambda, as Lambda shouldn't have IAM create permissions

	// Generate unique owner ID for this Lambda instance
	owner := fmt.Sprintf("lambda-%s-%d", prov.Region(), time.Now().UnixNano())

	// Create reconciler
	reconciler, err := controller.NewReconciler(prov, owner)
	if err != nil {
		return nil, fmt.Errorf("failed to create reconciler: %w", err)
	}

	// Create storage for accessing cluster states
	stor, err := storage.NewStorage()
	if err != nil {
		return nil, fmt.Errorf("failed to create storage: %w", err)
	}

	return &LambdaHandler{
		reconciler: reconciler,
		storage:    stor,
		provider:   prov,
	}, nil
}

// HandleRequest processes Lambda events
func (h *LambdaHandler) HandleRequest(ctx context.Context, event json.RawMessage) (*models.ReconcileResult, error) {
	log.Printf("Received event: %s", string(event))

	// Try to parse as direct event first
	var lambdaEvent LambdaEvent
	if err := json.Unmarshal(event, &lambdaEvent); err == nil && lambdaEvent.ClusterName != "" {
		// Direct invocation with cluster name
		return h.reconciler.ReconcileCluster(ctx, lambdaEvent.ClusterName)
	}

	// Try to parse as S3 event
	var s3Event S3Event
	if err := json.Unmarshal(event, &s3Event); err == nil && len(s3Event.Records) > 0 {
		// S3 event trigger
		for _, record := range s3Event.Records {
			if record.S3.Object.Key != "" {
				// Extract cluster name from S3 key
				// Expected format: clusters/{cluster-name}.json
				clusterName := extractClusterName(record.S3.Object.Key)
				if clusterName != "" {
					log.Printf("Processing S3 event for cluster: %s", clusterName)
					return h.reconciler.ReconcileCluster(ctx, clusterName)
				}
			}
		}
	}

	// Try to parse as EventBridge EC2 event
	var ec2Event EC2StateChangeEvent
	if err := json.Unmarshal(event, &ec2Event); err == nil && ec2Event.DetailType == "EC2 Instance State-change Notification" {
		// EC2 instance state change event
		instanceID := ec2Event.Detail.InstanceID
		state := ec2Event.Detail.State
		region := ec2Event.Region

		log.Printf("Processing EC2 state change event: instance %s in region %s changed to %s", instanceID, region, state)

		// Get the cluster owner from instance tags
		clusterName, err := h.getClusterFromInstanceTags(ctx, instanceID, region)
		if err != nil {
			log.Printf("Failed to get cluster from instance tags: %v", err)
			return nil, err
		}

		if clusterName == "" {
			log.Printf("Instance %s has no Cluster tag, ignoring state change to %s", instanceID, state)
			return &models.ReconcileResult{}, nil
		}

		log.Printf("Instance %s belongs to cluster %s (state: %s), triggering reconciliation",
			instanceID, clusterName, state)
		return h.reconciler.ReconcileCluster(ctx, clusterName)
	}

	return nil, fmt.Errorf("invalid event format or missing cluster name")
}

// S3Event represents an S3 event notification
type S3Event struct {
	Records []S3EventRecord `json:"Records"`
}

// EC2StateChangeEvent represents an EventBridge EC2 state change event
type EC2StateChangeEvent struct {
	DetailType string `json:"detail-type"`
	Region     string `json:"region"`
	Detail     struct {
		InstanceID string `json:"instance-id"`
		State      string `json:"state"`
	} `json:"detail"`
}

// S3EventRecord represents a single S3 event record
type S3EventRecord struct {
	S3 struct {
		Bucket struct {
			Name string `json:"name"`
		} `json:"bucket"`
		Object struct {
			Key string `json:"key"`
		} `json:"object"`
	} `json:"s3"`
}

// extractClusterName extracts cluster name from S3 object key
func extractClusterName(key string) string {
	// Handle the standard format: clusters/{cluster-name}.json
	// Example: clusters/k3s-cluster.json -> k3s-cluster
	if strings.HasPrefix(key, "clusters/") && strings.HasSuffix(key, ".json") {
		start := len("clusters/")
		end := len(key) - len(".json")
		if start < end {
			return key[start:end]
		}
	}

	return ""
}

// getClusterFromInstanceTags queries EC2 to get the Cluster tag value from an instance
func (h *LambdaHandler) getClusterFromInstanceTags(ctx context.Context, instanceID, region string) (string, error) {
	// Get EC2 client for the specific region
	computeService := h.provider.GetComputeService().(*ComputeService)
	ec2Client := computeService.getEC2Client(region)

	// Describe the instance to get its tags
	result, err := ec2Client.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
		InstanceIds: []string{instanceID},
	})
	if err != nil {
		return "", fmt.Errorf("failed to describe instance %s in region %s: %w", instanceID, region, err)
	}

	// Check if we found the instance
	if len(result.Reservations) == 0 || len(result.Reservations[0].Instances) == 0 {
		return "", fmt.Errorf("instance %s not found in region %s", instanceID, region)
	}

	instance := result.Reservations[0].Instances[0]

	// Look for the Cluster tag
	for _, tag := range instance.Tags {
		if tag.Key != nil && *tag.Key == "Cluster" && tag.Value != nil {
			return *tag.Value, nil
		}
	}

	// No Cluster tag found
	return "", nil
}

// StartLambdaHandler starts the Lambda handler
func StartLambdaHandler() {
	log.Println("Starting Lambda handler...")

	// Add panic recovery
	defer func() {
		if r := recover(); r != nil {
			log.Printf("PANIC in StartLambdaHandler: %v", r)
			panic(r)
		}
	}()

	handler, err := NewLambdaHandler()
	if err != nil {
		log.Printf("ERROR: Failed to create handler: %v", err)
		// Don't use log.Fatal as it calls os.Exit(1) immediately
		// Instead, let the error propagate properly
		panic(fmt.Sprintf("Failed to create handler: %v", err))
	}

	log.Println("Handler created, starting Lambda runtime...")
	lambda.Start(handler.HandleRequest)
	log.Println("Lambda.Start returned (should not happen)")
}
