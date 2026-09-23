package ratelimit

import (
	"context"
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/log"
)

// stopJoinTimeout bounds how long PTYConsumer.Stop() waits for pollLoop to exit.
const stopJoinTimeout = 10 * time.Second

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
		log.Info("rate limit detection started", "session", i.sessionID)
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
		log.Info("rate limit detection stopped", "session", i.sessionID)
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
	cancelFn     context.CancelFunc
	notifyCh     chan struct{}
	// doneCh is the current generation's pollLoop-exited signal (see Start()).
	// Read by Stop() under pc.mu, then waited on outside the lock.
	doneCh chan struct{}
}

func NewPTYConsumer(buffer BufferReader, manager *Manager) *PTYConsumer {
	return &PTYConsumer{
		buffer:       buffer,
		manager:      manager,
		pollInterval: 500 * time.Millisecond,
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

	ctx, cancel := context.WithCancel(context.Background())
	pc.cancelFn = cancel
	pc.running = true
	// done is local to this Start()/Stop() generation, not a struct field:
	// a sync.WaitGroup field here would panic ("WaitGroup is reused before
	// previous Wait has returned") under PTYConsumer_StartStop_Concurrent's
	// repeated concurrent Start()/Stop() cycles, since a new Add() can race
	// an outstanding Stop()'s Wait() from the previous generation. A fresh
	// channel per generation, closed by pollLoop and captured locally by
	// Stop(), has no such reuse hazard.
	done := make(chan struct{})
	pc.doneCh = done
	go pc.pollLoop(ctx, done)
}

func (pc *PTYConsumer) Stop() {
	pc.mu.Lock()

	if !pc.running {
		pc.mu.Unlock()
		return
	}

	pc.running = false
	done := pc.doneCh
	if pc.cancelFn != nil {
		pc.cancelFn()
		pc.cancelFn = nil
	}
	// Unlock explicitly (not via defer) before waiting: the wait below must
	// run after the lock is released, or a future pollLoop change that takes
	// pc.mu would deadlock against it.
	pc.mu.Unlock()

	select {
	case <-done:
	case <-time.After(stopJoinTimeout):
		log.Error("PTYConsumer.Stop: pollLoop did not exit within timeout", "timeout", stopJoinTimeout)
	}
}

func (pc *PTYConsumer) pollLoop(ctx context.Context, done chan struct{}) {
	defer close(done)
	heartbeat := time.NewTicker(5 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-ctx.Done():
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
