package log

import (
	"bytes"
	"io"
	stdlog "log"
	"sync"
	"sync/atomic"
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
	redirectLocksMu sync.Mutex                           //nolint:gochecknoglobals
	redirectLocks   = map[*stdlog.Logger]*sync.RWMutex{} //nolint:gochecknoglobals
)

func redirectLockFor(logger *stdlog.Logger) *sync.RWMutex {
	redirectLocksMu.Lock()
	defer redirectLocksMu.Unlock()
	mu, ok := redirectLocks[logger]
	if !ok {
		mu = &sync.RWMutex{}
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

// QuietLogger keeps logger's output from being redirected for the rest of the
// calling test, without capturing anything itself. Use it in a test that
// writes to logger but never asserts on it. Any number of quiet tests and
// CaptureLogger tests run concurrently, so a quiet test's writes DO land in
// concurrent capturers' buffers (which is why capturers must assert only on text
// unique to themselves); only RedirectLogger callers wait for quiet tests.
// Never take a second QuietLogger/CaptureLogger/RedirectLogger on the same
// logger in one test (including parallel subtests of it): the read lock is held
// until cleanup, and a pending RedirectLogger writer would deadlock the second
// acquisition.
func QuietLogger(t *testing.T, logger *stdlog.Logger) {
	t.Helper()
	mu := redirectLockFor(logger)
	mu.RLock()
	t.Cleanup(mu.RUnlock)
}

// logFanout is installed as a logger's output while at least one CaptureLogger
// subscriber exists. Write copies each line to every subscriber without taking
// a lock (subscribers live in a copy-on-write slice behind an atomic pointer),
// or to the original writer when nobody is subscribed.
type logFanout struct {
	orig io.Writer
	subs atomic.Pointer[[]*SyncBuffer]
}

func (f *logFanout) Write(p []byte) (int, error) {
	subs := f.subs.Load()
	if subs == nil || len(*subs) == 0 {
		return f.orig.Write(p)
	}
	for _, b := range *subs {
		_, _ = b.Write(p)
	}
	return len(p), nil
}

// update swaps in a new subscriber slice; callers hold redirectLocksMu, which
// serializes writers so the load-modify-store below cannot lose an update.
func (f *logFanout) update(modify func([]*SyncBuffer) []*SyncBuffer) {
	var cur []*SyncBuffer
	if p := f.subs.Load(); p != nil {
		cur = *p
	}
	next := modify(append([]*SyncBuffer(nil), cur...))
	f.subs.Store(&next)
}

type fanoutState struct {
	fanout     *logFanout
	refs       int
	origOut    io.Writer
	origPrefix string
	origFlags  int
}

var fanouts = map[*stdlog.Logger]*fanoutState{} //nolint:gochecknoglobals // guarded by redirectLocksMu

// CaptureLogger gives the calling test its own buffer of logger's output
// without excluding other tests: every concurrent capturer receives every line
// written to logger while it is subscribed (an aggregated stream, not an
// isolated one), so assertions must match on something unique to the test
// (an item ID, a title) rather than on the buffer being empty or exact. Use
// RedirectLogger instead when a test needs isolation. Capturers and quiet tests
// share the logger's read lock; only RedirectLogger callers wait for them. All
// capturers of one logger must pass the same prefix — the first one wins.
func CaptureLogger(t *testing.T, logger *stdlog.Logger, prefix string) *SyncBuffer {
	t.Helper()
	rw := redirectLockFor(logger)
	rw.RLock()
	buf := &SyncBuffer{}
	redirectLocksMu.Lock()
	st := fanouts[logger]
	if st == nil {
		st = &fanoutState{}
		fanouts[logger] = st
	}
	if st.refs == 0 {
		st.origOut, st.origPrefix, st.origFlags = logger.Writer(), logger.Prefix(), logger.Flags()
		st.fanout = &logFanout{orig: st.origOut}
		logger.SetOutput(st.fanout)
		logger.SetPrefix(prefix)
		logger.SetFlags(0)
	}
	st.refs++
	st.fanout.update(func(s []*SyncBuffer) []*SyncBuffer { return append(s, buf) })
	redirectLocksMu.Unlock()
	t.Cleanup(func() {
		redirectLocksMu.Lock()
		st.fanout.update(func(s []*SyncBuffer) []*SyncBuffer {
			for i, b := range s {
				if b == buf {
					return append(s[:i], s[i+1:]...)
				}
			}
			return s
		})
		st.refs--
		if st.refs == 0 {
			logger.SetOutput(st.origOut)
			logger.SetPrefix(st.origPrefix)
			logger.SetFlags(st.origFlags)
		}
		redirectLocksMu.Unlock()
		rw.RUnlock()
	})
	return buf
}
