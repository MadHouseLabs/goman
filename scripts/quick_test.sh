#!/bin/bash

# Quick Test Script for Goman
# Tests individual components without full E2E flow

set -e

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

print_test() {
    echo -e "\n${GREEN}[TEST]${NC} $1"
}

print_pass() {
    echo -e "${GREEN}  ✓ $1${NC}"
}

print_fail() {
    echo -e "${RED}  ✗ $1${NC}"
}

print_info() {
    echo -e "  $1"
}

# Test build
test_build() {
    print_test "Testing build process"
    
    if task build:ui &>/dev/null; then
        print_pass "UI build successful"
    else
        print_fail "UI build failed"
        return 1
    fi
    
    if task build:lambda &>/dev/null; then
        print_pass "Lambda build successful"
    else
        print_fail "Lambda build failed"
        return 1
    fi
}

# Test initialization check
test_init_check() {
    print_test "Testing initialization status"
    
    if ./goman status &>/dev/null; then
        print_pass "Initialization check passed"
        ./goman status | sed 's/^/  /'
    else
        print_fail "Not initialized"
        print_info "Run: ./goman init --non-interactive"
        return 1
    fi
}

# Test cluster CLI commands
test_cluster_commands() {
    print_test "Testing cluster CLI commands"
    
    # Test list command
    if ./goman cluster list --json &>/dev/null; then
        print_pass "Cluster list command works"
        cluster_count=$(./goman cluster list --json | jq '. | length')
        print_info "Found $cluster_count clusters"
    else
        print_fail "Cluster list command failed"
        return 1
    fi
    
    # Test create command help
    if ./goman cluster create 2>&1 | grep -q "Usage"; then
        print_pass "Cluster create help works"
    else
        print_fail "Cluster create help failed"
    fi
    
    # Test delete command help
    if ./goman cluster delete 2>&1 | grep -q "Usage"; then
        print_pass "Cluster delete help works"
    else
        print_fail "Cluster delete help failed"
    fi
    
    # Test status command help
    if ./goman cluster status 2>&1 | grep -q "Usage"; then
        print_pass "Cluster status help works"
    else
        print_fail "Cluster status help failed"
    fi
}

# Test resources command
test_resources_command() {
    print_test "Testing resources command"
    
    if ./goman resources list --region=ap-south-1 &>/dev/null; then
        print_pass "Resources list command works"
    else
        print_fail "Resources list command failed"
        return 1
    fi
}

# Test JSON output
test_json_output() {
    print_test "Testing JSON output format"
    
    # Test cluster list JSON
    if ./goman cluster list --json 2>/dev/null | jq empty 2>/dev/null; then
        print_pass "Cluster list JSON is valid"
    else
        print_fail "Cluster list JSON is invalid"
        return 1
    fi
}

# Test Lambda deployment
test_lambda_deployment() {
    print_test "Testing Lambda deployment"
    
    if task deploy:lambda &>/dev/null; then
        print_pass "Lambda deployment successful"
    else
        print_fail "Lambda deployment failed"
        print_info "Make sure Lambda function exists first"
        return 1
    fi
}

# Main test runner
main() {
    echo -e "${GREEN}========================================${NC}"
    echo -e "${GREEN} Goman Quick Test Suite${NC}"
    echo -e "${GREEN}========================================${NC}"
    
    failed_tests=0
    total_tests=0
    
    # Run tests
    tests=(
        "test_build"
        "test_init_check"
        "test_cluster_commands"
        "test_resources_command"
        "test_json_output"
    )
    
    for test in "${tests[@]}"; do
        total_tests=$((total_tests + 1))
        if ! $test; then
            failed_tests=$((failed_tests + 1))
        fi
    done
    
    # Optional tests (don't fail if they don't work)
    print_test "Testing optional components"
    if test_lambda_deployment 2>/dev/null; then
        print_pass "Lambda deployment available"
    else
        print_info "Lambda deployment not available (OK)"
    fi
    
    # Summary
    echo -e "\n${GREEN}========================================${NC}"
    echo -e "${GREEN} Test Summary${NC}"
    echo -e "${GREEN}========================================${NC}"
    
    passed_tests=$((total_tests - failed_tests))
    if [ $failed_tests -eq 0 ]; then
        echo -e "${GREEN}All tests passed! ($passed_tests/$total_tests)${NC}"
        exit 0
    else
        echo -e "${RED}Some tests failed: $failed_tests/$total_tests${NC}"
        echo -e "${GREEN}Passed: $passed_tests/$total_tests${NC}"
        exit 1
    fi
}

# Parse arguments
case "${1:-}" in
    --help)
        echo "Usage: $0 [test_name]"
        echo ""
        echo "Run quick tests for Goman components"
        echo ""
        echo "Available tests:"
        echo "  test_build            - Test build process"
        echo "  test_init_check       - Test initialization status"
        echo "  test_cluster_commands - Test cluster CLI commands"
        echo "  test_resources_command - Test resources command"
        echo "  test_json_output      - Test JSON output format"
        echo "  test_lambda_deployment - Test Lambda deployment"
        echo ""
        echo "Run without arguments to run all tests"
        exit 0
        ;;
    "")
        # Run all tests
        main
        ;;
    *)
        # Run specific test
        if declare -f "$1" > /dev/null; then
            $1
        else
            echo "Unknown test: $1"
            echo "Run with --help to see available tests"
            exit 1
        fi
        ;;
esac