package tmux

import "github.com/tstapler/stapler-squad/session/diagnose"

// SessionIdentity is the immutable pairing of a session's UUID and the owner
// marker written into its tmux pane, used to re-verify identity right before
// a nudge write (see ADR-002). Identity comparison lives here so it isn't
// re-derived ad hoc at each nudge call site.
type SessionIdentity struct {
	SessionUUID     string
	TmuxOwnerMarker string
}

// Matches reports whether both the session UUID and the tmux owner marker
// agree with expectedSessionUUID. On mismatch it names which check failed --
// the UUID check runs first, so a SessionUUID mismatch is reported even if
// TmuxOwnerMarker also disagrees.
func (id SessionIdentity) Matches(expectedSessionUUID string) (bool, diagnose.SafetyGateReason) {
	if id.SessionUUID != expectedSessionUUID {
		return false, diagnose.SafetyGateReasonIdentityMismatchInstance
	}
	if id.TmuxOwnerMarker != expectedSessionUUID {
		return false, diagnose.SafetyGateReasonIdentityMismatchTmuxMarker
	}
	return true, ""
}
