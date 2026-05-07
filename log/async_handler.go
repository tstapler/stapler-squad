package log

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
)

const defaultAsyncBufSize = 8192

type asyncWork struct {
	ctx    context.Context
	record slog.Record
}

type asyncState struct {
	ch      chan asyncWork
	wg      sync.WaitGroup
	dropped atomic.Int64
}

// AsyncHandler wraps a slog.Handler with a channel buffer. Log calls enqueue a
// cloned Record and return immediately; a background goroutine drains the channel.
// On full buffer the record is dropped and the drop counter increments.
// WithAttrs and WithGroup share the same underlying channel so a single goroutine
// drains all derived loggers.
type AsyncHandler struct {
	next   slog.Handler
	shared *asyncState
}

// NewAsyncHandler wraps next with an async channel of bufSize capacity.
func NewAsyncHandler(next slog.Handler, bufSize int) *AsyncHandler {
	return &AsyncHandler{
		next: next,
		shared: &asyncState{
			ch: make(chan asyncWork, bufSize),
		},
	}
}

// StartDrain launches the background drain goroutine. Must be called once before
// the handler is used. Call Flush to stop it and drain remaining work.
func (h *AsyncHandler) StartDrain() {
	h.shared.wg.Add(1)
	go func() {
		defer h.shared.wg.Done()
		for work := range h.shared.ch {
			_ = h.next.Handle(work.ctx, work.record)
		}
	}()
}

// Flush closes the channel and waits for all enqueued records to be written.
// After Flush the handler must not be used.
func (h *AsyncHandler) Flush(_ context.Context) error {
	close(h.shared.ch)
	h.shared.wg.Wait()
	return nil
}

// Handle enqueues the record for async writing. Drops and counts if buffer full.
func (h *AsyncHandler) Handle(ctx context.Context, r slog.Record) error {
	clone := r.Clone()
	select {
	case h.shared.ch <- asyncWork{ctx: ctx, record: clone}:
	default:
		h.shared.dropped.Add(1)
	}
	return nil
}

// Enabled delegates to the underlying handler.
func (h *AsyncHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

// WithAttrs returns a new AsyncHandler whose next handler has the given attrs,
// sharing the same channel so one drain goroutine serves all derived loggers.
func (h *AsyncHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &AsyncHandler{next: h.next.WithAttrs(attrs), shared: h.shared}
}

// WithGroup returns a new AsyncHandler with a grouped next handler, sharing the channel.
func (h *AsyncHandler) WithGroup(name string) slog.Handler {
	return &AsyncHandler{next: h.next.WithGroup(name), shared: h.shared}
}

// Dropped returns the number of records dropped due to a full buffer.
func (h *AsyncHandler) Dropped() int64 {
	return h.shared.dropped.Load()
}
