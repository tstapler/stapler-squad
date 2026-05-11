// Package executor provides safe subprocess management for stapler-squad.
// This file implements ManagedProcess, a lifecycle-managed long-running subprocess.
package executor

import (
	"bufio"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"runtime"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/tstapler/stapler-squad/executor/safeexec"
)

// processConfig holds configuration for StartProcess.
type processConfig struct {
	dir         string
	extraEnv    []string
	replaceEnv  []string
	stdin       io.Reader
	stdout      io.Writer // when set, used directly as cmd.Stdout; no reader exposed
	stderr      io.Writer // when set, used directly as cmd.Stderr; no reader exposed
	redactArgs  []int
	rlimits     RlimitConfig
	gracePeriod time.Duration
	noProcGroup bool
	noctty      bool // add Noctty: true to SysProcAttr (safe default for background processes)
	setsid      bool // add Setsid: true (strongest isolation, implies no controlling terminal)
}

// defaultGracePeriod is the default time ManagedProcess.Stop() waits between
// SIGTERM and SIGKILL. Set to 5 seconds — enough for graceful tmux shutdown.
const defaultGracePeriod = 5 * time.Second

// ProcessOption is a functional option for StartProcess.
type ProcessOption func(*processConfig)

// WithProcessDir sets the working directory for the subprocess.
func WithProcessDir(dir string) ProcessOption {
	return func(c *processConfig) { c.dir = dir }
}

// WithProcessEnv appends a single KEY=VALUE pair to the subprocess environment.
func WithProcessEnv(key, val string) ProcessOption {
	return func(c *processConfig) { c.extraEnv = append(c.extraEnv, key+"="+val) }
}

// WithProcessReplaceEnv replaces the entire subprocess environment with env.
func WithProcessReplaceEnv(env []string) ProcessOption {
	return func(c *processConfig) { c.replaceEnv = env }
}

// WithProcessStdin sets the subprocess's stdin reader. The reader must remain
// open for the lifetime of the process. For tmux control-mode, use an os.Pipe()
// and keep the write end open until you want the process to exit.
func WithProcessStdin(r io.Reader) ProcessOption {
	return func(c *processConfig) { c.stdin = r }
}

// WithConsumeStdout directs stdout to w instead of exposing it via Stdout().
// When this option is used, ManagedProcess.Stdout() returns nil.
func WithConsumeStdout(w io.Writer) ProcessOption {
	return func(c *processConfig) { c.stdout = w }
}

// WithConsumeStderr directs stderr to w instead of exposing it via Stderr().
// When this option is used, ManagedProcess.Stderr() returns nil.
func WithConsumeStderr(w io.Writer) ProcessOption {
	return func(c *processConfig) { c.stderr = w }
}

// WithGracePeriod sets the time between SIGTERM and SIGKILL during Stop().
// Defaults to 5 seconds. Set shorter for test processes.
func WithGracePeriod(d time.Duration) ProcessOption {
	return func(c *processConfig) { c.gracePeriod = d }
}

// WithProcessRlimits sets per-subprocess resource limits (Linux only).
func WithProcessRlimits(cfg RlimitConfig) ProcessOption {
	return func(c *processConfig) { c.rlimits = cfg }
}

// WithoutProcessGroupMP disables Setpgid for this process. Use for processes
// that need to remain in the parent's process group (rare; most background
// processes should use the default Setpgid: true).
func WithoutProcessGroupMP() ProcessOption {
	return func(c *processConfig) { c.noProcGroup = true }
}

// WithNoControllingTerminal sets Noctty: true on SysProcAttr. Use for background
// processes that must not receive SIGHUP when a terminal closes (e.g. tmux
// control-mode processes, daemons). This is the safe default and is set
// automatically; this option is provided for documentation clarity.
func WithNoControllingTerminal() ProcessOption {
	return func(c *processConfig) { c.noctty = true }
}

// WithNewSession sets Setsid: true on SysProcAttr, creating a new session.
// This provides the strongest terminal isolation: the child cannot receive
// signals from the parent's session. Implies no controlling terminal.
// Use with caution — some processes (like tmux) require session membership.
func WithNewSession() ProcessOption {
	return func(c *processConfig) { c.setsid = true }
}

// WithProcessRedactArgs specifies argv positions containing secrets.
// These positions are replaced with "<redacted>" in audit log entries.
func WithProcessRedactArgs(indices ...int) ProcessOption {
	return func(c *processConfig) { c.redactArgs = append(c.redactArgs, indices...) }
}

// ManagedProcess is a lifecycle handle for a long-running subprocess started
// with cmd.Start(). Construct via StartProcess; do not create directly.
//
// ManagedProcess ensures:
//   - The process runs in a new process group (Setpgid: true) by default
//   - Stop() sends SIGTERM to the process group, then SIGKILL after gracePeriod
//   - stdout/stderr are exposed as io.Reader (backed by os.Pipe(), not io.Pipe())
//   - A single reaper goroutine owns cmd.Wait(), preventing multiple Wait() races
//   - A finalizer provides last-resort cleanup if Stop() is never called
type ManagedProcess struct {
	cmd         *exec.Cmd
	cancel      context.CancelFunc // cancels the derived context passed to cmd
	stopCh      chan struct{}      // closed by Stop(); triggers graceful shutdown
	done        chan struct{}      // closed by reaper goroutine when cmd.Wait() returns
	waitErr     chan error         // buffered(1); written once by reaper goroutine
	stopped     atomic.Bool        // guards against concurrent Stop() calls
	gracePeriod time.Duration

	stdoutReader io.Reader // nil if WithConsumeStdout was used
	stderrReader io.Reader // nil if WithConsumeStderr was used

	// audit fields captured at start
	auditCtx context.Context
	name     string
	args     []string
	startAt  time.Time
}

// StartProcess starts name with args, applies opts, sets up a process group,
// pipes stdout/stderr via os.Pipe(), and launches a reaper goroutine. Returns
// a handle immediately after cmd.Start() succeeds.
//
// The process is started with Setpgid: true and Noctty: true by default,
// making it safe for background use. Callers that need a controlling terminal
// should use WithoutProcessGroupMP() and avoid this API (use raw exec.Cmd +
// pty.Start() instead, with //nolint:norawexec justification).
//
// ctx governs the audit hook extraction. The process is not killed when ctx
// is done — use Stop() or WithNewSession() + a context that owns the lifetime.
func StartProcess(ctx context.Context, name string, args []string, opts ...ProcessOption) (*ManagedProcess, error) {
	cfg := processConfig{
		gracePeriod: defaultGracePeriod,
		// noctty defaults to false. WithNoControllingTerminal() enables it.
		// On Linux, noctty prevents the child from acquiring a controlling terminal.
		// On macOS, noctty is not applied (see managed_process_darwin.go) — use
		// WithNewSession() for session isolation on macOS.
	}
	for _, o := range opts {
		o(&cfg)
	}

	// Derive a context whose cancel func we own. Cancelling this context
	// triggers cmd.Cancel (which sends SIGTERM to the process group).
	derived, cancel := context.WithCancel(ctx)

	// Build the exec.Cmd using safeexec (sets WaitDelay) but not CommandContextPG
	// since we apply SysProcAttr ourselves below.
	cmd := safeexec.CommandContext(derived, name, args...)

	// Set WaitDelay generously to give graceful shutdown time to complete.
	// WaitDelay is already set by safeexec.CommandContext to DefaultWaitDelay (2s),
	// but we want it to be at least gracePeriod + 2s for managed processes.
	cmd.WaitDelay = cfg.gracePeriod + 2*time.Second

	// Apply SysProcAttr: Setpgid, Noctty, and optionally Setsid.
	cmd.SysProcAttr = buildSysProcAttr(cfg)

	// Override cmd.Cancel to send SIGTERM to the process group instead of just
	// the direct child. This is the Go 1.20+ cmd.Cancel hook.
	// At this point cmd.Process is nil; the cancel func captures cmd so it can
	// access cmd.Process.Pid at invocation time (after Start).
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return killProcessGroup(cmd.Process.Pid, syscall.SIGTERM)
	}

	// Apply resource limits (Linux: save/restore setrlimit; others: no-op).
	if err := applyRlimits(cmd, cfg.rlimits); err != nil {
		cancel()
		return nil, err
	}

	// Wire I/O.
	// We use os.Pipe() (not io.Pipe()) to avoid deadlocks when the consumer
	// is slow: os.Pipe() is kernel-buffered so the child process doesn't block
	// on write even if the Go reader is not reading.
	var (
		stdoutReader io.Reader
		stderrReader io.Reader
	)

	if cfg.stdout != nil {
		cmd.Stdout = cfg.stdout
	} else {
		r, w, err := os.Pipe()
		if err != nil {
			cancel()
			return nil, err
		}
		cmd.Stdout = w
		stdoutReader = r
		// We close the write end after Start (child inherits it).
		defer func() {
			// This defer runs after cmd.Start returns. If Start failed, we
			// close both ends. If Start succeeded, we close only the write end
			// (the parent no longer needs it; only the child writes to it).
			_ = w.Close()
		}()
	}

	if cfg.stderr != nil {
		cmd.Stderr = cfg.stderr
	} else {
		r, w, err := os.Pipe()
		if err != nil {
			cancel()
			if stdoutReader != nil {
				_ = stdoutReader.(*os.File).Close()
			}
			return nil, err
		}
		cmd.Stderr = w
		stderrReader = r
		defer func() { _ = w.Close() }()
	}

	// Stdin.
	cmd.Stdin = cfg.stdin

	// Environment.
	if cfg.replaceEnv != nil {
		cmd.Env = cfg.replaceEnv
	} else if len(cfg.extraEnv) > 0 {
		cmd.Env = append(os.Environ(), cfg.extraEnv...)
	}

	// Start the process.
	if err := cmd.Start(); err != nil {
		cancel()
		// Close read ends if we opened them.
		if f, ok := stdoutReader.(*os.File); ok {
			_ = f.Close()
		}
		if f, ok := stderrReader.(*os.File); ok {
			_ = f.Close()
		}
		return nil, err
	}
	// Note: defer w.Close() runs here, closing the write ends in the parent.
	// The child has already inherited them via cmd.Start().

	p := &ManagedProcess{
		cmd:          cmd,
		cancel:       cancel,
		stopCh:       make(chan struct{}),
		done:         make(chan struct{}),
		waitErr:      make(chan error, 1),
		gracePeriod:  cfg.gracePeriod,
		stdoutReader: stdoutReader,
		stderrReader: stderrReader,
		auditCtx:     ctx,
		name:         name,
		args:         args,
		startAt:      time.Now(),
	}

	// Install finalizer as last-resort safety net. The reaper goroutine is the
	// primary cleanup mechanism; the finalizer only fires if Stop()/Wait() are
	// never called and the ManagedProcess becomes unreachable.
	runtime.SetFinalizer(p, managedProcessFinalizer)

	// Launch the reaper goroutine. This is the ONLY goroutine that calls
	// cmd.Wait(); it is created exactly once per ManagedProcess.
	go p.reap(cfg.redactArgs)

	return p, nil
}

// reap calls cmd.Wait() and handles the result. It is the sole owner of
// cmd.Wait() and runs as a goroutine launched by StartProcess.
func (p *ManagedProcess) reap(redactIndices []int) {
	err := p.cmd.Wait()

	// exec.ErrWaitDelay means WaitDelay fired before pipes drained: the process
	// was killed and pipes were force-closed. This is an expected outcome during
	// Stop() — filter it so callers don't see an unexpected error.
	if errors.Is(err, exec.ErrWaitDelay) {
		err = nil
	}

	// If the process was killed by our own Stop() call (p.stopped is true),
	// signal-kill errors are expected outcomes, not failures. Convert to nil.
	if p.stopped.Load() && isSignalExit(err) {
		err = nil
	}

	// Signal that the process has exited.
	close(p.done)

	// Write the wait result once (buffered channel, capacity 1).
	p.waitErr <- err

	// Emit audit entry.
	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
	} else if p.cmd.ProcessState != nil {
		exitCode = p.cmd.ProcessState.ExitCode()
	}

	emitAudit(p.auditCtx, AuditEntry{
		Command:      redactArgs(append([]string{p.name}, p.args...), redactIndices),
		WorkDir:      p.cmd.Dir,
		StartTime:    p.startAt,
		Duration:     time.Since(p.startAt),
		ExitCode:     exitCode,
		PID:          p.cmd.Process.Pid,
		KilledByStop: p.stopped.Load(),
	})
}

// Stop initiates graceful shutdown: sends SIGTERM to the process group (via
// p.cancel which triggers cmd.Cancel), then sends SIGKILL to the process group
// after gracePeriod if the process has not exited. Blocks until the process has
// exited. Idempotent: concurrent or repeated calls are safe.
func (p *ManagedProcess) Stop() error {
	// Guard against concurrent Stop() calls: only the first one proceeds.
	if !p.stopped.CompareAndSwap(false, true) {
		// Already stopped (or being stopped by another goroutine). Wait for done.
		<-p.done
		// Drain waitErr safely: if we're second, the first Stop() has already
		// drained it or is about to. Use a select to avoid blocking.
		select {
		case err := <-p.waitErr:
			runtime.KeepAlive(p)
			return err
		default:
			runtime.KeepAlive(p)
			return nil
		}
	}

	// Trigger cmd.Cancel (sends SIGTERM to process group).
	p.cancel()

	// Wait for clean exit within WaitDelay, or escalate to SIGKILL.
	timer := time.NewTimer(p.gracePeriod)
	defer timer.Stop()

	select {
	case <-p.done:
		// Process exited within grace period.
		err := <-p.waitErr
		runtime.KeepAlive(p)
		return err
	case <-timer.C:
		// Grace period expired: belt-and-suspenders SIGKILL to the process group.
		// cmd.WaitDelay handles the direct child, but the group may have outliers.
		if p.cmd.Process != nil {
			_ = killProcessGroup(p.cmd.Process.Pid, syscall.SIGKILL)
		}
		<-p.done
		err := <-p.waitErr
		runtime.KeepAlive(p)
		return err
	}
}

// Wait blocks until the process exits and returns the wait error. If the process
// was stopped via Stop() and exec.ErrWaitDelay fired, Wait returns nil.
func (p *ManagedProcess) Wait() error {
	<-p.done
	// The reaper goroutine writes to waitErr exactly once. We need to handle
	// the case where Stop() already drained it.
	select {
	case err := <-p.waitErr:
		runtime.KeepAlive(p)
		return err
	default:
		runtime.KeepAlive(p)
		return nil
	}
}

// PID returns the process PID. Valid after StartProcess returns.
// Returns 0 if the ManagedProcess was not properly initialized via StartProcess.
func (p *ManagedProcess) PID() int {
	if p.cmd == nil || p.cmd.Process == nil {
		return 0
	}
	return p.cmd.Process.Pid
}

// IsAlive returns true if the process has not yet exited. Non-blocking.
// Returns false if ManagedProcess was not properly initialized via StartProcess.
func (p *ManagedProcess) IsAlive() bool {
	if p.done == nil {
		return false
	}
	select {
	case <-p.done:
		return false
	default:
		return true
	}
}

// Stdout returns an io.Reader for the process's stdout. Returns nil if
// WithConsumeStdout was used. Reading delivers io.EOF when the process exits
// and all buffered output has been consumed.
func (p *ManagedProcess) Stdout() io.Reader {
	return p.stdoutReader
}

// Stderr returns an io.Reader for the process's stderr. Returns nil if
// WithConsumeStderr was used.
func (p *ManagedProcess) Stderr() io.Reader {
	return p.stderrReader
}

// ScanLines reads lines from Stdout() via bufio.Scanner until EOF or ctx is done.
// fn is called for each complete line (without the trailing newline). Blocks until
// the process exits, EOF is reached, or ctx is cancelled. Returns nil on natural
// EOF, ctx.Err() if cancelled.
//
// ScanLines returns nil if Stdout() is nil (WithConsumeStdout was used).
func (p *ManagedProcess) ScanLines(ctx context.Context, fn func(line string)) error {
	if p.stdoutReader == nil {
		return nil
	}

	scanner := bufio.NewScanner(p.stdoutReader)
	scanDone := make(chan error, 1)

	go func() {
		for scanner.Scan() {
			fn(scanner.Text())
		}
		scanDone <- scanner.Err()
	}()

	select {
	case err := <-scanDone:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// isSignalExit returns true if err is an *exec.ExitError caused by a signal.
// This is used to filter expected kill errors when Stop() has been called.
func isSignalExit(err error) bool {
	if err == nil {
		return false
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode() == -1
	}
	return false
}

// managedProcessFinalizer is a last-resort safety net. It kills the process
// group if Stop() was never called before the ManagedProcess became unreachable.
// Do NOT call cmd.Wait() here — the reaper goroutine owns that.
// Do NOT block — finalizer goroutine is shared and must not block.
func managedProcessFinalizer(p *ManagedProcess) {
	if p.stopped.Load() {
		return
	}
	if p.cmd.Process == nil {
		return
	}
	// Best-effort kill: prefer process group, fall back to direct kill.
	_ = killProcessGroup(p.cmd.Process.Pid, syscall.SIGKILL)
}
