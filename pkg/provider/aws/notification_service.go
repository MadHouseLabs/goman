package aws

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	snstypes "github.com/aws/aws-sdk-go-v2/service/sns/types"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
)

// NotificationService implements pub/sub using SNS and SQS
type NotificationService struct {
	snsClient *sns.Client
	sqsClient *sqs.Client
	accountID string
	region    string
	topicArns map[string]string // cache topic ARNs
	queueURLs map[string]string // cache queue URLs
}

// NewNotificationService creates a new SNS/SQS-based notification service
func NewNotificationService(snsClient *sns.Client, sqsClient *sqs.Client, accountID, region string) *NotificationService {
	return &NotificationService{
		snsClient: snsClient,
		sqsClient: sqsClient,
		accountID: accountID,
		region:    region,
		topicArns: make(map[string]string),
		queueURLs: make(map[string]string),
	}
}

// Initialize ensures required topics exist
func (s *NotificationService) Initialize(ctx context.Context) error {
	// In Lambda environment, skip initialization as SNS topics should already exist
	// and we might not have permissions to create them
	if os.Getenv("LAMBDA_TASK_ROOT") != "" {
		log.Println("Running in Lambda environment, skipping SNS topic initialization")
		// Just populate the ARNs for known topics without checking/creating
		topics := []string{
			"goman-cluster-events",
			"goman-reconcile-events",
			"goman-error-events",
		}
		for _, topic := range topics {
			// Construct the ARN directly without checking if topic exists
			arn := fmt.Sprintf("arn:aws:sns:%s:%s:%s", s.region, s.accountID, topic)
			s.topicArns[topic] = arn
		}
		return nil
	}

	// Create default topics (only in non-Lambda environment)
	topics := []string{
		"goman-cluster-events",
		"goman-reconcile-events",
		"goman-error-events",
	}

	for _, topic := range topics {
		arn, err := s.ensureTopic(ctx, topic)
		if err != nil {
			return fmt.Errorf("failed to ensure topic %s: %w", topic, err)
		}
		s.topicArns[topic] = arn
	}

	return nil
}

// Publish sends a message to a topic
func (s *NotificationService) Publish(ctx context.Context, topic string, message string) error {
	// Get or create topic ARN
	arn, ok := s.topicArns[topic]
	if !ok {
		var err error
		arn, err = s.ensureTopic(ctx, topic)
		if err != nil {
			return fmt.Errorf("failed to ensure topic %s: %w", topic, err)
		}
		s.topicArns[topic] = arn
	}

	// Publish message
	_, err := s.snsClient.Publish(ctx, &sns.PublishInput{
		TopicArn: aws.String(arn),
		Message:  aws.String(message),
		MessageAttributes: map[string]snstypes.MessageAttributeValue{
			"Source": {
				DataType:    aws.String("String"),
				StringValue: aws.String("goman"),
			},
		},
	})

	if err != nil {
		return fmt.Errorf("failed to publish message: %w", err)
	}

	return nil
}

// Subscribe creates a subscription to a topic
func (s *NotificationService) Subscribe(ctx context.Context, topic string) (string, error) {
	// Get or create topic ARN
	arn, ok := s.topicArns[topic]
	if !ok {
		var err error
		arn, err = s.ensureTopic(ctx, topic)
		if err != nil {
			return "", fmt.Errorf("failed to ensure topic %s: %w", topic, err)
		}
		s.topicArns[topic] = arn
	}

	// Create SQS queue for subscription
	queueName := fmt.Sprintf("goman-sub-%s-%d", topic, time.Now().Unix())
	queueURL, err := s.createQueue(ctx, queueName)
	if err != nil {
		return "", fmt.Errorf("failed to create queue: %w", err)
	}

	// Get queue ARN
	queueAttrs, err := s.sqsClient.GetQueueAttributes(ctx, &sqs.GetQueueAttributesInput{
		QueueUrl:       aws.String(queueURL),
		AttributeNames: []sqstypes.QueueAttributeName{sqstypes.QueueAttributeNameQueueArn},
	})
	if err != nil {
		return "", fmt.Errorf("failed to get queue ARN: %w", err)
	}

	queueArn := queueAttrs.Attributes["QueueArn"]

	// Subscribe queue to topic
	subResult, err := s.snsClient.Subscribe(ctx, &sns.SubscribeInput{
		TopicArn: aws.String(arn),
		Protocol: aws.String("sqs"),
		Endpoint: aws.String(queueArn),
		Attributes: map[string]string{
			"RawMessageDelivery": "true",
		},
	})

	if err != nil {
		return "", fmt.Errorf("failed to subscribe: %w", err)
	}

	// Store subscription ID
	s.queueURLs[*subResult.SubscriptionArn] = queueURL

	return *subResult.SubscriptionArn, nil
}

// Unsubscribe removes a subscription
func (s *NotificationService) Unsubscribe(ctx context.Context, subscriptionID string) error {
	// Unsubscribe from SNS
	_, err := s.snsClient.Unsubscribe(ctx, &sns.UnsubscribeInput{
		SubscriptionArn: aws.String(subscriptionID),
	})

	if err != nil {
		return fmt.Errorf("failed to unsubscribe: %w", err)
	}

	// Delete associated queue if exists
	if queueURL, ok := s.queueURLs[subscriptionID]; ok {
		_, err = s.sqsClient.DeleteQueue(ctx, &sqs.DeleteQueueInput{
			QueueUrl: aws.String(queueURL),
		})
		if err != nil {
			// Non-critical error
			fmt.Printf("Warning: failed to delete queue: %v\n", err)
		}
		delete(s.queueURLs, subscriptionID)
	}

	return nil
}

// ensureTopic creates a topic if it doesn't exist
func (s *NotificationService) ensureTopic(ctx context.Context, topicName string) (string, error) {
	// Check if topic exists
	listResult, err := s.snsClient.ListTopics(ctx, &sns.ListTopicsInput{})
	if err != nil {
		return "", fmt.Errorf("failed to list topics: %w", err)
	}

	for _, topic := range listResult.Topics {
		if strings.Contains(*topic.TopicArn, topicName) {
			return *topic.TopicArn, nil
		}
	}

	// Create topic
	createResult, err := s.snsClient.CreateTopic(ctx, &sns.CreateTopicInput{
		Name: aws.String(topicName),
		Tags: []snstypes.Tag{
			{
				Key:   aws.String("Application"),
				Value: aws.String("goman"),
			},
		},
	})

	if err != nil {
		return "", fmt.Errorf("failed to create topic: %w", err)
	}

	return *createResult.TopicArn, nil
}

// createQueue creates an SQS queue
func (s *NotificationService) createQueue(ctx context.Context, queueName string) (string, error) {
	result, err := s.sqsClient.CreateQueue(ctx, &sqs.CreateQueueInput{
		QueueName: aws.String(queueName),
		Attributes: map[string]string{
			"MessageRetentionPeriod": "86400", // 1 day
			"VisibilityTimeout":      "300",   // 5 minutes
		},
		Tags: map[string]string{
			"Application": "goman",
			"Purpose":     "notification-subscription",
		},
	})

	if err != nil {
		return "", fmt.Errorf("failed to create queue: %w", err)
	}

	return *result.QueueUrl, nil
}
