package streamhub_test

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/goleak"

	"github.com/tstapler/stapler-squad/session/streamhub"
)

// Story 1.4.2: the failure-mode test suite required by pre-mortem.md — this
// is the only rehearsal environment the project has before any real tmux
// traffic touches the hub (no staging environment for this single-operator
// instance). Every test here uses only MemoryTransport and the fake
// SessionController already defined in lifecycle_test.go — no real tmux
// process, no network socket.

// --- Hub crash / SessionController-error handling (mirrors controlModeExited) ---

func TestStreamHub_should_BroadcastStreamEndedSentinelToAllSubscribersAndAttemptRestart_When_ControlModeSubprocessExitsUnexpectedly(t *testing.T) {
	defer goleak.VerifyNone(t)

	// A subprocess exiting unexpectedly surfaces to the hub the same way any
	// other SessionController failure does: the next call the hub makes
	// against it (here, SetWindowSize during a resize) errors. hub.go's
	// handleControllerError is the single code path for both "the process
	// died" and "a call against a live process failed" — Epic 1.3's own doc
	// comment on handleControllerError notes this is the same path Story
	// 1.4.2's hub-death test exercises, since no separate restart machinery
	// exists yet (that would be later HubRegistry work): "attempt a
	// restart, or clean teardown if restart is not possible" always
	// resolves to the teardown branch today.
	wantErr := errors.New("control-mode subprocess exited unexpectedly")
	controller := newFakeSessionController()
	controller.setWindowSizeErr = wantErr
	hub := streamhub.NewStreamHub("crash-test-session", controller, streamhub.WithTeardownGrace(time.Hour))

	mt1 := streamhub.NewMemoryTransport()
	mt2 := streamhub.NewMemoryTransport()
	id1 := hub.AttachSubscriber(mt1, streamhub.SubscriberCapability{CanResize: true})
	hub.AttachSubscriber(mt2, streamhub.SubscriberCapability{CanResize: true})

	hub.RequestResize(id1, mustSize(t, 100, 30))

	if !waitFor(t, time.Second, func() bool {
		return len(mt1.ReceivedFrames()) == 1 && len(mt2.ReceivedFrames()) == 1
	}) {
		t.Fatalf("expected both subscribers to receive the stream-ended sentinel, got mt1=%d mt2=%d",
			len(mt1.ReceivedFrames()), len(mt2.ReceivedFrames()))
	}
	if !waitFor(t, time.Second, func() bool { return hub.State() == streamhub.HubTornDown }) {
		t.Fatalf("expected the hub to attempt restart-or-teardown after the simulated subprocess exit, got %v", hub.State())
	}
}

func TestStreamHub_should_BroadcastStreamEndedSentinelAndAttemptRestart_When_CapturePaneContentFailsAfterQuiescence(t *testing.T) {
	defer goleak.VerifyNone(t)

	wantErr := errors.New("capture-pane failed: pane gone")
	controller := newFakeSessionController()
	controller.captureErr = wantErr
	hub := streamhub.NewStreamHub("crash-test-session-2", controller,
		streamhub.WithTeardownGrace(time.Hour),
		streamhub.WithQuiescenceTimeout(30*time.Millisecond),
		streamhub.WithQuiescenceQuietPeriod(5*time.Millisecond),
	)

	mt1 := streamhub.NewMemoryTransport()
	mt2 := streamhub.NewMemoryTransport()
	id1 := hub.AttachSubscriber(mt1, streamhub.SubscriberCapability{CanResize: true})
	hub.AttachSubscriber(mt2, streamhub.SubscriberCapability{CanResize: true})

	hub.RequestResize(id1, mustSize(t, 90, 28))

	if !waitFor(t, time.Second, func() bool {
		return len(mt1.ReceivedFrames()) == 1 && len(mt2.ReceivedFrames()) == 1
	}) {
		t.Fatalf("expected both subscribers to receive the stream-ended sentinel, got mt1=%d mt2=%d",
			len(mt1.ReceivedFrames()), len(mt2.ReceivedFrames()))
	}
	if !waitFor(t, time.Second, func() bool { return hub.State() == streamhub.HubTornDown }) {
		t.Fatalf("expected the hub to attempt restart-or-teardown after CapturePaneContent failed, got %v", hub.State())
	}
}

// --- Transport.Send-error eviction, exercised via the exported MemoryTransport ---

func TestStreamHub_should_EvictSubscriberExactlyOnce_When_MemoryTransportConfiguredWithErrorSend(t *testing.T) {
	defer goleak.VerifyNone(t)

	hub := streamhub.NewStreamHub("send-error-test", nil, streamhub.WithTeardownGrace(time.Hour))
	sendErr := errors.New("boom")

	normal := streamhub.NewMemoryTransport()
	failing := streamhub.NewMemoryTransport(streamhub.WithErrorSend(sendErr))

	normalID := hub.AttachSubscriber(normal, streamhub.SubscriberCapability{})
	hub.AttachSubscriber(failing, streamhub.SubscriberCapability{})

	hub.Broadcast([]byte("frame"))

	if !waitFor(t, time.Second, func() bool { return hub.SubscriberCount() == 1 }) {
		t.Fatalf("expected the failing subscriber to be evicted after exactly one logged error, got SubscriberCount() == %d", hub.SubscriberCount())
	}
	if !waitFor(t, time.Second, func() bool { return len(normal.ReceivedFrames()) == 1 }) {
		t.Fatalf("expected the normal subscriber to still receive the broadcast, got %d frames", len(normal.ReceivedFrames()))
	}

	hub.DetachSubscriber(normalID)
}

// --- Sustained backpressure: eviction without stalling fast subscribers, drop counter ---

func TestStreamHub_should_EvictSlowSubscriberUnderSustainedBackpressure_And_IncrementDropsCounterExactlyOnce(t *testing.T) {
	defer goleak.VerifyNone(t)

	hub := streamhub.NewStreamHub("backpressure-test", nil,
		streamhub.WithSubscriberBufferSize(4),
		streamhub.WithSlowSubscriberGrace(20*time.Millisecond),
		streamhub.WithTeardownGrace(time.Hour),
	)

	normal := streamhub.NewMemoryTransport()
	blocked := streamhub.NewMemoryTransport(streamhub.WithBlockingSend())

	normalID := hub.AttachSubscriber(normal, streamhub.SubscriberCapability{})
	hub.AttachSubscriber(blocked, streamhub.SubscriberCapability{})

	const frameCount = 100
	start := time.Now()
	for i := 1; i <= frameCount; i++ {
		hub.Broadcast([]byte("frame"))
		if !waitFor(t, 500*time.Millisecond, func() bool { return len(normal.ReceivedFrames()) == i }) {
			t.Fatalf("expected the normal subscriber to have received %d frames by broadcast %d, got %d",
				i, i, len(normal.ReceivedFrames()))
		}
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("expected the sustained-backpressure broadcast loop to complete quickly despite a stalled subscriber, took %s", elapsed)
	}

	if !waitFor(t, time.Second, func() bool { return hub.SubscriberCount() == 1 }) {
		t.Fatalf("expected the blocked subscriber to be evicted before the loop finished, SubscriberCount() == %d", hub.SubscriberCount())
	}
	if got := hub.SlowSubscriberDropsTotal(); got != 1 {
		t.Fatalf("expected streamhub_slow_subscriber_drops_total to be incremented exactly once for the eviction (not once per dropped frame), got %d", got)
	}

	hub.DetachSubscriber(normalID)
}

// --- Flag-flip mid-session: single-owner resolution race ---
//
// StreamOwnershipLock (Epic 3.1, Story 3.1.1, session/streamhub/ownership.go)
// is the production primitive that makes this guarantee hold across package
// session and session/streamhub. Earlier phases of this plan (Epic 1.4)
// proved the same single-owner resolve-once guarantee against a test-scoped
// sync.Once stand-in before StreamOwnershipLock existed; the race test below
// now exercises the real primitive directly.

// assertOverlapInvariant calls the real, production-shipped
// streamhub.OverlapInvariant (session/streamhub/ownership.go) with a hook
// installed so a violation fails the test immediately via t.Fatal, per
// plan.md's OverlapInvariant Domain Glossary entry ("every -race test that
// could trigger it explicitly fails via t.Fatal if it fires during test
// execution"). This replaces Epic 1.4's test-local
// overlapInvariantViolated/assertOverlapInvariant stand-ins, which predated
// StreamOwnershipLock and the real OverlapInvariant implementation.
func assertOverlapInvariant(t *testing.T, sessionName string, ownerCount int) {
	t.Helper()
	violated := false
	streamhub.SetOverlapInvariantHookForTest(func(gotSession string, gotOwnerCount int) {
		violated = true
	})
	t.Cleanup(func() { streamhub.SetOverlapInvariantHookForTest(nil) })

	streamhub.OverlapInvariant(sessionName, ownerCount)
	if violated {
		t.Fatalf("OverlapInvariant violated: %d owners resolved for session %q, want at most 1", ownerCount, sessionName)
	}
}

func TestStreamOwnershipLock_should_ResolveToSingleWinner_When_ConcurrentCallersRaceFirstResolution(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping 1000-iteration race test in short mode")
	}
	defer goleak.VerifyNone(t)

	const iterations = 1000
	const racers = 8

	for i := 0; i < iterations; i++ {
		lock := streamhub.AcquireOwnershipLock(fmt.Sprintf("race-test-session-%d", i))

		var wg sync.WaitGroup
		results := make([]streamhub.StreamPath, racers)
		for g := 0; g < racers; g++ {
			wg.Add(1)
			// Alternate the requested path per goroutine so a real split
			// (if the resolver were broken) would show up as differing
			// results rather than being masked by every racer requesting
			// the same value.
			requested := g%2 == 0 // maps to PathHubOwned via Resolve(true)
			go func(idx int, flagValue bool) {
				defer wg.Done()
				results[idx] = lock.Resolve(flagValue)
			}(g, requested)
		}
		wg.Wait()

		distinct := map[streamhub.StreamPath]bool{}
		for _, r := range results {
			distinct[r] = true
		}
		assertOverlapInvariant(t, fmt.Sprintf("race-test-session-%d", i), len(distinct))
		if len(distinct) != 1 {
			t.Fatalf("iteration %d: expected every concurrent caller to observe the same resolved StreamPath, got %d distinct results: %v",
				i, len(distinct), results)
		}
	}
}

// TestOverlapInvariant_should_LogErrorAndIncrementMetric_When_TwoOwnersAreForciblySimulated
// proves the invariant check itself fires correctly by deliberately
// bypassing the resolver (never done by real callers) to construct a forced
// two-owner scenario — mirroring plan.md's AC in this project's actual,
// documented form: production always logs via slog.Error and increments
// streamhub_overlap_invariant_violations_total, and never panics (see
// plan.md's OverlapInvariant glossary entry). This test never expects or
// recovers a panic; assertOverlapInvariant (used by the race test above) is
// the t.Fatal-on-violation wrapper every -race test in this plan uses.
func TestOverlapInvariant_should_LogErrorAndIncrementMetric_When_TwoOwnersAreForciblySimulated(t *testing.T) {
	before := streamhub.OverlapInvariantViolationsTotal()

	streamhub.OverlapInvariant("forced-two-owner-session", 2)
	if got := streamhub.OverlapInvariantViolationsTotal(); got != before+1 {
		t.Fatalf("expected streamhub_overlap_invariant_violations_total to increment by 1 for a forced two-owner scenario, got %d -> %d", before, got)
	}

	streamhub.OverlapInvariant("single-owner-session", 1)
	if got := streamhub.OverlapInvariantViolationsTotal(); got != before+1 {
		t.Fatalf("expected no increment for a single resolved owner, got %d -> %d", before, got)
	}

	streamhub.OverlapInvariant("no-owner-yet-session", 0)
	if got := streamhub.OverlapInvariantViolationsTotal(); got != before+1 {
		t.Fatalf("expected no increment when no owner has resolved yet, got %d -> %d", before, got)
	}
}

// --- No goroutine leak across sustained attach/detach churn ---

func TestStreamHub_should_NotLeakGoroutines_When_1000AttachDetachCyclesRun(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping 1000-cycle goroutine-leak test in short mode")
	}
	defer goleak.VerifyNone(t)

	hub := streamhub.NewStreamHub("leak-test-session", nil, streamhub.WithTeardownGrace(time.Hour))

	const cycles = 1000
	var evicted atomic.Int64
	for i := 0; i < cycles; i++ {
		mt := streamhub.NewMemoryTransport()
		id := hub.AttachSubscriber(mt, streamhub.SubscriberCapability{})
		hub.DetachSubscriber(id)
		if !waitFor(t, time.Second, mt.IsClosed) {
			t.Fatalf("cycle %d: expected transport to be closed after DetachSubscriber", i)
		}
		evicted.Add(1)
	}

	if got := evicted.Load(); got != cycles {
		t.Fatalf("expected all %d attach/detach cycles to complete, got %d", cycles, got)
	}
	if got := hub.SubscriberCount(); got != 0 {
		t.Fatalf("expected SubscriberCount() == 0 after all cycles, got %d", got)
	}
	// goleak.VerifyNone (deferred above) proves no writer goroutine leaked
	// across the full 1000-cycle run.
}
