package session

// instance_tmux.go contains tmux session creation, terminal I/O, PTY access, and
// control-mode delegation methods. All methods delegate to i.processManager.

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

// pm returns the process manager, lazy-initializing with the default TmuxBackend if nil.
// This mirrors the old embedded-struct behaviour where a zero-value TmuxProcessManager
// was always valid for method calls (session == nil → IsAlive()/HasSession() return false).
// Tests that create bare &Instance{} structs rely on this guarantee.
func (i *Instance) pm() ProcessManager {
	i.pmMu.Lock()
	defer i.pmMu.Unlock()
	if i.processManager == nil {
		i.processManager = NewProcessManager(context.Background(), BackendTmux, ProcessManagerOptions{})
	}
	return i.processManager
}

// GetTmuxSessionName returns the sanitized tmux session name for reconciliation.
// Returns empty string for external or uninitialized sessions.
func (i *Instance) GetTmuxSessionName() string {
	return i.pm().GetSessionIdentifier()
}

// buildLaunchCommand constructs the final command string used to launch the program
// in tmux, incorporating Claude session resume flags, MCP server URL, and prompt.
func (i *Instance) buildLaunchCommand(claudeSessionID string) string {
	program := i.Program
	if claudeSessionID != "" && strings.Contains(program, "claude") {
		program = fmt.Sprintf("%s --resume %s", program, claudeSessionID)
	}
	if i.MCPServerURL != "" && strings.Contains(program, "claude") {
		var mcpFlag string
		if i.UUID != "" {
			mcpFlag = fmt.Sprintf(`--mcp-config '{"mcpServers":{"stapler-squad":{"type":"http","url":%q,"headers":{"X-Stapler-Session-UUID":%q}}}}'`, i.MCPServerURL, i.UUID)
		} else {
			mcpFlag = fmt.Sprintf(`--mcp-config '{"mcpServers":{"stapler-squad":{"type":"http","url":%q}}}'`, i.MCPServerURL)
		}
		program = program + " " + mcpFlag
	}
	if i.AppendSystemPrompt != "" && strings.Contains(program, "claude") {
		program = fmt.Sprintf("%s --append-system-prompt %q", program, i.AppendSystemPrompt)
	}
	if i.AllowedTools != "" && strings.Contains(program, "claude") {
		program = fmt.Sprintf("%s --allowedTools %q", program, i.AllowedTools)
	}
	if i.PermissionMode != "" && strings.Contains(program, "claude") {
		program = fmt.Sprintf("%s --permission-mode %q", program, i.PermissionMode)
	}
	if i.AutoYes && strings.Contains(program, "claude") {
		program = program + " --dangerously-skip-permissions"
	}
	if i.OneShot && strings.Contains(program, "claude") {
		program = program + " -p --output-format json"
	}
	if i.Prompt != "" && (claudeSessionID == "" || i.OneShot) && strings.Contains(program, "claude") {
		program = fmt.Sprintf("%s %q", program, i.Prompt)
	}
	if i.CLIFlags != "" {
		program = program + " " + i.CLIFlags
	}
	return program
}

// initTmuxSession creates (or reuses) the tmux.TmuxSession object without starting it.
func (i *Instance) initTmuxSession() {
	if i.pm().HasSession() {
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
	// Collect UUID sentinel and alias env vars into one SetExtraEnv call —
	// SetExtraEnv is an assignment, so multiple calls overwrite each other.
	var extraEnv []string
	if i.UUID != "" {
		extraEnv = append(extraEnv, "STAPLER_SESSION_UUID="+i.UUID)
	}
	for k, v := range i.EnvVars {
		extraEnv = append(extraEnv, fmt.Sprintf("%s=%s", k, v))
	}
	if len(extraEnv) > 0 {
		session.SetExtraEnv(extraEnv)
	}
	if tb, ok := i.processManager.(*TmuxBackend); ok {
		tb.TmuxManager().SetSession(session)
	}
}

// KillSession terminates the tmux session only (leaves worktree intact).
func (i *Instance) KillSession() error {
	if i.pm().HasSession() {
		if err := i.pm().Close(); err != nil {
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
// This only works for external sessions that were started via ssq-mux with tmux integration.
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
	updated, hasPrompt, content = i.pm().HasUpdated()

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
	if err := i.pm().TapEnter(); err != nil {
		log.Error("error tapping enter", "err", err)
	}
}

// Attach attaches to the tmux session and returns a done channel.
func (i *Instance) Attach() (chan struct{}, error) {
	if !i.started {
		return nil, fmt.Errorf("cannot attach instance that has not been started")
	}
	return i.pm().Attach()
}

// SetPreviewSize sets the detached terminal dimensions for preview rendering.
func (i *Instance) SetPreviewSize(width, height int) error {
	if !i.started || i.Status == Paused {
		return fmt.Errorf("cannot set preview size for instance that has not been started or " +
			"is paused")
	}
	return i.pm().SetDetachedSize(width, height, i.Title)
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
	return i.pm().IsAlive()
}

// TmuxAlive returns true if the tmux session is alive. This is a sanity check before attaching.
func (i *Instance) TmuxAlive() bool {
	if i.Status == Paused || i.Status == Stopped || !i.started || !i.pm().HasSession() {
		return false
	}
	return i.pm().IsAlive()
}

// GetPTYReader returns the PTY file handle for the tmux session.
func (i *Instance) GetPTYReader() (*os.File, error) {
	i.stateMutex.RLock()
	defer i.stateMutex.RUnlock()

	if !i.started {
		return nil, fmt.Errorf("session not started")
	}
	return i.pm().GetPTY()
}

// WriteToPTY writes data to the PTY, sending input to the terminal session.
// This is used for forwarding client input to the tmux session.
func (i *Instance) WriteToPTY(data []byte) (int, error) {
	i.stateMutex.RLock()
	defer i.stateMutex.RUnlock()

	if !i.started {
		return 0, fmt.Errorf("session not started")
	}
	return i.pm().SendKeys(string(data))
}

// ResizePTY resizes the terminal dimensions.
// This is used when clients resize their terminal windows.
func (i *Instance) ResizePTY(cols, rows int) error {
	i.stateMutex.RLock()
	defer i.stateMutex.RUnlock()

	if !i.started {
		return fmt.Errorf("session not started")
	}
	if err := i.pm().SetWindowSize(cols, rows); err != nil {
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
	return i.pm().CapturePaneContent()
}

// CapturePaneContentRaw captures pane content with ANSI codes preserved (no line joining).
// Essential for hybrid streaming where cursor positioning codes must be preserved.
func (i *Instance) CapturePaneContentRaw() (string, error) {
	i.stateMutex.RLock()
	defer i.stateMutex.RUnlock()

	if !i.started || i.Status == Paused {
		return "", fmt.Errorf("session not started or paused")
	}

	return i.pm().CapturePaneContentRaw()
}

// GetCurrentPaneContent captures the current visible tmux pane content.
// Delegates to processManager.CaptureViewport.
func (i *Instance) GetCurrentPaneContent(lines int) (string, error) {
	i.stateMutex.RLock()
	defer i.stateMutex.RUnlock()
	content, err := i.pm().CaptureViewport(lines)
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
	return i.pm().GetCursorPosition()
}

// GetPaneDimensions gets the current dimensions of the tmux pane.
// Returns width (columns) and height (rows).
func (i *Instance) GetPaneDimensions() (width, height int, err error) {
	i.stateMutex.RLock()
	defer i.stateMutex.RUnlock()
	return i.pm().GetPaneDimensions()
}

// GetScrollbackHistory captures scrollback history from tmux using line ranges.
// Uses tmux's native scrollback capabilities instead of stored sequences.
// startLine and endLine follow tmux conventions: negative numbers go back from current position,
// use "-" for the start/end of history.
func (i *Instance) GetScrollbackHistory(startLine, endLine string) (string, error) {
	i.stateMutex.RLock()
	defer i.stateMutex.RUnlock()
	return i.pm().CapturePaneContentWithOptions(startLine, endLine)
}

// SendPrompt sends a prompt to the tmux session. Delegates to processManager.SendPromptWithEnter.
func (i *Instance) SendPrompt(prompt string) error {
	if !i.started {
		return fmt.Errorf("instance not started")
	}
	return i.pm().SendPromptWithEnter(prompt)
}

// GetTmuxSession returns the underlying tmux session for direct access.
// Returns nil if the session hasn't been started yet or if the backend is not tmux.
func (i *Instance) GetTmuxSession() *tmux.TmuxSession {
	i.stateMutex.RLock()
	defer i.stateMutex.RUnlock()
	tb, ok := i.processManager.(*TmuxBackend)
	if !ok {
		return nil
	}
	return tb.TmuxManager().Session()
}

// ---- SessionStreamer delegation methods ----
// These methods satisfy the services.SessionStreamer interface without exposing
// the concrete *tmux.TmuxSession type to the server layer.

// StartControlMode starts the control mode stream on the underlying tmux session.
func (i *Instance) StartControlMode() error {
	return i.pm().StartControlMode()
}

// StopControlMode stops the control mode stream.
func (i *Instance) StopControlMode() error {
	return i.pm().StopControlMode()
}

// SubscribeControlModeUpdates returns a subscriber ID and a read-only output channel.
// Returns a pre-closed channel if the tmux session is not available.
func (i *Instance) SubscribeControlModeUpdates() (string, <-chan []byte) {
	return i.pm().SubscribeToControlModeUpdates()
}

// UnsubscribeControlModeUpdates removes a subscriber by ID.
func (i *Instance) UnsubscribeControlModeUpdates(id string) {
	i.pm().UnsubscribeFromControlModeUpdates(id)
}

// SetTmuxSession sets the tmux session for testing purposes.
func (i *Instance) SetTmuxSession(session *tmux.TmuxSession) {
	if tb, ok := i.processManager.(*TmuxBackend); ok {
		tb.TmuxManager().SetSession(session)
	}
	i.started = session != nil
}

// SetWindowSize propagates window size changes to the tmux session.
// This enables proper terminal resizing in environments like IntelliJ where SIGWINCH doesn't work.
func (i *Instance) SetWindowSize(cols, rows int) error {
	if i.pm().HasSession() {
		return i.pm().SetWindowSize(cols, rows)
	}
	return nil
}

// RefreshTmuxClient forces the tmux client to refresh, triggering a redraw
// of the process running inside. This is critical after resizing to ensure
// cursor positions and line wrapping are recalculated for the new dimensions.
func (i *Instance) RefreshTmuxClient() error {
	return i.pm().RefreshClient()
}

// SendKeys sends keys to the tmux session.
func (i *Instance) SendKeys(keys string) error {
	if !i.started || i.Status == Paused {
		return fmt.Errorf("cannot send keys to instance that has not been started or is paused")
	}
	_, err := i.pm().SendKeys(keys)
	return err
}

// SendInputViaControlMode sends raw bytes through the existing control mode connection,
// avoiding the subprocess spawn overhead and timeout risk of exec.CommandContext.
func (i *Instance) SendInputViaControlMode(ctx context.Context, data []byte) error {
	if !i.started || i.Status == Paused {
		return fmt.Errorf("cannot send input to instance that has not been started or is paused")
	}
	return i.pm().SendInputViaControlMode(ctx, data)
}

// GetPanePID returns the PID of the foreground process in the tmux pane.
// The DoesSessionExist guard is omitted here: TmuxSession.GetPanePID already uses
// the CM fast path (no subprocess) and falls back to display-message which returns
// an error if the session is gone. Avoiding a separate list-sessions call per instance
// prevents N concurrent tmux list-sessions subprocesses during HistoryLinker.ScanAll.
func (i *Instance) GetPanePID() (int32, error) {
	return i.pm().GetPanePID()
}
