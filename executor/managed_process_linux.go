//go:build linux

package executor

import "syscall"

// buildSysProcAttr constructs a SysProcAttr from processConfig on Linux.
//
// Setpgid places the child in a new process group (default unless noProcGroup).
// Noctty prevents the child from acquiring a controlling terminal. On Linux,
// this works even when the calling process has no controlling terminal.
// Setsid creates a new session (strongest isolation; implies no controlling terminal).
func buildSysProcAttr(cfg processConfig) *syscall.SysProcAttr {
	attr := &syscall.SysProcAttr{}
	if !cfg.noProcGroup {
		attr.Setpgid = true
	}
	if cfg.setsid {
		attr.Setsid = true
	} else if cfg.noctty {
		// On Linux, Noctty is safe even without a controlling terminal in the parent.
		attr.Noctty = true
	}
	return attr
}
