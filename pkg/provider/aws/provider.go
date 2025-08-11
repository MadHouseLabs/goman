package aws

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/eventbridge"
	eventbridgetypes "github.com/aws/aws-sdk-go-v2/service/eventbridge/types"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	lambdatypes "github.com/aws/aws-sdk-go-v2/service/lambda/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/madhouselabs/goman/pkg/logger"
	"github.com/madhouselabs/goman/pkg/provider"
)

// AWSProvider implements the Provider interface for AWS
type AWSProvider struct {
	profile   string
	region    string
	accountID string
	cfg       aws.Config

	// Services
	lockService         provider.LockService
	storageService      provider.StorageService
	notificationService provider.NotificationService
	functionService     provider.FunctionService
	computeService      provider.ComputeService

	// AWS clients
	dynamoClient *dynamodb.Client
	s3Client     *s3.Client
	snsClient    *sns.Client
	sqsClient    *sqs.Client
	lambdaClient *lambda.Client
	ec2Client    *ec2.Client
	stsClient    *sts.Client
	iamClient    *iam.Client
}


// NewProvider creates a new AWS provider
func NewProvider(profile, region string) (*AWSProvider, error) {
	// Create a context for initialization
	ctx := context.Background()

	logger.Printf("NewProvider called with profile='%s', region='%s'", profile, region)

	if region == "" {
		region = "ap-south-1" // Default to Mumbai
		logger.Printf("Using default region: %s", region)
	}

	// Load AWS config
	var cfgOptions []func(*config.LoadOptions) error
	cfgOptions = append(cfgOptions, config.WithRegion(region))

	// Only add profile if it's not empty (for Lambda environment)
	if profile != "" {
		logger.Printf("Adding profile to config: %s", profile)
		cfgOptions = append(cfgOptions, config.WithSharedConfigProfile(profile))
	} else {
		logger.Println("No profile specified, using default credentials chain")
	}

	logger.Println("Loading AWS config...")
	cfg, err := config.LoadDefaultConfig(ctx, cfgOptions...)
	if err != nil {
		logger.Printf("Failed to load AWS config: %v", err)
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}
	logger.Println("AWS config loaded successfully")

	// Get account ID
	logger.Println("Getting AWS account ID...")
	stsClient := sts.NewFromConfig(cfg)
	identity, err := stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		logger.Printf("Failed to get AWS account ID: %v", err)
		return nil, fmt.Errorf("failed to get AWS account ID: %w", err)
	}
	logger.Printf("AWS account ID: %s", *identity.Account)

	p := &AWSProvider{
		profile:      profile,
		region:       region,
		accountID:    *identity.Account,
		cfg:          cfg,
		dynamoClient: dynamodb.NewFromConfig(cfg),
		s3Client:     s3.NewFromConfig(cfg),
		snsClient:    sns.NewFromConfig(cfg),
		sqsClient:    sqs.NewFromConfig(cfg),
		lambdaClient: lambda.NewFromConfig(cfg),
		ec2Client:    ec2.NewFromConfig(cfg),
		stsClient:    stsClient,
		iamClient:    iam.NewFromConfig(cfg),
	}

	// Initialize services
	p.lockService = NewLockService(p.dynamoClient, p.accountID)
	p.storageService = NewStorageService(p.s3Client, p.accountID)
	p.notificationService = NewNotificationService(p.snsClient, p.sqsClient, p.accountID, p.region)
	p.functionService = NewFunctionService(p.lambdaClient, p.s3Client, p.iamClient, p.accountID, p.region)
	p.computeService = NewComputeService(p.ec2Client, p.iamClient, p.cfg)

	return p, nil
}

// GetLockService returns the lock service
func (p *AWSProvider) GetLockService() provider.LockService {
	return p.lockService
}

// GetStorageService returns the storage service
func (p *AWSProvider) GetStorageService() provider.StorageService {
	return p.storageService
}

// GetNotificationService returns the notification service
func (p *AWSProvider) GetNotificationService() provider.NotificationService {
	return p.notificationService
}

// GetFunctionService returns the function service
func (p *AWSProvider) GetFunctionService() provider.FunctionService {
	return p.functionService
}

// GetComputeService returns the compute service
func (p *AWSProvider) GetComputeService() provider.ComputeService {
	return p.computeService
}

// Name returns the provider name
func (p *AWSProvider) Name() string {
	return "aws"
}

// Region returns the provider region
func (p *AWSProvider) Region() string {
	return p.region
}

// AccountID returns the AWS account ID
func (p *AWSProvider) AccountID() string {
	return p.accountID
}

// GetEC2Client returns the EC2 client for direct AWS operations
func (p *AWSProvider) GetEC2Client() *ec2.Client {
	return p.ec2Client
}

// GetS3Client returns the S3 client for direct AWS operations
func (p *AWSProvider) GetS3Client() *s3.Client {
	return p.s3Client
}

// GetLambdaClient returns the Lambda client for direct AWS operations
func (p *AWSProvider) GetLambdaClient() *lambda.Client {
	return p.lambdaClient
}

// GetConfig returns the AWS config
func (p *AWSProvider) GetConfig() aws.Config {
	return p.cfg
}

// CleanupClusterResources implements the ClusterCleaner interface
func (p *AWSProvider) CleanupClusterResources(ctx context.Context, clusterName string) error {
	logger.Printf("Cleaning up AWS resources for cluster %s", clusterName)

	// Check if there are any instances still running
	instances, err := p.computeService.ListInstances(ctx, map[string]string{
		"tag:Cluster":         clusterName,
		"instance-state-name": "running,pending,stopping,stopped",
	})

	if err == nil && len(instances) > 0 {
		logger.Printf("Cannot cleanup: %d instances still exist for cluster %s", len(instances), clusterName)
		return fmt.Errorf("cannot cleanup: instances still exist")
	}

	// Clean up cluster-specific security groups
	// Look for all possible patterns: goman-{cluster}-sg, goman-{cluster}-*-sg
	patterns := []string{
		fmt.Sprintf("goman-%s-sg", clusterName),
		fmt.Sprintf("goman-%s-*", clusterName), // Catch any variations
	}
	
	for _, pattern := range patterns {
		describeSGOutput, err := p.ec2Client.DescribeSecurityGroups(ctx, &ec2.DescribeSecurityGroupsInput{
			Filters: []types.Filter{
				{
					Name:   aws.String("group-name"),
					Values: []string{pattern},
				},
			},
		})

		if err == nil && len(describeSGOutput.SecurityGroups) > 0 {
			for _, sg := range describeSGOutput.SecurityGroups {
				sgName := aws.ToString(sg.GroupName)
				sgID := sg.GroupId

				// Try to delete the security group
				_, err = p.ec2Client.DeleteSecurityGroup(ctx, &ec2.DeleteSecurityGroupInput{
					GroupId: sgID,
				})
				if err != nil {
					// Security group might still be in use, which is fine
					logger.Printf("Note: Security group %s not deleted (may be in use): %v", sgName, err)
				} else {
					logger.Printf("Deleted security group %s", sgName)
				}
			}
		}
	}


	logger.Printf("Cluster %s cleanup complete (preserving reusable resources)", clusterName)
	return nil
}

// Initialize sets up AWS infrastructure
func (p *AWSProvider) Initialize(ctx context.Context) (*provider.InitializeResult, error) {
	result := &provider.InitializeResult{
		ProviderType: "aws",
		Resources:    make(map[string]string),
		Errors:       []string{},
	}

	// Initialize storage service (S3)
	if err := p.storageService.Initialize(ctx); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("Storage: %v", err))
	} else {
		result.StorageReady = true
		result.Resources["s3_bucket"] = fmt.Sprintf("goman-%s", p.accountID)
	}

	// Initialize lock service (DynamoDB)
	if err := p.lockService.Initialize(ctx); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("LockService: %v", err))
	} else {
		result.LockServiceReady = true
		result.Resources["dynamodb_table"] = "goman-resource-locks"
	}

	// Initialize compute service (SSM instance profile)
	if computeService, ok := p.computeService.(*ComputeService); ok {
		if err := computeService.Initialize(ctx); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("Compute service: %v", err))
		} else {
			result.Resources["iam_role_ssm"] = "goman-ssm-instance-role"
		}
	}

	// Deploy function (Lambda)
	functionName := fmt.Sprintf("goman-controller-%s", p.accountID)
	packagePath := "build/lambda-aws-controller.zip"
	if err := p.functionService.DeployFunction(ctx, functionName, packagePath); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("Function: %v", err))
	} else {
		result.FunctionReady = true
		result.Resources["lambda_function"] = functionName
		
		// Wait for Lambda to be fully ready before setting up notifications
		// This avoids conflicts when Lambda is updating
		if err := p.waitForLambdaReady(ctx, functionName); err != nil {
			logger.Printf("Warning: Lambda may not be fully ready: %v", err)
		}
		
		// Set up S3 notifications for Lambda with retry
		retryCount := 3
		var lastErr error
		for i := 0; i < retryCount; i++ {
			if err := p.setupS3Notifications(ctx, functionName); err != nil {
				lastErr = err
				logger.Printf("Attempt %d: Failed to setup S3 notifications: %v", i+1, err)
				if i < retryCount-1 {
					time.Sleep(time.Duration(5*(i+1)) * time.Second) // Exponential backoff
				}
			} else {
				lastErr = nil
				break
			}
		}
		if lastErr != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("S3 notifications: %v", lastErr))
		}
		
		// Set up SQS queue for reconciliation requeue
		queueURL, err := p.setupSQSQueue(ctx, functionName)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("SQS queue: %v", err))
		} else {
			result.Resources["sqs_queue"] = queueURL
			logger.Printf("Created SQS queue for reconciliation: %s", queueURL)
		}
		
		// Set up EventBridge rule for EC2 state changes
		if err := p.setupEventBridgeRule(ctx, functionName); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("EventBridge rule: %v", err))
		}
	}

	// Auth is handled by IAM roles created during service initialization
	result.AuthReady = true
	result.Resources["iam_role_lambda"] = fmt.Sprintf("goman-lambda-role-%s", p.accountID)

	return result, nil
}

// Cleanup removes AWS infrastructure
func (p *AWSProvider) Cleanup(ctx context.Context) error {
	var errors []string
	
	bucketName := fmt.Sprintf("goman-%s", p.accountID)
	functionName := fmt.Sprintf("goman-controller-%s", p.accountID)
	tableName := "goman-resource-locks"
	lambdaRoleName := fmt.Sprintf("goman-lambda-role-%s", p.accountID)
	lambdaPolicyName := fmt.Sprintf("goman-lambda-policy-%s", p.accountID)
	ssmRoleName := "goman-ssm-instance-role"
	ssmProfileName := "goman-ssm-instance-profile"
	
	eventClient := eventbridge.NewFromConfig(p.cfg)
	ruleName := "goman-ec2-state-change-rule"
	
	eventClient.RemoveTargets(ctx, &eventbridge.RemoveTargetsInput{
		Rule: aws.String(ruleName),
		Ids:  []string{"1"},
	})
	
	eventClient.DeleteRule(ctx, &eventbridge.DeleteRuleInput{
		Name: aws.String(ruleName),
	})
	
	if err := p.functionService.DeleteFunction(ctx, functionName); err != nil {
		if !strings.Contains(err.Error(), "ResourceNotFoundException") {
			errors = append(errors, fmt.Sprintf("Lambda: %v", err))
		}
	}
	
	snsTopics := []string{
		"goman-cluster-events",
		"goman-reconcile-events",
		"goman-error-events",
	}
	for _, topicName := range snsTopics {
		topicArn := fmt.Sprintf("arn:aws:sns:%s:%s:%s", p.region, p.accountID, topicName)
		_, err := p.snsClient.DeleteTopic(ctx, &sns.DeleteTopicInput{
			TopicArn: aws.String(topicArn),
		})
		if err != nil && !strings.Contains(err.Error(), "NotFound") {
			errors = append(errors, fmt.Sprintf("SNS topic %s: %v", topicName, err))
		}
	}
	
	listOutput, err := p.s3Client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket: aws.String(bucketName),
	})
	if err == nil && listOutput.Contents != nil {
		for _, obj := range listOutput.Contents {
			p.s3Client.DeleteObject(ctx, &s3.DeleteObjectInput{
				Bucket: aws.String(bucketName),
				Key:    obj.Key,
			})
		}
	}
	_, err = p.s3Client.DeleteBucket(ctx, &s3.DeleteBucketInput{
		Bucket: aws.String(bucketName),
	})
	if err != nil && !strings.Contains(err.Error(), "NoSuchBucket") {
		errors = append(errors, fmt.Sprintf("S3: %v", err))
	}
	
	_, err = p.dynamoClient.DeleteTable(ctx, &dynamodb.DeleteTableInput{
		TableName: aws.String(tableName),
	})
	if err != nil && !strings.Contains(err.Error(), "ResourceNotFoundException") {
		errors = append(errors, fmt.Sprintf("DynamoDB: %v", err))
	}
	
	p.iamClient.RemoveRoleFromInstanceProfile(ctx, &iam.RemoveRoleFromInstanceProfileInput{
		InstanceProfileName: aws.String(ssmProfileName),
		RoleName:            aws.String(ssmRoleName),
	})
	
	p.iamClient.DeleteInstanceProfile(ctx, &iam.DeleteInstanceProfileInput{
		InstanceProfileName: aws.String(ssmProfileName),
	})
	
	p.iamClient.DetachRolePolicy(ctx, &iam.DetachRolePolicyInput{
		RoleName:  aws.String(ssmRoleName),
		PolicyArn: aws.String("arn:aws:iam::aws:policy/AmazonSSMManagedInstanceCore"),
	})
	p.iamClient.DeleteRole(ctx, &iam.DeleteRoleInput{
		RoleName: aws.String(ssmRoleName),
	})
	
	listAttached, err := p.iamClient.ListAttachedRolePolicies(ctx, &iam.ListAttachedRolePoliciesInput{
		RoleName: aws.String(lambdaRoleName),
	})
	if err == nil && listAttached.AttachedPolicies != nil {
		for _, policy := range listAttached.AttachedPolicies {
			p.iamClient.DetachRolePolicy(ctx, &iam.DetachRolePolicyInput{
				RoleName:  aws.String(lambdaRoleName),
				PolicyArn: policy.PolicyArn,
			})
		}
	}
	
	policyArn := fmt.Sprintf("arn:aws:iam::%s:policy/%s", p.accountID, lambdaPolicyName)
	listVersionsOutput, err := p.iamClient.ListPolicyVersions(ctx, &iam.ListPolicyVersionsInput{
		PolicyArn: aws.String(policyArn),
	})
	if err == nil && listVersionsOutput.Versions != nil {
		for _, version := range listVersionsOutput.Versions {
			if !version.IsDefaultVersion {
				p.iamClient.DeletePolicyVersion(ctx, &iam.DeletePolicyVersionInput{
					PolicyArn: aws.String(policyArn),
					VersionId: version.VersionId,
				})
			}
		}
	}
	p.iamClient.DeletePolicy(ctx, &iam.DeletePolicyInput{
		PolicyArn: aws.String(policyArn),
	})
	
	p.iamClient.DeleteRole(ctx, &iam.DeleteRoleInput{
		RoleName: aws.String(lambdaRoleName),
	})
	
	if len(errors) > 0 {
		return fmt.Errorf("some resources failed to delete: %s", strings.Join(errors, "; "))
	}
	
	return nil
}

// GetStatus checks the status of AWS infrastructure
func (p *AWSProvider) GetStatus(ctx context.Context) (*provider.InfrastructureStatus, error) {
	status := &provider.InfrastructureStatus{
		Resources: make(map[string]string),
	}

	bucketName := fmt.Sprintf("goman-%s", p.accountID)
	status.Resources["s3_bucket"] = bucketName
	status.StorageStatus = "ready"

	status.Resources["dynamodb_table"] = "goman-resource-locks"
	status.LockStatus = "ready"
	functionName := fmt.Sprintf("goman-controller-%s", p.accountID)
	status.Resources["lambda_function"] = functionName
	exists, _ := p.functionService.FunctionExists(ctx, functionName)
	if exists {
		status.FunctionStatus = "ready"
	} else {
		status.FunctionStatus = "not_deployed"
	}

	// Check IAM roles
	status.Resources["iam_role_lambda"] = fmt.Sprintf("goman-lambda-role-%s", p.accountID)
	status.Resources["iam_role_ssm"] = "goman-ssm-instance-role"
	status.AuthStatus = "ready"

	// Overall status
	status.Initialized = status.StorageStatus == "ready" &&
		status.FunctionStatus == "ready" &&
		status.LockStatus == "ready"

	return status, nil
}

// waitForLambdaReady waits for Lambda function to be in Active state
func (p *AWSProvider) waitForLambdaReady(ctx context.Context, functionName string) error {
	maxRetries := 30
	for i := 0; i < maxRetries; i++ {
		config, err := p.lambdaClient.GetFunctionConfiguration(ctx, &lambda.GetFunctionConfigurationInput{
			FunctionName: aws.String(functionName),
		})
		if err != nil {
			return fmt.Errorf("failed to get function configuration: %w", err)
		}
		
		if config.State == "Active" && config.LastUpdateStatus == "Successful" {
			return nil
		}
		
		if config.State == "Failed" || config.LastUpdateStatus == "Failed" {
			return fmt.Errorf("function is in failed state")
		}
		
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("timeout waiting for Lambda to be ready")
}

// setupS3Notifications configures S3 bucket notifications to trigger Lambda
func (p *AWSProvider) setupS3Notifications(ctx context.Context, functionName string) error {
	bucketName := fmt.Sprintf("goman-%s", p.accountID)
	
	// First check if notifications are already configured
	existingConfig, err := p.s3Client.GetBucketNotificationConfiguration(ctx, &s3.GetBucketNotificationConfigurationInput{
		Bucket: aws.String(bucketName),
	})
	if err == nil && existingConfig != nil {
		// Check if our notification already exists
		for _, config := range existingConfig.LambdaFunctionConfigurations {
			if config.Id != nil && *config.Id == "goman-cluster-changes" {
				logger.Printf("S3 notifications already configured for bucket %s", bucketName)
				return nil
			}
		}
	}
	
	// Add permission for S3 to invoke the Lambda function
	_, err = p.lambdaClient.AddPermission(ctx, &lambda.AddPermissionInput{
		FunctionName: aws.String(functionName),
		StatementId:  aws.String("s3-invoke-permission"),
		Action:       aws.String("lambda:InvokeFunction"),
		Principal:    aws.String("s3.amazonaws.com"),
		SourceArn:    aws.String(fmt.Sprintf("arn:aws:s3:::%s", bucketName)),
	})
	
	if err != nil {
		if !strings.Contains(err.Error(), "ResourceConflictException") {
			return fmt.Errorf("failed to add S3 invoke permission: %w", err)
		}
		// Permission already exists, which is fine
	}
	
	// Get function ARN
	functionConfig, err := p.lambdaClient.GetFunction(ctx, &lambda.GetFunctionInput{
		FunctionName: aws.String(functionName),
	})
	if err != nil {
		return fmt.Errorf("failed to get function configuration: %w", err)
	}
	
	// Configure S3 bucket notification for both old and new formats
	notificationConfig := &s3.PutBucketNotificationConfigurationInput{
		Bucket: aws.String(bucketName),
		NotificationConfiguration: &s3types.NotificationConfiguration{
			LambdaFunctionConfigurations: []s3types.LambdaFunctionConfiguration{
				{
					Id:                aws.String("goman-cluster-changes"),
					LambdaFunctionArn: functionConfig.Configuration.FunctionArn,
					Events: []s3types.Event{
						s3types.EventS3ObjectCreatedPut,
						s3types.EventS3ObjectCreatedPost,
						s3types.EventS3ObjectRemovedDelete,
					},
					Filter: &s3types.NotificationConfigurationFilter{
						Key: &s3types.S3KeyFilter{
							FilterRules: []s3types.FilterRule{
								{
									Name:  s3types.FilterRuleNamePrefix,
									Value: aws.String("clusters/"),
								},
								{
									Name:  s3types.FilterRuleNameSuffix,
									Value: aws.String(".json"),
								},
							},
						},
					},
				},
			},
		},
	}
	
	_, err = p.s3Client.PutBucketNotificationConfiguration(ctx, notificationConfig)
	if err != nil {
		return fmt.Errorf("failed to set up S3 bucket notification: %w", err)
	}
	
	return nil
}

// setupSQSQueue creates an SQS queue for reconciliation requeue
func (p *AWSProvider) setupSQSQueue(ctx context.Context, functionName string) (string, error) {
	sqsClient := sqs.NewFromConfig(p.cfg)
	queueName := fmt.Sprintf("goman-reconcile-queue-%s", p.accountID)
	
	// Create or get the queue
	createResult, err := sqsClient.CreateQueue(ctx, &sqs.CreateQueueInput{
		QueueName: aws.String(queueName),
		Attributes: map[string]string{
			"MessageRetentionPeriod": "3600",  // 1 hour
			"VisibilityTimeout":      "300",    // 5 minutes (match Lambda timeout)
			"MaximumMessageSize":     "262144", // 256 KB
		},
	})
	if err != nil {
		// If queue already exists, get its URL
		if strings.Contains(err.Error(), "QueueAlreadyExists") {
			getResult, getErr := sqsClient.GetQueueUrl(ctx, &sqs.GetQueueUrlInput{
				QueueName: aws.String(queueName),
			})
			if getErr != nil {
				return "", fmt.Errorf("failed to get existing queue URL: %w", getErr)
			}
			createResult = &sqs.CreateQueueOutput{
				QueueUrl: getResult.QueueUrl,
			}
		} else {
			return "", fmt.Errorf("failed to create SQS queue: %w", err)
		}
	}
	
	queueURL := *createResult.QueueUrl
	
	// Get queue ARN
	queueAttrs, err := sqsClient.GetQueueAttributes(ctx, &sqs.GetQueueAttributesInput{
		QueueUrl: aws.String(queueURL),
		AttributeNames: []sqstypes.QueueAttributeName{sqstypes.QueueAttributeNameAll},
	})
	if err != nil {
		return "", fmt.Errorf("failed to get queue ARN: %w", err)
	}
	
	queueArn := queueAttrs.Attributes["QueueArn"]
	
	// Check if mapping already exists
	listResult, err := p.lambdaClient.ListEventSourceMappings(ctx, &lambda.ListEventSourceMappingsInput{
		FunctionName: aws.String(functionName),
		EventSourceArn: aws.String(queueArn),
	})
	
	if err == nil && len(listResult.EventSourceMappings) == 0 {
		// Create new event source mapping
		_, err = p.lambdaClient.CreateEventSourceMapping(ctx, &lambda.CreateEventSourceMappingInput{
			EventSourceArn: aws.String(queueArn),
			FunctionName:   aws.String(functionName),
			BatchSize:      aws.Int32(1), // Process one message at a time
			Enabled:        aws.Bool(true),
		})
		if err != nil {
			return queueURL, fmt.Errorf("failed to create event source mapping: %w", err)
		}
		logger.Printf("Created SQS event source mapping for Lambda function %s", functionName)
	}
	
	// Update Lambda environment variables to include queue URL
	_, err = p.lambdaClient.UpdateFunctionConfiguration(ctx, &lambda.UpdateFunctionConfigurationInput{
		FunctionName: aws.String(functionName),
		Environment: &lambdatypes.Environment{
			Variables: map[string]string{
				"RECONCILE_QUEUE_URL": queueURL,
			},
		},
	})
	if err != nil {
		logger.Printf("Warning: Failed to update Lambda environment with queue URL: %v", err)
	}
	
	return queueURL, nil
}

// setupEventBridgeRule creates an EventBridge rule to trigger Lambda on EC2 instance state changes
func (p *AWSProvider) setupEventBridgeRule(ctx context.Context, functionName string) error {
	// Create EventBridge client
	eventClient := eventbridge.NewFromConfig(p.cfg)
	
	// Define the rule name
	ruleName := "goman-ec2-state-change-rule"
	
	// Create event pattern for ALL EC2 instance state changes
	eventPattern := map[string]interface{}{
		"source":      []string{"aws.ec2"},
		"detail-type": []string{"EC2 Instance State-change Notification"},
		// No state filter - we want ALL state changes
	}
	
	eventPatternJSON, err := json.Marshal(eventPattern)
	if err != nil {
		return fmt.Errorf("failed to marshal event pattern: %w", err)
	}
	
	// Create or update the rule
	_, err = eventClient.PutRule(ctx, &eventbridge.PutRuleInput{
		Name:         aws.String(ruleName),
		Description:  aws.String("Trigger Lambda on any EC2 instance state change for Goman cluster reconciliation"),
		EventPattern: aws.String(string(eventPatternJSON)),
		State:        eventbridgetypes.RuleStateEnabled,
	})
	if err != nil {
		return fmt.Errorf("failed to create EventBridge rule: %w", err)
	}
	
	// Get Lambda function ARN
	functionConfig, err := p.lambdaClient.GetFunction(ctx, &lambda.GetFunctionInput{
		FunctionName: aws.String(functionName),
	})
	if err != nil {
		return fmt.Errorf("failed to get Lambda function: %w", err)
	}
	
	// Add Lambda permission for EventBridge to invoke the function
	_, err = p.lambdaClient.AddPermission(ctx, &lambda.AddPermissionInput{
		FunctionName: aws.String(functionName),
		StatementId:  aws.String("eventbridge-ec2-invoke"),
		Action:       aws.String("lambda:InvokeFunction"),
		Principal:    aws.String("events.amazonaws.com"),
		SourceArn:    aws.String(fmt.Sprintf("arn:aws:events:%s:%s:rule/%s", p.region, p.accountID, ruleName)),
	})
	if err != nil && !strings.Contains(err.Error(), "ResourceConflictException") {
		return fmt.Errorf("failed to add Lambda permission for EventBridge: %w", err)
	}
	
	// Add Lambda as target for the rule
	_, err = eventClient.PutTargets(ctx, &eventbridge.PutTargetsInput{
		Rule: aws.String(ruleName),
		Targets: []eventbridgetypes.Target{
			{
				Id:  aws.String("1"),
				Arn: functionConfig.Configuration.FunctionArn,
			},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to add Lambda target to EventBridge rule: %w", err)
	}
	
	return nil
}
