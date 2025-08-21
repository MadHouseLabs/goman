# Learnings and Past Issues

This document captures important learnings from past debugging sessions and issues that have been resolved.

## Resolved Issues and Their Fixes

### 1. Lambda Reconciliation Getting Stuck
**Problem**: Clusters would get stuck in "Installing" or "Provisioning" state indefinitely.

**Root Cause**: 
- Lambda couldn't requeue itself for continued processing
- RECONCILE_QUEUE_URL environment variable was not being set
- SQS event source mapping was disabled

**Solution**:
- Modified `setupSQSQueue()` in `pkg/provider/aws/provider.go` to preserve existing environment variables when updating Lambda configuration
- Added logic to check and enable disabled SQS event source mappings
- Ensured proper merging of environment variables instead of replacement

**Key Learning**: Always preserve existing Lambda environment variables when updating configuration.

### 2. Metrics Not Loading in UI
**Problem**: Metrics section showed no data even for running clusters.

**Root Cause**:
- Node name mismatch between config and status files
- Config used "k3s-cluster-xxx-master" while status used "k3s-cluster-xxx-master-0"
- Node IDs weren't being populated in the cluster state

**Solution**:
- Added fallback logic in `pkg/storage/cluster_types.go` to check both naming patterns
- Try exact match first, then try with suffix patterns (-0 for masters, -N for workers)

**Key Learning**: Be flexible with naming patterns when matching resources across different components.

### 3. Status Flickering Between Running and Creating
**Problem**: Cluster would show as "running" then flicker back to "creating" state.

**Root Cause**:
- Stale status files with `deletion_requested` metadata from previously deleted clusters
- Logic was too aggressive in treating status as stale

**Solution**:
- Improved stale detection logic in `pkg/storage/cluster_types.go`
- Only treat status as stale if it has `deletion_requested` AND config doesn't have deletion timestamp
- Trust status if it shows running/configuring/installing states

**Key Learning**: Be careful with stale data detection - don't be too aggressive in ignoring potentially valid data.

### 4. Editor Not Opening After Cluster Deletion
**Problem**: Pressing 'c' to create new cluster didn't work after deleting all clusters.

**Root Cause**:
- When switching from table view to empty placeholder, keyboard focus wasn't being set
- Key handlers were attached but the component didn't have focus

**Solution**:
- Added `app.SetFocus(emptyPlaceholder)` when showing empty state
- Added `app.SetFocus(clusterTable)` when showing table
- Ensured proper focus management during view transitions

**Key Learning**: Always explicitly set focus when switching between UI components in TUI applications.

### 5. Platform Resources Created in Wrong Region
**Problem**: Lambda and other platform resources were being created in user's AWS CLI region instead of ap-south-1.

**Root Cause**:
- Provider initialization was using the user's AWS_REGION environment variable
- Platform resources should always be in ap-south-1 regardless of where clusters are created

**Solution**:
- Hardcoded ap-south-1 for platform resource creation
- Only use user's region for cluster-specific resources

**Key Learning**: Distinguish between platform resources (fixed region) and user resources (flexible region).

### 6. Mode Naming Inconsistency
**Problem**: UI and code used "developer mode" while actual mode was "dev".

**Root Cause**:
- Inconsistent naming throughout the codebase
- Documentation mismatch

**Solution**:
- Renamed all references from "developer" to "dev"
- Updated S3 stored configurations
- Fixed documentation

**Key Learning**: Maintain consistent terminology throughout the codebase and documentation.

## Architecture Insights

### Lambda Environment Updates
When updating Lambda environment variables, you must:
1. Get existing configuration first
2. Merge new variables with existing ones
3. Update with the merged set
```go
existingConfig, _ := lambdaClient.GetFunctionConfiguration(...)
envVars := existingConfig.Environment.Variables
envVars["NEW_VAR"] = "value"
lambdaClient.UpdateFunctionConfiguration(...) 
```

### State File Management
The system uses split storage in S3:
- `config.yaml` - Desired state (what user wants)
- `status.yaml` - Actual state (what Lambda sees)
- Node names may differ between these files due to indexing

### Focus Management in TUI
When dynamically switching views:
- Always call `app.SetFocus()` on the new component
- Attach input handlers to all interactive components
- Test keyboard navigation after view changes

### Region Strategy
- Platform resources (Lambda, DynamoDB, SQS): Always in ap-south-1
- Cluster resources (EC2, Security Groups): In user-specified region
- This separation allows global platform with regional clusters

## Testing Checklist for Common Issues

When making changes, verify:
- [ ] Lambda can requeue itself (check CloudWatch logs for "Scheduled requeue")
- [ ] Metrics load for running clusters
- [ ] No status flickering when refreshing cluster list
- [ ] Keyboard shortcuts work in both table and empty states
- [ ] Lambda environment variables are preserved during updates
- [ ] Resources are created in correct regions