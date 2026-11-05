package svctest

import (
	"bytes"
	"log/slog"
	"sync"
	"testing"

	ssqlog "github.com/tstapler/stapler-squad/log"
)

// slogDefaultMu serializes every test in this package that swaps the injectable
// log.SetSlogDefaultForTest seam (log/log.go). Swapping the seam is itself race-free
// (it's an atomic pointer), but two t.Parallel() tests both redirecting it still race
// semantically: one test's log lines would land in another test's capture buffer.
// Every swap site in this package (captureLogs, captureInfoLog/captureErrorLog in
// session_service_client_log_test.go, and the inline swaps in search_service_test.go
// and slack_notifier_test.go) must hold this lock for the full swap-to-restore window.
var SlogDefaultMu sync.Mutex

// syncLogBuffer is a mutex-guarded bytes.Buffer. Even after this package's tests stop
// calling slog.SetDefault() directly, captureLogs's buffer can still legitimately be
// written to by a background goroutine the test under exercise itself spawns (via
// log.Warn/Info) concurrently with the owning test's String() read. A plain
// bytes.Buffer would make that a genuine data race under -race; this makes the access
// itself safe regardless of which code path writes to it.
type SyncLogBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *SyncLogBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *SyncLogBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// captureLogs swaps the default slog logger for one that writes to a buffer at Debug level,
// restoring the previous logger via t.Cleanup. Returns the buffer to inspect after the call.
func CaptureLogs(t *testing.T) *SyncLogBuffer {
	t.Helper()
	SlogDefaultMu.Lock()
	buf := &SyncLogBuffer{}
	prev := ssqlog.SetSlogDefaultForTest(slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() {
		ssqlog.SetSlogDefaultForTest(prev)
		SlogDefaultMu.Unlock()
	})
	return buf
}
