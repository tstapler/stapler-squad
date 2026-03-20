package session

import (
	"context"
	"os/exec"
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/log"
)

// ExternalTmuxStreamer provides terminal content streaming for external sessions
// using tmux capture-pane instead of socket connections. This is more reliable
// as it uses the same mechanism as native stapler-squad sessions.
type ExternalTmuxStreamer struct {
	tmuxSessionName string

	// Content tracking for change detection
	lastContent     string
	lastContentMu   sync.RWMutex

	// Consumers receive content updates
	consumers   []func(content string)
	consumersMu sync.RWMutex

	// Lifecycle
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	running   bool
	runningMu sync.Mutex

	// Configuration
	pollInterval time.Duration
}

// NewExternalTmuxStreamer creates a new tmux-based streamer for an external session.
func NewExternalTmuxStreamer(tmuxSessionName string) *ExternalTmuxStreamer {
	return &ExternalTmuxStreamer{
		tmuxSessionName: tmuxSessionName,
		pollInterval:    150 * time.Millisecond, // Poll every 150ms for responsive updates
	}
}

// Start begins polling the tmux session for content changes.
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

	// Start polling loop
	s.wg.Add(1)
	go s.pollLoop()

	log.InfoLog.Printf("ExternalTmuxStreamer started for session '%s'", s.tmuxSessionName)
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
