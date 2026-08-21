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
// consulting WaitDelay, so WaitDelay bounds only pipe-closing, never a child
// that ignores or never receives SIGTERM — SIGKILL can't be caught or
// blocked, so escalating to it is what actually bounds the wait.
//
// A var, not a const, purely so tests can shorten it (see safeexec_pg_test.go).
var sigkillGrace = 5 * time.Second

// CommandContextPG returns an exec.Cmd with WaitDelay pre-set AND Setpgid: true,
// so cmd.Cancel (overridden below) can SIGTERM the whole process group instead
// of just the direct child, catching grandchildren the child spawns.
//
// IMPORTANT: Do NOT use CommandContextPG for processes needing a controlling
// terminal (e.g. "tmux attach-session" via pty.Start()) — Setpgid without a
// matching Setsid can trigger SIGTTIN/SIGTTOU. Use CommandContext for those.
func CommandContextPG(ctx context.Context, name string, arg ...string) *exec.Cmd {
	cmd := CommandContext(ctx, name, arg...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}
	// Without this override, the default Cancel (SIGKILL to just the child
	// PID) leaves grandchildren alive holding the child's stdout/stderr pipes
	// open, which can hang CombinedOutput's Wait() past the context deadline.
	cmd.Cancel = func() error {
		// Setpgid: true with no explicit Pgid makes the child its own group
		// leader, so its pgid equals its pid; negating it below targets the
		// whole group rather than just this one process.
		leaderPid := cmd.Process.Pid
		err := syscall.Kill(-leaderPid, syscall.SIGTERM)
		slog.Debug("safeexec: sent SIGTERM to process group", "pgid", leaderPid, "err", err)
		// Captured once, not re-read inside the AfterFunc below: sigkillGrace
		// is test-overridable, and a test restoring it on cleanup while this
		// timer is still pending would otherwise race with that write.
		grace := sigkillGrace
		time.AfterFunc(grace, func() {
			// Snapshot before the kill: SIGKILL can leave the process gone or
			// dying by the time we'd otherwise read its state, which is the
			// diagnostic this log line exists to capture.
			procState := procStateSnapshot(leaderPid)
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
				"proc_state", procState,
				"err", killErr,
			)
		})
		return err
	}
	return cmd
}
