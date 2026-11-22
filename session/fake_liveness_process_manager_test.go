package session

import (
	"context"
	"os"

	"github.com/tstapler/stapler-squad/session/instance"
)

// fakeLivenessProcessManager is a full instance.ProcessManager whose IsAlive follows alive.
// restoreRelease, when set, blocks RestoreWithWorkDir until closed; onRestore runs inside it.
type fakeLivenessProcessManager struct {
	alive          bool
	restoreRelease chan struct{}
	onRestore      func()
}

func (m *fakeLivenessProcessManager) WireLaunch(instance.LaunchSpec) {}
func (m *fakeLivenessProcessManager) AppendLaunchEnv(...string)      {}
func (m *fakeLivenessProcessManager) Start(dir string) error         { return nil }
func (m *fakeLivenessProcessManager) RestoreWithWorkDir(dir string) error {
	if m.restoreRelease != nil {
		<-m.restoreRelease
	}
	if m.onRestore != nil {
		m.onRestore()
	}
	return nil
}
func (m *fakeLivenessProcessManager) Close() error                 { return nil }
func (m *fakeLivenessProcessManager) IsAlive() bool                { return m.alive }
func (m *fakeLivenessProcessManager) HasLiveSessionNoCache() bool  { return m.alive }
func (m *fakeLivenessProcessManager) GetSessionIdentifier() string { return "phase0-repro" }
func (m *fakeLivenessProcessManager) HasSession() bool             { return true }
func (m *fakeLivenessProcessManager) GetCurrentWorkingDirectory() (string, error) {
	return "/tmp", nil
}
func (m *fakeLivenessProcessManager) GetPTY() (*os.File, error)          { return nil, nil } //nolint:nilnil
func (m *fakeLivenessProcessManager) SendKeys(keys string) (int, error)  { return len(keys), nil }
func (m *fakeLivenessProcessManager) TapEnter() error                    { return nil }
func (m *fakeLivenessProcessManager) SendPromptWithEnter(p string) error { return nil }
func (m *fakeLivenessProcessManager) SendInputViaControlMode(ctx context.Context, data []byte) error {
	return nil
}

func (m *fakeLivenessProcessManager) content() string { return "" }
func (m *fakeLivenessProcessManager) CapturePaneContent() (string, error) {
	// Always returns the same (or growing, see growPerCall) content: simulates a
	// stuck/stale pane read during a flapping episode where the underlying tmux
	// session never advances, or an active session producing real new output.
	return m.content(), nil
}
func (m *fakeLivenessProcessManager) CapturePaneContentRaw() (string, error) {
	return m.content(), nil
}
func (m *fakeLivenessProcessManager) CapturePaneContentWithOptions(startLine, endLine string) (string, error) {
	return m.content(), nil
}
func (m *fakeLivenessProcessManager) CaptureViewport(lines int) (string, error) {
	return m.content(), nil
}
func (m *fakeLivenessProcessManager) GetCursorPosition() (int, int, error) { return 0, 0, nil }
func (m *fakeLivenessProcessManager) GetPaneDimensions() (int, int, error) { return 80, 24, nil }
func (m *fakeLivenessProcessManager) SetWindowSize(cols, rows int) error   { return nil }
func (m *fakeLivenessProcessManager) SetDetachedSize(w, h int, title string) error {
	return nil
}
func (m *fakeLivenessProcessManager) RefreshClient() error       { return nil }
func (m *fakeLivenessProcessManager) GetPanePID() (int32, error) { return 0, nil }
func (m *fakeLivenessProcessManager) HasUpdated() (bool, bool, string) {
	return false, false, ""
}
func (m *fakeLivenessProcessManager) FilterBanners(content string) (string, int) {
	return content, 0
}
func (m *fakeLivenessProcessManager) HasMeaningfulContent(content string) bool { return true }
func (m *fakeLivenessProcessManager) StartControlMode() error                  { return nil }
func (m *fakeLivenessProcessManager) StopControlMode() error                   { return nil }
func (m *fakeLivenessProcessManager) SubscribeToControlModeUpdates() (string, chan []byte) {
	return "", nil
}
func (m *fakeLivenessProcessManager) UnsubscribeFromControlModeUpdates(id string) {}
func (m *fakeLivenessProcessManager) Attach() (chan struct{}, error)              { return nil, nil } //nolint:nilnil
func (m *fakeLivenessProcessManager) DetachSafely() error                         { return nil }
func (m *fakeLivenessProcessManager) SetOnExitCallback(fn func(string))           {}
func (m *fakeLivenessProcessManager) ResetExitOnce()                              {}
