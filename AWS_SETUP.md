# AWS Setup Guide

## Prerequisites

### 1. AWS Credentials
Configure AWS credentials using one of these methods:

```bash
# Method 1: AWS CLI
aws configure

# Method 2: Environment variables
export AWS_PROFILE=your-profile
export AWS_REGION=ap-south-1  # Default region

# Method 3: AWS IAM Role (for EC2 instances)
# Automatically uses instance role
```

### 2. Required AWS Permissions

The IAM user/role needs these permissions:

#### S3 (State Storage)
- `s3:CreateBucket`
- `s3:GetObject`
- `s3:PutObject`
- `s3:DeleteObject`
- `s3:ListBucket`

#### DynamoDB (Distributed Locking)
- `dynamodb:CreateTable`
- `dynamodb:PutItem`
- `dynamodb:GetItem`
- `dynamodb:DeleteItem`
- `dynamodb:UpdateItem`

#### Lambda (Serverless Controller)
- `lambda:CreateFunction`
- `lambda:UpdateFunctionCode`
- `lambda:InvokeFunction`
- `lambda:GetFunction`
- `lambda:AddPermission`

#### EC2 (Cluster Infrastructure)
- `ec2:RunInstances`
- `ec2:TerminateInstances`
- `ec2:DescribeInstances`
- `ec2:CreateVpc`
- `ec2:CreateSubnet`
- `ec2:CreateSecurityGroup`
- `ec2:AuthorizeSecurityGroupIngress`
- `ec2:CreateKeyPair`
- `ec2:DeleteKeyPair`

#### IAM (Lambda Execution Role)
- `iam:CreateRole`
- `iam:AttachRolePolicy`
- `iam:PassRole`

## Automatic Resource Creation

Goman automatically creates these resources:

### Storage & State
- **S3 Bucket**: `goman-{AccountID}` in your configured region
- **Bucket Structure**: `/clusters/`, `/state/`, `/logs/`

### Distributed Locking
- **DynamoDB Table**: `goman-resource-locks`
- **Billing**: Pay-per-request mode
- **TTL**: Automatic expiration of stale locks

### Serverless Processing
- **Lambda Function**: `goman-cluster-controller`
- **Trigger**: S3 PutObject events
- **Execution Role**: Created automatically with required permissions

## Network Architecture

Each K3s cluster gets:
- **VPC**: Dedicated VPC (10.0.0.0/16)
- **Subnet**: Public subnet for nodes (10.0.1.0/24)
- **Security Group**: Configured for K3s communication
- **Internet Gateway**: For external connectivity

## Cost Considerations

### Pay-per-use Services
- **S3**: ~$0.023 per GB/month
- **DynamoDB**: ~$0.25 per million requests
- **Lambda**: ~$0.20 per million requests + compute time

### EC2 Instances
- **t3.medium**: ~$0.0416/hour (default)
- **Data Transfer**: Varies by region

## Security Best Practices

1. **Use IAM Roles** instead of access keys when possible
2. **Enable S3 Versioning** for state recovery
3. **Restrict Security Groups** to minimum required ports
4. **Rotate Access Keys** regularly
5. **Use SSM Parameter Store** for sensitive data

## Troubleshooting

### Check AWS Configuration
```bash
aws sts get-caller-identity
```

### Verify S3 Bucket
```bash
aws s3 ls s3://goman-$(aws sts get-caller-identity --query Account --output text)/
```

### Check Lambda Function
```bash
aws lambda get-function --function-name goman-cluster-controller
```

### View Lambda Logs
```bash
aws logs tail /aws/lambda/goman-cluster-controller --follow
```

## Clean Up Resources

To remove all Goman resources:

```bash
# Delete S3 bucket (WARNING: Deletes all state!)
aws s3 rb s3://goman-$(aws sts get-caller-identity --query Account --output text) --force

# Delete DynamoDB table
aws dynamodb delete-table --table-name goman-resource-locks

# Delete Lambda function
aws lambda delete-function --function-name goman-cluster-controller

# Delete clusters via UI first to clean up EC2 resources
```