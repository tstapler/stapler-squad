package session

// instance_tmux.go contains tmux session creation, terminal I/O, PTY access, and
// control-mode delegation methods. All methods delegate to i.tmuxManager.

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/tstapler/stapler-squad/executor/safeexec"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/tmux"
)

// GetTmuxSessionName returns the sanitized tmux session name for reconciliation.
// Returns empty string for external or uninitialized sessions.
func (i *Instance) GetTmuxSessionName() string {
	return i.tmuxManager.GetTmuxSessionName()
}

// buildLaunchCommand constructs the final command string used to launch the program
// in tmux, incorporating Claude session resume flags, MCP server URL, and prompt.
func (i *Instance) buildLaunchCommand(claudeSessionID string) string {
	program := i.Program
	if claudeSessionID != "" && strings.Contains(program, "claude") {
		program = fmt.Sprintf("%s --resume %s", program, claudeSessionID)
	}
	if i.MCPServerURL != "" && strings.Contains(program, "claude") {
		mcpFlag := fmt.Sprintf(`--mcp-config '{"mcpServers":{"stapler-squad":{"type":"http","url":%q}}}'`, i.MCPServerURL)
		program = program + " " + mcpFlag
	}
	if i.AppendSystemPrompt != "" && strings.Contains(program, "claude") {
		program = fmt.Sprintf("%s --append-system-prompt %q", program, i.AppendSystemPrompt)
	}
	if i.AutoYes && strings.Contains(program, "claude") {
		program = program + " -y"
	}
	if i.Prompt != "" && claudeSessionID == "" && strings.Contains(program, "claude") {
		program = fmt.Sprintf("%s %q", program, i.Prompt)
	}
	return program
}

// initTmuxSession creates (or reuses) the tmux.TmuxSession object without starting it.
func (i *Instance) initTmuxSession() {
	if i.tmuxManager.HasSession() {
		log.Info("reusing existing tmux session", "session", i.Title)
		return
	}
	var claudeSessionID string
	if i.claudeSession != nil {
		claudeSessionID = i.claudeSession.ConversationUUID
	}
	enrichedProgram := i.buildLaunchCommand(claudeSessionID)
	i.LaunchCommand = enrichedProgram
	log.Info("creating tmux session", "session", i.Title, "program", enrichedProgram)

	tmuxPrefix := i.TmuxPrefix
	if tmuxPrefix == "" {
		tmuxPrefix = "staplersquad_"
	}

	var session *tmux.TmuxSession
	if i.TmuxServerSocket != "" {
		session = tmux.NewTmuxSessionWithServerSocket(i.Title, enrichedProgram, tmuxPrefix, i.TmuxServerSocket, tmux.WithRegistry(nil))
	} else {
		session = tmux.NewTmuxSessionWithPrefix(i.Title, enrichedProgram, tmuxPrefix)
	}
	if i.UUID != "" {
		session.SetExtraEnv([]string{"STAPLER_SESSION_UUID=" + i.UUID})
	}
	i.tmuxManager.SetSession(session)
}

// KillSession terminates the tmux session only (leaves worktree intact).
func (i *Instance) KillSession() error {
	if i.tmuxManager.HasSession() {
		if err := i.tmuxManager.Close(); err != nil {
			return fmt.Errorf("failed to close tmux session: %w", err)
		}
	}
	return nil
}

// KillSessionKeepWorktree terminates tmux session but preserves worktree for recovery scenarios.
func (i *Instance) KillSessionKeepWorktree() error {
	return i.KillSession()
}

// KillExternalSession terminates an external mux session by killing its tmux session.
// This only works for external sessions that were started via claude-mux with tmux integration.
// Returns an error if this is not an external instance or lacks tmux session name.
func (i *Instance) KillExternalSession() error {
	if i.InstanceType != InstanceTypeExternal {
		return fmt.Errorf("not an external instance")
	}
	if i.ExternalMetadata == nil || i.ExternalMetadata.TmuxSessionName == "" {
		return fmt.Errorf("no tmux session name available (session may not support destroy)")
	}

	// Stop the controller if running
	i.StopController()

	// Kill the tmux session
	killCtx, killCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer killCancel()
	cmd := safeexec.CommandContext(killCtx, "tmux", "kill-session", "-t", i.ExternalMetadata.TmuxSessionName)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to kill tmux session '%s': %w", i.ExternalMetadata.TmuxSessionName, err)
	}

	return nil
}

// HasUpdated reports whether terminal content has changed since the last check.
// Returns (updated, hasPrompt) and side-effects terminal timestamps on change.
func (i *Instance) HasUpdated() (updated bool, hasPrompt bool) {
	if !i.started || i.Status == Paused {
		return false, false
	}

	// Check if the tmux session is still alive
	if !i.TmuxAlive() {
		return false, false
	}

	var content string
	updated, hasPrompt, content = i.tmuxManager.HasUpdated()

	// Update timestamps when content has actually changed.
	// HasUpdated returns the already-captured content, so no second CapturePaneContent call needed.
	if updated && content != "" {
		i.UpdateTerminalTimestamps(content, false)
	}

	return updated, hasPrompt
}

// TapEnter sends an enter key press to the tmux session if AutoYes is enabled.
func (i *Instance) TapEnter() {
	if !i.started || !i.AutoYes {
		return
	}
	if err := i.tmuxManager.TapEnter(); err != nil {
		log.Error("error tapping enter", "err", err)
	}
}

// Attach attaches to the tmux session and returns a done channel.
func (i *Instance) Attach() (chan struct{}, error) {
	if !i.started {
		return nil, fmt.Errorf("cannot attach instance that has not been started")
	}
	return i.tmuxManager.Attach()
}

// SetPreviewSize sets the detached terminal dimensions for preview rendering.
func (i *Instance) SetPreviewSize(width, height int) error {
	if !i.started || i.Status == Paused {
		return fmt.Errorf("cannot set preview size for instance that has not been started or " +
			"is paused")
	}
	return i.tmuxManager.SetDetachedSize(width, height, i.Title)
}

// trackRestartRate records a restart timestamp and logs a warning when the
// session has restarted more than 5 times in the last 5 minutes (crash loop).
func (i *Instance) trackRestartRate() {
	const window = 5 * time.Minute
	const threshold = 5

	now := time.Now()
	i.restartMu.Lock()
	defer i.restartMu.Unlock()

	i.restartCount++

	// Drop timestamps outside the window.
	cutoff := now.Add(-window)
	kept := i.recentRestartTimes[:0]
	for _, t := range i.recentRestartTimes {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	i.recentRestartTimes = append(kept, now)

	if int64(len(i.recentRestartTimes)) >= threshold {
		log.Warn("restart storm detected, possible crash loop", "session", i.Title, "count", len(i.recentRestartTimes), "window", window.Seconds(), "total", i.restartCount)
	}
}

// TmuxSessionExists reports whether the underlying tmux session is currently alive.
// Used at startup to reconcile stale Stopped status against live tmux sessions.
func (i *Instance) TmuxSessionExists() bool {
	return i.tmuxManager.DoesSessionExist()
}

// TmuxAlive returns true if the tmux session is alive. This is a sanity check before attaching.
func (i *Instance) TmuxAlive() bool {
	if i.Status == Paused || i.Status == Stopped || !i.started || !i.tmuxManager.HasSession() {
		return false
	}
	return i.tmuxManager.IsAlive()
}

// GetPTYReader returns the PTY file handle for the tmux session.
func (i *Instance) GetPTYReader() (*os.File, error) {
	i.stateMutex.RLock()
	defer i.stateMutex.RUnlock()

	if !i.started {
		return nil, fmt.Errorf("session not started")
	}
	return i.tmuxManager.GetPTY()
}

// WriteToPTY writes data to the PTY, sending input to the terminal session.
// This is used for forwarding client input to the tmux session.
func (i *Instance) WriteToPTY(data []byte) (int, error) {
	i.stateMutex.RLock()
	defer i.stateMutex.RUnlock()

	if !i.started {
		return 0, fmt.Errorf("session not started")
	}
	return i.tmuxManager.SendKeys(string(data))
}

// ResizePTY resizes the terminal dimensions.
// This is used when clients resize their terminal windows.
func (i *Instance) ResizePTY(cols, rows int) error {
	i.stateMutex.RLock()
	defer i.stateMutex.RUnlock()

	if !i.started {
		return fmt.Errorf("session not started")
	}
	if err := i.tmuxManager.SetWindowSize(cols, rows); err != nil {
		return fmt.Errorf("failed to resize terminal: %w", err)
	}
	return nil
}

// CapturePaneContent captures the current visible tmux pane content.
// This is a simple wrapper around TmuxSession.CapturePaneContent() for compatibility
// with the terminal WebSocket handlers.
func (i *Instance) CapturePaneContent() (string, error) {
	i.stateMutex.RLock()
	defer i.stateMutex.RUnlock()

	if !i.started || i.Status == Paused {
		return "", fmt.Errorf("session not started or paused")
	}
	return i.tmuxManager.CapturePaneContent()
}

// CapturePaneContentRaw captures pane content with ANSI codes preserved (no line joining).
// Essential for hybrid streaming where cursor positioning codes must be preserved.
func (i *Instance) CapturePaneContentRaw() (string, error) {
	i.stateMutex.RLock()
	defer i.stateMutex.RUnlock()

	if !i.started || i.Status == Paused {
		return "", fmt.Errorf("session not started or paused")
	}

	return i.tmuxManager.CapturePaneContentRaw()
}

// GetCurrentPaneContent captures the current visible tmux pane content.
// Delegates to tmuxManager.CaptureViewport.
func (i *Instance) GetCurrentPaneContent(lines int) (string, error) {
	i.stateMutex.RLock()
	defer i.stateMutex.RUnlock()
	content, err := i.tmuxManager.CaptureViewport(lines)
	if err != nil {
		return "", fmt.Errorf("failed to capture current pane content: %w", err)
	}
	return content, nil
}

// GetPaneCursorPosition gets the current cursor position in the tmux pane.
// Returns cursor X (column) and Y (row) coordinates, both 0-based.
func (i *Instance) GetPaneCursorPosition() (x, y int, err error) {
	i.stateMutex.RLock()
	defer i.stateMutex.RUnlock()
	return i.tmuxManager.GetCursorPosition()
}

// GetPaneDimensions gets the current dimensions of the tmux pane.
// Returns width (columns) and height (rows).
func (i *Instance) GetPaneDimensions() (width, height int, err error) {
	i.stateMutex.RLock()
	defer i.stateMutex.RUnlock()
	return i.tmuxManager.GetPaneDimensions()
}

// GetScrollbackHistory captures scrollback history from tmux using line ranges.
// Uses tmux's native scrollback capabilities instead of stored sequences.
// startLine and endLine follow tmux conventions: negative numbers go back from current position,
// use "-" for the start/end of history.
func (i *Instance) GetScrollbackHistory(startLine, endLine string) (string, error) {
	i.stateMutex.RLock()
	defer i.stateMutex.RUnlock()
	return i.tmuxManager.CapturePaneContentWithOptions(startLine, endLine)
}

// SendPrompt sends a prompt to the tmux session. Delegates to tmuxManager.SendPromptWithEnter.
func (i *Instance) SendPrompt(prompt string) error {
	if !i.started {
		return fmt.Errorf("instance not started")
	}
	return i.tmuxManager.SendPromptWithEnter(prompt)
}

// GetTmuxSession returns the underlying tmux session for direct access.
// Returns nil if the session hasn't been started yet.
func (i *Instance) GetTmuxSession() *tmux.TmuxSession {
	i.stateMutex.RLock()
	defer i.stateMutex.RUnlock()
	return i.tmuxManager.Session()
}

// ---- SessionStreamer delegation methods ----
// These methods satisfy the services.SessionStreamer interface without exposing
// the concrete *tmux.TmuxSession type to the server layer.

// StartControlMode starts the control mode stream on the underlying tmux session.
func (i *Instance) StartControlMode() error {
	return i.tmuxManager.StartControlMode()
}

// StopControlMode stops the control mode stream.
func (i *Instance) StopControlMode() error {
	return i.tmuxManager.StopControlMode()
}

// SubscribeControlModeUpdates returns a subscriber ID and a read-only output channel.
// Returns a pre-closed channel if the tmux session is not available.
func (i *Instance) SubscribeControlModeUpdates() (string, <-chan []byte) {
	return i.tmuxManager.SubscribeToControlModeUpdates()
}

// UnsubscribeControlModeUpdates removes a subscriber by ID.
func (i *Instance) UnsubscribeControlModeUpdates(id string) {
	i.tmuxManager.UnsubscribeFromControlModeUpdates(id)
}

// SetTmuxSession sets the tmux session for testing purposes.
func (i *Instance) SetTmuxSession(session *tmux.TmuxSession) {
	i.tmuxManager.SetSession(session)
	i.started = session != nil
}

// SetWindowSize propagates window size changes to the tmux session.
// This enables proper terminal resizing in environments like IntelliJ where SIGWINCH doesn't work.
func (i *Instance) SetWindowSize(cols, rows int) error {
	if i.tmuxManager.HasSession() {
		return i.tmuxManager.SetWindowSize(cols, rows)
	}
	return nil
}

// RefreshTmuxClient forces the tmux client to refresh, triggering a redraw
// of the process running inside. This is critical after resizing to ensure
// cursor positions and line wrapping are recalculated for the new dimensions.
func (i *Instance) RefreshTmuxClient() error {
	return i.tmuxManager.RefreshClient()
}

// SendKeys sends keys to the tmux session.
func (i *Instance) SendKeys(keys string) error {
	if !i.started || i.Status == Paused {
		return fmt.Errorf("cannot send keys to instance that has not been started or is paused")
	}
	_, err := i.tmuxManager.SendKeys(keys)
	return err
}

// SendInputViaControlMode sends raw bytes through the existing control mode connection,
// avoiding the subprocess spawn overhead and timeout risk of exec.CommandContext.
func (i *Instance) SendInputViaControlMode(ctx context.Context, data []byte) error {
	if !i.started || i.Status == Paused {
		return fmt.Errorf("cannot send input to instance that has not been started or is paused")
	}
	return i.tmuxManager.SendInputViaControlMode(ctx, data)
}

// GetPanePID returns the PID of the foreground process in the tmux pane.
func (i *Instance) GetPanePID() (int32, error) {
	if !i.tmuxManager.DoesSessionExist() {
		return 0, fmt.Errorf("tmux session not alive for '%s'", i.Title)
	}
	return i.tmuxManager.GetPanePID()
}
