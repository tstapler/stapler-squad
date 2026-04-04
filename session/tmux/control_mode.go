package tmux

import (
	"bufio"
	"bytes"
	"fmt"
	"github.com/tstapler/stapler-squad/log"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

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

	// Signal termination
	if t.controlModeDone != nil {
		close(t.controlModeDone)
		t.controlModeDone = nil
	}

	// Close stdin to signal tmux to exit
	if t.controlModeStdin != nil {
		t.controlModeStdin.Close()
		t.controlModeStdin = nil
	}

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
		t.controlModeCmd.Process.Kill()
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

	// Control mode process has exited. Close all subscriber channels so that waiting
	// goroutines (e.g. streamViaControlMode) detect the end-of-stream and unblock.
	// Using controlModeSubMu (write lock) ensures this is serialized with StopControlMode
	// and SubscribeToControlModeUpdates, preventing double-close panics.
	t.controlModeSubMu.Lock()
	t.controlModeExited = true
	for id, ch := range t.controlModeSubscribers {
		close(ch)
		delete(t.controlModeSubscribers, id)
	}
	t.controlModeSubMu.Unlock()
}

// monitorControlModeErrors monitors stderr for control mode errors.
func (t *TmuxSession) monitorControlModeErrors(stderr io.ReadCloser) {
	defer stderr.Close()

	scanner := bufio.NewScanner(stderr)
	for scanner.Scan() {
		select {
		case <-t.controlModeDone:
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
//	%output %PANE_ID DATA     - Terminal output from pane
//	%begin TIME MSGID FLAGS   - Begin command response
//	%end TIME MSGID FLAGS     - End command response
//	%error ERROR_MESSAGE      - Error notification
//	%exit                     - Session closed
func (t *TmuxSession) processControlModeLine(line string) {
	// Skip empty lines
	if line == "" {
		return
	}

	// All control mode lines start with %
	if !strings.HasPrefix(line, "%") {
		// Regular output (shouldn't happen in control mode, but log if it does)
		log.DebugLog.Printf("Unexpected non-control line from tmux: %s", line)
		return
	}

	// Parse control mode notification type
	fields := strings.SplitN(line, " ", 3)
	if len(fields) < 1 {
		return
	}

	notificationType := fields[0]

	switch notificationType {
	case "%output":
		// %output %PANE_ID DATA
		// DATA is octal-encoded for non-printable characters
		if len(fields) >= 3 {
			paneID := fields[1]
			encodedData := fields[2]

			// Decode octal-encoded output
			data := t.decodeControlModeOutput(encodedData)

			if len(data) > 0 {
				// Broadcast to all subscribers
				t.broadcastControlModeUpdate(data)

				if log.DebugLog != nil {
					log.DebugLog.Printf("Control mode output for session '%s' pane %s: %d bytes",
						t.sanitizedName, paneID, len(data))
				}
			}
		}

	case "%exit":
		// Session closed
		log.InfoLog.Printf("Control mode received %%exit for session '%s'", t.sanitizedName)
		// Don't stop here - let the caller handle cleanup

	case "%session-changed":
		// %session-changed $SESSION_ID NAME
		if len(fields) >= 3 {
			log.InfoLog.Printf("Control mode session-changed for '%s': %s", t.sanitizedName, fields[2])
		}

	case "%begin", "%end":
		// Command response markers - we don't send commands, so ignore
		return

	case "%error":
		// %error ERROR_MESSAGE
		if len(fields) >= 2 {
			errorMsg := strings.Join(fields[1:], " ")
			log.ErrorLog.Printf("Control mode error for session '%s': %s", t.sanitizedName, errorMsg)
		}

	default:
		// Unknown notification type - log for debugging
		if log.DebugLog != nil {
			log.DebugLog.Printf("Unknown control mode notification for session '%s': %s", t.sanitizedName, line)
		}
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
