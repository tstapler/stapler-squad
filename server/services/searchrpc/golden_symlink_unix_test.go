//go:build !windows

package searchrpc

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

// setLinkMtime sets a symlink's own mtime (not its target's).
func setLinkMtime(t *testing.T, path string, mt time.Time) {
	t.Helper()
	ts := unix.NsecToTimespec(mt.UnixNano())
	require.NoError(t, unix.UtimesNanoAt(unix.AT_FDCWD, path, []unix.Timespec{ts, ts}, unix.AT_SYMLINK_NOFOLLOW))
}
