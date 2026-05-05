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
	if !cfg.noProcGroup {
		attr.Setpgid = true
	}
	if cfg.setsid {
		attr.Setsid = true
	}
	return attr
}
