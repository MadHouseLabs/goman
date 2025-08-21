# Goman - K3s Cluster Manager

A serverless, event-driven TUI application for managing K3s Kubernetes clusters on cloud providers. Features a clean provider abstraction, distributed locking, and Kubernetes-style reconciliation loops.

## 🏗️ Project Structure

```
goman/
├── cmd/                    # Application entry points
│   └── goman/             # Main TUI application
├── pkg/                    # Core packages
│   ├── cluster/           # Cluster management logic
│   ├── config/            # Configuration management
│   ├── models/            # Data models and types
│   ├── provider/          # Provider abstraction
│   │   ├── aws/           # AWS provider implementation
│   │   └── registry/      # Provider registry
│   ├── queue/             # Job queue system (legacy)
│   ├── storage/           # Storage abstraction
│   ├── ui/                # TUI components
│   └── utils/             # Utilities (retry, etc.)
├── lambda/                 # Serverless functions
│   └── controller/        # Reconciliation controller
├── Taskfile.yml           # Task automation
└── ARCHITECTURE.md        # Architecture documentation
```

## 🚀 Quick Start

### Prerequisites

- Go 1.20+
- AWS CLI configured with credentials
- Task (go-task) installed: `brew install go-task/tap/go-task`
- AWS Account with appropriate permissions

### Installation

```bash
# Clone the repository
git clone https://github.com/madhouselabs/goman
cd goman

# Install dependencies
task deps

# Build all binaries and Lambda packages
task build

# Initialize infrastructure
./goman init --non-interactive

# Start the TUI
./goman
```

## 📋 Available Commands

### TUI Mode

```bash
# Start the interactive TUI
./goman
```

### CLI Mode

```bash
# Initialize infrastructure
./goman init --non-interactive

# Check initialization status
./goman status

# Manage clusters via CLI
./goman cluster create <name> --region=<region> --mode=<dev|ha> --wait --json
./goman cluster list [--region=<region>] [--json]
./goman cluster status <name> [--json]
./goman cluster delete <name> [--json]

# List AWS resources
./goman resources list [--region=<region>] [--json]

# Cleanup infrastructure (hidden command)
./goman uninit
```

### Build & Deploy

```bash
# Build everything
task build

# Build specific components
task build:ui        # Build TUI binary
task build:lambda    # Build Lambda package

# Deploy Lambda function
task deploy:lambda
```

### Testing

```bash
# Run quick tests
task test:quick

# Run end-to-end tests
task test:e2e

# Check AWS resources across regions
task check:resources

# Manual test scripts
./scripts/quick_test.sh      # Quick component tests
./scripts/e2e_test.sh        # Full end-to-end test
./scripts/check_resources.sh # Check AWS resources
```

### Development

```bash
# Run tests
task test

# Format code
task fmt

# Run linter
task lint

# Run all checks
task check

# Clean build artifacts
task clean
```

## 🎯 Features

### Cluster Management
- **Create** K3s clusters on AWS EC2
- **Delete** clusters and clean up resources
- **List** all clusters with real-time status
- **Sync** clusters from AWS

### Serverless Processing
- AWS Lambda with Kubernetes-style reconciliation
- S3-triggered event processing
- Distributed locking with DynamoDB
- Automatic retry with exponential backoff
- Context timeouts for all operations

### Architecture

#### Key Features

1. **Provider Abstraction**
   - Clean interface for multi-cloud support
   - AWS implementation with EC2, S3, DynamoDB, Lambda
   - Pluggable architecture for future providers

2. **Reconciliation Controller**
   - Kubernetes-style control loops
   - Phase-based state machine
   - Automatic rescheduling for long operations
   - Distributed locking prevents conflicts

3. **Event-Driven Processing**
   - S3 object changes trigger Lambda
   - No polling or daemon processes
   - Automatic scaling with AWS Lambda

#### Core Components

- **TUI Application** (`cmd/goman`): Interactive interface for cluster management
- **Lambda Controller** (`lambda/controller`): Reconciliation logic
- **Provider System** (`pkg/provider`): Cloud abstraction layer
- **Storage Backend** (`pkg/storage`): S3-based state management
- **Lock Service** (`pkg/provider/aws/lock_service.go`): DynamoDB distributed locks

## 🔧 Configuration

### Environment Variables

```bash
# AWS Configuration
export AWS_PROFILE=myprofile           # AWS profile (default: "default")
export AWS_REGION=ap-south-1          # AWS region (default: "ap-south-1")

# Provider Configuration
export GOMAN_AWS_AMI_ID=ami-xxx       # Override default AMI
export GOMAN_AWS_INSTANCE_TYPE=t3.medium  # Instance type
export GOMAN_AWS_KEY_PREFIX=goman     # SSH key prefix
export GOMAN_DEFAULT_NODE_COUNT=3     # Default cluster size
export GOMAN_K3S_VERSION=v1.28.5+k3s1 # K3s version
```

### Automatic Resources

Goman automatically creates and manages:
- **S3 Bucket**: `goman-{AccountID}`
- **DynamoDB Table**: `goman-resource-locks`
- **Lambda Function**: `goman-cluster-controller`
- **IAM Roles**: As needed for Lambda execution

## 📦 State Management

All state is stored in AWS S3 automatically:

- **Bucket**: `goman-{AccountID}` in ap-south-1 region
- **Structure**: `state/{ProfileName}/clusters/`, `jobs/`, etc.
- **Automatic**: Bucket created on first run
- **Persistent**: State survives local failures
- **Collaborative**: Teams can share state

See [S3_STORAGE.md](S3_STORAGE.md) for details.

## 🧪 Testing

```bash
# Run all tests
task test

# Run with coverage
task test:coverage

# Run specific package tests
go test ./pkg/cluster/...
```

## 🐳 Docker Support

```bash
# Build Docker image
task docker:build

# Run in Docker
task docker:run
```

## 🔍 Troubleshooting

### Run Integration Tests
```bash
./test_integration.sh
```

### Check Lambda Logs
```bash
# View CloudWatch logs for Lambda function
aws logs tail /aws/lambda/goman-cluster-controller --follow
```

### Verify AWS Setup
```bash
# Check AWS credentials
aws sts get-caller-identity

# Check S3 bucket
aws s3 ls s3://goman-$(aws sts get-caller-identity --query Account --output text)/

# Check DynamoDB table
aws dynamodb describe-table --table-name goman-resource-locks

# Check Lambda function
aws lambda get-function --function-name goman-cluster-controller
```

### Clean Build Artifacts
```bash
task clean
```

## 📄 License

MIT License - see LICENSE file for details

## 🤝 Contributing

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## 📞 Support

For issues and questions, please open an issue on GitHub.