package aws

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/lambda/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/madhouselabs/goman/pkg/logger"
)

const (
	LambdaRolePrefix = "goman-lambda-role"
)

// FunctionService implements serverless functions using Lambda
type FunctionService struct {
	lambdaClient *lambda.Client
	s3Client     *s3.Client
	iamClient    *iam.Client
	accountID    string
	region       string
	roleArn      string // Cached IAM role ARN
}

// NewFunctionService creates a new Lambda-based function service
func NewFunctionService(lambdaClient *lambda.Client, s3Client *s3.Client, iamClient *iam.Client, accountID, region string) *FunctionService {
	return &FunctionService{
		lambdaClient: lambdaClient,
		s3Client:     s3Client,
		iamClient:    iamClient,
		accountID:    accountID,
		region:       region,
	}
}

// Initialize ensures required resources exist
func (s *FunctionService) Initialize(ctx context.Context) error {
	// The IAM role will be created on first deployment
	return nil
}

// DeployFunction deploys a function from a package
func (s *FunctionService) DeployFunction(ctx context.Context, name string, packagePath string) error {
	// For AWS provider, we expect the Lambda package to be at:
	// build/lambda-aws-controller.zip

	// Check if AWS-specific package exists
	awsPackagePath := packagePath
	if !strings.Contains(packagePath, "aws") {
		// Convert generic path to AWS-specific path
		awsPackagePath = strings.Replace(packagePath, "lambda-controller.zip", "lambda-aws-controller.zip", 1)
	}

	// Read the package
	packageData, err := os.ReadFile(awsPackagePath)
	if err != nil {
		// Fallback to generic package if AWS-specific doesn't exist
		packageData, err = os.ReadFile(packagePath)
		if err != nil {
			return fmt.Errorf("failed to read package (tried %s and %s): %w", awsPackagePath, packagePath, err)
		}
	}

	// Check package size - if over 50MB, upload to S3 first
	var codeLocation types.FunctionCode
	if len(packageData) > 50*1024*1024 { // 50MB limit for direct upload
		// Upload to S3
		bucketName := fmt.Sprintf("goman-%s", s.accountID)
		keyName := fmt.Sprintf("lambda/%s.zip", name)

		_, err = s.s3Client.PutObject(ctx, &s3.PutObjectInput{
			Bucket: aws.String(bucketName),
			Key:    aws.String(keyName),
			Body:   bytes.NewReader(packageData),
		})
		if err != nil {
			return fmt.Errorf("failed to upload Lambda package to S3: %w", err)
		}

		codeLocation = types.FunctionCode{
			S3Bucket: aws.String(bucketName),
			S3Key:    aws.String(keyName),
		}
	} else {
		codeLocation = types.FunctionCode{
			ZipFile: packageData,
		}
	}

	// Check if function exists
	exists, err := s.FunctionExists(ctx, name)
	if err != nil {
		return fmt.Errorf("failed to check function existence: %w", err)
	}

	if exists {
		// ALWAYS ensure IAM role and policy are up to date, even for existing functions
		// This ensures policy changes in code are applied when re-initializing
		if s.roleArn == "" {
			roleArn, err := s.ensureIAMRole(ctx)
			if err != nil {
				return fmt.Errorf("failed to ensure IAM role: %w", err)
			}
			s.roleArn = roleArn
		} else {
			// Even if we have a cached roleArn, still update the policy
			_, err := s.ensureIAMRole(ctx)
			if err != nil {
				return fmt.Errorf("failed to update IAM role policy: %w", err)
			}
		}

		// Update existing function
		if len(packageData) > 50*1024*1024 {
			// Update from S3
			bucketName := fmt.Sprintf("goman-%s", s.accountID)
			keyName := fmt.Sprintf("lambda/%s.zip", name)
			_, err = s.lambdaClient.UpdateFunctionCode(ctx, &lambda.UpdateFunctionCodeInput{
				FunctionName: aws.String(name),
				S3Bucket:     aws.String(bucketName),
				S3Key:        aws.String(keyName),
			})
		} else {
			_, err = s.lambdaClient.UpdateFunctionCode(ctx, &lambda.UpdateFunctionCodeInput{
				FunctionName: aws.String(name),
				ZipFile:      packageData,
			})
		}
		if err != nil {
			return fmt.Errorf("failed to update function code: %w", err)
		}

		// Update configuration
		_, err = s.lambdaClient.UpdateFunctionConfiguration(ctx, &lambda.UpdateFunctionConfigurationInput{
			FunctionName: aws.String(name),
			Timeout:      aws.Int32(900), // 15 minutes
			MemorySize:   aws.Int32(512),
			Environment: &types.Environment{
				Variables: map[string]string{
					"GOMAN_REGION":     s.region,
					"GOMAN_ACCOUNT_ID": s.accountID,
				},
			},
		})
		if err != nil {
			return fmt.Errorf("failed to update function configuration: %w", err)
		}

		// Ensure S3 trigger is configured for existing functions too
		// This handles cases where notifications were lost
		err = s.setupS3Trigger(ctx, name)
		if err != nil {
			logger.Printf("Warning: Failed to ensure S3 trigger: %v", err)
			// Don't fail deployment if trigger setup fails - it can be retried
		}
	} else {
		// Ensure IAM role exists
		if s.roleArn == "" {
			roleArn, err := s.ensureIAMRole(ctx)
			if err != nil {
				return fmt.Errorf("failed to ensure IAM role: %w", err)
			}
			s.roleArn = roleArn
		}

		// Create new function
		_, err = s.lambdaClient.CreateFunction(ctx, &lambda.CreateFunctionInput{
			FunctionName: aws.String(name),
			Runtime:      types.RuntimeProvidedal2, // Use custom runtime for Go
			Role:         aws.String(s.roleArn),
			Handler:      aws.String("bootstrap"), // Required for provided.al2 runtime
			Code:         &codeLocation,
			Timeout:      aws.Int32(900), // 15 minutes
			MemorySize:   aws.Int32(512),
			Description:  aws.String(fmt.Sprintf("Goman function: %s", name)),
			Environment: &types.Environment{
				Variables: map[string]string{
					"GOMAN_REGION":     s.region,
					"GOMAN_ACCOUNT_ID": s.accountID,
				},
			},
			Tags: map[string]string{
				"Application": "goman",
				"ManagedBy":   "goman",
			},
		})
		if err != nil {
			return fmt.Errorf("failed to create function: %w", err)
		}

		// Wait for function to be active
		waiter := lambda.NewFunctionActiveV2Waiter(s.lambdaClient)
		err = waiter.Wait(ctx, &lambda.GetFunctionInput{
			FunctionName: aws.String(name),
		}, 2*time.Minute)
		if err != nil {
			return fmt.Errorf("failed waiting for function to be active: %w", err)
		}

		// Set up S3 trigger
		err = s.setupS3Trigger(ctx, name)
		if err != nil {
			return fmt.Errorf("failed to setup S3 trigger: %w", err)
		}
	}

	return nil
}

// InvokeFunction invokes a function with payload
func (s *FunctionService) InvokeFunction(ctx context.Context, name string, payload []byte) ([]byte, error) {
	result, err := s.lambdaClient.Invoke(ctx, &lambda.InvokeInput{
		FunctionName: aws.String(name),
		Payload:      payload,
	})

	if err != nil {
		return nil, fmt.Errorf("failed to invoke function: %w", err)
	}

	if result.FunctionError != nil {
		return nil, fmt.Errorf("function error: %s", *result.FunctionError)
	}

	return result.Payload, nil
}

// DeleteFunction deletes a function
func (s *FunctionService) DeleteFunction(ctx context.Context, name string) error {
	_, err := s.lambdaClient.DeleteFunction(ctx, &lambda.DeleteFunctionInput{
		FunctionName: aws.String(name),
	})

	if err != nil {
		return fmt.Errorf("failed to delete function: %w", err)
	}

	return nil
}

// FunctionExists checks if a function exists
func (s *FunctionService) FunctionExists(ctx context.Context, name string) (bool, error) {
	_, err := s.lambdaClient.GetFunction(ctx, &lambda.GetFunctionInput{
		FunctionName: aws.String(name),
	})

	if err != nil {
		// Check if it's a not found error
		if isNotFoundError(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to get function: %w", err)
	}

	return true, nil
}

// GetFunctionURL returns the invocation URL for a function
func (s *FunctionService) GetFunctionURL(ctx context.Context, name string) (string, error) {
	// Check if function URL config exists
	urlConfig, err := s.lambdaClient.GetFunctionUrlConfig(ctx, &lambda.GetFunctionUrlConfigInput{
		FunctionName: aws.String(name),
	})

	if err != nil {
		if isNotFoundError(err) {
			// Create function URL
			createResult, err := s.lambdaClient.CreateFunctionUrlConfig(ctx, &lambda.CreateFunctionUrlConfigInput{
				FunctionName: aws.String(name),
				AuthType:     types.FunctionUrlAuthTypeAwsIam,
				Cors: &types.Cors{
					AllowOrigins: []string{"*"},
					AllowMethods: []string{"*"},
					AllowHeaders: []string{"*"},
				},
			})
			if err != nil {
				return "", fmt.Errorf("failed to create function URL: %w", err)
			}
			return *createResult.FunctionUrl, nil
		}
		return "", fmt.Errorf("failed to get function URL: %w", err)
	}

	return *urlConfig.FunctionUrl, nil
}

// ensureIAMRole creates or gets the IAM role for Lambda
func (s *FunctionService) ensureIAMRole(ctx context.Context) (string, error) {
	roleName := fmt.Sprintf("%s-%s", LambdaRolePrefix, s.accountID)
	var roleArn string

	// Check if role exists
	getRoleOutput, err := s.iamClient.GetRole(ctx, &iam.GetRoleInput{
		RoleName: aws.String(roleName),
	})

	if err == nil {
		// Role exists, but we still need to ensure the policy is up to date
		roleArn = *getRoleOutput.Role.Arn
		// Continue to check/update the policy below
	} else {
		// Role doesn't exist, we'll create it below
		roleArn = ""
	}

	// If role doesn't exist, create it
	if roleArn == "" {
		// Create trust policy for Lambda
		trustPolicy := map[string]interface{}{
			"Version": "2012-10-17",
			"Statement": []map[string]interface{}{
				{
					"Effect": "Allow",
					"Principal": map[string]string{
						"Service": "lambda.amazonaws.com",
					},
					"Action": "sts:AssumeRole",
				},
			},
		}

		trustPolicyJSON, err := json.Marshal(trustPolicy)
		if err != nil {
			return "", fmt.Errorf("failed to marshal trust policy: %w", err)
		}

		// Create the role
		createRoleOutput, err := s.iamClient.CreateRole(ctx, &iam.CreateRoleInput{
			RoleName:                 aws.String(roleName),
			AssumeRolePolicyDocument: aws.String(string(trustPolicyJSON)),
			Description:              aws.String("Role for Goman Lambda functions"),
			Tags: []iamtypes.Tag{
				{
					Key:   aws.String("Application"),
					Value: aws.String("goman"),
				},
				{
					Key:   aws.String("ManagedBy"),
					Value: aws.String("goman"),
				},
			},
		})

		if err != nil {
			return "", fmt.Errorf("failed to create IAM role: %w", err)
		}

		roleArn = *createRoleOutput.Role.Arn
	}

	// Attach basic Lambda execution policy
	policies := []string{
		"arn:aws:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole",
	}

	// Create and attach custom policy with least privilege
	policyName := fmt.Sprintf("goman-lambda-policy-%s", s.accountID)
	policyDocument := map[string]interface{}{
		"Version": "2012-10-17",
		"Statement": []map[string]interface{}{
			// S3 permissions for state management
			{
				"Effect": "Allow",
				"Action": []string{
					"s3:GetObject",
					"s3:PutObject",
					"s3:DeleteObject",
					"s3:ListBucket",
				},
				"Resource": []string{
					fmt.Sprintf("arn:aws:s3:::goman-%s/*", s.accountID),
					fmt.Sprintf("arn:aws:s3:::goman-%s", s.accountID),
				},
			},
			// DynamoDB permissions for distributed locking
			{
				"Effect": "Allow",
				"Action": []string{
					"dynamodb:DescribeTable",
					"dynamodb:GetItem",
					"dynamodb:PutItem",
					"dynamodb:DeleteItem",
					"dynamodb:UpdateItem",
				},
				"Resource": fmt.Sprintf("arn:aws:dynamodb:%s:%s:table/goman-resource-locks", s.region, s.accountID),
			},
			// EC2 permissions for instance management - split for least privilege
			{
				"Effect": "Allow",
				"Action": []string{
					"ec2:DescribeInstances",
					"ec2:DescribeSecurityGroups",
					"ec2:DescribeVpcs",
					"ec2:DescribeSubnets",
				},
				"Resource": "*", // Read operations require wildcard
			},
			{
				"Effect": "Allow",
				"Action": []string{
					"ec2:RunInstances",
					"ec2:TerminateInstances",
					"ec2:StopInstances",
					"ec2:StartInstances",
					"ec2:CreateSecurityGroup",
					"ec2:AuthorizeSecurityGroupIngress",
					"ec2:DeleteSecurityGroup",
					"ec2:CreateTags",
					"ec2:ModifyInstanceAttribute",
				},
				"Resource": []string{
					fmt.Sprintf("arn:aws:ec2:*:%s:instance/*", s.accountID),
					fmt.Sprintf("arn:aws:ec2:*:%s:security-group/*", s.accountID),
					fmt.Sprintf("arn:aws:ec2:*:%s:vpc/*", s.accountID), // Required for CreateSecurityGroup
					fmt.Sprintf("arn:aws:ec2:*:%s:subnet/*", s.accountID),
					fmt.Sprintf("arn:aws:ec2:*:%s:volume/*", s.accountID),
					fmt.Sprintf("arn:aws:ec2:*:%s:network-interface/*", s.accountID),
					"arn:aws:ec2:*::image/*", // AMIs can be public (no account ID needed)
				},
			},
			// IAM permissions for using instance profiles (not creating them)
			{
				"Effect": "Allow",
				"Action": []string{
					"iam:GetRole",
					"iam:GetInstanceProfile",
					"iam:PassRole",
				},
				"Resource": []string{
					fmt.Sprintf("arn:aws:iam::%s:role/goman-ssm-instance-role", s.accountID),
					fmt.Sprintf("arn:aws:iam::%s:instance-profile/goman-ssm-instance-profile", s.accountID),
				},
			},
			// SSM permissions for remote command execution and parameter access
			{
				"Effect": "Allow",
				"Action": []string{
					"ssm:SendCommand",
					"ssm:GetCommandInvocation",
					"ssm:ListCommandInvocations",
				},
				"Resource": []string{
					fmt.Sprintf("arn:aws:ssm:*:%s:*", s.accountID),
					fmt.Sprintf("arn:aws:ec2:*:%s:instance/*", s.accountID),
					"arn:aws:ssm:*::document/AWS-RunShellScript",
				},
			},
			{
				"Effect": "Allow",
				"Action": []string{
					"ssm:GetParameter",
				},
				"Resource": []string{
					"arn:aws:ssm:*::parameter/aws/service/canonical/ubuntu/*",
					"arn:aws:ssm:*::parameter/aws/service/ami-amazon-linux-latest/*",
				},
			},
			// SNS permissions for notification service
			{
				"Effect": "Allow",
				"Action": []string{
					"sns:ListTopics",
					"sns:CreateTopic",
					"sns:Publish",
					"sns:Subscribe",
					"sns:Unsubscribe",
					"sns:GetTopicAttributes",
				},
				"Resource": []string{
					fmt.Sprintf("arn:aws:sns:*:%s:goman-*", s.accountID),
				},
			},
			// SQS permissions for notification service subscriptions
			{
				"Effect": "Allow",
				"Action": []string{
					"sqs:CreateQueue",
					"sqs:DeleteQueue",
					"sqs:GetQueueAttributes",
					"sqs:ReceiveMessage",
					"sqs:DeleteMessage",
					"sqs:SendMessage",
				},
				"Resource": []string{
					fmt.Sprintf("arn:aws:sqs:*:%s:goman-*", s.accountID),
				},
			},
		},
	}

	policyJSON, err := json.Marshal(policyDocument)
	if err != nil {
		return "", fmt.Errorf("failed to marshal policy document: %w", err)
	}

	// Try to create the policy first
	policyArn := fmt.Sprintf("arn:aws:iam::%s:policy/%s", s.accountID, policyName)
	createPolicyOutput, err := s.iamClient.CreatePolicy(ctx, &iam.CreatePolicyInput{
		PolicyName:     aws.String(policyName),
		PolicyDocument: aws.String(string(policyJSON)),
		Description:    aws.String("Least privilege policy for goman Lambda function"),
	})

	if err != nil {
		// Policy already exists, update it by creating a new version
		if strings.Contains(err.Error(), "EntityAlreadyExists") {
			// First, list and delete old policy versions to make room
			listVersionsOutput, listErr := s.iamClient.ListPolicyVersions(ctx, &iam.ListPolicyVersionsInput{
				PolicyArn: aws.String(policyArn),
			})

			if listErr == nil && listVersionsOutput.Versions != nil {
				// AWS allows max 5 versions. Delete old non-default versions
				for _, version := range listVersionsOutput.Versions {
					if !version.IsDefaultVersion && len(listVersionsOutput.Versions) >= 5 {
						_, delErr := s.iamClient.DeletePolicyVersion(ctx, &iam.DeletePolicyVersionInput{
							PolicyArn: aws.String(policyArn),
							VersionId: version.VersionId,
						})
						if delErr != nil {
							logger.Printf("Warning: Failed to delete old policy version %s: %v", *version.VersionId, delErr)
						}
					}
				}
			}

			// Create new policy version
			_, versionErr := s.iamClient.CreatePolicyVersion(ctx, &iam.CreatePolicyVersionInput{
				PolicyArn:      aws.String(policyArn),
				PolicyDocument: aws.String(string(policyJSON)),
				SetAsDefault:   true,
			})

			if versionErr != nil {
				return "", fmt.Errorf("failed to create new policy version: %w", versionErr)
			}

			// Policy updated successfully
			policies = append(policies, policyArn)
		} else {
			// Some other error occurred
			return "", fmt.Errorf("failed to create policy: %w", err)
		}
	} else {
		policies = append(policies, *createPolicyOutput.Policy.Arn)
	}

	for _, policyArn := range policies {
		_, err = s.iamClient.AttachRolePolicy(ctx, &iam.AttachRolePolicyInput{
			RoleName:  aws.String(roleName),
			PolicyArn: aws.String(policyArn),
		})
		if err != nil {
			return "", fmt.Errorf("failed to attach policy %s: %w", policyArn, err)
		}
	}

	// Wait a bit for the role to be available
	time.Sleep(10 * time.Second)

	return roleArn, nil
}

// setupS3Trigger sets up S3 event notification to trigger Lambda
func (s *FunctionService) setupS3Trigger(ctx context.Context, functionName string) error {
	bucketName := fmt.Sprintf("goman-%s", s.accountID)

	// First check if notifications are already configured
	existingConfig, err := s.s3Client.GetBucketNotificationConfiguration(ctx, &s3.GetBucketNotificationConfigurationInput{
		Bucket: aws.String(bucketName),
	})
	if err == nil && existingConfig != nil {
		// Check if our notification already exists (could be either ID)
		for _, config := range existingConfig.LambdaFunctionConfigurations {
			if config.Id != nil && (*config.Id == "goman-state-changes" || *config.Id == "goman-cluster-changes") {
				logger.Printf("S3 notifications already configured for function %s", functionName)
				return nil
			}
		}
	}

	// Add permission for S3 to invoke the function
	_, err = s.lambdaClient.AddPermission(ctx, &lambda.AddPermissionInput{
		FunctionName: aws.String(functionName),
		StatementId:  aws.String("s3-invoke-permission"),
		Action:       aws.String("lambda:InvokeFunction"),
		Principal:    aws.String("s3.amazonaws.com"),
		SourceArn:    aws.String(fmt.Sprintf("arn:aws:s3:::%s", bucketName)),
	})

	if err != nil {
		// Permission might already exist
		if !strings.Contains(err.Error(), "ResourceConflictException") {
			return fmt.Errorf("failed to add S3 invoke permission: %w", err)
		}
	}

	// Get function ARN
	functionConfig, err := s.lambdaClient.GetFunction(ctx, &lambda.GetFunctionInput{
		FunctionName: aws.String(functionName),
	})
	if err != nil {
		return fmt.Errorf("failed to get function configuration: %w", err)
	}

	// Set up S3 bucket notification
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

	_, err = s.s3Client.PutBucketNotificationConfiguration(ctx, notificationConfig)
	if err != nil {
		return fmt.Errorf("failed to set up S3 bucket notification: %w", err)
	}

	return nil
}

// isNotFoundError checks if error is a not found error
func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	// Check for various AWS not found error types
	return strings.Contains(err.Error(), "ResourceNotFoundException") ||
		strings.Contains(err.Error(), "NotFound")
}
