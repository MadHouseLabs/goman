# S3 Storage for Goman

Goman uses AWS S3 for all state storage, providing centralized, persistent, and collaborative state management for K3s clusters.

## Overview

- **Automatic S3 Storage**: All state is stored in S3 (no local storage option)
- **Standardized Bucket**: `goman-{AccountID}` (e.g., `goman-123456789012`)
- **Fixed Region**: `ap-south-1` (Mumbai, India)
- **Automatic Setup**: Bucket is created automatically if it doesn't exist
- **Profile-based Organization**: Each AWS profile gets its own namespace

## Configuration

### Required Setup

You only need to set your AWS profile:

```bash
export AWS_PROFILE=myprofile  # Optional, defaults to "default"
```

That's it! Goman will:
1. Get your AWS account ID automatically
2. Create bucket `goman-{AccountID}` in ap-south-1 if it doesn't exist
3. Enable versioning for data safety
4. Store all state under `state/{ProfileName}/`

### S3 Bucket Structure

```
s3://goman-123456789012/     # Your account-specific bucket in ap-south-1
└── state/
    └── myprofile/           # Your AWS profile name
        ├── config.json      # Application configuration
        ├── clusters/        # Cluster states
        │   ├── clusters.json
        │   └── *.state.json
        └── jobs/            # Background jobs
            └── *.json
```

## Usage Examples

### Basic Usage
```bash
# Set your AWS profile
export AWS_PROFILE=production

# Run goman - automatically uses S3
./goman
```

### Multiple Environments
Different profiles get separate namespaces in the same bucket:

```bash
# Development environment
export AWS_PROFILE=dev
./goman  # Uses s3://goman-123456789012/state/dev/

# Production environment  
export AWS_PROFILE=prod
./goman  # Uses s3://goman-123456789012/state/prod/
```

### Team Collaboration
Team members using the same AWS account and profile automatically share state:

```bash
# Team member 1
export AWS_PROFILE=team
./goman

# Team member 2
export AWS_PROFILE=team
./goman
# Both use s3://goman-{AccountID}/state/team/
```

## IAM Permissions

Your AWS credentials need these permissions:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "sts:GetCallerIdentity"
      ],
      "Resource": "*"
    },
    {
      "Effect": "Allow",
      "Action": [
        "s3:CreateBucket",
        "s3:HeadBucket",
        "s3:ListBucket",
        "s3:GetBucketLocation",
        "s3:GetBucketVersioning",
        "s3:PutBucketVersioning"
      ],
      "Resource": "arn:aws:s3:::goman-*"
    },
    {
      "Effect": "Allow",
      "Action": [
        "s3:GetObject",
        "s3:PutObject",
        "s3:DeleteObject",
        "s3:GetObjectVersion",
        "s3:ListObjectsV2"
      ],
      "Resource": "arn:aws:s3:::goman-*/*"
    },
    {
      "Effect": "Allow",
      "Action": [
        "ec2:*"
      ],
      "Resource": "*"
    }
  ]
}
```

## Migration from Local State

If you have existing local state in `.state/`:

```bash
# Set your AWS profile
export AWS_PROFILE=myprofile

# Get your AWS account ID
ACCOUNT_ID=$(aws sts get-caller-identity --query Account --output text)

# Copy local state to S3
aws s3 sync .state/ s3://goman-${ACCOUNT_ID}/state/${AWS_PROFILE}/ --region ap-south-1

# Remove local state (optional)
rm -rf .state/
```

## Benefits

1. **Zero Configuration**: Works automatically with just AWS credentials
2. **Persistent State**: Survives local machine failures
3. **Team Collaboration**: Shared state for teams
4. **Versioning**: Automatic backup with S3 versioning
5. **Account Isolation**: Each AWS account has its own bucket
6. **Profile Separation**: Different environments are isolated

## Troubleshooting

### Access Denied
Ensure your AWS credentials have the required S3 and STS permissions listed above.

### Bucket Creation Failed
- Check that your credentials have `s3:CreateBucket` permission
- Verify you're not hitting S3 bucket limits (100 buckets per account)
- The bucket name `goman-{AccountID}` should be globally unique

### Region Issues
All operations use `ap-south-1` (Mumbai). If you need to verify:
```bash
aws s3 ls s3://goman-$(aws sts get-caller-identity --query Account --output text)/ --region ap-south-1
```

### Connection Issues
- Verify network connectivity to AWS
- Check AWS credentials are valid: `aws sts get-caller-identity`
- Ensure your network allows HTTPS traffic to S3

## Important Notes

- **No Local Storage**: All state is in S3, ensuring consistency
- **Automatic Creation**: S3 bucket is created on first run
- **Mumbai Region**: All resources are in ap-south-1 for low latency
- **Cost**: S3 storage costs apply (typically minimal for state files)