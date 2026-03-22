// Package ssp implements the State Synchronization Protocol (SSP) for efficient
// terminal streaming. Inspired by Mosh (Mobile Shell), SSP provides:
//
//   - Minimal ANSI escape sequence diffs (not full screen updates)
//   - Predictive echo for low-latency typing experience
//   - RTT-based adaptive frame rate throttling
//   - Automatic resync on sequence mismatch
//
// The Coordinator manages the server-side SSP state for a single session,
// tracking per-client state and generating optimized diffs.
package ssp

import (
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/framebuffer"
)

// Coordinator manages SSP state for a single terminal session.
// It tracks the current framebuffer state and generates diffs for connected clients.
type Coordinator struct {
	sessionID string
	mu        sync.RWMutex

	// Current terminal state (source of truth)
	framebuffer *session.TerminalState

	// Diff generator for creating minimal ANSI sequences
	diffGenerator *framebuffer.DiffGenerator

	// Per-client state tracking
	clients map[string]*ClientState

	// Echo tracking for predictive typing
	echoHistory *EchoHistory

	// Frame rate control
	lastFrameTime time.Time
	minFrameInterval time.Duration

	// Configuration
	config CoordinatorConfig
}

// CoordinatorConfig holds configuration options for the coordinator.
type CoordinatorConfig struct {
	// MinFrameIntervalMs is the minimum time between frame updates (default: 16ms = 60fps)
	MinFrameIntervalMs int

	// MaxDiffSize is the maximum diff size before falling back to full state (default: 64KB)
	MaxDiffSize int

	// EchoTimeoutMs is the timeout for predictive echo acknowledgment (default: 50ms, like Mosh)
	EchoTimeoutMs int

	// EchoHistorySize is the number of echo entries to track (default: 1000)
	EchoHistorySize int
}

// DefaultConfig returns the default coordinator configuration.
func DefaultConfig() CoordinatorConfig {
	return CoordinatorConfig{
		MinFrameIntervalMs: 16,    // 60 fps max
		MaxDiffSize:        65536, // 64KB
		EchoTimeoutMs:      50,    // Mosh default
		EchoHistorySize:    1000,
	}
}

// NewCoordinator creates a new SSP coordinator for the given session.
func NewCoordinator(sessionID string, config CoordinatorConfig) *Coordinator {
	if config.MinFrameIntervalMs <= 0 {
		config.MinFrameIntervalMs = DefaultConfig().MinFrameIntervalMs
	}
	if config.MaxDiffSize <= 0 {
		config.MaxDiffSize = DefaultConfig().MaxDiffSize
	}
	if config.EchoTimeoutMs <= 0 {
		config.EchoTimeoutMs = DefaultConfig().EchoTimeoutMs
	}
	if config.EchoHistorySize <= 0 {
		config.EchoHistorySize = DefaultConfig().EchoHistorySize
	}

	return &Coordinator{
		sessionID:        sessionID,
		framebuffer:      nil, // Will be initialized on first PTY output
		diffGenerator:    framebuffer.NewDiffGenerator(),
		clients:          make(map[string]*ClientState),
		echoHistory:      NewEchoHistory(config.EchoHistorySize),
		minFrameInterval: time.Duration(config.MinFrameIntervalMs) * time.Millisecond,
		config:           config,
	}
}

// RegisterClient adds a new client for SSP updates.
// Returns the current framebuffer state for initial sync.
func (c *Coordinator) RegisterClient(clientID string, capabilities *ClientCapabilities) *framebuffer.DiffResult {
	c.mu.Lock()
	defer c.mu.Unlock()

	state := NewClientState(clientID, capabilities)
	c.clients[clientID] = state

	// Generate full redraw for initial sync
	if c.framebuffer != nil {
		return c.diffGenerator.GenerateDiff(nil, c.framebuffer)
	}

	return nil
}

// UnregisterClient removes a client from SSP tracking.
func (c *Coordinator) UnregisterClient(clientID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.clients, clientID)
}

// ProcessPTYOutput handles new PTY output by updating the framebuffer
// and generating diffs for all connected clients.
//
// Returns a map of clientID -> diff for clients that should receive updates.
// Some clients may be skipped due to frame rate throttling.
func (c *Coordinator) ProcessPTYOutput(data []byte) map[string]*framebuffer.DiffResult {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Initialize framebuffer if this is the first output
	if c.framebuffer == nil {
		c.framebuffer = session.NewTerminalState(24, 80) // Default size
	}

	// Store old state for diffing
	oldState := c.framebuffer.Clone()

	// Process the PTY output through the ANSI parser
	c.framebuffer.ProcessOutput(data)

	// Check frame rate throttling
	now := time.Now()
	if now.Sub(c.lastFrameTime) < c.minFrameInterval {
		// Too soon, skip this frame
		return nil
	}
	c.lastFrameTime = now

	// Generate diffs for each client
	results := make(map[string]*framebuffer.DiffResult)
	for clientID, clientState := range c.clients {
		// Use client's last known state for more accurate diff
		var baseState *session.TerminalState
		if clientState.LastFramebuffer != nil {
			baseState = clientState.LastFramebuffer
		} else {
			baseState = oldState
		}

		diff := c.diffGenerator.GenerateDiff(baseState, c.framebuffer)

		// Check if diff is too large, fall back to full state
		if len(diff.DiffBytes) > c.config.MaxDiffSize && !diff.FullRedraw {
			diff = c.diffGenerator.GenerateDiff(nil, c.framebuffer)
		}

		// Check for pending echo acknowledgments
		echoAck := c.checkEchoAck(clientID)
		if echoAck != nil {
			diff.FromSequence = echoAck.EchoAckNum // Piggyback echo ack
		}

		results[clientID] = diff

		// Update client's last known state
		clientState.LastFramebuffer = c.framebuffer.Clone()
		clientState.LastSequence = diff.ToSequence
	}

	return results
}

// ProcessResize handles terminal resize events.
func (c *Coordinator) ProcessResize(rows, cols int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.framebuffer == nil {
		c.framebuffer = session.NewTerminalState(rows, cols)
		return
	}

	// Resize triggers a full redraw for all clients
	c.framebuffer = session.NewTerminalState(rows, cols)
	c.lastFrameTime = time.Time{} // Force next frame to be sent
}

// ProcessInput handles user input with echo tracking.
// Returns the echo number for predictive echo acknowledgment.
func (c *Coordinator) ProcessInput(clientID string, data []byte, echoNum uint64, clientTimestampMs int64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Record input for echo acknowledgment
	c.echoHistory.Record(clientID, echoNum, clientTimestampMs, data)
}

// checkEchoAck checks if there are echo acknowledgments ready for a client.
// Called with lock held.
func (c *Coordinator) checkEchoAck(clientID string) *EchoAck {
	client, ok := c.clients[clientID]
	if !ok {
		return nil
	}

	// Find the highest echo number that has been processed
	// (PTY output received after input was sent)
	ackNum := c.echoHistory.GetAckNum(clientID)
	if ackNum > client.LastEchoAckNum {
		client.LastEchoAckNum = ackNum
		return &EchoAck{
			EchoAckNum:        ackNum,
			ServerTimestampMs: time.Now().UnixMilli(),
		}
	}

	return nil
}

// RequestResync requests a full state sync for a client.
// Used when client detects sequence mismatch.
func (c *Coordinator) RequestResync(clientID string) *framebuffer.DiffResult {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.framebuffer == nil {
		return nil
	}

	// Generate full redraw
	diff := c.diffGenerator.GenerateDiff(nil, c.framebuffer)

	// Update client state
	if client, ok := c.clients[clientID]; ok {
		client.LastFramebuffer = c.framebuffer.Clone()
		client.LastSequence = diff.ToSequence
	}

	return diff
}

// GetCurrentSequence returns the current framebuffer sequence number.
func (c *Coordinator) GetCurrentSequence() uint64 {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.framebuffer == nil {
		return 0
	}
	return c.framebuffer.Version
}

// GetClientCount returns the number of connected clients.
func (c *Coordinator) GetClientCount() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.clients)
}

// SetFrameInterval updates the minimum frame interval.
// Can be adjusted based on RTT measurements.
func (c *Coordinator) SetFrameInterval(interval time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.minFrameInterval = interval
}

// EchoAck represents an echo acknowledgment to be sent to a client.
type EchoAck struct {
	EchoAckNum        uint64
	ServerTimestampMs int64
}
