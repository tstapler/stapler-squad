// Package executor provides safe subprocess management for stapler-squad.
// This file defines the RlimitConfig struct shared across all platforms.
package executor

// RlimitConfig specifies per-subprocess resource limits. Zero values mean
// "no limit" — the subprocess inherits the parent process's limits unchanged.
//
// Resource limits are enforced on Linux via golang.org/x/sys/unix SysProcAttr.Rlimits,
// which applies the limits only to the child process (not the parent Go runtime).
// On non-Linux platforms (macOS, Windows), RlimitConfig is accepted by the API
// but has no effect — the struct compiles on all platforms, but applyRlimits is
// a no-op stub.
type RlimitConfig struct {
	// MaxCPUSecs is the RLIMIT_CPU limit in seconds. The process receives
	// SIGXCPU when it reaches the soft limit. Zero means no limit.
	MaxCPUSecs uint64

	// MaxVirtBytes is the RLIMIT_AS (virtual address space) limit in bytes.
	// On Linux, attempts to grow the virtual address space beyond this limit
	// fail with ENOMEM. Zero means no limit.
	MaxVirtBytes uint64

	// MaxOpenFiles is the RLIMIT_NOFILE limit (max open file descriptors).
	// Attempts to open more than this many files fail with EMFILE.
	// Zero means no limit.
	MaxOpenFiles uint64
}
