//go:build !windows

package executor

import (
	"errors"
	"os/exec"
	"syscall"
)

// isTimeoutKill reports whether err reflects a process actually killed by
// exec.CommandContext's SIGKILL on context expiry, as opposed to a command
// that simply exited (with any status) before the context expired.
//
// Checking ctx.Err() alone after the command returns is racy: under host
// load, the calling goroutine can be descheduled between the command
// finishing and the ctx.Err() check, letting the timeout elapse in that gap
// even though the command completed on its own. That misclassified a fast,
// non-zero-exit command (e.g. "sh -c exit 1") as "timed out" under full test
// suite load. Requiring the process to have actually been killed by SIGKILL
// makes the classification depend on what happened to the process, not on
// how much wall-clock time has passed by the time we happen to check.
func isTimeoutKill(err error) bool {
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return false
	}
	ws, ok := exitErr.Sys().(syscall.WaitStatus)
	return ok && ws.Signaled() && ws.Signal() == syscall.SIGKILL
}
