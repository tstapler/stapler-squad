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
	// ScrollGateNoActiveController means no ClaudeController/PiStatusSource is
	// active for inst, so DetectedStatus carries no live signal at all --
	// distinct from ScrollGateUnsafeStatus, where a controller is active but
	// reported a specific unsafe status.
	ScrollGateNoActiveController
	// ScrollGateUnsafeStatus means inst's DetectedStatus, with a controller
	// active, is not one of the idle-equivalent statuses AppScrollGate
	// allowlists (see its doc comment).
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
// (3) a status controller is active and its DetectedStatus is idle-equivalent,
// and (4) exactly one subscriber is connected.
//
// Check (3) allowlists idle-equivalent statuses rather than denylisting
// unsafe ones, so a new DetectedStatus value is unsafe by default --
// forwarding mid-turn (StatusExecuting/NeedsApproval/InputRequired) is the
// highest-severity concurrency race in research/pitfalls.md §1. The
// allowlist is isIdleStatus's {StatusIdle, StatusReady, StatusSuccess} --
// the same "Claude is at rest, safe to interact with" set AutonomousDriver
// already uses to decide it's safe to inject the next turn's input, a more
// sensitive operation than a single scroll keystroke (session/autonomous_driver.go)
// -- plus StatusUnknown. StatusUnknown belongs alongside StatusReady because
// MatchLines' `.*` ready-prompt catch-all (session/detection/pattern_set.go)
// deliberately reports it as StatusUnknown, not StatusReady, so the status
// badge stays blank for the common case; live testing (manual instance,
// 2026-09-21) confirmed both matter in practice -- a fresh/reconnected
// session sits at the StatusUnknown catch-all, while a session that just
// finished a turn sits at StatusSuccess indefinitely (Claude Code's "Churned
// for Ns · done HH:MM" banner persists on screen until the next turn, so
// MatchLines keeps matching Success on every redraw). StatusUnknown is ALSO
// what GetDetectedStatus returns when NO controller is active at all -- an
// unrelated meaning that would be unsafe to allowlist -- so this check reads
// GetDetectedStatusInfo's controllerActive flag first and rejects outright
// (ScrollGateNoActiveController) when it's false, before StatusUnknown is
// ever treated as safe.
//
// subscriberCount is passed in (not queried internally) to keep this file
// free of a streamhub import; PathLegacyPerConnection callers pass a -1
// sentinel to force check (4) to fail as the distinct
// ScrollGateUnsupportedPath reason rather than ScrollGateTooManyViewers, so a
// solo legacy-path user never sees "another viewer connected" copy.
//
// reason is for logs only; branch on ScrollGateFailure instead.
func AppScrollGate(inst *Instance, subscriberCount int) (ok bool, failure ScrollGateFailure, reason string) {
	if resolveScrollAdapter(inst.GetProgram()) == nil {
		return false, ScrollGateNoCapability, fmt.Sprintf("no scroll capability for program: %s", inst.GetProgram())
	}
	if !inst.GetAltScreenActive() {
		return false, ScrollGateNotAltScreen, "not in alternate screen buffer"
	}
	status, controllerActive := inst.GetDetectedStatusInfo()
	if !controllerActive {
		return false, ScrollGateNoActiveController, "no active status controller"
	}
	if !isIdleStatus(status) && status != detection.StatusUnknown {
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
// snake_case-ish label used in AppScrollGate's rejection reasons. Every
// non-safe DetectedStatus value gets its own explicit case so the log can
// tell them apart -- StatusUnknown never reaches here (AppScrollGate
// allowlists it), so "unknown" in the default case means a future
// DetectedStatus value this function hasn't been updated for yet.
func detectedStatusScrollGateLabel(status detection.DetectedStatus) string {
	switch status {
	case detection.StatusReady:
		return "ready"
	case detection.StatusProcessing:
		return "processing"
	case detection.StatusNeedsApproval:
		return "needs_approval"
	case detection.StatusInputRequired:
		return "input_required"
	case detection.StatusError:
		return "error"
	case detection.StatusTestsFailing:
		return "tests_failing"
	case detection.StatusIdle:
		return "idle"
	case detection.StatusExecuting:
		return "executing"
	case detection.StatusSuccess:
		return "success"
	case detection.StatusWaitingForAgent:
		return "waiting_for_agent"
	case detection.StatusCompacting:
		return "compacting"
	default:
		return "unknown"
	}
}
