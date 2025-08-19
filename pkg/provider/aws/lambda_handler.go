package aws

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
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
	sqsClient  *sqs.Client
	queueURL   string
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

	if err := prov.GetLockService().Initialize(ctx); err != nil {
		log.Printf("Warning: Lock service initialization error: %v", err)
	}

	// Initialize storage service
	if err := prov.GetStorageService().Initialize(ctx); err != nil {
		return nil, fmt.Errorf("failed to initialize storage service: %w", err)
	}

	if err := prov.GetNotificationService().Initialize(ctx); err != nil {
		log.Printf("Warning: Notification service initialization error: %v", err)
	}

	owner := fmt.Sprintf("lambda-%s-%d", prov.Region(), time.Now().UnixNano())

	// Create reconciler
	reconciler, err := controller.NewReconciler(prov, owner)
	if err != nil {
		return nil, fmt.Errorf("failed to create reconciler: %w", err)
	}

	stor, err := storage.NewStorageWithProvider(prov)
	if err != nil {
		return nil, fmt.Errorf("failed to create storage: %w", err)
	}

	// Initialize SQS client for requeue functionality
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	sqsClient := sqs.NewFromConfig(cfg)
	
	// Get queue URL from environment variable
	queueURL := os.Getenv("RECONCILE_QUEUE_URL")
	if queueURL == "" {
		log.Println("Warning: RECONCILE_QUEUE_URL not set, requeue functionality will be disabled")
	}

	return &LambdaHandler{
		reconciler: reconciler,
		storage:    stor,
		provider:   prov,
		sqsClient:  sqsClient,
		queueURL:   queueURL,
	}, nil
}

// HandleRequest processes Lambda events
func (h *LambdaHandler) HandleRequest(ctx context.Context, event json.RawMessage) (*models.ReconcileResult, error) {
	log.Printf("Received event: %s", string(event))

	// Store the cluster name for requeue if needed
	var clusterName string
	var result *models.ReconcileResult
	var err error

	// Use a separate block to avoid goto jumping over declarations
	{
		// Check for direct Lambda event
		var lambdaEvent LambdaEvent
		if err := json.Unmarshal(event, &lambdaEvent); err == nil && lambdaEvent.ClusterName != "" {
			clusterName = lambdaEvent.ClusterName
			result, err = h.reconciler.ReconcileCluster(ctx, clusterName)
			goto handleRequeue
		}

		// Check for SQS event
		var sqsEvent SQSEvent
		if err := json.Unmarshal(event, &sqsEvent); err == nil && len(sqsEvent.Records) > 0 {
			for _, record := range sqsEvent.Records {
				// Parse the SQS message body
				var requeueMsg RequeueMessage
				if err := json.Unmarshal([]byte(record.Body), &requeueMsg); err == nil && requeueMsg.ClusterName != "" {
					log.Printf("Processing SQS requeue event for cluster: %s", requeueMsg.ClusterName)
					clusterName = requeueMsg.ClusterName
					result, err = h.reconciler.ReconcileCluster(ctx, clusterName)
					goto handleRequeue
				}
			}
		}

		// Check for S3 event
		var s3Event S3Event
		if err := json.Unmarshal(event, &s3Event); err == nil && len(s3Event.Records) > 0 {
			for _, record := range s3Event.Records {
				if record.S3.Object.Key != "" {
					clusterName = extractClusterName(record.S3.Object.Key)
					if clusterName != "" {
						log.Printf("Processing S3 event for cluster: %s", clusterName)
						result, err = h.reconciler.ReconcileCluster(ctx, clusterName)
						goto handleRequeue
					}
				}
			}
		}

		// Check for EC2 state change event
		var ec2Event EC2StateChangeEvent
		if err := json.Unmarshal(event, &ec2Event); err == nil && ec2Event.DetailType == "EC2 Instance State-change Notification" {
			instanceID := ec2Event.Detail.InstanceID
			state := ec2Event.Detail.State
			region := ec2Event.Region

			log.Printf("Processing EC2 state change event: instance %s in region %s changed to %s", instanceID, region, state)

			clusterName, err = h.getClusterFromInstanceTags(ctx, instanceID, region)
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
			result, err = h.reconciler.ReconcileCluster(ctx, clusterName)
			goto handleRequeue
		}

		return nil, fmt.Errorf("invalid event format or missing cluster name")
	}

handleRequeue:
	// If reconciliation succeeded and requeue is requested, schedule next reconciliation
	if err == nil && result != nil && result.Requeue && clusterName != "" {
		if requeueErr := h.scheduleRequeue(ctx, clusterName, result.RequeueAfter); requeueErr != nil {
			log.Printf("Failed to schedule requeue for cluster %s: %v", clusterName, requeueErr)
		}
	}

	return result, err
}

// SQSEvent represents an SQS event notification
type SQSEvent struct {
	Records []SQSRecord `json:"Records"`
}

// SQSRecord represents a single SQS message
type SQSRecord struct {
	Body string `json:"body"`
}

// RequeueMessage represents a requeue request message
type RequeueMessage struct {
	ClusterName string `json:"cluster_name"`
	Attempt     int    `json:"attempt"`
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
	// Only handle new format: clusters/{cluster-name}/config.yaml or clusters/{cluster-name}/status.yaml
	
	if strings.HasPrefix(key, "clusters/") {
		path := key[len("clusters/"):]
		
		// Check for new format: {cluster-name}/config.yaml or {cluster-name}/status.yaml
		if strings.Contains(path, "/") {
			parts := strings.Split(path, "/")
			if len(parts) == 2 && (parts[1] == "config.yaml" || parts[1] == "status.yaml") {
				return parts[0]
			}
		}
	}

	return ""
}

// getClusterFromInstanceTags queries EC2 to get the Cluster tag value from an instance
func (h *LambdaHandler) getClusterFromInstanceTags(ctx context.Context, instanceID, region string) (string, error) {
	computeService := h.provider.GetComputeService().(*ComputeService)
	ec2Client := computeService.getEC2Client(region)

	result, err := ec2Client.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
		InstanceIds: []string{instanceID},
	})
	if err != nil {
		return "", fmt.Errorf("failed to describe instance %s in region %s: %w", instanceID, region, err)
	}

	if len(result.Reservations) == 0 || len(result.Reservations[0].Instances) == 0 {
		return "", fmt.Errorf("instance %s not found in region %s", instanceID, region)
	}

	instance := result.Reservations[0].Instances[0]

	for _, tag := range instance.Tags {
		if tag.Key != nil && *tag.Key == "Cluster" && tag.Value != nil {
			return *tag.Value, nil
		}
	}

	return "", nil
}

// scheduleRequeue schedules a requeue message to SQS with delay
func (h *LambdaHandler) scheduleRequeue(ctx context.Context, clusterName string, requeueAfter time.Duration) error {
	if h.queueURL == "" {
		log.Printf("Cannot schedule requeue: RECONCILE_QUEUE_URL not configured")
		return nil // Not an error, just skip requeue
	}

	// First check if the cluster still exists before scheduling requeue
	configKey := fmt.Sprintf("clusters/%s/config.yaml", clusterName)
	_, err := h.provider.GetStorageService().GetObject(ctx, configKey)
	if err != nil {
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "NoSuchKey") {
			log.Printf("Cluster %s no longer exists, not scheduling requeue", clusterName)
			return nil // Cluster deleted, don't requeue
		}
		// Some other error checking existence, log but continue with requeue
		log.Printf("Warning: could not check cluster %s existence: %v", clusterName, err)
	}

	// Calculate delay in seconds (SQS supports 0-900 seconds)
	delaySeconds := int32(requeueAfter.Seconds())
	if delaySeconds > 900 {
		delaySeconds = 900 // Max SQS delay
	}
	if delaySeconds < 0 {
		delaySeconds = 0
	}

	// Create requeue message
	requeueMsg := RequeueMessage{
		ClusterName: clusterName,
		Attempt:     1, // Could track attempts if needed
	}

	msgBody, err := json.Marshal(requeueMsg)
	if err != nil {
		return fmt.Errorf("failed to marshal requeue message: %w", err)
	}

	// Send message to SQS with delay
	_, err = h.sqsClient.SendMessage(ctx, &sqs.SendMessageInput{
		QueueUrl:     aws.String(h.queueURL),
		MessageBody:  aws.String(string(msgBody)),
		DelaySeconds: delaySeconds,
		MessageAttributes: map[string]types.MessageAttributeValue{
			"ClusterName": {
				DataType:    aws.String("String"),
				StringValue: aws.String(clusterName),
			},
		},
	})

	if err != nil {
		return fmt.Errorf("failed to send requeue message to SQS: %w", err)
	}

	log.Printf("Scheduled requeue for cluster %s in %d seconds", clusterName, delaySeconds)
	return nil
}

// StartLambdaHandler starts the Lambda handler
func StartLambdaHandler() {
	log.Println("Starting Lambda handler...")


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
