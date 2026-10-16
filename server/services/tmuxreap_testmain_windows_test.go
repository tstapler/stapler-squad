//go:build windows

package services

// testutil/tmuxreap is unix-only (it shells out to real tmux/pgrep-style
// process listing); on Windows this package's tests don't spawn real tmux
// servers via that mechanism, so these are no-ops.
func reapLeakedTmuxTestServers() {}

func startTmuxTestServerWatchdog() {}
