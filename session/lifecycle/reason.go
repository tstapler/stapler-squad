// Package lifecycle classifies "why did this goroutine generation end" from
// several independent local signals (a deliberate-close flag, a clean-exit
// flag, ctx.Err()) into a closed Reason, so a subsystem's reconnect/exit-
// callback decision is one function call instead of a boolean-OR of flags.
// See project_plans/session-lifecycle-state-machine for the motivating
// incidents.
package lifecycle

// Reason is a closed enum representing why a goroutine-lifecycle generation
// ended. Its zero value, ReasonUnknown, is safe by default: any code path
// that never explicitly classifies a reason (a forgotten assignment, or a
// future variant added without updating a predicate's switch) is treated as
// "stop, and notify" by ShouldContinue/ShouldFireExitCallback respectively,
// rather than silently reconnecting or silently swallowing an exit. Mirrors
// session/detection.DetectedStatus's iota-plus-exhaustive-linter idiom (see
// .golangci.yml's exhaustive carve-out for this package).
type Reason int

const (
	// ReasonUnknown is Reason's zero value — see the type doc comment.
	ReasonUnknown Reason = iota
	// ReasonDeliberateClose means the whole session/session-owner is ending
	// on purpose.
	ReasonDeliberateClose
	// ReasonDeliberateSupersede means this generation is being torn down
	// deliberately for a reopen, but the owning session is not closing.
	ReasonDeliberateSupersede
	// ReasonCleanExit means the remote side already reported a clean exit
	// before this generation's read/receive failed.
	ReasonCleanExit
	// ReasonTransportDrop means an unexpected, non-deliberate stream/process
	// end — the only Reason for which ShouldContinue() returns true.
	ReasonTransportDrop
	// ReasonReconnectExhausted means a reconnect loop ran out of attempts
	// without reconnecting, as opposed to being interrupted by a deliberate
	// close.
	ReasonReconnectExhausted
)

// String returns r's snake_case label for use as a span/metric attribute
// value. An unhandled value returns "unknown", matching ReasonUnknown's
// label.
func (r Reason) String() string {
	switch r {
	case ReasonUnknown:
		return "unknown"
	case ReasonDeliberateClose:
		return "deliberate_close"
	case ReasonDeliberateSupersede:
		return "deliberate_supersede"
	case ReasonCleanExit:
		return "clean_exit"
	case ReasonTransportDrop:
		return "transport_drop"
	case ReasonReconnectExhausted:
		return "reconnect_exhausted"
	default:
		return "unknown"
	}
}

// ShouldContinue reports whether the calling goroutine should attempt to
// reconnect/retry for this Reason. Only ReasonTransportDrop returns true;
// every other value — including ReasonUnknown and any unhandled future
// variant — returns false (fail toward stopping).
func (r Reason) ShouldContinue() bool {
	switch r {
	case ReasonTransportDrop:
		return true
	case ReasonUnknown, ReasonDeliberateClose, ReasonDeliberateSupersede, ReasonCleanExit, ReasonReconnectExhausted:
		return false
	default:
		return false
	}
}

// ShouldFireExitCallback reports whether a one-shot exit callback should
// fire for this Reason. Only ReasonDeliberateClose returns false; every
// other value — including ReasonUnknown and any unhandled future variant —
// returns true (fail toward notifying, since a silently-swallowed exit is
// the worse outcome for a one-shot callback contract).
func (r Reason) ShouldFireExitCallback() bool {
	switch r {
	case ReasonDeliberateClose:
		return false
	case ReasonUnknown, ReasonDeliberateSupersede, ReasonCleanExit, ReasonTransportDrop, ReasonReconnectExhausted:
		return true
	default:
		return true
	}
}
