package session

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/mux"
)

// OutputConsumer is a callback that receives terminal output from external sessions.
type OutputConsumer func(data []byte)

// ExternalStreamer connects to a mux socket and streams terminal output.
// It handles reconnection and broadcasts output to registered consumers.
type ExternalStreamer struct {
	socketPath string
	conn       net.Conn

	// Output consumers
	consumers   []OutputConsumer
	consumersMu sync.RWMutex

	// Ring buffer for recent output (for new consumers to catch up)
	buffer     *ringBuffer
	bufferSize int

	// Metadata from the mux session
	metadata   *mux.SessionMetadata
	metadataMu sync.RWMutex

	// Snapshot request/response channel (for synchronization with readLoop)
	snapshotReq  chan struct{}
	snapshotResp chan []byte

	// Lifecycle
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	connected  bool
	connMu     sync.RWMutex
	lastError  error
	reconnects int
}

// ringBuffer is a simple ring buffer for storing recent output.
type ringBuffer struct {
	data  []byte
	size  int
	start int
	len   int
	mu    sync.Mutex
}

func newRingBuffer(size int) *ringBuffer {
	return &ringBuffer{
		data: make([]byte, size),
		size: size,
	}
}

func (r *ringBuffer) Write(p []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, b := range p {
		pos := (r.start + r.len) % r.size
		r.data[pos] = b
		if r.len < r.size {
			r.len++
		} else {
			r.start = (r.start + 1) % r.size
		}
	}
}

func (r *ringBuffer) Read() []byte {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.len == 0 {
		return nil
	}

	result := make([]byte, r.len)
	for i := 0; i < r.len; i++ {
		result[i] = r.data[(r.start+i)%r.size]
	}
	return result
}

func (r *ringBuffer) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.start = 0
	r.len = 0
}

// NewExternalStreamer creates a new streamer for the given mux socket.
func NewExternalStreamer(socketPath string, bufferSize int) *ExternalStreamer {
	if bufferSize <= 0 {
		bufferSize = 64 * 1024 // 64KB default
	}
	return &ExternalStreamer{
		socketPath:   socketPath,
		bufferSize:   bufferSize,
		buffer:       newRingBuffer(bufferSize),
		snapshotReq:  make(chan struct{}, 1),
		snapshotResp: make(chan []byte, 1),
	}
}

// SocketPath returns the path to the mux socket.
func (s *ExternalStreamer) SocketPath() string {
	return s.socketPath
}

// IsConnected returns whether the streamer is currently connected.
func (s *ExternalStreamer) IsConnected() bool {
	s.connMu.RLock()
	defer s.connMu.RUnlock()
	return s.connected
}

// GetMetadata returns the session metadata from the mux.
func (s *ExternalStreamer) GetMetadata() *mux.SessionMetadata {
	s.metadataMu.RLock()
	defer s.metadataMu.RUnlock()
	return s.metadata
}

// AddConsumer registers a callback to receive output data.
// If catchUp is true, the consumer receives buffered recent output first.
func (s *ExternalStreamer) AddConsumer(consumer OutputConsumer, catchUp bool) {
	s.consumersMu.Lock()
	s.consumers = append(s.consumers, consumer)
	s.consumersMu.Unlock()

	// Send buffered data to new consumer
	if catchUp {
		if buffered := s.buffer.Read(); len(buffered) > 0 {
			consumer(buffered)
		}
	}
}

// RemoveConsumer unregisters a consumer callback.
// Note: This uses function pointer comparison which may not work for closures.
// Consider using a consumer ID pattern for production use.
func (s *ExternalStreamer) RemoveConsumer(consumer OutputConsumer) {
	s.consumersMu.Lock()
	defer s.consumersMu.Unlock()

	// Find and remove the consumer
	for i, c := range s.consumers {
		// Note: This pointer comparison works for non-closure functions
		if fmt.Sprintf("%p", c) == fmt.Sprintf("%p", consumer) {
			s.consumers = append(s.consumers[:i], s.consumers[i+1:]...)
			return
		}
	}
}

// ConsumerCount returns the number of registered consumers.
func (s *ExternalStreamer) ConsumerCount() int {
	s.consumersMu.RLock()
	defer s.consumersMu.RUnlock()
	return len(s.consumers)
}

// Start connects to the mux socket and begins streaming.
func (s *ExternalStreamer) Start() error {
	s.ctx, s.cancel = context.WithCancel(context.Background())

	// Initial connection
	if err := s.connect(); err != nil {
		return fmt.Errorf("initial connection failed: %w", err)
	}

	// Start read loop
	s.wg.Add(1)
	go s.readLoop()

	log.InfoLog.Printf("External streamer started for socket: %s", s.socketPath)
	return nil
}

// Stop disconnects and stops the streamer.
func (s *ExternalStreamer) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
	s.wg.Wait()

	s.connMu.Lock()
	if s.conn != nil {
		s.conn.Close()
		s.conn = nil
	}
	s.connected = false
	s.connMu.Unlock()

	log.InfoLog.Printf("External streamer stopped for socket: %s", s.socketPath)
}

// SendInput sends input data to the mux session.
func (s *ExternalStreamer) SendInput(data []byte) error {
	s.connMu.RLock()
	conn := s.conn
	connected := s.connected
	s.connMu.RUnlock()

	if !connected || conn == nil {
		return fmt.Errorf("not connected")
	}

	msg := mux.NewInputMessage(data)
	return mux.WriteMessage(conn, msg)
}

// SendResize sends a terminal resize command to the mux session.
func (s *ExternalStreamer) SendResize(cols, rows uint16) error {
	s.connMu.RLock()
	conn := s.conn
	connected := s.connected
	s.connMu.RUnlock()

	if !connected || conn == nil {
		return fmt.Errorf("not connected")
	}

	msg := mux.NewResizeMessage(cols, rows)
	return mux.WriteMessage(conn, msg)
}

// GetRecentOutput returns the buffered recent output.
func (s *ExternalStreamer) GetRecentOutput() []byte {
	return s.buffer.Read()
}

// GetSnapshot requests a clean screen snapshot from the mux session.
// This uses tmux capture-pane on the server side to get clean terminal content
// without ANSI escape sequences, suitable for pattern matching and initial state.
// The snapshot request is coordinated with the readLoop to avoid race conditions.
func (s *ExternalStreamer) GetSnapshot() ([]byte, error) {
	s.connMu.RLock()
	conn := s.conn
	connected := s.connected
	s.connMu.RUnlock()

	if !connected || conn == nil {
		return nil, fmt.Errorf("not connected")
	}

	// Signal the readLoop to request a snapshot
	// Non-blocking send - if channel is full, another request is pending
	select {
	case s.snapshotReq <- struct{}{}:
	default:
		// Request already pending
	}

	// Send snapshot request message
	if err := mux.WriteMessage(conn, mux.NewSnapshotRequestMessage()); err != nil {
		return nil, fmt.Errorf("failed to send snapshot request: %w", err)
	}

	// Wait for response from readLoop with timeout
	select {
	case data := <-s.snapshotResp:
		return data, nil
	case <-time.After(5 * time.Second):
		return nil, fmt.Errorf("timeout waiting for snapshot reply")
	case <-s.ctx.Done():
		return nil, fmt.Errorf("streamer stopped")
	}
}

// connect establishes a connection to the mux socket.
func (s *ExternalStreamer) connect() error {
	conn, err := net.DialTimeout("unix", s.socketPath, 5*time.Second)
	if err != nil {
		return fmt.Errorf("failed to connect to socket: %w", err)
	}

	// Set read deadline for initial metadata
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))

	// Read initial metadata message
	msg, err := mux.DecodeMessage(conn)
	if err != nil {
		conn.Close()
		return fmt.Errorf("failed to read metadata: %w", err)
	}

	if msg.Type != mux.MessageTypeMetadata {
		conn.Close()
		return fmt.Errorf("expected metadata message, got type %d", msg.Type)
	}

	metadata, err := mux.ParseMetadataMessage(msg)
	if err != nil {
		conn.Close()
		return fmt.Errorf("failed to parse metadata: %w", err)
	}

	// Clear deadline for ongoing reads
	conn.SetReadDeadline(time.Time{})

	// Store connection and metadata
	s.connMu.Lock()
	s.conn = conn
	s.connected = true
	s.connMu.Unlock()

	s.metadataMu.Lock()
	s.metadata = metadata
	s.metadataMu.Unlock()

	log.InfoLog.Printf("Connected to mux socket: %s (pid: %d, cwd: %s)",
		s.socketPath, metadata.PID, metadata.Cwd)

	return nil
}

// readLoop continuously reads from the socket and broadcasts to consumers.
func (s *ExternalStreamer) readLoop() {
	defer s.wg.Done()

	reconnectDelay := 1 * time.Second
	maxReconnectDelay := 30 * time.Second

	for {
		select {
		case <-s.ctx.Done():
			return
		default:
		}

		s.connMu.RLock()
		conn := s.conn
		s.connMu.RUnlock()

		if conn == nil {
			// Need to reconnect
			if err := s.reconnect(reconnectDelay); err != nil {
				// Increase backoff
				reconnectDelay = reconnectDelay * 2
				if reconnectDelay > maxReconnectDelay {
					reconnectDelay = maxReconnectDelay
				}
				continue
			}
			// Reset backoff on successful reconnect
			reconnectDelay = 1 * time.Second
			continue
		}

		// Set read deadline to allow checking context
		conn.SetReadDeadline(time.Now().Add(1 * time.Second))

		msg, err := mux.DecodeMessage(conn)
		if err != nil {
			// Check for timeout errors (may be wrapped by DecodeMessage)
			// The error can come in several forms:
			// 1. Direct net.Error with Timeout()
			// 2. Wrapped in "failed to read message header: %w"
			// 3. os.ErrDeadlineExceeded
			// 4. io.ErrUnexpectedEOF when partial read before timeout
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				// Timeout is expected, continue loop
				continue
			}
			// Check for os.ErrDeadlineExceeded which is common with wrapped timeouts
			if errors.Is(err, os.ErrDeadlineExceeded) {
				continue
			}
			// Check for io.ErrUnexpectedEOF which happens when partial data read before timeout
			// This is NOT a real connection close - it's a timeout during io.ReadFull
			if errors.Is(err, io.ErrUnexpectedEOF) {
				continue
			}
			// Check error message for timeout indicators (fallback for wrapped errors)
			errStr := err.Error()
			if strings.Contains(errStr, "i/o timeout") || strings.Contains(errStr, "deadline exceeded") {
				continue
			}

			if err == io.EOF || errors.Is(err, io.EOF) {
				log.InfoLog.Printf("Mux connection closed (EOF): %s", s.socketPath)
			} else {
				log.WarningLog.Printf("Error reading from mux (unhandled error type %T): %v", err, err)
			}

			// Mark as disconnected
			s.connMu.Lock()
			if s.conn != nil {
				s.conn.Close()
				s.conn = nil
			}
			s.connected = false
			s.lastError = err
			s.connMu.Unlock()

			continue
		}

		// Handle message
		switch msg.Type {
		case mux.MessageTypeOutput:
			// Buffer the output
			s.buffer.Write(msg.Data)

			// Broadcast to consumers
			s.broadcast(msg.Data)

		case mux.MessageTypePing:
			// Respond with pong
			mux.WriteMessage(conn, mux.NewPongMessage())

		case mux.MessageTypeSnapshotReply:
			// Send snapshot response to waiting GetSnapshot() caller
			select {
			case s.snapshotResp <- msg.Data:
			default:
				// No one waiting for snapshot, discard
			}

		case mux.MessageTypeClose:
			// Server is closing
			log.InfoLog.Printf("Mux server closing connection: %s", s.socketPath)
			s.connMu.Lock()
			if s.conn != nil {
				s.conn.Close()
				s.conn = nil
			}
			s.connected = false
			s.connMu.Unlock()
		}
	}
}

// reconnect attempts to reconnect to the mux socket.
func (s *ExternalStreamer) reconnect(delay time.Duration) error {
	select {
	case <-s.ctx.Done():
		return s.ctx.Err()
	case <-time.After(delay):
	}

	s.reconnects++
	log.InfoLog.Printf("Attempting reconnect to %s (attempt %d)", s.socketPath, s.reconnects)

	if err := s.connect(); err != nil {
		log.WarningLog.Printf("Reconnect failed: %v", err)
		return err
	}

	return nil
}

// broadcast sends data to all registered consumers.
func (s *ExternalStreamer) broadcast(data []byte) {
	s.consumersMu.RLock()
	consumers := make([]OutputConsumer, len(s.consumers))
	copy(consumers, s.consumers)
	s.consumersMu.RUnlock()

	for _, consumer := range consumers {
		// Call consumer in goroutine to prevent blocking
		go func(c OutputConsumer) {
			defer func() {
				if r := recover(); r != nil {
					log.WarningLog.Printf("Consumer panic: %v", r)
				}
			}()
			c(data)
		}(consumer)
	}
}

// ExternalStreamerManager manages multiple external streamers.
type ExternalStreamerManager struct {
	streamers   map[string]*ExternalStreamer
	streamersMu sync.RWMutex
	bufferSize  int
}

// NewExternalStreamerManager creates a new streamer manager.
func NewExternalStreamerManager(bufferSize int) *ExternalStreamerManager {
	return &ExternalStreamerManager{
		streamers:  make(map[string]*ExternalStreamer),
		bufferSize: bufferSize,
	}
}

// GetOrCreate returns an existing streamer or creates a new one.
func (m *ExternalStreamerManager) GetOrCreate(socketPath string) (*ExternalStreamer, error) {
	m.streamersMu.Lock()
	defer m.streamersMu.Unlock()

	if streamer, exists := m.streamers[socketPath]; exists {
		return streamer, nil
	}

	streamer := NewExternalStreamer(socketPath, m.bufferSize)
	if err := streamer.Start(); err != nil {
		return nil, err
	}

	m.streamers[socketPath] = streamer
	return streamer, nil
}

// Get returns a streamer if it exists.
func (m *ExternalStreamerManager) Get(socketPath string) *ExternalStreamer {
	m.streamersMu.RLock()
	defer m.streamersMu.RUnlock()
	return m.streamers[socketPath]
}

// Remove stops and removes a streamer.
func (m *ExternalStreamerManager) Remove(socketPath string) {
	m.streamersMu.Lock()
	defer m.streamersMu.Unlock()

	if streamer, exists := m.streamers[socketPath]; exists {
		streamer.Stop()
		delete(m.streamers, socketPath)
	}
}

// StopAll stops all streamers.
func (m *ExternalStreamerManager) StopAll() {
	m.streamersMu.Lock()
	defer m.streamersMu.Unlock()

	for _, streamer := range m.streamers {
		streamer.Stop()
	}
	m.streamers = make(map[string]*ExternalStreamer)
}

// Count returns the number of active streamers.
func (m *ExternalStreamerManager) Count() int {
	m.streamersMu.RLock()
	defer m.streamersMu.RUnlock()
	return len(m.streamers)
}
