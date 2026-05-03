package ratelimit

import (
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/log"
)

type BufferReader interface {
	GetRecentOutput(n int) []byte
}

type Integration struct {
	manager   *Manager
	session   SessionAccessor
	buffer    BufferReader
	sessionID string
	mu        sync.Mutex
	started   bool
}

func NewIntegrationWithAccessor(sessionID string, session SessionAccessor, buffer BufferReader) *Integration {
	integration := &Integration{
		sessionID: sessionID,
		session:   session,
		buffer:    buffer,
	}

	if session != nil {
		integration.manager = NewManager(sessionID, session)
	}

	return integration
}

func (i *Integration) Start() {
	i.mu.Lock()
	defer i.mu.Unlock()

	if i.started {
		return
	}

	if i.manager != nil {
		i.manager.Start()
		i.started = true
		log.InfoLog.Printf("Rate limit detection started for session '%s'", i.sessionID)
	}
}

func (i *Integration) Stop() {
	i.mu.Lock()
	defer i.mu.Unlock()

	if !i.started {
		return
	}

	if i.manager != nil {
		i.manager.Stop()
		i.started = false
		log.InfoLog.Printf("Rate limit detection stopped for session '%s'", i.sessionID)
	}
}

func (i *Integration) GetManager() *Manager {
	return i.manager
}

func (i *Integration) SetEnabled(enabled bool) {
	if i.manager != nil {
		i.manager.SetEnabled(enabled)
	}
}

func (i *Integration) IsEnabled() bool {
	if i.manager != nil {
		return i.manager.IsEnabled()
	}
	return false
}

type PTYConsumer struct {
	buffer       BufferReader
	manager      *Manager
	pollInterval time.Duration
	mu           sync.Mutex
	running      bool
	stopCh       chan struct{}
	notifyCh     chan struct{}
}

func NewPTYConsumer(buffer BufferReader, manager *Manager) *PTYConsumer {
	return &PTYConsumer{
		buffer:       buffer,
		manager:      manager,
		pollInterval: 500 * time.Millisecond,
		stopCh:       make(chan struct{}),
		notifyCh:     make(chan struct{}, 1),
	}
}

// NotifyOutput signals the poll loop that new data is available, avoiding the
// 500ms polling delay. The send is non-blocking: if a notification is already
// pending, the extra signal is dropped rather than blocking the caller.
func (pc *PTYConsumer) NotifyOutput() {
	select {
	case pc.notifyCh <- struct{}{}:
	default:
	}
}

func (pc *PTYConsumer) Start() {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	if pc.running {
		return
	}

	pc.running = true
	go pc.pollLoop()
}

func (pc *PTYConsumer) Stop() {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	if !pc.running {
		return
	}

	pc.running = false
	close(pc.stopCh)
	pc.stopCh = make(chan struct{})
}

func (pc *PTYConsumer) pollLoop() {
	heartbeat := time.NewTicker(5 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-pc.stopCh:
			return
		case <-pc.notifyCh:
			data := pc.buffer.GetRecentOutput(4096)
			if len(data) > 0 {
				pc.manager.ProcessOutput(data)
			}
		case <-heartbeat.C:
			data := pc.buffer.GetRecentOutput(4096)
			if len(data) > 0 {
				pc.manager.ProcessOutput(data)
			}
		}
	}
}

func (pc *PTYConsumer) GetRateLimitState() RateLimitState {
	if pc.manager != nil {
		return pc.manager.GetState()
	}
	return StateNone
}

// GetResetTime returns the current rate limit reset time from the underlying manager.
func (pc *PTYConsumer) GetResetTime() time.Time {
	if pc.manager != nil {
		return pc.manager.GetResetTime()
	}
	return time.Time{}
}

// GetManager returns the underlying Manager for advanced callback wiring.
func (pc *PTYConsumer) GetManager() *Manager {
	return pc.manager
}

func (pc *PTYConsumer) SetEnabled(enabled bool) {
	if pc.manager != nil {
		pc.manager.SetEnabled(enabled)
	}
}

func (pc *PTYConsumer) IsEnabled() bool {
	if pc.manager != nil {
		return pc.manager.IsEnabled()
	}
	return false
}
