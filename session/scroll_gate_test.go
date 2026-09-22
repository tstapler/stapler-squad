package session

import (
	"testing"

	"github.com/tstapler/stapler-squad/session/detection"
)

// newScrollGateTestInstance builds an *Instance with Program/AltScreenActive
// set directly and DetectedStatus driven via a PiStatusSource registered on
// a fresh InstanceStatusManager -- the lightest existing seam for making
// Instance.GetDetectedStatus() return an arbitrary status in a unit test
// (see instance_status_test.go's TestInstanceStatusManager_GetStatus_PiFallback
// for the same pattern). The program value is independent of this mechanism:
// it only decides what resolveScrollAdapter sees.
func newScrollGateTestInstance(t *testing.T, program string, altScreenActive bool, status detection.DetectedStatus) *Instance {
	t.Helper()
	inst := &Instance{Title: t.Name(), Program: program, AltScreenActive: altScreenActive}

	src := NewPiStatusSource(inst.Title, nil)
	src.status.Store(int32(status))

	mgr := NewInstanceStatusManager()
	mgr.RegisterPiStatusSource(inst.Title, src)
	inst.SetStatusManager(mgr)

	return inst
}

func TestAppScrollGate_should_ReturnTrue_When_CapabilityAltScreenIdleAndSingleSubscriberAllPass(t *testing.T) {
	inst := newScrollGateTestInstance(t, "claude", true, detection.StatusIdle)

	ok, failure, reason := AppScrollGate(inst, 1)

	if !ok || failure != ScrollGateOK || reason != "" {
		t.Fatalf("AppScrollGate() = (%v, %v, %q), want (true, ScrollGateOK, \"\")", ok, failure, reason)
	}
}

// TestAppScrollGate_should_ReturnTrue_When_DetectedStatusIsUnknownWithActiveController
// covers Claude Code's ordinary resting prompt: MatchLines's `.*` ready
// catch-all (session/detection/pattern_set.go) deliberately reports that as
// StatusUnknown, not StatusIdle, so the gate must allowlist StatusUnknown
// too whenever a controller is confirmed active -- otherwise every
// scroll-forward request against a genuinely idle Claude session is
// rejected (the bug this test guards against).
func TestAppScrollGate_should_ReturnTrue_When_DetectedStatusIsUnknownWithActiveController(t *testing.T) {
	inst := newScrollGateTestInstance(t, "claude", true, detection.StatusUnknown)

	ok, failure, reason := AppScrollGate(inst, 1)

	if !ok || failure != ScrollGateOK || reason != "" {
		t.Fatalf("AppScrollGate() = (%v, %v, %q), want (true, ScrollGateOK, \"\")", ok, failure, reason)
	}
}

// TestAppScrollGate_should_ReturnTrue_When_DetectedStatusIsSuccess covers the
// other real resting state found via live manual testing (2026-09-21):
// Claude Code's "Churned for Ns · done HH:MM" completion banner persists on
// screen after a turn finishes, so MatchLines keeps matching StatusSuccess
// indefinitely until the next turn -- a session that has ever completed a
// turn sits here, not at StatusIdle/StatusUnknown, until it's interacted
// with again. AutonomousDriver's isIdleStatus already treats StatusSuccess
// as idle-equivalent for the more sensitive operation of injecting real
// input (session/autonomous_driver.go); the scroll gate must match.
func TestAppScrollGate_should_ReturnTrue_When_DetectedStatusIsSuccess(t *testing.T) {
	inst := newScrollGateTestInstance(t, "claude", true, detection.StatusSuccess)

	ok, failure, reason := AppScrollGate(inst, 1)

	if !ok || failure != ScrollGateOK || reason != "" {
		t.Fatalf("AppScrollGate() = (%v, %v, %q), want (true, ScrollGateOK, \"\")", ok, failure, reason)
	}
}

// TestAppScrollGate_should_ReturnFalseWithNoActiveControllerReason_When_NoControllerIsRegistered
// covers the other source of StatusUnknown: GetDetectedStatus also returns
// it when no ClaudeController/PiStatusSource is active at all, which must
// stay unsafe even though bare StatusUnknown is now allowlisted above.
func TestAppScrollGate_should_ReturnFalseWithNoActiveControllerReason_When_NoControllerIsRegistered(t *testing.T) {
	inst := &Instance{Title: t.Name(), Program: "claude", AltScreenActive: true}
	inst.SetStatusManager(NewInstanceStatusManager())

	ok, failure, reason := AppScrollGate(inst, 1)

	if ok || failure != ScrollGateNoActiveController || reason != "no active status controller" {
		t.Fatalf("AppScrollGate() = (%v, %v, %q), want (false, ScrollGateNoActiveController, \"no active status controller\")", ok, failure, reason)
	}
}

func TestAppScrollGate_should_ReturnFalseWithUnsafeStatusReason_When_DetectedStatusIsExecuting(t *testing.T) {
	inst := newScrollGateTestInstance(t, "claude", true, detection.StatusExecuting)

	ok, failure, reason := AppScrollGate(inst, 1)

	if ok || failure != ScrollGateUnsafeStatus || reason != "unsafe detected status: executing" {
		t.Fatalf("AppScrollGate() = (%v, %v, %q), want (false, ScrollGateUnsafeStatus, \"unsafe detected status: executing\")", ok, failure, reason)
	}
}

func TestAppScrollGate_should_ReturnFalseWithUnsafeStatusReason_When_DetectedStatusIsNeedsApproval(t *testing.T) {
	inst := newScrollGateTestInstance(t, "claude", true, detection.StatusNeedsApproval)

	ok, failure, reason := AppScrollGate(inst, 1)

	if ok || failure != ScrollGateUnsafeStatus || reason != "unsafe detected status: needs_approval" {
		t.Fatalf("AppScrollGate() = (%v, %v, %q), want (false, ScrollGateUnsafeStatus, \"unsafe detected status: needs_approval\")", ok, failure, reason)
	}
}

// TestAppScrollGate_should_ReturnTooManyViewersFailure_When_SubscriberCountIsTwoRegardlessOfConnectionOrigin
// documents the current, accepted behavior (Risk Control's "Decision --
// per-connection, not per-user identity"): AppScrollGate has no
// user-identity parameter at all, only subscriberCount int, so it cannot
// and does not distinguish "two different people" from "one person, two
// tabs" -- both produce the identical ScrollGateTooManyViewers failure.
// Consolidates what used to be two near-duplicate tests
// (TestAppScrollGate_should_ReturnFalseWithMultipleViewersReason_When_SubscriberCountIsGreaterThanOne
// and this one) now that the file is being touched for Fix 1 -- the second
// added no incremental coverage over the first.
func TestAppScrollGate_should_ReturnTooManyViewersFailure_When_SubscriberCountIsTwoRegardlessOfConnectionOrigin(t *testing.T) {
	inst := newScrollGateTestInstance(t, "claude", true, detection.StatusIdle)

	ok, failure, reason := AppScrollGate(inst, 2)

	if ok || failure != ScrollGateTooManyViewers || reason != "multiple viewers connected" {
		t.Fatalf("AppScrollGate() = (%v, %v, %q), want (false, ScrollGateTooManyViewers, \"multiple viewers connected\")", ok, failure, reason)
	}
}

func TestAppScrollGate_should_ReturnFalse_When_NotInAltScreen(t *testing.T) {
	inst := newScrollGateTestInstance(t, "claude", false, detection.StatusIdle)

	ok, failure, reason := AppScrollGate(inst, 1)

	if ok || failure != ScrollGateNotAltScreen || reason != "not in alternate screen buffer" {
		t.Fatalf("AppScrollGate() = (%v, %v, %q), want (false, ScrollGateNotAltScreen, \"not in alternate screen buffer\")", ok, failure, reason)
	}
}

func TestAppScrollGate_should_ReturnFalse_When_ProgramHasNoScrollCapability(t *testing.T) {
	inst := newScrollGateTestInstance(t, "bash", true, detection.StatusIdle)

	ok, failure, reason := AppScrollGate(inst, 1)

	if ok || failure != ScrollGateNoCapability || reason != "no scroll capability for program: bash" {
		t.Fatalf("AppScrollGate() = (%v, %v, %q), want (false, ScrollGateNoCapability, \"no scroll capability for program: bash\")", ok, failure, reason)
	}
}

func TestAppScrollGate_should_ReturnUnsupportedPathFailure_When_LegacyPerConnectionPathPassesMinusOneSentinel(t *testing.T) {
	inst := newScrollGateTestInstance(t, "claude", true, detection.StatusIdle)

	ok, failure, reason := AppScrollGate(inst, -1)

	if ok || failure != ScrollGateUnsupportedPath || reason != "unsupported streaming path" {
		t.Fatalf("AppScrollGate() = (%v, %v, %q), want (false, ScrollGateUnsupportedPath, \"unsupported streaming path\")", ok, failure, reason)
	}
}
