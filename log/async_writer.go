package log

import (
	"io"
	"sync"
	"sync/atomic"
)

const asyncWriterBufSize = 4096

// asyncWriter wraps an io.Writer with a buffered channel so callers never block on I/O.
// A single drain goroutine writes entries in arrival order — no mutex is needed on the
// write path. stdlib log.Logger's internal mutex is held only for message formatting and
// a non-blocking channel send (O(1)), not for the underlying file/console write.
//
// On a full buffer the entry is dropped and the Dropped counter increments.
type asyncWriter struct {
	out     io.Writer
	queue   chan []byte
	once    sync.Once
	wg      sync.WaitGroup
	dropped atomic.Int64
}

// newAsyncWriter creates an asyncWriter wrapping out with a channel of the given capacity
// and starts the single drain goroutine immediately.
func newAsyncWriter(out io.Writer, bufSize int) *asyncWriter {
	if bufSize <= 0 {
		bufSize = asyncWriterBufSize
	}
	aw := &asyncWriter{
		out:   out,
		queue: make(chan []byte, bufSize),
	}
	aw.wg.Add(1)
	go func() {
		defer aw.wg.Done()
		for b := range aw.queue {
			_, _ = aw.out.Write(b)
		}
	}()
	return aw
}

// Write copies p and enqueues it for async writing. It never blocks: on a full queue
// the entry is dropped and Dropped increments. Always returns (len(p), nil).
func (aw *asyncWriter) Write(p []byte) (int, error) {
	b := make([]byte, len(p))
	copy(b, p)
	select {
	case aw.queue <- b:
	default:
		aw.dropped.Add(1)
	}
	return len(p), nil
}

// Dropped returns the number of log entries dropped because the queue was full.
func (aw *asyncWriter) Dropped() int64 {
	return aw.dropped.Load()
}

// Close drains all pending entries and stops the drain goroutine.
// After Close the writer must not be used.
func (aw *asyncWriter) Close() error {
	aw.once.Do(func() {
		close(aw.queue)
		aw.wg.Wait()
	})
	return nil
}
