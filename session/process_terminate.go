//go:build !windows

package session

import "syscall"

// terminateProcess sends SIGTERM to pid. Used by
// TmuxProcessManager.TerminateCachedPanePID to reap a process that may have
// been orphaned when its tmux session vanished (e.g. the tmux server itself
// was killed/restarted) before a replacement is launched in its place.
func terminateProcess(pid int32) error {
	return syscall.Kill(int(pid), syscall.SIGTERM)
}
