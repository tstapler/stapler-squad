//go:build darwin

package executor

import "syscall"

// buildSysProcAttr constructs a SysProcAttr from processConfig on macOS.
//
// On macOS, Noctty requires the parent process to have a controlling terminal
// to detach from. Since test processes and background services may have no
// controlling terminal, we skip Noctty on macOS to avoid "operation not
// supported by device" errors. Instead, Setsid provides equivalent session
// isolation when callers use WithNewSession().
//
// Setpgid places the child in a new process group (default unless noProcGroup).
// Setsid creates a new session (strongest isolation; safe on all platforms).
func buildSysProcAttr(cfg processConfig) *syscall.SysProcAttr {
	attr := &syscall.SysProcAttr{}
	if !cfg.noProcGroup && !cfg.setsid {
		// Setpgid is implied by Setsid: setsid(2) automatically makes the caller
		// the process group leader of a new process group. Setting Setpgid at the
		// same time causes setpgid(0,0) to run on a session leader, which returns
		// EPERM on Darwin (POSIX forbids changing the PGID of a session leader).
		attr.Setpgid = true
	}
	if cfg.setsid {
		attr.Setsid = true
	}
	// Note: cfg.noctty is intentionally NOT applied on macOS.
	// Use WithNewSession() for session isolation on macOS.
	return attr
}
