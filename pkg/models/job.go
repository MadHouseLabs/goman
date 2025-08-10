package models

import (
	"time"
)

// JobType represents the type of job to execute
type JobType string

const (
	JobTypeCreate JobType = "create"
	JobTypeDelete JobType = "delete"
	JobTypeSync   JobType = "sync"
	JobTypeStatus JobType = "status"
)

// JobStatus represents the status of a job
type JobStatus string

const (
	JobStatusPending   JobStatus = "pending"
	JobStatusRunning   JobStatus = "running"
	JobStatusCompleted JobStatus = "completed"
	JobStatusFailed    JobStatus = "failed"
)

// Job represents a background job with phases
type Job struct {
	ID           string                 `json:"id"`
	Type         JobType                `json:"type"`
	Status       JobStatus              `json:"status"`
	Phase        string                 `json:"phase,omitempty"`        // Current phase of execution
	Conditions   []JobCondition         `json:"conditions,omitempty"`   // Conditions like Ready, Progressing
	Payload      map[string]interface{} `json:"payload"`
	Error        string                 `json:"error,omitempty"`
	CreatedAt    time.Time              `json:"created_at"`
	UpdatedAt    time.Time              `json:"updated_at"`
	Result       interface{}            `json:"result,omitempty"`
	RetryCount   int                    `json:"retry_count,omitempty"`
	NextReconcile *time.Time            `json:"next_reconcile,omitempty"` // When to reconcile next
	ObservedGeneration int               `json:"observed_generation,omitempty"`
}

// JobCondition represents a condition of a job
type JobCondition struct {
	Type               string    `json:"type"`               // Ready, Progressing, Failed
	Status             string    `json:"status"`             // True, False, Unknown
	LastTransitionTime time.Time `json:"lastTransitionTime"`
	Reason             string    `json:"reason,omitempty"`
	Message            string    `json:"message,omitempty"`
}

// Job phases for create cluster
const (
	PhaseInitializing = "Initializing"
	PhaseNetworking   = "Networking"
	PhaseProvisioning = "Provisioning"
	PhaseConfiguring  = "Configuring"
	PhaseVerifying    = "Verifying"
	PhaseReady        = "Ready"
	PhaseFailed       = "Failed"
	PhaseDeleting     = "Deleting"
)