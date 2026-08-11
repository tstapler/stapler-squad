package log

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"
)

// countingHandler counts how many records were handled.
type countingHandler struct {
	mu    sync.Mutex
	count int
}

func (h *countingHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }
func (h *countingHandler) Handle(_ context.Context, _ slog.Record) error {
	h.mu.Lock()
	h.count++
	h.mu.Unlock()
	return nil
}
func (h *countingHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *countingHandler) WithGroup(_ string) slog.Handler      { return h }
func (h *countingHandler) Count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.count
}

func TestAsyncHandler_DeliversRecords(t *testing.T) {
	inner := &countingHandler{}
	h := NewAsyncHandler(inner, defaultAsyncBufSize)
	h.StartDrain()

	ctx := context.Background()
	const n = 50
	for i := 0; i < n; i++ {
		if err := h.Handle(ctx, slog.NewRecord(time.Now(), slog.LevelInfo, "msg", 0)); err != nil {
			t.Fatalf("Handle: %v", err)
		}
	}

	if err := h.Flush(ctx); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	if inner.Count() != n {
		t.Errorf("expected %d records delivered, got %d", n, inner.Count())
	}
}

// TestAsyncHandler_NoPanicOnConcurrentFlush is the regression test for the
// "send on closed channel" panic that occurred when control-mode goroutines
// called log.Debug() concurrently with Flush() during service restart.
func TestAsyncHandler_NoPanicOnConcurrentFlush(t *testing.T) {
	inner := &countingHandler{}
	h := NewAsyncHandler(inner, 64)
	h.StartDrain()

	ctx := context.Background()
	var wg sync.WaitGroup

	// Hammer Handle() from many goroutines while Flush() is called concurrently.
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				// Must not panic regardless of whether Flush has been called.
				_ = h.Handle(ctx, slog.NewRecord(time.Now(), slog.LevelDebug, "msg", 0))
			}
		}()
	}

	// Flush races with the goroutines above — this is the exact scenario from
	// the bug: shutdown closes the channel while writers are still active.
	if err := h.Flush(ctx); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	wg.Wait()
	// No panic == pass. Dropped counter may be non-zero; that is expected.
}

func TestAsyncHandler_DropsAfterFlush(t *testing.T) {
	inner := &countingHandler{}
	h := NewAsyncHandler(inner, defaultAsyncBufSize)
	h.StartDrain()

	ctx := context.Background()
	if err := h.Flush(ctx); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	// All writes after Flush must drop, not panic.
	for i := 0; i < 10; i++ {
		_ = h.Handle(ctx, slog.NewRecord(time.Now(), slog.LevelInfo, "post-flush", 0))
	}
	if h.Dropped() == 0 {
		t.Error("expected non-zero dropped count after Flush")
	}
}

func TestAsyncHandler_DropsWhenFull(t *testing.T) {
	// Block the drain so the queue fills immediately.
	blocker := make(chan struct{})
	inner := &blockingHandler{block: blocker}

	h := NewAsyncHandler(inner, 4)
	h.StartDrain()

	ctx := context.Background()
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 100; i++ {
			_ = h.Handle(ctx, slog.NewRecord(time.Now(), slog.LevelInfo, "x", 0))
		}
	}()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Handle blocked the caller; must be non-blocking")
	}

	close(blocker)
	if err := h.Flush(ctx); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	if h.Dropped() == 0 {
		t.Error("expected drops when queue overflows")
	}
}

func TestAsyncHandler_WithAttrsSharesChannel(t *testing.T) {
	inner := &countingHandler{}
	h := NewAsyncHandler(inner, defaultAsyncBufSize)
	h.StartDrain()

	derived := h.WithAttrs([]slog.Attr{slog.String("k", "v")})
	ctx := context.Background()
	_ = derived.Handle(ctx, slog.NewRecord(time.Now(), slog.LevelInfo, "via derived", 0))

	// Flush the root — derived shares the same channel so all records drain.
	if err := h.Flush(ctx); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	if inner.Count() != 1 {
		t.Errorf("expected 1 record via derived handler, got %d", inner.Count())
	}
}

// blockingHandler blocks every Handle call until the block channel is closed.
type blockingHandler struct {
	block <-chan struct{}
}

func (h *blockingHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }
func (h *blockingHandler) Handle(_ context.Context, _ slog.Record) error {
	<-h.block
	return nil
}
func (h *blockingHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *blockingHandler) WithGroup(_ string) slog.Handler      { return h }
