package datamanager

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/madhouselabs/goman/pkg/cluster"
	"github.com/madhouselabs/goman/pkg/models"
	"github.com/madhouselabs/goman/pkg/storage"
)

// DataManager is the central data management system
type DataManager struct {
	// Data stores
	clusters      map[string]*models.K3sCluster
	clusterStates map[string]*storage.K3sClusterState
	metrics       map[string]*MetricsData
	events        []EventData
	dataMutex     sync.RWMutex

	// Subscription management
	subscribers    map[string]DataSubscriber
	subscriptions  map[string]*SubscriptionConfig
	updateChannels map[string]chan DataUpdate
	subMutex       sync.RWMutex

	// Refresh management
	refreshConfigs map[DataType]*RefreshConfig
	refreshTimers  map[DataType]*time.Timer
	refreshMutex   sync.RWMutex

	// Request queue
	requestQueue chan DataRequest
	
	// Backend services
	clusterManager *cluster.Manager
	storage        *storage.Storage
	
	// Control
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewDataManager creates a new data manager instance
func NewDataManager() (*DataManager, error) {
	ctx, cancel := context.WithCancel(context.Background())
	
	// Initialize cluster manager
	clusterMgr := cluster.NewManager()
	
	// Initialize storage
	stor, err := storage.NewStorage()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to initialize storage: %w", err)
	}
	
	dm := &DataManager{
		clusters:       make(map[string]*models.K3sCluster),
		clusterStates:  make(map[string]*storage.K3sClusterState),
		metrics:        make(map[string]*MetricsData),
		events:         make([]EventData, 0),
		subscribers:    make(map[string]DataSubscriber),
		subscriptions:  make(map[string]*SubscriptionConfig),
		updateChannels: make(map[string]chan DataUpdate),
		refreshConfigs: make(map[DataType]*RefreshConfig),
		refreshTimers:  make(map[DataType]*time.Timer),
		requestQueue:   make(chan DataRequest, 100),
		clusterManager: clusterMgr,
		storage:        stor,
		ctx:            ctx,
		cancel:         cancel,
	}
	
	// Start background workers
	dm.wg.Add(2)
	go dm.requestProcessor()
	go dm.refreshScheduler()
	
	return dm, nil
}

// Subscribe registers a subscriber for data updates
func (dm *DataManager) Subscribe(config SubscriptionConfig) error {
	dm.subMutex.Lock()
	defer dm.subMutex.Unlock()
	
	// Create update channel for subscriber
	updateChan := make(chan DataUpdate, 100)
	dm.updateChannels[config.SubscriberID] = updateChan
	dm.subscriptions[config.SubscriberID] = &config
	
	// Update refresh configs for requested data types
	dm.refreshMutex.Lock()
	for _, dataType := range config.DataTypes {
		if refreshConfig, exists := dm.refreshConfigs[dataType]; exists {
			// Update frequency if subscriber wants faster updates
			if config.RefreshRate < refreshConfig.Frequency {
				refreshConfig.Frequency = config.RefreshRate
			}
			refreshConfig.Subscribers = append(refreshConfig.Subscribers, config.SubscriberID)
		} else {
			// Create new refresh config
			dm.refreshConfigs[dataType] = &RefreshConfig{
				DataType:    dataType,
				Frequency:   config.RefreshRate,
				AutoRefresh: true,
				Priority:    config.Priority,
				Subscribers: []string{config.SubscriberID},
			}
		}
	}
	dm.refreshMutex.Unlock()
	
	// Send initial data if available
	dm.sendInitialData(config.SubscriberID, config.DataTypes)
	
	return nil
}

// Unsubscribe removes a subscriber
func (dm *DataManager) Unsubscribe(subscriberID string) {
	dm.subMutex.Lock()
	defer dm.subMutex.Unlock()
	
	// Close and remove update channel
	if ch, exists := dm.updateChannels[subscriberID]; exists {
		close(ch)
		delete(dm.updateChannels, subscriberID)
	}
	
	// Remove from subscriptions
	delete(dm.subscriptions, subscriberID)
	
	// Remove from refresh configs
	dm.refreshMutex.Lock()
	for _, config := range dm.refreshConfigs {
		for i, id := range config.Subscribers {
			if id == subscriberID {
				config.Subscribers = append(config.Subscribers[:i], config.Subscribers[i+1:]...)
				break
			}
		}
	}
	dm.refreshMutex.Unlock()
}

// RequestData handles data requests from components
func (dm *DataManager) RequestData(request DataRequest) {
	select {
	case dm.requestQueue <- request:
		// Request queued successfully
	case <-time.After(5 * time.Second):
		// Timeout - send error response
		if request.ResponseChan != nil {
			request.ResponseChan <- DataResponse{
				RequestID: request.ID,
				Type:      request.Type,
				Error:     fmt.Errorf("request timeout"),
				Timestamp: time.Now(),
			}
		}
	}
}

// RefreshData forces a refresh of specific data type
func (dm *DataManager) RefreshData(dataType DataType) {
	dm.requestQueue <- DataRequest{
		Type:     dataType,
		Policy:   PolicyForceRefresh,
		Priority: PriorityHigh,
	}
}

// GetCachedData returns cached data immediately
func (dm *DataManager) GetCachedData(dataType DataType) (interface{}, bool) {
	dm.dataMutex.RLock()
	defer dm.dataMutex.RUnlock()
	
	switch dataType {
	case DataTypeClusters:
		clusters := make([]models.K3sCluster, 0, len(dm.clusters))
		for _, c := range dm.clusters {
			clusters = append(clusters, *c)
		}
		return clusters, true
		
	case DataTypeClusterStates:
		states := make(map[string]*storage.K3sClusterState)
		for k, v := range dm.clusterStates {
			states[k] = v
		}
		return states, true
		
	default:
		return nil, false
	}
}

// requestProcessor processes data requests from the queue
func (dm *DataManager) requestProcessor() {
	defer dm.wg.Done()
	
	for {
		select {
		case <-dm.ctx.Done():
			return
			
		case request := <-dm.requestQueue:
			dm.processRequest(request)
		}
	}
}

// processRequest handles a single data request
func (dm *DataManager) processRequest(request DataRequest) {
	var data interface{}
	var err error
	fromCache := false
	
	switch request.Policy {
	case PolicyUseCache:
		data, fromCache = dm.GetCachedData(request.Type)
		
	case PolicyFreshIfStale:
		// Check if cache is stale
		if dm.isCacheStale(request.Type) {
			data, err = dm.fetchFreshData(request.Type)
		} else {
			data, fromCache = dm.GetCachedData(request.Type)
		}
		
	case PolicyForceRefresh:
		data, err = dm.fetchFreshData(request.Type)
		
	case PolicySubscribe:
		// Handle subscription separately
		return
	}
	
	// Send response if channel provided
	if request.ResponseChan != nil {
		request.ResponseChan <- DataResponse{
			RequestID: request.ID,
			Type:      request.Type,
			Data:      data,
			Error:     err,
			FromCache: fromCache,
			Timestamp: time.Now(),
		}
	}
}

// fetchFreshData fetches fresh data from backend
func (dm *DataManager) fetchFreshData(dataType DataType) (interface{}, error) {
	switch dataType {
	case DataTypeClusters, DataTypeClusterStates:
		// Fetch clusters and states together
		clusters := dm.clusterManager.GetClusters()
		states := dm.clusterManager.GetAllClusterStates()
		
		// Update cache
		dm.dataMutex.Lock()
		dm.clusters = make(map[string]*models.K3sCluster)
		for i := range clusters {
			dm.clusters[clusters[i].Name] = &clusters[i]
		}
		dm.clusterStates = states
		dm.dataMutex.Unlock()
		
		// Broadcast update to subscribers
		dm.broadcastUpdate(DataUpdate{
			Type:      dataType,
			Action:    ActionReload,
			Data:      ClusterData{Clusters: clusters, States: states},
			Timestamp: time.Now(),
		})
		
		if dataType == DataTypeClusters {
			return clusters, nil
		}
		return states, nil
		
	default:
		return nil, fmt.Errorf("unsupported data type: %s", dataType)
	}
}

// isCacheStale checks if cached data is stale
func (dm *DataManager) isCacheStale(dataType DataType) bool {
	dm.refreshMutex.RLock()
	defer dm.refreshMutex.RUnlock()
	
	config, exists := dm.refreshConfigs[dataType]
	if !exists {
		return true // No config means stale
	}
	
	return time.Since(config.LastRefresh) > config.Frequency
}

// broadcastUpdate sends updates to all relevant subscribers
func (dm *DataManager) broadcastUpdate(update DataUpdate) {
	dm.subMutex.RLock()
	defer dm.subMutex.RUnlock()
	
	for subscriberID, config := range dm.subscriptions {
		// Check if subscriber is interested in this data type
		for _, dataType := range config.DataTypes {
			if dataType == update.Type {
				// Send update to subscriber's channel
				if ch, exists := dm.updateChannels[subscriberID]; exists {
					select {
					case ch <- update:
						// Update sent
					default:
						// Channel full, skip
					}
				}
				break
			}
		}
	}
}

// refreshScheduler manages automatic data refresh
func (dm *DataManager) refreshScheduler() {
	defer dm.wg.Done()
	
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-dm.ctx.Done():
			return
			
		case <-ticker.C:
			dm.checkAndRefresh()
		}
	}
}

// checkAndRefresh checks if any data needs refreshing
func (dm *DataManager) checkAndRefresh() {
	dm.refreshMutex.RLock()
	configs := make([]*RefreshConfig, 0, len(dm.refreshConfigs))
	for _, config := range dm.refreshConfigs {
		if config.AutoRefresh && time.Now().After(config.NextRefresh) {
			configs = append(configs, config)
		}
	}
	dm.refreshMutex.RUnlock()
	
	// Trigger refresh for due data types
	for _, config := range configs {
		dm.RequestData(DataRequest{
			Type:     config.DataType,
			Policy:   PolicyFreshIfStale,
			Priority: config.Priority,
		})
		
		// Update next refresh time
		dm.refreshMutex.Lock()
		config.LastRefresh = time.Now()
		config.NextRefresh = time.Now().Add(config.Frequency)
		dm.refreshMutex.Unlock()
	}
}

// sendInitialData sends cached data to new subscriber
func (dm *DataManager) sendInitialData(subscriberID string, dataTypes []DataType) {
	dm.dataMutex.RLock()
	defer dm.dataMutex.RUnlock()
	
	ch, exists := dm.updateChannels[subscriberID]
	if !exists {
		return
	}
	
	for _, dataType := range dataTypes {
		var data interface{}
		
		switch dataType {
		case DataTypeClusters:
			clusters := make([]models.K3sCluster, 0, len(dm.clusters))
			for _, c := range dm.clusters {
				clusters = append(clusters, *c)
			}
			data = clusters
			
		case DataTypeClusterStates:
			data = dm.clusterStates
		}
		
		if data != nil {
			select {
			case ch <- DataUpdate{
				Type:      dataType,
				Action:    ActionReload,
				Data:      data,
				Timestamp: time.Now(),
				Source:    "cache",
			}:
			default:
				// Channel full
			}
		}
	}
}

// GetUpdateChannel returns the update channel for a subscriber
func (dm *DataManager) GetUpdateChannel(subscriberID string) (<-chan DataUpdate, bool) {
	dm.subMutex.RLock()
	defer dm.subMutex.RUnlock()
	
	ch, exists := dm.updateChannels[subscriberID]
	return ch, exists
}

// Shutdown gracefully shuts down the data manager
func (dm *DataManager) Shutdown() {
	dm.cancel()
	dm.wg.Wait()
	
	// Close all update channels
	dm.subMutex.Lock()
	for _, ch := range dm.updateChannels {
		close(ch)
	}
	dm.subMutex.Unlock()
}