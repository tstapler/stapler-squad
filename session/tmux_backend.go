package session

import (
	"context"
	"os"
)

// TmuxBackend implements ProcessManager by delegating to TmuxManager.
// It is the default backend used when process_manager_backend = "tmux" (or empty).
type TmuxBackend struct {
	mgr TmuxManager
}

// NewTmuxBackend creates a TmuxBackend wrapping the given TmuxManager.
func NewTmuxBackend(mgr TmuxManager) *TmuxBackend {
	return &TmuxBackend{mgr: mgr}
}

// TmuxManager returns the underlying TmuxManager for type assertions in
// reconciliation paths that need tmux-specific operations (e.g. Session(), SetSession()).
func (b *TmuxBackend) TmuxManager() TmuxManager { return b.mgr }

// GetSessionIdentifier implements ProcessManager by delegating to GetTmuxSessionName.
// This is the name-mapping method: backend-agnostic callers use GetSessionIdentifier,
// but the value is identical to what GetTmuxSessionName returns for the tmux backend.
func (b *TmuxBackend) GetSessionIdentifier() string { return b.mgr.GetTmuxSessionName() }

// GetCurrentWorkingDirectory returns the current working directory of the pane.
// Delegates to the underlying Session().GetPaneCurrentPath() via type assertion.
func (b *TmuxBackend) GetCurrentWorkingDirectory() (string, error) {
	tb, ok := b.mgr.(*TmuxProcessManager)
	if !ok {
		// For mock/test implementations that don't have a real TmuxSession
		return "", nil
	}
	sess := tb.Session()
	if sess == nil {
		return "", nil
	}
	return sess.GetPaneCurrentPath()
}

// --- Lifecycle ---

func (b *TmuxBackend) Start(dir string) error            { return b.mgr.Start(dir) }
func (b *TmuxBackend) RestoreWithWorkDir(w string) error { return b.mgr.RestoreWithWorkDir(w) }
func (b *TmuxBackend) Close() error                      { return b.mgr.Close() }
func (b *TmuxBackend) IsAlive() bool                     { return b.mgr.IsAlive() }
func (b *TmuxBackend) HasSession() bool                  { return b.mgr.HasSession() }

// --- Terminal I/O ---

func (b *TmuxBackend) GetPTY() (*os.File, error)          { return b.mgr.GetPTY() }
func (b *TmuxBackend) SendKeys(keys string) (int, error)  { return b.mgr.SendKeys(keys) }
func (b *TmuxBackend) TapEnter() error                    { return b.mgr.TapEnter() }
func (b *TmuxBackend) SendPromptWithEnter(p string) error { return b.mgr.SendPromptWithEnter(p) }
func (b *TmuxBackend) SendInputViaControlMode(ctx context.Context, data []byte) error {
	return b.mgr.SendInputViaControlMode(ctx, data)
}

// --- Terminal state ---

func (b *TmuxBackend) CapturePaneContent() (string, error) { return b.mgr.CapturePaneContent() }
func (b *TmuxBackend) CapturePaneContentRaw() (string, error) {
	return b.mgr.CapturePaneContentRaw()
}
func (b *TmuxBackend) CapturePaneContentWithOptions(start, end string) (string, error) {
	return b.mgr.CapturePaneContentWithOptions(start, end)
}
func (b *TmuxBackend) CaptureViewport(lines int) (string, error) {
	return b.mgr.CaptureViewport(lines)
}
func (b *TmuxBackend) GetCursorPosition() (x, y int, err error) {
	return b.mgr.GetCursorPosition()
}
func (b *TmuxBackend) GetPaneDimensions() (width, height int, err error) {
	return b.mgr.GetPaneDimensions()
}
func (b *TmuxBackend) SetWindowSize(cols, rows int) error { return b.mgr.SetWindowSize(cols, rows) }
func (b *TmuxBackend) SetDetachedSize(w, h int, title string) error {
	return b.mgr.SetDetachedSize(w, h, title)
}
func (b *TmuxBackend) RefreshClient() error { return b.mgr.RefreshClient() }

// --- Process metadata ---

func (b *TmuxBackend) GetPanePID() (int32, error) { return b.mgr.GetPanePID() }

// --- Content helpers ---

func (b *TmuxBackend) HasUpdated() (updated bool, hasPrompt bool, content string) {
	return b.mgr.HasUpdated()
}
func (b *TmuxBackend) FilterBanners(content string) (string, int) {
	return b.mgr.FilterBanners(content)
}
func (b *TmuxBackend) HasMeaningfulContent(content string) bool {
	return b.mgr.HasMeaningfulContent(content)
}

// --- Streaming (control mode) ---

func (b *TmuxBackend) StartControlMode() error { return b.mgr.StartControlMode() }
func (b *TmuxBackend) StopControlMode() error  { return b.mgr.StopControlMode() }
func (b *TmuxBackend) SubscribeToControlModeUpdates() (string, chan []byte) {
	return b.mgr.SubscribeToControlModeUpdates()
}
func (b *TmuxBackend) UnsubscribeFromControlModeUpdates(id string) {
	b.mgr.UnsubscribeFromControlModeUpdates(id)
}

// --- Attach ---

func (b *TmuxBackend) Attach() (chan struct{}, error) { return b.mgr.Attach() }
func (b *TmuxBackend) DetachSafely() error            { return b.mgr.DetachSafely() }

// --- Exit notifications ---

func (b *TmuxBackend) SetOnExitCallback(fn func(string)) { b.mgr.SetOnExitCallback(fn) }
func (b *TmuxBackend) ResetExitOnce()                    { b.mgr.ResetExitOnce() }

// compile-time check that *TmuxBackend satisfies ProcessManager.
var _ ProcessManager = (*TmuxBackend)(nil)
