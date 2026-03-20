package session

import (
	"bufio"
	"context"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"

	"claude-squad/log"
)

// ExternalTmuxStreamer provides terminal content streaming for external sessions.
//
// It uses two strategies in priority order:
//
//  1. Control mode (preferred): Starts "tmux -C attach-session -t <name> -r" which
//     provides real-time %output notifications via the tmux control protocol. When an
//     %output event arrives it signals that the pane content has changed, triggering a
//     single capture-pane call to obtain the full terminal snapshot. This eliminates
//     blind polling while preserving the full-snapshot semantic that consumers expect.
//
//  2. Capture-pane polling (fallback): If control mode fails to start (e.g. older tmux,
//     session not found) the streamer falls back to polling capture-pane every 500ms.
//     This is less responsive but universally compatible.
type ExternalTmuxStreamer struct {
	tmuxSessionName string

	// Content tracking for change detection
	lastContent   string
	lastContentMu sync.RWMutex

	// Consumers receive content updates
	consumers   []func(content string)
	consumersMu sync.RWMutex

	// Lifecycle
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	running   bool
	runningMu sync.Mutex

	// Control mode infrastructure
	controlModeCmd    *exec.Cmd
	controlModeActive bool

	// Configuration
	pollInterval time.Duration
}

// NewExternalTmuxStreamer creates a new tmux-based streamer for an external session.
func NewExternalTmuxStreamer(tmuxSessionName string) *ExternalTmuxStreamer {
	return &ExternalTmuxStreamer{
		tmuxSessionName: tmuxSessionName,
		pollInterval:    500 * time.Millisecond, // Fallback poll interval (only used when control mode unavailable)
	}
}

// Start begins streaming the tmux session for content changes.
// It first attempts to use tmux control mode for event-driven updates.
// If control mode is unavailable, it falls back to capture-pane polling.
func (s *ExternalTmuxStreamer) Start() error {
	s.runningMu.Lock()
	if s.running {
		s.runningMu.Unlock()
		return nil // Already running
	}
	s.running = true
	s.runningMu.Unlock()

	s.ctx, s.cancel = context.WithCancel(context.Background())

	// Get initial content
	content, err := s.capturePane()
	if err != nil {
		log.WarningLog.Printf("Initial capture failed for external session '%s': %v", s.tmuxSessionName, err)
		// Continue anyway - session might not be fully ready yet
	} else {
		s.lastContentMu.Lock()
		s.lastContent = content
		s.lastContentMu.Unlock()
	}

	// Try to start control mode for event-driven streaming
	if s.startControlMode() {
		log.InfoLog.Printf("ExternalTmuxStreamer started for session '%s' (control mode)", s.tmuxSessionName)
	} else {
		// Fall back to polling
		log.InfoLog.Printf("ExternalTmuxStreamer started for session '%s' (capture-pane polling at %v)",
			s.tmuxSessionName, s.pollInterval)
		s.wg.Add(1)
		go s.pollLoop()
	}

	return nil
}

// Stop stops the streamer.
func (s *ExternalTmuxStreamer) Stop() {
	s.runningMu.Lock()
	if !s.running {
		s.runningMu.Unlock()
		return
	}
	s.running = false
	s.runningMu.Unlock()

	if s.cancel != nil {
		s.cancel()
	}

	// Kill control mode process if running
	s.stopControlMode()

	s.wg.Wait()

	log.InfoLog.Printf("ExternalTmuxStreamer stopped for session '%s'", s.tmuxSessionName)
}

// IsRunning returns whether the streamer is currently running.
func (s *ExternalTmuxStreamer) IsRunning() bool {
	s.runningMu.Lock()
	defer s.runningMu.Unlock()
	return s.running
}

// AddConsumer registers a callback to receive content updates.
// The consumer will be called with the full terminal content whenever it changes.
func (s *ExternalTmuxStreamer) AddConsumer(consumer func(content string)) {
	s.consumersMu.Lock()
	s.consumers = append(s.consumers, consumer)
	s.consumersMu.Unlock()

	// Send current content immediately to the new consumer
	s.lastContentMu.RLock()
	content := s.lastContent
	s.lastContentMu.RUnlock()

	if content != "" {
		go consumer(content)
	}
}

// RemoveConsumer removes a consumer. Note: this uses pointer comparison
// which may not work for closures.
func (s *ExternalTmuxStreamer) RemoveConsumer(consumer func(content string)) {
	s.consumersMu.Lock()
	defer s.consumersMu.Unlock()

	for i := range s.consumers {
		// Can't reliably compare function pointers, but this is the best we can do
		// In practice, consumers should track their own lifecycle
		_ = i
	}
	// For now, consumers are not removed - they'll fail silently if channel is closed
}

// GetContent returns the current terminal content.
func (s *ExternalTmuxStreamer) GetContent() string {
	s.lastContentMu.RLock()
	defer s.lastContentMu.RUnlock()
	return s.lastContent
}

// ConsumerCount returns the number of registered consumers.
func (s *ExternalTmuxStreamer) ConsumerCount() int {
	s.consumersMu.RLock()
	defer s.consumersMu.RUnlock()
	return len(s.consumers)
}

// ---------------------------------------------------------------------------
// Control mode: event-driven streaming via tmux -C attach-session -t <name> -r
// ---------------------------------------------------------------------------

// startControlMode starts a read-only tmux control mode attachment.
// Returns true if control mode started successfully, false if it failed (caller
// should fall back to polling).
func (s *ExternalTmuxStreamer) startControlMode() bool {
	cmd := exec.Command("tmux", "-C", "attach-session", "-t", s.tmuxSessionName, "-r")

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.WarningLog.Printf("Control mode stdout pipe failed for '%s': %v", s.tmuxSessionName, err)
		return false
	}

	// Capture stderr for diagnostics but don't block on it
	stderr, err := cmd.StderrPipe()
	if err != nil {
		stdout.Close()
		log.WarningLog.Printf("Control mode stderr pipe failed for '%s': %v", s.tmuxSessionName, err)
		return false
	}

	if err := cmd.Start(); err != nil {
		log.WarningLog.Printf("Control mode failed to start for '%s': %v", s.tmuxSessionName, err)
		return false
	}

	s.controlModeCmd = cmd
	s.controlModeActive = true

	log.InfoLog.Printf("Control mode started for external session '%s' (pid: %d)",
		s.tmuxSessionName, cmd.Process.Pid)

	// Goroutine: read control mode stdout and trigger captures on %output events
	s.wg.Add(1)
	go s.readControlMode(stdout)

	// Goroutine: drain stderr
	go s.drainStderr(stderr)

	return true
}

// stopControlMode terminates the control mode process.
func (s *ExternalTmuxStreamer) stopControlMode() {
	if s.controlModeCmd == nil {
		return
	}

	s.controlModeActive = false

	// Kill the process
	if s.controlModeCmd.Process != nil {
		s.controlModeCmd.Process.Kill()
	}

	// Wait with a timeout to avoid blocking forever
	done := make(chan error, 1)
	go func() {
		done <- s.controlModeCmd.Wait()
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		log.WarningLog.Printf("Control mode process for '%s' did not exit, force killing", s.tmuxSessionName)
		if s.controlModeCmd.Process != nil {
			s.controlModeCmd.Process.Kill()
		}
		<-done
	}

	s.controlModeCmd = nil
}

// readControlMode reads control mode output and triggers capture-pane when %output is received.
// It debounces rapid %output events to avoid excessive capture-pane calls.
func (s *ExternalTmuxStreamer) readControlMode(stdout io.ReadCloser) {
	defer s.wg.Done()
	defer stdout.Close()

	scanner := bufio.NewScanner(stdout)

	// Debounce: coalesce rapid %output events into a single capture-pane call.
	// notifyCh is written to on every %output, and a separate goroutine drains it
	// with a debounce window.
	notifyCh := make(chan struct{}, 1)

	s.wg.Add(1)
	go s.debounceCaptures(notifyCh)

	for scanner.Scan() {
		select {
		case <-s.ctx.Done():
			return
		default:
		}

		line := scanner.Text()
		if line == "" {
			continue
		}

		// We only care about %output events -- they signal pane content changed.
		// Other events (%begin, %end, %exit, %session-changed) are logged for debugging.
		if strings.HasPrefix(line, "%output") {
			// Signal that content has changed
			select {
			case notifyCh <- struct{}{}:
			default:
				// Already signaled, debouncer will handle it
			}
		} else if strings.HasPrefix(line, "%exit") {
			log.InfoLog.Printf("Control mode received %%exit for external session '%s'", s.tmuxSessionName)
			return
		} else if strings.HasPrefix(line, "%error") {
			log.WarningLog.Printf("Control mode error for '%s': %s", s.tmuxSessionName, line)
		}
		// Ignore %begin, %end, %session-changed, and other notifications silently
	}

	if err := scanner.Err(); err != nil && err != io.EOF {
		select {
		case <-s.ctx.Done():
			// Expected during shutdown
		default:
			log.WarningLog.Printf("Control mode scanner error for '%s': %v", s.tmuxSessionName, err)
		}
	}

	log.InfoLog.Printf("Control mode reader finished for external session '%s'", s.tmuxSessionName)

	// If control mode exits unexpectedly while we're still running, fall back to polling
	s.runningMu.Lock()
	stillRunning := s.running
	s.runningMu.Unlock()

	if stillRunning && s.controlModeActive {
		s.controlModeActive = false
		log.WarningLog.Printf("Control mode exited for '%s', falling back to capture-pane polling", s.tmuxSessionName)
		s.wg.Add(1)
		go s.pollLoop()
	}
}

// debounceCaptures coalesces rapid change notifications into capture-pane calls.
// It waits for a brief quiet period (50ms) after the last notification before capturing,
// ensuring that bursts of %output events result in a single capture rather than many.
func (s *ExternalTmuxStreamer) debounceCaptures(notifyCh <-chan struct{}) {
	defer s.wg.Done()

	const debounceDelay = 50 * time.Millisecond
	timer := time.NewTimer(debounceDelay)
	timer.Stop() // Don't fire initially
	defer timer.Stop()

	pending := false

	for {
		select {
		case <-s.ctx.Done():
			return

		case _, ok := <-notifyCh:
			if !ok {
				return
			}
			// Reset debounce timer on each notification
			pending = true
			timer.Reset(debounceDelay)

		case <-timer.C:
			if pending {
				pending = false
				s.checkForUpdates()
			}
		}
	}
}

// drainStderr reads and logs stderr from the control mode process.
func (s *ExternalTmuxStreamer) drainStderr(stderr io.ReadCloser) {
	defer stderr.Close()
	scanner := bufio.NewScanner(stderr)
	for scanner.Scan() {
		line := scanner.Text()
		if line != "" {
			log.WarningLog.Printf("Control mode stderr for '%s': %s", s.tmuxSessionName, line)
		}
	}
}

// ---------------------------------------------------------------------------
// Capture-pane polling (fallback when control mode is unavailable)
// ---------------------------------------------------------------------------

// pollLoop continuously polls tmux for content changes.
func (s *ExternalTmuxStreamer) pollLoop() {
	defer s.wg.Done()

	ticker := time.NewTicker(s.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.checkForUpdates()
		}
	}
}

// ---------------------------------------------------------------------------
// Shared helpers (used by both control mode and polling)
// ---------------------------------------------------------------------------

// checkForUpdates captures the pane and notifies consumers if content changed.
func (s *ExternalTmuxStreamer) checkForUpdates() {
	content, err := s.capturePane()
	if err != nil {
		// Session might have ended
		log.DebugLog.Printf("Capture failed for '%s': %v", s.tmuxSessionName, err)
		return
	}

	// Check if content changed
	s.lastContentMu.Lock()
	changed := content != s.lastContent
	if changed {
		s.lastContent = content
	}
	s.lastContentMu.Unlock()

	if changed {
		s.notifyConsumers(content)
	}
}

// capturePane runs tmux capture-pane to get the terminal content.
func (s *ExternalTmuxStreamer) capturePane() (string, error) {
	// Use -e to preserve ANSI escape sequences (colors)
	// Use -p to print to stdout
	// Use -J to join wrapped lines
	cmd := exec.Command("tmux", "capture-pane", "-p", "-e", "-J", "-t", s.tmuxSessionName)
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(output), nil
}

// notifyConsumers sends content to all registered consumers.
func (s *ExternalTmuxStreamer) notifyConsumers(content string) {
	s.consumersMu.RLock()
	consumers := make([]func(string), len(s.consumers))
	copy(consumers, s.consumers)
	s.consumersMu.RUnlock()

	for _, consumer := range consumers {
		go func(c func(string)) {
			defer func() {
				if r := recover(); r != nil {
					log.WarningLog.Printf("Consumer panic: %v", r)
				}
			}()
			c(content)
		}(consumer)
	}
}

// ExternalTmuxStreamerManager manages multiple external tmux streamers.
type ExternalTmuxStreamerManager struct {
	streamers   map[string]*ExternalTmuxStreamer
	streamersMu sync.RWMutex
}

// NewExternalTmuxStreamerManager creates a new streamer manager.
func NewExternalTmuxStreamerManager() *ExternalTmuxStreamerManager {
	return &ExternalTmuxStreamerManager{
		streamers: make(map[string]*ExternalTmuxStreamer),
	}
}

// GetOrCreate returns an existing streamer or creates a new one.
func (m *ExternalTmuxStreamerManager) GetOrCreate(tmuxSessionName string) (*ExternalTmuxStreamer, error) {
	m.streamersMu.Lock()
	defer m.streamersMu.Unlock()

	if streamer, exists := m.streamers[tmuxSessionName]; exists {
		return streamer, nil
	}

	streamer := NewExternalTmuxStreamer(tmuxSessionName)
	if err := streamer.Start(); err != nil {
		return nil, err
	}

	m.streamers[tmuxSessionName] = streamer
	return streamer, nil
}

// Get returns a streamer if it exists.
func (m *ExternalTmuxStreamerManager) Get(tmuxSessionName string) *ExternalTmuxStreamer {
	m.streamersMu.RLock()
	defer m.streamersMu.RUnlock()
	return m.streamers[tmuxSessionName]
}

// Remove stops and removes a streamer.
func (m *ExternalTmuxStreamerManager) Remove(tmuxSessionName string) {
	m.streamersMu.Lock()
	defer m.streamersMu.Unlock()

	if streamer, exists := m.streamers[tmuxSessionName]; exists {
		streamer.Stop()
		delete(m.streamers, tmuxSessionName)
	}
}

// StopAll stops all streamers.
func (m *ExternalTmuxStreamerManager) StopAll() {
	m.streamersMu.Lock()
	defer m.streamersMu.Unlock()

	for _, streamer := range m.streamers {
		streamer.Stop()
	}
	m.streamers = make(map[string]*ExternalTmuxStreamer)
}

// Count returns the number of active streamers.
func (m *ExternalTmuxStreamerManager) Count() int {
	m.streamersMu.RLock()
	defer m.streamersMu.RUnlock()
	return len(m.streamers)
}
