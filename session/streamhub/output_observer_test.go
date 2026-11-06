package streamhub

import (
	"bytes"
	"sync"
	"testing"
	"time"
)

func TestStreamHubOutputObserverRunsOncePerCoalescedUnit(t *testing.T) {
	var mu sync.Mutex
	var observed [][]byte
	h := NewStreamHub("observer-test", nil,
		WithBatchMaxWindow(time.Hour),
		WithOutputObserver(func(data []byte) {
			mu.Lock()
			defer mu.Unlock()
			observed = append(observed, append([]byte(nil), data...))
		}),
	)
	t.Cleanup(func() { _ = h.ForceTeardown() })

	h.OnRawOutput([]byte("a"))
	h.OnRawOutput([]byte("\x1b[31m"))
	h.OnRawOutput([]byte("b"))
	h.batchWindow.TryFlush()

	mu.Lock()
	defer mu.Unlock()
	if len(observed) != 1 {
		t.Fatalf("observer calls = %d, want 1", len(observed))
	}
	if want := []byte("a\x1b[31mb"); !bytes.Equal(observed[0], want) {
		t.Fatalf("observer data = %q, want %q", observed[0], want)
	}
}
