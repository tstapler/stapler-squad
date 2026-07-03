package session

import "github.com/linkdata/deadlock"

import (
	"context"
	"fmt"
	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/pkg/analytics"
	"io"
	"strings"
	"sync"
	"time"
)

// ResponseChunk represents a chunk of output from the Claude instance.
type ResponseChunk struct {
	Data      []byte
	Timestamp time.Time
	Error     error
}

// Subscriber represents a client that is receiving response chunks.
type Subscriber struct {
	ID      string
	Ch      chan ResponseChunk
	created time.Time
}

// exitTailSize is the number of bytes kept in the rolling pre-exit buffer.
const exitTailSize = 2048

// ResponseStream manages real-time streaming of Claude instance responses to multiple subscribers.
// It reads from the PTY access layer and broadcasts output to all active subscribers.
type ResponseStream struct {
	sessionName  string
	ptyAccess    *PTYAccess
	subscribers  map[string]*Subscriber
	mu           deadlock.RWMutex
	exitTailMu   sync.Mutex // protects exitTail independently of subscriber state
	ctx          context.Context
	cancel       context.CancelFunc
	wg           sync.WaitGroup
	started      bool
	bufferSize   int                         // Channel buffer size for each subscriber
	escapeParser *analytics.EscapeCodeParser // For escape code analytics
	onOutput     func()                      // Called on every PTY read with data (for event-driven activity tracking)
	OnEOF        func()                      // Called when the PTY exits unexpectedly (program exit, not Stop())
	exitTail     []byte                      // Rolling buffer of last exitTailSize bytes; logged on PTY EOF
}

// mangleCorrelatorTTL is how long a Stage 1 observation waits for its matching
// Stage 2 observation before being evicted and recorded as "stripped".
const mangleCorrelatorTTL = 5 * time.Second

// mangleCorrelatorMaxPending bounds the correlator's in-memory pending map.
const mangleCorrelatorMaxPending = 10000

// newEscapeParserForSession creates and configures an EscapeCodeParser for a session,
// wiring in the global escape event writer and config-driven settings.
func newEscapeParserForSession(sessionName string) *analytics.EscapeCodeParser {
	cfg := loadAnalyticsConfig()
	parser := analytics.NewEscapeCodeParser(analytics.GetGlobalStore(), sessionName)
	writer := analytics.GetGlobalEscapeWriter()
	if cfg.captureLevel != "off" {
		parser.SetEventWriter(writer, cfg.captureLevel, cfg.redactOSC, cfg.samplingRate)
		parser.SetCorrelator(analytics.NewMangleCorrelator(mangleCorrelatorTTL, mangleCorrelatorMaxPending))
	} else {
		parser.SetEventWriter(analytics.NoopEscapeEventWriter{}, "off", true, 0)
	}
	parser.SetEnabled(true)
	return parser
}

// escapeAnalyticsConfig holds the subset of config fields needed for parser wiring.
type escapeAnalyticsConfig struct {
	captureLevel string
	redactOSC    bool
	samplingRate float64
}

// loadAnalyticsConfig reads the current config for escape analytics.
// Falls back to safe defaults if config cannot be loaded.
func loadAnalyticsConfig() escapeAnalyticsConfig {
	cfg := escapeAnalyticsConfig{
		captureLevel: "summary",
		redactOSC:    true,
		samplingRate: 1.0,
	}
	appCfg := loadAppConfig()
	if appCfg.EscapeAnalyticsCaptureLevel != "" {
		cfg.captureLevel = appCfg.EscapeAnalyticsCaptureLevel
	}
	cfg.redactOSC = appCfg.OSCPayloadsAreRedacted()
	if appCfg.EscapeAnalyticsSamplingRate != nil {
		cfg.samplingRate = *appCfg.EscapeAnalyticsSamplingRate
	}
	return cfg
}

// loadAppConfig loads the application config. Always returns a non-nil config;
// config.LoadConfig falls back to DefaultConfig on any load error.
func loadAppConfig() *config.Config {
	appCfg := config.LoadConfig()
	return appCfg
}

// NewResponseStream creates a new response stream for the given session.
// The bufferSize parameter determines how many chunks can be buffered per subscriber.
func NewResponseStream(sessionName string, ptyAccess *PTYAccess) *ResponseStream {
	return &ResponseStream{
		sessionName:  sessionName,
		ptyAccess:    ptyAccess,
		subscribers:  make(map[string]*Subscriber),
		bufferSize:   10000, // Large buffer to handle high-output scenarios (build errors, code generation)
		started:      false,
		escapeParser: newEscapeParserForSession(sessionName),
	}
}

// NewResponseStreamWithBuffer creates a response stream with a custom buffer size.
func NewResponseStreamWithBuffer(sessionName string, ptyAccess *PTYAccess, bufferSize int) *ResponseStream {
	return &ResponseStream{
		sessionName:  sessionName,
		ptyAccess:    ptyAccess,
		subscribers:  make(map[string]*Subscriber),
		bufferSize:   bufferSize,
		started:      false,
		escapeParser: newEscapeParserForSession(sessionName),
	}
}

// SetOnOutput registers a callback invoked each time PTY bytes arrive.
// Used by ClaudeController to drive event-based activity tracking in IdleDetector.
// Must be called before Start().
func (rs *ResponseStream) SetOnOutput(fn func()) {
	rs.onOutput = fn
}

// SetStableSessionID switches the escape parser's recorded session identifier
// from the tmux session name (used at construction time, before the owning
// Instance's stable UUID is available) to the stable UUID. This only affects
// how escape_event rows are tagged — it does not change rs.sessionName, which
// is still used for logging, PTY naming, and history keyed off the tmux name.
func (rs *ResponseStream) SetStableSessionID(id string) {
	if rs.escapeParser != nil && id != "" {
		rs.escapeParser.SetStableSessionID(id)
	}
}

// Start begins streaming responses from the PTY to all subscribers.
// This is a non-blocking call that starts a background goroutine.
// Use the provided context to stop the stream.
func (rs *ResponseStream) Start(ctx context.Context) error {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	if rs.started {
		return fmt.Errorf("response stream already started for session '%s'", rs.sessionName)
	}

	if rs.ptyAccess == nil {
		return fmt.Errorf("PTY access not initialized for session '%s'", rs.sessionName)
	}

	innerCtx, cancel := context.WithCancel(ctx)
	rs.ctx = innerCtx
	rs.cancel = cancel
	rs.started = true

	// Start the mangle-correlator eviction loop (no-op if no correlator is attached,
	// e.g. capture_level=off). Tied to innerCtx so it stops when the stream stops, and
	// tracked by rs.wg like streamLoop so Stop() genuinely blocks until both have exited
	// (Stop()'s doc comment promises full drain, not "everything but this one goroutine").
	if rs.escapeParser != nil {
		rs.wg.Add(1)
		go func() {
			defer rs.wg.Done()
			rs.escapeParser.RunCorrelatorEviction(innerCtx)
		}()
	}

	// Start the streaming goroutine
	rs.wg.Add(1)
	go rs.streamLoop(innerCtx)

	log.Info("response stream started", "session", rs.sessionName)
	return nil
}

// logEscapeAnalyticsSummary logs a summary of escape analytics for NFR-4.
func (rs *ResponseStream) logEscapeAnalyticsSummary() {
	if rs.escapeParser == nil {
		return
	}
	stats := rs.escapeParser.GetStats()
	log.Info("escape analytics: session closed",
		"session", rs.sessionName,
		"sequences", stats.TotalSequences,
		"mangled", stats.TotalMangled,
	)
}

// streamLoop is the main streaming loop that reads from PTY and broadcasts to subscribers.
func (rs *ResponseStream) streamLoop(ctx context.Context) {
	defer rs.wg.Done()
	defer rs.logEscapeAnalyticsSummary()
	defer log.Info("response stream stopped", "session", rs.sessionName)

	// Buffer for reading PTY output
	readBuf := make([]byte, 4096)

	for {
		select {
		case <-ctx.Done():
			// Stream was cancelled
			rs.closeAllSubscribers()
			return
		default:
			// Try to read from PTY with timeout
			pty, closed := rs.ptyAccess.GetFile()

			if closed {
				// PTY is closed, stop streaming
				rs.closeAllSubscribers()
				return
			}

			if pty == nil {
				// PTY not available, wait a bit
				time.Sleep(100 * time.Millisecond)
				continue
			}

			// Set read deadline to avoid blocking forever
			pty.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
			n, err := pty.Read(readBuf)

			if err != nil {
				if err == io.EOF {
					// PTY closed - the tmux session's program has exited
					log.ForSession(rs.sessionName).Info("session program exited (PTY EOF)")
					rs.closeAllSubscribers()
					rs.mu.Lock()
					rs.started = false
					rs.mu.Unlock()
					if rs.OnEOF != nil {
						rs.OnEOF()
					}
					return
				}
				// Check if it's a timeout error
				if netErr, ok := err.(interface{ Timeout() bool }); ok && netErr.Timeout() {
					// Timeout is expected, continue loop
					continue
				}
				// Check for "file already closed" or Linux PTY "input/output error" which indicate EOF
				errMsg := err.Error()
				if strings.Contains(errMsg, "file already closed") ||
					strings.Contains(errMsg, "bad file descriptor") ||
					strings.Contains(errMsg, "input/output error") {
					// PTY has been closed - the tmux session's program has exited
					log.ForSession(rs.sessionName).Info("session program exited (PTY closed)", "err", err)
					rs.closeAllSubscribers()
					rs.mu.Lock()
					rs.started = false
					rs.mu.Unlock()
					if rs.OnEOF != nil {
						rs.OnEOF()
					}
					return
				}
				// Other errors - log and continue
				log.Error("error reading from PTY in response stream", "session", rs.sessionName, "err", err)
				continue
			}

			if n > 0 {
				// Update rolling pre-exit tail buffer (keeps last exitTailSize bytes).
				rs.exitTailMu.Lock()
				combined := append(rs.exitTail, readBuf[:n]...)
				if len(combined) > exitTailSize {
					combined = combined[len(combined)-exitTailSize:]
				}
				rs.exitTail = combined
				rs.exitTailMu.Unlock()

				// Got some data, broadcast to subscribers
				chunk := ResponseChunk{
					Data:      make([]byte, n),
					Timestamp: time.Now(),
				}
				copy(chunk.Data, readBuf[:n])

				// Notify activity listener (e.g. IdleDetector.RecordActivity)
				if rs.onOutput != nil {
					rs.onOutput()
				}

				// Capture the byte offset BEFORE writing to buffer so sessionSeq
				// represents the start of this chunk in the cumulative stream.
				var sessionSeq int64
				if rs.ptyAccess.buffer != nil {
					sessionSeq = rs.ptyAccess.buffer.TotalBytesWritten()
				}

				// Parse escape codes for analytics (passthrough - doesn't modify data)
				if rs.escapeParser != nil {
					rs.escapeParser.Parse(chunk.Data, sessionSeq)
				}

				// Also write to circular buffer for history
				if rs.ptyAccess.buffer != nil {
					rs.ptyAccess.buffer.Write(chunk.Data)
				}

				rs.broadcast(chunk)
			}
		}
	}
}

// broadcast sends a response chunk to all subscribers.
func (rs *ResponseStream) broadcast(chunk ResponseChunk) {
	rs.mu.RLock()
	defer rs.mu.RUnlock()

	for id, sub := range rs.subscribers {
		select {
		case sub.Ch <- chunk:
			// Successfully sent
		default:
			// Channel is full, log warning but don't block
			log.Warn("subscriber channel full, dropping chunk", "subscriber", id, "session", rs.sessionName)
		}
	}
}

// Subscribe registers a new subscriber and returns a channel for receiving response chunks.
// The subscriber ID should be unique. Returns an error if the ID is already in use.
func (rs *ResponseStream) Subscribe(subscriberID string) (<-chan ResponseChunk, error) {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	if _, exists := rs.subscribers[subscriberID]; exists {
		return nil, fmt.Errorf("subscriber '%s' already exists for session '%s'", subscriberID, rs.sessionName)
	}

	sub := &Subscriber{
		ID:      subscriberID,
		Ch:      make(chan ResponseChunk, rs.bufferSize),
		created: time.Now(),
	}

	rs.subscribers[subscriberID] = sub
	log.Info("subscriber registered", "subscriber", subscriberID, "session", rs.sessionName)

	return sub.Ch, nil
}

// Unsubscribe removes a subscriber and closes their channel.
func (rs *ResponseStream) Unsubscribe(subscriberID string) error {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	sub, exists := rs.subscribers[subscriberID]
	if !exists {
		return fmt.Errorf("subscriber '%s' not found for session '%s'", subscriberID, rs.sessionName)
	}

	close(sub.Ch)
	delete(rs.subscribers, subscriberID)
	log.Info("subscriber unregistered", "subscriber", subscriberID, "session", rs.sessionName)

	return nil
}

// closeAllSubscribers closes all subscriber channels.
func (rs *ResponseStream) closeAllSubscribers() {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	for id, sub := range rs.subscribers {
		close(sub.Ch)
		log.Info("closed subscriber", "subscriber", id, "session", rs.sessionName)
	}
	rs.subscribers = make(map[string]*Subscriber)
}

// Stop stops the response stream and closes all subscriber channels.
// This is a blocking call that waits for the streaming goroutine to finish.
func (rs *ResponseStream) Stop() error {
	rs.mu.Lock()
	if !rs.started {
		rs.mu.Unlock()
		return fmt.Errorf("response stream not started for session '%s'", rs.sessionName)
	}
	rs.mu.Unlock()

	// Cancel context to signal stop
	if rs.cancel != nil {
		rs.cancel()
	}

	// Wait for streaming goroutine to finish
	rs.wg.Wait()

	rs.mu.Lock()
	rs.started = false
	rs.mu.Unlock()

	log.Info("response stream stopped", "session", rs.sessionName)
	return nil
}

// GetSubscriberCount returns the number of active subscribers.
func (rs *ResponseStream) GetSubscriberCount() int {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	return len(rs.subscribers)
}

// GetSubscriberIDs returns the IDs of all active subscribers.
func (rs *ResponseStream) GetSubscriberIDs() []string {
	rs.mu.RLock()
	defer rs.mu.RUnlock()

	ids := make([]string, 0, len(rs.subscribers))
	for id := range rs.subscribers {
		ids = append(ids, id)
	}
	return ids
}

// IsStarted returns whether the stream is currently active.
func (rs *ResponseStream) IsStarted() bool {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	return rs.started
}

// GetSubscriberInfo returns information about a specific subscriber.
func (rs *ResponseStream) GetSubscriberInfo(subscriberID string) (created time.Time, exists bool) {
	rs.mu.RLock()
	defer rs.mu.RUnlock()

	sub, exists := rs.subscribers[subscriberID]
	if !exists {
		return time.Time{}, false
	}
	return sub.created, true
}

// SetBufferSize sets the buffer size for future subscribers.
// Does not affect existing subscribers.
func (rs *ResponseStream) SetBufferSize(size int) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	rs.bufferSize = size
}

// GetBufferSize returns the current buffer size setting.
func (rs *ResponseStream) GetBufferSize() int {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	return rs.bufferSize
}

// GetEscapeParser returns the escape code parser for this stream.
// Used by the WebSocket handler for Stage 2 analytics observations.
// Returns nil if no parser is configured.
func (rs *ResponseStream) GetEscapeParser() *analytics.EscapeCodeParser {
	return rs.escapeParser
}

// GetTotalBytesWritten returns the monotonic PTY byte offset from the circular
// buffer. This is the same counter used by Stage 1 (Parse) so Stage 2
// (ParseStage2) session_seq values are stable across WebSocket reconnections.
// Returns 0 if no buffer is available.
func (rs *ResponseStream) GetTotalBytesWritten() int64 {
	if rs.ptyAccess == nil || rs.ptyAccess.buffer == nil {
		return 0
	}
	return rs.ptyAccess.buffer.TotalBytesWritten()
}

// GetExitTail returns a copy of the last bytes seen before the PTY exited.
// Returns nil if the stream has not yet exited or no output was captured.
func (rs *ResponseStream) GetExitTail() []byte {
	rs.exitTailMu.Lock()
	defer rs.exitTailMu.Unlock()
	if len(rs.exitTail) == 0 {
		return nil
	}
	out := make([]byte, len(rs.exitTail))
	copy(out, rs.exitTail)
	return out
}
