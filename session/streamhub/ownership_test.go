package streamhub_test

import (
	"testing"

	"github.com/tstapler/stapler-squad/session/streamhub"
)

// TestStreamOwnershipLock_should_ReturnCachedPath_When_FlagFlipsAfterFirstResolution
// is validation.md's REQ-7 happy-path scenario (plan.md Story 3.1.1 AC1): a
// session's first connection resolves with the flag true, and a later
// connection to the same session must observe the sticky PathHubOwned value
// even though the flag it now passes is false — proving the flip window
// this epic exists to close is actually closed.
func TestStreamOwnershipLock_should_ReturnCachedPath_When_FlagFlipsAfterFirstResolution(t *testing.T) {
	lock := streamhub.AcquireOwnershipLock("sticky-test-" + t.Name())

	first := lock.Resolve(true)
	if first != streamhub.PathHubOwned {
		t.Fatalf("expected first resolution to be PathHubOwned, got %v", first)
	}

	// Simulate the flag flipping to false in the environment before a later
	// connection arrives (plan.md's "10 seconds later" scenario).
	second := lock.Resolve(false)
	if second != streamhub.PathHubOwned {
		t.Fatalf("expected sticky resolution to remain PathHubOwned after flag flip, got %v", second)
	}
}

// TestStreamOwnershipLock_should_ResolveIndependently_When_TwoDifferentSessionNamesAreQueried
// is validation.md's REQ-7 integration scenario (plan.md Story 3.1.1 AC2):
// resolution is keyed per tmux session name, not a single global value, so
// two sessions can land on different StreamPaths simultaneously.
func TestStreamOwnershipLock_should_ResolveIndependently_When_TwoDifferentSessionNamesAreQueried(t *testing.T) {
	sessionA := "session-a-" + t.Name()
	sessionB := "session-b-" + t.Name()

	pathA := streamhub.AcquireOwnershipLock(sessionA).Resolve(true)
	pathB := streamhub.AcquireOwnershipLock(sessionB).Resolve(false)

	if pathA != streamhub.PathHubOwned {
		t.Fatalf("expected session-a to resolve PathHubOwned, got %v", pathA)
	}
	if pathB != streamhub.PathLegacyPerConnection {
		t.Fatalf("expected session-b to resolve PathLegacyPerConnection, got %v", pathB)
	}

	// Re-querying session-a with a different flag value must not disturb its
	// own sticky resolution, nor leak into session-b's.
	if got := streamhub.AcquireOwnershipLock(sessionA).Resolve(false); got != streamhub.PathHubOwned {
		t.Fatalf("expected session-a's resolution to remain sticky, got %v", got)
	}
	if got := streamhub.AcquireOwnershipLock(sessionB).Resolve(true); got != streamhub.PathLegacyPerConnection {
		t.Fatalf("expected session-b's resolution to remain sticky, got %v", got)
	}
}

// TestAcquireOwnershipLock_should_ReturnSameLockInstance_When_CalledWithSameSessionName
// is validation.md's REQ-8 happy-path scenario: AcquireOwnershipLock is a
// get-or-create accessor over a package-level registry, so every caller
// passing the same session name observes the same *StreamOwnershipLock
// instance — the precondition Story 3.1.2's mutual-exclusion guarantee
// depends on.
func TestAcquireOwnershipLock_should_ReturnSameLockInstance_When_CalledWithSameSessionName(t *testing.T) {
	sessionName := "exclusive-test-" + t.Name()

	first := streamhub.AcquireOwnershipLock(sessionName)
	second := streamhub.AcquireOwnershipLock(sessionName)

	if first != second {
		t.Fatalf("expected AcquireOwnershipLock to return the same instance for the same session name")
	}

	// Prove it's genuinely shared: resolving through one reference is
	// visible through the other.
	first.Resolve(true)
	if got := second.Resolve(false); got != streamhub.PathHubOwned {
		t.Fatalf("expected shared lock instance to see the resolution made via the other reference, got %v", got)
	}
}
