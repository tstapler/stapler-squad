package log

import (
	"bytes"
	stdlog "log"
	"sync"
	"testing"
)

// SyncBuffer wraps bytes.Buffer with its own mutex so a test's later String()
// read is synchronized against concurrent writers. A raw bytes.Buffer given
// as a *log.Logger's output relies on the Logger's own mutex to serialize
// writes against each other, but that mutex never guards a direct
// buf.String() read from the test goroutine — so without this wrapper, a
// background goroutine (e.g. a reconciliation loop still running against a
// live server under test) calling Printf concurrently with the test's read
// races on the underlying byte slice even though each individual Printf
// looks safe. Mirrors session/sync_buffer_test.go's syncBuffer.
type SyncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

// Write implements io.Writer.
func (b *SyncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

// String returns the buffer's contents so far, synchronized against
// concurrent writers.
func (b *SyncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// Len returns the number of bytes written so far, synchronized against
// concurrent writers.
func (b *SyncBuffer) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Len()
}

// RedirectLogger swaps the given atomicLogger-backed package logger (obtained
// via get, e.g. ErrorLog) to a new *log.Logger writing into a SyncBuffer with
// the given prefix, restoring the original logger via set (e.g.
// SetErrorLogForTest) when t's cleanup runs.
//
// It replaces the logger wholesale via the atomic set/get pair rather than
// mutating the existing *log.Logger's output/prefix in place — the same
// swap-not-reassign approach SetErrorLogForTest already uses — so concurrent
// readers that call get() mid-test never observe a partially-updated logger.
func RedirectLogger(t *testing.T, get func() *stdlog.Logger, set func(*stdlog.Logger) *stdlog.Logger, prefix string) *SyncBuffer {
	t.Helper()
	buf := &SyncBuffer{}
	newLogger := stdlog.New(buf, prefix, get().Flags())
	orig := set(newLogger)
	t.Cleanup(func() { set(orig) })
	return buf
}
