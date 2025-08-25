package aws

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/google/uuid"
	"github.com/madhouselabs/goman/pkg/provider"
)

const (
	LockTableName    = "goman-resource-locks"
	LockTTLAttribute = "expires_at"
)

// LockService implements distributed locking using DynamoDB
type LockService struct {
	client    *dynamodb.Client
	tableName string
}

// LockItem represents a lock in DynamoDB
type LockItem struct {
	ResourceID string `dynamodbav:"resource_id"`
	Owner      string `dynamodbav:"owner"`
	Token      string `dynamodbav:"token"`
	ExpiresAt  int64  `dynamodbav:"expires_at"` // Unix timestamp for TTL
	CreatedAt  string `dynamodbav:"created_at"`
	// Metadata fields
	Phase     string `dynamodbav:"phase,omitempty"`
	Step      string `dynamodbav:"step,omitempty"`
	RequestID string `dynamodbav:"request_id,omitempty"`
	StartedAt string `dynamodbav:"started_at,omitempty"`
}

// NewLockService creates a new DynamoDB-based lock service
func NewLockService(client *dynamodb.Client, accountID string) *LockService {
	return &LockService{
		client:    client,
		tableName: LockTableName,
	}
}

// Initialize ensures the DynamoDB table exists
func (s *LockService) Initialize(ctx context.Context) error {
	_, err := s.client.DescribeTable(ctx, &dynamodb.DescribeTableInput{
		TableName: aws.String(s.tableName),
	})

	if err == nil {
		return nil
	}

	if os.Getenv("LAMBDA_TASK_ROOT") != "" {
		log.Printf("Warning: Could not describe DynamoDB table %s in Lambda environment: %v", s.tableName, err)
		return nil
	}

	_, err = s.client.CreateTable(ctx, &dynamodb.CreateTableInput{
		TableName: aws.String(s.tableName),
		KeySchema: []types.KeySchemaElement{
			{
				AttributeName: aws.String("resource_id"),
				KeyType:       types.KeyTypeHash,
			},
		},
		AttributeDefinitions: []types.AttributeDefinition{
			{
				AttributeName: aws.String("resource_id"),
				AttributeType: types.ScalarAttributeTypeS,
			},
		},
		BillingMode: types.BillingModePayPerRequest,
		Tags: []types.Tag{
			{
				Key:   aws.String("Application"),
				Value: aws.String("goman"),
			},
			{
				Key:   aws.String("Purpose"),
				Value: aws.String("resource-locking"),
			},
		},
	})

	if err != nil {
		return fmt.Errorf("failed to create lock table: %w", err)
	}

	waiter := dynamodb.NewTableExistsWaiter(s.client)
	err = waiter.Wait(ctx, &dynamodb.DescribeTableInput{
		TableName: aws.String(s.tableName),
	}, 2*time.Minute)

	if err != nil {
		return fmt.Errorf("failed waiting for table to be active: %w", err)
	}

	return nil
}

// AcquireLock tries to acquire a lock for a resource
func (s *LockService) AcquireLock(ctx context.Context, resourceID string, owner string, ttl time.Duration) (string, error) {
	token := uuid.New().String()
	expiresAt := time.Now().Add(ttl).Unix()

	item := LockItem{
		ResourceID: resourceID,
		Owner:      owner,
		Token:      token,
		ExpiresAt:  expiresAt,
		CreatedAt:  time.Now().Format(time.RFC3339),
	}

	av, err := attributevalue.MarshalMap(item)
	if err != nil {
		return "", fmt.Errorf("failed to marshal lock item: %w", err)
	}

	_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           aws.String(s.tableName),
		Item:                av,
		ConditionExpression: aws.String("attribute_not_exists(resource_id) OR expires_at < :now"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":now": &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", time.Now().Unix())},
		},
	})

	if err != nil {
		if _, ok := err.(*types.ConditionalCheckFailedException); ok {
			locked, lockOwner, _ := s.IsLocked(ctx, resourceID)
			if locked {
				return "", fmt.Errorf("resource %s is locked by %s", resourceID, lockOwner)
			}
			return "", fmt.Errorf("lock condition check failed")
		}
		return "", fmt.Errorf("failed to acquire lock: %w", err)
	}

	return token, nil
}

// AcquireLockWithMetadata tries to acquire a lock with additional metadata
func (s *LockService) AcquireLockWithMetadata(ctx context.Context, resourceID string, owner string, ttl time.Duration, metadata *provider.LockMetadata) (string, error) {
	token := uuid.New().String()
	expiresAt := time.Now().Add(ttl).Unix()

	item := LockItem{
		ResourceID: resourceID,
		Owner:      owner,
		Token:      token,
		ExpiresAt:  expiresAt,
		CreatedAt:  time.Now().Format(time.RFC3339),
	}

	// Add metadata if provided
	if metadata != nil {
		item.Phase = metadata.Phase
		item.Step = metadata.Step
		item.RequestID = metadata.RequestID
		item.StartedAt = metadata.StartedAt.Format(time.RFC3339)
	}

	av, err := attributevalue.MarshalMap(item)
	if err != nil {
		return "", fmt.Errorf("failed to marshal lock item: %w", err)
	}

	_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           aws.String(s.tableName),
		Item:                av,
		ConditionExpression: aws.String("attribute_not_exists(resource_id) OR expires_at < :now"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":now": &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", time.Now().Unix())},
		},
	})

	if err != nil {
		if _, ok := err.(*types.ConditionalCheckFailedException); ok {
			locked, lockOwner, _ := s.IsLocked(ctx, resourceID)
			if locked {
				return "", fmt.Errorf("resource %s is locked by %s", resourceID, lockOwner)
			}
			return "", fmt.Errorf("lock condition check failed")
		}
		return "", fmt.Errorf("failed to acquire lock: %w", err)
	}

	log.Printf("[LOCK] Acquired lock for %s (phase: %s, step: %s, requestId: %s)", 
		resourceID, 
		func() string { if metadata != nil { return metadata.Phase } else { return "unknown" } }(),
		func() string { if metadata != nil { return metadata.Step } else { return "unknown" } }(),
		func() string { if metadata != nil { return metadata.RequestID } else { return "unknown" } }())

	return token, nil
}

// ReleaseLock releases a lock using the token
func (s *LockService) ReleaseLock(ctx context.Context, resourceID string, token string) error {
	_, err := s.client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"resource_id": &types.AttributeValueMemberS{Value: resourceID},
		},
		ConditionExpression: aws.String("#t = :token"),
		ExpressionAttributeNames: map[string]string{
			"#t": "token",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":token": &types.AttributeValueMemberS{Value: token},
		},
	})

	if err != nil {
		if _, ok := err.(*types.ConditionalCheckFailedException); ok {
			log.Printf("[LOCK] Failed to release lock for %s: invalid token or lock already released", resourceID)
			return fmt.Errorf("invalid token or lock already released")
		}
		log.Printf("[LOCK] Failed to release lock for %s: %v", resourceID, err)
		return fmt.Errorf("failed to release lock: %w", err)
	}

	log.Printf("[LOCK] Released lock for %s", resourceID)
	return nil
}

// RenewLock extends the TTL of an existing lock
func (s *LockService) RenewLock(ctx context.Context, resourceID string, token string, ttl time.Duration) error {
	expiresAt := time.Now().Add(ttl).Unix()

	_, err := s.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"resource_id": &types.AttributeValueMemberS{Value: resourceID},
		},
		UpdateExpression:    aws.String("SET expires_at = :expires_at"),
		ConditionExpression: aws.String("#t = :token AND expires_at > :now"),
		ExpressionAttributeNames: map[string]string{
			"#t": "token",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":token":      &types.AttributeValueMemberS{Value: token},
			":expires_at": &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", expiresAt)},
			":now":        &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", time.Now().Unix())},
		},
	})

	if err != nil {
		if _, ok := err.(*types.ConditionalCheckFailedException); ok {
			return fmt.Errorf("invalid token or lock expired")
		}
		return fmt.Errorf("failed to renew lock: %w", err)
	}

	return nil
}

// IsLocked checks if a resource is currently locked
func (s *LockService) IsLocked(ctx context.Context, resourceID string) (bool, string, error) {
	result, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"resource_id": &types.AttributeValueMemberS{Value: resourceID},
		},
	})

	if err != nil {
		return false, "", fmt.Errorf("failed to get lock item: %w", err)
	}

	if len(result.Item) == 0 {
		return false, "", nil
	}

	var item LockItem
	err = attributevalue.UnmarshalMap(result.Item, &item)
	if err != nil {
		return false, "", fmt.Errorf("failed to unmarshal lock item: %w", err)
	}

	if item.ExpiresAt < time.Now().Unix() {
		return false, "", nil
	}

	return true, item.Owner, nil
}
