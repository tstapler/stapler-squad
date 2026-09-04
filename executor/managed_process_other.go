//go:build !windows && !linux && !darwin

package executor

import "syscall"

// buildSysProcAttr constructs a SysProcAttr from processConfig on other Unix
// platforms (FreeBSD, OpenBSD, etc.).
//
// We use the same conservative approach as macOS: skip Noctty to avoid
// platform-specific compatibility issues. Setpgid and Setsid are well-supported
// on all POSIX platforms.
func buildSysProcAttr(cfg processConfig) *syscall.SysProcAttr {
	attr := &syscall.SysProcAttr{}
	if !cfg.noProcGroup && !cfg.setsid {
		// Setpgid is implied by Setsid: setsid(2) automatically makes the caller
		// the process group leader of a new process group. Setting Setpgid at the
		// same time causes setpgid(0,0) to run on a session leader, which returns
		// EPERM (POSIX forbids changing the PGID of a session leader).
		attr.Setpgid = true
	}
	if cfg.setsid {
		attr.Setsid = true
	}
	return attr
}
