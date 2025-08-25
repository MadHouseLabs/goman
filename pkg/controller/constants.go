package controller

import (
	"time"
)

// Timeout constants
const (
	// ReconcileTimeout is the maximum time for a complete reconciliation cycle
	ReconcileTimeout = 10 * time.Minute
	
	// LockAcquireTimeout is the timeout for acquiring a distributed lock
	LockAcquireTimeout = 30 * time.Second
	
	// LockTTL is the default time-to-live for a distributed lock
	LockTTL = 5 * time.Minute
	
	// LockRenewInterval is how often to renew the lock during long operations
	LockRenewInterval = 2 * time.Minute
	
	// LockRenewTimeout is the timeout for renewing a lock
	LockRenewTimeout = 10 * time.Second
	
	// LockReleaseTimeout is the timeout for releasing a lock
	LockReleaseTimeout = 10 * time.Second
	
	// DeleteInstanceTimeout is the timeout for requesting instance deletion
	DeleteInstanceTimeout = 30 * time.Second
)

// Phase-specific lock TTLs for optimized lock management
const (
	// Quick phases - operations that should complete in seconds
	LockTTLQuick = 30 * time.Second  // Pending, Failed, Stopped, Running health checks
	
	// Medium phases - operations that may take a minute or two  
	LockTTLMedium = 2 * time.Minute // Provisioning, Starting, Stopping
	
	// Long phases - operations that can take several minutes
	LockTTLLong = 3 * time.Minute   // Installing, Configuring
	
	// Emergency phases - operations that need extra time
	LockTTLEmergency = 5 * time.Minute // Deleting, complex recovery operations
)

// Retry constants
const (
	// MaxProvisionRetries is the maximum number of provisioning attempts
	MaxProvisionRetries = 10
	
	// LockedClusterRetryInterval is how long to wait before retrying a locked cluster
	LockedClusterRetryInterval = 20 * time.Second
)

// Requeue intervals
const (
	// PendingRequeuInterval is the requeue interval for pending phase
	PendingRequeuInterval = 5 * time.Second
	
	// ProvisioningRequeuInterval is the requeue interval during provisioning
	ProvisioningRequeuInterval = 10 * time.Second
	
	// RunningRecheckInterval is how often to check running clusters
	RunningRecheckInterval = 45 * time.Second
	
	// DeletingRecheckInterval is how often to check deletion progress
	DeletingRecheckInterval = 15 * time.Second
	
	// FailedRetryInterval is how long to wait before retrying a failed cluster
	FailedRetryInterval = 20 * time.Second
)

// Instance management constants
const (
	// MaxInstancesPerBatch is the maximum number of instances to create in one batch
	MaxInstancesPerBatch = 10
	
	// InstanceHealthCheckTimeout is the timeout for checking instance health
	InstanceHealthCheckTimeout = 2 * time.Minute
)

// Logging prefixes for better traceability
const (
	LogPrefixReconcile = "[RECONCILE]"
	LogPrefixLock      = "[LOCK]"
	LogPrefixLoad      = "[LOAD]"
	LogPrefixSave      = "[SAVE]"
	LogPrefixPhase     = "[PHASE]"
	LogPrefixPending   = "[PENDING]"
	LogPrefixProvision = "[PROVISION]"
	LogPrefixRunning   = "[RUNNING]"
	LogPrefixDelete    = "[DELETE]"
	LogPrefixError     = "[ERROR]"
	LogPrefixRequeue   = "[REQUEUE]"
	LogPrefixComplete  = "[COMPLETE]"
	LogPrefixSuccess   = "[SUCCESS]"
)