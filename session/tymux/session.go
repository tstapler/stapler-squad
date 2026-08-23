package tymux

import (
	"context"
	"errors"
	"os"
)

// ErrNotImplemented is returned by tymuxGRPCSession methods that Epic 2.2+ has not
// yet implemented. This lets BackendTymux and backend_factory.go compile and be
// wired end-to-end (Epic 2.1) before the real RPC-backed behavior lands.
var ErrNotImplemented = errors.New("tymux: not implemented")

// tymuxGRPCSession is the real TymuxManager implementation, backed by a live
// tymuxd daemon over gRPC via rpcTransport. Every method is a stub for now —
// Epic 2.2 implements the real RPC calls behind this same interface.
type tymuxGRPCSession struct {
	transport rpcTransport
}

// NewTymuxGRPCSession constructs a tymuxGRPCSession using the given rpcTransport,
// returned as a TymuxManager since tymuxGRPCSession itself is unexported.
func NewTymuxGRPCSession(transport rpcTransport) TymuxManager {
	return &tymuxGRPCSession{transport: transport}
}

// --- Lifecycle ---

func (s *tymuxGRPCSession) Start(dir string) error            { return ErrNotImplemented }
func (s *tymuxGRPCSession) RestoreWithWorkDir(w string) error { return ErrNotImplemented }
func (s *tymuxGRPCSession) Close() error                      { return ErrNotImplemented }
func (s *tymuxGRPCSession) IsAlive() bool                     { return false }

// --- Identification ---

func (s *tymuxGRPCSession) GetSessionIdentifier() string { return "" }

// --- Existence / state ---

func (s *tymuxGRPCSession) HasSession() bool { return false }

// --- Working directory ---

func (s *tymuxGRPCSession) GetCurrentWorkingDirectory() (string, error) {
	return "", ErrNotImplemented
}

// --- Terminal I/O ---

func (s *tymuxGRPCSession) GetPTY() (*os.File, error)          { return nil, ErrNotImplemented }
func (s *tymuxGRPCSession) SendKeys(keys string) (int, error)  { return 0, ErrNotImplemented }
func (s *tymuxGRPCSession) TapEnter() error                    { return ErrNotImplemented }
func (s *tymuxGRPCSession) SendPromptWithEnter(p string) error { return ErrNotImplemented }
func (s *tymuxGRPCSession) SendInputViaControlMode(ctx context.Context, data []byte) error {
	return ErrNotImplemented
}

// --- Terminal state ---

func (s *tymuxGRPCSession) CapturePaneContent() (string, error)    { return "", ErrNotImplemented }
func (s *tymuxGRPCSession) CapturePaneContentRaw() (string, error) { return "", ErrNotImplemented }
func (s *tymuxGRPCSession) CapturePaneContentWithOptions(startLine, endLine string) (string, error) {
	return "", ErrNotImplemented
}
func (s *tymuxGRPCSession) CaptureViewport(lines int) (string, error) {
	return "", ErrNotImplemented
}
func (s *tymuxGRPCSession) GetCursorPosition() (x, y int, err error) {
	return 0, 0, ErrNotImplemented
}
func (s *tymuxGRPCSession) GetPaneDimensions() (width, height int, err error) {
	return 0, 0, ErrNotImplemented
}
func (s *tymuxGRPCSession) SetWindowSize(cols, rows int) error { return ErrNotImplemented }
func (s *tymuxGRPCSession) SetDetachedSize(w, h int, title string) error {
	return ErrNotImplemented
}
func (s *tymuxGRPCSession) RefreshClient() error { return ErrNotImplemented }

// --- Process metadata ---

func (s *tymuxGRPCSession) GetPanePID() (int32, error) { return 0, ErrNotImplemented }

// --- Content helpers ---

func (s *tymuxGRPCSession) HasUpdated() (updated bool, hasPrompt bool, content string) {
	return false, false, ""
}
func (s *tymuxGRPCSession) FilterBanners(content string) (string, int) { return content, 0 }
func (s *tymuxGRPCSession) HasMeaningfulContent(content string) bool   { return false }

// --- Streaming (control mode) ---

func (s *tymuxGRPCSession) StartControlMode() error { return ErrNotImplemented }
func (s *tymuxGRPCSession) StopControlMode() error  { return ErrNotImplemented }
func (s *tymuxGRPCSession) SubscribeToControlModeUpdates() (string, chan []byte) {
	return "", nil
}
func (s *tymuxGRPCSession) UnsubscribeFromControlModeUpdates(id string) {}

// --- Attach ---

func (s *tymuxGRPCSession) Attach() (chan struct{}, error) { return nil, ErrNotImplemented }
func (s *tymuxGRPCSession) DetachSafely() error            { return ErrNotImplemented }

// --- Exit notifications ---

func (s *tymuxGRPCSession) SetOnExitCallback(fn func(string)) {}
func (s *tymuxGRPCSession) ResetExitOnce()                    {}

// compile-time check that *tymuxGRPCSession satisfies TymuxManager.
var _ TymuxManager = (*tymuxGRPCSession)(nil)
