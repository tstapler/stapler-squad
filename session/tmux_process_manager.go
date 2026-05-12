package session

import (
	"context"
	"fmt"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/tmux"
	"os"
	"strings"
	"time"
)

// TmuxProcessManager owns the tmux session and preview-size tracking state that
// were previously scattered as bare fields on Instance.
//
// Instance keeps thin wrapper methods (with started/paused guards) that delegate
// here.  TmuxProcessManager itself has no knowledge of Instance lifecycle; it
// only manages the tmux session and the preview-resize bookkeeping.
type TmuxProcessManager struct {
	session *tmux.TmuxSession

	// Preview size tracking — avoid sending redundant resize commands.
	lastPreviewWidth   int
	lastPreviewHeight  int
	lastPTYWarningTime time.Time
}

// HasSession reports whether a tmux session has been initialized.
func (tm *TmuxProcessManager) HasSession() bool {
	return tm.session != nil
}

// Session returns the underlying tmux session (may be nil before Start).
func (tm *TmuxProcessManager) Session() *tmux.TmuxSession {
	return tm.session
}

// SetSession replaces the underlying tmux session.  Used by tests and by
// Instance.start() when reusing a pre-created session.
func (tm *TmuxProcessManager) SetSession(s *tmux.TmuxSession) {
	tm.session = s
}

// GetTmuxSessionName returns the sanitized tmux session name for reconciliation.
// Returns empty string when no session has been initialized.
func (tm *TmuxProcessManager) GetTmuxSessionName() string {
	if tm.session == nil {
		return ""
	}
	return tm.session.GetSanitizedName()
}

// IsAlive reports whether the tmux session process is still running.
func (tm *TmuxProcessManager) IsAlive() bool {
	if tm.session == nil {
		return false
	}
	return tm.session.DoesSessionExist()
}

// Close terminates the tmux session.
func (tm *TmuxProcessManager) Close() error {
	if tm.session == nil {
		return nil
	}
	if err := tm.session.Close(); err != nil {
		return fmt.Errorf("failed to close tmux session: %w", err)
	}
	return nil
}

// DetachSafely detaches the current tmux client from the session without closing it.
func (tm *TmuxProcessManager) DetachSafely() error {
	if tm.session == nil {
		return nil
	}
	return tm.session.DetachSafely()
}

// DoesSessionExist returns true if the tmux session name is registered with the server.
func (tm *TmuxProcessManager) DoesSessionExist() bool {
	if tm.session == nil {
		return false
	}
	return tm.session.DoesSessionExist()
}

// SetDetachedSize updates the tmux window dimensions without attaching.
// Rate-limits PTY-not-initialized warnings to avoid log spam.
func (tm *TmuxProcessManager) SetDetachedSize(width, height int, instanceTitle string) error {
	if tm.session == nil {
		return fmt.Errorf("tmux session not initialized")
	}

	// Skip resize if dimensions haven't changed.
	if width == tm.lastPreviewWidth && height == tm.lastPreviewHeight {
		return nil
	}

	if err := tm.session.SetDetachedSize(width, height); err != nil {
		if strings.Contains(err.Error(), "PTY is not initialized") {
			// Rate-limit: warn at most once per 30 seconds per instance.
			if time.Since(tm.lastPTYWarningTime) > 30*time.Second {
				log.Warn("PTY not ready for instance, skipping resize", "session", instanceTitle, "err", err)
				tm.lastPTYWarningTime = time.Now()
			}
			return nil // Not fatal — don't disrupt the UI.
		}
		return err
	}

	tm.lastPreviewWidth = width
	tm.lastPreviewHeight = height
	return nil
}

// Attach returns a channel that closes when the user detaches from the session.
func (tm *TmuxProcessManager) Attach() (chan struct{}, error) {
	if tm.session == nil {
		return nil, fmt.Errorf("tmux session not initialized")
	}
	return tm.session.Attach()
}

// CapturePaneContent returns the current visible pane content.
func (tm *TmuxProcessManager) CapturePaneContent() (string, error) {
	if tm.session == nil {
		return "", fmt.Errorf("tmux session not initialized")
	}
	return tm.session.CapturePaneContent()
}

// CapturePaneContentRaw returns pane content with ANSI escape codes preserved.
func (tm *TmuxProcessManager) CapturePaneContentRaw() (string, error) {
	if tm.session == nil {
		return "", fmt.Errorf("tmux session not initialized")
	}
	return tm.session.CapturePaneContentRaw()
}

// CapturePaneContentWithOptions captures pane content between startLine and endLine.
func (tm *TmuxProcessManager) CapturePaneContentWithOptions(startLine, endLine string) (string, error) {
	if tm.session == nil {
		return "", fmt.Errorf("tmux session not initialized")
	}
	return tm.session.CapturePaneContentWithOptions(startLine, endLine)
}

// GetPaneDimensions returns the current pane width and height.
func (tm *TmuxProcessManager) GetPaneDimensions() (width, height int, err error) {
	if tm.session == nil {
		return 0, 0, fmt.Errorf("tmux session not initialized")
	}
	return tm.session.GetPaneDimensions()
}

// GetCursorPosition returns the current cursor column and row (0-based).
func (tm *TmuxProcessManager) GetCursorPosition() (x, y int, err error) {
	if tm.session == nil {
		return 0, 0, fmt.Errorf("tmux session not initialized")
	}
	return tm.session.GetCursorPosition()
}

// GetPTY returns the PTY master file for reading terminal output.
func (tm *TmuxProcessManager) GetPTY() (*os.File, error) {
	if tm.session == nil {
		return nil, fmt.Errorf("tmux session not initialized")
	}
	return tm.session.GetPTY()
}

// SendKeys sends a string of keys to the tmux session and returns the number of bytes written.
func (tm *TmuxProcessManager) SendKeys(keys string) (int, error) {
	if tm.session == nil {
		return 0, fmt.Errorf("tmux session not initialized")
	}
	return tm.session.SendKeys(keys)
}

// SendInputViaControlMode sends raw bytes through the existing control mode connection.
func (tm *TmuxProcessManager) SendInputViaControlMode(ctx context.Context, data []byte) error {
	if tm.session == nil {
		return fmt.Errorf("tmux session not initialized")
	}
	return tm.session.SendInputViaControlMode(ctx, data)
}

// SetWindowSize resizes the tmux window to the given columns and rows.
func (tm *TmuxProcessManager) SetWindowSize(cols, rows int) error {
	if tm.session == nil {
		return nil
	}
	return tm.session.SetWindowSize(cols, rows)
}

// RefreshClient forces the tmux client to redraw.
func (tm *TmuxProcessManager) RefreshClient() error {
	if tm.session == nil {
		return nil
	}
	return tm.session.RefreshClient()
}

// TapEnter sends an Enter key to the session.
func (tm *TmuxProcessManager) TapEnter() error {
	if tm.session == nil {
		return fmt.Errorf("tmux session not initialized")
	}
	return tm.session.TapEnter()
}

// HasUpdated reports whether the pane content has changed since the last check.
func (tm *TmuxProcessManager) HasUpdated() (updated bool, hasPrompt bool, content string) {
	if tm.session == nil {
		return false, false, ""
	}
	return tm.session.HasUpdated()
}

// RestoreWithWorkDir re-attaches to an existing session in the given directory.
func (tm *TmuxProcessManager) RestoreWithWorkDir(workDir string) error {
	if tm.session == nil {
		return fmt.Errorf("tmux session not initialized")
	}
	return tm.session.RestoreWithWorkDir(workDir)
}

// Start creates and starts the tmux session in the given directory.
func (tm *TmuxProcessManager) Start(dir string) error {
	if tm.session == nil {
		return fmt.Errorf("tmux session not initialized")
	}
	return tm.session.Start(dir)
}

// FilterBanners strips banner/header content from terminal output.
func (tm *TmuxProcessManager) FilterBanners(content string) (string, int) {
	if tm.session == nil {
		return content, 0 // No-op without a session.
	}
	return tm.session.FilterBanners(content)
}

// HasMeaningfulContent reports whether the terminal output contains substantive content.
func (tm *TmuxProcessManager) HasMeaningfulContent(content string) bool {
	if tm.session == nil {
		return false
	}
	return tm.session.HasMeaningfulContent(content)
}

// CaptureViewport captures the last N lines of the pane.
// If lines <= 0, captures the current viewport height.
func (tm *TmuxProcessManager) CaptureViewport(lines int) (string, error) {
	if tm.session == nil {
		return "", fmt.Errorf("tmux session not initialized")
	}
	if lines <= 0 {
		_, height, err := tm.session.GetPaneDimensions()
		if err != nil {
			lines = 40 // Fallback
		} else {
			lines = height
		}
	}
	startLine := fmt.Sprintf("-%d", lines)
	return tm.session.CapturePaneContentWithOptions(startLine, "-")
}

// SendPromptWithEnter sends text to the session followed by Enter key.
// Includes a brief pause between text and Enter to prevent interpretation issues.
func (tm *TmuxProcessManager) SendPromptWithEnter(prompt string) error {
	if tm.session == nil {
		return fmt.Errorf("tmux session not initialized")
	}
	if _, err := tm.session.SendKeys(prompt); err != nil {
		return fmt.Errorf("error sending keys to tmux session: %w", err)
	}
	time.Sleep(100 * time.Millisecond)
	if err := tm.session.TapEnter(); err != nil {
		return fmt.Errorf("error tapping enter: %w", err)
	}
	return nil
}

// GetPanePID returns the PID of the foreground process in the pane.
// Returns an error if the session is not initialized or the command fails.
func (tm *TmuxProcessManager) GetPanePID() (int32, error) {
	if tm.session == nil {
		return 0, fmt.Errorf("tmux session not initialized")
	}
	return tm.session.GetPanePID()
}

// SetOnExitCallback registers a callback that fires when the tmux session exits
// unexpectedly. No-op if no session is initialized.
func (tm *TmuxProcessManager) SetOnExitCallback(fn func(string)) {
	if tm.session == nil {
		return
	}
	tm.session.SetOnExitCallback(fn)
}

// ResetExitOnce resets the sync.Once guard so that the exit callback can fire
// again on the next start cycle (e.g., after a restart). No-op if no session.
func (tm *TmuxProcessManager) ResetExitOnce() {
	if tm.session == nil {
		return
	}
	tm.session.ResetExitOnce()
}

// StartControlMode starts the tmux control mode stream.
// Returns nil if no session is initialized.
func (tm *TmuxProcessManager) StartControlMode() error {
	if tm.session == nil {
		return nil
	}
	return tm.session.StartControlMode()
}

// StopControlMode stops the tmux control mode stream.
// Returns nil if no session is initialized.
func (tm *TmuxProcessManager) StopControlMode() error {
	if tm.session == nil {
		return nil
	}
	return tm.session.StopControlMode()
}

// SubscribeToControlModeUpdates registers a new subscriber for real-time terminal output.
// Returns a pre-closed channel if no session is initialized.
func (tm *TmuxProcessManager) SubscribeToControlModeUpdates() (string, chan []byte) {
	if tm.session == nil {
		ch := make(chan []byte)
		close(ch)
		return "", ch
	}
	return tm.session.SubscribeToControlModeUpdates()
}

// UnsubscribeFromControlModeUpdates removes a subscriber by ID.
// No-op if no session is initialized.
func (tm *TmuxProcessManager) UnsubscribeFromControlModeUpdates(id string) {
	if tm.session == nil {
		return
	}
	tm.session.UnsubscribeFromControlModeUpdates(id)
}

// TmuxManager is the interface satisfied by *TmuxProcessManager.
// It covers all tmux session operations used by Instance and can be implemented
// by test doubles to avoid requiring a real tmux server.
type TmuxManager interface {
	HasSession() bool
	Session() *tmux.TmuxSession
	SetSession(*tmux.TmuxSession)
	GetTmuxSessionName() string
	IsAlive() bool
	Close() error
	DetachSafely() error
	DoesSessionExist() bool
	SetDetachedSize(width, height int, instanceTitle string) error
	Attach() (chan struct{}, error)
	CapturePaneContent() (string, error)
	CapturePaneContentRaw() (string, error)
	CapturePaneContentWithOptions(startLine, endLine string) (string, error)
	GetPaneDimensions() (width, height int, err error)
	GetCursorPosition() (x, y int, err error)
	GetPTY() (*os.File, error)
	SendKeys(keys string) (int, error)
	SetWindowSize(cols, rows int) error
	RefreshClient() error
	TapEnter() error
	HasUpdated() (updated bool, hasPrompt bool, content string)
	RestoreWithWorkDir(workDir string) error
	Start(dir string) error
	FilterBanners(content string) (string, int)
	HasMeaningfulContent(content string) bool
	CaptureViewport(lines int) (string, error)
	SendPromptWithEnter(prompt string) error
	GetPanePID() (int32, error)
	SetOnExitCallback(fn func(string))
	ResetExitOnce()
	StartControlMode() error
	StopControlMode() error
	SubscribeToControlModeUpdates() (string, chan []byte)
	UnsubscribeFromControlModeUpdates(id string)
}

// compile-time check that *TmuxProcessManager satisfies TmuxManager.
var _ TmuxManager = (*TmuxProcessManager)(nil)
