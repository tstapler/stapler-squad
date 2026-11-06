package session

import (
	"context"
	"os"

	"github.com/tstapler/stapler-squad/session/launchcmd"
	"github.com/tstapler/stapler-squad/session/tmux"
)

// ProcessManager abstracts terminal process lifecycle and I/O.
// Implementations: TmuxBackend (wraps TmuxProcessManager), NativeProcessManager (Phase 2).
type ProcessManager interface {
	// WireLaunch tells the backend what program to run, before Start or
	// RestoreWithWorkDir. It is a required method so a new backend cannot compile
	// without deciding how it consumes the launch command; callers never
	// type-switch on the concrete backend to do this.
	WireLaunch(spec LaunchSpec)

	// AppendLaunchEnv adds environment (KEY=VALUE) for the program the next Start
	// launches. A backend that cannot pass env must say so by doing nothing, explicitly.
	AppendLaunchEnv(env ...string)

	// Lifecycle
	Start(dir string) error
	RestoreWithWorkDir(workDir string) error
	Close() error
	IsAlive() bool
	// HasLiveSessionNoCache is IsAlive() with any cached liveness flag
	// bypassed — always a fresh check. Callers use this before an action
	// whose cost of a stale positive is high (e.g. attaching control-mode
	// to what turns out to be a dead session and immediately receiving
	// %exit).
	HasLiveSessionNoCache() bool

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

// LaunchSpec is everything a backend needs to launch an instance's program.
type LaunchSpec struct {
	// Program is the full launch command (including any --resume flag) as shell
	// text, for backends that run it through a shell.
	Program string
	// Command is the same launch command as data, for backends that exec directly.
	Command launchcmd.LaunchCommand
	// NewTmuxSession builds the instance-configured tmux session (runner,
	// prefix, socket, env) around Program; only the tmux backend calls it.
	NewTmuxSession func(extra ...tmux.TmuxSessionOption) *tmux.TmuxSession
}

// ProcessManagerBackend identifies the backend implementation.
type ProcessManagerBackend string

const (
	BackendTmux   ProcessManagerBackend = "tmux"
	BackendNative ProcessManagerBackend = "native"
	BackendTymux  ProcessManagerBackend = "tymux"
)

// pushesLivenessEvents lists backends that report liveness push-style
// (instanceOnExitCallback) instead of being independently pollable by name
// — the single place to add a future backend instead of touching every
// poll site that needs to skip it.
var pushesLivenessEvents = map[ProcessManagerBackend]bool{
	BackendTymux: true,
}

// SkipsPollBasedLiveness reports whether tmux-socket-based liveness polling
// (reconcileSessions, health.go) has anything correct to add for this backend.
func (b ProcessManagerBackend) SkipsPollBasedLiveness() bool {
	return pushesLivenessEvents[b]
}

// ProcessManagerOptions holds constructor parameters for NewProcessManager.
type ProcessManagerOptions struct {
	SessionName  string
	Prefix       string
	ServerSocket string
	Program      string
	// Command, when set, is used instead of Program by backends that exec directly.
	Command launchcmd.LaunchCommand
	Args    []string
	// Backend, when non-empty, overrides both the process-wide default
	// (RegisterBackendProvider) and NewProcessManager's defaultBackend argument for
	// this one call — an explicit per-session choice always wins. Zero value ("")
	// means "no per-session override," preserving today's behavior for every caller
	// that doesn't set it. See NewProcessManager's precedence in backend_factory.go.
	Backend ProcessManagerBackend
	// TymuxSessionID is the persisted daemon session id for a restored tymux Instance.
	TymuxSessionID string
}
