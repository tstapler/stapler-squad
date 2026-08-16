//go:build !windows

// Package safeexec provides thin wrappers around os/exec that pre-set
// WaitDelay on every command. This eliminates the zombie process accumulation
// hazard that occurs when exec.CommandContext is used directly.
package safeexec

import (
	"context"
	"log/slog"
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
//
// A package-level var (not a const) purely so tests can shorten it — see
// safeexec_pg_test.go — without changing CommandContextPG's signature.
var sigkillGrace = 5 * time.Second

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
		// Setpgid: true with no explicit Pgid makes the child its own group
		// leader, so its pgid equals its pid; negating it below targets the
		// whole group rather than just this one process.
		leaderPid := cmd.Process.Pid
		err := syscall.Kill(-leaderPid, syscall.SIGTERM)
		slog.Debug("safeexec: sent SIGTERM to process group", "pgid", leaderPid, "err", err)
		// Captured once here, not re-read from the package var inside the
		// AfterFunc closure below: sigkillGrace is test-overridable (see its
		// doc comment) and a test that restores it on cleanup while this
		// timer is still pending would otherwise race with that write.
		grace := sigkillGrace
		time.AfterFunc(grace, func() {
			killErr := syscall.Kill(-leaderPid, syscall.SIGKILL)
			if killErr == syscall.ESRCH {
				// The group already exited on its own within the grace
				// period (the common/normal-exit path) — not an
				// escalation, so no Warn log or counter increment.
				return
			}
			sigkillEscalations.Add(context.Background(), 1)
			slog.Warn("safeexec: process group ignored SIGTERM, escalated to SIGKILL",
				"pgid", leaderPid,
				"grace", grace,
				"proc_state", procStateSnapshot(leaderPid),
				"err", killErr,
			)
		})
		return err
	}
	return cmd
}
