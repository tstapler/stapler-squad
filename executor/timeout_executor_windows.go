//go:build windows

package executor

// isTimeoutKill reports whether err reflects a process killed on context
// expiry. Windows terminates processes via TerminateProcess, not POSIX
// signals, so syscall.WaitStatus carries no signal information there
// (Signaled() is a hardcoded stub that always returns false) — there is no
// way to distinguish "killed by us" from "exited on its own" from err alone.
// Callers already gate on ctx.Err() != nil before calling this, so always
// returning true here preserves that check as the sole signal on Windows,
// same as before this file split existed.
func isTimeoutKill(_ error) bool {
	return true
}
