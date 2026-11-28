package access

import (
	"context"
	"errors"
	"fmt"
)

// ErrSteerPaneOwnership marks a SteerFailed caused by the pre-write pane
// ownership check (the tmux target no longer belongs to the session), as
// opposed to a failed write. Nothing was written.
var ErrSteerPaneOwnership = errors.New("pane ownership verification failed")

// SteerOutcome is the typed result of a guarded steer.
type SteerOutcome int

const (
	// SteerUnspecified is the zero value so an unset outcome never reads as delivered.
	SteerUnspecified SteerOutcome = iota
	SteerDelivered
	// SteerGuardBusy: another delivery to the session is in flight.
	SteerGuardBusy
	// SteerDuplicate: the same reason signature was delivered moments ago.
	SteerDuplicate
	// SteerCoolingDown: the same reason signature just failed to deliver; retry shortly.
	SteerCoolingDown
	// SteerBusy: the session is not idle.
	SteerBusy
	// SteerNoStatusSource: no active controller/status source to confirm idleness.
	SteerNoStatusSource
	// SteerNotTracked: no live instance for the session.
	SteerNotTracked
	// SteerFailed: the write or the pre-write pane-ownership check failed; see the error.
	SteerFailed
)

func (o SteerOutcome) String() string {
	switch o {
	case SteerUnspecified:
		return "unspecified"
	case SteerDelivered:
		return "delivered"
	case SteerGuardBusy:
		return "guard_busy"
	case SteerDuplicate:
		return "duplicate"
	case SteerCoolingDown:
		return "cooling_down"
	case SteerBusy:
		return "busy"
	case SteerNoStatusSource:
		return "no_status_source"
	case SteerNotTracked:
		return "not_tracked"
	case SteerFailed:
		return "failed"
	default:
		return fmt.Sprintf("SteerOutcome(%d)", int(o))
	}
}

// NudgeRecorder records a user-originated intervention on a session. Only
// human-driven RPC handlers may call it; automated steers never do.
type NudgeRecorder interface {
	RecordManualNudgeForSession(ctx context.Context, sessionUUID, value string)
}
