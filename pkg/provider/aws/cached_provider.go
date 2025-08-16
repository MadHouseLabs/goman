package aws

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

// CachedCredentials stores cached AWS credentials and account info
type CachedCredentials struct {
	AccountID   string
	Region      string
	Profile     string
	CachedAt    time.Time
	TTL         time.Duration
}

var (
	credCache     *CachedCredentials
	credCacheMu   sync.RWMutex
	providerCache *AWSProvider
	providerMu    sync.RWMutex
)

// GetCachedProvider returns a cached AWS provider instance
// This avoids making STS calls on every provider creation
func GetCachedProvider(profile, region string) (*AWSProvider, error) {
	providerMu.RLock()
	if providerCache != nil {
		// Check if the cached provider matches the requested profile/region
		if providerCache.profile == profile && providerCache.region == region {
			providerMu.RUnlock()
			return providerCache, nil
		}
	}
	providerMu.RUnlock()

	// Need to create a new provider
	providerMu.Lock()
	defer providerMu.Unlock()

	// Double-check after acquiring write lock
	if providerCache != nil && providerCache.profile == profile && providerCache.region == region {
		return providerCache, nil
	}

	// Create new provider with cached credentials if available
	provider, err := NewProviderWithCache(profile, region)
	if err != nil {
		return nil, err
	}

	providerCache = provider
	return provider, nil
}

// NewProviderWithCache creates a new AWS provider using cached credentials when possible
func NewProviderWithCache(profile, region string) (*AWSProvider, error) {
	ctx := context.Background()

	if region == "" {
		region = "ap-south-1" // Default to Mumbai
	}

	// Check if we have cached credentials
	accountID := getCachedAccountID(profile, region)
	
	// Load AWS config
	var cfgOptions []func(*config.LoadOptions) error
	cfgOptions = append(cfgOptions, config.WithRegion(region))

	if profile != "" {
		cfgOptions = append(cfgOptions, config.WithSharedConfigProfile(profile))
	}

	cfg, err := config.LoadDefaultConfig(ctx, cfgOptions...)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	// If we don't have cached account ID, get it now
	if accountID == "" {
		stsClient := sts.NewFromConfig(cfg)
		identity, err := stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
		if err != nil {
			return nil, fmt.Errorf("failed to get AWS account ID: %w", err)
		}
		accountID = *identity.Account
		
		// Cache the credentials
		setCachedCredentials(profile, region, accountID)
	}

	// Create provider with all the services
	p := &AWSProvider{
		profile:      profile,
		region:       region,
		accountID:    accountID,
		cfg:          cfg,
		dynamoClient: dynamodb.NewFromConfig(cfg),
		s3Client:     s3.NewFromConfig(cfg),
		snsClient:    sns.NewFromConfig(cfg),
		sqsClient:    sqs.NewFromConfig(cfg),
		lambdaClient: lambda.NewFromConfig(cfg),
		ec2Client:    ec2.NewFromConfig(cfg),
		stsClient:    sts.NewFromConfig(cfg),
		iamClient:    iam.NewFromConfig(cfg),
	}

	// Initialize services
	p.lockService = NewLockService(p.dynamoClient, p.accountID)
	p.storageService = NewStorageService(p.s3Client, p.accountID)
	p.notificationService = NewNotificationService(p.snsClient, p.sqsClient, p.accountID, p.region)
	p.functionService = NewFunctionService(p.lambdaClient, p.s3Client, p.iamClient, p.accountID, p.region)
	p.computeService = NewComputeService(p.ec2Client, p.iamClient, p.cfg, p.accountID)

	return p, nil
}

// getCachedAccountID returns cached account ID if valid
func getCachedAccountID(profile, region string) string {
	credCacheMu.RLock()
	defer credCacheMu.RUnlock()

	if credCache == nil {
		return ""
	}

	// Check if cache matches and is still valid
	if credCache.Profile == profile && 
	   credCache.Region == region &&
	   time.Since(credCache.CachedAt) < credCache.TTL {
		return credCache.AccountID
	}

	return ""
}

// setCachedCredentials caches the AWS credentials
func setCachedCredentials(profile, region, accountID string) {
	credCacheMu.Lock()
	defer credCacheMu.Unlock()

	credCache = &CachedCredentials{
		AccountID: accountID,
		Region:    region,
		Profile:   profile,
		CachedAt:  time.Now(),
		TTL:       1 * time.Hour, // Cache for 1 hour
	}
}

// ClearProviderCache clears the cached provider
// Useful for testing or when switching accounts
func ClearProviderCache() {
	providerMu.Lock()
	defer providerMu.Unlock()
	providerCache = nil

	credCacheMu.Lock()
	defer credCacheMu.Unlock()
	credCache = nil
}