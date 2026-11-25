package procbackendtest

import (
	"context"
	"os"

	"github.com/tstapler/stapler-squad/session/instance"
)

// FakeLivenessProcessManager is a full instance.ProcessManager whose IsAlive follows alive.
// restoreRelease, when set, blocks RestoreWithWorkDir until closed; onRestore runs inside it.
type FakeLivenessProcessManager struct {
	Alive          bool
	RestoreRelease chan struct{}
	OnRestore      func()
}

func (m *FakeLivenessProcessManager) WireLaunch(instance.LaunchSpec) {}
func (m *FakeLivenessProcessManager) AppendLaunchEnv(...string)      {}
func (m *FakeLivenessProcessManager) Start(dir string) error         { return nil }
func (m *FakeLivenessProcessManager) RestoreWithWorkDir(dir string) error {
	if m.RestoreRelease != nil {
		<-m.RestoreRelease
	}
	if m.OnRestore != nil {
		m.OnRestore()
	}
	return nil
}
func (m *FakeLivenessProcessManager) Close() error                 { return nil }
func (m *FakeLivenessProcessManager) IsAlive() bool                { return m.Alive }
func (m *FakeLivenessProcessManager) HasLiveSessionNoCache() bool  { return m.Alive }
func (m *FakeLivenessProcessManager) GetSessionIdentifier() string { return "phase0-repro" }
func (m *FakeLivenessProcessManager) HasSession() bool             { return true }
func (m *FakeLivenessProcessManager) GetCurrentWorkingDirectory() (string, error) {
	return "/tmp", nil
}
func (m *FakeLivenessProcessManager) GetPTY() (*os.File, error)          { return nil, nil } //nolint:nilnil
func (m *FakeLivenessProcessManager) SendKeys(keys string) (int, error)  { return len(keys), nil }
func (m *FakeLivenessProcessManager) TapEnter() error                    { return nil }
func (m *FakeLivenessProcessManager) SendPromptWithEnter(p string) error { return nil }
func (m *FakeLivenessProcessManager) SendInputViaControlMode(ctx context.Context, data []byte) error {
	return nil
}

func (m *FakeLivenessProcessManager) content() string { return "" }
func (m *FakeLivenessProcessManager) CapturePaneContent() (string, error) {
	// Always returns the same (or growing, see growPerCall) content: simulates a
	// stuck/stale pane read during a flapping episode where the underlying tmux
	// session never advances, or an active session producing real new output.
	return m.content(), nil
}
func (m *FakeLivenessProcessManager) CapturePaneContentRaw() (string, error) {
	return m.content(), nil
}
func (m *FakeLivenessProcessManager) CapturePaneContentWithOptions(startLine, endLine string) (string, error) {
	return m.content(), nil
}
func (m *FakeLivenessProcessManager) CaptureViewport(lines int) (string, error) {
	return m.content(), nil
}
func (m *FakeLivenessProcessManager) GetCursorPosition() (int, int, error) { return 0, 0, nil }
func (m *FakeLivenessProcessManager) GetPaneDimensions() (int, int, error) { return 80, 24, nil }
func (m *FakeLivenessProcessManager) SetWindowSize(cols, rows int) error   { return nil }
func (m *FakeLivenessProcessManager) SetDetachedSize(w, h int, title string) error {
	return nil
}
func (m *FakeLivenessProcessManager) RefreshClient() error       { return nil }
func (m *FakeLivenessProcessManager) GetPanePID() (int32, error) { return 0, nil }
func (m *FakeLivenessProcessManager) HasUpdated() (bool, bool, string) {
	return false, false, ""
}
func (m *FakeLivenessProcessManager) FilterBanners(content string) (string, int) {
	return content, 0
}
func (m *FakeLivenessProcessManager) HasMeaningfulContent(content string) bool { return true }
func (m *FakeLivenessProcessManager) StartControlMode() error                  { return nil }
func (m *FakeLivenessProcessManager) StopControlMode() error                   { return nil }
func (m *FakeLivenessProcessManager) SubscribeToControlModeUpdates() (string, chan []byte) {
	return "", nil
}
func (m *FakeLivenessProcessManager) UnsubscribeFromControlModeUpdates(id string) {}
func (m *FakeLivenessProcessManager) Attach() (chan struct{}, error)              { return nil, nil } //nolint:nilnil
func (m *FakeLivenessProcessManager) DetachSafely() error                         { return nil }
func (m *FakeLivenessProcessManager) SetOnExitCallback(fn func(string))           {}
func (m *FakeLivenessProcessManager) ResetExitOnce()                              {}
