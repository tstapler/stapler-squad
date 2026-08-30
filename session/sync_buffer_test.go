package session

import (
	"bytes"
	"sync"
	"testing"

	"github.com/tstapler/stapler-squad/log"
)

// warningLogMu serializes all swapWarningLog sessions package-wide. Every
// caller redirects the SAME shared log.WarningLog logger at its own private
// buffer; with multiple t.Parallel() tests/subtests doing this concurrently,
// only one buffer can be the logger's "current" output at any instant, so
// one test's Printf can land in a different (concurrently-read) test's
// buffer — a genuine data race on that bytes.Buffer, not fixable by making
// the individual SetOutput/Printf calls atomic. Holding this mutex for the
// full swap-to-restore window ensures only one test owns the redirection at
// a time, regardless of how many are marked parallel.
var warningLogMu sync.Mutex

// syncBuffer wraps bytes.Buffer with its own mutex so a test's later
// buf.String() read is synchronized against writes. warningLogMu only
// serializes tests that themselves call swapWarningLog — it does nothing for
// a goroutine leaked by an unrelated, already-finished test that still holds
// a reference to log.WarningLog and calls Printf on it later. *log.Logger's
// Printf serializes that write against other Printf/SetOutput calls via its
// own internal mutex, but a raw bytes.Buffer.String() read never goes
// through that mutex — so without this wrapper, that read and the leaked
// goroutine's write race on the underlying byte slice even though each
// individually looks synchronized.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func (b *syncBuffer) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Len()
}

func (b *syncBuffer) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf.Reset()
}

// swapWarningLog redirects log.WarningLog's output to a buffer for the
// duration of the calling test, restoring the original on cleanup. It
// mutates the existing *log.Logger in place via its own mutex-protected
// SetOutput/SetPrefix/SetFlags rather than reassigning the log.WarningLog
// package variable — other tests in this codebase run in parallel
// (t.Parallel) and some spawn background goroutines that call
// log.WarningLog().Printf directly; reassigning the variable itself races with
// those concurrent reads even though *log.Logger's own methods are
// individually concurrency-safe. The returned buffer is a syncBuffer (see
// doc comment above), not a raw bytes.Buffer, so this test's later
// .String() read is synchronized against concurrent writes.
func swapWarningLog(t *testing.T) *syncBuffer {
	t.Helper()
	warningLogMu.Lock()
	buf := &syncBuffer{}
	logger := log.WarningLog()
	origOutput := logger.Writer()
	origPrefix := logger.Prefix()
	origFlags := logger.Flags()
	logger.SetOutput(buf)
	logger.SetPrefix("WARNING: ")
	logger.SetFlags(0)
	t.Cleanup(func() {
		logger.SetOutput(origOutput)
		logger.SetPrefix(origPrefix)
		logger.SetFlags(origFlags)
		warningLogMu.Unlock()
	})
	return buf
}
