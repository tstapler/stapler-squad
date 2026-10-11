package session

import (
	"testing"

	"github.com/tstapler/stapler-squad/log"
)

// quietWarningLog is for tests that may write to log.WarningLog but never
// assert on it: it only excludes capturing tests (see log.QuietLogger), so
// quiet tests no longer serialize against each other for their whole duration.
func quietWarningLog(t *testing.T) {
	t.Helper()
	log.QuietLogger(t, log.WarningLog())
}

// captureWarningLog returns this test's own view of log.WarningLog without
// serializing against other tests (see log.CaptureLogger): assert on text unique
// to the test, never on the buffer being empty.
func captureWarningLog(t *testing.T) *log.SyncBuffer {
	t.Helper()
	return log.CaptureLogger(t, log.WarningLog(), "WARNING: ")
}
