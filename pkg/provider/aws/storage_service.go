package aws

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// StorageService implements object storage using S3
type StorageService struct {
	client     *s3.Client
	bucketName string
}

// NewStorageService creates a new S3-based storage service
func NewStorageService(client *s3.Client, accountID string) *StorageService {
	return &StorageService{
		client:     client,
		bucketName: fmt.Sprintf("goman-%s", accountID),
	}
}

// Initialize ensures the S3 bucket exists
func (s *StorageService) Initialize(ctx context.Context) error {
	// Try to list objects first - this is a lighter check that usually works
	_, err := s.client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket:  aws.String(s.bucketName),
		MaxKeys: aws.Int32(1),
	})
	
	if err == nil {
		// Bucket exists and we can access it
		return nil
	}
	
	// If we can't list, try to check if bucket exists
	_, headErr := s.client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(s.bucketName),
	})
	
	if headErr == nil {
		// Bucket exists
		return nil
	}
	
	// Only try to create if we're certain it doesn't exist
	// In Lambda environment, we should not create buckets
	// The bucket should be created during setup
	return fmt.Errorf("bucket %s not accessible or does not exist: %w (original error: %v)", s.bucketName, headErr, err)
}

// PutObject stores an object
func (s *StorageService) PutObject(ctx context.Context, key string, data []byte) error {
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.bucketName),
		Key:    aws.String(key),
		Body:   bytes.NewReader(data),
	})
	
	if err != nil {
		return fmt.Errorf("failed to put object: %w", err)
	}
	
	return nil
}

// GetObject retrieves an object
func (s *StorageService) GetObject(ctx context.Context, key string) ([]byte, error) {
	result, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucketName),
		Key:    aws.String(key),
	})
	
	if err != nil {
		return nil, fmt.Errorf("failed to get object: %w", err)
	}
	defer result.Body.Close()
	
	data, err := io.ReadAll(result.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read object: %w", err)
	}
	
	return data, nil
}

// DeleteObject deletes an object
func (s *StorageService) DeleteObject(ctx context.Context, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucketName),
		Key:    aws.String(key),
	})
	
	if err != nil {
		return fmt.Errorf("failed to delete object: %w", err)
	}
	
	return nil
}

// ListObjects lists objects with a prefix
func (s *StorageService) ListObjects(ctx context.Context, prefix string) ([]string, error) {
	var keys []string
	
	paginator := s3.NewListObjectsV2Paginator(s.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(s.bucketName),
		Prefix: aws.String(prefix),
	})
	
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list objects: %w", err)
		}
		
		for _, obj := range page.Contents {
			keys = append(keys, *obj.Key)
		}
	}
	
	return keys, nil
}