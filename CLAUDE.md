# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Goman is a serverless K3s cluster management tool that uses AWS Lambda for reconciliation and S3 for state management. It provides both a TUI (Terminal User Interface) and CLI for managing Kubernetes clusters without requiring dedicated infrastructure.

## Development Approach

Work on one task at a time. Focus on completing each task fully before moving to the next.

## Build and Run Commands

### Essential Development Commands
```bash
# Build everything
task build              # Builds both UI and Lambda

# Build specific components
task build:ui          # Build TUI binary (./goman)
task build:lambda      # Build Lambda package (build/lambda-aws-controller.zip)

# Run the application
task run               # Build and run TUI
./goman               # Run TUI directly
./goman init          # Initialize AWS infrastructure
./goman cluster create <name> --region=<region> --mode=<dev|ha>

# Testing
task test             # Run all tests
task test:coverage    # Run tests with coverage
task test:e2e        # Run end-to-end tests
task test:quick      # Run quick component tests
go test ./pkg/cluster/...  # Test specific package

# Code quality
task fmt              # Format code
task lint            # Run golangci-lint
task check           # Run fmt, lint, and test

# Lambda operations
task deploy:lambda    # Deploy Lambda to AWS
task logs:lambda     # View Lambda CloudWatch logs

# Cleanup
task clean           # Remove build artifacts
./goman uninit       # Hidden command to cleanup AWS resources
```

## Important Restrictions

### Immutable Fields
The following cluster fields **cannot be changed** after creation:
- **name**: Cluster name is immutable as it's used as the unique identifier
- **mode**: Cluster mode (dev/ha) cannot be changed as it determines the number of master nodes (1 for dev, 3 for HA)

When attempting to change these fields:
- The UI will show a validation error as a comment at the top of the vim editor
- The changes will not be saved to storage
- Users must create a new cluster if they need a different mode

### Mutable Fields
The following fields **can be changed** after creation:
- **description**: Cluster description
- **region**: AWS region (Note: changing region will provision new instances in the new region)
- **instanceType**: EC2 instance type (will trigger automatic resize of existing instances)
- **k3sVersion**: K3s version (for upgrades)

## Architecture and Code Organization

### Core Provider Abstraction Pattern
The codebase uses a provider pattern to abstract cloud operations. All cloud interactions go through the provider interface:

```
Provider Interface (pkg/provider/provider.go)
    ↓
AWS Implementation (pkg/provider/aws/)
    ├── provider.go         - Main provider implementation
    ├── cached_provider.go  - Singleton caching layer
    ├── compute_service.go  - EC2 operations
    ├── storage_service.go  - S3 operations
    ├── lock_service.go     - DynamoDB distributed locking
    └── function_service.go - Lambda operations
```

Key insight: Always use `GetCachedProvider()` for AWS operations to avoid recreating clients.

### State Management Flow
1. **Local State**: UI creates/modifies cluster state
2. **S3 Storage**: State saved to `s3://goman-{accountID}/clusters/{name}.json`
3. **Lambda Trigger**: S3 event triggers Lambda controller
4. **Reconciliation**: Lambda reads desired state and reconciles with actual AWS resources
5. **Lock Management**: DynamoDB ensures single writer for each cluster

### TUI Component System
The TUI uses a custom component library built on Bubble Tea:

```
pkg/tui/components/
    ├── base.go      - Component interface and base implementation
    ├── form.go      - Professional form with sections and scrolling
    └── [others]     - Input, display, and layout components

pkg/tui/storybook/   - Interactive component showcase
    ├── stories/     - Component demonstrations
    ├── wrappers/    - Keyboard interaction wrappers
    └── cmd/main.go  - Storybook entry point
```

All components implement the `Component` interface with `Init()`, `Update()`, and `View()` methods.

### Lambda Controller Pattern
The Lambda controller (`lambda/controller/`) implements Kubernetes-style reconciliation:

1. **Phase-based processing**: Each cluster operation goes through phases
2. **Idempotent operations**: Can safely retry at any phase
3. **Distributed locking**: Prevents concurrent modifications
4. **Automatic requeueing**: Long operations reschedule themselves

### Critical Files and Their Purpose

- `pkg/cluster/manager.go` - Central cluster management logic, coordinates all operations
- `pkg/provider/aws/provider.go` - AWS provider implementation, all AWS SDK calls
- `lambda/controller/handler.go` - Lambda entry point, S3 event processing
- `pkg/ui/ui_professional.go` - Main TUI implementation with forms
- `pkg/models/cluster.go` - Core data models for clusters and nodes
- `pkg/storage/storage.go` - S3 storage abstraction layer

## Environment Variables and Configuration

```bash
# AWS Configuration
AWS_PROFILE=default           # AWS profile to use
AWS_REGION=ap-south-1         # Primary region for resources

# Provider-specific (optional)
GOMAN_AWS_AMI_ID             # Override default AMI
GOMAN_AWS_INSTANCE_TYPE      # Default: t3.medium
GOMAN_AWS_KEY_PREFIX         # SSH key prefix (default: goman)
GOMAN_DEFAULT_NODE_COUNT     # Default cluster size
GOMAN_K3S_VERSION           # K3s version to install
```

## AWS Resources Created

The system creates these resources automatically:
- **S3 Bucket**: `goman-{accountID}` - State storage
- **DynamoDB Table**: `goman-resource-locks` - Distributed locking
- **Lambda Function**: `goman-controller-{accountID}` - Reconciliation controller
- **IAM Roles**: Lambda execution and SSM instance profiles
- **Security Groups**: Per-cluster network isolation
- **EC2 Instances**: Cluster nodes with SSM agent

## Testing Strategy

1. **Unit Tests**: Test individual components in isolation
2. **Integration Tests**: Test provider implementations with AWS
3. **E2E Tests**: Full cluster lifecycle testing (`scripts/e2e_test.sh`)
4. **Quick Tests**: Component verification (`scripts/quick_test.sh`)

## Error Handling Patterns

- All AWS operations use context with timeout
- Retry logic with exponential backoff for transient failures
- Distributed locking prevents race conditions
- Lambda automatically retries on failure
- State consistency maintained through S3 versioning

## Development Tips

1. **Provider Caching**: Always use `GetCachedProvider()` to avoid recreating AWS clients
2. **Context Usage**: Pass context through all operations for cancellation
3. **State Updates**: Always acquire lock before modifying cluster state
4. **TUI Components**: Use the storybook to test new components (`cd pkg/tui/storybook/cmd && go run main.go`)
5. **Lambda Debugging**: Use `task logs:lambda` to view CloudWatch logs
6. **Resource Cleanup**: Always tag resources with cluster name for cleanup

## Common Troubleshooting

```bash
# Verify AWS setup
aws sts get-caller-identity
aws s3 ls s3://goman-$(aws sts get-caller-identity --query Account --output text)/
aws dynamodb describe-table --table-name goman-resource-locks

# Check Lambda logs
task logs:lambda

# Force cleanup (hidden command)
./goman uninit

# Check resources across regions
task check:resources
```