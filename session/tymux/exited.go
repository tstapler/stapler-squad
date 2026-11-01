package tymux

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"connectrpc.com/connect"

	v1 "github.com/tstapler/tymux/clients/go/gen/tymux/v1"
)

// ErrPaneExited matches (errors.Is) a capture of a pane whose process has
// exited. Since tymuxd v1.10.0 CapturePane on such a pane succeeds with
// liveness DEAD instead of FAILED_PRECONDITION, so "the call worked" no longer
// implies "the pane is alive".
var ErrPaneExited = errors.New("tymux: pane exited")

// ErrPaneNotRevivable matches (errors.Is) a restore that tymuxd will not revive: the pane is
// FAILED or STOPPED (tymuxd's ReviveSession answers FAILED_PRECONDITION for those, and the
// pre-check in reviveForRestore reports the same without the RPC). It needs an operator, so a
// caller should surface it rather than retry. No caller outside session/tymux branches on it
// yet: Instance's hot-restore path wraps it with %w and fails the start like any restore error.
var ErrPaneNotRevivable = errors.New("tymux: pane not revivable")

// PaneExitedError is the typed form of ErrPaneExited. It keeps the pane's last
// rendered grid so a caller that wants a finished session's final output can
// still read it (see FinalScreen) while a liveness-sensitive loop fails fast.
type PaneExitedError struct {
	// ExitCode is nil when the daemon did not report one.
	ExitCode *int32
	// Screen is the last grid, rendered like a live capture.
	Screen string
	// RestartPending is true while tymuxd's restart policy will bring the pane back
	// (BACKOFF or RESTARTING), so DEAD is transient. False also when the daemon
	// cannot say (no pane.lifecycle capability, or the status call failed).
	RestartPending bool
}

func (e *PaneExitedError) Error() string {
	msg := ErrPaneExited.Error()
	if e.ExitCode != nil {
		msg = fmt.Sprintf("%s (exit code %d)", msg, *e.ExitCode)
	}
	if e.RestartPending {
		msg += ", restart pending"
	}
	return msg
}

func (e *PaneExitedError) Is(target error) bool { return target == ErrPaneExited }

// IsFinalExit reports whether err is an exited-pane error that will not recover:
// callers that fail fast on exit must keep waiting when a restart is pending.
func IsFinalExit(err error) bool {
	var exited *PaneExitedError
	if errors.As(err, &exited) {
		return !exited.RestartPending
	}
	return errors.Is(err, ErrPaneExited)
}

// FinalScreen turns an exited-pane capture back into its last grid, for
// callers (previews, debug dumps, last-output readers) that want it. Any other
// result passes through unchanged.
func FinalScreen(content string, err error) (string, error) {
	var exited *PaneExitedError
	if errors.As(err, &exited) {
		return exited.Screen, nil
	}
	return content, err
}

// exitedPaneError returns a *PaneExitedError when snap reports a dead pane,
// else nil. Only an explicit DEAD counts: an older daemon's UNSPECIFIED is not
// evidence of exit.
func exitedPaneError(snap *v1.PaneSnapshot) *PaneExitedError {
	if snap.GetLiveness() != v1.Liveness_LIVENESS_DEAD {
		return nil
	}
	screen, _ := CellsToSGR(snap.GetGrid(), snap.GetHyperlinks())
	e := &PaneExitedError{Screen: screen}
	if snap.ExitCode != nil {
		code := snap.GetExitCode()
		e.ExitCode = &code
	}
	return e
}

// capPaneLifecycle is the GetServerInfo capability for GetPaneStatus and the restart policy.
const capPaneLifecycle = "pane.lifecycle"

// paneLifecycleState asks the daemon for a pane's lifecycle state. ok is false when it cannot
// say (older daemon without pane.lifecycle, or the call failed), which callers must treat as
// "unknown" rather than as any particular state.
func (s *tymuxGRPCSession) paneLifecycleState(ctx context.Context, paneID string) (state v1.PaneState, ok bool) {
	ctx, cancel := context.WithTimeout(ctx, lifecycleRPCTimeout)
	defer cancel()
	if !s.supportsPaneLifecycle(ctx) {
		return v1.PaneState_PANE_STATE_UNSPECIFIED, false
	}
	resp, err := s.transport.GetPaneStatus(ctx, connect.NewRequest(&v1.GetPaneStatusRequest{PaneId: paneID}))
	if err != nil {
		return v1.PaneState_PANE_STATE_UNSPECIFIED, false
	}
	return resp.Msg.GetState(), true
}

// lifecycleRPCTimeout bounds the GetServerInfo/GetPaneStatus pair so a wedged daemon cannot stall
// IsAlive or a capture.
const lifecycleRPCTimeout = 3 * time.Second

// Values of tymuxGRPCSession.lifecycleCap.
const (
	capUnknown int32 = iota
	capSupported
	capUnsupported
)

// supportsPaneLifecycle answers whether the daemon advertises pane.lifecycle. A definite answer
// is cached for the session's lifetime (the daemon binary is pinned per release); a failed
// GetServerInfo is not cached, so a transient fault cannot permanently disable the lookup.
func (s *tymuxGRPCSession) supportsPaneLifecycle(ctx context.Context) bool {
	switch s.lifecycleCap.Load() {
	case capSupported:
		return true
	case capUnsupported:
		return false
	}
	info, err := s.transport.GetServerInfo(ctx, connect.NewRequest(&v1.GetServerInfoRequest{}))
	if connect.CodeOf(err) == connect.CodeUnimplemented {
		s.lifecycleCap.Store(capUnsupported)
		return false
	}
	if err != nil {
		return false
	}
	supported := slices.Contains(info.Msg.GetCapabilities(), capPaneLifecycle)
	if supported {
		s.lifecycleCap.Store(capSupported)
	} else {
		s.lifecycleCap.Store(capUnsupported)
	}
	return supported
}

func isRestartPending(state v1.PaneState) bool {
	return state == v1.PaneState_PANE_STATE_BACKOFF || state == v1.PaneState_PANE_STATE_RESTARTING
}

// isTerminalPaneState is true for states tymuxd leaves alone until an operator acts.
func isTerminalPaneState(state v1.PaneState) bool {
	return state == v1.PaneState_PANE_STATE_FAILED || state == v1.PaneState_PANE_STATE_STOPPED
}

// restartPending asks the daemon whether a DEAD pane is between automatic restarts. Any
// failure or an older daemon answers false, which is the plain "exited" behavior.
func (s *tymuxGRPCSession) restartPending(ctx context.Context, paneID string) bool {
	state, ok := s.paneLifecycleState(ctx, paneID)
	return ok && isRestartPending(state)
}

// notRevivableError wraps ErrPaneNotRevivable with the state that blocks revival.
func notRevivableError(sessionID string, state v1.PaneState) error {
	return fmt.Errorf("%w: session %q pane is %s", ErrPaneNotRevivable, sessionID, state)
}
