package mux

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// DiscoveredSession represents a discovered ssq-mux session.
type DiscoveredSession struct {
	SocketPath string
	Metadata   *SessionMetadata
	LastSeen   time.Time
}

// Discovery scans for and tracks ssq-mux sessions.
type Discovery struct {
	sessions  map[string]*DiscoveredSession
	mu        sync.RWMutex
	callbacks []func(*DiscoveredSession, bool) // (session, isNew)
}

// NewDiscovery creates a new session discovery service.
func NewDiscovery() *Discovery {
	return &Discovery{
		sessions:  make(map[string]*DiscoveredSession),
		callbacks: nil,
	}
}

// OnSessionChange registers a callback for session discovery/removal events.
// The callback receives the session and a boolean indicating if it's new (true) or removed (false).
func (d *Discovery) OnSessionChange(callback func(*DiscoveredSession, bool)) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.callbacks = append(d.callbacks, callback)
}

// Scan searches for active ssq-mux sockets and returns discovered sessions.
func (d *Discovery) Scan() ([]*DiscoveredSession, error) {
	// Find all potential socket files
	pattern := filepath.Join(os.TempDir(), "ssq-mux-*.sock")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("failed to glob socket files: %w", err)
	}

	var discovered []*DiscoveredSession
	var newSessions []*DiscoveredSession
	currentPaths := make(map[string]bool)

	for _, socketPath := range matches {
		currentPaths[socketPath] = true

		// Check if we already know about this session
		d.mu.RLock()
		existing, exists := d.sessions[socketPath]
		d.mu.RUnlock()

		if exists {
			// Update last seen time
			existing.LastSeen = time.Now()
			discovered = append(discovered, existing)
			continue
		}

		// Try to connect and get metadata
		meta, err := probeSocket(socketPath)
		if err != nil {
			// Socket exists but can't connect - might be stale
			continue
		}

		session := &DiscoveredSession{
			SocketPath: socketPath,
			Metadata:   meta,
			LastSeen:   time.Now(),
		}

		// Register the new session
		d.mu.Lock()
		d.sessions[socketPath] = session
		d.mu.Unlock()

		discovered = append(discovered, session)
		newSessions = append(newSessions, session)
	}

	// Check for removed sessions
	d.mu.Lock()
	var removedSessions []*DiscoveredSession
	for path, session := range d.sessions {
		if !currentPaths[path] {
			removedSessions = append(removedSessions, session)
			delete(d.sessions, path)
		}
	}
	callbacks := d.callbacks
	d.mu.Unlock()

	// Notify callbacks (outside lock)
	for _, session := range newSessions {
		for _, cb := range callbacks {
			cb(session, true)
		}
	}
	for _, session := range removedSessions {
		for _, cb := range callbacks {
			cb(session, false)
		}
	}

	return discovered, nil
}

// GetSessions returns all currently known sessions.
func (d *Discovery) GetSessions() []*DiscoveredSession {
	d.mu.RLock()
	defer d.mu.RUnlock()

	sessions := make([]*DiscoveredSession, 0, len(d.sessions))
	for _, session := range d.sessions {
		sessions = append(sessions, session)
	}
	return sessions
}

// GetClaudeSessions returns only sessions running Claude.
func (d *Discovery) GetClaudeSessions() []*DiscoveredSession {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var claudeSessions []*DiscoveredSession
	for _, session := range d.sessions {
		if session.Metadata != nil && isClaudeCommand(session.Metadata.Command) {
			claudeSessions = append(claudeSessions, session)
		}
	}
	return claudeSessions
}

// ScanFromUserOptions performs a fast discovery pass using tmux user options
// written by WriteSessionUserOptions. Unlike Scan(), this issues a single
// `tmux list-sessions` call instead of probing N sockets, making it ideal for
// initial discovery on server startup or after a restart.
//
// Results are merged into d.sessions and callbacks are fired for new sessions,
// matching the behaviour of Scan(). Existing sessions are not removed — call
// Scan() afterward to reconcile against the live socket set if needed.
func (d *Discovery) ScanFromUserOptions() ([]*DiscoveredSession, error) {
	discovered, err := ScanByUserOptions()
	if err != nil {
		return nil, err
	}

	var newSessions []*DiscoveredSession

	d.mu.Lock()
	for _, session := range discovered {
		key := session.SocketPath
		if existing, exists := d.sessions[key]; exists {
			existing.LastSeen = time.Now()
		} else {
			session.LastSeen = time.Now()
			d.sessions[key] = session
			newSessions = append(newSessions, session)
		}
	}
	callbacks := d.callbacks
	d.mu.Unlock()

	for _, session := range newSessions {
		for _, cb := range callbacks {
			cb(session, true)
		}
	}

	return discovered, nil
}

// StartPolling starts periodic scanning for sessions.
// Returns a channel that will be closed when polling stops.
func (d *Discovery) StartPolling(ctx context.Context, interval time.Duration) <-chan struct{} {
	done := make(chan struct{})

	go func() {
		defer close(done)

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		// Initial scan
		_, _ = d.Scan()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_, _ = d.Scan()
			}
		}
	}()

	return done
}

// probeSocket connects to a socket and retrieves metadata.
func probeSocket(socketPath string) (*SessionMetadata, error) {
	// Connect with timeout
	conn, err := net.DialTimeout("unix", socketPath, 2*time.Second)
	if err != nil {
		return nil, fmt.Errorf("failed to connect: %w", err)
	}
	defer conn.Close()

	// Set read timeout
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))

	// Read the initial metadata message
	msg, err := DecodeMessage(conn)
	if err != nil {
		return nil, fmt.Errorf("failed to read metadata: %w", err)
	}

	if msg.Type != MessageTypeMetadata {
		return nil, fmt.Errorf("expected metadata message, got type %d", msg.Type)
	}

	return ParseMetadataMessage(msg)
}

// isClaudeCommand checks if a command invokes the claude binary.
// Uses exact basename match to avoid false positives from wrappers or paths containing "claude".
func isClaudeCommand(command string) bool {
	return strings.ToLower(filepath.Base(command)) == "claude"
}

// CleanStaleSocket removes a socket file if it's no longer connected to a running process.
func CleanStaleSocket(socketPath string) error {
	// Try to connect
	conn, err := net.DialTimeout("unix", socketPath, 500*time.Millisecond)
	if err != nil {
		// Can't connect, likely stale - safe to remove
		return os.Remove(socketPath)
	}
	_ = conn.Close() // just a liveness probe; nothing more to do with this connection
	return nil       // Socket is active, don't remove
}

// CleanAllStaleSockets removes all stale ssq-mux sockets.
func CleanAllStaleSockets() error {
	pattern := filepath.Join(os.TempDir(), "ssq-mux-*.sock")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return err
	}

	for _, socketPath := range matches {
		_ = CleanStaleSocket(socketPath)
	}
	return nil
}
