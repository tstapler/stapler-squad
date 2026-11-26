package driver

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/tmux"
)

// The paneactivity gate's staleness bound is built from the TTL the driver passes in; a wrong
// or zero value silently shrinks it. The leaf's tests mirror the TTL as 1s.
func TestDriverPaneGate_WiresProcessManagerCaptureTTL(t *testing.T) {
	t.Parallel()
	require.Equal(t, time.Second, tmux.CapturePaneCacheTTL, "paneactivity's tests mirror this value")

	src, err := os.ReadFile("session_driver.go")
	if os.IsNotExist(err) {
		t.Skip("source not in the Bazel sandbox runfiles; the native go test run enforces this")
	}
	require.NoError(t, err)
	require.Contains(t, string(src), "paneactivity.NewDriverPreview(inst, tmux.CapturePaneCacheTTL)")
}
