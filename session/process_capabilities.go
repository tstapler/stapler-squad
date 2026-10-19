package session

import (
	"context"

	"github.com/tstapler/stapler-squad/session/tmux"
)

// Backend behaviour splits in two:
//   - Anything that decides what the launched program sees (WireLaunch,
//     AppendLaunchEnv) is a required ProcessManager method, so a new backend
//     cannot compile without choosing.
//   - Optional, backend-specific behaviour is a capability below. Instance asks for
//     one via capabilityOf and degrades when absent; it never names a concrete backend.

// capabilityOf returns pm as capability T when its backend provides it.
func capabilityOf[T any](pm ProcessManager) (T, bool) {
	c, ok := pm.(T)
	return c, ok
}

// ExitStatusReporter reports that the wrapped program exited while the session
// object lives on (tmux remain-on-exit).
type ExitStatusReporter interface {
	PaneExitStatus() (code int, signal string, dead bool)
}

// OrphanProber checks the cached pane PID, catching a program that died without
// the session noticing.
type OrphanProber interface {
	PanePIDStillAlive() bool
}

// PriorityCapturer routes pane reads through the resync exec-gate fast lane.
type PriorityCapturer interface {
	CapturePaneContentPriority() (string, error)
	CapturePaneContentRawPriority(ctx context.Context) (string, error)
	RefreshClientPriority(ctx context.Context) error
	GetPaneDimensionsPriority(ctx context.Context) (width, height int, err error)
}

// ContextCapturer captures pane content with a cancellable subprocess.
type ContextCapturer interface {
	CapturePaneContentContext(ctx context.Context) (string, error)
}

// AltScreenQuerier asks the backend whether the pane shows its alternate screen.
type AltScreenQuerier interface {
	IsAlternateScreenActive() (bool, error)
}

// CachedScreenCapturer marks backends whose CapturePaneContent is TTL-cached,
// so polling it every tick costs no extra subprocess.
type CachedScreenCapturer interface {
	CapturesAreCached()
}

// PaneCwdReporter reports the pane's live working directory (not a cached start dir).
type PaneCwdReporter interface {
	PaneCurrentPath() (string, error)
}

// RemoteSessionEnsurer re-confirms a remote session is up instead of Start,
// which validates paths locally.
type RemoteSessionEnsurer interface {
	EnsureRemoteSession(ctx context.Context, path string) error
}

// TmuxSessionAccessor exposes the tmux session object for the few callers that need it.
type TmuxSessionAccessor interface {
	TmuxSession() *tmux.TmuxSession
	SetTmuxSession(*tmux.TmuxSession)
}
