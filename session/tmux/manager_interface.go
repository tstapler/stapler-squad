package tmux

import (
	"context"
	"os"
)

// TmuxManager is the interface satisfied by *TmuxProcessManager.
// It covers all tmux session operations and can be implemented by test
// doubles to avoid requiring a real tmux server.
type TmuxManager interface {
	HasSession() bool
	Session() *TmuxSession
	SetSession(*TmuxSession)
	GetTmuxSessionName() string
	IsAlive() bool
	Close() error
	DetachSafely() error
	DoesSessionExist() bool
	DoesSessionExistNoCache() bool
	SetDetachedSize(width, height int, instanceTitle string) error
	Attach() (chan struct{}, error)
	CapturePaneContent() (string, error)
	CapturePaneContentPriority() (string, error)
	CapturePaneContentRaw() (string, error)
	CapturePaneContentRawPriority(ctx context.Context) (string, error)
	CapturePaneContentWithOptions(startLine, endLine string) (string, error)
	GetPaneDimensions() (width, height int, err error)
	GetPaneDimensionsPriority(ctx context.Context) (width, height int, err error)
	GetCursorPosition() (x, y int, err error)
	GetPTY() (*os.File, error)
	SendKeys(keys string) (int, error)
	SetWindowSize(cols, rows int) error
	RefreshClient() error
	RefreshClientPriority(ctx context.Context) error
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
