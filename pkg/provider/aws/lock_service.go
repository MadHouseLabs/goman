package aws

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/google/uuid"
)

const (
	LockTableName = "goman-resource-locks"
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
	// Check if table exists
	_, err := s.client.DescribeTable(ctx, &dynamodb.DescribeTableInput{
		TableName: aws.String(s.tableName),
	})
	
	if err == nil {
		// Table exists
		return nil
	}
	
	// Create table - only if we have permission
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
	
	// Wait for table to be active
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
	
	// Try to put item with condition that it doesn't exist or has expired
	_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(s.tableName),
		Item:      av,
		ConditionExpression: aws.String("attribute_not_exists(resource_id) OR expires_at < :now"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":now": &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", time.Now().Unix())},
		},
	})
	
	if err != nil {
		// Check if it's a conditional check failure
		if _, ok := err.(*types.ConditionalCheckFailedException); ok {
			// Lock is held by another process
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
		// Check if it's a conditional check failure
		if _, ok := err.(*types.ConditionalCheckFailedException); ok {
			return fmt.Errorf("invalid token or lock already released")
		}
		return fmt.Errorf("failed to release lock: %w", err)
	}
	
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
		UpdateExpression: aws.String("SET expires_at = :expires_at"),
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
		// Check if it's a conditional check failure
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
		// No lock exists
		return false, "", nil
	}
	
	var item LockItem
	err = attributevalue.UnmarshalMap(result.Item, &item)
	if err != nil {
		return false, "", fmt.Errorf("failed to unmarshal lock item: %w", err)
	}
	
	// Check if lock has expired
	if item.ExpiresAt < time.Now().Unix() {
		// Lock has expired
		return false, "", nil
	}
	
	return true, item.Owner, nil
}