package session

import (
	"context"
	"os"
	"time"

	"github.com/tstapler/stapler-squad/session/tymux"
)

// TymuxBackend implements ProcessManager by delegating to a tymux.TymuxManager.
// It is the tymux-backed alternative to TmuxBackend, mirroring TmuxBackend's exact
// one-line-forward shape. Selected per session via ProcessManagerOptions.Backend, or
// process-wide via RegisterBackendProvider(BackendTymux) — see backend_factory.go.
type TymuxBackend struct {
	mgr tymux.TymuxManager
}

// NewTymuxBackend creates a TymuxBackend wrapping the given TymuxManager.
func NewTymuxBackend(mgr tymux.TymuxManager) *TymuxBackend {
	return &TymuxBackend{mgr: mgr}
}

// TymuxManager returns the underlying TymuxManager, e.g. for tymux-specific
// reconciliation paths that need it directly.
func (b *TymuxBackend) TymuxManager() tymux.TymuxManager { return b.mgr }

// --- Lifecycle ---

func (b *TymuxBackend) Start(dir string) error            { return b.mgr.Start(dir) }
func (b *TymuxBackend) RestoreWithWorkDir(w string) error { return b.mgr.RestoreWithWorkDir(w) }
func (b *TymuxBackend) Close() error                      { return b.mgr.Close() }
func (b *TymuxBackend) IsAlive() bool                     { return b.mgr.IsAlive() }
func (b *TymuxBackend) HasSession() bool                  { return b.mgr.HasSession() }

// GetSessionIdentifier implements ProcessManager by delegating to the underlying
// TymuxManager's own identifier.
func (b *TymuxBackend) GetSessionIdentifier() string { return b.mgr.GetSessionIdentifier() }

// GetCurrentWorkingDirectory returns the current working directory of the pane.
func (b *TymuxBackend) GetCurrentWorkingDirectory() (string, error) {
	return b.mgr.GetCurrentWorkingDirectory()
}

// --- Terminal I/O ---

func (b *TymuxBackend) GetPTY() (*os.File, error)          { return b.mgr.GetPTY() }
func (b *TymuxBackend) SendKeys(keys string) (int, error)  { return b.mgr.SendKeys(keys) }
func (b *TymuxBackend) TapEnter() error                    { return b.mgr.TapEnter() }
func (b *TymuxBackend) SendPromptWithEnter(p string) error { return b.mgr.SendPromptWithEnter(p) }
func (b *TymuxBackend) SendInputViaControlMode(ctx context.Context, data []byte) error {
	return b.mgr.SendInputViaControlMode(ctx, data)
}

// --- Terminal state ---

func (b *TymuxBackend) CapturePaneContent() (string, error) { return b.mgr.CapturePaneContent() }
func (b *TymuxBackend) CapturePaneContentRaw() (string, error) {
	return b.mgr.CapturePaneContentRaw()
}
func (b *TymuxBackend) CapturePaneContentWithOptions(start, end string) (string, error) {
	return b.mgr.CapturePaneContentWithOptions(start, end)
}
func (b *TymuxBackend) CaptureViewport(lines int) (string, error) {
	return b.mgr.CaptureViewport(lines)
}
func (b *TymuxBackend) GetCursorPosition() (x, y int, err error) {
	return b.mgr.GetCursorPosition()
}
func (b *TymuxBackend) GetPaneDimensions() (width, height int, err error) {
	return b.mgr.GetPaneDimensions()
}
func (b *TymuxBackend) SetWindowSize(cols, rows int) error {
	return b.mgr.SetWindowSize(cols, rows)
}
func (b *TymuxBackend) SetDetachedSize(w, h int, title string) error {
	return b.mgr.SetDetachedSize(w, h, title)
}
func (b *TymuxBackend) RefreshClient() error { return b.mgr.RefreshClient() }

// --- Process metadata ---

func (b *TymuxBackend) GetPanePID() (int32, error) { return b.mgr.GetPanePID() }

// --- Content helpers ---

func (b *TymuxBackend) HasUpdated() (updated bool, hasPrompt bool, content string) {
	return b.mgr.HasUpdated()
}
func (b *TymuxBackend) FilterBanners(content string) (string, int) {
	return b.mgr.FilterBanners(content)
}
func (b *TymuxBackend) HasMeaningfulContent(content string) bool {
	return b.mgr.HasMeaningfulContent(content)
}

// --- Streaming (control mode) ---

func (b *TymuxBackend) StartControlMode() error { return b.mgr.StartControlMode() }
func (b *TymuxBackend) StopControlMode() error  { return b.mgr.StopControlMode() }
func (b *TymuxBackend) SubscribeToControlModeUpdates() (string, chan []byte) {
	return b.mgr.SubscribeToControlModeUpdates()
}
func (b *TymuxBackend) UnsubscribeFromControlModeUpdates(id string) {
	b.mgr.UnsubscribeFromControlModeUpdates(id)
}

// --- Attach ---

func (b *TymuxBackend) Attach() (chan struct{}, error) { return b.mgr.Attach() }
func (b *TymuxBackend) DetachSafely() error            { return b.mgr.DetachSafely() }

// --- Exit notifications ---

func (b *TymuxBackend) SetOnExitCallback(fn func(string)) { b.mgr.SetOnExitCallback(fn) }
func (b *TymuxBackend) ResetExitOnce()                    { b.mgr.ResetExitOnce() }

// --- Reconnect / restart observability (Task 2.5.2e, Story 2.5.3) ---
//
// These two are deliberate additions beyond ProcessManager's mirrored method
// set (requirements.md's constraint keeps ProcessManager itself untouched),
// forwarded here so a real caller holding a *TymuxBackend (or the
// tymux.TymuxManager it wraps) can observe reconnect/restart state, not just
// a test asserting on the unexported tymuxGRPCSession concrete type.

// ReconnectState reports whether the standing Attach stream is currently
// reconnecting, which attempt it's on, and what triggered it.
func (b *TymuxBackend) ReconnectState() (reconnecting bool, attempt int, cause string) {
	return b.mgr.ReconnectState()
}

// BackendRestarted reports whether tymuxd was detected to have restarted out
// from under this session (Story 2.5.3's daemon-restart contract) — the
// pane's original process is orphaned, not reattached.
func (b *TymuxBackend) BackendRestarted() (restarted bool, since time.Time) {
	return b.mgr.BackendRestarted()
}

// compile-time check that *TymuxBackend satisfies ProcessManager.
var _ ProcessManager = (*TymuxBackend)(nil)
