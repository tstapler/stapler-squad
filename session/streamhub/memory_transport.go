package streamhub

import "sync"

// MemoryTransport is an in-process Transport implementation (Story 1.4.1):
// it lets hub tests exercise attach/broadcast/eviction behavior without a
// real tmux process or network socket, the required Testability NFR. It
// records every frame it receives and can be configured, via With* options,
// to block or error on Send so tests can deterministically drive the
// slow-subscriber and Transport.Send-error eviction paths (Story 1.2.1,
// Story 1.4.2) instead of relying on real network timing.
type MemoryTransport struct {
	mu       sync.Mutex
	frames   [][]byte
	closed   bool
	unblock  chan struct{}
	errSend  error
	blocking bool
}

// MemoryTransportOption configures a MemoryTransport at construction time.
type MemoryTransportOption func(*MemoryTransport)

// WithBlockingSend makes every Send call block until Unblock is called (or
// the transport is closed), simulating a stalled writer — e.g. a dead
// network connection whose write never returns — so tests can deterministically
// exercise the slow-subscriber eviction path.
func WithBlockingSend() MemoryTransportOption {
	return func(m *MemoryTransport) { m.blocking = true }
}

// WithErrorSend makes every Send call return err instead of recording the
// frame, so tests can exercise the Transport.Send-error eviction path.
func WithErrorSend(err error) MemoryTransportOption {
	return func(m *MemoryTransport) { m.errSend = err }
}

// NewMemoryTransport returns a MemoryTransport configured by opts. With no
// options, Send always succeeds immediately and records the frame.
func NewMemoryTransport(opts ...MemoryTransportOption) *MemoryTransport {
	m := &MemoryTransport{unblock: make(chan struct{})}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// Send implements Transport. Behavior depends on how the transport was
// constructed: WithErrorSend returns its configured error immediately;
// WithBlockingSend blocks until Unblock or Close is called; otherwise the
// frame is recorded and Send returns nil.
func (m *MemoryTransport) Send(data []byte) error {
	m.mu.Lock()
	if m.errSend != nil {
		err := m.errSend
		m.mu.Unlock()
		return err
	}
	blocking := m.blocking
	unblock := m.unblock
	m.mu.Unlock()

	if blocking {
		<-unblock
		return nil
	}

	m.mu.Lock()
	m.frames = append(m.frames, append([]byte(nil), data...))
	m.mu.Unlock()
	return nil
}

// Unblock releases any Send call currently blocked by WithBlockingSend. It
// is safe to call even if no Send is currently blocked, and safe to call
// more than once.
func (m *MemoryTransport) Unblock() {
	m.mu.Lock()
	defer m.mu.Unlock()
	select {
	case <-m.unblock:
		// Already unblocked.
	default:
		close(m.unblock)
	}
}

// Close implements Transport. It releases any Send call blocked by
// WithBlockingSend (treated the same as an explicit Unblock) and is safe to
// call more than once.
func (m *MemoryTransport) Close() error {
	m.Unblock()
	m.mu.Lock()
	m.closed = true
	m.mu.Unlock()
	return nil
}

// ReceivedFrames returns a copy of every frame successfully delivered via
// Send so far, in delivery order.
func (m *MemoryTransport) ReceivedFrames() [][]byte {
	m.mu.Lock()
	defer m.mu.Unlock()
	frames := make([][]byte, len(m.frames))
	copy(frames, m.frames)
	return frames
}

// IsClosed reports whether Close has been called.
func (m *MemoryTransport) IsClosed() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.closed
}
