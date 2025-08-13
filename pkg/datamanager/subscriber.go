package datamanager

import (
	"fmt"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
)

// TUISubscriber adapts DataSubscriber for Bubble Tea components
type TUISubscriber struct {
	id         string
	dataTypes  []DataType
	updateChan <-chan DataUpdate
	program    *tea.Program
	mu         sync.RWMutex
}

// NewTUISubscriber creates a new TUI subscriber
func NewTUISubscriber(id string, dataTypes []DataType) *TUISubscriber {
	return &TUISubscriber{
		id:        id,
		dataTypes: dataTypes,
	}
}

// SetProgram sets the Bubble Tea program for sending messages
func (s *TUISubscriber) SetProgram(p *tea.Program) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.program = p
}

// SetUpdateChannel sets the channel for receiving updates
func (s *TUISubscriber) SetUpdateChannel(ch <-chan DataUpdate) {
	s.updateChan = ch
}

// OnDataUpdate handles data updates
func (s *TUISubscriber) OnDataUpdate(update DataUpdate) {
	s.mu.RLock()
	p := s.program
	s.mu.RUnlock()
	
	if p != nil {
		// Send update as Bubble Tea message
		p.Send(update)
	}
}

// GetSubscriptionID returns the subscriber's ID
func (s *TUISubscriber) GetSubscriptionID() string {
	return s.id
}

// GetDataTypes returns the data types this subscriber is interested in
func (s *TUISubscriber) GetDataTypes() []DataType {
	return s.dataTypes
}

// StartListening starts listening for updates in a goroutine
func (s *TUISubscriber) StartListening() {
	if s.updateChan == nil {
		return
	}
	
	go func() {
		for update := range s.updateChan {
			s.OnDataUpdate(update)
		}
	}()
}

// DataManagerMsg wraps DataManager responses for Bubble Tea
type DataManagerMsg struct {
	Update   *DataUpdate
	Response *DataResponse
	Error    error
}

// String implements the fmt.Stringer interface
func (m DataManagerMsg) String() string {
	if m.Error != nil {
		return fmt.Sprintf("DataManager error: %v", m.Error)
	}
	if m.Update != nil {
		return fmt.Sprintf("DataUpdate: %s %s", m.Update.Type, m.Update.Action)
	}
	if m.Response != nil {
		return fmt.Sprintf("DataResponse: %s", m.Response.Type)
	}
	return "Empty DataManagerMsg"
}

// RequestDataCmd creates a Bubble Tea command for data requests
func RequestDataCmd(dm *DataManager, request DataRequest) tea.Cmd {
	return func() tea.Msg {
		// Create response channel
		responseChan := make(chan DataResponse, 1)
		request.ResponseChan = responseChan
		
		// Send request
		dm.RequestData(request)
		
		// Wait for response
		response := <-responseChan
		
		return DataManagerMsg{
			Response: &response,
			Error:    response.Error,
		}
	}
}

// RefreshDataCmd creates a command to refresh data
func RefreshDataCmd(dm *DataManager, dataType DataType) tea.Cmd {
	return func() tea.Msg {
		dm.RefreshData(dataType)
		return DataManagerMsg{
			Update: &DataUpdate{
				Type:   dataType,
				Action: ActionUpdate,
			},
		}
	}
}

// SubscribeCmd creates a command to subscribe to data updates
func SubscribeCmd(dm *DataManager, config SubscriptionConfig) tea.Cmd {
	return func() tea.Msg {
		err := dm.Subscribe(config)
		if err != nil {
			return DataManagerMsg{Error: err}
		}
		return DataManagerMsg{}
	}
}