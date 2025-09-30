package config

import (
	"claude-squad/log"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// StateService manages asynchronous state saving with a single-threaded queue
// This prevents lock contention and UI blocking during saves
type StateService struct {
	state      *State
	saveQueue  chan saveRequest
	stopChan   chan struct{}
	stoppedWg  sync.WaitGroup
	mu         sync.Mutex
	running    bool
}

// saveRequest represents a request to save state
type saveRequest struct {
	instancesData json.RawMessage
	doneChan      chan error // Optional channel for synchronous saves
}

// NewStateService creates a new state service for async saves
func NewStateService(state *State) *StateService {
	return &StateService{
		state:     state,
		saveQueue: make(chan saveRequest, 100), // Buffer up to 100 pending saves
		stopChan:  make(chan struct{}),
	}
}

// Start begins processing save requests in the background
func (s *StateService) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		log.WarningLog.Printf("StateService already running")
		return
	}

	s.running = true
	s.stoppedWg.Add(1)

	go s.processQueue()
	log.InfoLog.Printf("StateService started")
}

// processQueue runs in a goroutine and processes save requests sequentially
func (s *StateService) processQueue() {
	defer s.stoppedWg.Done()

	// Keep track of pending request for coalescing
	var pendingRequest *saveRequest
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case req := <-s.saveQueue:
			// If we already have a pending async request, replace it with the new one
			// This coalesces rapid saves into a single operation
			if pendingRequest != nil && pendingRequest.doneChan == nil {
				log.InfoLog.Printf("Coalescing async save request")
				pendingRequest = &req
			} else if pendingRequest == nil {
				pendingRequest = &req
			} else {
				// We have a pending synchronous request, process it first
				s.executeSave(pendingRequest)
				pendingRequest = &req
			}

			// If this is a synchronous request, process immediately
			if req.doneChan != nil {
				s.executeSave(pendingRequest)
				pendingRequest = nil
			}

		case <-ticker.C:
			// Periodically flush pending saves to avoid losing data
			if pendingRequest != nil {
				s.executeSave(pendingRequest)
				pendingRequest = nil
			}

		case <-s.stopChan:
			// Flush any pending save before stopping
			if pendingRequest != nil {
				log.InfoLog.Printf("Flushing pending save on shutdown")
				s.executeSave(pendingRequest)
			}
			// Drain the queue
			for {
				select {
				case req := <-s.saveQueue:
					s.executeSave(&req)
				default:
					log.InfoLog.Printf("StateService stopped")
					return
				}
			}
		}
	}
}

// executeSave performs the actual save operation
func (s *StateService) executeSave(req *saveRequest) {
	if req == nil {
		return
	}

	// Update the state's instances data
	s.state.InstancesData = req.instancesData

	// Perform the save
	err := SaveState(s.state)

	// If this is a synchronous request, send the result
	if req.doneChan != nil {
		req.doneChan <- err
	} else if err != nil {
		// For async requests, just log errors
		log.ErrorLog.Printf("Async save failed: %v", err)
	}
}

// SaveAsync queues an async save (non-blocking)
func (s *StateService) SaveAsync(instancesData json.RawMessage) {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		log.WarningLog.Printf("StateService not running, cannot save asynchronously")
		return
	}
	s.mu.Unlock()

	select {
	case s.saveQueue <- saveRequest{instancesData: instancesData}:
		// Queued successfully
	default:
		log.WarningLog.Printf("Save queue full, dropping async save request")
	}
}

// SaveSync performs a synchronous save (blocks until complete)
func (s *StateService) SaveSync(instancesData json.RawMessage) error {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		// If service not running, fall back to direct save
		s.state.InstancesData = instancesData
		return SaveState(s.state)
	}
	s.mu.Unlock()

	doneChan := make(chan error, 1)
	req := saveRequest{
		instancesData: instancesData,
		doneChan:      doneChan,
	}

	// Send request
	select {
	case s.saveQueue <- req:
		// Wait for completion
		return <-doneChan
	case <-time.After(10 * time.Second):
		return fmt.Errorf("synchronous save timed out after 10 seconds")
	}
}

// Shutdown stops the service and flushes all pending saves
func (s *StateService) Shutdown() error {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		log.WarningLog.Printf("StateService not running")
		return nil
	}
	s.running = false
	s.mu.Unlock()

	log.InfoLog.Printf("Shutting down StateService")

	// Signal shutdown
	close(s.stopChan)

	// Wait for goroutine to finish with timeout
	done := make(chan struct{})
	go func() {
		s.stoppedWg.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.InfoLog.Printf("StateService shutdown complete")
		return nil
	case <-time.After(5 * time.Second):
		return fmt.Errorf("StateService shutdown timed out")
	}
}
