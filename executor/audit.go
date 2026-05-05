// Package executor provides safe subprocess management for stapler-squad.
// This file implements audit logging for all subprocess invocations.
package executor

import (
	"context"
	"log/slog"
	"time"
)

// AuditEntry holds structured metadata for one subprocess invocation.
// It is passed to AuditHook.OnExec after cmd.Wait() returns.
type AuditEntry struct {
	// Command is the argv for the subprocess. Secret positions are replaced
	// with "<redacted>" when WithRedactArgs or WithProcessRedactArgs is used.
	Command []string

	// WorkDir is cmd.Dir at invocation time. Empty string means the process
	// inherited the Go binary's current working directory.
	WorkDir string

	// StartTime is the time the subprocess was started (before cmd.Start).
	StartTime time.Time

	// Duration is the elapsed time from Start to Wait returning.
	Duration time.Duration

	// ExitCode is the process exit code. -1 if the process was killed by signal.
	ExitCode int

	// PID is the process ID, valid after cmd.Start returns.
	PID int

	// KilledByCtx is true if the subprocess was killed because its context was
	// cancelled or its deadline expired.
	KilledByCtx bool

	// KilledByStop is true if the subprocess was killed by ManagedProcess.Stop().
	KilledByStop bool
}

// AuditHook is implemented by consumers that want to observe subprocess
// invocations. OnExec is called synchronously after cmd.Wait() returns;
// it must not block. Heavy work (disk I/O, network calls) should be
// dispatched to a background goroutine inside the implementation.
type AuditHook interface {
	OnExec(entry AuditEntry)
}

// LoggingAuditHook is the default AuditHook implementation. It emits
// structured log records via log/slog:
//   - slog.LevelDebug for successful exits (ExitCode == 0, no kill)
//   - slog.LevelInfo for non-zero exit codes or killed processes
type LoggingAuditHook struct {
	// Logger is the slog.Logger to use. If nil, slog.Default() is used.
	Logger *slog.Logger
}

// OnExec implements AuditHook. It logs the AuditEntry at Debug or Info level.
func (h *LoggingAuditHook) OnExec(entry AuditEntry) {
	logger := h.Logger
	if logger == nil {
		logger = slog.Default()
	}

	level := slog.LevelDebug
	if entry.ExitCode != 0 || entry.KilledByCtx || entry.KilledByStop {
		level = slog.LevelInfo
	}

	logger.Log(context.Background(), level, "subprocess completed",
		slog.Group("subprocess",
			slog.Any("command", entry.Command),
			slog.String("work_dir", entry.WorkDir),
			slog.Time("start_time", entry.StartTime),
			slog.Duration("duration", entry.Duration),
			slog.Int("exit_code", entry.ExitCode),
			slog.Int("pid", entry.PID),
			slog.Bool("killed_by_ctx", entry.KilledByCtx),
			slog.Bool("killed_by_stop", entry.KilledByStop),
		),
	)
}

// ctxKey is the unexported context key type for AuditHook values.
// Using a private type prevents key collisions with other packages.
type ctxKey struct{}

// WithAuditHook returns a new context that carries hook. Pass this context
// to executor.New or executor.StartProcess to enable audit logging for that
// invocation. The hook is called once per subprocess after Wait() returns.
func WithAuditHook(ctx context.Context, hook AuditHook) context.Context {
	return context.WithValue(ctx, ctxKey{}, hook)
}

// AuditHookFromCtx extracts the AuditHook from ctx. Returns nil if no hook
// has been associated with this context via WithAuditHook.
func AuditHookFromCtx(ctx context.Context) AuditHook {
	hook, _ := ctx.Value(ctxKey{}).(AuditHook)
	return hook
}

// emitAudit extracts the AuditHook from ctx (if any) and calls OnExec.
// This is a no-op if ctx carries no hook. Called internally by ShortLivedCmd
// and ManagedProcess after Wait returns.
func emitAudit(ctx context.Context, entry AuditEntry) {
	hook := AuditHookFromCtx(ctx)
	if hook == nil {
		return
	}
	hook.OnExec(entry)
}

// redactArgs returns a copy of argv with the positions listed in indices
// replaced by "<redacted>". Indices out of range are silently ignored.
func redactArgs(argv []string, indices []int) []string {
	if len(indices) == 0 {
		return argv
	}
	out := make([]string, len(argv))
	copy(out, argv)
	for _, i := range indices {
		if i >= 0 && i < len(out) {
			out[i] = "<redacted>"
		}
	}
	return out
}
