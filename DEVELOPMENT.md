# Development Guidelines

## Project Overview
Goman is a serverless Kubernetes cluster management tool using AWS Lambda for reconciliation, S3 for state storage, and Systems Manager for agentless EC2 management.

## Critical Design Principles

### 1. State Management
- **Single source of truth**: S3 bucket `goman-{accountID}`
- **File format**: JSON only at `clusters/{cluster-name}.json`
- **No YAML files**: Removed for consistency
- **Use cluster names**: Not IDs in file paths

### 2. Security
- **No SSH keys**: Use Systems Manager instead
- **Least privilege IAM**: Never use FullAccess policies
- **Agentless**: SSM for remote execution

### 3. AWS Resources
- **Standardized naming**:
  - S3: `goman-{accountID}`
  - Lambda: `goman-cluster-controller`
  - DynamoDB: `goman-resource-locks`
  - IAM roles: `goman-lambda-role-{accountID}`, `goman-ssm-instance-role`

### 4. Development Practices
- **Use task commands**: Not make or direct go commands
- **Clean initialization**: `goman uninit` removes ALL resources
- **No backward compatibility**: Fresh start approach

## Common Tasks

### Building
```bash
task build:ui          # Build CLI
task build:lambda:aws  # Build Lambda
task build            # Build everything
```

### Testing
```bash
./goman init          # Initialize AWS infrastructure
./goman uninit        # Clean up everything (hidden command)
task lambda:logs      # View Lambda logs
```

### Debugging Lambda
```bash
# Check if Lambda was triggered
aws logs tail /aws/lambda/goman-cluster-controller --region ap-south-1

# Check S3 trigger configuration
aws s3api get-bucket-notification-configuration --bucket goman-$(aws sts get-caller-identity --query Account --output text)

# Manually invoke Lambda
echo '{"cluster_name": "test"}' | base64 | aws lambda invoke --function-name goman-cluster-controller --region ap-south-1 --payload file:///dev/stdin /tmp/response.json
```

## Important Files

- `pkg/controller/reconciler.go` - Core reconciliation logic
- `pkg/storage/s3_backend.go` - S3 state management
- `pkg/provider/aws/` - AWS provider implementation
- `cmd/goman/cli.go` - CLI entry point with init/uninit
- `lambda/controller/main.go` - Lambda entry point

## Known Issues & Solutions

### Lambda not triggering
1. Check S3 notification configuration
2. Ensure file path matches `clusters/*.json`
3. Verify Lambda has CloudWatch Logs permissions

### IAM issues
1. Run `goman uninit` to clean up
2. Run `goman init` for fresh setup
3. Lambda role should only have:
   - `AWSLambdaBasicExecutionRole`
   - Custom `goman-lambda-policy-{accountID}`

### State inconsistencies
1. Check S3 for duplicate files
2. Ensure only `clusters/{name}.json` format
3. No `state/` prefix in paths

## Architecture Notes

```
User -> CLI -> S3 -> Lambda -> EC2/SSM
                 ↓
             DynamoDB (locking)
```

- **Event-driven**: S3 triggers Lambda on state changes
- **Reconciliation**: Lambda ensures actual matches desired state
- **Distributed locking**: DynamoDB prevents race conditions
- **Agentless**: SSM for command execution, no SSH

## Do's and Don'ts

### DO:
- Use cluster names in paths
- Clean up with `uninit` before major changes
- Use least-privilege IAM policies
- Test Lambda triggers with file uploads
- Use task commands for building

### DON'T:
- Create YAML files
- Use SSH keys or key pairs
- Add FullAccess IAM policies
- Use nested state/ paths
- Assume backward compatibility

## Commit Message Format
```
feat: Add parallel node creation
fix: Correct S3 trigger paths
docs: Update architecture documentation
refactor: Simplify state management
```

## Testing Checklist
- [ ] Run `goman init` successfully
- [ ] Create a cluster via TUI
- [ ] Verify Lambda triggered (check CloudWatch logs)
- [ ] Verify EC2 instances created
- [ ] Delete cluster via TUI
- [ ] Verify instances terminated
- [ ] Run `goman uninit` cleanly