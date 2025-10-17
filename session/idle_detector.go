package session

import (
	"fmt"
	"sync"
	"time"
)

// IdleState represents the idle state of a Claude Code session.
type IdleState int

const (
	IdleStateUnknown IdleState = iota // Unable to determine state
	IdleStateActive                   // Actively processing commands (shows "esc to interrupt")
	IdleStateWaiting                  // Waiting for user input (INSERT mode, command prompt)
	IdleStateTimeout                  // No activity for extended period
)

// IdleDetectorConfig contains configuration for idle detection behavior.
type IdleDetectorConfig struct {
	IdleThreshold time.Duration // Duration before considering session timed out
	DebounceDelay time.Duration // Delay before changing state to prevent flickering
	BufferSize    int           // Number of bytes to analyze from recent output
}

// DefaultIdleDetectorConfig returns sensible defaults for idle detection.
func DefaultIdleDetectorConfig() IdleDetectorConfig {
	return IdleDetectorConfig{
		IdleThreshold: 10 * time.Second, // Reduced from 30s for faster detection
		DebounceDelay: 500 * time.Millisecond, // Reduced from 2s for faster response
		BufferSize:    4096, // 4KB should capture recent status indicators
	}
}

// IdleDetector monitors PTY output to determine if a Claude Code session is idle.
// It uses pattern matching on recent output and tracks state transitions with debouncing.
type IdleDetector struct {
	sessionName string
	statusDetector *StatusDetector
	ptyAccess *PTYAccess
	config IdleDetectorConfig

	// State tracking
	currentState    IdleState
	lastStateChange time.Time
	lastActivity    time.Time

	mu sync.RWMutex
}

// NewIdleDetector creates a new idle detector for a session.
func NewIdleDetector(sessionName string, ptyAccess *PTYAccess) *IdleDetector {
	return NewIdleDetectorWithConfig(sessionName, ptyAccess, DefaultIdleDetectorConfig())
}

// NewIdleDetectorWithConfig creates a new idle detector with custom configuration.
func NewIdleDetectorWithConfig(sessionName string, ptyAccess *PTYAccess, config IdleDetectorConfig) *IdleDetector {
	now := time.Now()
	return &IdleDetector{
		sessionName:     sessionName,
		statusDetector:  NewStatusDetector(),
		ptyAccess:       ptyAccess,
		config:          config,
		currentState:    IdleStateUnknown,
		lastStateChange: now,
		lastActivity:    now,
	}
}

// DetectState analyzes recent PTY output and returns the current idle state.
// This method applies pattern matching and debouncing logic.
func (id *IdleDetector) DetectState() IdleState {
	id.mu.Lock()
	defer id.mu.Unlock()

	// Get recent output for pattern matching
	recentOutput := id.ptyAccess.GetRecentOutput(id.config.BufferSize)
	if len(recentOutput) == 0 {
		// No output yet, keep current state
		return id.currentState
	}

	// Detect status from patterns
	status := id.statusDetector.Detect(recentOutput)

	// Map detected status to idle state
	newState := id.mapStatusToIdleState(status)

	// Apply debouncing to prevent rapid state changes
	// BUT: if current state is Unknown, always transition immediately
	if newState != id.currentState {
		if id.currentState == IdleStateUnknown || time.Since(id.lastStateChange) >= id.config.DebounceDelay {
			id.currentState = newState
			id.lastStateChange = time.Now()
		}
		// If debouncing, keep current state
	}

	return id.currentState
}

// mapStatusToIdleState converts a DetectedStatus to an IdleState.
func (id *IdleDetector) mapStatusToIdleState(status DetectedStatus) IdleState {
	switch status {
	case StatusActive:
		// Actively executing commands - update activity timestamp
		id.lastActivity = time.Now()
		return IdleStateActive

	case StatusProcessing:
		// Processing but not showing active indicators - still consider active
		id.lastActivity = time.Now()
		return IdleStateActive

	case StatusIdle, StatusReady:
		// Waiting for input - check if we've been idle too long
		idleDuration := time.Since(id.lastActivity)
		if idleDuration > id.config.IdleThreshold {
			return IdleStateTimeout
		}
		return IdleStateWaiting

	case StatusNeedsApproval:
		// Waiting for approval - consider this as waiting
		return IdleStateWaiting

	case StatusError:
		// Error state - consider as waiting (needs user attention)
		return IdleStateWaiting

	default:
		// Unknown status - don't maintain Unknown, default to Waiting
		// This handles fresh starts where we haven't detected anything yet
		return IdleStateWaiting
	}
}

// IsIdle returns true if the session is currently idle (waiting or timed out).
func (id *IdleDetector) IsIdle() bool {
	state := id.DetectState()
	return state == IdleStateWaiting || state == IdleStateTimeout
}

// IsActive returns true if the session is actively processing commands.
func (id *IdleDetector) IsActive() bool {
	return id.DetectState() == IdleStateActive
}

// GetState returns the current idle state without triggering detection.
// Use this when you want the cached state without analyzing PTY output.
func (id *IdleDetector) GetState() IdleState {
	id.mu.RLock()
	defer id.mu.RUnlock()
	return id.currentState
}

// GetLastActivity returns the timestamp of the last detected activity.
func (id *IdleDetector) GetLastActivity() time.Time {
	id.mu.RLock()
	defer id.mu.RUnlock()
	return id.lastActivity
}

// GetIdleDuration returns how long the session has been idle.
func (id *IdleDetector) GetIdleDuration() time.Duration {
	id.mu.RLock()
	defer id.mu.RUnlock()
	return time.Since(id.lastActivity)
}

// GetStateInfo returns comprehensive state information for debugging and display.
func (id *IdleDetector) GetStateInfo() IdleStateInfo {
	id.mu.RLock()
	defer id.mu.RUnlock()

	return IdleStateInfo{
		State:           id.currentState,
		LastActivity:    id.lastActivity,
		IdleDuration:    time.Since(id.lastActivity),
		LastStateChange: id.lastStateChange,
		SessionName:     id.sessionName,
	}
}

// Reset resets the idle detector's state tracking.
// Use this when reattaching to a session or after significant changes.
func (id *IdleDetector) Reset() {
	id.mu.Lock()
	defer id.mu.Unlock()

	now := time.Now()
	id.currentState = IdleStateUnknown
	id.lastStateChange = now
	id.lastActivity = now
}

// UpdateConfig updates the idle detector configuration.
func (id *IdleDetector) UpdateConfig(config IdleDetectorConfig) {
	id.mu.Lock()
	defer id.mu.Unlock()
	id.config = config
}

// IdleStateInfo contains comprehensive information about the current idle state.
type IdleStateInfo struct {
	State           IdleState
	LastActivity    time.Time
	IdleDuration    time.Duration
	LastStateChange time.Time
	SessionName     string
}

// String returns a human-readable string representation of the idle state.
func (s IdleState) String() string {
	switch s {
	case IdleStateActive:
		return "Active"
	case IdleStateWaiting:
		return "Waiting"
	case IdleStateTimeout:
		return "Timeout"
	default:
		return "Unknown"
	}
}

// Description returns a detailed description of the idle state info.
func (info IdleStateInfo) Description() string {
	return fmt.Sprintf("Session '%s' is %s (idle for %s, last activity: %s)",
		info.SessionName,
		info.State.String(),
		formatDuration(info.IdleDuration),
		info.LastActivity.Format("15:04:05"))
}

// formatDuration formats a duration in a human-readable way.
func formatDuration(d time.Duration) string {
	if d < time.Second {
		return "< 1s"
	}
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm %ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh %dm", int(d.Hours()), int(d.Minutes())%60)
}
