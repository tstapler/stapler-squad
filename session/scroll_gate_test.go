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
