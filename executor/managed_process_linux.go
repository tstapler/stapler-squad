//go:build linux

package executor

import "syscall"

// buildSysProcAttr constructs a SysProcAttr from processConfig on Linux.
//
// Setpgid places the child in a new process group (default unless noProcGroup).
// Noctty calls ioctl(0, TIOCNOTTY) in the child to detach from the controlling
// terminal. This fails with ENOTTY when the parent has no controlling terminal
// (e.g. systemd services). Prefer WithNewSession() for background processes.
// Setsid creates a new session (strongest isolation; implies no controlling terminal).
func buildSysProcAttr(cfg processConfig) *syscall.SysProcAttr {
	attr := &syscall.SysProcAttr{}
	if !cfg.noProcGroup && !cfg.setsid {
		// Setpgid is implied by Setsid: setsid(2) automatically makes the caller
		// the process group leader of a new process group. Setting Setpgid at the
		// same time causes setpgid(0,0) to run on a session leader, which returns
		// EPERM on Linux (POSIX forbids changing the PGID of a session leader).
		attr.Setpgid = true
	}
	if cfg.setsid {
		attr.Setsid = true
	} else if cfg.noctty {
		// Noctty works only when the parent has a controlling terminal.
		// When running as a systemd service (no TTY), TIOCNOTTY returns ENOTTY
		// and the exec fails. Callers that need terminal isolation without a
		// controlling terminal should use WithNewSession() instead.
		attr.Noctty = true
	}
	return attr
}
