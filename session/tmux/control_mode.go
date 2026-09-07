package tmux

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/lifecycle"
	"github.com/tstapler/stapler-squad/telemetry"
)

// cmdResult carries the response body and error for a control mode command.
type cmdResult struct {
	body string
	err  error
}

// cmSendReq is an outgoing command queued for the priority sender goroutine.
type cmSendReq struct {
	line     string         // full tmux command line (e.g. "send-keys -t sess -H 61")
	resultCh chan cmdResult // buffered(1) channel for the response
}

// highPrioritySendCh and normalPrioritySendCh are distinct named types over
// the same underlying chan cmSendReq, used for TmuxSession.highPriSendCh/
// normPriSendCh and runCMSender's corresponding parameters. Without this
// distinction, two adjacent same-typed chan cmSendReq parameters/arguments
// can be silently transposed by a future edit and still compile -- inverting
// send priority with no compiler error. A make(chan cmSendReq, N) literal
// remains assignable to either (an unnamed type is assignable to any named
// type sharing its underlying type), so no call site needs to change.
type highPrioritySendCh chan cmSendReq
type normalPrioritySendCh chan cmSendReq

var (
	// ErrControlModeNotRunning is returned when sendCMCommand is called but control mode is not active.
	ErrControlModeNotRunning = errors.New("control mode not running")
	// ErrControlModeStopped is sent to all in-flight commands when the control mode process exits.
	ErrControlModeStopped = errors.New("control mode stopped")
)

// cmCommandsEnabled gates the CM command dispatch path.
// Enabled by default; set STAPLER_SQUAD_CM_COMMANDS=false to opt out.
var cmCommandsEnabled atomic.Bool

// controlModeSlowSubscriberGrace bounds how long broadcastControlModeUpdate will block
// waiting for room in a full subscriber channel before giving up and closing it. A fast
// typing burst (readline/prompt redraws emit several %output events per keystroke) can
// momentarily fill the 100-slot buffer while the consumer is mid-write on a coalesced
// WebSocket frame; that consumer is healthy and about to drain, not stuck. Closing on the
// very first instantaneously-full send conflated that transient burst with a genuinely
// dead subscriber, disconnecting the terminal. Waiting up to this grace period lets a
// bursty-but-healthy consumer catch up; only a subscriber still full after the grace period
// is treated as stuck.
const controlModeSlowSubscriberGrace = 250 * time.Millisecond

func init() {
	cmCommandsEnabled.Store(os.Getenv("STAPLER_SQUAD_CM_COMMANDS") != "false")
}

// StartControlMode begins streaming terminal output via tmux control mode (-C flag).
// This is the proper way to get real-time terminal output from tmux, replacing pipe-pane + FIFO.
// Control mode provides structured notifications (%output, %session-changed, etc.) via stdout.
//
// Benefits over pipe-pane:
// - No FIFO complexity or EOF issues
// - Direct protocol communication with tmux
// - Structured, parseable output format
// - Real-time notifications (no polling)
// - Native tmux feature (not a hack)
//
// See: https://github.com/tmux/tmux/wiki/Control-Mode
func (t *TmuxSession) StartControlMode() error {
	// Serialize concurrent first-time starts. Held for the full fork+init sequence
	// to prevent two callers from both passing the controlModeCmd==nil check.
	t.controlModeStartMu.Lock()
	defer t.controlModeStartMu.Unlock()

	// Increment refcount if already running — atomic under the same lock that
	// protects controlModeCmd/controlModeRemoteProc so no TOCTOU between
	// check and increment.
	t.controlModeSubMu.Lock()
	if t.controlModeCmd != nil || t.controlModeRemoteProc != nil {
		t.controlModeRefCount++
		t.controlModeSubMu.Unlock()
		return nil // Already running; just bumped the refcount
	}
	t.controlModeSubMu.Unlock()

	// Remote branch (ssh-remote-workspaces Phase 4, Task 4.4.1c): tmux
	// control mode's protocol is plain text over stdin/stdout -- it needs no
	// PTY, unlike the raw-PTY-attach path (session/tmux/pty.go's
	// RemotePtyFactory) -- so CommandRunner.Start (already remote-capable,
	// see its own doc comment: "pending Epic 2.3's remote control-mode
	// wiring", this is that wiring) is sufficient. This was gated off in
	// Phase 1 (ADR-002) because the local branch below tracks the spawned
	// process via TrackChildPID/UntrackChildPID and tears it down via
	// cmd.Process.Kill()/.Wait() -- OS-process semantics with no SSH analog.
	// remoteControlModeProc (below) is the local/remote-agnostic replacement
	// for that teardown surface: wait() maps to CommandRunner.Start's own
	// wait func, and kill() closes the SSH channel (there is no "signal a
	// remote PID" over CommandRunner -- closing the channel is the SSH
	// analog of Process.Kill(), see sshSessionStdout.Close()'s doc comment).
	if t.commandRunner().IsRemote() {
		return t.startRemoteControlMode()
	}

	// Build tmux -C attach command
	cmd := t.buildTmuxCommand("-C", "attach-session", "-t", t.sanitizedName)

	// Set up pipes for bidirectional communication
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe for control mode: %w", err)
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		_ = stdout.Close()
		return fmt.Errorf("failed to create stdin pipe for control mode: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		_ = stdout.Close()
		_ = stdin.Close()
		return fmt.Errorf("failed to create stderr pipe for control mode: %w", err)
	}

	// Start the control mode process
	if err := cmd.Start(); err != nil {
		_ = stdout.Close()
		_ = stdin.Close()
		_ = stderr.Close()
		return fmt.Errorf("failed to start control mode for session '%s': %w", t.sanitizedName, err)
	}
	TrackChildPID(cmd.Process.Pid, "tmux control-mode session="+t.sanitizedName)

	// Store all infrastructure and initialize subscriber state atomically under subMu.
	t.controlModeSubMu.Lock()
	t.controlModeCmd = cmd
	t.controlModeStdout = stdout
	t.controlModeStdin = stdin
	t.controlModeDone = make(chan struct{})
	t.highPriSendCh = make(chan cmSendReq, 64)
	t.normPriSendCh = make(chan cmSendReq, 256)
	t.cmSenderExited = make(chan struct{})
	if t.controlModeSubscribers == nil {
		t.controlModeSubscribers = make(map[string]chan []byte)
	}
	t.controlModeExited = false
	t.controlModeRefCount = 1
	t.controlModeSubMu.Unlock()

	// Start goroutines: priority sender, output reader, stderr monitor.
	// highPriSendCh/normPriSendCh are captured here (not read from the struct
	// fields inside runCMSender) so a later StartControlMode call reassigning
	// those fields for a fresh session can never race with this goroutine's
	// own reads of its own channels -- see runCMSender's doc comment.
	doneCh := t.controlModeDone
	highPriSendCh := t.highPriSendCh
	normPriSendCh := t.normPriSendCh
	cmSenderExited := t.cmSenderExited
	go t.runCMSender(doneCh, stdin, highPriSendCh, normPriSendCh, cmSenderExited)
	go t.readControlModeOutput()
	go t.monitorControlModeErrors(stderr)

	return nil
}

// remoteControlModeProc is the remote counterpart of the local
// controlModeCmd (*exec.Cmd): wait blocks until the remote "tmux -C
// attach-session" process exits (CommandRunner.Start's own wait func); kill
// force-terminates it when wait doesn't return within StopControlMode's
// grace period. There is no PID to track/signal over SSH (see
// CommandRunner's own doc comment on why OS-process semantics are
// deliberately kept out of that interface), so kill closes the underlying
// SSH channel instead -- the SSH analog of Process.Kill().
type remoteControlModeProc struct {
	wait func() error
	kill func()
}

// startRemoteControlMode is StartControlMode's remote branch (Task 4.4.1c):
// starts "tmux [-L socket] -C attach-session -t name" as a plain (non-PTY)
// piped remote command via CommandRunner.Start, wires its stdin/stdout into
// the same controlModeStdin/controlModeStdout fields the local branch uses
// (both are already interface-typed -- io.WriteCloser/io.ReadCloser -- so
// runCMSender and readControlModeOutput need no changes at all to work
// against a remote-backed pipe instead of a local one), and starts the same
// sender/reader goroutines the local branch starts.
//
// Not started here: monitorControlModeErrors. CommandRunner.Start has no
// stderr pipe (SSH's exec channel doesn't expose a separable stderr stream
// through this abstraction); monitorControlModeErrors is diagnostic-only
// (see its doc comment -- it just logs stderr lines at Debug), so skipping
// it for the remote path loses debug-level visibility only, not correctness.
func (t *TmuxSession) startRemoteControlMode() error {
	args := Socket(t.serverSocket).Args("-C", "attach-session", "-t", t.sanitizedName)
	// wrapRemoteCommand unsets $TMUX and forces a known-good $TERM before
	// this command reaches the remote tmux server -- see its doc comment
	// and research/pitfalls.md §2; the same wrapping listSessionsRaw's
	// remote fallback and EnsureRemoteSession already apply to every other
	// remote tmux invocation.
	runName, runArgs := wrapRemoteCommand(Binary(), args)

	stdin, stdout, wait, err := t.commandRunner().Start(context.Background(), "", runName, runArgs...)
	if err != nil {
		return fmt.Errorf("failed to start remote control mode for session '%s': %w", t.sanitizedName, err)
	}

	t.controlModeSubMu.Lock()
	t.controlModeRemoteProc = &remoteControlModeProc{
		wait: wait,
		kill: func() { _ = stdout.Close() },
	}
	t.controlModeStdout = stdout
	t.controlModeStdin = stdin
	t.controlModeDone = make(chan struct{})
	t.highPriSendCh = make(chan cmSendReq, 64)
	t.normPriSendCh = make(chan cmSendReq, 256)
	t.cmSenderExited = make(chan struct{})
	if t.controlModeSubscribers == nil {
		t.controlModeSubscribers = make(map[string]chan []byte)
	}
	t.controlModeExited = false
	t.controlModeRefCount = 1
	t.controlModeSubMu.Unlock()

	doneCh := t.controlModeDone
	highPriSendCh := t.highPriSendCh
	normPriSendCh := t.normPriSendCh
	cmSenderExited := t.cmSenderExited
	go t.runCMSender(doneCh, stdin, highPriSendCh, normPriSendCh, cmSenderExited)
	go t.readControlModeOutput()

	log.Info("successfully started remote control mode", "session", t.sanitizedName)

	return nil
}

// StopControlMode stops the control mode streaming and cleans up resources.
// With refcounting, this only actually stops the underlying process when the last
// caller disconnects. Intermediate callers decrement the refcount and return early.
func (t *TmuxSession) StopControlMode() error {
	// Serialize with StartControlMode to prevent races during the 0→1 and 1→0 transitions.
	t.controlModeStartMu.Lock()
	defer t.controlModeStartMu.Unlock()

	// Decrement refcount under the lock. Only proceed to teardown when the count
	// reaches zero (i.e., this is the last caller).
	t.controlModeSubMu.Lock()
	if t.controlModeRefCount > 0 {
		t.controlModeRefCount--
	} else {
		log.Warn("StopControlMode called with refcount already 0", "session", t.sanitizedName)
	}
	remaining := t.controlModeRefCount
	t.controlModeSubMu.Unlock()

	if remaining > 0 {
		return nil // Other callers still active; leave the process running.
	}

	if t.controlModeCmd == nil && t.controlModeRemoteProc == nil {
		return nil // Not running (or already stopped by a prior call).
	}

	// Mark as intentional before closing anything so that the scanner-EOF path
	// in readControlModeOutput() knows not to fire the onExit callback.
	t.intentionalStop.Store(true)

	// Signal termination — close controlModeDone under a lock to prevent
	// a panic if readControlModeOutput's unilateral-exit path has already nilled
	// this channel before we reach teardown.
	t.controlModeSubMu.Lock()
	if t.controlModeDone != nil {
		close(t.controlModeDone)
		t.controlModeDone = nil
	}
	t.controlModeSubMu.Unlock()

	// Wait for the sender goroutine to exit before closing stdin. The sender
	// owns all stdin writes; closing stdin underneath it would panic or corrupt state.
	if t.cmSenderExited != nil {
		select {
		case <-t.cmSenderExited:
		case <-time.After(2 * time.Second):
			log.Warn("CM sender goroutine did not exit in time", "session", t.sanitizedName)
		}
		t.cmSenderExited = nil
	}

	// Nil out send queues so cmEnabled() returns false immediately.
	// Must be done under controlModeSubMu to prevent data races with
	// SendInputViaControlMode and enqueueCMCommand which read these fields.
	t.controlModeSubMu.Lock()
	t.highPriSendCh = nil
	t.normPriSendCh = nil
	t.controlModeSubMu.Unlock()

	// Close stdin to signal tmux to exit. A failure here is not fatal: the
	// wait/kill logic below falls back to a hard kill after a 2s timeout if
	// tmux never sees EOF and exits on its own.
	t.cmdSendMu.Lock()
	if t.controlModeStdin != nil {
		if err := t.controlModeStdin.Close(); err != nil {
			log.Warn("failed to close control mode stdin", "session", t.sanitizedName, "err", err)
		}
		t.controlModeStdin = nil
	}
	t.cmdSendMu.Unlock()

	// Wait for process to exit (with timeout). Exactly one of
	// t.controlModeCmd/t.controlModeRemoteProc is non-nil here (guarded by
	// the early-return above); wait/kill are resolved to the matching
	// local (*exec.Cmd) or remote (SSH-channel) implementation.
	var wait func() error
	var kill func()
	if t.controlModeCmd != nil {
		UntrackChildPID(t.controlModeCmd.Process.Pid)
		cmd := t.controlModeCmd
		wait = cmd.Wait
		kill = func() { _ = cmd.Process.Kill() }
	} else {
		proc := t.controlModeRemoteProc
		wait = proc.wait
		kill = proc.kill
	}

	done := make(chan error, 1)
	go func() {
		done <- wait()
	}()

	select {
	case err := <-done:
		if err != nil && err.Error() != "signal: killed" {
			log.Warn("control mode process exited with error", "session", t.sanitizedName, "err", err)
		}
	case <-time.After(2 * time.Second):
		// Timeout after 2 seconds - force kill
		log.Warn("control mode process did not exit cleanly, killing", "session", t.sanitizedName)
		kill()
		<-done // Wait for kill to complete
	}

	// Close stdout. The process has already exited or been killed above, so
	// any close error here is inconsequential cleanup.
	if t.controlModeStdout != nil {
		_ = t.controlModeStdout.Close()
		t.controlModeStdout = nil
	}

	// Close all subscriber channels and nil the cmd/refcount under the same lock
	// so that StartControlMode() cannot observe a stale non-nil cmd after teardown.
	t.controlModeSubMu.Lock()
	t.closeAllSubscribersLocked()
	t.controlModeCmd = nil
	t.controlModeRemoteProc = nil
	t.controlModeRefCount = 0
	t.controlModeSubMu.Unlock()

	return nil
}

// classifyControlModeExit reports why this control-mode generation ended,
// consolidating what used to be 3 independently-repeated t.intentionalStop.Load()
// checks (scanner-EOF fallback, %exit handler, %session-closed handler) into
// one shared helper (endControlModeGenerationV2), called from every v2 exit route.
func (t *TmuxSession) classifyControlModeExit() lifecycle.Reason {
	if t.intentionalStop.Load() {
		return lifecycle.ReasonDeliberateClose
	}
	return lifecycle.ReasonTransportDrop
}

// endControlModeGenerationV2 ends the span for this generation (decrementing
// session_lifecycle_active_generations and tagging session_lifecycle_ends_total
// with reason) and fires onExit at most once, if reason.ShouldFireExitCallback()
// says so. Guarded by t.onExitOnce so it's safe to call from every v2 exit
// route (the scan loop's %exit/%session-closed detection, the scanner-EOF
// fallback, and the doneCh-closed shutdown branch) without double-counting,
// regardless of which one reaches it first for a given generation.
func (t *TmuxSession) endControlModeGenerationV2(span trace.Span, reason lifecycle.Reason, message string) {
	t.onExitOnce.Do(func() {
		lifecycle.EndGeneration(span, "tmux_control_mode", reason)
		if reason.ShouldFireExitCallback() && t.onExit != nil {
			t.onExit(message)
		}
	})
}

// readControlModeOutput reads and parses control mode notifications from tmux.
// This runs in a goroutine and processes lines like:
//
//	%output %0 hello world
//	%session-changed $13 session-name
//	%exit
func (t *TmuxSession) readControlModeOutput() {
	// Captured once, like doneCh below, so every decision in this generation
	// (whether to open the span, the scanner-EOF fallback, the doneCh-closed
	// exit, and processControlModeLineWithV2's %exit/%session-closed cases)
	// sees the same value -- re-reading the env var independently at each of
	// those points could otherwise see it flip mid-generation and call
	// EndGeneration on a stale/zero-value span, or skip it when it shouldn't.
	v2Enabled := config.TmuxLifecycleV2Enabled()

	// One generation span per invocation, opened only when the v2 lifecycle
	// path is enabled (Story 3.1.2's "the new mechanism, including its
	// observability, is what's opt-in" framing). Deliberately no paired
	// defer EndGeneration here -- span is ended exactly once by whichever of
	// the v2 exit routes (see endControlModeGenerationV2) reaches it first.
	var span trace.Span
	if v2Enabled {
		_, span = lifecycle.StartGeneration(context.Background(), "tmux_control_mode")
	}

	// Capture under RLock — StopControlMode nils/closes controlModeDone under
	// controlModeSubMu.Lock(), so reading the field without a lock races against
	// that write. The comment below predates the fix; the snapshot itself (using
	// a possibly-stale channel value after unlock) remains safe and intentional.
	t.controlModeSubMu.RLock()
	doneCh := t.controlModeDone // capture before StopControlMode can nil it
	t.controlModeSubMu.RUnlock()
	scanner := bufio.NewScanner(t.controlModeStdout)

	for scanner.Scan() {
		select {
		case <-doneCh:
			// doneCh is only closed by StopControlMode() while this generation's
			// scan loop is still running (see classifyControlModeExit's callers
			// and the tail of this function for why no other path can close
			// *this* generation's doneCh) -- so reaching here is definitionally
			// the deliberate-stop case. Ending the generation here (previously
			// missing entirely) closes a false-positive "stuck generation"
			// signal: this is an ordinary shutdown race (a line arrived and was
			// scanned in the narrow window between StopControlMode closing
			// doneCh and it closing controlModeStdout), not a wedge.
			if v2Enabled {
				t.endControlModeGenerationV2(span, lifecycle.ReasonDeliberateClose, "control-mode-stopped")
			}
			return
		default:
			// %output is the hot case (every terminal frame). Handle it in-place using
			// scanner.Bytes() — no string allocation — falling back to scanner.Text() for
			// all other (infrequent) events, which may pass sub-strings to async loggers.
			b := scanner.Bytes()
			if hasOutputPrefix(b) {
				t.handleOutputBytes(b)
			} else if exit := t.processControlModeLineWithV2(scanner.Text(), v2Enabled); exit.detected && v2Enabled {
				// v2 path only (Story 3.1.2): processControlModeLineWithV2's %exit/
				// %session-closed cases report a detected exit here rather than
				// firing onExitOnce themselves, so this is the one place in
				// scope with span (opened above) that ends the generation --
				// regardless of which of the v2 exit routes (this, the scanner-EOF
				// fallback below, or the doneCh case above) reaches it first.
				//
				// The explicit v2Enabled check is redundant today -- exit.detected
				// can only be true when v2Enabled is also true, enforced inside
				// processControlModeLineWithV2 -- but span is a nil trace.Span
				// interface when v2Enabled is false, and endControlModeGenerationV2
				// unconditionally calls span.SetAttributes/span.End(). Keeping the
				// guard local and explicit here, matching the other 2 v2 exit routes
				// (the doneCh case above and the scanner-EOF fallback below), means a
				// future edit that decouples the two invariants can't reintroduce a
				// nil-span panic.
				t.endControlModeGenerationV2(span, t.classifyControlModeExit(), exit.message)
			}
		}
	}

	if err := scanner.Err(); err != nil && err != io.EOF {
		// StopControlMode closes the stdout pipe during shutdown, which produces
		// "file already closed" instead of a clean EOF. Suppress it when expected.
		select {
		case <-doneCh:
			// Shutdown was initiated — pipe closure is expected, not an error.
		default:
			log.Error("control mode output scanner error", "session", t.sanitizedName, "err", err)
		}
	}

	// Drain any in-flight command response (reader-goroutine-only fields; no lock needed).
	if t.curCmdCh != nil {
		select {
		case t.curCmdCh <- cmdResult{err: ErrControlModeStopped}:
		default:
		}
		t.curCmdCh = nil
	}
	t.inCmdResp = false
	t.cmdBodyBuf.Reset()

	// Control mode process has exited. Close all subscriber channels and drain pending
	// commands so that waiting goroutines detect end-of-stream and unblock.
	// Also reset refcount and cmd so that a subsequent StartControlMode() can fork a
	// fresh process instead of fast-returning against a dead (but non-nil) cmd (ARCH-1).
	t.controlModeSubMu.Lock()
	t.controlModeExited = true
	for _, ch := range t.pendingCmds {
		select {
		case ch <- cmdResult{err: ErrControlModeStopped}:
		default:
		}
	}
	t.pendingCmds = nil
	t.closeAllSubscribersLocked()
	// Unilateral exit (process killed/crashed without StopControlMode being
	// called) leaves runCMSender blocked forever on doneCh, since only
	// StopControlMode used to close it -- close it here too so the sender
	// goroutine doesn't leak.
	//
	// Close the doneCh captured locally above (this goroutine's own
	// generation), not necessarily whatever t.controlModeDone currently
	// holds: a fresh StartControlMode() racing this unilateral exit (see
	// ARCH-1 above) can already have reassigned the field to a NEW
	// generation's channel by the time we get here. Closing the live field
	// unconditionally would close the wrong (newer) generation's doneCh
	// instead of unblocking this (older) generation's runCMSender.
	//
	//   - t.controlModeDone == doneCh: nothing has replaced or closed it
	//     yet. Close-and-nil under the lock, same as StopControlMode's own
	//     guard -- whichever of the two gets here first wins; the other
	//     sees nil and skips, so this shared-field case can't double-close.
	//   - t.controlModeDone == nil: StopControlMode already closed and
	//     nilled it (same generation) -- already handled, skip.
	//   - t.controlModeDone is some other non-nil channel: a newer
	//     generation already replaced the field. Our doneCh is now
	//     orphaned from it entirely -- this is the only goroutine that
	//     ever holds this specific reference (monitorControlModeErrors
	//     never closes doneCh), so it's safe to close directly.
	if t.controlModeDone == doneCh {
		close(doneCh)
		t.controlModeDone = nil
	} else if t.controlModeDone != nil {
		close(doneCh)
	}
	// Reset so that the next StartControlMode() call sees a clean slate.
	t.controlModeRefCount = 0
	t.controlModeCmd = nil
	t.controlModeRemoteProc = nil
	t.controlModeSubMu.Unlock()

	// Scanner-EOF fallback: if the pipe closed without a %exit notification (e.g. the
	// tmux server crashed or the process was killed), fire the onExit callback here.
	// intentionalStop guards against false-positive fires during clean StopControlMode().
	if v2Enabled {
		t.endControlModeGenerationV2(span, t.classifyControlModeExit(), "control-mode-pipe-closed")
	} else {
		if !t.intentionalStop.Load() {
			t.onExitOnce.Do(func() {
				if t.onExit != nil {
					t.onExit("control-mode-pipe-closed")
				}
			})
		}
	}
}

// pendingCommandDepth reports how many commands are currently queued waiting
// for a %begin/%end response from tmux's control-mode connection — callers
// use this to skip a control-mode attempt that FIFO ordering already dooms
// to time out (see fastLaneCMAttemptTimeout/controlModeQueueBackpressureThreshold
// in tmux.go).
func (t *TmuxSession) pendingCommandDepth() int {
	t.controlModeSubMu.RLock()
	defer t.controlModeSubMu.RUnlock()
	return len(t.pendingCmds)
}

// monitorControlModeErrors monitors stderr for control mode errors.
func (t *TmuxSession) monitorControlModeErrors(stderr io.ReadCloser) {
	// Capture under RLock — see readControlModeOutput for why the raw field
	// read races against StopControlMode's write under controlModeSubMu.
	t.controlModeSubMu.RLock()
	doneCh := t.controlModeDone // capture before StopControlMode can nil it
	t.controlModeSubMu.RUnlock()
	defer stderr.Close()

	scanner := bufio.NewScanner(stderr)
	for scanner.Scan() {
		select {
		case <-doneCh:
			return
		default:
			line := scanner.Text()
			if line != "" {
				log.Debug("control mode stderr", "session", t.sanitizedName, "line", line)
			}
		}
	}
}

// controlModeExitSignal reports whether processControlModeLineWithV2 observed a
// unilateral exit notification (%exit or %session-closed) that its caller,
// readControlModeOutput, must classify and fire onExitOnce for. Only
// populated when config.TmuxLifecycleV2Enabled() is true: readControlModeOutput
// is the one place that holds this generation's span (opened at its top), so
// the v2 path reports the event back up to it rather than firing onExitOnce
// here, keeping StartGeneration/EndGeneration in the same function scope.
// The v1 (flag-off) path never populates this -- it fires onExitOnce itself,
// verbatim as before.
type controlModeExitSignal struct {
	detected bool
	message  string
}

// processControlModeLine parses and handles a single control mode notification line,
// reading config.TmuxLifecycleV2Enabled() fresh. This is the stable entry point for
// direct-call test scaffolding (see control_mode_dispatch_test.go); readControlModeOutput
// itself calls processControlModeLineWithV2 so every call within one generation shares
// the single value captured at the top of that function -- see its doc comment.
func (t *TmuxSession) processControlModeLine(line string) controlModeExitSignal {
	return t.processControlModeLineWithV2(line, config.TmuxLifecycleV2Enabled())
}

// processControlModeLineWithV2 parses and handles a single control mode notification
// line. Control mode lines start with % and follow specific formats:
//
//	%output %PANE_ID DATA     - Terminal output from pane (always broadcast, even inside response)
//	%begin TIME CMDNUM FLAGS  - Begin command response; pops head of pendingCmds
//	%end TIME CMDNUM FLAGS    - End command response; delivers body to curCmdCh
//	%error TIME CMDNUM FLAGS  - Command failed; delivers error to curCmdCh
//	%exit                     - Session closed
//
// v2Enabled is the caller's single captured config.TmuxLifecycleV2Enabled() value (see
// processControlModeLine and readControlModeOutput) rather than read fresh here, so a
// generation's %exit/%session-closed handling can't disagree with the span/EndGeneration
// decisions made elsewhere in the same generation.
//
// This method is called exclusively from the reader goroutine; inCmdResp, cmdBodyBuf,
// and curCmdCh are reader-goroutine-only fields and require no locking.
func (t *TmuxSession) processControlModeLineWithV2(line string, v2Enabled bool) controlModeExitSignal {
	if line == "" {
		return controlModeExitSignal{}
	}

	// Non-% lines between %begin and %end are body content for the current command.
	if t.inCmdResp && !strings.HasPrefix(line, "%") {
		t.cmdBodyBuf.WriteString(line)
		t.cmdBodyBuf.WriteByte('\n')
		return controlModeExitSignal{}
	}

	if !strings.HasPrefix(line, "%") {
		log.Debug("unexpected non-control line from tmux", "line", line)
		return controlModeExitSignal{}
	}

	notificationType, rest, _ := strings.Cut(line, " ")

	switch notificationType {
	case "%output":
		// Hot path is handled by handleOutputBytes in the scanner loop (no string alloc).
		// This case is kept as fallback for tests and any caller that uses processControlModeLine directly.
		t.handleOutputBytes([]byte(line))
	case "%begin":
		t.handleBeginNotification()
	case "%end":
		t.handleEndNotification()
	case "%error":
		t.handleErrorNotification(rest)
	case "%exit":
		return t.handleExitNotification(v2Enabled)
	case "%session-closed":
		return t.handleSessionClosedNotification(v2Enabled, rest)
	case "%session-changed":
		t.handleSessionChangedNotification(rest)
	default:
		log.Debug("unknown control mode notification", "session", t.sanitizedName, "line", line)
	}
	return controlModeExitSignal{}
}

// handleBeginNotification processes a %begin notification: the start of a command
// response. If we're already in a response (unexpected double-%begin), the previous
// pending command is failed before state resets, then the head of the FIFO pending-
// commands queue is popped to become the current in-flight command.
func (t *TmuxSession) handleBeginNotification() {
	if t.inCmdResp && t.curCmdCh != nil {
		select {
		case t.curCmdCh <- cmdResult{err: errors.New("tmux: unexpected %begin before %end")}:
		default:
		}
		t.curCmdCh = nil
	}
	t.controlModeSubMu.Lock()
	if len(t.pendingCmds) > 0 {
		t.curCmdCh = t.pendingCmds[0]
		t.pendingCmds = t.pendingCmds[1:]
	}
	t.controlModeSubMu.Unlock()
	t.inCmdResp = true
	t.cmdBodyBuf.Reset()
}

// handleEndNotification processes a %end notification: delivers the accumulated
// command body to curCmdCh, if a response was in flight.
func (t *TmuxSession) handleEndNotification() {
	if !t.inCmdResp {
		return
	}
	body := strings.TrimRight(t.cmdBodyBuf.String(), "\n")
	if t.curCmdCh != nil {
		select {
		case t.curCmdCh <- cmdResult{body: body}:
		default:
		}
		t.curCmdCh = nil
	}
	t.inCmdResp = false
	t.cmdBodyBuf.Reset()
}

// handleErrorNotification processes a %error notification: delivers the error to
// curCmdCh when a command response was in flight (error description lines appear
// between %begin and %error in the body buffer), otherwise logs it as an
// out-of-band control-mode error.
func (t *TmuxSession) handleErrorNotification(rest string) {
	if !t.inCmdResp {
		if rest != "" {
			log.Error("control mode error", "session", t.sanitizedName, "detail", rest)
		}
		return
	}
	errMsg := strings.TrimSpace(t.cmdBodyBuf.String())
	if errMsg == "" && rest != "" {
		errMsg = rest
	}
	if t.curCmdCh != nil {
		select {
		case t.curCmdCh <- cmdResult{err: fmt.Errorf("tmux: %s", errMsg)}:
		default:
		}
		t.curCmdCh = nil
	}
	t.inCmdResp = false
	t.cmdBodyBuf.Reset()
}

// handleExitNotification processes a %exit notification: tmux control mode itself
// exited. Drains any in-flight command and the pending-commands queue, closes
// subscribers, resets refcount/cmd so a fresh StartControlMode() can fork a new
// process (ARCH-1), and reports (v2) or fires (v1) the exit.
func (t *TmuxSession) handleExitNotification(v2Enabled bool) controlModeExitSignal {
	// Drain the in-flight command (reader-goroutine-only fields, no lock needed).
	if t.inCmdResp && t.curCmdCh != nil {
		select {
		case t.curCmdCh <- cmdResult{err: ErrControlModeStopped}:
		default:
		}
		t.curCmdCh = nil
		t.inCmdResp = false
		t.cmdBodyBuf.Reset()
	}

	// Immediately mark exited and drain so waiting goroutines unblock in <1ms
	// rather than waiting for their 3-second context timeout.  The scanner-EOF
	// path in readControlModeOutput() does the same drain, but there is a race
	// window between %exit and EOF where runCMSender can append a new resultCh
	// to pendingCmds after the EOF drain has already run, leaving it orphaned.
	t.controlModeSubMu.Lock()
	if !t.controlModeExited {
		t.controlModeExited = true
		for _, ch := range t.pendingCmds {
			select {
			case ch <- cmdResult{err: ErrControlModeStopped}:
			default:
			}
		}
		t.pendingCmds = nil
		t.closeAllSubscribersLocked()
		// Reset so the next StartControlMode() call sees a clean slate.
		t.controlModeRefCount = 0
		t.controlModeCmd = nil
		t.controlModeRemoteProc = nil
	}
	t.controlModeSubMu.Unlock()

	log.Info("control mode received %exit", "session", t.sanitizedName)
	if v2Enabled {
		return controlModeExitSignal{detected: true, message: "control-mode-%exit"}
	}
	if !t.intentionalStop.Load() {
		t.onExitOnce.Do(func() {
			if t.onExit != nil {
				t.onExit("control-mode-%exit")
			}
		})
	}
	return controlModeExitSignal{}
}

// handleSessionClosedNotification processes a %session-closed notification: the
// underlying tmux session itself was closed (distinct from control mode exiting).
// Reports (v2) or fires (v1) the exit, matching handleExitNotification's contract.
func (t *TmuxSession) handleSessionClosedNotification(v2Enabled bool, rest string) controlModeExitSignal {
	if rest != "" {
		log.Info("control mode session-closed", "session", t.sanitizedName, "detail", rest)
	}
	if v2Enabled {
		return controlModeExitSignal{detected: true, message: "session-closed"}
	}
	if !t.intentionalStop.Load() {
		t.onExitOnce.Do(func() {
			if t.onExit != nil {
				t.onExit("session-closed")
			}
		})
	}
	return controlModeExitSignal{}
}

// handleSessionChangedNotification processes a %session-changed notification: purely
// informational (the client's attached session changed), just logged.
func (t *TmuxSession) handleSessionChangedNotification(rest string) {
	_, newSession, _ := strings.Cut(rest, " ")
	if newSession != "" {
		log.Info("control mode session-changed", "session", t.sanitizedName, "newSession", newSession)
	}
}

// runCMSender is the single goroutine that owns all stdin writes to the control mode
// process. It drains highPriSendCh (user input) before touching normPriSendCh
// (background polling / resize), giving interactive keystrokes true queue-jumping
// priority over background operations.
//
// doneCh is closed by StopControlMode to trigger shutdown. The goroutine closes
// cmSenderExited when it returns so that StopControlMode can safely close stdin.
//
// highPriSendCh/normPriSendCh/cmSenderExited are passed as parameters (captured
// once by the caller under controlModeSubMu at the same moment as doneCh/stdin)
// rather than read from t.highPriSendCh/t.normPriSendCh/t.cmSenderExited here.
// A unilateral %exit (see the "%exit" case above, ARCH-1) resets controlModeCmd
// to let a fresh StartControlMode() proceed without waiting for this goroutine
// to exit first -- so a new StartControlMode call can reassign those same t.*
// fields to new channels while this goroutine is still running against the old
// ones. Reading t.* directly here raced (confirmed via go test -race) between
// this goroutine's reads and the new call's writes; using only the captured
// locals gives each runCMSender goroutine its own stable, non-racing view for
// its entire lifetime, however many StartControlMode/​%exit cycles follow it.
func (t *TmuxSession) runCMSender(doneCh <-chan struct{}, stdin io.WriteCloser, highPriSendCh highPrioritySendCh, normPriSendCh normalPrioritySendCh, cmSenderExited chan struct{}) {
	defer close(cmSenderExited)

	process := func(req cmSendReq) {
		// Enqueue the response channel BEFORE writing so the reader goroutine
		// never encounters a %begin with no matching pending channel.
		// Guard against controlModeExited: if %exit was already processed, the drain
		// has run and no one will ever drain a newly appended resultCh, causing a
		// 3-second context timeout for the caller.
		t.controlModeSubMu.Lock()
		if t.controlModeExited {
			t.controlModeSubMu.Unlock()
			select {
			case req.resultCh <- cmdResult{err: ErrControlModeStopped}:
			default:
			}
			return
		}
		t.pendingCmds = append(t.pendingCmds, req.resultCh)
		depth := len(t.pendingCmds)
		t.controlModeSubMu.Unlock()
		// Sampled outside the lock: how backed up the %begin/%end response FIFO
		// already was when this command joined it — see
		// control_mode_observability.go's doc comment for why this and the
		// command-duration metric are the two signals needed to tell "queue is
		// backed up" apart from "this one command was individually slow."
		recordControlModePendingDepth(depth)

		if _, err := fmt.Fprintf(stdin, "%s\n", req.line); err != nil {
			log.Debug("CM sender write error", "session", t.sanitizedName, "err", err)
		}
	}

	drain := func(err error) {
		for {
			select {
			case req := <-highPriSendCh:
				select {
				case req.resultCh <- cmdResult{err: err}:
				default:
				}
			case req := <-normPriSendCh:
				select {
				case req.resultCh <- cmdResult{err: err}:
				default:
				}
			default:
				return
			}
		}
	}

	for {
		// Always drain high-priority queue first before considering normal-priority.
		select {
		case req := <-highPriSendCh:
			process(req)
			continue
		default:
		}

		select {
		case req := <-highPriSendCh:
			process(req)
		case req := <-normPriSendCh:
			process(req)
		case <-doneCh:
			drain(ErrControlModeStopped)
			return
		}
	}
}

// sendCMCommand enqueues a normal-priority command and waits for its response.
// Background operations (capture-pane, resize, display-message) use this path.
// User input calls SendInputViaControlMode which enqueues directly to highPriSendCh.
func (t *TmuxSession) sendCMCommand(ctx context.Context, args ...string) (string, error) {
	t.controlModeSubMu.RLock()
	ch := t.normPriSendCh
	t.controlModeSubMu.RUnlock()
	return t.enqueueCMCommand(ctx, ch, args...)
}

// enqueueCMCommand is the shared implementation: builds the request, sends it to
// the appropriate priority channel, then waits for the response or ctx cancellation.
//
// Wrapped in a "tmux.control_mode.command" span/tmux_control_mode_command_duration_ms
// metric (control_mode_observability.go) covering the whole round trip — enqueue
// wait plus the wait for tmux's %begin/%end response — since either half can
// dominate: a full channel buffer stalls the enqueue select, while a busy reader
// goroutine (backed up processing live %output scrollback ahead of our command's
// response) stalls the second. command is args[0] only (e.g. "display-message"),
// never the full args slice, to keep metric cardinality bounded and avoid leaking
// pane content into a label.
func (t *TmuxSession) enqueueCMCommand(ctx context.Context, ch chan cmSendReq, args ...string) (string, error) {
	command := ""
	if len(args) > 0 {
		command = args[0]
	}
	_, span := telemetry.StartSpan(ctx, "tmux.control_mode.command")
	span.SetAttributes(attribute.String("session", t.sanitizedName), attribute.String("command", command))
	start := time.Now()
	defer func() {
		span.End()
	}()

	if ch == nil {
		recordControlModeCommand(command, time.Since(start), false)
		span.SetAttributes(attribute.Bool("control_mode_not_running", true))
		return "", ErrControlModeNotRunning
	}
	resultCh := make(chan cmdResult, 1)
	req := cmSendReq{line: strings.Join(args, " "), resultCh: resultCh}

	select {
	case ch <- req:
	case <-ctx.Done():
		dur := time.Since(start)
		span.SetAttributes(attribute.Int64("duration_ms", dur.Milliseconds()), attribute.Bool("timed_out", true), attribute.String("stage", "enqueue"))
		recordControlModeCommand(command, dur, true)
		return "", ctx.Err()
	}

	select {
	case result := <-resultCh:
		dur := time.Since(start)
		span.SetAttributes(attribute.Int64("duration_ms", dur.Milliseconds()))
		recordControlModeCommand(command, dur, false)
		return result.body, result.err
	case <-ctx.Done():
		dur := time.Since(start)
		span.SetAttributes(attribute.Int64("duration_ms", dur.Milliseconds()), attribute.Bool("timed_out", true), attribute.String("stage", "response"))
		recordControlModeCommand(command, dur, true)
		return "", ctx.Err()
	}
}

// octalVal maps ASCII byte → its octal digit value (0–7); zero for non-octal bytes.
// Inline table eliminates strconv.ParseUint call and error allocation on every %output event.
var octalVal [256]byte

func init() {
	for c := byte('0'); c <= '7'; c++ {
		octalVal[c] = c - '0'
	}
}

// outputLinePrefix is the literal prefix of every tmux control mode %output notification.
var outputLinePrefix = []byte("%output ")

// hasOutputPrefix reports whether b starts with "%output ".
func hasOutputPrefix(b []byte) bool {
	return bytes.HasPrefix(b, outputLinePrefix)
}

// handleOutputBytes processes a %output line from scanner.Bytes() without allocating a string.
// Format: %output %PANE_ID DATA
func (t *TmuxSession) handleOutputBytes(b []byte) {
	// Skip "%output " prefix (8 bytes).
	rest := b[8:]
	// Find the space separating pane ID from encoded data.
	spaceIdx := bytes.IndexByte(rest, ' ')
	if spaceIdx < 0 {
		return
	}
	encodedData := rest[spaceIdx+1:]
	if len(encodedData) == 0 {
		return
	}
	data := t.decodeControlModeOutput(encodedData)
	if len(data) > 0 {
		t.broadcastControlModeUpdate(data)
		log.Debug("control mode output", "session", t.sanitizedName, "bytes", len(data))
	}
}

// decodeControlModeOutput decodes tmux control mode output format.
// Control mode replaces characters < ASCII 32 and backslash with octal escape sequences (\ooo).
// For example: "hello\012world" represents "hello\nworld"
func (t *TmuxSession) decodeControlModeOutput(encoded []byte) []byte {
	// Pre-allocate at input length: decoded output is never longer than the encoded input
	// (octal escapes encode 1 byte as 4 chars, so the decode is always ≤ len(encoded)).
	result := make([]byte, 0, len(encoded))
	i := 0
	for i < len(encoded) {
		if encoded[i] == '\\' && i+3 < len(encoded) {
			a, b, c := encoded[i+1], encoded[i+2], encoded[i+3]
			if a >= '0' && a <= '7' && b >= '0' && b <= '7' && c >= '0' && c <= '7' {
				result = append(result, octalVal[a]<<6|octalVal[b]<<3|octalVal[c])
				i += 4
				continue
			}
		}
		result = append(result, encoded[i])
		i++
	}
	return result
}

// broadcastControlModeUpdate sends terminal output to all subscribed WebSocket clients.
// A subscriber whose channel is full is closed rather than silently dropped, since any
// gap corrupts the ANSI stream; the grace-period drain and close both run off this call
// (drainSlowSubscriber) so a stalled consumer never blocks the synchronous tmux read loop
// this is called from.
func (t *TmuxSession) broadcastControlModeUpdate(data []byte) {
	t.controlModeSubMu.Lock()
	defer t.controlModeSubMu.Unlock()

	for subscriberID, ch := range t.controlModeSubscribers {
		// Check this before ever attempting the fast-path send below: a goroutine
		// spawned for an older frame may not have parked on ch yet, so trying a
		// send here first could win a freed slot and deliver this frame ahead of
		// that older one. An older frame still draining means the subscriber
		// isn't keeping up, so close it instead of racing frame order.
		if t.slowSendInFlight[subscriberID] {
			log.Warn("control mode subscriber channel still full while a previous frame was draining, closing subscriber", "subscriber", subscriberID, "session", t.sanitizedName)
			t.closeSubscriberLocked(subscriberID, ch)
			continue
		}

		select {
		case ch <- data:
			// Successfully sent
			continue
		default:
			// Channel momentarily full - fall through rather than concluding the
			// subscriber is stuck on a single snapshot.
		}

		if t.slowSendInFlight == nil {
			t.slowSendInFlight = make(map[string]bool)
		}
		t.slowSendInFlight[subscriberID] = true
		go t.drainSlowSubscriber(subscriberID, ch, data)
	}
}

// closeSubscriberLocked closes and removes subscriberID's channel, deferring the close
// via pendingCloseAfterDrain if a drainSlowSubscriber goroutine is still blocked sending
// on it (avoids a send-on-closed-channel panic). Callers must hold controlModeSubMu.
func (t *TmuxSession) closeSubscriberLocked(subscriberID string, ch chan []byte) {
	delete(t.controlModeSubscribers, subscriberID)
	if t.slowSendInFlight[subscriberID] {
		if t.pendingCloseAfterDrain == nil {
			t.pendingCloseAfterDrain = make(map[string]chan []byte)
		}
		t.pendingCloseAfterDrain[subscriberID] = ch
		return
	}
	close(ch)
}

// closeAllSubscribersLocked closes and removes every current subscriber
// channel, safely against any in-flight drainSlowSubscriber goroutines (see
// closeSubscriberLocked). Callers must hold controlModeSubMu.
func (t *TmuxSession) closeAllSubscribersLocked() {
	for id, ch := range t.controlModeSubscribers {
		t.closeSubscriberLocked(id, ch)
	}
}

// drainSlowSubscriber waits up to controlModeSlowSubscriberGrace to deliver data to a
// full subscriber channel, off the synchronous read loop (see broadcastControlModeUpdate).
// Closes and removes the subscriber if the grace period elapses with no room.
func (t *TmuxSession) drainSlowSubscriber(subscriberID string, ch chan []byte, data []byte) {
	sent := false
	select {
	case ch <- data:
		sent = true // Consumer drained in time - a burst, not sustained lag.
	case <-time.After(controlModeSlowSubscriberGrace):
	}

	// One critical section for the whole post-wait decision: a concurrent close
	// request (recorded in pendingCloseAfterDrain while we waited) must resolve
	// to exactly one close(ch) below, never two.
	t.controlModeSubMu.Lock()
	defer t.controlModeSubMu.Unlock()

	delete(t.slowSendInFlight, subscriberID)
	closeCh, deferredClose := t.pendingCloseAfterDrain[subscriberID]
	delete(t.pendingCloseAfterDrain, subscriberID)

	if cur, ok := t.controlModeSubscribers[subscriberID]; !sent && ok && cur == ch {
		// Grace period elapsed with no room, and no concurrent closeSubscriberLocked
		// call has already removed this subscriber (which would have set
		// deferredClose instead — see closeSubscriberLocked).
		delete(t.controlModeSubscribers, subscriberID)
		close(ch)
		log.Warn("control mode subscriber channel full after grace period, closing subscriber", "subscriber", subscriberID, "session", t.sanitizedName)
		return
	}

	// Either the send succeeded, or a concurrent closeSubscriberLocked call already
	// removed this subscriber and deferred the close to us. Honor exactly one such
	// deferred close, if any.
	if deferredClose {
		close(closeCh)
	}
}

// SubscribeToControlModeUpdates registers a new subscriber for real-time terminal output.
// Returns a subscriber ID and a channel that receives terminal output bytes.
// The channel has a buffer of 100 messages to handle burst traffic.
func (t *TmuxSession) SubscribeToControlModeUpdates() (string, chan []byte) {
	t.controlModeSubMu.Lock()
	defer t.controlModeSubMu.Unlock()

	subscriberID := uuid.New().String()
	ch := make(chan []byte, 100) // Buffered channel for burst handling

	// If the control mode process already exited before we subscribed, return a
	// pre-closed channel so the caller immediately sees end-of-stream.
	if t.controlModeExited {
		log.Info("control mode already exited, returning pre-closed channel", "session", t.sanitizedName, "subscriber", subscriberID)
		close(ch)
		return subscriberID, ch
	}

	if t.controlModeSubscribers == nil {
		t.controlModeSubscribers = make(map[string]chan []byte)
	}
	t.controlModeSubscribers[subscriberID] = ch

	return subscriberID, ch
}

// UnsubscribeFromControlModeUpdates removes a subscriber and closes its channel.
func (t *TmuxSession) UnsubscribeFromControlModeUpdates(subscriberID string) {
	t.controlModeSubMu.Lock()
	defer t.controlModeSubMu.Unlock()

	if ch, exists := t.controlModeSubscribers[subscriberID]; exists {
		t.closeSubscriberLocked(subscriberID, ch)
	}
}

// SendInputViaControlMode sends raw bytes to the active pane through the already-open
// control mode connection. Uses the HIGH-PRIORITY queue so user keystrokes always
// jump ahead of any queued background operations (capture-pane, resize, etc.).
//
// Fire-and-forget: enqueues the send-keys command and returns immediately without
// waiting for the tmux %begin/%end ack. The ack is consumed by the reader goroutine
// and discarded. This eliminates one CM round-trip from the interactive input path.
func (t *TmuxSession) SendInputViaControlMode(ctx context.Context, data []byte) error {
	if len(data) == 0 {
		return nil
	}
	t.controlModeSubMu.RLock()
	ch := t.highPriSendCh
	t.controlModeSubMu.RUnlock()
	if ch == nil {
		return ErrControlModeNotRunning
	}
	args := []string{"send-keys", "-t", t.sanitizedName, "-H"}
	for _, b := range data {
		args = append(args, fmt.Sprintf("%02x", b))
	}
	// resultCh is buffered(1): the reader goroutine delivers the ack into it and
	// moves on; nobody reads it, and Go GCs it. Safe because all send sites use
	// `select { case ch <- result: default: }` (non-blocking).
	resultCh := make(chan cmdResult, 1)
	req := cmSendReq{line: strings.Join(args, " "), resultCh: resultCh}
	select {
	case ch <- req:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
