package aws

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/madhouselabs/goman/pkg/controller"
	"github.com/madhouselabs/goman/pkg/models"
)

// LambdaEvent represents the incoming Lambda event
type LambdaEvent struct {
	ClusterName string `json:"cluster_name"`
	Action      string `json:"action"`
}

// LambdaHandler wraps the reconciler for AWS Lambda
type LambdaHandler struct {
	reconciler *controller.Reconciler
}

// NewLambdaHandler creates a new Lambda handler
func NewLambdaHandler() (*LambdaHandler, error) {
	log.Println("Creating Lambda handler...")
	
	// Create AWS provider directly (we're in AWS Lambda environment)
	log.Println("Creating AWS provider...")
	prov, err := NewProvider("", "")  // Will use defaults from environment
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
	
	return &LambdaHandler{
		reconciler: reconciler,
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
		
		log.Printf("Processing EC2 event: instance %s is now %s", instanceID, state)
		
		// Find which cluster this instance belongs to by checking tags
		// For now, trigger reconciliation for all clusters (Lambda will figure out which one needs updating)
		// In production, you'd want to be more targeted
		log.Printf("EC2 instance %s changed state to %s, triggering cluster reconciliation", instanceID, state)
		
		// We don't know which specific cluster, so we'll need to check all
		// This is a limitation that could be improved by storing instance-to-cluster mapping
		return &models.ReconcileResult{
			Requeue:      true,
			RequeueAfter: 5 * time.Second,
		}, nil
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