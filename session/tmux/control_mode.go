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

	// Store control mode infrastructure
	t.controlModeCmd = cmd
	t.controlModeStdout = stdout
	t.controlModeStdin = stdin
	t.controlModeDone = make(chan struct{})

	// Initialize subscriber map and reset exited flag
	t.controlModeSubMu.Lock()
	if t.controlModeSubscribers == nil {
		t.controlModeSubscribers = make(map[string]chan []byte)
	}
	t.controlModeExited = false
	t.controlModeSubMu.Unlock()

	// Start goroutines for output processing and error monitoring
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

	// Signal termination
	if t.controlModeDone != nil {
		close(t.controlModeDone)
		t.controlModeDone = nil
	}

	// Close stdin to signal tmux to exit. cmdSendMu prevents a concurrent
	// sendCMCommand from writing to the pipe after we nil it.
	t.cmdSendMu.Lock()
	if t.controlModeStdin != nil {
		t.controlModeStdin.Close()
		t.controlModeStdin = nil
	}
	t.cmdSendMu.Unlock()

	// Wait for process to exit (with timeout)
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

// sendCMCommand writes a tmux command over the control mode stdin pipe and waits
// for the corresponding %begin/%end response. Returns the response body or an error.
//
// args are joined with spaces and terminated with a newline. Format strings containing
// spaces must be pre-quoted (e.g. "'#{pane_width} #{pane_height}'").
//
// Concurrent calls are safe: a dedicated mutex serializes the (enqueue + write) pair so
// that tmux receives commands in the same order as response channels enter the FIFO queue.
// If ctx is cancelled before the response arrives, sendCMCommand returns immediately;
// the stale response channel remains in the queue until tmux delivers its response.
func (t *TmuxSession) sendCMCommand(ctx context.Context, args ...string) (string, error) {
	resultCh := make(chan cmdResult, 1)

	t.cmdSendMu.Lock()
	if t.controlModeStdin == nil {
		t.cmdSendMu.Unlock()
		return "", ErrControlModeNotRunning
	}
	t.controlModeSubMu.Lock()
	t.pendingCmds = append(t.pendingCmds, resultCh)
	t.controlModeSubMu.Unlock()
	_, writeErr := fmt.Fprintf(t.controlModeStdin, "%s\n", strings.Join(args, " "))
	t.cmdSendMu.Unlock()

	if writeErr != nil {
		return "", fmt.Errorf("write to control mode stdin: %w", writeErr)
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
