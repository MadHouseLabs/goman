# Lambda Controller Architecture

Goman uses AWS Lambda for serverless reconciliation of K3s clusters. The controller follows Kubernetes-style reconciliation patterns with distributed locking.

## Architecture

```
User → TUI → Write State to S3 → S3 Event → Lambda Controller → Reconcile State
                ↓                                    ↓
           Read State ← TUI displays ← Update State in S3
```

## Key Features

### Reconciliation Loop
- **Phase-based State Machine**: Clusters progress through phases (Pending → Provisioning → Running)
- **Automatic Rescheduling**: Long-running operations are rescheduled with exponential backoff
- **Idempotent Operations**: Safe to retry without side effects

### Distributed Locking
- **DynamoDB-based Locks**: Prevents concurrent modifications
- **TTL-based Expiration**: Automatic lock release after timeout
- **Lock Renewal**: Long operations renew locks periodically

### Error Handling
- **Exponential Backoff**: Automatic retry with increasing delays
- **Context Timeouts**: All operations have timeout protection
- **Graceful Degradation**: Failures don't affect other clusters

## Deployment

The Lambda function is automatically deployed on first cluster creation:

```bash
# Build Lambda package
task build:lambda:aws

# Package will be at
build/lambda-aws-controller.zip
```

## Configuration

Lambda function configuration:
- **Runtime**: Go 1.x
- **Memory**: 512 MB
- **Timeout**: 900 seconds (15 minutes)
- **Trigger**: S3 PutObject events

## Monitoring

View Lambda logs:
```bash
task lambda:logs
# or
aws logs tail /aws/lambda/goman-cluster-controller --follow
```

## Event Flow

1. **State Change**: User creates/modifies cluster via TUI
2. **S3 Write**: State written to S3 bucket
3. **Event Trigger**: S3 triggers Lambda via event notification
4. **Lock Acquisition**: Lambda acquires distributed lock
5. **Reconciliation**: Controller reconciles actual vs desired state
6. **State Update**: Updated state written back to S3
7. **Lock Release**: Distributed lock released

## Development

To test the Lambda function locally:
```bash
# Set environment variables
export AWS_PROFILE=your-profile
export AWS_REGION=ap-south-1

# Run the controller
go run lambda/controller/*.go
```