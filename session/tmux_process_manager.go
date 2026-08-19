package session

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/tmux"
)

// capturePaneCacheTTL is the TTL for the CapturePaneContent result cache.
// Avoids spawning a tmux subprocess on every poll tick.
const capturePaneCacheTTL = time.Second

// TmuxProcessManager owns the tmux session and preview-size tracking state that
// were previously scattered as bare fields on Instance.
//
// Instance keeps thin wrapper methods (with started/paused guards) that delegate
// here.  TmuxProcessManager itself has no knowledge of Instance lifecycle; it
// only manages the tmux session and the preview-resize bookkeeping.
type TmuxProcessManager struct {
	// session is written once (or rarely) via SetSession and read on every method call.
	// atomic.Pointer eliminates the ~30 RLock/RUnlock pairs that existed when this
	// was a plain pointer guarded by mu.
	session atomic.Pointer[tmux.TmuxSession]

	// mu guards only the capture-pane cache and preview-resize fields below.
	mu sync.RWMutex

	// Preview size tracking — avoid sending redundant resize commands.
	lastPreviewWidth   int
	lastPreviewHeight  int
	lastPTYWarningTime time.Time

	// Capture-pane cache — avoids subprocess on every poll tick.
	captureContent   string
	captureContentAt time.Time

	// panePID caches the foreground PID after first successful lookup (stable per pane).
	panePIDCached atomic.Int32
	panePIDSet    atomic.Bool
}

// HasSession reports whether a tmux session has been initialized.
func (tm *TmuxProcessManager) HasSession() bool {
	return tm.session.Load() != nil
}

// Session returns the underlying tmux session (may be nil before Start).
func (tm *TmuxProcessManager) Session() *tmux.TmuxSession {
	return tm.session.Load()
}

// SetSession replaces the underlying tmux session.  Used by tests and by
// Instance.start() when reusing a pre-created session.
func (tm *TmuxProcessManager) SetSession(s *tmux.TmuxSession) {
	tm.session.Store(s)
	// Invalidate the capture-pane cache and PID cache under mu.
	tm.mu.Lock()
	tm.captureContentAt = time.Time{}
	tm.mu.Unlock()
	tm.panePIDSet.Store(false)
}

// GetTmuxSessionName returns the sanitized tmux session name for reconciliation.
// Returns empty string when no session has been initialized.
func (tm *TmuxProcessManager) GetTmuxSessionName() string {
	s := tm.session.Load()
	if s == nil {
		return ""
	}
	return s.GetSanitizedName()
}

// IsAlive reports whether the tmux session process is still running.
func (tm *TmuxProcessManager) IsAlive() bool {
	s := tm.session.Load()
	if s == nil {
		return false
	}
	return s.DoesSessionExist()
}

// PaneExitStatus reports the wrapped program's exit code/signal for a dead
// pane whose tmux session is otherwise still alive (remain-on-exit keeps the
// placeholder pane around after the wrapped program exits/is killed). Returns
// dead=false if there is no session, or the pane is still running.
func (tm *TmuxProcessManager) PaneExitStatus() (code int, signal string, dead bool) {
	s := tm.session.Load()
	if s == nil {
		return 0, "", false
	}
	return s.ExitStatus()
}

// Close terminates the tmux session.
func (tm *TmuxProcessManager) Close() error {
	s := tm.session.Load()
	if s == nil {
		return nil
	}
	if err := s.Close(); err != nil {
		return fmt.Errorf("failed to close tmux session: %w", err)
	}
	return nil
}

// DetachSafely detaches the current tmux client from the session without closing it.
func (tm *TmuxProcessManager) DetachSafely() error {
	s := tm.session.Load()
	if s == nil {
		return nil
	}
	return s.DetachSafely()
}

// DoesSessionExist returns true if the tmux session name is registered with the server.
func (tm *TmuxProcessManager) DoesSessionExist() bool {
	s := tm.session.Load()
	if s == nil {
		return false
	}
	return s.DoesSessionExist()
}

// SetDetachedSize updates the tmux window dimensions without attaching.
// Rate-limits PTY-not-initialized warnings to avoid log spam.
func (tm *TmuxProcessManager) SetDetachedSize(width, height int, instanceTitle string) error {
	s := tm.session.Load()
	if s == nil {
		return fmt.Errorf("tmux session not initialized")
	}
	tm.mu.Lock()
	if width == tm.lastPreviewWidth && height == tm.lastPreviewHeight {
		tm.mu.Unlock()
		return nil
	}
	tm.mu.Unlock()
	if err := s.SetDetachedSize(width, height); err != nil {
		if strings.Contains(err.Error(), "PTY is not initialized") {
			tm.mu.Lock()
			if time.Since(tm.lastPTYWarningTime) > 30*time.Second {
				log.Warn("PTY not ready for instance, skipping resize", "session", instanceTitle, "err", err)
				tm.lastPTYWarningTime = time.Now()
			}
			tm.mu.Unlock()
			return nil
		}
		return err
	}
	tm.mu.Lock()
	tm.lastPreviewWidth = width
	tm.lastPreviewHeight = height
	tm.mu.Unlock()
	return nil
}

// Attach returns a channel that closes when the user detaches from the session.
func (tm *TmuxProcessManager) Attach() (chan struct{}, error) {
	s := tm.session.Load()
	if s == nil {
		return nil, fmt.Errorf("tmux session not initialized")
	}
	return s.Attach()
}

// CapturePaneContent returns the current visible pane content.
// Results are cached for capturePaneCacheTTL to reduce subprocess/forkLock
// contention when called per-session on every poll tick.
func (tm *TmuxProcessManager) CapturePaneContent() (string, error) {
	// Fast path: serve from cache if fresh.
	tm.mu.RLock()
	if time.Since(tm.captureContentAt) < capturePaneCacheTTL {
		cached := tm.captureContent
		tm.mu.RUnlock()
		return cached, nil
	}
	tm.mu.RUnlock()

	s := tm.session.Load()
	if s == nil {
		return "", fmt.Errorf("tmux session not initialized")
	}
	content, err := s.CapturePaneContent()
	if err != nil {
		return "", err
	}

	tm.mu.Lock()
	tm.captureContent = content
	tm.captureContentAt = time.Now()
	tm.mu.Unlock()
	return content, nil
}

// CapturePaneContentPriority mirrors CapturePaneContent but routes the
// subprocess call through the resync exec-gate fast lane instead of the
// default pool (Epic 4.2, terminal:resync-exec-gate-fast-lane), and always
// captures fresh — it deliberately bypasses the capturePaneCacheTTL cache
// since resync's whole point is an up-to-date snapshot, not the cached value
// a concurrent poll tick may have populated.
func (tm *TmuxProcessManager) CapturePaneContentPriority() (string, error) {
	s := tm.session.Load()
	if s == nil {
		return "", fmt.Errorf("tmux session not initialized")
	}
	content, err := s.CapturePaneContentPriority()
	if err != nil {
		return "", err
	}

	tm.mu.Lock()
	tm.captureContent = content
	tm.captureContentAt = time.Now()
	tm.mu.Unlock()
	return content, nil
}

// CapturePaneContentRaw returns pane content with ANSI escape codes preserved.
func (tm *TmuxProcessManager) CapturePaneContentRaw() (string, error) {
	s := tm.session.Load()
	if s == nil {
		return "", fmt.Errorf("tmux session not initialized")
	}
	return s.CapturePaneContentRaw()
}

// CapturePaneContentWithOptions captures pane content between startLine and endLine.
func (tm *TmuxProcessManager) CapturePaneContentWithOptions(startLine, endLine string) (string, error) {
	s := tm.session.Load()
	if s == nil {
		return "", fmt.Errorf("tmux session not initialized")
	}
	return s.CapturePaneContentWithOptions(startLine, endLine)
}

// GetPaneDimensions returns the current pane width and height.
func (tm *TmuxProcessManager) GetPaneDimensions() (width, height int, err error) {
	s := tm.session.Load()
	if s == nil {
		return 0, 0, fmt.Errorf("tmux session not initialized")
	}
	return s.GetPaneDimensions()
}

// GetCursorPosition returns the current cursor column and row (0-based).
func (tm *TmuxProcessManager) GetCursorPosition() (x, y int, err error) {
	s := tm.session.Load()
	if s == nil {
		return 0, 0, fmt.Errorf("tmux session not initialized")
	}
	return s.GetCursorPosition()
}

// GetPTY returns the PTY master file for reading terminal output.
func (tm *TmuxProcessManager) GetPTY() (*os.File, error) {
	s := tm.session.Load()
	if s == nil {
		return nil, fmt.Errorf("tmux session not initialized")
	}
	return s.GetPTY()
}

// SendKeys sends a string of keys to the tmux session and returns the number of bytes written.
func (tm *TmuxProcessManager) SendKeys(keys string) (int, error) {
	s := tm.session.Load()
	if s == nil {
		return 0, fmt.Errorf("tmux session not initialized")
	}
	return s.SendKeys(keys)
}

// SendInputViaControlMode sends raw bytes through the existing control mode connection.
func (tm *TmuxProcessManager) SendInputViaControlMode(ctx context.Context, data []byte) error {
	s := tm.session.Load()
	if s == nil {
		return fmt.Errorf("tmux session not initialized")
	}
	return s.SendInputViaControlMode(ctx, data)
}

// SetWindowSize resizes the tmux window to the given columns and rows.
func (tm *TmuxProcessManager) SetWindowSize(cols, rows int) error {
	s := tm.session.Load()
	if s == nil {
		return nil
	}
	return s.SetWindowSize(cols, rows)
}

// RefreshClient forces the tmux client to redraw.
func (tm *TmuxProcessManager) RefreshClient() error {
	s := tm.session.Load()
	if s == nil {
		return nil
	}
	return s.RefreshClient()
}

// RefreshClientPriority mirrors RefreshClient but routes the subprocess call
// through the resync exec-gate fast lane instead of the default pool
// (Epic 4.2, terminal:resync-exec-gate-fast-lane).
func (tm *TmuxProcessManager) RefreshClientPriority() error {
	s := tm.session.Load()
	if s == nil {
		return nil
	}
	return s.RefreshClientPriority()
}

// TapEnter sends an Enter key to the session.
func (tm *TmuxProcessManager) TapEnter() error {
	s := tm.session.Load()
	if s == nil {
		return fmt.Errorf("tmux session not initialized")
	}
	return s.TapEnter()
}

// HasUpdated reports whether the pane content has changed since the last check.
func (tm *TmuxProcessManager) HasUpdated() (updated bool, hasPrompt bool, content string) {
	s := tm.session.Load()
	if s == nil {
		return false, false, ""
	}
	return s.HasUpdated()
}

// RestoreWithWorkDir re-attaches to an existing session in the given directory.
func (tm *TmuxProcessManager) RestoreWithWorkDir(workDir string) error {
	s := tm.session.Load()
	if s == nil {
		return fmt.Errorf("tmux session not initialized")
	}
	return s.RestoreWithWorkDir(workDir)
}

// Start creates and starts the tmux session in the given directory.
func (tm *TmuxProcessManager) Start(dir string) error {
	s := tm.session.Load()
	if s == nil {
		return fmt.Errorf("tmux session not initialized")
	}
	return s.Start(dir)
}

// FilterBanners strips banner/header content from terminal output.
func (tm *TmuxProcessManager) FilterBanners(content string) (string, int) {
	s := tm.session.Load()
	if s == nil {
		return content, 0
	}
	return s.FilterBanners(content)
}

// HasMeaningfulContent reports whether the terminal output contains substantive content.
func (tm *TmuxProcessManager) HasMeaningfulContent(content string) bool {
	s := tm.session.Load()
	if s == nil {
		return false
	}
	return s.HasMeaningfulContent(content)
}

// CaptureViewport captures the last N lines of the pane.
// If lines <= 0, captures the current viewport height.
func (tm *TmuxProcessManager) CaptureViewport(lines int) (string, error) {
	s := tm.session.Load()
	if s == nil {
		return "", fmt.Errorf("tmux session not initialized")
	}
	if lines <= 0 {
		_, height, err := s.GetPaneDimensions()
		if err != nil {
			lines = 40
		} else {
			lines = height
		}
	}
	startLine := fmt.Sprintf("-%d", lines)
	return s.CapturePaneContentWithOptions(startLine, "-")
}

// SendPromptWithEnter sends text to the session followed by Enter key.
// Includes a brief pause between text and Enter to prevent interpretation issues.
func (tm *TmuxProcessManager) SendPromptWithEnter(prompt string) error {
	s := tm.session.Load()
	if s == nil {
		return fmt.Errorf("tmux session not initialized")
	}
	if _, err := s.SendKeys(prompt); err != nil {
		return fmt.Errorf("error sending keys to tmux session: %w", err)
	}
	time.Sleep(100 * time.Millisecond)
	if err := s.TapEnter(); err != nil {
		return fmt.Errorf("error tapping enter: %w", err)
	}
	return nil
}

// GetPanePID returns the PID of the foreground process in the pane.
// The pane PID is stable for the lifetime of a tmux pane, so the result is
// cached after the first successful lookup to avoid repeated subprocess calls.
func (tm *TmuxProcessManager) GetPanePID() (int32, error) {
	if tm.panePIDSet.Load() {
		return tm.panePIDCached.Load(), nil
	}
	s := tm.session.Load()
	if s == nil {
		return 0, fmt.Errorf("tmux session not initialized")
	}
	pid, err := s.GetPanePID()
	if err != nil {
		return 0, err
	}
	tm.panePIDCached.Store(pid)
	tm.panePIDSet.Store(true)
	return pid, nil
}

// SetOnExitCallback registers a callback that fires when the tmux session exits
// unexpectedly. No-op if no session is initialized.
func (tm *TmuxProcessManager) SetOnExitCallback(fn func(string)) {
	s := tm.session.Load()
	if s == nil {
		return
	}
	s.SetOnExitCallback(fn)
}

// ResetExitOnce resets the sync.Once guard so that the exit callback can fire
// again on the next start cycle (e.g., after a restart). No-op if no session.
func (tm *TmuxProcessManager) ResetExitOnce() {
	s := tm.session.Load()
	if s == nil {
		return
	}
	s.ResetExitOnce()
}

// StartControlMode starts the tmux control mode stream.
// Returns nil if no session is initialized.
func (tm *TmuxProcessManager) StartControlMode() error {
	s := tm.session.Load()
	if s == nil {
		return nil
	}
	return s.StartControlMode()
}

// StopControlMode stops the tmux control mode stream.
// Returns nil if no session is initialized.
func (tm *TmuxProcessManager) StopControlMode() error {
	s := tm.session.Load()
	if s == nil {
		return nil
	}
	return s.StopControlMode()
}

// SubscribeToControlModeUpdates registers a new subscriber for real-time terminal output.
// Returns a pre-closed channel if no session is initialized.
func (tm *TmuxProcessManager) SubscribeToControlModeUpdates() (string, chan []byte) {
	s := tm.session.Load()
	if s == nil {
		ch := make(chan []byte)
		close(ch)
		return "", ch
	}
	return s.SubscribeToControlModeUpdates()
}

// UnsubscribeFromControlModeUpdates removes a subscriber by ID.
// No-op if no session is initialized.
func (tm *TmuxProcessManager) UnsubscribeFromControlModeUpdates(id string) {
	s := tm.session.Load()
	if s == nil {
		return
	}
	s.UnsubscribeFromControlModeUpdates(id)
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
	CapturePaneContentPriority() (string, error)
	CapturePaneContentRaw() (string, error)
	CapturePaneContentWithOptions(startLine, endLine string) (string, error)
	GetPaneDimensions() (width, height int, err error)
	GetCursorPosition() (x, y int, err error)
	GetPTY() (*os.File, error)
	SendKeys(keys string) (int, error)
	SetWindowSize(cols, rows int) error
	RefreshClient() error
	RefreshClientPriority() error
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
	SendInputViaControlMode(ctx context.Context, data []byte) error
	// PaneExitStatus reports whether the pane's wrapped program has already
	// exited even though the tmux session object itself is still alive.
	// remain-on-exit keeps a "Pane is dead" placeholder pane around after the
	// wrapped program is killed (e.g. OOM SIGKILL) instead of tearing the
	// session down, so IsAlive()/HasSession() alone cannot detect this state.
	PaneExitStatus() (code int, signal string, dead bool)
}

// compile-time check that *TmuxProcessManager satisfies TmuxManager.
var _ TmuxManager = (*TmuxProcessManager)(nil)
