package session

import (
	"context"
	"os"
)

// ProcessManager abstracts terminal process lifecycle and I/O.
// Implementations: TmuxBackend (wraps TmuxProcessManager), NativeProcessManager (Phase 2).
type ProcessManager interface {
	// Lifecycle
	Start(dir string) error
	RestoreWithWorkDir(workDir string) error
	Close() error
	IsAlive() bool

	// Identification
	GetSessionIdentifier() string

	// Existence / state
	HasSession() bool

	// Working directory (via pane or process introspection)
	GetCurrentWorkingDirectory() (string, error)

	// Terminal I/O
	GetPTY() (*os.File, error)
	SendKeys(keys string) (int, error)
	TapEnter() error
	SendPromptWithEnter(prompt string) error
	SendInputViaControlMode(ctx context.Context, data []byte) error

	// Terminal state
	CapturePaneContent() (string, error)
	CapturePaneContentRaw() (string, error)
	CapturePaneContentWithOptions(startLine, endLine string) (string, error)
	CaptureViewport(lines int) (string, error)
	GetCursorPosition() (x, y int, err error)
	GetPaneDimensions() (width, height int, err error)
	SetWindowSize(cols, rows int) error
	SetDetachedSize(width, height int, instanceTitle string) error
	RefreshClient() error

	// Process metadata
	GetPanePID() (int32, error)

	// Content helpers
	HasUpdated() (updated bool, hasPrompt bool, content string)
	FilterBanners(content string) (string, int)
	HasMeaningfulContent(content string) bool

	// Streaming (control mode)
	StartControlMode() error
	StopControlMode() error
	// SubscribeToControlModeUpdates returns a subscription ID and a bidirectional channel.
	// The channel must be bidirectional (chan []byte, not <-chan []byte) because some callers
	// write synthetic frames for testing. Implementations must not write to the channel themselves.
	SubscribeToControlModeUpdates() (string, chan []byte)
	UnsubscribeFromControlModeUpdates(id string)

	// Attach (interactive TUI)
	Attach() (chan struct{}, error)
	DetachSafely() error

	// Exit notifications
	SetOnExitCallback(fn func(string))
	ResetExitOnce()
}

// ProcessManagerBackend identifies the backend implementation.
type ProcessManagerBackend string

const (
	BackendTmux   ProcessManagerBackend = "tmux"
	BackendNative ProcessManagerBackend = "native"
)

// ProcessManagerOptions holds constructor parameters for NewProcessManager.
type ProcessManagerOptions struct {
	SessionName  string
	Prefix       string
	ServerSocket string
	Program      string
	Args         []string
}
