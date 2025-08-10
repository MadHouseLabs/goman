# Security Best Practices for Goman

## Overview
Goman is designed with security as a priority. This document outlines the security measures implemented and best practices for deployment.

## Security Features

### 1. Least Privilege IAM Policies
- Lambda functions use custom least-privilege policies instead of `FullAccess` policies
- EC2 instances use IAM instance profiles for SSM access only
- No hardcoded credentials in code

### 2. Network Security
- **No SSH Access**: EC2 instances are managed via AWS Systems Manager Session Manager
- **Restricted API Access**: K3s API server (port 6443) is restricted to VPC internal traffic only
- **Security Groups**: Minimal ingress rules, only allowing necessary cluster communication

### 3. Secure Remote Access
- **Systems Manager**: All remote command execution uses AWS SSM instead of SSH
- **No Key Pairs**: No SSH key pairs are created or managed
- **IAM-based Authentication**: Access control through AWS IAM roles and policies

### 4. State Management Security
- **S3 Bucket Policies**: Restricted access to state buckets
- **Encryption at Rest**: S3 buckets use default encryption
- **DynamoDB Locking**: Prevents concurrent modifications with distributed locks

## IAM Permissions Required

### Lambda Function Role
The Lambda function uses a custom policy with only the necessary permissions:
- **S3**: GetObject, PutObject, DeleteObject, ListBucket (only for goman bucket)
- **DynamoDB**: GetItem, PutItem, DeleteItem, UpdateItem (only for locks table)
- **EC2**: RunInstances, TerminateInstances, DescribeInstances, Security Group management
- **IAM**: PassRole for SSM instance profile only
- **SSM**: SendCommand, GetCommandInvocation for remote execution

### EC2 Instance Profile
Instances use the `goman-ssm-instance-profile` with:
- `AmazonSSMManagedInstanceCore` policy for Systems Manager access

## Deployment Security Checklist

### Before Deployment
- [ ] Review IAM policies for least privilege
- [ ] Ensure S3 bucket has appropriate access policies
- [ ] Verify Lambda function has timeout and memory limits set
- [ ] Check security group rules are restrictive

### During Deployment
- [ ] Monitor CloudTrail for unusual API calls
- [ ] Check Lambda logs for errors or suspicious activity
- [ ] Verify instances are created with correct IAM profiles

### After Deployment
- [ ] Regularly review and rotate IAM credentials
- [ ] Monitor costs to detect unusual resource usage
- [ ] Keep Lambda function and dependencies updated
- [ ] Review security group rules periodically

## Security Improvements Made

1. **Removed SSH Dependencies**: Completely eliminated SSH key pair creation and management
2. **Restricted Network Access**: K3s API no longer exposed to internet (0.0.0.0/0)
3. **Least Privilege IAM**: Replaced FullAccess policies with specific, minimal permissions
4. **Proper Context Usage**: Replaced context.TODO() with proper context propagation
5. **Resource Cleanup**: Implemented proper cleanup of security groups and resources

## Reporting Security Issues

If you discover a security vulnerability, please:
1. Do NOT create a public GitHub issue
2. Email security details to the maintainers
3. Allow time for a patch before public disclosure

## Compliance Notes

- All AWS resources are tagged with `ManagedBy: goman` for tracking
- Resources are created in user-specified regions only
- No cross-account access is configured
- All API calls are logged via CloudTrail

## Future Security Enhancements

- [ ] Add AWS Secrets Manager support for sensitive configuration
- [ ] Implement VPC endpoints for private S3/DynamoDB access
- [ ] Add CloudWatch alerting for suspicious activities
- [ ] Support for customer-managed KMS keys for encryption
- [ ] Implement audit logging for all cluster operations