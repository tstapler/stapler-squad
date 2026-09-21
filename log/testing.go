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

// Reset clears the buffer's contents, synchronized against concurrent
// writers.
func (b *SyncBuffer) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf.Reset()
}

// redirectLocks holds one *sync.Mutex per *log.Logger ever passed to
// RedirectLogger, guarded by redirectLocksMu. Locking is scoped per-logger
// rather than process-wide: two callers redirecting the SAME logger
// concurrently (e.g. two t.Parallel() tests) would otherwise race over whose
// SyncBuffer is "current" — one test's Printf output could land in another
// test's buffer — so that case still serializes, mirrored from
// session/sync_buffer_test.go's warningLogMu. But a single process-wide lock
// would also serialize two callers redirecting two DIFFERENT loggers for no
// reason, and would deadlock a test that redirects two different loggers in
// the same t.Run (the first redirect's unlock only happens in t.Cleanup,
// which fires after the test function returns — the second redirect's Lock
// call would block forever). Per-logger scoping avoids both.
var (
	redirectLocksMu sync.Mutex                         //nolint:gochecknoglobals
	redirectLocks   = map[*stdlog.Logger]*sync.Mutex{} //nolint:gochecknoglobals
)

func redirectLockFor(logger *stdlog.Logger) *sync.Mutex {
	redirectLocksMu.Lock()
	defer redirectLocksMu.Unlock()
	mu, ok := redirectLocks[logger]
	if !ok {
		mu = &sync.Mutex{}
		redirectLocks[logger] = mu
	}
	return mu
}

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
	mu := redirectLockFor(logger)
	mu.Lock()
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
		mu.Unlock()
	})
	return buf
}
