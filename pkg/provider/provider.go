package provider

import (
	"context"
	"time"
)

// Provider defines the interface for cloud providers
type Provider interface {
	// Core services
	GetLockService() LockService
	GetStorageService() StorageService
	GetNotificationService() NotificationService
	GetFunctionService() FunctionService
	GetComputeService() ComputeService

	// Provider info
	Name() string
	Region() string

	// Infrastructure management
	Initialize(ctx context.Context) (*InitializeResult, error)
	Cleanup(ctx context.Context) error
	GetStatus(ctx context.Context) (*InfrastructureStatus, error)
}

// LockService provides distributed locking for resources
type LockService interface {
	// Initialize ensures the lock backend exists (e.g., DynamoDB table)
	Initialize(ctx context.Context) error

	// AcquireLock tries to acquire a lock for a resource
	// Returns a lock token if successful, error if lock is held by another process
	AcquireLock(ctx context.Context, resourceID string, owner string, ttl time.Duration) (string, error)

	// ReleaseLock releases a lock using the token
	ReleaseLock(ctx context.Context, resourceID string, token string) error

	// RenewLock extends the TTL of an existing lock
	RenewLock(ctx context.Context, resourceID string, token string, ttl time.Duration) error

	// IsLocked checks if a resource is currently locked
	IsLocked(ctx context.Context, resourceID string) (bool, string, error) // returns locked, owner, error
}

// StorageService provides object storage operations
type StorageService interface {
	Initialize(ctx context.Context) error
	PutObject(ctx context.Context, key string, data []byte) error
	GetObject(ctx context.Context, key string) ([]byte, error)
	DeleteObject(ctx context.Context, key string) error
	ListObjects(ctx context.Context, prefix string) ([]string, error)
}

// NotificationService provides pub/sub messaging
type NotificationService interface {
	Initialize(ctx context.Context) error
	Publish(ctx context.Context, topic string, message string) error
	Subscribe(ctx context.Context, topic string) (string, error) // returns subscription ID
	Unsubscribe(ctx context.Context, subscriptionID string) error
}

// FunctionService provides serverless function management
type FunctionService interface {
	Initialize(ctx context.Context) error
	DeployFunction(ctx context.Context, name string, packagePath string) error
	InvokeFunction(ctx context.Context, name string, payload []byte) ([]byte, error)
	DeleteFunction(ctx context.Context, name string) error
	FunctionExists(ctx context.Context, name string) (bool, error)
	GetFunctionURL(ctx context.Context, name string) (string, error)
}

// ComputeService provides VM/instance management
type ComputeService interface {
	CreateInstance(ctx context.Context, config InstanceConfig) (*Instance, error)
	DeleteInstance(ctx context.Context, instanceID string) error
	GetInstance(ctx context.Context, instanceID string) (*Instance, error)
	ListInstances(ctx context.Context, filters map[string]string) ([]*Instance, error)
	StartInstance(ctx context.Context, instanceID string) error
	StopInstance(ctx context.Context, instanceID string) error
	ModifyInstanceType(ctx context.Context, instanceID string, instanceType string) error

	// RunCommand executes a command on instances using cloud-native methods (e.g., SSM for AWS)
	RunCommand(ctx context.Context, instanceIDs []string, command string) (*CommandResult, error)
}

// CommandResult represents the result of running a command on instances
type CommandResult struct {
	CommandID string
	Status    string
	Instances map[string]*InstanceCommandResult
}

// InstanceCommandResult represents the result for a single instance
type InstanceCommandResult struct {
	InstanceID string
	Status     string
	Output     string
	Error      string
	ExitCode   int
}

// InstanceConfig defines configuration for creating instances
type InstanceConfig struct {
	Name           string
	Region         string // Region where instance should be created
	InstanceType   string
	ImageID        string
	SubnetID       string
	SecurityGroups []string
	// KeyName field removed - using Systems Manager for instance access
	UserData        string
	Tags            map[string]string
	InstanceProfile string // IAM instance profile for SSM access
}

// Instance represents a compute instance
type Instance struct {
	ID           string
	Name         string
	State        string
	PrivateIP    string
	PublicIP     string
	InstanceType string
	LaunchTime   time.Time
	Tags         map[string]string
}

// Lock represents a distributed lock
type Lock struct {
	ResourceID string
	Owner      string
	Token      string
	ExpiresAt  time.Time
}

// InitializeResult represents the result of infrastructure initialization
type InitializeResult struct {
	StorageReady       bool              // Storage service initialized
	FunctionReady      bool              // Function service deployed
	LockServiceReady   bool              // Lock service initialized
	NotificationsReady bool              // Notification service configured
	AuthReady          bool              // Authentication/authorization configured
	ProviderType       string            // Provider type (aws, gcp, azure)
	Resources          map[string]string // Provider-specific resource identifiers
	Errors             []string          // Any errors during initialization
}

// InfrastructureStatus represents the current infrastructure status
type InfrastructureStatus struct {
	Initialized    bool              // Overall initialization status
	StorageStatus  string            // Storage service status
	FunctionStatus string            // Function service status
	LockStatus     string            // Lock service status
	AuthStatus     string            // Auth service status
	Resources      map[string]string // Provider-specific resource details
}

// Instance state constants (provider-agnostic)
const (
	InstanceStatePending     = "pending"
	InstanceStateRunning     = "running"
	InstanceStateStopping    = "stopping"
	InstanceStateStopped     = "stopped"
	InstanceStateTerminating = "terminating"
	InstanceStateTerminated  = "terminated"
)
