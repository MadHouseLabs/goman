# K3s Operator Refactor & Fix: Detailed Task Breakdown

## Overview
This document breaks down the comprehensive K3s operator refactor into manageable tasks across 6 sprints over 14 days.

## Sprint 1: Critical Fixes (Day 1-2)
**Goal**: Fix immediate blockers that prevent cluster creation

### Task 1.1: Verify PendingOperations Persistence ⭐ PRIORITY
**Status**: 🟡 In Progress  
**Files to modify**:
- `pkg/controller/reconciler.go` (lines 1475-1485, 1327-1378)
- Test S3 save/load operations

**Acceptance Criteria**:
- [ ] PendingOperations are saved to S3 metadata section
- [ ] PendingOperations are loaded from S3 on reconciliation start
- [ ] Add debug logging to confirm save/load operations
- [ ] Test with simple cluster creation and verify no command restarts

**Implementation Steps**:
1. Add debug logs to `saveClusterResource()` when saving PendingOperations
2. Add debug logs to `convertStateToResource()` when loading PendingOperations
3. Verify timeout deserialization (Duration needs special handling)
4. Test with actual cluster creation
5. Check CloudWatch logs for proper save/load

---

### Task 1.2: Fix Cross-Region Resource Management
**Status**: 🔴 Not Started  
**Files to modify**:
- `pkg/provider/aws/compute_service.go`
- `pkg/controller/reconciler.go` (region detection)

**Acceptance Criteria**:
- [ ] Region detection works correctly for cross-region clusters
- [ ] SSM commands are sent to correct region
- [ ] Region info stored in cluster state
- [ ] No more "commands sent to wrong region" errors

**Implementation Steps**:
1. Fix region client initialization in compute_service.go
2. Store region in cluster resource state
3. Add region validation before SSM commands
4. Test cross-region cluster creation

---

### Task 1.3: Optimize Requeue Intervals
**Status**: 🔴 Not Started  
**Files to modify**:
- `pkg/controller/constants.go`
- `pkg/controller/phases.go`

**Acceptance Criteria**:
- [ ] Reduce requeue intervals from 30-60s to 15-30s
- [ ] Adjust phase-specific requeue times
- [ ] Monitor Lambda invocation count impact
- [ ] No increase in Lambda costs > 20%

**Implementation Steps**:
1. Update requeue constants in constants.go
2. Adjust phase-specific intervals
3. Deploy and monitor Lambda invocations
4. Adjust if too aggressive

---

## Sprint 2: Enhanced Debugging (Day 3-4)
**Goal**: Make the system easily debuggable

### Task 2.1: Implement Comprehensive Step Logging
**Status**: 🔴 Not Started  
**Files to modify**:
- All phase files (`k3s_configurator.go`, `k3s_installer.go`, etc.)
- Add new logging utility

**Acceptance Criteria**:
- [ ] Step-by-step logging with step X/Y format
- [ ] Timing information for each step
- [ ] Easy grep/searchable log format
- [ ] No performance impact from logging

**Implementation Steps**:
1. Create logging utility with step tracking
2. Add to all major operations in each phase
3. Include timing and progress information
4. Test log readability

---

### Task 2.2: Add Trace ID System
**Status**: 🔴 Not Started  
**Files to modify**:
- `pkg/models/resource.go` (add TraceID field)
- `pkg/controller/reconciler.go` (generate and use trace ID)
- All logging statements

**Acceptance Criteria**:
- [ ] Unique trace ID per cluster lifecycle
- [ ] Trace ID persists across reconciliations
- [ ] All logs include trace ID
- [ ] Reconciliation counter implemented

**Implementation Steps**:
1. Add TraceID field to ClusterResourceStatus
2. Generate trace ID on first reconciliation
3. Persist in status.yaml
4. Add to all log statements
5. Add reconciliation counter

---

### Task 2.3: Implement Check Validation Framework
**Status**: 🔴 Not Started  
**Files to modify**:
- `pkg/controller/reconciler.go` (new validateAllChecks function)
- All phase files (add validation calls)

**Acceptance Criteria**:
- [ ] validateAllChecks() function runs in every reconciliation
- [ ] All checks run regardless of phase
- [ ] Check results added to progress metrics
- [ ] Log check failures with remediation hints

**Implementation Steps**:
1. Create validateAllChecks() function
2. Implement individual check functions
3. Add to main reconciliation loop
4. Include in progress metrics
5. Add remediation suggestions

---

### Task 2.4: Enhanced Error Reporting
**Status**: 🔴 Not Started  
**Files to modify**:
- All files with error handling
- Create new error utility package

**Acceptance Criteria**:
- [ ] Context added to all error messages
- [ ] Suggested fixes in error logs
- [ ] Error categorization (transient/permanent)
- [ ] Error aggregation for similar issues

**Implementation Steps**:
1. Create error utility package
2. Add context to error messages
3. Categorize error types
4. Add suggested remediation steps
5. Update all error handling

---

## Sprint 3: Background Process Management (Day 5-7)
**Goal**: Handle long-running operations without blocking

### Task 3.1: Implement BackgroundProcess Data Model
**Status**: 🔴 Not Started  
**Files to modify**:
- `pkg/models/resource.go` (add BackgroundProcess struct)
- Serialization/deserialization in reconciler

**Acceptance Criteria**:
- [ ] BackgroundProcess struct defined
- [ ] BackgroundProcesses map in PendingOperations
- [ ] Proper JSON serialization/deserialization
- [ ] Integrated with save/load operations

**Implementation Steps**:
1. Add BackgroundProcess struct to models
2. Add BackgroundProcesses map to PendingOperations
3. Implement JSON serialization
4. Update save/load operations
5. Test serialization round-trip

---

### Task 3.2: Create Background Execution Framework
**Status**: 🔴 Not Started  
**Files to modify**:
- `pkg/controller/reconciler.go` (new background functions)
- Background process utilities

**Acceptance Criteria**:
- [ ] startBackgroundCheck() function implemented
- [ ] PID file management working
- [ ] Log file capture implemented
- [ ] Process timeout handling added

**Implementation Steps**:
1. Implement startBackgroundCheck() function
2. Add PID file management
3. Implement log file capture
4. Add timeout handling
5. Test with simple background process

---

### Task 3.3: Implement Process Status Checking
**Status**: 🔴 Not Started  
**Files to modify**:
- `pkg/controller/reconciler.go` (checkBackgroundProcess function)
- Process monitoring utilities

**Acceptance Criteria**:
- [ ] checkBackgroundProcess() function implemented
- [ ] PID-based process monitoring working
- [ ] Log file retrieval implemented
- [ ] Cleanup of completed processes working

**Implementation Steps**:
1. Implement checkBackgroundProcess() function
2. Add PID-based monitoring
3. Implement log file retrieval
4. Add process cleanup
5. Test process lifecycle

---

### Task 3.4: Convert Long Operations to Background
**Status**: 🔴 Not Started  
**Files to modify**:
- `k3s_configurator.go`, `k3s_installer.go` (convert operations)
- Health check functions

**Acceptance Criteria**:
- [ ] All operations > 30 seconds converted to background
- [ ] K3s health checks run in background
- [ ] Cluster verification runs in background
- [ ] Tested with failure scenarios

**Implementation Steps**:
1. Identify operations > 30 seconds
2. Convert K3s health checks to background
3. Convert cluster verification to background
4. Test with various scenarios
5. Verify no blocking operations remain

---

## Sprint 4: Reliability Enhancements (Day 8-10)
**Goal**: Make the system production-ready and self-healing

### Task 4.1: Implement Circuit Breaker Pattern
**Status**: 🔴 Not Started  
**Files to modify**:
- `pkg/models/resource.go` (OperationCircuitBreaker struct)
- `pkg/controller/reconciler.go` (circuit breaker logic)

**Acceptance Criteria**:
- [ ] OperationCircuitBreaker struct implemented
- [ ] Circuit breaker state persisted in cluster resource
- [ ] Failure counting and circuit opening working
- [ ] Circuit breaker reset logic implemented

**Implementation Steps**:
1. Create OperationCircuitBreaker struct
2. Add to cluster resource state
3. Implement failure counting
4. Add circuit opening/closing logic
5. Test with failing operations

---

### Task 4.2: Enhanced Idempotency Patterns
**Status**: 🔴 Not Started  
**Files to modify**:
- All operation functions across all files
- Add idempotency utility functions

**Acceptance Criteria**:
- [ ] All operations audited for idempotency
- [ ] State checking added before operations
- [ ] Operation deduplication implemented
- [ ] Idempotency tests passing

**Implementation Steps**:
1. Audit all operations for idempotency
2. Add state checking before operations
3. Implement operation deduplication
4. Create idempotency utility functions
5. Add comprehensive tests

---

### Task 4.3: Atomic State Updates
**Status**: 🔴 Not Started  
**Files to modify**:
- `pkg/controller/reconciler.go` (atomic update functions)
- State validation utilities

**Acceptance Criteria**:
- [ ] State checkpointing implemented
- [ ] Rollback capability added
- [ ] State validation functions created
- [ ] Consistency checks implemented

**Implementation Steps**:
1. Implement state checkpointing
2. Add rollback capability
3. Create state validation functions
4. Add consistency checks
5. Test rollback scenarios

---

### Task 4.4: Exponential Backoff Implementation
**Status**: 🔴 Not Started  
**Files to modify**:
- Replace all fixed retry logic
- Create retry utility package

**Acceptance Criteria**:
- [ ] Fixed retries replaced with exponential backoff
- [ ] Jitter added to prevent thundering herd
- [ ] Per-operation retry policies configured
- [ ] Retry metrics implemented

**Implementation Steps**:
1. Create retry utility package
2. Replace fixed retries with exponential backoff
3. Add jitter to retry timing
4. Configure per-operation policies
5. Add retry metrics

---

## Sprint 5: Performance Optimization (Day 11-12)
**Goal**: Reduce cluster creation time to fit within Lambda timeout

### Task 5.1: Parallel Operation Analysis
**Status**: 🔴 Not Started  
**Files to modify**:
- `pkg/controller/k3s_installer.go` (parallel installation)
- Provisioning logic (parallel instance creation)

**Acceptance Criteria**:
- [ ] Operations identified that can run in parallel
- [ ] Parallel SSM command execution implemented
- [ ] Parallel instance provisioning added
- [ ] Tested with different cluster sizes

**Implementation Steps**:
1. Analyze operations for parallelization opportunities
2. Implement parallel SSM command execution
3. Add parallel instance provisioning
4. Test with various cluster sizes
5. Measure performance improvements

---

### Task 5.2: K3s Script Optimization
**Status**: 🔴 Not Started  
**Files to modify**:
- `pkg/controller/k3s_configurator.go` (installation scripts)
- Script generation functions

**Acceptance Criteria**:
- [ ] K3s installation scripts simplified
- [ ] Unnecessary checks removed
- [ ] Download methods optimized
- [ ] Configuration files pre-compiled

**Implementation Steps**:
1. Simplify K3s installation scripts
2. Remove unnecessary checks and waits
3. Optimize download methods
4. Pre-compile configuration files
5. Test script reliability

---

### Task 5.3: Pre-built AMI Creation (Optional)
**Status**: 🔴 Not Started  
**Files to modify**:
- AMI building scripts (new)
- Instance creation logic (AMI selection)

**Acceptance Criteria**:
- [ ] AMI created with K3s pre-installed
- [ ] AMI selection logic implemented
- [ ] Cluster creation tested with pre-built AMI
- [ ] AMI build process documented

**Implementation Steps**:
1. Create AMI building scripts
2. Add AMI selection logic
3. Test cluster creation with pre-built AMI
4. Document AMI build process
5. Compare performance vs standard AMI

---

### Task 5.4: Operation Profiling
**Status**: 🔴 Not Started  
**Files to modify**:
- Add timing to all operations
- CloudWatch metrics integration

**Acceptance Criteria**:
- [ ] Timing metrics added to all operations
- [ ] Bottlenecks identified
- [ ] Performance dashboard created
- [ ] CloudWatch alarms configured

**Implementation Steps**:
1. Add timing metrics to all operations
2. Send metrics to CloudWatch
3. Create performance dashboard
4. Set up alarms for long operations
5. Analyze and optimize bottlenecks

---

## Sprint 6: Testing & Documentation (Day 13-14)
**Goal**: Ensure reliability and maintainability

### Task 6.1: Unit Test Coverage
**Status**: 🔴 Not Started  
**Files to modify**:
- Create test files for all major components
- Add test utilities

**Acceptance Criteria**:
- [ ] PendingOperations persistence tested
- [ ] Background process management tested
- [ ] Circuit breaker logic tested
- [ ] Idempotency patterns tested

**Implementation Steps**:
1. Create unit tests for PendingOperations
2. Test background process management
3. Test circuit breaker logic
4. Test idempotency patterns
5. Achieve >80% code coverage

---

### Task 6.2: Integration Tests
**Status**: 🔴 Not Started  
**Files to modify**:
- Create integration test suite
- Test infrastructure setup

**Acceptance Criteria**:
- [ ] Full cluster lifecycle tested
- [ ] Failure recovery tested
- [ ] Concurrent operations tested
- [ ] Cross-region deployments tested

**Implementation Steps**:
1. Set up integration test infrastructure
2. Test full cluster lifecycle
3. Test failure recovery scenarios
4. Test concurrent operations
5. Test cross-region deployments

---

### Task 6.3: Load Testing
**Status**: 🔴 Not Started  
**Files to modify**:
- Load testing scripts
- Performance monitoring

**Acceptance Criteria**:
- [ ] Multiple concurrent clusters tested
- [ ] Lambda timeout scenarios tested
- [ ] Network delay scenarios tested
- [ ] SSM throttling scenarios tested

**Implementation Steps**:
1. Create load testing scripts
2. Test multiple concurrent clusters
3. Test Lambda timeout scenarios
4. Test with network delays
5. Test SSM throttling handling

---

### Task 6.4: Documentation Updates
**Status**: 🔴 Not Started  
**Files to modify**:
- Update all documentation files
- Create new troubleshooting guides

**Acceptance Criteria**:
- [ ] Operation runbooks updated
- [ ] New debugging features documented
- [ ] Troubleshooting guide created
- [ ] Architecture diagrams updated

**Implementation Steps**:
1. Update operation runbooks
2. Document new debugging features
3. Create comprehensive troubleshooting guide
4. Update architecture diagrams
5. Review and validate all documentation

---

## Implementation Guidelines

### Daily Progress Tracking
- Update task status daily
- Log blockers and dependencies
- Track actual vs estimated time
- Adjust plan as needed

### Testing Strategy
- Test each task individually before moving on
- Integration test after each sprint
- Keep a running test cluster for validation
- Document all test scenarios and results

### Rollback Plan
- Each sprint should be independently deployable
- Keep previous Lambda versions for quick rollback
- Test rollback procedures
- Document rollback steps

### Success Metrics
- **Sprint 1**: PendingOperations work correctly, no command restarts
- **Sprint 2**: All operations fully traceable in logs
- **Sprint 3**: No blocking operations >30 seconds
- **Sprint 4**: >95% success rate, circuit breakers prevent cascading failures
- **Sprint 5**: Cluster creation <10 minutes consistently
- **Sprint 6**: All tests pass, documentation complete

## Risk Mitigation
- **Lambda timeout risk**: Implement Sprint 3 (background processes) early
- **State corruption risk**: Implement Sprint 4 (atomic updates) before major changes
- **Performance regression**: Continuous monitoring throughout all sprints
- **Cross-region issues**: Address in Sprint 1 to prevent future complications