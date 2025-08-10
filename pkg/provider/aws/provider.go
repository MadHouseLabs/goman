package aws

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/madhouselabs/goman/pkg/provider"
)

// AWSProvider implements the Provider interface for AWS
type AWSProvider struct {
	profile        string
	region         string
	accountID      string
	cfg            aws.Config
	
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
	
	// Check if we're running in Lambda environment
	isLambda := os.Getenv("AWS_LAMBDA_RUNTIME_API") != ""
	
	if isLambda {
		log.Printf("NewProvider called with profile='%s', region='%s'", profile, region)
	}
	
	if region == "" {
		region = "ap-south-1" // Default to Mumbai
		if isLambda {
			log.Printf("Using default region: %s", region)
		}
	}
	
	// Load AWS config
	var cfgOptions []func(*config.LoadOptions) error
	cfgOptions = append(cfgOptions, config.WithRegion(region))
	
	// Only add profile if it's not empty (for Lambda environment)
	if profile != "" {
		if isLambda {
			log.Printf("Adding profile to config: %s", profile)
		}
		cfgOptions = append(cfgOptions, config.WithSharedConfigProfile(profile))
	} else if isLambda {
		log.Println("No profile specified, using default credentials chain")
	}
	
	if isLambda {
		log.Println("Loading AWS config...")
	}
	cfg, err := config.LoadDefaultConfig(ctx, cfgOptions...)
	if err != nil {
		if isLambda {
			log.Printf("Failed to load AWS config: %v", err)
		}
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}
	if isLambda {
		log.Println("AWS config loaded successfully")
	}
	
	// Get account ID
	if isLambda {
		log.Println("Getting AWS account ID...")
	}
	stsClient := sts.NewFromConfig(cfg)
	identity, err := stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		if isLambda {
			log.Printf("Failed to get AWS account ID: %v", err)
		}
		return nil, fmt.Errorf("failed to get AWS account ID: %w", err)
	}
	if isLambda {
		log.Printf("AWS account ID: %s", *identity.Account)
	}
	
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
	log.Printf("Cleaning up AWS resources for cluster %s", clusterName)
	
	// Check if there are any instances still running
	instances, err := p.computeService.ListInstances(ctx, map[string]string{
		"tag:Cluster": clusterName,
		"instance-state-name": "running,pending,stopping,stopped",
	})
	
	if err == nil && len(instances) > 0 {
		log.Printf("Cannot cleanup: %d instances still exist for cluster %s", len(instances), clusterName)
		return fmt.Errorf("cannot cleanup: instances still exist")
	}
	
	// Clean up cluster-specific security group
	sgName := fmt.Sprintf("goman-%s-sg", clusterName)
	describeSGOutput, err := p.ec2Client.DescribeSecurityGroups(ctx, &ec2.DescribeSecurityGroupsInput{
		Filters: []types.Filter{
			{
				Name:   aws.String("group-name"),
				Values: []string{sgName},
			},
		},
	})
	
	if err == nil && len(describeSGOutput.SecurityGroups) > 0 {
		sgID := describeSGOutput.SecurityGroups[0].GroupId
		
		// Try to delete the security group
		_, err = p.ec2Client.DeleteSecurityGroup(ctx, &ec2.DeleteSecurityGroupInput{
			GroupId: sgID,
		})
		if err != nil {
			// Security group might still be in use, which is fine
			log.Printf("Note: Security group %s not deleted (may be in use): %v", sgName, err)
		} else {
			log.Printf("Deleted security group %s", sgName)
		}
	}
	
	// Note: We intentionally DO NOT delete:
	// - VPC (using default VPC)
	// - Subnets (using default subnets) 
	// - IAM roles/instance profiles (can be reused)
	
	log.Printf("Cluster %s cleanup complete (preserving reusable resources)", clusterName)
	return nil
}