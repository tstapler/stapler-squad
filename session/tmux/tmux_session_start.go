package tmux

// tmux_session_start.go — establishing a tmux session: local creation (Start/
// StartWithCleanup/start), restoration after a server restart (Restore/
// RestoreWithWorkDir), and the remote-host equivalent (EnsureRemoteSession/
// createRemoteSession/remoteHasSession). Split out of tmux.go (2026-09-12,
// complexity x churn hotspot — see docs/reference/hotspot-ranking.md): start()
// (gocyclo 31) and RestoreWithWorkDir() (gocyclo 22) were the file's two most
// complex functions. Pure file-level move, no behavior change.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/creack/pty"
	"github.com/tstapler/stapler-squad/executor"
	"github.com/tstapler/stapler-squad/log"
)

// Start creates and starts a new tmux session, then attaches to it. Program is the command to run in
// the session (ex. claude). workdir is the git worktree directory.
func (t *TmuxSession) Start(workDir string) error {
	return t.start(workDir, false, nil)
}

// StartWithCleanup creates and starts a new tmux session and returns a cleanup function.
// Usage: cleanup, err := session.StartWithCleanup(workDir); if err == nil { defer cleanup() }
func (t *TmuxSession) StartWithCleanup(workDir string) (CleanupFunc, error) {
	cleanup := CleanupFunc(func() error {
		return t.Close()
	})
	err := t.start(workDir, true, &cleanup)
	if err != nil {
		return nil, err
	}
	return cleanup, nil
}

// setRemainOnExit keeps the pane around when its program exits instead of tmux's
// default of destroying the whole session. Without this, an unexpected exit of the
// wrapped program (OS-killed, crashed, or otherwise) silently erases the session --
// including any output that would explain why it exited -- and the only trace left
// behind is "session doesn't exist" on the next check. Called after every path that
// creates a session (fresh start, and the "recreate after not found" restore fallback).
func (t *TmuxSession) setRemainOnExit() {
	remainCmd := t.buildTmuxCommand("set-option", "-t", t.sanitizedName, "remain-on-exit", "on")
	if err := runGatedErr(context.Background(), t.serverSocket, func() error {
		return t.cmdExec.Run(remainCmd)
	}); err != nil {
		log.Warn("failed to set remain-on-exit for session", "session", t.sanitizedName, "err", err)
	}
}

// ErrWorkDirMissing indicates a session's working directory is unset or no
// longer exists on disk (e.g. a pruned git worktree). Callers can match on it
// with errors.Is to distinguish a permanent failure — the session should be
// failed with a clear status, not silently retried against a guessed directory.
var ErrWorkDirMissing = errors.New("session working directory missing")

// ValidateWorkDir rejects an empty or nonexistent working directory instead
// of letting a caller silently fall back to a guessed one (e.g. os.Getwd(),
// often $HOME for a long-running server). Exported for reuse by
// session/tymux's validateWorkDir wrapper.
func ValidateWorkDir(workDir string) error {
	if workDir == "" {
		return fmt.Errorf("working directory not set: %w", ErrWorkDirMissing)
	}
	info, err := os.Stat(workDir)
	if err != nil {
		return fmt.Errorf("working directory %q is not accessible: %w: %w", workDir, ErrWorkDirMissing, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("working directory %q is not a directory: %w", workDir, ErrWorkDirMissing)
	}
	return nil
}

// preconfigureServerBeforeSession sets exit-empty off and remain-on-exit on,
// in ONE chained tmux invocation, before the session in start() below is
// created -- closing a race that's always present but usually wins locally
// and loses under CI's slower, -race-instrumented syscalls:
// t.setRemainOnExit() only runs AFTER the new-session command returns AND
// existence is confirmed, but a fast-exiting program (e.g. Program="true",
// which exits in microseconds) can already have destroyed the session --
// and, since it was the server's only session, killed the server itself --
// before that point, especially on a brand-new socket with no pre-existing
// server to inherit options from.
//
// MUST be one chained invocation (`cmd1 \; cmd2 \; cmd3`), not separate
// commands run back-to-back -- confirmed empirically (PR #445): a tmux
// server that reaches zero sessions exits near-instantly by default
// (exit-empty defaults to on), so `start-server` followed by a SEPARATE
// `set-option` invocation already finds "no server running" -- the server
// that start-server just reported success for is already gone by the time
// the next process connects. Chaining into one invocation means the server
// never has a gap where it's both running and unprotected: start-server
// brings it up, exit-empty off keeps a zero-session server alive,
// remain-on-exit on then protects the session new-session (in start()) is
// about to create from destruction the instant its program exits. `;` here
// is tmux's own command-separator syntax (parsed by tmux itself from
// distinct argv elements), not a shell operator -- no shell is involved via
// exec.Cmd, so no escaping is needed or applicable.
//
// Retried with the same backoff schedule as EnsureServerRunning
// (serverStartAttempts/serverStartBackoffStart/serverStartBackoffMax): this
// chain's own "start-server" sub-command can transiently fail with "server
// exited unexpectedly" under the same concurrent-spawn contention documented
// there, and returning that failure to the caller (rather than logging and
// continuing) matters here because -- unlike every other non-essential
// option this method used to lump itself in with (history-limit,
// setRemainOnExit) -- a failed start-server here means no server exists at
// all, so the new-session call start() makes right after this returns is
// guaranteed to fail too, just with a more confusing "error starting tmux
// session" message instead of surfacing the real cause.
//
// Known accepted gap: startServerSucceededDespiteError only checks "is a
// server running", so if start-server succeeds but a trailing set-option in
// this chain fails (exit-empty/remain-on-exit are built-in global options on
// a live server, so this is not expected in practice), the retry treats it
// as success with no log at all -- silently leaving the session unprotected
// against the exact fast-exit race this method exists to close. Splitting
// the chain to detect this precisely was rejected: a separate start-server
// then set-option invocation reintroduces the race PR #445 fixed (a
// zero-session server can exit before the second call reaches it).
func (t *TmuxSession) preconfigureServerBeforeSession() error {
	run := func() ([]byte, error) {
		runCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return runGated(runCtx, t.serverSocket, func() ([]byte, error) {
			// A fresh *exec.Cmd every attempt: exec.Cmd can only be Run() once,
			// so reusing one built outside this closure across retries failed
			// every retry with "exec: already started" instead of actually
			// retrying (see TestComprehensiveSessionCreation flakiness).
			preconfigureCmd := t.buildTmuxCommand("start-server", ";", "set-option", "-g", "exit-empty", "off", ";", "set-option", "-g", "remain-on-exit", "on")
			return t.cmdExec.CombinedOutput(preconfigureCmd)
		})
	}
	out, err := ensureServerRunningWithRetry(run, func() bool { return checkServerNotRunning(t.serverSocket) }, serverStartAttempts, serverStartBackoffStart, serverStartBackoffMax)
	if err != nil {
		return fmt.Errorf("failed to pre-configure tmux server before session creation: %w (output: %s)", err, out)
	}
	return nil
}

// start is the internal implementation for Start and StartWithCleanup
//
//nolint:gocognit,gocyclo // pre-existing complexity relocated verbatim by the tmux.go split (sdd:fix-hotspot, 2026-09-12); reducing it is a separate follow-up, not a file move
func (t *TmuxSession) start(workDir string, setupCleanup bool, cleanup *CleanupFunc) error {
	// This method unconditionally builds and runs a LOCAL *exec.Cmd via
	// t.cmdExec -- unlike EnsureRemoteSession, it has no CommandRunner-routed
	// remote path. Refuse loudly for a remote-backed session instead of
	// silently operating against the wrong host, matching the same
	// commandRunner().IsRemote() guard SetWindowSize/RefreshClient already
	// have. This is cheap defense-in-depth, not a fix for the underlying
	// gap: today the only way this path is reached for a remote session is
	// the not-yet-implemented resume-after-restart flow (see commit
	// 808e70eee's "Known gap" note and session/instance.go's resume path) --
	// EnsureRemoteSession is the correct entry point for starting a fresh
	// remote session.
	if t.commandRunner().IsRemote() {
		return fmt.Errorf("start: no local-subprocess fallback for remote session %q -- use EnsureRemoteSession instead", t.sanitizedName)
	}

	// Use a no-cache check here to detect stale sessions from previous server runs.
	// The registry only tracks sessions from the current run, so a session left over
	// from a crashed/restarted server would not be in the registry and DoesSessionExist()
	// would return false, causing new-session to fail with "duplicate session".
	if t.DoesSessionExistNoCache() {
		// Session already exists - we can reuse it
		log.Info("tmux session already exists, reusing", "session", t.sanitizedName)

		// Set up cleanup if requested
		if setupCleanup && cleanup != nil {
			*cleanup = func() error {
				return t.Close()
			}
		}

		return nil
	}

	if err := ValidateWorkDir(workDir); err != nil {
		return fmt.Errorf("cannot start tmux session %s: %w", t.sanitizedName, err)
	}

	if err := t.preconfigureServerBeforeSession(); err != nil {
		return fmt.Errorf("cannot start tmux session %s: %w", t.sanitizedName, err)
	}

	// Create a new detached tmux session and start the program in it.
	// Pass -e CLAUDECODE= to unset CLAUDECODE in the child environment so that
	// nested Claude Code sessions are not blocked by the "nested session" guard.
	historyPath := fmt.Sprintf("%s/.stapler_squad_history", workDir)
	programWithHistory := fmt.Sprintf("env HISTFILE=%s %s", historyPath, t.program)
	newSessionArgs := []string{"new-session", "-d", "-s", t.sanitizedName, "-e", "CLAUDECODE="}
	for _, kv := range t.ExtraEnv {
		newSessionArgs = append(newSessionArgs, "-e", kv)
	}
	for _, kv := range t.extraEnv {
		newSessionArgs = append(newSessionArgs, "-e", kv)
	}
	newSessionArgs = append(newSessionArgs, "-c", workDir, programWithHistory)
	cmd := t.buildTmuxCommand(newSessionArgs...)

	// Use cmdExec.Run() instead of pty.Start() for detached session creation
	// since detached sessions don't need PTY attachment during creation.
	//
	// stderr goes to a scratch FILE, not a pipe/buffer: `tmux new-session -d`
	// forks a detached server that inherits these fds, so a buffer-based
	// capture (CombinedOutput, bytes.Buffer) blocks forever waiting for EOF
	// that the still-running server never sends. A file has no such wait.
	var stderrOutput string
	stderrFile, tmpErr := os.CreateTemp("", "tmux-new-session-stderr-*")
	if tmpErr == nil {
		cmd.Stderr = stderrFile
		defer os.Remove(stderrFile.Name())
		defer stderrFile.Close()
	}
	err := runGatedErr(context.Background(), t.serverSocket, func() error {
		return t.cmdExec.Run(cmd)
	})
	if stderrFile != nil {
		if data, readErr := os.ReadFile(stderrFile.Name()); readErr == nil {
			stderrOutput = strings.TrimSpace(string(data))
		}
	}
	if err != nil {
		// Cleanup any partially created session if any exists.
		if t.DoesSessionExist() {
			cleanupCmd := t.buildTmuxCommand("kill-session", "-t", t.sanitizedName)
			cleanupErr := runGatedErr(context.Background(), t.serverSocket, func() error {
				return t.cmdExec.Run(cleanupCmd)
			})
			if cleanupErr != nil {
				err = fmt.Errorf("%v (cleanup error: %v)", err, cleanupErr)
			}
			t.invalidateExistsCache() // Session was killed, invalidate cache
		}
		// If we have a cleanup function pointer, set it to nil since startup failed
		if setupCleanup && cleanup != nil {
			*cleanup = func() error { return nil }
		}
		if stderrOutput != "" {
			return fmt.Errorf("error starting tmux session: %s (%w)", stderrOutput, err)
		}
		return fmt.Errorf("error starting tmux session: %w", err)
	}

	// `tmux new-session -d` reported success (exit 0) at this point -- confirmed
	// unambiguously via PID and socket file existence, not just the exit code,
	// since a false-positive exit 0 with a not-yet-listening server is exactly
	// the failure mode under investigation for PR #445's CI-only flake (see
	// DoesSessionExistNoCache's expanded error log for the other half of this
	// diagnostic pair).
	log.Info("tmux new-session command succeeded", "session", t.sanitizedName, "serverSocket", t.serverSocket, "stderr", stderrOutput)

	// Invalidate cache so the poll loop gets a fresh check immediately.
	// The pre-creation DoesSessionExist() call above caches a "false" result,
	// and the 5s cache TTL would otherwise cause the first 5s of the
	// timeout window to be wasted on stale data.
	t.invalidateExistsCache()

	// Fast path: confirm session existence directly via list-sessions before entering
	// the poll loop. The push-based registry can lag behind tmux reality (the
	// %session-created event arrives asynchronously), so using the registry alone
	// causes poll-loop timeouts when the event is delayed. A single no-cache check
	// right after successful new-session avoids the 10s wait in the common case.
	//
	// Uses the fast-lane priority variant (doesSessionExistNoCachePriority), not
	// the default-pool DoesSessionExistNoCache: `tmux new-session -d` above already
	// exited 0, so a negative here should only mean "not visible yet", never "gate
	// congestion". Confirmed in production (2026-09-10) that the default pool can
	// starve this check for the entire sessionCreateTimeout window when busy with
	// ordinary traffic (ReviewQueuePoller, control-mode streaming, other sessions'
	// health checks), causing Start() to report a false timeout -- and then abandon,
	// not kill, the session it just created (Close()'s own DoesSessionExist() call
	// hits the identical false negative, so kill-session never runs): the tmux pane
	// is left running, alive and orphaned from the app's perspective, while the
	// caller (session_creation_pipeline.go) marks the session Failed and never wires
	// SessionDriver -- so neither the startup trust-dialog nor the initial prompt
	// ever get delivered even though the session is fine. Routing this specific,
	// latency-sensitive, one-shot check through the resync fast lane (the same
	// isolation CapturePaneContentPriority/RefreshClientPriority already use for
	// this exact class of problem, Epic 4.2) fixes it at the source instead of
	// papering over it with a longer timeout.
	if t.doesSessionExistNoCachePriority() {
		// Proactively update the registry so DoesSessionExist() returns true
		// immediately — the async %session-created event may not have arrived yet.
		if notifier, ok := t.registry.(interface{ NotifySessionCreated(string) }); ok {
			notifier.NotifySessionCreated(t.sanitizedName)
		}
		t.invalidateExistsCache()
		// The registry is push-based: %session-created arrives asynchronously and may
		// not be reflected yet. Wait briefly so that DoesSessionExist() (which takes
		// the registry fast path when healthy) is consistent the moment Start() returns.
		// Without this, callers see false immediately after a successful Start().
		if t.registry != nil && t.registry.IsHealthy() {
			registryDeadline := time.Now().Add(2 * time.Second)
			for !t.registry.SessionExists(t.sanitizedName) {
				if time.Now().After(registryDeadline) {
					log.Warn("registry lagged for session after creation; continuing", "session", t.sanitizedName)
					break
				}
				time.Sleep(5 * time.Millisecond)
			}
			t.invalidateExistsCache()
		}
	} else {
		// Fall back to the poll loop for the rare case where the session isn't
		// immediately visible (e.g. tmux server under heavy load).
		// sessionCreateTimeout gives enough headroom when the tmux server is under load
		// from multiple active sessions (ReviewQueuePoller, control-mode streaming, etc.).
		timeout := time.After(sessionCreateTimeout)
		sleepDuration := sessionPollInitialDelay
		for !t.doesSessionExistNoCachePriority() {
			select {
			case <-timeout:
				if cleanupErr := t.Close(); cleanupErr != nil {
					err = fmt.Errorf("%v (cleanup error: %v)", err, cleanupErr)
				}
				return fmt.Errorf("timed out waiting for tmux session %s: %v", t.sanitizedName, err)
			default:
				time.Sleep(sleepDuration)
				// Exponential backoff up to sessionPollMaxDelay.
				if sleepDuration < sessionPollMaxDelay {
					sleepDuration *= 2
				}
			}
		}
		// Session confirmed by poll loop, invalidate cache for fresh state
		t.invalidateExistsCache()
	}

	// Session exists now, invalidate cache to ensure fresh state
	t.invalidateExistsCache()

	// Set history limit to enable scrollback (default is 2000, we'll use 10000 for more history)
	historyCmd := t.buildTmuxCommand("set-option", "-t", t.sanitizedName, "history-limit", "10000")
	if err := runGatedErr(context.Background(), t.serverSocket, func() error {
		return t.cmdExec.Run(historyCmd)
	}); err != nil {
		log.Warn("failed to set history-limit for session", "session", t.sanitizedName, "err", err)
	}

	t.setRemainOnExit()

	// Set up monitoring for session status tracking
	t.monitor.Store(newStatusMonitor())

	// Set up cleanup if requested
	if setupCleanup && cleanup != nil {
		*cleanup = func() error {
			return t.Close()
		}
	}

	// Session is created and ready - let the user handle any program-specific interactions
	log.Info("tmux session created successfully", "session", t.sanitizedName, "program", programWithHistory)
	return nil
}

// ErrEnsureRemoteSessionRequiresRemoteRunner is returned by EnsureRemoteSession
// when t's CommandRunner is not remote. The local session-creation path
// (start(), reached via Start/StartWithCleanup) already implements the
// equivalent existence-check-then-create flow against t.cmdExec/PTY
// machinery; EnsureRemoteSession exists specifically for the remote case,
// where the connection itself (not just the command) can drop mid-flight.
var ErrEnsureRemoteSessionRequiresRemoteRunner = errors.New("EnsureRemoteSession requires a remote CommandRunner")

// EnsureRemoteSession creates the remote tmux session t.sanitizedName over
// t.commandRunner() if it does not already exist, reusing it if it does.
// This is Story 2.3.2's existence-check-before-create logic in isolation:
// unlike start() (the local session-creation path), it does not set up a
// PTY, control mode, or any of the other local-session machinery -- wiring
// a remote TmuxSession into a full production session lifecycle is Phase
// 4's job (project_plans/ssh-remote-workspaces/implementation/plan.md), not
// this epic's.
//
// The failure mode this closes (research/pitfalls.md §1): an SSH channel
// drop mid-command can be indistinguishable, from the caller's side, from
// the remote command itself failing -- the remote tmux new-session may have
// already succeeded even though Run() returned an error. A caller that
// blindly retries plain "new-session" on that basis would get "duplicate
// session" at best, or -- if the sanitized name were ever allowed to differ
// between attempts -- a genuine duplicate at worst.
//
// This is NOT closed by "-A" (attach-if-exists) alone, despite that being
// the obvious-looking fix: "-A" attaches to an already-existing session by
// re-executing the client against it, which requires a PTY -- and
// SSHRunner.Run never requests one (no RequestPty call anywhere in
// ssh_runner.go, by design: Run is the one-shot "get combined output"
// case, not an interactive attach). Confirmed empirically
// (TestNewSessionA_AgainstExistingSession_FailsOverNonPTYChannel): "tmux
// new-session -A -d" against an already-existing session, run over a
// non-PTY SSH channel, fails with "open terminal failed: not a terminal"
// (exit status 1) -- it does not silently attach. So "-A" alone still
// leaves a caller-visible error in the exact race window this function
// exists to close: has-session reports absent, a concurrent creator (a
// prior dropped-connection retry, or a genuinely concurrent caller) wins
// before this call's own new-session -A runs, and that new-session -A then
// fails against the now-existing session for the PTY reason above.
//
// The actual sequence, each command run through wrapRemoteCommand (Story
// 2.3.1) since t.commandRunner().IsRemote() is required to call this at
// all:
//  1. An explicit "has-session" check first (remoteHasSession), so a caller
//     can distinguish "reused" from "created" without depending on
//     new-session's exit code, and so the common case (no race) never
//     issues a doomed-to-fail new-session -A against an existing session.
//  2. If absent, "new-session -A -d ..." (createRemoteSession). "-A" still
//     matters here even though it can't silently attach over a non-PTY
//     channel: without it, a race-losing new-session would fail with
//     "duplicate session" -- a different, but equally real, tmux-side error
//     -- so this is not "assume -A works and skip everything else," it's
//     "keep -A anyway (correct if a PTY caller ever attaches through this
//     path) and add the recheck this channel actually needs."
//  3. If step 2 fails, one more remoteHasSession recheck before surfacing
//     the error: if the session now exists, that failure was the race
//     above, not a real creation failure, and is treated as success.
func (t *TmuxSession) EnsureRemoteSession(ctx context.Context, workDir string) error {
	runner := t.commandRunner()
	if !runner.IsRemote() {
		return fmt.Errorf("%w (session %q)", ErrEnsureRemoteSessionRequiresRemoteRunner, t.sanitizedName)
	}

	if t.remoteHasSession(ctx, runner) {
		log.Info("remote tmux session already exists, reusing", "session", t.sanitizedName)
		return nil
	}
	log.Info("remote tmux session not found, will create", "session", t.sanitizedName)

	// NOT ValidateWorkDir: that helper's os.Stat(workDir) checks THIS PROCESS's
	// local filesystem, which is the right check for the local start() path
	// (ValidateWorkDir's other two call sites) but wrong here -- workDir is a
	// path on the remote host, which this process cannot os.Stat at all. A
	// remote path that happens to also exist locally (e.g. this package's own
	// tests, whose "remote" is a co-located test sshd exec'ing against the real
	// local filesystem) would pass either check, silently masking the bug for
	// every test written against that pattern; a genuinely different remote
	// path would always fail ValidateWorkDir's local stat regardless of whether
	// it exists on the actual remote host. Checked via the runner instead,
	// mirroring RemoteWorktreeOps.CreateWorktree's own remote `test -d`
	// base_path check (session/git/remote_worktree.go).
	if workDir == "" {
		return fmt.Errorf("cannot start remote tmux session %s: working directory not set: %w", t.sanitizedName, ErrWorkDirMissing)
	}
	existsCtx, existsCancel := context.WithTimeout(ctx, ExistenceCheckTimeout)
	defer existsCancel()
	if out, err := runner.Run(existsCtx, "", "test", "-d", workDir); err != nil {
		return fmt.Errorf("cannot start remote tmux session %s: working directory %q is not accessible on remote host: %w: %s",
			t.sanitizedName, workDir, ErrWorkDirMissing, strings.TrimSpace(string(out)))
	}

	return t.createRemoteSession(ctx, runner, workDir)
}

// createRemoteSession runs "tmux new-session -A -d" for t.sanitizedName
// over runner (already confirmed remote by the caller). See
// EnsureRemoteSession's doc comment for why a new-session -A failure
// triggers one more remoteHasSession recheck before being surfaced as an
// error, rather than being trusted at face value: over SSHRunner's non-PTY
// channel, "-A" against an already-existing session fails instead of
// silently attaching, and that specific failure must not be reported as
// "session creation failed" when the session in fact exists.
func (t *TmuxSession) createRemoteSession(ctx context.Context, runner CommandRunner, workDir string) error {
	newArgs := []string{"new-session", "-A", "-d", "-s", t.sanitizedName, "-c", workDir, t.program}
	newArgs = Socket(t.serverSocket).Args(newArgs...)
	newName, newArgs := wrapRemoteCommand(Binary(), newArgs)
	newCtx, newCancel := context.WithTimeout(ctx, LongRunningCommandTimeout)
	defer newCancel()
	out, err := runner.Run(newCtx, "", newName, newArgs...)
	if err != nil {
		if t.remoteHasSession(ctx, runner) {
			log.Info("remote tmux new-session -A failed but session exists (race with a concurrent creator), treating as success",
				"session", t.sanitizedName, "newSessionErr", err, "newSessionOutput", strings.TrimSpace(string(out)))
			return nil
		}
		return fmt.Errorf("failed to create/attach remote tmux session %s: %w (output: %s)", t.sanitizedName, err, out)
	}

	log.Info("remote tmux session ensured", "session", t.sanitizedName, "workDir", workDir)
	return nil
}

// remoteHasSession runs "tmux has-session" for t.sanitizedName over runner
// (already confirmed remote by the caller), bounded by
// ExistenceCheckTimeout nested inside ctx, and reports whether it succeeded
// (the session exists).
func (t *TmuxSession) remoteHasSession(ctx context.Context, runner CommandRunner) bool {
	hasArgs := Socket(t.serverSocket).Args("has-session", "-t", t.sanitizedName)
	hasName, hasArgs := wrapRemoteCommand(Binary(), hasArgs)
	hasCtx, hasCancel := context.WithTimeout(ctx, ExistenceCheckTimeout)
	defer hasCancel()
	_, err := runner.Run(hasCtx, "", hasName, hasArgs...)
	return err == nil
}

// Restore attaches to an existing session and restores the window size
func (t *TmuxSession) Restore() error {
	return t.RestoreWithWorkDir("")
}

func (t *TmuxSession) RestoreWithWorkDir(workDir string) error {
	// recreateMu serializes the existence-check-then-maybe-create section
	// below across concurrent callers (see its doc comment) -- released
	// before the PTY-attach section, which has its own concurrency-safe
	// design and must keep running for concurrent callers.
	t.recreateMu.Lock()
	if err := t.ensureSessionExistsLocked(workDir); err != nil {
		t.recreateMu.Unlock()
		return err
	}
	t.recreateMu.Unlock()

	return t.attachPTYAfterRestore()
}

// sessionExistsMaxRetries bounds probeSessionExistsWithRetries's exponential
// backoff loop (100ms, 200ms, 400ms, 800ms).
const sessionExistsMaxRetries = 5

// ensureSessionExistsLocked implements RestoreWithWorkDir's "does the
// session still exist, and if not, recreate it" decision. Must be called
// with recreateMu held.
func (t *TmuxSession) ensureSessionExistsLocked(workDir string) error {
	// ponytail: caller already ran DoesSessionExistNoCache() and got false — cache is stale, flush it.
	t.invalidateExistsCache()
	if t.probeSessionExistsWithRetries() {
		log.Info("found existing tmux session, will reattach to preserve history", "session", t.sanitizedName)
		return nil
	}

	// Session doesn't exist after multiple retries
	// CRITICAL: One final check without cache before recreating to prevent accidental destruction
	log.Info("tmux session not found, performing final non-cached verification", "session", t.sanitizedName, "cachedChecks", sessionExistsMaxRetries)
	if t.DoesSessionExistNoCache() {
		// Session actually exists - cache was stale or timing issue
		log.Info("found existing tmux session on final non-cached check (cache was stale), will reattach", "session", t.sanitizedName)
		return nil
	}

	// Session truly doesn't exist after all checks - safe to create new one,
	// once ensureNoLiveOrphan confirms the prior process is actually gone.
	return t.recreateMissingSession(workDir)
}

// probeSessionExistsWithRetries retries DoesSessionExist() with exponential
// backoff to ride out slow tmux startup or transient unavailability before
// RestoreWithWorkDir treats a session as truly gone.
func (t *TmuxSession) probeSessionExistsWithRetries() bool {
	for i := 0; i < sessionExistsMaxRetries; i++ {
		if t.DoesSessionExist() {
			return true
		}
		if i < sessionExistsMaxRetries-1 {
			// Wait before retrying (exponential backoff: 100ms, 200ms, 400ms, 800ms)
			delay := time.Duration(100*(1<<uint(i))) * time.Millisecond
			log.Info("tmux session not found, retrying", "session", t.sanitizedName, "attempt", i+1, "maxRetries", sessionExistsMaxRetries, "delay", delay)
			time.Sleep(delay)
			t.invalidateExistsCache() // Clear cache before retry
		}
	}
	return false
}

// recreateMissingSession creates a fresh tmux session for a name
// RestoreWithWorkDir has confirmed tmux has no record of. Must be called
// with recreateMu held (ensureSessionExistsLocked's only caller).
//
// Uses launchProgram(), not the frozen `program` field, so a relaunch here
// carries the current command (e.g. --resume <uuid>) rather than whatever was
// current when this TmuxSession was first built -- see launchProgram's doc
// comment (BUG matching #791, a different call site: initTmuxSession's own
// reuse guard).
func (t *TmuxSession) recreateMissingSession(workDir string) error {
	// ponytail: never guess a directory here (e.g. os.Getwd(), which for a
	// long-running server process is often $HOME) — a wrong guess silently
	// reconnects the session to the wrong workspace. Fail loudly instead so
	// the caller can surface a clear status to the user.
	if err := ValidateWorkDir(workDir); err != nil {
		return fmt.Errorf("cannot recreate tmux session %s: %w", t.sanitizedName, err)
	}

	t.terminateOrphanedPriorProcess()

	log.Warn("tmux session doesn't exist after all attempts, creating new session instead of restoring", "session", t.sanitizedName, "attempts", sessionExistsMaxRetries)

	program := t.launchProgram()
	cmd := t.buildTmuxCommand(t.newSessionArgs(workDir, program)...)
	return t.runNewSessionCommand(cmd, workDir, program)
}

// newSessionArgs builds the `tmux new-session` argv for recreateMissingSession
// (avoiding a recursive call to Start). -e CLAUDECODE= unsets CLAUDECODE in
// the child environment so a nested Claude Code session isn't blocked by the
// "nested session" guard.
func (t *TmuxSession) newSessionArgs(workDir, program string) []string {
	args := make([]string, 0, 6+2*len(t.ExtraEnv)+2*len(t.extraEnv)+3)
	args = append(args, "new-session", "-d", "-s", t.sanitizedName, "-e", "CLAUDECODE=")
	for _, kv := range t.ExtraEnv {
		args = append(args, "-e", kv)
	}
	for _, kv := range t.extraEnv {
		args = append(args, "-e", kv)
	}
	return append(args, "-c", workDir, program)
}

// runNewSessionCommand executes cmd (built by newSessionArgs) and interprets
// the result: a failure is tolerated when a concurrent creator already won
// (the session now exists) since DoesSessionExist may have timed out and
// returned false incorrectly rather than the create genuinely failing.
func (t *TmuxSession) runNewSessionCommand(cmd *exec.Cmd, workDir, program string) error {
	err := runGatedErr(context.Background(), t.serverSocket, func() error {
		return t.cmdExec.Run(cmd)
	})
	if err != nil {
		// Session creation failed - but it might be because the session already exists
		// (DoesSessionExist may have timed out and returned false incorrectly)
		// Invalidate cache and re-check before returning error
		t.invalidateExistsCache()
		if t.DoesSessionExist() {
			// Session actually exists - the initial check was wrong (likely timeout)
			// Continue with restore instead of returning error
			log.Info("tmux session already exists (initial check was incorrect), continuing with restore", "session", t.sanitizedName)
			return nil
		}
		return fmt.Errorf("failed to create tmux session '%s': %w", t.sanitizedName, err)
	}

	log.Info("created new tmux session", "session", t.sanitizedName, "dir", workDir, "program", program)
	t.invalidateExistsCache() // Session was created, invalidate cache
	// new-session started the tmux server; reset this session's circuit breakers
	// so subsequent DoesSessionExist() calls can verify the session is running.
	if r, ok := t.cmdExec.(executor.Resettable); ok {
		r.Reset()
	}
	t.setRemainOnExit()
	return nil
}

// terminateOrphanedPriorProcess runs the orphan guard (see
// priorProcessAliveFunc's doc comment) immediately before a confirmed
// recreate: `tmux has-session` failing proves tmux itself lost track of the
// session, not that the process it originally launched is dead -- if the
// tmux server was killed/restarted out from under it, that child can survive
// as an orphan. Terminating it here (rather than launching a replacement
// unconditionally) prevents two live processes writing two competing
// transcripts (2026-09-12 incident). No-op when the guard isn't wired up
// (priorProcessAliveFunc is nil, the default).
func (t *TmuxSession) terminateOrphanedPriorProcess() {
	if t.priorProcessAliveFunc == nil || !t.priorProcessAliveFunc() {
		return
	}
	log.Warn("tmux session gone but a previously-launched process for it may still be alive (orphaned by a killed/restarted tmux server); terminating it before relaunching",
		"session", t.sanitizedName)
	if t.killPriorProcessFunc == nil {
		return
	}
	if err := t.killPriorProcessFunc(); err != nil {
		log.Warn("failed to terminate orphaned prior process before relaunch", "session", t.sanitizedName, "err", err)
	}
}

// attachPTYAfterRestore creates (or reuses) the PTY connection for detached
// operations once ensureSessionExistsLocked has confirmed the session
// exists. Split out of RestoreWithWorkDir so recreateMu (held only for the
// existence-check-then-maybe-create decision) is released before this runs --
// see recreateMu's doc comment for why this part must stay concurrency-safe
// for parallel callers rather than being serialized too.
//
// This is needed for SetDetachedSize(), SendKeys(), and the Direct Claude Command Interface.
// We use tmux attach-session to get a PTY handle without actually attaching interactively.
// Always close any existing PTY before creating a new one: the old attach-session may have
// exited (returning EIO on reads) but left t.ptmx non-nil, which would cause the new
// response stream to immediately get EIO. Closing and reopening guarantees a live connection.
// Check-then-act, made race-free the same way as AttachToExisting: snapshot the slot,
// run the blocking ptyFactory.StartWithSize() unlocked, then compare-and-swap install --
// see tryInstallPTYTriple's doc comment.
func (t *TmuxSession) attachPTYAfterRestore() error {
	const ptyMaxRetries = 3
	var lastPTYErr error
	file, gen, closed := t.ptySnapshot()
	if file != nil && !closed {
		_ = t.closePTYAndAttachCmd()
		file, gen, closed = t.ptySnapshot()
	}
	switch {
	case closed:
		lastPTYErr = fmt.Errorf("tmux session '%s' is closed", t.sanitizedName)
	case file == nil:
		for attempt := 0; attempt < ptyMaxRetries; attempt++ {
			if attempt > 0 {
				delay := time.Duration(100*(1<<uint(attempt-1))) * time.Millisecond
				log.Info("retrying PTY attach for session", "session", t.sanitizedName, "attempt", attempt+1, "maxRetries", ptyMaxRetries, "delay", delay)
				time.Sleep(delay)
			}
			// Use StartWithSize so the PTY has non-zero dimensions before tmux attach-session
			// forks. Without this, running headless (systemd with no controlling terminal)
			// produces a 0×0 PTY; tmux reads that size at client startup and immediately
			// disconnects, causing EIO within ~1ms of the response stream starting.
			ws := &pty.Winsize{
				// #nosec G115 -- lastKnownRows/Cols are only ever written via clampWinsizeDim (or the defaultAttachRows/Cols constants), so Load() is always in [0, 65535]
				Rows: uint16(t.lastKnownRows.Load()),
				// #nosec G115 -- see justification above
				Cols: uint16(t.lastKnownCols.Load()),
			}
			ptmx, attachCmd, err := t.ptyFactory.StartWithSize(t.buildAttachCommand(), ws)
			if err != nil {
				lastPTYErr = err
				continue
			}
			waitOnce := new(sync.Once)
			ok, _, closedNow := t.tryInstallPTYTriple(gen, ptmx, attachCmd, waitOnce)
			if !ok {
				// Lost the race: another install (or Close()) won. Discard our own
				// PTY/process instead of leaking them.
				closePTYTriple(ptmx, attachCmd, waitOnce, t.sanitizedName)
				if closedNow {
					lastPTYErr = fmt.Errorf("tmux session '%s' was closed while attaching", t.sanitizedName)
				} else {
					lastPTYErr = nil // another install won; a PTY is already in place
				}
				break
			}
			log.Info("successfully restored PTY connection for tmux session", "session", t.sanitizedName)
			// Diagnostic: watch for unexpected early exit of the attach process.
			// Uses the same sync.Once as closePTYAndAttachCmd so Wait is called exactly once.
			go func(cmd *exec.Cmd, name string, once *sync.Once) {
				var err error
				once.Do(func() { err = cmd.Wait() })
				log.Info("attach-session process exited", "session", name, "exitErr", err)
			}(attachCmd, t.sanitizedName, waitOnce)
			lastPTYErr = nil
			break
		}
	default:
		// file != nil && !closed: a concurrent AttachToExisting/RestoreWithWorkDir already
		// installed a PTY between our close-then-reread above -- defer to the winner, nothing to do.
	}
	if lastPTYErr != nil {
		// Graceful degradation - session can still be viewed via tmux capture-pane,
		// but PTY-based operations (resizing, SendKeys, controller) will be unavailable.
		log.Warn("PTY initialization failed for session after all attempts", "session", t.sanitizedName, "attempts", ptyMaxRetries, "err", lastPTYErr)
	}

	t.monitor.Store(newStatusMonitor())
	return nil
}
