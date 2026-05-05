//go:build !windows

// Package safeexec provides thin wrappers around os/exec that pre-set
// WaitDelay on every command. This eliminates the zombie process accumulation
// hazard that occurs when exec.CommandContext is used directly.
package safeexec

import (
	"context"
	"os/exec"
	"syscall"
)

// CommandContextPG returns an exec.Cmd with WaitDelay pre-set AND Setpgid: true.
//
// The Setpgid flag causes the child process to be placed in a new process group.
// When the context fires its cancel func, Go's internal watchCtx goroutine calls
// cmd.Cancel, which (via the override set below) sends SIGTERM to the entire
// process group rather than just the direct child. This ensures grandchildren
// spawned by the child are also terminated, preventing orphaned processes.
//
// IMPORTANT: Do NOT use CommandContextPG for processes that require a controlling
// terminal (e.g. "tmux attach-session" passed to pty.Start()). Setting Setpgid
// without a corresponding Setsid causes the child to remain in the parent's
// session but in a new process group, which can cause SIGTTIN/SIGTTOU issues
// when the child tries to access the terminal. For PTY-attached processes, use
// CommandContext instead and let pty.Start() manage the terminal assignment.
func CommandContextPG(ctx context.Context, name string, arg ...string) *exec.Cmd {
	cmd := CommandContext(ctx, name, arg...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}
	return cmd
}
