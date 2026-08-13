//go:build windows

package session

import (
	"context"
	"fmt"
	"os"
)

// NativeProcessManager is not supported on Windows.
// All methods return an "unsupported platform" error.
type NativeProcessManager struct{}

// NewNativeProcessManager returns an unsupported-platform NativeProcessManager on Windows.
func NewNativeProcessManager(_ ProcessManagerOptions) *NativeProcessManager {
	return &NativeProcessManager{}
}

var _ ProcessManager = (*NativeProcessManager)(nil)

func (n *NativeProcessManager) Start(_ string) error {
	return fmt.Errorf("NativeProcessManager: not supported on Windows")
}
func (n *NativeProcessManager) RestoreWithWorkDir(_ string) error { return nil }
func (n *NativeProcessManager) Close() error                      { return nil }
func (n *NativeProcessManager) IsAlive() bool                     { return false }
func (n *NativeProcessManager) HasSession() bool                  { return false }
func (n *NativeProcessManager) GetCurrentWorkingDirectory() (string, error) {
	return "", nil
}
func (n *NativeProcessManager) GetSessionIdentifier() string { return "" }
func (n *NativeProcessManager) GetPTY() (*os.File, error) {
	return nil, fmt.Errorf("NativeProcessManager: not supported on Windows")
}
func (n *NativeProcessManager) SendKeys(_ string) (int, error) {
	return 0, fmt.Errorf("NativeProcessManager: not supported on Windows")
}
func (n *NativeProcessManager) TapEnter() error {
	return fmt.Errorf("NativeProcessManager: not supported on Windows")
}
func (n *NativeProcessManager) SendPromptWithEnter(_ string) error {
	return fmt.Errorf("NativeProcessManager: not supported on Windows")
}
func (n *NativeProcessManager) SendInputViaControlMode(_ context.Context, _ []byte) error {
	return fmt.Errorf("NativeProcessManager: not supported on Windows")
}
func (n *NativeProcessManager) CapturePaneContent() (string, error)    { return "", nil }
func (n *NativeProcessManager) CapturePaneContentRaw() (string, error) { return "", nil }
func (n *NativeProcessManager) CapturePaneContentWithOptions(_, _ string) (string, error) {
	return "", nil
}
func (n *NativeProcessManager) CaptureViewport(_ int) (string, error) { return "", nil }
func (n *NativeProcessManager) GetCursorPosition() (x, y int, err error) {
	return 0, 0, nil
}
func (n *NativeProcessManager) GetPaneDimensions() (width, height int, err error) {
	return 0, 0, nil
}
func (n *NativeProcessManager) SetWindowSize(_, _ int) error               { return nil }
func (n *NativeProcessManager) SetDetachedSize(_, _ int, _ string) error   { return nil }
func (n *NativeProcessManager) RefreshClient() error                       { return nil }
func (n *NativeProcessManager) GetPanePID() (int32, error)                 { return 0, nil }
func (n *NativeProcessManager) HasUpdated() (bool, bool, string)           { return false, false, "" }
func (n *NativeProcessManager) FilterBanners(content string) (string, int) { return content, 0 }
func (n *NativeProcessManager) HasMeaningfulContent(_ string) bool         { return false }
func (n *NativeProcessManager) StartControlMode() error                    { return nil }
func (n *NativeProcessManager) StopControlMode() error                     { return nil }
func (n *NativeProcessManager) SubscribeToControlModeUpdates() (string, chan []byte) {
	return "", make(chan []byte)
}
func (n *NativeProcessManager) UnsubscribeFromControlModeUpdates(_ string) {}
func (n *NativeProcessManager) Attach() (chan struct{}, error) {
	return nil, fmt.Errorf("NativeProcessManager: not supported on Windows")
}
func (n *NativeProcessManager) DetachSafely() error              { return nil }
func (n *NativeProcessManager) SetOnExitCallback(_ func(string)) {}
func (n *NativeProcessManager) ResetExitOnce()                   {}
