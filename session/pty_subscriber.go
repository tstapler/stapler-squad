//go:build !windows

package session

import (
	"errors"
	"sync"
)

// ErrSubscriberFull is returned by PTYSubscriber.Push when the internal buffer
// has exceeded its capacity limit. The caller should close the subscriber.
var ErrSubscriberFull = errors.New("PTYSubscriber: internal buffer exceeded capacity limit")

// PTYSubscriber is a lossless, ordered buffer for raw PTY bytes from a single session.
// fanOut calls Push; the consumer reads from Chan.
// Implementations must be goroutine-safe.
//
// The interface is intentionally minimal so that alternative backends (e.g. a
// memory-mapped circular file for large or persistent sessions) can be substituted
// without changing callers.
type PTYSubscriber interface {
	// Push appends data to the buffer. Must be goroutine-safe and never block.
	// Returns ErrSubscriberFull if the buffer is at capacity; the caller should
	// then close the subscriber and force the consumer to reconnect.
	Push(data []byte) error
	// Chan returns the receive-only channel from which the consumer reads buffered
	// data. The channel is closed when Close is called and all queued data is drained.
	Chan() <-chan []byte
	// Close signals that no more data will be pushed and releases resources.
	Close()
}

// memPTYSubscriber is an in-memory PTYSubscriber backed by an intermediate
// buffering channel and a drain goroutine. It never drops data unless the buffer
// reaches maxPushBufEntries pending chunks, at which point Push returns
// ErrSubscriberFull.
//
// The ch field is intentionally accessible within the package so that
// SubscribeToControlModeUpdates can satisfy the ProcessManager interface which
// requires a bidirectional chan []byte.
type memPTYSubscriber struct {
	pushCh chan []byte // internal buffer; fanOut writes here
	ch     chan []byte // consumer-facing; drain goroutine writes here
	stopCh chan struct{}
	once   sync.Once
}

// maxPushBufEntries caps the number of in-flight chunks before the subscriber
// is considered full (~4 MB at 4096 bytes per chunk).
const maxPushBufEntries = 1024

// newMemPTYSubscriber creates and starts a memPTYSubscriber.
// Call Close() when the subscription ends.
func newMemPTYSubscriber() *memPTYSubscriber {
	s := &memPTYSubscriber{
		pushCh: make(chan []byte, maxPushBufEntries),
		ch:     make(chan []byte, 64),
		stopCh: make(chan struct{}),
	}
	go s.drain()
	return s
}

// Push copies data into the internal buffer for delivery to the consumer.
// Returns ErrSubscriberFull if pushCh is full.
func (s *memPTYSubscriber) Push(data []byte) error {
	cp := make([]byte, len(data))
	copy(cp, data)
	select {
	case s.pushCh <- cp:
		return nil
	default:
		return ErrSubscriberFull
	}
}

// Chan returns the receive-only consumer channel.
func (s *memPTYSubscriber) Chan() <-chan []byte {
	return s.ch
}

// Close signals the drain goroutine to exit after draining queued data.
func (s *memPTYSubscriber) Close() {
	s.once.Do(func() { close(s.stopCh) })
}

// drain coalesces available chunks from pushCh and forwards them to ch.
// Multiple small chunks are merged into a single send to reduce channel overhead.
// Exits when stopCh is closed.
func (s *memPTYSubscriber) drain() {
	defer close(s.ch)
	for {
		var chunk []byte
		select {
		case chunk = <-s.pushCh:
		case <-s.stopCh:
			return
		}
		// Coalesce any immediately available chunks to reduce round-trips.
		for coalescing := true; coalescing; {
			select {
			case more := <-s.pushCh:
				chunk = append(chunk, more...)
			default:
				coalescing = false
			}
		}
		// Forward to the consumer; abort on close.
		select {
		case s.ch <- chunk:
		case <-s.stopCh:
			return
		}
	}
}
