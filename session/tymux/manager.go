// Package tymux contains the TymuxBackend implementation of session.ProcessManager
// (session/backend_tymux.go), which delegates to a tymuxd instance over gRPC via the
// generated Connect-Go client in github.com/tstapler/tymux/clients/go.
package tymux

import (
	"context"
	"os"
	"time"
)

// TymuxManager is the interface satisfied by the concrete tymux gRPC session
// implementation (tymuxGRPCSession, session.go). It mirrors session.ProcessManager's
// exact method set so TymuxBackend (session/backend_tymux.go) can forward every call
// one-to-one, the same shape TmuxBackend/TmuxManager use for the tmux backend
// (session/tmux_backend.go, session/tmux_process_manager.go).
//
// Defined as its own interface here (rather than the session package's ProcessManager
// directly) because session/tymux is imported BY the session package — depending on
// session.ProcessManager here would create an import cycle. Duplicating the method set
// also gives tests a seam to substitute a fake TymuxManager without a live tymuxd daemon.
type TymuxManager interface {
	// Lifecycle
	Start(dir string) error
	RestoreWithWorkDir(workDir string) error
	Close() error
	IsAlive() bool

	// Identification
	GetSessionIdentifier() string

	// Existence / state
	HasSession() bool

	// Working directory (via pane introspection)
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

	// Reconnect state (Task 2.5.2e): exposes ReconnectLoop's live
	// progress — whether this session's standing stream is currently
	// reconnecting, which attempt it's on, and what triggered it — so a
	// future UI (ux.md Surface 2) doesn't have to reverse-engineer it
	// from the aggregate tymux_attach_stream_reconnects_total metric.
	// The one deliberate addition beyond ProcessManager's mirrored
	// method set (requirements.md's constraint keeps ProcessManager
	// itself untouched); state exposure only, no UI here.
	ReconnectState() (reconnecting bool, attempt int, cause string)

	// BackendRestarted reports whether tymuxd was detected to have restarted
	// out from under this session (Story 2.5.3's daemon-restart contract) —
	// the pane's original process is orphaned, not reattached, per
	// Engine::revive_session's always-spawn-fresh behavior on the tymux
	// side. Exposed on the interface (not just the concrete type) so a real
	// caller holding only a TymuxManager/ProcessManager reference can
	// observe this state, not just tests asserting on the unexported type.
	BackendRestarted() (restarted bool, since time.Time)
}
