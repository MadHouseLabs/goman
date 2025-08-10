package setup

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/eventbridge"
	eventbridgetypes "github.com/aws/aws-sdk-go-v2/service/eventbridge/types"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/madhouselabs/goman/pkg/provider/aws"
	"github.com/madhouselabs/goman/pkg/provider/registry"
	"github.com/madhouselabs/goman/pkg/storage"
)

// InitializeResult contains the results of initialization
type InitializeResult struct {
	S3BucketCreated      bool
	LambdaDeployed       bool
	DynamoDBCreated      bool
	NotificationsSetup   bool
	SSMProfileCreated    bool
	Errors               []string
}

// EnsureFullSetup ensures all required AWS resources are properly configured
func EnsureFullSetup(ctx context.Context) (*InitializeResult, error) {
	result := &InitializeResult{}
	
	// Step 1: Initialize storage (creates S3 bucket)
	_, err := storage.NewStorage()
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("Storage setup failed: %v", err))
		return result, fmt.Errorf("failed to initialize storage: %w", err)
	}
	result.S3BucketCreated = true
	
	// Step 2: Get the provider
	provider, err := registry.GetDefaultProvider()
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("Provider setup failed: %v", err))
		return result, fmt.Errorf("failed to get provider: %w", err)
	}
	
	// Step 3: Deploy Lambda function
	functionService := provider.GetFunctionService()
	
	// Initialize function service (this creates IAM roles, etc.)
	if err := functionService.Initialize(ctx); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("Function service init failed: %v", err))
		return result, fmt.Errorf("failed to initialize function service: %w", err)
	}
	
	// Get function package path
	packagePath := registry.GetFunctionPackagePath(provider.Name())
	functionName := "goman-cluster-controller"
	
	// Check if function exists
	exists, err := functionService.FunctionExists(ctx, functionName)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("Function check failed: %v", err))
	}
	
	if !exists {
		// Deploy the function (this also sets up S3 notifications)
		if err := functionService.DeployFunction(ctx, functionName, packagePath); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("Lambda deployment failed: %v", err))
			// Continue anyway - Lambda is optional for basic operations
		} else {
			result.LambdaDeployed = true
			result.NotificationsSetup = true
		}
	} else {
		result.LambdaDeployed = true
		
		// Ensure S3 notifications are set up even if Lambda exists
		// This is important because the bucket might have been recreated
		if err := ensureS3Notifications(ctx, provider); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("S3 notifications setup failed: %v", err))
		} else {
			result.NotificationsSetup = true
		}
	}
	
	// Step 4: Initialize lock service (creates DynamoDB table)
	lockService := provider.GetLockService()
	if err := lockService.Initialize(ctx); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("Lock service init failed: %v", err))
		// Continue anyway - locking is optional
	} else {
		result.DynamoDBCreated = true
	}
	
	// Step 5: Initialize compute service (creates SSM instance profile)
	// This needs to be done from local, not Lambda, because Lambda doesn't have
	// permission to create IAM roles (and shouldn't have)
	if awsProvider, ok := provider.(*aws.AWSProvider); ok {
		if computeService, ok := awsProvider.GetComputeService().(*aws.ComputeService); ok {
			if err := computeService.Initialize(ctx); err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("Compute service init failed: %v", err))
				// Continue anyway - SSM is optional (but recommended)
			} else {
				result.SSMProfileCreated = true
			}
		}
	}
	
	// Step 6: Set up EventBridge rule for EC2 instance state changes
	if awsProvider, ok := provider.(*aws.AWSProvider); ok {
		if err := setupEC2EventRule(ctx, awsProvider, functionName); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("EventBridge setup failed: %v", err))
		}
	}
	
	// Give AWS services a moment to stabilize
	time.Sleep(2 * time.Second)
	
	return result, nil
}

// ensureS3Notifications ensures S3 notifications are configured for the Lambda
func ensureS3Notifications(ctx context.Context, provider interface{}) error {
	// Type assert to AWS provider to access the function service
	awsProvider, ok := provider.(*aws.AWSProvider)
	if !ok {
		return fmt.Errorf("provider is not AWS provider")
	}
	
	functionService := awsProvider.GetFunctionService()
	
	// Get the Lambda function name
	functionName := fmt.Sprintf("goman-cluster-controller")
	
	// Check if Lambda function exists
	exists, err := functionService.FunctionExists(ctx, functionName)
	if err != nil {
		return fmt.Errorf("failed to check function existence: %w", err)
	}
	
	if !exists {
		return fmt.Errorf("Lambda function %s does not exist", functionName)
	}
	
	// Set up S3 trigger for the function
	// This will configure S3 bucket notifications to trigger Lambda on file changes
	if err := setupS3BucketNotifications(ctx, awsProvider, functionName); err != nil {
		return fmt.Errorf("failed to setup S3 notifications: %w", err)
	}
	
	return nil
}

// setupS3BucketNotifications configures S3 to trigger Lambda on cluster file changes
func setupS3BucketNotifications(ctx context.Context, provider *aws.AWSProvider, functionName string) error {
	bucketName := fmt.Sprintf("goman-%s", provider.AccountID())
	
	// Get S3 client from provider
	s3Client := provider.GetS3Client()
	if s3Client == nil {
		return fmt.Errorf("S3 client not available")
	}
	
	// Get Lambda client from provider
	lambdaClient := provider.GetLambdaClient()
	if lambdaClient == nil {
		return fmt.Errorf("Lambda client not available")
	}
	
	// Add permission for S3 to invoke the Lambda function
	_, err := lambdaClient.AddPermission(ctx, &lambda.AddPermissionInput{
		FunctionName: awssdk.String(functionName),
		StatementId:  awssdk.String("s3-invoke-permission"),
		Action:       awssdk.String("lambda:InvokeFunction"),
		Principal:    awssdk.String("s3.amazonaws.com"),
		SourceArn:    awssdk.String(fmt.Sprintf("arn:aws:s3:::%s", bucketName)),
	})
	
	if err != nil {
		// Permission might already exist, which is fine
		if !strings.Contains(err.Error(), "ResourceConflictException") {
			return fmt.Errorf("failed to add S3 invoke permission: %w", err)
		}
	}
	
	// Get function ARN
	functionConfig, err := lambdaClient.GetFunction(ctx, &lambda.GetFunctionInput{
		FunctionName: awssdk.String(functionName),
	})
	if err != nil {
		return fmt.Errorf("failed to get function configuration: %w", err)
	}
	
	// Configure S3 bucket notification
	notificationConfig := &s3.PutBucketNotificationConfigurationInput{
		Bucket: awssdk.String(bucketName),
		NotificationConfiguration: &s3types.NotificationConfiguration{
			LambdaFunctionConfigurations: []s3types.LambdaFunctionConfiguration{
				{
					Id:                awssdk.String("goman-cluster-changes"),
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
									Value: awssdk.String("clusters/"),
								},
								{
									Name:  s3types.FilterRuleNameSuffix,
									Value: awssdk.String(".json"),
								},
							},
						},
					},
				},
			},
		},
	}
	
	_, err = s3Client.PutBucketNotificationConfiguration(ctx, notificationConfig)
	if err != nil {
		return fmt.Errorf("failed to set up S3 bucket notification: %w", err)
	}
	
	return nil
}

// setupEC2EventRule creates an EventBridge rule to trigger Lambda on EC2 instance termination
func setupEC2EventRule(ctx context.Context, provider *aws.AWSProvider, functionName string) error {
	// Create EventBridge client
	eventClient := eventbridge.NewFromConfig(provider.GetConfig())
	lambdaClient := provider.GetLambdaClient()
	
	// Define the rule name
	ruleName := "goman-ec2-termination-rule"
	
	// Create event pattern for EC2 instance state changes
	eventPattern := map[string]interface{}{
		"source":      []string{"aws.ec2"},
		"detail-type": []string{"EC2 Instance State-change Notification"},
		"detail": map[string]interface{}{
			"state": []string{"terminated", "terminating", "stopped"},
		},
	}
	
	eventPatternJSON, err := json.Marshal(eventPattern)
	if err != nil {
		return fmt.Errorf("failed to marshal event pattern: %w", err)
	}
	
	// Create or update the rule
	_, err = eventClient.PutRule(ctx, &eventbridge.PutRuleInput{
		Name:         awssdk.String(ruleName),
		Description:  awssdk.String("Trigger Lambda on EC2 instance termination for Goman clusters"),
		EventPattern: awssdk.String(string(eventPatternJSON)),
		State:        eventbridgetypes.RuleStateEnabled,
	})
	if err != nil {
		return fmt.Errorf("failed to create EventBridge rule: %w", err)
	}
	
	// Get Lambda function ARN
	functionConfig, err := lambdaClient.GetFunction(ctx, &lambda.GetFunctionInput{
		FunctionName: awssdk.String(functionName),
	})
	if err != nil {
		return fmt.Errorf("failed to get Lambda function: %w", err)
	}
	
	// Add Lambda permission for EventBridge to invoke the function
	_, err = lambdaClient.AddPermission(ctx, &lambda.AddPermissionInput{
		FunctionName: awssdk.String(functionName),
		StatementId:  awssdk.String("eventbridge-ec2-invoke"),
		Action:       awssdk.String("lambda:InvokeFunction"),
		Principal:    awssdk.String("events.amazonaws.com"),
		SourceArn:    awssdk.String(fmt.Sprintf("arn:aws:events:%s:%s:rule/%s", provider.Region(), provider.AccountID(), ruleName)),
	})
	if err != nil && !strings.Contains(err.Error(), "ResourceConflictException") {
		return fmt.Errorf("failed to add Lambda permission for EventBridge: %w", err)
	}
	
	// Add Lambda as target for the rule
	_, err = eventClient.PutTargets(ctx, &eventbridge.PutTargetsInput{
		Rule: awssdk.String(ruleName),
		Targets: []eventbridgetypes.Target{
			{
				Id:  awssdk.String("1"),
				Arn: functionConfig.Configuration.FunctionArn,
			},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to add Lambda target to EventBridge rule: %w", err)
	}
	
	return nil
}