#!/bin/bash

# End-to-End Testing Script for Goman
# This script tests the complete lifecycle of the goman cluster management system

set -e  # Exit on error

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Configuration
TEST_CLUSTER_NAME="test-cluster-$(date +%s)"
TEST_CLUSTER_NAME_HA="test-cluster-ha-$(date +%s)"
PRIMARY_REGION="${AWS_REGION:-ap-south-1}"
SECONDARY_REGION="us-west-1"
LOG_FILE="e2e_test_$(date +%Y%m%d_%H%M%S).log"

# Test tracking variables
TEST_FAILURES=0
TEST_WARNINGS=0
FAILED_TESTS=()
WARNING_TESTS=()

# Function to print colored output
print_info() {
    echo -e "${GREEN}[INFO]${NC} $1" | tee -a "$LOG_FILE"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1" | tee -a "$LOG_FILE"
    ((TEST_FAILURES++))
    FAILED_TESTS+=("$1")
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1" | tee -a "$LOG_FILE"
    ((TEST_WARNINGS++))
    WARNING_TESTS+=("$1")
}

print_section() {
    echo -e "\n${GREEN}========================================${NC}" | tee -a "$LOG_FILE"
    echo -e "${GREEN} $1${NC}" | tee -a "$LOG_FILE"
    echo -e "${GREEN}========================================${NC}\n" | tee -a "$LOG_FILE"
}

# Function to check command exists
check_command() {
    if ! command -v $1 &> /dev/null; then
        print_error "$1 is not installed"
        exit 1
    fi
}

# Function to check AWS credentials
check_aws_credentials() {
    if ! aws sts get-caller-identity &> /dev/null; then
        print_error "AWS credentials not configured"
        exit 1
    fi
    ACCOUNT_ID=$(aws sts get-caller-identity --query Account --output text)
    print_info "Using AWS Account: $ACCOUNT_ID"
}

# Function to verify EC2 instances are terminated
verify_instances_terminated() {
    local region=$1
    local cluster_prefix=$2
    
    print_info "Verifying EC2 instances are terminated in $region for cluster prefix: $cluster_prefix"
    
    # Get instances with the cluster tag
    instances=$(aws ec2 describe-instances \
        --region "$region" \
        --filters "Name=tag:Cluster,Values=${cluster_prefix}*" \
        --query 'Reservations[*].Instances[*].[InstanceId,State.Name,Tags[?Key==`Name`].Value|[0]]' \
        --output json 2>/dev/null || echo "[]")
    
    if [ "$instances" != "[]" ] && [ "$(echo "$instances" | jq -r '.[][]' 2>/dev/null)" != "" ]; then
        echo "$instances" | jq -r '.[][] | @tsv' | while IFS=$'\t' read -r instance_id state name; do
            if [ "$state" != "terminated" ] && [ "$state" != "" ]; then
                print_warning "Instance $instance_id ($name) is in state: $state"
                return 1
            else
                print_info "Instance $instance_id ($name) is properly terminated"
            fi
        done
    else
        print_info "No instances found for cluster prefix $cluster_prefix in $region"
    fi
    
    return 0
}

# Function to cleanup on exit
cleanup() {
    print_section "Cleanup"
    
    # Delete test clusters if they exist
    if ./goman cluster list --json 2>/dev/null | jq -e ".[] | select(.name == \"$TEST_CLUSTER_NAME\")" &>/dev/null; then
        print_info "Deleting test cluster: $TEST_CLUSTER_NAME"
        ./goman cluster delete "$TEST_CLUSTER_NAME" --json || true
    fi
    
    if ./goman cluster list --json 2>/dev/null | jq -e ".[] | select(.name == \"$TEST_CLUSTER_NAME_HA\")" &>/dev/null; then
        print_info "Deleting HA test cluster: $TEST_CLUSTER_NAME_HA"
        ./goman cluster delete "$TEST_CLUSTER_NAME_HA" --json || true
    fi
    
    # Wait for clusters to be deleted
    print_info "Waiting for clusters to be deleted..."
    sleep 30
    
    # Check for any remaining resources
    print_info "Checking for remaining resources in $PRIMARY_REGION..."
    ./goman resources list --region="$PRIMARY_REGION" || true
    
    print_info "Checking for remaining resources in $SECONDARY_REGION..."
    ./goman resources list --region="$SECONDARY_REGION" || true
    
    # Final uninit if infrastructure still exists
    if ./goman status 2>/dev/null | grep -q "Initialized: true"; then
        print_info "Running final uninit to clean up infrastructure..."
        echo "yes" | ./goman uninit || true
    fi
}

# Function to wait for cluster status
wait_for_cluster_status() {
    local cluster_name=$1
    local expected_status=$2
    local timeout=$3
    local elapsed=0
    
    print_info "Waiting for cluster '$cluster_name' to reach status '$expected_status'..."
    
    while [ $elapsed -lt $timeout ]; do
        status=$(./goman cluster status "$cluster_name" --json 2>/dev/null | jq -r '.status' || echo "unknown")
        
        if [ "$status" == "$expected_status" ]; then
            print_info "Cluster '$cluster_name' reached status '$expected_status'"
            return 0
        elif [ "$status" == "error" ]; then
            print_error "Cluster '$cluster_name' entered error state"
            return 1
        fi
        
        sleep 10
        elapsed=$((elapsed + 10))
        echo -n "." | tee -a "$LOG_FILE"
    done
    
    print_error "Timeout waiting for cluster '$cluster_name' to reach status '$expected_status'"
    return 1
}

# Main test execution
main() {
    print_section "Goman End-to-End Test Suite"
    print_info "Test started at: $(date)"
    print_info "Log file: $LOG_FILE"
    
    # Prerequisites check
    print_section "Prerequisites Check"
    check_command "aws"
    check_command "jq"
    check_command "task"
    check_aws_credentials
    
    # Build phase
    print_section "Build Phase"
    print_info "Building goman binary..."
    if ! task build; then
        print_error "Failed to build goman binary"
        exit 1
    fi
    
    print_info "Building Lambda binary..."
    if ! task build:lambda; then
        print_error "Failed to build Lambda binary"
        exit 1
    fi
    
    # Cleanup any existing infrastructure
    print_section "Initial Cleanup"
    print_info "Running uninit to ensure clean state..."
    echo "yes" | ./goman uninit || true
    
    # Verify no resources exist before starting
    print_info "Verifying clean state in $PRIMARY_REGION..."
    verify_instances_terminated "$PRIMARY_REGION" "test-cluster"
    print_info "Verifying clean state in $SECONDARY_REGION..."
    verify_instances_terminated "$SECONDARY_REGION" "test-cluster"
    
    # Initialization phase (this will deploy Lambda as part of init)
    print_section "Infrastructure Initialization"
    print_info "Initializing Goman infrastructure (includes Lambda deployment)..."
    if ! ./goman init --non-interactive; then
        print_error "Failed to initialize infrastructure"
        exit 1
    fi
    
    # Verify initialization
    print_info "Verifying initialization status..."
    if ! ./goman status; then
        print_error "Failed to get infrastructure status"
    fi
    
    # Test cluster creation - Dev mode
    print_section "Cluster Creation - Dev Mode"
    print_info "Creating dev mode cluster in $PRIMARY_REGION..."
    ./goman cluster create "$TEST_CLUSTER_NAME" \
        --region="$PRIMARY_REGION" \
        --mode=dev \
        --instance-type=t3.micro \
        --json | tee -a "$LOG_FILE"
    
    # Wait for cluster to be ready
    if ! wait_for_cluster_status "$TEST_CLUSTER_NAME" "running" 600; then
        print_error "Dev cluster failed to reach running state"
    fi
    
    # List clusters
    print_section "Cluster Listing"
    print_info "Listing all clusters..."
    ./goman cluster list --json | jq '.' | tee -a "$LOG_FILE"
    
    print_info "Listing clusters in $PRIMARY_REGION..."
    ./goman cluster list --region="$PRIMARY_REGION" --json | jq '.' | tee -a "$LOG_FILE"
    
    # Get cluster status
    print_section "Cluster Status Check"
    print_info "Getting detailed status of $TEST_CLUSTER_NAME..."
    ./goman cluster status "$TEST_CLUSTER_NAME" --json | jq '.' | tee -a "$LOG_FILE"
    
    # Check resources in primary region
    print_section "Resource Verification - Primary Region"
    print_info "Checking resources in $PRIMARY_REGION..."
    ./goman resources list --region="$PRIMARY_REGION" | tee -a "$LOG_FILE"
    
    # Test cluster creation - HA mode in secondary region
    print_section "Cluster Creation - HA Mode"
    print_info "Creating HA mode cluster in $SECONDARY_REGION..."
    ./goman cluster create "$TEST_CLUSTER_NAME_HA" \
        --region="$SECONDARY_REGION" \
        --mode=ha \
        --instance-type=t3.small \
        --json | tee -a "$LOG_FILE"
    
    # Wait for HA cluster to be ready
    if ! wait_for_cluster_status "$TEST_CLUSTER_NAME_HA" "running" 900; then
        print_error "HA cluster failed to reach running state"
    fi
    
    # Check resources in secondary region
    print_section "Resource Verification - Secondary Region"
    print_info "Checking resources in $SECONDARY_REGION..."
    ./goman resources list --region="$SECONDARY_REGION" | tee -a "$LOG_FILE"
    
    # List all clusters across regions
    print_section "Cross-Region Cluster Verification"
    print_info "Listing all clusters across regions..."
    ./goman cluster list --json | jq '.' | tee -a "$LOG_FILE"
    
    # Test Lambda reconciliation
    print_section "Lambda Reconciliation Test"
    print_info "Waiting for Lambda reconciliation cycle..."
    sleep 30
    
    print_info "Checking cluster status after reconciliation..."
    ./goman cluster status "$TEST_CLUSTER_NAME" --json | jq '.status' | tee -a "$LOG_FILE"
    ./goman cluster status "$TEST_CLUSTER_NAME_HA" --json | jq '.status' | tee -a "$LOG_FILE"
    
    # Delete clusters
    print_section "Cluster Deletion"
    print_info "Deleting dev mode cluster..."
    ./goman cluster delete "$TEST_CLUSTER_NAME" --json | tee -a "$LOG_FILE"
    
    print_info "Deleting HA mode cluster..."
    ./goman cluster delete "$TEST_CLUSTER_NAME_HA" --json | tee -a "$LOG_FILE"
    
    # Wait for deletion
    print_info "Waiting for clusters to be deleted..."
    sleep 60
    
    # Verify deletion
    print_section "Deletion Verification"
    print_info "Verifying clusters are deleted..."
    
    cluster_count=$(./goman cluster list --json 2>/dev/null | jq '. | length' || echo "0")
    if [ "$cluster_count" -eq 0 ]; then
        print_info "All test clusters successfully deleted from tracking"
    else
        print_warning "$cluster_count cluster(s) still exist in tracking after deletion"
        ./goman cluster list --json | jq '.' | tee -a "$LOG_FILE"
    fi
    
    # Verify EC2 instances are actually terminated
    print_section "EC2 Instance Termination Verification"
    print_info "Verifying EC2 instances are terminated in $PRIMARY_REGION..."
    if ! verify_instances_terminated "$PRIMARY_REGION" "$TEST_CLUSTER_NAME"; then
        print_error "EC2 instances in $PRIMARY_REGION are not properly terminated after cluster deletion"
    fi
    
    print_info "Verifying EC2 instances are terminated in $SECONDARY_REGION..."
    if ! verify_instances_terminated "$SECONDARY_REGION" "$TEST_CLUSTER_NAME_HA"; then
        print_error "EC2 instances in $SECONDARY_REGION are not properly terminated after cluster deletion"
    fi
    
    # Additional wait for instance termination to complete
    print_info "Waiting for instance termination to complete..."
    sleep 30
    
    # Final verification of instance states
    print_info "Final EC2 instance state check..."
    aws ec2 describe-instances \
        --region "$PRIMARY_REGION" \
        --filters "Name=tag:Cluster,Values=$TEST_CLUSTER_NAME" \
        --query 'Reservations[*].Instances[*].[InstanceId,State.Name]' \
        --output table 2>/dev/null || true
    
    aws ec2 describe-instances \
        --region "$SECONDARY_REGION" \
        --filters "Name=tag:Cluster,Values=$TEST_CLUSTER_NAME_HA" \
        --query 'Reservations[*].Instances[*].[InstanceId,State.Name]' \
        --output table 2>/dev/null || true
    
    # Check for remaining resources
    print_section "Final Resource Check"
    print_info "Checking for remaining resources in $PRIMARY_REGION..."
    ./goman resources list --region="$PRIMARY_REGION" | tee -a "$LOG_FILE"
    
    print_info "Checking for remaining resources in $SECONDARY_REGION..."
    ./goman resources list --region="$SECONDARY_REGION" | tee -a "$LOG_FILE"
    
    # Final cleanup
    print_section "Final Infrastructure Cleanup"
    print_info "Running uninit to remove all infrastructure..."
    if ! echo "yes" | ./goman uninit; then
        print_error "Final uninit failed"
    fi
    
    # Generate test report
    print_section "Test Summary Report"
    
    echo -e "\n${GREEN}======== E2E TEST REPORT ========${NC}" | tee -a "$LOG_FILE"
    echo "Test ended at: $(date)" | tee -a "$LOG_FILE"
    echo "Log file: $LOG_FILE" | tee -a "$LOG_FILE"
    echo "" | tee -a "$LOG_FILE"
    
    # Overall status
    if [ $TEST_FAILURES -eq 0 ] && [ $TEST_WARNINGS -eq 0 ]; then
        echo -e "${GREEN}✓ ALL TESTS PASSED${NC}" | tee -a "$LOG_FILE"
        echo "Status: SUCCESS" | tee -a "$LOG_FILE"
    elif [ $TEST_FAILURES -eq 0 ] && [ $TEST_WARNINGS -gt 0 ]; then
        echo -e "${YELLOW}⚠ TESTS PASSED WITH WARNINGS${NC}" | tee -a "$LOG_FILE"
        echo "Status: SUCCESS WITH WARNINGS" | tee -a "$LOG_FILE"
    else
        echo -e "${RED}✗ TESTS FAILED${NC}" | tee -a "$LOG_FILE"
        echo "Status: FAILURE" | tee -a "$LOG_FILE"
    fi
    
    echo "" | tee -a "$LOG_FILE"
    echo "Test Statistics:" | tee -a "$LOG_FILE"
    echo "  - Errors: $TEST_FAILURES" | tee -a "$LOG_FILE"
    echo "  - Warnings: $TEST_WARNINGS" | tee -a "$LOG_FILE"
    
    # List failures if any
    if [ $TEST_FAILURES -gt 0 ]; then
        echo "" | tee -a "$LOG_FILE"
        echo -e "${RED}Failed Tests:${NC}" | tee -a "$LOG_FILE"
        for failure in "${FAILED_TESTS[@]}"; do
            echo "  ✗ $failure" | tee -a "$LOG_FILE"
        done
    fi
    
    # List warnings if any
    if [ $TEST_WARNINGS -gt 0 ]; then
        echo "" | tee -a "$LOG_FILE"
        echo -e "${YELLOW}Warnings:${NC}" | tee -a "$LOG_FILE"
        for warning in "${WARNING_TESTS[@]}"; do
            echo "  ⚠ $warning" | tee -a "$LOG_FILE"
        done
    fi
    
    echo "" | tee -a "$LOG_FILE"
    echo "================================" | tee -a "$LOG_FILE"
    
    # Exit with appropriate code
    if [ $TEST_FAILURES -gt 0 ]; then
        exit 1
    else
        exit 0
    fi
}

# Set trap for cleanup on exit
trap cleanup EXIT

# Run main test
main "$@"