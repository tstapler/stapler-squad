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
	"strconv"
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
	// Check if control mode is already running
	if t.controlModeCmd != nil {
		return nil // Already started
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
		return fmt.Errorf("failed to start control mode for session '%s': %w", t.sanitizedName, err)
	}
	TrackChildPID(cmd.Process.Pid, "tmux control-mode session="+t.sanitizedName)

	// Store control mode infrastructure
	t.controlModeCmd = cmd
	t.controlModeStdout = stdout
	t.controlModeStdin = stdin
	t.controlModeDone = make(chan struct{})

	// Initialize priority send queues and sender-exit signal.
	t.highPriSendCh = make(chan cmSendReq, 64)
	t.normPriSendCh = make(chan cmSendReq, 256)
	t.cmSenderExited = make(chan struct{})

	// Initialize subscriber map and reset exited flag
	t.controlModeSubMu.Lock()
	if t.controlModeSubscribers == nil {
		t.controlModeSubscribers = make(map[string]chan []byte)
	}
	t.controlModeExited = false
	t.controlModeSubMu.Unlock()

	// Start goroutines: priority sender, output reader, stderr monitor.
	doneCh := t.controlModeDone
	go t.runCMSender(doneCh, stdin)
	go t.readControlModeOutput()
	go t.monitorControlModeErrors(stderr)

	return nil
}

// StopControlMode stops the control mode streaming and cleans up resources.
func (t *TmuxSession) StopControlMode() error {
	if t.controlModeCmd == nil {
		return nil // Not running
	}

	// Mark as intentional before closing anything so that the scanner-EOF path
	// in readControlModeOutput() knows not to fire the onExit callback.
	t.intentionalStop.Store(true)

	// Signal termination — this causes runCMSender to drain its queues and exit.
	if t.controlModeDone != nil {
		close(t.controlModeDone)
		t.controlModeDone = nil
	}

	// Wait for the sender goroutine to exit before closing stdin. The sender
	// owns all stdin writes; closing stdin underneath it would panic or corrupt state.
	if t.cmSenderExited != nil {
		select {
		case <-t.cmSenderExited:
		case <-time.After(2 * time.Second):
			log.WarningLog.Printf("CM sender goroutine did not exit in time for session '%s'", t.sanitizedName)
		}
		t.cmSenderExited = nil
	}

	// Nil out send queues so cmEnabled() returns false immediately.
	t.highPriSendCh = nil
	t.normPriSendCh = nil

	// Close stdin to signal tmux to exit.
	t.cmdSendMu.Lock()
	if t.controlModeStdin != nil {
		t.controlModeStdin.Close()
		t.controlModeStdin = nil
	}
	t.cmdSendMu.Unlock()

	// Wait for process to exit (with timeout)
	UntrackChildPID(t.controlModeCmd.Process.Pid)
	done := make(chan error, 1)
	go func() {
		done <- t.controlModeCmd.Wait()
	}()

	select {
	case err := <-done:
		if err != nil && err.Error() != "signal: killed" {
			log.WarningLog.Printf("Control mode process exited with error: %v", err)
		}
	case <-time.After(2 * time.Second):
		// Timeout after 2 seconds - force kill
		log.WarningLog.Printf("Control mode process did not exit cleanly, killing")
		_ = t.controlModeCmd.Process.Kill()
		<-done // Wait for kill to complete
	}

	// Close stdout
	if t.controlModeStdout != nil {
		t.controlModeStdout.Close()
		t.controlModeStdout = nil
	}

	// Close all subscriber channels
	t.controlModeSubMu.Lock()
	for id, ch := range t.controlModeSubscribers {
		close(ch)
		delete(t.controlModeSubscribers, id)
	}
	t.controlModeSubMu.Unlock()

	t.controlModeCmd = nil
	return nil
}

// readControlModeOutput reads and parses control mode notifications from tmux.
// This runs in a goroutine and processes lines like:
//
//	%output %0 hello world
//	%session-changed $13 session-name
//	%exit
func (t *TmuxSession) readControlModeOutput() {
	doneCh := t.controlModeDone // capture before StopControlMode can nil it
	scanner := bufio.NewScanner(t.controlModeStdout)

	for scanner.Scan() {
		select {
		case <-doneCh:
			return
		default:
			line := scanner.Text()
			t.processControlModeLine(line)
		}
	}

	if err := scanner.Err(); err != nil && err != io.EOF {
		// StopControlMode closes the stdout pipe during shutdown, which produces
		// "file already closed" instead of a clean EOF. Suppress it when expected.
		select {
		case <-doneCh:
			// Shutdown was initiated — pipe closure is expected, not an error.
		default:
			log.ErrorLog.Printf("Control mode output scanner error for session '%s': %v", t.sanitizedName, err)
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
	doneCh := t.controlModeDone // capture before StopControlMode can nil it
	defer stderr.Close()

	scanner := bufio.NewScanner(stderr)
	for scanner.Scan() {
		select {
		case <-doneCh:
			return
		default:
			line := scanner.Text()
			if line != "" {
				log.WarningLog.Printf("Control mode stderr for session '%s': %s", t.sanitizedName, line)
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
		if log.DebugLog != nil {
			log.DebugLog.Printf("Unexpected non-control line from tmux: %s", line)
		}
		return
	}

	fields := strings.SplitN(line, " ", 3)
	notificationType := fields[0]

	switch notificationType {
	case "%output":
		// %output %PANE_ID DATA — broadcast even inside a command response block.
		if len(fields) >= 3 {
			paneID := fields[1]
			encodedData := fields[2]
			data := t.decodeControlModeOutput(encodedData)
			if len(data) > 0 {
				t.broadcastControlModeUpdate(data)
				if log.DebugLog != nil {
					log.DebugLog.Printf("Control mode output for session '%s' pane %s: %d bytes",
						t.sanitizedName, paneID, len(data))
				}
			}
		}

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
			if errMsg == "" && len(fields) >= 2 {
				errMsg = strings.Join(fields[1:], " ")
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
			if len(fields) >= 2 {
				log.ErrorLog.Printf("Control mode error for session '%s': %s", t.sanitizedName, strings.Join(fields[1:], " "))
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
		}
		t.controlModeSubMu.Unlock()

		log.InfoLog.Printf("Control mode received %%exit for session '%s'", t.sanitizedName)
		if !t.intentionalStop.Load() {
			t.onExitOnce.Do(func() {
				if t.onExit != nil {
					t.onExit("control-mode-%exit")
				}
			})
		}

	case "%session-closed":
		if len(fields) >= 2 {
			log.InfoLog.Printf("Control mode session-closed for '%s': %s", t.sanitizedName, strings.Join(fields[1:], " "))
		}
		if !t.intentionalStop.Load() {
			t.onExitOnce.Do(func() {
				if t.onExit != nil {
					t.onExit("session-closed")
				}
			})
		}

	case "%session-changed":
		if len(fields) >= 3 {
			log.InfoLog.Printf("Control mode session-changed for '%s': %s", t.sanitizedName, fields[2])
		}

	default:
		if log.DebugLog != nil {
			log.DebugLog.Printf("Unknown control mode notification for session '%s': %s", t.sanitizedName, line)
		}
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
			if log.DebugLog != nil {
				log.DebugLog.Printf("CM sender write error for session '%s': %v", t.sanitizedName, err)
			}
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
	return t.enqueueCMCommand(ctx, t.normPriSendCh, args...)
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

// decodeControlModeOutput decodes tmux control mode output format.
// Control mode replaces characters < ASCII 32 and backslash with octal escape sequences (\ooo).
// For example: "hello\012world" represents "hello\nworld"
func (t *TmuxSession) decodeControlModeOutput(encoded string) []byte {
	var result bytes.Buffer

	i := 0
	for i < len(encoded) {
		if encoded[i] == '\\' && i+3 < len(encoded) {
			// Check for octal escape sequence (\ooo)
			octal := encoded[i+1 : i+4]
			if isOctalDigits(octal) {
				// Parse octal value
				value, err := strconv.ParseUint(octal, 8, 8)
				if err == nil {
					result.WriteByte(byte(value))
					i += 4 // Skip \ooo
					continue
				}
			}
		}

		// Regular character (not an octal escape)
		result.WriteByte(encoded[i])
		i++
	}

	return result.Bytes()
}

// isOctalDigits checks if a string contains exactly 3 octal digits (0-7).
func isOctalDigits(s string) bool {
	if len(s) != 3 {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '7' {
			return false
		}
	}
	return true
}

// broadcastControlModeUpdate sends terminal output to all subscribed WebSocket clients.
func (t *TmuxSession) broadcastControlModeUpdate(data []byte) {
	t.controlModeSubMu.RLock()
	defer t.controlModeSubMu.RUnlock()

	for subscriberID, ch := range t.controlModeSubscribers {
		select {
		case ch <- data:
			// Successfully sent
		default:
			// Channel full - subscriber can't keep up
			// Don't block other subscribers, just log
			log.WarningLog.Printf("Control mode subscriber %s channel full for session '%s', dropping update",
				subscriberID, t.sanitizedName)
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
		log.InfoLog.Printf("Control mode already exited for session '%s', returning pre-closed channel to subscriber %s",
			t.sanitizedName, subscriberID)
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
	ch := t.highPriSendCh
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
