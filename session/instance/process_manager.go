package instance

import (
	"context"
	"fmt"
	"os"

	"github.com/tstapler/stapler-squad/session/domain"
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
	Backend domain.ProcessManagerBackend
	// TymuxSessionID is the persisted daemon session id for a restored tymux Instance.
	TymuxSessionID string
}

// BackendKeyed is the optional capability naming the registry key a ProcessManager
// was built for.
type BackendKeyed interface {
	BackendKey() domain.ProcessManagerBackend
}

// processManagerBackendKey returns the backend a ProcessManager actually is, or ""
// when it declares none (nil or a test fake).
func ProcessManagerBackendKey(pm ProcessManager) domain.ProcessManagerBackend {
	if k, ok := capabilityOf[BackendKeyed](pm); ok {
		return k.BackendKey()
	}
	return ""
}

// ErrUnrecognizedBackend is returned by NewProcessManager when the resolved
// ProcessManagerBackend value (from opts.Backend, the process-wide global, or
// defaultBackend) matches none of the known backend constants
// (BackendTmux/BackendNative/BackendTymux). Story 2.1.3's acceptance
// criteria (UX-9.2) require this to fail loudly at construction, not
// silently fall back to BackendTmux and not panic — an unrecognized value is
// most likely a typo'd constant or corrupted persisted data, and a silent
// tmux fallback would mask that.
var ErrUnrecognizedBackend = fmt.Errorf("session: unrecognized ProcessManagerBackend")

// LaunchCommand returns opts.Command, or opts.Program parsed from shell text; nil when neither is set.
func (o ProcessManagerOptions) LaunchCommand() launchcmd.LaunchCommand {
	if o.Command != nil {
		return o.Command
	}
	return ShellLaunch(o.Program)
}

// ProcessManagerFactory builds the ProcessManager for a session. Instance code builds
// its ProcessManager through processManagerFactoryHook so it does not name
// backend_factory.go (tmux, tymux and native backends), which stays above it. A
// genuine seam: three production backends plus test fakes.
type ProcessManagerFactory func(ctx context.Context, defaultBackend domain.ProcessManagerBackend, opts ProcessManagerOptions) (ProcessManager, error)

// processManagerFactoryHook is assigned once, in backend_factory.go's init().
var ProcessManagerFactoryHook ProcessManagerFactory

func newProcessManager(ctx context.Context, defaultBackend domain.ProcessManagerBackend, opts ProcessManagerOptions) (ProcessManager, error) {
	if ProcessManagerFactoryHook == nil {
		panic("session: ProcessManagerFactory not registered (backend_factory.go's init must set processManagerFactoryHook)")
	}
	return ProcessManagerFactoryHook(ctx, defaultBackend, opts)
}
