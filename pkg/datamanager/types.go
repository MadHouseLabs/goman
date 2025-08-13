package datamanager

import (
	"time"

	"github.com/madhouselabs/goman/pkg/models"
	"github.com/madhouselabs/goman/pkg/storage"
)

// DataType represents the type of data being managed
type DataType string

const (
	DataTypeClusters      DataType = "clusters"
	DataTypeClusterStates DataType = "cluster_states"
	DataTypeClusterDetail DataType = "cluster_detail"
	DataTypeMetrics       DataType = "metrics"
	DataTypeEvents        DataType = "events"
)

// Action represents the type of data update
type Action string

const (
	ActionCreate Action = "create"
	ActionUpdate Action = "update"
	ActionDelete Action = "delete"
	ActionReload Action = "reload"
)

// FetchPolicy defines how data should be fetched
type FetchPolicy string

const (
	PolicyUseCache     FetchPolicy = "use_cache"      // Return cached data immediately
	PolicyFreshIfStale FetchPolicy = "fresh_if_stale"  // Return cache, fetch if older than threshold
	PolicyForceRefresh FetchPolicy = "force_refresh"   // Clear cache and fetch new data
	PolicySubscribe    FetchPolicy = "subscribe"       // Subscribe to updates at specified frequency
)

// Priority levels for data requests
type Priority int

const (
	PriorityLow    Priority = 0
	PriorityNormal Priority = 5
	PriorityHigh   Priority = 10
	PriorityUrgent Priority = 15
)

// DataUpdate represents an update sent to subscribers
type DataUpdate struct {
	Type      DataType
	Action    Action
	Data      interface{}
	Timestamp time.Time
	Source    string
}

// DataRequest represents a request for data
type DataRequest struct {
	ID           string
	Type         DataType
	Policy       FetchPolicy
	RefreshRate  time.Duration
	Priority     Priority
	Filters      map[string]interface{}
	ResponseChan chan DataResponse
}

// DataResponse represents a response to a data request
type DataResponse struct {
	RequestID string
	Type      DataType
	Data      interface{}
	Error     error
	FromCache bool
	Timestamp time.Time
}

// RefreshConfig manages refresh settings for a data type
type RefreshConfig struct {
	DataType     DataType
	Frequency    time.Duration
	LastRefresh  time.Time
	NextRefresh  time.Time
	AutoRefresh  bool
	Priority     Priority
	Subscribers  []string
}

// DataSubscriber interface for components that receive data updates
type DataSubscriber interface {
	// OnDataUpdate is called when data is updated
	OnDataUpdate(update DataUpdate)
	
	// GetSubscriptionID returns unique identifier for this subscriber
	GetSubscriptionID() string
	
	// GetDataTypes returns the data types this subscriber is interested in
	GetDataTypes() []DataType
}

// SubscriptionConfig defines subscription settings
type SubscriptionConfig struct {
	SubscriberID string
	DataTypes    []DataType
	RefreshRate  time.Duration
	Priority     Priority
	Filters      map[string]interface{}
}

// ClusterData wraps cluster-related data
type ClusterData struct {
	Clusters []models.K3sCluster
	States   map[string]*storage.K3sClusterState
}

// MetricsData represents cluster metrics
type MetricsData struct {
	ClusterName string
	CPU         float64
	Memory      float64
	Storage     float64
	Network     float64
	Timestamp   time.Time
}

// EventData represents cluster events
type EventData struct {
	ClusterName string
	EventType   string
	Message     string
	Severity    string
	Timestamp   time.Time
}