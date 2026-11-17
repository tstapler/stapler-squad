package instance

import (
	"context"
	"errors"
	"os"

	"github.com/tstapler/stapler-squad/session/domain"
)

// InstallHermeticHooksForTest registers no-op stand-ins for the four ports Instance reaches
// through hooks the staying session code fills in at init (process-manager factory, history
// registry, history porter, driver control), so the Instance package's own test binary, which
// cannot import those backends, can build and run Instances. Call it once from TestMain.
func InstallHermeticHooksForTest() {
	ProcessManagerFactoryHook = func(context.Context, domain.ProcessManagerBackend, ProcessManagerOptions) (ProcessManager, error) {
		return noopProcessManager{}, nil
	}
	reg, err := NewHistoryAdapterRegistry(noopHistoryAdapter{})
	if err != nil {
		panic(err)
	}
	HistoryRegistryHook = reg
	HistoryPorterHook = func(context.Context, string, string, *Instance) error { return nil }
	DriverControlHook = noopDriverControl{}
}

var errNoBackend = errors.New("hermetic test backend has no process")

// noopProcessManager is a backend with no process: every query answers "nothing there".
type noopProcessManager struct{}

func (noopProcessManager) WireLaunch(spec LaunchSpec)                                     {}
func (noopProcessManager) AppendLaunchEnv(env ...string)                                  {}
func (noopProcessManager) Start(dir string) error                                         { return nil }
func (noopProcessManager) RestoreWithWorkDir(workDir string) error                        { return nil }
func (noopProcessManager) Close() error                                                   { return nil }
func (noopProcessManager) IsAlive() bool                                                  { return false }
func (noopProcessManager) HasLiveSessionNoCache() bool                                    { return false }
func (noopProcessManager) GetSessionIdentifier() string                                   { return "" }
func (noopProcessManager) HasSession() bool                                               { return false }
func (noopProcessManager) GetCurrentWorkingDirectory() (string, error)                    { return "", nil }
func (noopProcessManager) GetPTY() (*os.File, error)                                      { return nil, errNoBackend }
func (noopProcessManager) SendKeys(keys string) (int, error)                              { return 0, nil }
func (noopProcessManager) TapEnter() error                                                { return nil }
func (noopProcessManager) SendPromptWithEnter(prompt string) error                        { return nil }
func (noopProcessManager) SendInputViaControlMode(ctx context.Context, data []byte) error { return nil }
func (noopProcessManager) CapturePaneContent() (string, error)                            { return "", nil }
func (noopProcessManager) CapturePaneContentRaw() (string, error)                         { return "", nil }
func (noopProcessManager) CapturePaneContentWithOptions(startLine, endLine string) (string, error) {
	return "", nil
}
func (noopProcessManager) CaptureViewport(lines int) (string, error)                     { return "", nil }
func (noopProcessManager) GetCursorPosition() (x, y int, err error)                      { return 0, 0, nil }
func (noopProcessManager) GetPaneDimensions() (width, height int, err error)             { return 0, 0, nil }
func (noopProcessManager) SetWindowSize(cols, rows int) error                            { return nil }
func (noopProcessManager) SetDetachedSize(width, height int, instanceTitle string) error { return nil }
func (noopProcessManager) RefreshClient() error                                          { return nil }
func (noopProcessManager) GetPanePID() (int32, error)                                    { return 0, nil }
func (noopProcessManager) HasUpdated() (updated bool, hasPrompt bool, content string) {
	return false, false, ""
}
func (noopProcessManager) FilterBanners(content string) (string, int)           { return content, 0 }
func (noopProcessManager) HasMeaningfulContent(content string) bool             { return false }
func (noopProcessManager) StartControlMode() error                              { return nil }
func (noopProcessManager) StopControlMode() error                               { return nil }
func (noopProcessManager) SubscribeToControlModeUpdates() (string, chan []byte) { return "", nil }
func (noopProcessManager) UnsubscribeFromControlModeUpdates(id string)          {}
func (noopProcessManager) Attach() (chan struct{}, error)                       { return nil, errNoBackend }
func (noopProcessManager) DetachSafely() error                                  { return nil }
func (noopProcessManager) SetOnExitCallback(fn func(string))                    {}
func (noopProcessManager) ResetExitOnce()                                       {}

type noopHistoryAdapter struct{}

func (noopHistoryAdapter) Name() string          { return "noop" }
func (noopHistoryAdapter) CanHandle(string) bool { return false }
func (noopHistoryAdapter) Import(context.Context, *Instance) ([]CanonicalTurn, error) {
	return nil, nil
}
func (noopHistoryAdapter) Export(context.Context, []CanonicalTurn, *Instance) error { return nil }

type noopDriverControl struct{}

func (noopDriverControl) Start(*Instance, string) {}
func (noopDriverControl) RestartForRetry(*Instance, string, string, RetryPolicy, <-chan struct{}) error {
	return nil
}
