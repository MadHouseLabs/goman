#!/bin/bash

# Resource Checker Script for Goman
# Checks for Goman-related AWS resources across multiple regions

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Default regions to check
REGIONS="${REGIONS:-ap-south-1 us-east-1 us-west-1 us-west-2 eu-west-1}"

# Get AWS account ID
ACCOUNT_ID=$(aws sts get-caller-identity --query Account --output text 2>/dev/null || echo "unknown")

print_header() {
    echo -e "\n${BLUE}========================================${NC}"
    echo -e "${BLUE} $1${NC}"
    echo -e "${BLUE}========================================${NC}"
}

print_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Function to check EC2 instances in a region
check_ec2_instances() {
    local region=$1
    print_info "Checking EC2 instances in $region..."
    
    instances=$(aws ec2 describe-instances \
        --region "$region" \
        --filters "Name=tag:ManagedBy,Values=goman" \
        --query "Reservations[*].Instances[*].[InstanceId,State.Name,Tags[?Key=='Name'].Value|[0],InstanceType,LaunchTime]" \
        --output json 2>/dev/null || echo "[]")
    
    if [ "$instances" != "[]" ] && [ -n "$instances" ]; then
        echo -e "${GREEN}  Found instances:${NC}"
        echo "$instances" | jq -r '.[][] | @tsv' | while IFS=$'\t' read -r id state name type launch; do
            echo "    - $id: $name ($type) - $state - Launched: $launch"
        done
    else
        echo "    No instances found"
    fi
}

# Function to check VPCs in a region
check_vpcs() {
    local region=$1
    print_info "Checking VPCs in $region..."
    
    vpcs=$(aws ec2 describe-vpcs \
        --region "$region" \
        --filters "Name=tag:ManagedBy,Values=goman" \
        --query "Vpcs[*].[VpcId,CidrBlock,Tags[?Key=='Name'].Value|[0]]" \
        --output json 2>/dev/null || echo "[]")
    
    if [ "$vpcs" != "[]" ] && [ -n "$vpcs" ]; then
        echo -e "${GREEN}  Found VPCs:${NC}"
        echo "$vpcs" | jq -r '.[] | @tsv' | while IFS=$'\t' read -r id cidr name; do
            echo "    - $id: $name ($cidr)"
        done
    else
        echo "    No VPCs found"
    fi
}

# Function to check security groups in a region
check_security_groups() {
    local region=$1
    print_info "Checking Security Groups in $region..."
    
    sgs=$(aws ec2 describe-security-groups \
        --region "$region" \
        --filters "Name=tag:ManagedBy,Values=goman" \
        --query "SecurityGroups[*].[GroupId,GroupName,Description]" \
        --output json 2>/dev/null || echo "[]")
    
    if [ "$sgs" != "[]" ] && [ -n "$sgs" ]; then
        echo -e "${GREEN}  Found Security Groups:${NC}"
        echo "$sgs" | jq -r '.[] | @tsv' | while IFS=$'\t' read -r id name desc; do
            echo "    - $id: $name - $desc"
        done
    else
        echo "    No Security Groups found"
    fi
}

# Function to check S3 buckets
check_s3_buckets() {
    print_info "Checking S3 buckets..."
    
    bucket_name="goman-state-${ACCOUNT_ID}"
    
    if aws s3api head-bucket --bucket "$bucket_name" 2>/dev/null; then
        echo -e "${GREEN}  Found S3 bucket: $bucket_name${NC}"
        
        # List objects in the bucket
        object_count=$(aws s3api list-objects-v2 --bucket "$bucket_name" --query 'KeyCount' --output text 2>/dev/null || echo "0")
        echo "    Objects in bucket: $object_count"
        
        # Show cluster directories if any
        clusters=$(aws s3api list-objects-v2 --bucket "$bucket_name" --prefix "clusters/" --delimiter "/" --query 'CommonPrefixes[*].Prefix' --output text 2>/dev/null || echo "")
        if [ -n "$clusters" ]; then
            echo "    Cluster states:"
            for cluster in $clusters; do
                echo "      - $cluster"
            done
        fi
    else
        echo "    No S3 bucket found"
    fi
}

# Function to check Lambda functions
check_lambda_functions() {
    local region=$1
    print_info "Checking Lambda functions in $region..."
    
    function_name="goman-controller-${ACCOUNT_ID}"
    
    if aws lambda get-function --function-name "$function_name" --region "$region" &>/dev/null; then
        echo -e "${GREEN}  Found Lambda function: $function_name${NC}"
        
        # Get function details
        runtime=$(aws lambda get-function-configuration --function-name "$function_name" --region "$region" --query 'Runtime' --output text 2>/dev/null)
        last_modified=$(aws lambda get-function-configuration --function-name "$function_name" --region "$region" --query 'LastModified' --output text 2>/dev/null)
        
        echo "    Runtime: $runtime"
        echo "    Last modified: $last_modified"
    else
        echo "    No Lambda function found"
    fi
}

# Function to check DynamoDB tables
check_dynamodb_tables() {
    local region=$1
    print_info "Checking DynamoDB tables in $region..."
    
    table_name="goman-locks"
    
    if aws dynamodb describe-table --table-name "$table_name" --region "$region" &>/dev/null; then
        echo -e "${GREEN}  Found DynamoDB table: $table_name${NC}"
        
        # Get table details
        status=$(aws dynamodb describe-table --table-name "$table_name" --region "$region" --query 'Table.TableStatus' --output text 2>/dev/null)
        item_count=$(aws dynamodb describe-table --table-name "$table_name" --region "$region" --query 'Table.ItemCount' --output text 2>/dev/null)
        
        echo "    Status: $status"
        echo "    Item count: $item_count"
    else
        echo "    No DynamoDB table found"
    fi
}

# Function to check IAM roles
check_iam_roles() {
    print_info "Checking IAM roles..."
    
    roles=("goman-lambda-role" "goman-ssm-instance-role")
    
    for role in "${roles[@]}"; do
        if aws iam get-role --role-name "$role" &>/dev/null; then
            echo -e "${GREEN}  Found IAM role: $role${NC}"
            
            # Get attached policies
            policies=$(aws iam list-attached-role-policies --role-name "$role" --query 'AttachedPolicies[*].PolicyName' --output text 2>/dev/null)
            if [ -n "$policies" ]; then
                echo "    Attached policies: $policies"
            fi
        else
            echo "    IAM role not found: $role"
        fi
    done
}

# Main function
main() {
    print_header "Goman AWS Resource Check"
    echo "Account ID: $ACCOUNT_ID"
    echo "Checking regions: $REGIONS"
    
    # Check global resources
    print_header "Global Resources"
    check_s3_buckets
    check_iam_roles
    
    # Check regional resources
    for region in $REGIONS; do
        print_header "Region: $region"
        check_ec2_instances "$region"
        check_vpcs "$region"
        check_security_groups "$region"
        check_lambda_functions "$region"
        check_dynamodb_tables "$region"
    done
    
    print_header "Resource Check Complete"
}

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --regions)
            REGIONS="$2"
            shift 2
            ;;
        --help)
            echo "Usage: $0 [--regions \"region1 region2 ...\"]"
            echo "Check for Goman-related AWS resources across regions"
            echo ""
            echo "Options:"
            echo "  --regions    Space-separated list of regions to check"
            echo "               Default: ap-south-1 us-east-1 us-west-1 us-west-2 eu-west-1"
            echo "  --help       Show this help message"
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            exit 1
            ;;
    esac
done

# Run main function
main