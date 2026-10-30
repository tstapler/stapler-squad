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

	// Test seams; zero values mean os.TempDir(), probeSocketContext, time.Now.
	dir   string
	probe func(ctx context.Context, socketPath string) (*SessionMetadata, error)
	now   func() time.Time

	scan scanState
}

const (
	// fullScanBackstop bounds how long an unchanged directory mtime can
	// suppress a rescan, covering coarse-mtime filesystems and missed updates.
	fullScanBackstop = 30 * time.Second
	// probeRetryBudget is how many consecutive scans a socket that fails its
	// probe forces a rescan on its own (it may just not be listening yet);
	// afterwards only the backstop retries it.
	probeRetryBudget = 3
)

// scanState remembers what the last full scan saw so unchanged polls can
// skip it. Guarded by its own mutex: scans may run concurrently with Scan().
type scanState struct {
	mu        sync.Mutex
	dirMtime  time.Time
	scannedAt time.Time
	failed    map[string]int // socket path -> consecutive failed probes
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
	return d.scanContext(context.Background())
}

// scanContext is Scan, abandoned (leaving known sessions untouched) once ctx is done, so a
// socket that accepts but never sends metadata can't hold up shutdown for the probe timeout.
func (d *Discovery) scanContext(ctx context.Context) ([]*DiscoveredSession, error) {
	dir := d.socketDir()
	// Stat before listing: a change landing mid-scan then differs from the
	// recorded mtime and forces the next scan instead of being missed.
	mtime, haveMtime := dirMtime(dir)
	if haveMtime && d.canSkipScan(mtime) {
		return d.touchKnownSessions(), nil
	}

	// Find all potential socket files
	pattern := filepath.Join(dir, "ssq-mux-*.sock")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("failed to glob socket files: %w", err)
	}
	failed := make(map[string]int)

	var discovered []*DiscoveredSession
	var newSessions []*DiscoveredSession
	currentPaths := make(map[string]bool)

	for _, socketPath := range matches {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
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
		meta, err := d.probeSocket(ctx, socketPath)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			// Socket exists but can't connect - might be stale
			failed[socketPath] = d.previousFailures(socketPath) + 1
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

	d.recordScan(mtime, haveMtime, failed)

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

func (d *Discovery) socketDir() string {
	if d.dir != "" {
		return d.dir
	}
	return os.TempDir()
}

func (d *Discovery) clock() time.Time {
	if d.now != nil {
		return d.now()
	}
	return time.Now()
}

func (d *Discovery) probeSocket(ctx context.Context, socketPath string) (*SessionMetadata, error) {
	if d.probe != nil {
		return d.probe(ctx, socketPath)
	}
	return probeSocketContext(ctx, socketPath)
}

func dirMtime(dir string) (time.Time, bool) {
	info, err := os.Stat(dir)
	if err != nil {
		return time.Time{}, false
	}
	return info.ModTime(), true
}

// canSkipScan reports whether the socket directory is unchanged since the last
// full scan, so listing and probing would find nothing new: creating or
// removing a socket bumps the directory mtime. Sockets still within their
// probe retry budget, and the periodic backstop, force a real scan.
func (d *Discovery) canSkipScan(mtime time.Time) bool {
	d.scan.mu.Lock()
	defer d.scan.mu.Unlock()
	if d.scan.scannedAt.IsZero() || !mtime.Equal(d.scan.dirMtime) {
		return false
	}
	if d.clock().Sub(d.scan.scannedAt) >= fullScanBackstop {
		return false
	}
	for _, attempts := range d.scan.failed {
		if attempts < probeRetryBudget {
			return false
		}
	}
	return true
}

func (d *Discovery) previousFailures(socketPath string) int {
	d.scan.mu.Lock()
	defer d.scan.mu.Unlock()
	return d.scan.failed[socketPath]
}

// recordScan stores the outcome of a full scan. Without a readable mtime the
// state is cleared so the next scan runs in full.
func (d *Discovery) recordScan(mtime time.Time, haveMtime bool, failed map[string]int) {
	d.scan.mu.Lock()
	defer d.scan.mu.Unlock()
	if !haveMtime {
		d.scan.scannedAt = time.Time{}
		return
	}
	d.scan.dirMtime = mtime
	d.scan.scannedAt = d.clock()
	d.scan.failed = failed
}

// touchKnownSessions is the skipped-scan result: every known session is still
// present, so refresh LastSeen as a full scan would and return them.
func (d *Discovery) touchKnownSessions() []*DiscoveredSession {
	now := time.Now()
	d.mu.Lock()
	defer d.mu.Unlock()
	sessions := make([]*DiscoveredSession, 0, len(d.sessions))
	for _, session := range d.sessions {
		session.LastSeen = now
		sessions = append(sessions, session)
	}
	return sessions
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
		_, _ = d.scanContext(ctx)

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_, _ = d.scanContext(ctx)
			}
		}
	}()

	return done
}

// probeSocket connects to a socket and retrieves metadata.
func probeSocket(socketPath string) (*SessionMetadata, error) {
	return probeSocketContext(context.Background(), socketPath)
}

// probeSocketContext probes socketPath, aborting the dial or metadata read when ctx is done.
func probeSocketContext(ctx context.Context, socketPath string) (*SessionMetadata, error) {
	dialCtx, cancelDial := context.WithTimeout(ctx, 2*time.Second)
	defer cancelDial()
	var dialer net.Dialer
	conn, err := dialer.DialContext(dialCtx, "unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("failed to connect: %w", err)
	}
	defer conn.Close()

	// Set read timeout, then arrange for ctx cancellation to expire it (order matters: this
	// deadline must not overwrite the cancellation one).
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	stop := context.AfterFunc(ctx, func() { _ = conn.SetReadDeadline(time.Unix(1, 0)) })
	defer stop()

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
