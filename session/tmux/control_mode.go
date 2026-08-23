package tmux

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/tstapler/stapler-squad/log"
	"io"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
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
		stdout.Close()
		return fmt.Errorf("failed to create stdin pipe for control mode: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		stdout.Close()
		stdin.Close()
		return fmt.Errorf("failed to create stderr pipe for control mode: %w", err)
	}

	// Start the control mode process
	if err := cmd.Start(); err != nil {
		stdout.Close()
		stdin.Close()
		stderr.Close()
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
	doneCh := t.controlModeDone
	go t.runCMSender(doneCh, stdin)
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
	go t.runCMSender(doneCh, stdin)
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

	// Close stdin to signal tmux to exit.
	t.cmdSendMu.Lock()
	if t.controlModeStdin != nil {
		t.controlModeStdin.Close()
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

	// Close stdout
	if t.controlModeStdout != nil {
		t.controlModeStdout.Close()
		t.controlModeStdout = nil
	}

	// Close all subscriber channels and nil the cmd/refcount under the same lock
	// so that StartControlMode() cannot observe a stale non-nil cmd after teardown.
	t.controlModeSubMu.Lock()
	for id, ch := range t.controlModeSubscribers {
		close(ch)
		delete(t.controlModeSubscribers, id)
	}
	t.controlModeCmd = nil
	t.controlModeRemoteProc = nil
	t.controlModeRefCount = 0
	t.controlModeSubMu.Unlock()

	return nil
}

// readControlModeOutput reads and parses control mode notifications from tmux.
// This runs in a goroutine and processes lines like:
//
//	%output %0 hello world
//	%session-changed $13 session-name
//	%exit
func (t *TmuxSession) readControlModeOutput() {
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
			return
		default:
			// %output is the hot case (every terminal frame). Handle it in-place using
			// scanner.Bytes() — no string allocation — falling back to scanner.Text() for
			// all other (infrequent) events, which may pass sub-strings to async loggers.
			b := scanner.Bytes()
			if hasOutputPrefix(b) {
				t.handleOutputBytes(b)
			} else {
				t.processControlModeLine(scanner.Text())
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
	for id, ch := range t.controlModeSubscribers {
		close(ch)
		delete(t.controlModeSubscribers, id)
	}
	// Unilateral exit (process killed/crashed without StopControlMode being
	// called) leaves runCMSender blocked forever on doneCh, since only
	// StopControlMode used to close it -- close it here too so the sender
	// goroutine doesn't leak. Guarded/nilled the same way StopControlMode
	// does, since both can race to close this channel.
	if t.controlModeDone != nil {
		close(t.controlModeDone)
		t.controlModeDone = nil
	}
	// Reset so that the next StartControlMode() call sees a clean slate.
	t.controlModeRefCount = 0
	t.controlModeCmd = nil
	t.controlModeRemoteProc = nil
	t.controlModeSubMu.Unlock()

	// Scanner-EOF fallback: if the pipe closed without a %exit notification (e.g. the
	// tmux server crashed or the process was killed), fire the onExit callback here.
	// intentionalStop guards against false-positive fires during clean StopControlMode().
	if !t.intentionalStop.Load() {
		t.onExitOnce.Do(func() {
			if t.onExit != nil {
				t.onExit("control-mode-pipe-closed")
			}
		})
	}
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

// processControlModeLine parses and handles a single control mode notification line.
// Control mode lines start with % and follow specific formats:
//
//	%output %PANE_ID DATA     - Terminal output from pane (always broadcast, even inside response)
//	%begin TIME CMDNUM FLAGS  - Begin command response; pops head of pendingCmds
//	%end TIME CMDNUM FLAGS    - End command response; delivers body to curCmdCh
//	%error TIME CMDNUM FLAGS  - Command failed; delivers error to curCmdCh
//	%exit                     - Session closed
//
// This method is called exclusively from the reader goroutine; inCmdResp, cmdBodyBuf,
// and curCmdCh are reader-goroutine-only fields and require no locking.
func (t *TmuxSession) processControlModeLine(line string) {
	if line == "" {
		return
	}

	// Non-% lines between %begin and %end are body content for the current command.
	if t.inCmdResp && !strings.HasPrefix(line, "%") {
		t.cmdBodyBuf.WriteString(line)
		t.cmdBodyBuf.WriteByte('\n')
		return
	}

	if !strings.HasPrefix(line, "%") {
		log.Debug("unexpected non-control line from tmux", "line", line)
		return
	}

	notificationType, rest, _ := strings.Cut(line, " ")

	switch notificationType {
	case "%output":
		// Hot path is handled by handleOutputBytes in the scanner loop (no string alloc).
		// This case is kept as fallback for tests and any caller that uses processControlModeLine directly.
		t.handleOutputBytes([]byte(line))

	case "%begin":
		// Start of a command response. If we're already in a response (unexpected
		// double-%begin), fail the previous pending command before resetting state.
		if t.inCmdResp && t.curCmdCh != nil {
			select {
			case t.curCmdCh <- cmdResult{err: errors.New("tmux: unexpected %begin before %end")}:
			default:
			}
			t.curCmdCh = nil
		}
		// Pop the head of the FIFO queue.
		t.controlModeSubMu.Lock()
		if len(t.pendingCmds) > 0 {
			t.curCmdCh = t.pendingCmds[0]
			t.pendingCmds = t.pendingCmds[1:]
		}
		t.controlModeSubMu.Unlock()
		t.inCmdResp = true
		t.cmdBodyBuf.Reset()

	case "%end":
		if t.inCmdResp {
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

	case "%error":
		if t.inCmdResp {
			// Error description lines appear between %begin and %error in the body buffer.
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
		} else {
			if rest != "" {
				log.Error("control mode error", "session", t.sanitizedName, "detail", rest)
			}
		}

	case "%exit":
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
		// Also reset refcount and cmd so that StartControlMode() can fork a fresh
		// process after this unilateral exit (ARCH-1).
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
			for id, ch := range t.controlModeSubscribers {
				close(ch)
				delete(t.controlModeSubscribers, id)
			}
			// Reset so the next StartControlMode() call sees a clean slate.
			t.controlModeRefCount = 0
			t.controlModeCmd = nil
			t.controlModeRemoteProc = nil
		}
		t.controlModeSubMu.Unlock()

		log.Info("control mode received %exit", "session", t.sanitizedName)
		if !t.intentionalStop.Load() {
			t.onExitOnce.Do(func() {
				if t.onExit != nil {
					t.onExit("control-mode-%exit")
				}
			})
		}

	case "%session-closed":
		if rest != "" {
			log.Info("control mode session-closed", "session", t.sanitizedName, "detail", rest)
		}
		if !t.intentionalStop.Load() {
			t.onExitOnce.Do(func() {
				if t.onExit != nil {
					t.onExit("session-closed")
				}
			})
		}

	case "%session-changed":
		_, newSession, _ := strings.Cut(rest, " ")
		if newSession != "" {
			log.Info("control mode session-changed", "session", t.sanitizedName, "newSession", newSession)
		}

	default:
		log.Debug("unknown control mode notification", "session", t.sanitizedName, "line", line)
	}
}

// runCMSender is the single goroutine that owns all stdin writes to the control mode
// process. It drains highPriSendCh (user input) before touching normPriSendCh
// (background polling / resize), giving interactive keystrokes true queue-jumping
// priority over background operations.
//
// doneCh is closed by StopControlMode to trigger shutdown. The goroutine closes
// cmSenderExited when it returns so that StopControlMode can safely close stdin.
func (t *TmuxSession) runCMSender(doneCh <-chan struct{}, stdin io.WriteCloser) {
	defer close(t.cmSenderExited)

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
		t.controlModeSubMu.Unlock()

		if _, err := fmt.Fprintf(stdin, "%s\n", req.line); err != nil {
			log.Debug("CM sender write error", "session", t.sanitizedName, "err", err)
		}
	}

	drain := func(err error) {
		for {
			select {
			case req := <-t.highPriSendCh:
				select {
				case req.resultCh <- cmdResult{err: err}:
				default:
				}
			case req := <-t.normPriSendCh:
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
		case req := <-t.highPriSendCh:
			process(req)
			continue
		default:
		}

		select {
		case req := <-t.highPriSendCh:
			process(req)
		case req := <-t.normPriSendCh:
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
func (t *TmuxSession) enqueueCMCommand(ctx context.Context, ch chan cmSendReq, args ...string) (string, error) {
	if ch == nil {
		return "", ErrControlModeNotRunning
	}
	resultCh := make(chan cmdResult, 1)
	req := cmSendReq{line: strings.Join(args, " "), resultCh: resultCh}

	select {
	case ch <- req:
	case <-ctx.Done():
		return "", ctx.Err()
	}

	select {
	case result := <-resultCh:
		return result.body, result.err
	case <-ctx.Done():
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
// Takes the full write lock (not RLock) because a slow subscriber is closed and removed
// from t.controlModeSubscribers here rather than having its update dropped: dropping any
// byte of this stream corrupts ANSI/cursor state for terminal consumers, since this is the
// stream actually rendered in the browser (mirrors NativeProcessManager.fanOut's
// close-and-remove pattern in session/native_process_manager.go).
//
// A channel that's instantaneously full is given a bounded grace period
// (controlModeSlowSubscriberGrace) to drain before being closed — see that constant's doc
// comment for why an instant close-on-first-full-send disconnects healthy-but-bursty
// consumers, not just genuinely stuck ones.
func (t *TmuxSession) broadcastControlModeUpdate(data []byte) {
	t.controlModeSubMu.Lock()
	defer t.controlModeSubMu.Unlock()

	for subscriberID, ch := range t.controlModeSubscribers {
		select {
		case ch <- data:
			// Successfully sent
			continue
		default:
			// Channel momentarily full - fall through to the bounded wait below rather
			// than concluding the subscriber is stuck on a single snapshot.
		}

		select {
		case ch <- data:
			// Consumer drained in time - a burst, not sustained lag.
		case <-time.After(controlModeSlowSubscriberGrace):
			// Still full after the grace period - subscriber genuinely can't keep up.
			// Close and remove it rather than dropping this chunk, so the consumer sees
			// end-of-stream instead of a silently corrupted terminal.
			close(ch)
			delete(t.controlModeSubscribers, subscriberID)
			log.Warn("control mode subscriber channel full after grace period, closing subscriber", "subscriber", subscriberID, "session", t.sanitizedName)
		}
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
		close(ch)
		delete(t.controlModeSubscribers, subscriberID)
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
