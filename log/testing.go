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

// redirectMu serializes every RedirectLogger call across the whole process.
// RedirectLogger mutates the given *log.Logger in place rather than swapping
// in a new instance, so two callers redirecting the SAME logger concurrently
// (e.g. two t.Parallel() tests) would otherwise race over whose SyncBuffer is
// "current" — one test's Printf output could land in another test's buffer.
// Holding this lock for the full redirect-to-restore window, mirrored from
// session/sync_buffer_test.go's warningLogMu, prevents that.
var redirectMu sync.Mutex //nolint:gochecknoglobals

// RedirectLogger redirects logger's output to a fresh SyncBuffer for the
// duration of the calling test, restoring the logger's original
// output/prefix/flags via t.Cleanup. It mutates the existing *log.Logger in
// place via its own thread-safe SetOutput/SetPrefix/SetFlags methods rather
// than reassigning a package-level logger variable, so any caller that
// already holds a reference to logger (obtained via an accessor like
// ErrorLog() before this call) keeps writing to the right place. The
// returned buffer is safe to read concurrently with writes still landing on
// it.
func RedirectLogger(t *testing.T, logger *stdlog.Logger, prefix string) *SyncBuffer {
	t.Helper()
	redirectMu.Lock()
	buf := &SyncBuffer{}
	origOutput := logger.Writer()
	origPrefix := logger.Prefix()
	origFlags := logger.Flags()
	logger.SetOutput(buf)
	logger.SetPrefix(prefix)
	logger.SetFlags(0)
	t.Cleanup(func() {
		logger.SetOutput(origOutput)
		logger.SetPrefix(origPrefix)
		logger.SetFlags(origFlags)
		redirectMu.Unlock()
	})
	return buf
}
