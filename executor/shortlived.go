// Package executor provides safe subprocess management for stapler-squad.
// This file implements ShortLivedCmd, a builder API for one-shot subprocesses.
package executor

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/tstapler/stapler-squad/executor/safeexec"
)

// config holds all optional parameters for a ShortLivedCmd invocation.
// The zero value is valid: no timeout override, inherited environment, no
// rlimits, no process group override, no stdin.
type config struct {
	// timeout, if non-zero, overrides the context deadline when shorter.
	timeout time.Duration

	// dir sets cmd.Dir. Empty string inherits the calling process's cwd.
	dir string

	// extraEnv holds KEY=VALUE pairs appended to os.Environ(). Used when
	// only a few variables need to be added without replacing the full env.
	extraEnv []string

	// replaceEnv, when non-nil, replaces os.Environ() entirely. Takes
	// precedence over extraEnv.
	replaceEnv []string

	// stdin is set as cmd.Stdin. Nil means os.DevNull (no input).
	stdin io.Reader

	// redactIndices lists argv positions to replace with "<redacted>" in
	// AuditEntry.Command. Secrets (tokens, passwords) should be redacted.
	redactIndices []int

	// rlimits configures per-subprocess resource limits (Linux only).
	rlimits RlimitConfig

	// noProcGroup disables Setpgid. Use for processes that need to share
	// the parent's process group (e.g. terminal-owning processes, PTY sessions).
	noProcGroup bool
}

// Option is a functional option for ShortLivedCmd.
type Option func(*config)

// WithTimeout sets a per-command timeout. If d is shorter than the context's
// remaining deadline, the shorter of the two is used. If d is 0 or the context
// already has a shorter deadline, this option has no effect.
func WithTimeout(d time.Duration) Option {
	return func(c *config) { c.timeout = d }
}

// WithDir sets the working directory for the subprocess. If dir is empty,
// the subprocess inherits the calling process's current directory.
func WithDir(dir string) Option {
	return func(c *config) { c.dir = dir }
}

// WithEnv appends a single KEY=VALUE pair to the subprocess environment.
// Multiple calls to WithEnv accumulate; all pairs are appended to os.Environ().
// To replace the entire environment, use WithReplaceEnv instead.
func WithEnv(key, val string) Option {
	return func(c *config) { c.extraEnv = append(c.extraEnv, key+"="+val) }
}

// WithReplaceEnv replaces the entire subprocess environment with env.
// env must contain KEY=VALUE pairs. Takes precedence over WithEnv.
// Use when the subprocess must run with a minimal or controlled environment.
func WithReplaceEnv(env []string) Option {
	return func(c *config) { c.replaceEnv = env }
}

// WithStdin sets the subprocess's stdin reader. If not set, stdin is connected
// to /dev/null (the subprocess receives no input).
func WithStdin(r io.Reader) Option {
	return func(c *config) { c.stdin = r }
}

// WithRedactArgs specifies argv positions that contain secrets. In the AuditEntry
// emitted after the command runs, these positions are replaced with "<redacted>".
// Positions are 0-indexed (0 = the command name, 1 = first arg, etc.).
func WithRedactArgs(indices ...int) Option {
	return func(c *config) { c.redactIndices = append(c.redactIndices, indices...) }
}

// WithRlimits sets per-subprocess resource limits. On Linux, limits are applied
// to the child process via setrlimit. On other platforms this is a no-op.
func WithRlimits(cfg RlimitConfig) Option {
	return func(c *config) { c.rlimits = cfg }
}

// WithoutProcessGroup disables Setpgid for this command. Use when the process
// needs to remain in the parent's process group (e.g. when a terminal or
// controlling PTY is involved). By default, all ShortLivedCmd instances run
// in a new process group for clean signal propagation.
func WithoutProcessGroup() Option {
	return func(c *config) { c.noProcGroup = true }
}

// ShortLivedCmd is a configured, not-yet-started one-shot subprocess.
// Construct with New(). Do not reuse after calling Run, Output, or CombinedOutput.
type ShortLivedCmd struct {
	ctx  context.Context
	name string
	args []string
	cfg  config
}

// New constructs a ShortLivedCmd. ctx governs cancellation, deadline, and audit
// hook extraction (via WithAuditHook). opts are applied in order; later options
// take precedence over earlier ones for scalar fields.
func New(ctx context.Context, name string, args []string, opts ...Option) *ShortLivedCmd {
	c := &ShortLivedCmd{ctx: ctx, name: name, args: args}
	for _, o := range opts {
		o(&c.cfg)
	}
	return c
}

// build constructs an exec.Cmd from the ShortLivedCmd configuration.
// It does NOT start the process. Called internally by Run/Output/CombinedOutput.
//
// The returned context may be derived from c.ctx (e.g. if WithTimeout was set).
// The caller is responsible for cancelling this derived context after the command
// finishes (via defer cancel()).
func (c *ShortLivedCmd) build() (ctx context.Context, cancel context.CancelFunc, cmd *exec.Cmd) {
	ctx = c.ctx
	cancel = func() {} // no-op default

	// Apply per-command timeout if shorter than existing context deadline.
	if c.cfg.timeout > 0 {
		deadline, hasDeadline := ctx.Deadline()
		remaining := time.Until(deadline)
		if !hasDeadline || c.cfg.timeout < remaining {
			ctx, cancel = context.WithTimeout(ctx, c.cfg.timeout)
		}
	}

	// Construct the base command. Use CommandContextPG (with Setpgid) unless
	// the caller opted out with WithoutProcessGroup().
	if c.cfg.noProcGroup {
		cmd = safeexec.CommandContext(ctx, c.name, c.args...)
	} else {
		cmd = safeexec.CommandContextPG(ctx, c.name, c.args...)
	}

	// Working directory.
	cmd.Dir = c.cfg.dir

	// Stdin.
	cmd.Stdin = c.cfg.stdin

	// Environment.
	if c.cfg.replaceEnv != nil {
		cmd.Env = c.cfg.replaceEnv
	} else if len(c.cfg.extraEnv) > 0 {
		cmd.Env = append(os.Environ(), c.cfg.extraEnv...)
	}
	// If neither replaceEnv nor extraEnv is set, cmd.Env remains nil,
	// which causes exec.Cmd to inherit the parent's environment.

	// Resource limits (Linux: save/restore via setrlimit; others: no-op).
	// Note: applyRlimits must be called after the SysProcAttr is set by
	// CommandContextPG (which sets Setpgid) so it can merge Pdeathsig in.
	if err := applyRlimits(cmd, c.cfg.rlimits); err != nil {
		// applyRlimits failures are non-fatal on Darwin (no-op). On Linux,
		// if setrlimit fails (e.g. raising above hard limit), we still run
		// the command — the rlimit simply won't be applied.
		// In a future iteration, this could be surfaced as a warning via
		// the audit log.
		_ = err
	}

	return ctx, cancel, cmd
}

// Run runs the command and discards all output. Returns an error if the command
// exits with a non-zero status, is killed by a signal, or the context is done.
// Equivalent to exec.Cmd.Run() but with WaitDelay, process group, and audit
// logging applied.
func (c *ShortLivedCmd) Run() error {
	ctx, cancel, cmd := c.build()
	defer cancel()

	startAt := time.Now()
	err := cmd.Run()
	duration := time.Since(startAt)

	exitCode, killedByCtx := exitInfo(cmd, err)
	emitAudit(ctx, AuditEntry{
		Command:     redactArgs(append([]string{c.name}, c.args...), c.cfg.redactIndices),
		WorkDir:     c.cfg.dir,
		StartTime:   startAt,
		Duration:    duration,
		ExitCode:    exitCode,
		PID:         pidFromCmd(cmd),
		KilledByCtx: killedByCtx,
	})
	return err
}

// Output runs the command and returns its stdout as a byte slice. Stderr is
// discarded. Returns an error if the command exits with a non-zero status.
func (c *ShortLivedCmd) Output() ([]byte, error) {
	ctx, cancel, cmd := c.build()
	defer cancel()

	startAt := time.Now()
	out, err := cmd.Output()
	duration := time.Since(startAt)

	exitCode, killedByCtx := exitInfo(cmd, err)
	emitAudit(ctx, AuditEntry{
		Command:     redactArgs(append([]string{c.name}, c.args...), c.cfg.redactIndices),
		WorkDir:     c.cfg.dir,
		StartTime:   startAt,
		Duration:    duration,
		ExitCode:    exitCode,
		PID:         pidFromCmd(cmd),
		KilledByCtx: killedByCtx,
	})
	return out, err
}

// CombinedOutput runs the command and returns stdout and stderr merged into a
// single byte slice. Returns an error if the command exits with a non-zero status.
func (c *ShortLivedCmd) CombinedOutput() ([]byte, error) {
	ctx, cancel, cmd := c.build()
	defer cancel()

	startAt := time.Now()
	out, err := cmd.CombinedOutput()
	duration := time.Since(startAt)

	exitCode, killedByCtx := exitInfo(cmd, err)
	emitAudit(ctx, AuditEntry{
		Command:     redactArgs(append([]string{c.name}, c.args...), c.cfg.redactIndices),
		WorkDir:     c.cfg.dir,
		StartTime:   startAt,
		Duration:    duration,
		ExitCode:    exitCode,
		PID:         pidFromCmd(cmd),
		KilledByCtx: killedByCtx,
	})
	return out, err
}

// exitInfo extracts the exit code and context-kill flag from a completed cmd.
func exitInfo(cmd *exec.Cmd, err error) (exitCode int, killedByCtx bool) {
	if err == nil {
		return 0, false
	}

	killedByCtx = errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		exitCode = exitErr.ExitCode()
		// ExitCode() returns -1 for processes killed by signal.
		return exitCode, killedByCtx
	}

	return -1, killedByCtx
}

// pidFromCmd returns the process PID after cmd.Start, or 0 if not available.
func pidFromCmd(cmd *exec.Cmd) int {
	if cmd.Process != nil {
		return cmd.Process.Pid
	}
	return 0
}
