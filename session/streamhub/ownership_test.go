package streamhub_test

import (
	"errors"
	"fmt"
	"sync"
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

// TestHubRegistry_should_JoinLegacyPathExplicitly_When_LockAlreadyResolvedLegacyPerConnection
// is validation.md's REQ-8 error/edge-path scenario (plan.md Task 3.1.2b):
// once a session's StreamOwnershipLock has already resolved
// PathLegacyPerConnection (a legacy StartControlMode call got there first),
// a hub-creation attempt (ResolveExpecting(true, PathHubOwned) — exactly
// what server/services' HubRegistry.GetOrCreate calls before creating a
// hub) must not silently succeed or reinterpret the resolution as its own;
// it must fail explicitly with ErrOwnershipResolvedToOtherPath so the
// caller joins the legacy path instead of creating a competing hub.
func TestHubRegistry_should_JoinLegacyPathExplicitly_When_LockAlreadyResolvedLegacyPerConnection(t *testing.T) {
	sessionName := "exclusive-test-" + t.Name()

	// The legacy path gets there first and resolves the lock.
	first := streamhub.AcquireOwnershipLock(sessionName).Resolve(false)
	if first != streamhub.PathLegacyPerConnection {
		t.Fatalf("expected first resolution to be PathLegacyPerConnection, got %v", first)
	}

	// Hub creation's intent-asserting resolve must refuse rather than
	// silently reinterpret this session as PathHubOwned.
	got, err := streamhub.AcquireOwnershipLock(sessionName).ResolveExpecting(true, streamhub.PathHubOwned)
	if !errors.Is(err, streamhub.ErrOwnershipResolvedToOtherPath) {
		t.Fatalf("expected ErrOwnershipResolvedToOtherPath, got %v", err)
	}
	if got != streamhub.PathLegacyPerConnection {
		t.Fatalf("expected ResolveExpecting to still report the actual resolved path so the caller can join it, got %v", got)
	}
}

// TestStreamOwnershipLock_should_SucceedResolveExpecting_When_RequestedPathMatchesResolution
// is ResolveExpecting's happy-path counterpart: when the caller's intended
// path matches what actually resolved (win or first-resolver), no error is
// returned.
func TestStreamOwnershipLock_should_SucceedResolveExpecting_When_RequestedPathMatchesResolution(t *testing.T) {
	sessionName := "resolve-expecting-happy-" + t.Name()

	got, err := streamhub.AcquireOwnershipLock(sessionName).ResolveExpecting(true, streamhub.PathHubOwned)
	if err != nil {
		t.Fatalf("expected no error when the requested path matches resolution, got %v", err)
	}
	if got != streamhub.PathHubOwned {
		t.Fatalf("expected PathHubOwned, got %v", got)
	}
}

// TestStreamOwnershipLock_should_ForcePathHubOwned_When_SessionOverrideIsSet
// is plan.md Story 3.3.1's AC1 scenario: with a per-session override
// installed via SetSessionOverrideLookup forcing session "canary-1" to
// PathHubOwned, the first connection to that session resolves PathHubOwned
// even though the global flag value passed is false, while a simultaneous
// connection to a different, non-overridden session resolves
// PathLegacyPerConnection from the same global flag.
func TestStreamOwnershipLock_should_ForcePathHubOwned_When_SessionOverrideIsSet(t *testing.T) {
	canarySession := "canary-1-" + t.Name()
	normalSession := "normal-1-" + t.Name()

	streamhub.SetSessionOverrideLookup(func(sessionName string) (bool, bool) {
		if sessionName == canarySession {
			return true, true
		}
		return false, false
	})
	t.Cleanup(func() { streamhub.SetSessionOverrideLookup(nil) })

	// Global flag is false for both; only the canary session has an override.
	canaryPath := streamhub.AcquireOwnershipLock(canarySession).Resolve(false)
	if canaryPath != streamhub.PathHubOwned {
		t.Fatalf("expected overridden session to resolve PathHubOwned, got %v", canaryPath)
	}

	normalPath := streamhub.AcquireOwnershipLock(normalSession).Resolve(false)
	if normalPath != streamhub.PathLegacyPerConnection {
		t.Fatalf("expected non-overridden session to resolve PathLegacyPerConnection, got %v", normalPath)
	}
}

// TestStreamOwnershipLock_should_IgnoreOverride_When_NoLookupIsInstalled
// verifies the zero-value/backwards-compatible behavior: with no
// SetSessionOverrideLookup call in effect (nil, the package default),
// Resolve behaves exactly as it did before Story 3.3.1.
func TestStreamOwnershipLock_should_IgnoreOverride_When_NoLookupIsInstalled(t *testing.T) {
	sessionName := "no-override-" + t.Name()

	got := streamhub.AcquireOwnershipLock(sessionName).Resolve(false)
	if got != streamhub.PathLegacyPerConnection {
		t.Fatalf("expected PathLegacyPerConnection with no override lookup installed, got %v", got)
	}
}

// TestStreamOwnershipLock_should_KeepOverrideStickyResolution_When_LookupIsClearedLater
// verifies the override only affects the *first* resolution, like the global
// flag: once resolved via an override, clearing the lookup afterward must
// not disturb the already-sticky PathHubOwned resolution.
func TestStreamOwnershipLock_should_KeepOverrideStickyResolution_When_LookupIsClearedLater(t *testing.T) {
	sessionName := "sticky-override-" + t.Name()

	streamhub.SetSessionOverrideLookup(func(name string) (bool, bool) { return true, true })
	first := streamhub.AcquireOwnershipLock(sessionName).Resolve(false)
	if first != streamhub.PathHubOwned {
		t.Fatalf("expected PathHubOwned from override, got %v", first)
	}

	streamhub.SetSessionOverrideLookup(nil)
	second := streamhub.AcquireOwnershipLock(sessionName).Resolve(false)
	if second != streamhub.PathHubOwned {
		t.Fatalf("expected sticky PathHubOwned to survive override removal, got %v", second)
	}
}

// TestStreamOwnershipLock_should_NeverProduceTwoOwners_When_HubAndLegacyIntentsRaceConcurrently
// is plan.md Story 3.1.2 AC2's race scenario at the StreamOwnershipLock
// primitive level: many goroutines concurrently call ResolveExpecting with
// opposing intents (hub creation vs. legacy start) for the same session
// name. Exactly one intent must win per session (zero error) and the loser
// side must uniformly observe ErrOwnershipResolvedToOtherPath — never a mix
// where both sides believe they won, which is what "two owners" would look
// like at this layer.
func TestStreamOwnershipLock_should_NeverProduceTwoOwners_When_HubAndLegacyIntentsRaceConcurrently(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping 1000-iteration race test in short mode")
	}

	const iterations = 1000
	const racersPerSide = 8

	for i := 0; i < iterations; i++ {
		sessionName := fmt.Sprintf("ownership-race-hub-vs-legacy-%d", i)

		var wg sync.WaitGroup
		hubResults := make([]error, racersPerSide)
		legacyResults := make([]error, racersPerSide)

		for g := 0; g < racersPerSide; g++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				_, err := streamhub.AcquireOwnershipLock(sessionName).ResolveExpecting(true, streamhub.PathHubOwned)
				hubResults[idx] = err
			}(g)
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				_, err := streamhub.AcquireOwnershipLock(sessionName).ResolveExpecting(false, streamhub.PathLegacyPerConnection)
				legacyResults[idx] = err
			}(g)
		}
		wg.Wait()

		hubWins, legacyWins := 0, 0
		for _, err := range hubResults {
			if err == nil {
				hubWins++
			}
		}
		for _, err := range legacyResults {
			if err == nil {
				legacyWins++
			}
		}

		if hubWins > 0 && legacyWins > 0 {
			t.Fatalf("iteration %d: OverlapInvariant violated — both hub (%d) and legacy (%d) intents won for the same session", i, hubWins, legacyWins)
		}
		if hubWins+legacyWins == 0 {
			t.Fatalf("iteration %d: expected exactly one side to win, but neither did", i)
		}
		if hubWins != 0 && hubWins != racersPerSide {
			t.Fatalf("iteration %d: expected all %d hub-intent racers to agree, got %d wins", i, racersPerSide, hubWins)
		}
		if legacyWins != 0 && legacyWins != racersPerSide {
			t.Fatalf("iteration %d: expected all %d legacy-intent racers to agree, got %d wins", i, racersPerSide, legacyWins)
		}
	}
}
