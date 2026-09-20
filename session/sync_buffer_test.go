package session

import (
	"testing"

	"github.com/tstapler/stapler-squad/log"
)

// swapWarningLog redirects log.WarningLog's output to a buffer for the
// duration of the calling test, restoring the original on cleanup.
func swapWarningLog(t *testing.T) *log.SyncBuffer {
	t.Helper()
	return log.RedirectLogger(t, log.WarningLog(), "WARNING: ")
}
