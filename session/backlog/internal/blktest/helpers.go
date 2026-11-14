package blktest

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/instance"
	"github.com/tstapler/stapler-squad/session/internal/sessiontest"
)

// CreateTestStorage returns a Storage over a fresh in-memory test repository; the cleanup func is a no-op kept so
// call sites mirror session's own helper of the same name (NewTestEntRepository registers its own t.Cleanup).
func CreateTestStorage(t *testing.T) (*session.Storage, func()) {
	t.Helper()
	storage, err := session.NewStorageWithRepository(session.NewTestEntRepository(t))
	require.NoError(t, err)
	return storage, func() {}
}

// NewTestInstance returns a minimal Paused Instance ready for Storage.AddInstance.
func NewTestInstance(title string) *instance.Instance { return instance.NewPausedTestInstance(title) }

// QuietWarningLog is for tests that may write to log.WarningLog but never assert on it.
func QuietWarningLog(t *testing.T) {
	t.Helper()
	log.QuietLogger(t, log.WarningLog())
}

// CaptureWarningLog returns this test's own view of log.WarningLog: assert on text unique
// to the test, never on the buffer being empty.
func CaptureWarningLog(t *testing.T) *log.SyncBuffer {
	t.Helper()
	return log.CaptureLogger(t, log.WarningLog(), "WARNING: ")
}

// RedirectInfoLog redirects log.InfoLog's output to a returned buffer for the duration of the test.
func RedirectInfoLog(t *testing.T) *log.SyncBuffer {
	t.Helper()
	return log.RedirectLogger(t, log.InfoLog(), "INFO: ")
}

// RunGitCapture runs a git command in dir and returns its combined output, failing the test on error.
func RunGitCapture(t *testing.T, dir string, args ...string) string {
	t.Helper()
	return sessiontest.RunGitOutputOrFail(t, dir, args...)
}
