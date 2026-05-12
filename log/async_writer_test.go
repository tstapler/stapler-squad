package log

import (
	"bytes"
	"io"
	"sync"
	"testing"
	"time"
)

func TestAsyncWriter_DeliversAllEntries(t *testing.T) {
	var buf bytes.Buffer
	var mu sync.Mutex
	aw := newAsyncWriter(&safeWriter{buf: &buf, mu: &mu}, asyncWriterBufSize)

	const n = 100
	for i := 0; i < n; i++ {
		_, err := aw.Write([]byte("x"))
		if err != nil {
			t.Fatalf("Write returned error: %v", err)
		}
	}

	if err := aw.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	mu.Lock()
	got := buf.Len()
	mu.Unlock()

	if got != n {
		t.Errorf("expected %d bytes written, got %d (dropped=%d)", n, got, aw.Dropped())
	}
	if aw.Dropped() != 0 {
		t.Errorf("expected 0 drops, got %d", aw.Dropped())
	}
}

func TestAsyncWriter_NeverBlocksCallerWhenFull(t *testing.T) {
	// blockingWriter stalls until unblocked so the queue fills quickly.
	blocker := make(chan struct{})
	bw := &blockingWriter{block: blocker}

	aw := newAsyncWriter(bw, 4) // tiny queue

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 100; i++ {
			_, _ = aw.Write([]byte("x"))
		}
	}()

	select {
	case <-done:
		// good — caller returned without blocking
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Write blocked the caller; asyncWriter must never block")
	}

	close(blocker) // unblock drain goroutine
	_ = aw.Close()

	if aw.Dropped() == 0 {
		t.Error("expected some drops when queue overflows, got 0")
	}
}

func TestAsyncWriter_CloseIdempotent(t *testing.T) {
	aw := newAsyncWriter(io.Discard, asyncWriterBufSize)
	if err := aw.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := aw.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

// safeWriter wraps a bytes.Buffer with a mutex so the test's concurrent drain
// goroutine and the checking goroutine don't race.
type safeWriter struct {
	buf *bytes.Buffer
	mu  *sync.Mutex
}

func (sw *safeWriter) Write(p []byte) (int, error) {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	return sw.buf.Write(p)
}

// blockingWriter blocks on every Write call until the block channel is closed.
type blockingWriter struct {
	block <-chan struct{}
}

func (bw *blockingWriter) Write(p []byte) (int, error) {
	<-bw.block
	return len(p), nil
}
