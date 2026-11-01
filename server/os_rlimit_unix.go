//go:build !windows

package server

import "golang.org/x/sys/unix"

// openFDLimit is the soft RLIMIT_NOFILE, clamped so it always fits an int64.
func openFDLimit() (int64, bool) {
	var rl unix.Rlimit
	if unix.Getrlimit(unix.RLIMIT_NOFILE, &rl) != nil {
		return 0, false
	}
	return int64(min(rl.Cur, uint64(1)<<62)), true // #nosec G115 -- clamped
}
