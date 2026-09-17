package session

import (
	"fmt"

	"github.com/tstapler/stapler-squad/session/detection"
)

// ScrollGateFailure is a closed, typed classification of why AppScrollGate
// rejected a scroll-forward request -- callers (mapGateReason) switch on
// this instead of string-matching AppScrollGate's human-readable reason,
// which is for logs only and not a stable comparison key (Fix 1,
// adversarial-review BLOCKER: see AppScrollGate's doc comment).
type ScrollGateFailure int

const (
	// ScrollGateOK means AppScrollGate passed every check.
	ScrollGateOK ScrollGateFailure = iota
	// ScrollGateNoCapability means inst's program has no registered
	// ScrollAdapter.
	ScrollGateNoCapability
	// ScrollGateNotAltScreen means the pane is not in the alternate screen
	// buffer.
	ScrollGateNotAltScreen
	// ScrollGateUnsafeStatus means inst's DetectedStatus is not StatusIdle.
	ScrollGateUnsafeStatus
	// ScrollGateUnsupportedPath means subscriberCount was the
	// PathLegacyPerConnection sentinel (< 0), not a genuine multi-viewer
	// count.
	ScrollGateUnsupportedPath
	// ScrollGateTooManyViewers means subscriberCount was a genuine value
	// greater than one.
	ScrollGateTooManyViewers
)

// AppScrollGate reports whether it is currently safe to forward a scroll
// keystroke to inst, composing four checks in order: (1) a ScrollAdapter
// exists for inst's program, (2) the pane is in the alternate screen buffer,
// (3) DetectedStatus is exactly StatusIdle, and (4) exactly one subscriber is
// connected.
//
// Check (3) is an allowlist of one, not a denylist of unsafe statuses:
// StatusExecuting ("actively executing commands mid-turn") is unsafe, same
// as StatusNeedsApproval/StatusInputRequired -- forwarding a scroll keystroke
// while the agent is mid-turn is the highest-severity concurrency race
// identified in research/pitfalls.md §1 and the opposite of what
// research/ux.md §4c recommends (bias toward not forwarding while output is
// actively streaming). Every other DetectedStatus value is unsafe by
// construction too, since the check is `== StatusIdle`, not a maintained
// exclusion list -- a new status added later is unsafe by default.
//
// subscriberCount is passed in rather than queried internally, since the
// caller (Epic 1.3) already has it from the hub/legacy path and this keeps
// scroll_gate.go free of a streamhub import. PathLegacyPerConnection callers
// pass a -1 sentinel to guarantee check (4) fails, which is reported as the
// distinct ScrollGateUnsupportedPath failure rather than
// ScrollGateTooManyViewers -- the caller (mapGateReason) needs the two
// disambiguated, since a solo PathLegacyPerConnection user must never see
// "another viewer connected" copy.
//
// reason is a human-readable string for logs only; callers that need to
// branch on the failure must switch on the returned ScrollGateFailure
// instead.
func AppScrollGate(inst *Instance, subscriberCount int) (ok bool, failure ScrollGateFailure, reason string) {
	if resolveScrollAdapter(inst.GetProgram()) == nil {
		return false, ScrollGateNoCapability, fmt.Sprintf("no scroll capability for program: %s", inst.GetProgram())
	}
	if !inst.GetAltScreenActive() {
		return false, ScrollGateNotAltScreen, "not in alternate screen buffer"
	}
	if status := inst.GetDetectedStatus(); status != detection.StatusIdle {
		return false, ScrollGateUnsafeStatus, "unsafe detected status: " + detectedStatusScrollGateLabel(status)
	}
	if subscriberCount != 1 {
		if subscriberCount < 0 {
			return false, ScrollGateUnsupportedPath, "unsupported streaming path"
		}
		return false, ScrollGateTooManyViewers, "multiple viewers connected"
	}
	return true, ScrollGateOK, ""
}

// detectedStatusScrollGateLabel maps a DetectedStatus to the lowercase,
// snake_case-ish label used in AppScrollGate's rejection reasons.
func detectedStatusScrollGateLabel(status detection.DetectedStatus) string {
	switch status {
	case detection.StatusNeedsApproval:
		return "needs_approval"
	case detection.StatusInputRequired:
		return "input_required"
	case detection.StatusExecuting:
		return "executing"
	case detection.StatusIdle:
		return "idle"
	default:
		return "unknown"
	}
}
