#!/bin/bash

# Cleanup script to completely remove a cluster from AWS
# Usage: ./cleanup_cluster.sh <cluster-name>

set -e

CLUSTER_NAME=$1
REGION=${AWS_REGION:-ap-south-1}
ACCOUNT_ID=$(aws sts get-caller-identity --query Account --output text)

if [ -z "$CLUSTER_NAME" ]; then
    echo "Usage: $0 <cluster-name>"
    exit 1
fi

echo "🧹 Starting cleanup for cluster: $CLUSTER_NAME"
echo "Region: $REGION"
echo "Account: $ACCOUNT_ID"
echo ""

# 1. Delete S3 files
echo "📦 Deleting S3 files..."
aws s3 rm "s3://goman-${ACCOUNT_ID}/clusters/${CLUSTER_NAME}/" --recursive --region "$REGION" 2>/dev/null || echo "  No S3 files found"

# 2. List and terminate EC2 instances
echo "🖥️  Finding EC2 instances..."
INSTANCE_IDS=$(aws ec2 describe-instances \
    --region "$REGION" \
    --filters "Name=tag:ClusterName,Values=${CLUSTER_NAME}" \
              "Name=instance-state-name,Values=pending,running,stopping,stopped" \
    --query 'Reservations[*].Instances[*].InstanceId' \
    --output text)

if [ -n "$INSTANCE_IDS" ]; then
    echo "  Found instances: $INSTANCE_IDS"
    echo "  Terminating instances..."
    aws ec2 terminate-instances --instance-ids $INSTANCE_IDS --region "$REGION" > /dev/null
    echo "  ✓ Termination initiated"
    
    # Wait for termination
    echo "  Waiting for instances to terminate..."
    aws ec2 wait instance-terminated --instance-ids $INSTANCE_IDS --region "$REGION" 2>/dev/null || true
    echo "  ✓ Instances terminated"
else
    echo "  No instances found"
fi

# 3. Delete DynamoDB locks
echo "🔒 Removing DynamoDB locks..."
LOCK_KEY="cluster-${CLUSTER_NAME}"
aws dynamodb delete-item \
    --table-name goman-resource-locks \
    --key "{\"ResourceID\": {\"S\": \"$LOCK_KEY\"}}" \
    --region "$REGION" 2>/dev/null || echo "  No lock found"

# 4. Delete security groups (if any)
echo "🔐 Finding security groups..."
SG_IDS=$(aws ec2 describe-security-groups \
    --region "$REGION" \
    --filters "Name=tag:ClusterName,Values=${CLUSTER_NAME}" \
    --query 'SecurityGroups[*].GroupId' \
    --output text)

if [ -n "$SG_IDS" ]; then
    echo "  Found security groups: $SG_IDS"
    for sg in $SG_IDS; do
        echo "  Deleting security group: $sg"
        aws ec2 delete-security-group --group-id "$sg" --region "$REGION" 2>/dev/null || echo "    Failed (may have dependencies)"
    done
else
    echo "  No security groups found"
fi

# 5. Delete SSH key pair (if exists)
echo "🔑 Removing SSH key pair..."
KEY_NAME="goman-${CLUSTER_NAME}"
aws ec2 delete-key-pair --key-name "$KEY_NAME" --region "$REGION" 2>/dev/null || echo "  No key pair found"

echo ""
echo "✅ Cleanup complete for cluster: $CLUSTER_NAME"
echo ""
echo "Summary:"
echo "  - S3 files deleted from s3://goman-${ACCOUNT_ID}/clusters/${CLUSTER_NAME}/"
echo "  - EC2 instances terminated"
echo "  - DynamoDB lock removed"
echo "  - Security groups cleaned up"
echo "  - SSH key pair deleted"