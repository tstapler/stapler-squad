package session

import "github.com/linkdata/deadlock"

import (
	"context"
	"fmt"
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

// NewResponseStream creates a new response stream for the given session.
// The bufferSize parameter determines how many chunks can be buffered per subscriber.
func NewResponseStream(sessionName string, ptyAccess *PTYAccess) *ResponseStream {
	// Create escape code parser using global store
	escapeParser := analytics.NewEscapeCodeParser(analytics.GetGlobalStore(), sessionName)
	// Parser is enabled/disabled via the global store's enabled state
	escapeParser.SetEnabled(true) // Always parse when store is enabled

	return &ResponseStream{
		sessionName:  sessionName,
		ptyAccess:    ptyAccess,
		subscribers:  make(map[string]*Subscriber),
		bufferSize:   10000, // Large buffer to handle high-output scenarios (build errors, code generation)
		started:      false,
		escapeParser: escapeParser,
	}
}

// NewResponseStreamWithBuffer creates a response stream with a custom buffer size.
func NewResponseStreamWithBuffer(sessionName string, ptyAccess *PTYAccess, bufferSize int) *ResponseStream {
	// Create escape code parser using global store
	escapeParser := analytics.NewEscapeCodeParser(analytics.GetGlobalStore(), sessionName)
	escapeParser.SetEnabled(true) // Always parse when store is enabled

	return &ResponseStream{
		sessionName:  sessionName,
		ptyAccess:    ptyAccess,
		subscribers:  make(map[string]*Subscriber),
		bufferSize:   bufferSize,
		started:      false,
		escapeParser: escapeParser,
	}
}

// SetOnOutput registers a callback invoked each time PTY bytes arrive.
// Used by ClaudeController to drive event-based activity tracking in IdleDetector.
// Must be called before Start().
func (rs *ResponseStream) SetOnOutput(fn func()) {
	rs.onOutput = fn
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

	rs.ctx, rs.cancel = context.WithCancel(ctx)
	rs.started = true

	// Start the streaming goroutine
	rs.wg.Add(1)
	go rs.streamLoop()

	log.InfoLog.Printf("Response stream started for session '%s'", rs.sessionName)
	return nil
}

// streamLoop is the main streaming loop that reads from PTY and broadcasts to subscribers.
func (rs *ResponseStream) streamLoop() {
	defer rs.wg.Done()
	defer log.InfoLog.Printf("Response stream stopped for session '%s'", rs.sessionName)

	// Buffer for reading PTY output
	readBuf := make([]byte, 4096)

	for {
		select {
		case <-rs.ctx.Done():
			// Stream was cancelled
			rs.closeAllSubscribers()
			return
		default:
			// Try to read from PTY with timeout
			rs.ptyAccess.mu.RLock()
			pty := rs.ptyAccess.pty
			closed := rs.ptyAccess.closed
			rs.ptyAccess.mu.RUnlock()

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
					log.InfoLog.Printf("Session '%s': PTY reached EOF - program exited", rs.sessionName)
					log.ForSession(rs.sessionName).Info("Session program exited (PTY EOF)")
					if len(rs.exitTail) > 0 {
						log.InfoLog.Printf("Session '%s': last output before exit:\n%s", rs.sessionName, string(rs.exitTail))
					}
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
					log.InfoLog.Printf("Session '%s': PTY closed (%v) - program exited", rs.sessionName, err)
					log.ForSession(rs.sessionName).Info("Session program exited (PTY closed: %v)", err)
					if len(rs.exitTail) > 0 {
						log.InfoLog.Printf("Session '%s': last output before exit:\n%s", rs.sessionName, string(rs.exitTail))
					}
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
				log.ErrorLog.Printf("Error reading from PTY in response stream for '%s': %v", rs.sessionName, err)
				continue
			}

			if n > 0 {
				// Update rolling pre-exit tail buffer (keeps last exitTailSize bytes).
				rs.mu.Lock()
				combined := append(rs.exitTail, readBuf[:n]...)
				if len(combined) > exitTailSize {
					combined = combined[len(combined)-exitTailSize:]
				}
				rs.exitTail = combined
				rs.mu.Unlock()

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

				// Parse escape codes for analytics (passthrough - doesn't modify data)
				if rs.escapeParser != nil {
					rs.escapeParser.Parse(chunk.Data)
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
			log.WarningLog.Printf("Subscriber %s channel full in session '%s', dropping chunk", id, rs.sessionName)
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
	log.InfoLog.Printf("Subscriber '%s' registered for session '%s'", subscriberID, rs.sessionName)

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
	log.InfoLog.Printf("Subscriber '%s' unregistered from session '%s'", subscriberID, rs.sessionName)

	return nil
}

// closeAllSubscribers closes all subscriber channels.
func (rs *ResponseStream) closeAllSubscribers() {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	for id, sub := range rs.subscribers {
		close(sub.Ch)
		log.InfoLog.Printf("Closed subscriber '%s' for session '%s'", id, rs.sessionName)
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

	log.InfoLog.Printf("Response stream stopped for session '%s'", rs.sessionName)
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

// GetExitTail returns a copy of the last bytes seen before the PTY exited.
// Returns nil if the stream has not yet exited or no output was captured.
func (rs *ResponseStream) GetExitTail() []byte {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	if len(rs.exitTail) == 0 {
		return nil
	}
	out := make([]byte, len(rs.exitTail))
	copy(out, rs.exitTail)
	return out
}
