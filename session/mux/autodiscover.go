package mux

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
)

// AutoDiscovery provides filesystem watching for immediate session discovery.
// It watches /tmp/ for claude-mux-*.sock file creation/deletion and immediately
// connects to new sessions without polling delay.
type AutoDiscovery struct {
	*Discovery
	watcher     *fsnotify.Watcher
	watcherDone chan struct{}
	useFallback bool
}

// NewAutoDiscovery creates a new auto-discovery service with filesystem watching.
func NewAutoDiscovery() (*AutoDiscovery, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create filesystem watcher: %w", err)
	}

	return &AutoDiscovery{
		Discovery:   NewDiscovery(),
		watcher:     watcher,
		watcherDone: make(chan struct{}),
		useFallback: false,
	}, nil
}

// NewAutoDiscoveryWithFallback creates an auto-discovery service that falls back
// to polling if filesystem watching fails.
func NewAutoDiscoveryWithFallback() *AutoDiscovery {
	ad, err := NewAutoDiscovery()
	if err != nil {
		// Fall back to polling-only mode
		return &AutoDiscovery{
			Discovery:   NewDiscovery(),
			watcher:     nil,
			watcherDone: make(chan struct{}),
			useFallback: true,
		}
	}
	return ad
}

// Start begins filesystem watching or polling based on availability.
// Returns a done channel that closes when the auto-discovery stops.
func (ad *AutoDiscovery) Start(ctx context.Context) (<-chan struct{}, error) {
	if ad.watcher != nil {
		return ad.startWatching(ctx)
	}

	// Fallback to polling mode
	if ad.useFallback {
		return ad.startPollingFallback(ctx)
	}

	return nil, fmt.Errorf("no watcher available and fallback not enabled")
}

// startWatching starts filesystem watching for immediate discovery.
func (ad *AutoDiscovery) startWatching(ctx context.Context) (<-chan struct{}, error) {
	tmpDir := os.TempDir()

	// Add /tmp to watcher. If this fails (e.g. stale temp files on macOS), fall back to polling.
	if err := ad.watcher.Add(tmpDir); err != nil {
		ad.useFallback = true
		return ad.startPollingFallback(ctx)
	}

	done := make(chan struct{})

	go func() {
		defer close(done)
		defer close(ad.watcherDone)

		// Initial scan to catch existing sessions
		ad.Scan()

		for {
			select {
			case <-ctx.Done():
				return

			case event, ok := <-ad.watcher.Events:
				if !ok {
					return
				}

				// Only process claude-mux socket files
				if !isClaudeMuxSocket(event.Name) {
					continue
				}

				switch {
				case event.Op&fsnotify.Create == fsnotify.Create:
					// New socket created - immediate discovery
					ad.handleSocketCreated(event.Name)

				case event.Op&fsnotify.Remove == fsnotify.Remove:
					// Socket removed - immediate cleanup
					ad.handleSocketRemoved(event.Name)

				case event.Op&fsnotify.Write == fsnotify.Write:
					// Socket modified (unusual but handle it)
					ad.handleSocketModified(event.Name)
				}

			case err, ok := <-ad.watcher.Errors:
				if !ok {
					return
				}
				// Log error but continue watching
				fmt.Fprintf(os.Stderr, "autodiscover: watcher error: %v\n", err)
			}
		}
	}()

	return done, nil
}

// startPollingFallback starts traditional polling when watching unavailable.
func (ad *AutoDiscovery) startPollingFallback(ctx context.Context) (<-chan struct{}, error) {
	// Use default polling interval of 2 seconds
	return ad.Discovery.StartPolling(ctx, 2*time.Second), nil
}

// Stop gracefully stops the auto-discovery service.
func (ad *AutoDiscovery) Stop() error {
	if ad.watcher != nil {
		// Close watcher (will cause event loop to exit)
		if err := ad.watcher.Close(); err != nil {
			return err
		}

		// Only wait for watcher goroutine if Start() was called
		// Use a non-blocking check with a timeout to avoid hanging
		select {
		case <-ad.watcherDone:
			// Goroutine finished
		default:
			// Goroutine might not have been started (Start() never called)
			// Just return - watcher.Close() already cleaned up resources
		}
	}
	return nil
}

// handleSocketCreated processes a newly created socket file.
func (ad *AutoDiscovery) handleSocketCreated(socketPath string) {
	// Give the socket a moment to be fully initialized
	time.Sleep(50 * time.Millisecond)

	// Try to probe the socket
	meta, err := probeSocket(socketPath)
	if err != nil {
		// Socket not ready yet or invalid, skip for now
		// It will be picked up on next scan if valid
		return
	}

	// Create discovered session
	session := &DiscoveredSession{
		SocketPath: socketPath,
		Metadata:   meta,
		LastSeen:   time.Now(),
	}

	// Register the session
	ad.mu.Lock()
	ad.sessions[socketPath] = session
	callbacks := ad.callbacks
	ad.mu.Unlock()

	// Notify callbacks (new session)
	for _, cb := range callbacks {
		cb(session, true)
	}
}

// handleSocketRemoved processes a removed socket file.
func (ad *AutoDiscovery) handleSocketRemoved(socketPath string) {
	ad.mu.Lock()
	session, exists := ad.sessions[socketPath]
	if exists {
		delete(ad.sessions, socketPath)
	}
	callbacks := ad.callbacks
	ad.mu.Unlock()

	// Notify callbacks if session existed (removed)
	if exists {
		for _, cb := range callbacks {
			cb(session, false)
		}
	}
}

// handleSocketModified processes a modified socket file.
func (ad *AutoDiscovery) handleSocketModified(socketPath string) {
	// Socket modification is unusual, but we can try to re-probe
	ad.mu.RLock()
	_, exists := ad.sessions[socketPath]
	ad.mu.RUnlock()

	if !exists {
		// Not tracking yet, treat as creation
		ad.handleSocketCreated(socketPath)
	} else {
		// Already tracking, update last seen
		ad.mu.Lock()
		if session, ok := ad.sessions[socketPath]; ok {
			session.LastSeen = time.Now()
		}
		ad.mu.Unlock()
	}
}

// isClaudeMuxSocket checks if a file path is a claude-mux socket.
func isClaudeMuxSocket(path string) bool {
	base := filepath.Base(path)
	return strings.HasPrefix(base, "claude-mux-") && strings.HasSuffix(base, ".sock")
}

// WatcherActive returns true if filesystem watching is active (not in fallback mode).
func (ad *AutoDiscovery) WatcherActive() bool {
	return ad.watcher != nil && !ad.useFallback
}

// IsUsingFallback returns true if auto-discovery is using polling fallback.
func (ad *AutoDiscovery) IsUsingFallback() bool {
	return ad.useFallback || ad.watcher == nil
}
