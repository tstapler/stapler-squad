//go:build !windows

// Package safeexec provides thin wrappers around os/exec that pre-set
// WaitDelay on every command. This eliminates the zombie process accumulation
// hazard that occurs when exec.CommandContext is used directly.
package safeexec

import (
	"context"
	"os/exec"
	"syscall"
	"time"
)

// sigkillGrace bounds how long CommandContextPG waits after SIGTERM before
// escalating to SIGKILL. Cmd.Wait calls Process.Wait unconditionally before
// ever consulting WaitDelay (see os/exec.(*Cmd).Wait in the stdlib), so
// WaitDelay bounds only pipe-closing, never the wait for actual process
// exit — if the child ignores or never receives SIGTERM (e.g. a detached
// git gc.auto grandchild, or a foreground process that traps SIGTERM),
// Process.Wait blocks forever regardless of the caller's context deadline
// or WaitDelay. SIGKILL cannot be caught or blocked, so escalating to it
// bounds that wait to sigkillGrace instead of indefinitely.
const sigkillGrace = 5 * time.Second

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
	// Setpgid alone only places the child in its own process group — it does
	// not change what cmd.Cancel signals. Without this override, the default
	// Cancel (cmd.Process.Kill(), i.e. SIGKILL to the single child PID only)
	// still applies, leaving grandchildren (e.g. git spawning a background
	// pack-objects/gc helper) alive and holding the child's inherited
	// stdout/stderr pipes open, which can hang CombinedOutput's Wait() well
	// past both the context deadline and WaitDelay. Killing the negative PID
	// signals the whole process group instead of just the direct child.
	cmd.Cancel = func() error {
		pgid := -cmd.Process.Pid
		err := syscall.Kill(pgid, syscall.SIGTERM)
		time.AfterFunc(sigkillGrace, func() {
			// ESRCH here just means the group already exited; ignore it.
			_ = syscall.Kill(pgid, syscall.SIGKILL)
		})
		return err
	}
	return cmd
}
