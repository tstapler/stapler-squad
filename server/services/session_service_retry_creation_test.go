package services

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
)

// Epic 3.3 (async-session-creation): RetrySessionCreation RPC. See
// project_plans/async-session-creation/implementation/plan.md's Story 3.3.1
// and validation.md's "Epic 3.3" test mapping.

// newRetryTestInstance drives a session through the REAL CreateSession path
// (unlike newCancelTestInstance's bare session.NewInstance, which Cancel's
// tests can get away with since Cancel never re-enters the pipeline) and
// waits for the original attempt's Background Resolution Pipeline to reach
// Active. RetrySessionCreation re-spawns that same pipeline against an
// instance that must therefore be fully wired exactly like a real
// CreateSession-produced instance (registry, storage, poller, wireCallbacks)
// -- exercising the pipeline against an under-initialized hand-built
// instance is what a bare session.NewInstance fixture would do, and it
// panics deep in Instance.Start()'s post-start wiring.
func newRetryTestInstance(t *testing.T, svc *SessionService, storage *session.Storage, title string) *session.Instance {
	t.Helper()
	// A Registry is required for CreateManagedInstance to wrap the new
	// instance in a session.LiveInstance (session/create_managed_instance.go
	// only does so `if params.Registry != nil`) -- without it there is no
	// actor goroutine serializing this instance's concurrent state access,
	// and both TryForceStatusIfEpoch (the retried pipeline's own terminal
	// write) and TryStartRetry (the double-click race test) rely on
	// actor-mailbox ordering for their atomicity guarantees, exactly like
	// newCancelTestInstance's wrapLive parameter documents for Cancel's own
	// race test.
	if svc.registry == nil {
		svc.SetRegistry(session.NewRegistry(storage, nil))
	}
	resp, err := svc.CreateSession(context.Background(), connect.NewRequest(&sessionv1.CreateSessionRequest{
		Title:   title,
		Path:    t.TempDir(),
		Program: "claude",
	}))
	require.NoError(t, err)
	t.Cleanup(func() { destroyCreatedSession(t, svc, resp.Msg.Session.Id) })

	inst := svc.FindLiveInstance(title)
	require.NotNil(t, inst)

	// Polls via the lock-free GetStatus()/Snapshot() path, not
	// StatusAndFailureReason(): this instance's pipeline goroutine is still
	// running concurrently at this point, and GetStatus() reads the
	// atomically-published snapshot every writer already synchronizes
	// through, matching the pattern every other pipeline test in this
	// package uses to poll status concurrently with a live pipeline
	// goroutine (e.g.
	// TestBackgroundResolutionPipeline_should_ContinueRunning_When_RPCContextIsCanceled).
	require.Eventually(t, func() bool {
		return session.Status(inst.GetStatus()) == session.Active
	}, 10*time.Second, 20*time.Millisecond, "precondition: original CreateSession attempt must reach Active before this test forces it to Failed")

	return inst
}

// TestRetrySessionCreation_should_TransitionFailedToCreating_When_SameInstanceRetried
// covers Story 3.3.1's happy path: a Failed instance is retried in place --
// same instance ID, exactly zero additional SessionCreatedEvents are
// published (a retry only ever produces SessionUpdatedEvents), and the
// re-spawned pipeline reaches a terminal status (Active or Failed again)
// end to end.
func TestRetrySessionCreation_should_TransitionFailedToCreating_When_SameInstanceRetried(t *testing.T) {
	storage := createTestStorage(t)
	svc := newCreateTestService(t, storage)
	inst := newRetryTestInstance(t, svc, storage, "retry-happy-path")

	// ForceStatus, not TryForceStatusIfEpoch: the latter only ever permits a
	// transition FROM Creating (or to its own already-current target) --
	// exactly the terminal-write fencing Story 2.2.3 needs in production --
	// so it cannot move an Active fixture instance to Failed for this test's
	// setup. ForceStatus is the unconditional, non-epoch-gated setter
	// (session/instance_state.go) meant for exactly this kind of direct
	// test-fixture status injection.
	inst.ForceStatus(session.Failed)

	ch, subID := svc.eventBus.Subscribe(context.Background())
	defer svc.eventBus.Unsubscribe(subID)

	resp, err := svc.RetrySessionCreation(context.Background(), connect.NewRequest(&sessionv1.RetrySessionCreationRequest{
		Id: "retry-happy-path",
	}))
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.True(t, resp.Msg.Success)

	// Drain events published in response to the retry call for a short
	// window: must see at least one SessionUpdatedEvent and exactly zero
	// SessionCreatedEvents.
	var sawUpdated bool
	var createdCount int
	drainDeadline := time.After(1 * time.Second)
drain:
	for {
		select {
		case ev := <-ch:
			switch ev.Type {
			case events.EventSessionCreated:
				createdCount++
			case events.EventSessionUpdated:
				sawUpdated = true
			}
		case <-drainDeadline:
			break drain
		}
	}
	assert.True(t, sawUpdated, "expected at least one SessionUpdatedEvent for the retry")
	assert.Equal(t, 0, createdCount, "retry must never publish a second SessionCreatedEvent")

	// The re-spawned pipeline must reach a terminal status end to end --
	// Active (it re-ran the same real CreateSession-equivalent startup that
	// already succeeded once for this instance) -- never hang at Creating.
	// GetStatus(), not StatusAndFailureReason() -- see newRetryTestInstance's
	// doc comment: this polls concurrently with the just-respawned pipeline
	// goroutine, and this fixture's instance has no live actor to make
	// StatusAndFailureReason's read safe.
	require.Eventually(t, func() bool {
		status := session.Status(inst.GetStatus())
		return status == session.Active || status == session.Failed
	}, 10*time.Second, 20*time.Millisecond, "retried pipeline must reach a terminal status")
}

// TestRetrySessionCreation_should_ReturnFailedPrecondition_When_StatusIsNotFailed
// covers Story 3.3.1's rejection path: retry only ever applies to a Failed
// instance, and a non-Failed instance must be left untouched.
func TestRetrySessionCreation_should_ReturnFailedPrecondition_When_StatusIsNotFailed(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	svc := newCreateTestService(t, storage)
	inst := newRetryTestInstance(t, svc, storage, "retry-not-failed")

	status, _ := inst.StatusAndFailureReason()
	require.Equal(t, session.Active, status, "precondition: fixture instance is Active, not Failed")

	_, err := svc.RetrySessionCreation(context.Background(), connect.NewRequest(&sessionv1.RetrySessionCreationRequest{
		Id: "retry-not-failed",
	}))
	require.Error(t, err)
	connectErr, ok := err.(*connect.Error)
	require.True(t, ok, "expected *connect.Error, got %T", err)
	assert.Equal(t, connect.CodeFailedPrecondition, connectErr.Code())

	// Instance must be untouched: still present, still Active.
	require.NotNil(t, svc.reviewQueuePoller.FindInstance("retry-not-failed"))
	status, _ = inst.StatusAndFailureReason()
	assert.Equal(t, session.Active, status)
}

// TestRetrySessionCreation_should_SpawnExactlyOnePipeline_When_CalledConcurrently
// covers Task 3.3.1d: two concurrent RetrySessionCreation calls against the
// same Failed instance (impatient double-click) must resolve deterministically
// to exactly one success (which spawns the one live pipeline) and one
// FailedPrecondition (which spawns nothing) -- mirroring Task 3.2.1c's
// bump-then-check shape for Cancel, backed here by TryStartRetry's own
// Status==Failed mailbox check (Epic 1.2). Run with -race -count=50.
func TestRetrySessionCreation_should_SpawnExactlyOnePipeline_When_CalledConcurrently(t *testing.T) {
	storage := createTestStorage(t)
	svc := newCreateTestService(t, storage)
	inst := newRetryTestInstance(t, svc, storage, "retry-race-test")
	inst.ForceStatus(session.Failed)

	// pipelineEntries counts how many times the re-spawned pipeline actually
	// reaches its "Resolving defaults..." phase -- the first setPhase call
	// common to every pipeline invocation, GitHub-URL or not (see
	// runBackgroundResolutionPipeline) -- so a second, erroneously-spawned
	// pipeline goroutine would be caught even if TryStartRetry's own
	// exactly-one-winner guarantee somehow regressed.
	var pipelineEntries atomic.Int32
	svc.creationPhaseHook = func(msg string) {
		if msg == "Resolving defaults..." {
			pipelineEntries.Add(1)
		}
	}

	var wg sync.WaitGroup
	results := make(chan error, 2)
	wg.Add(2)
	for range 2 {
		go func() {
			defer wg.Done()
			_, err := svc.RetrySessionCreation(context.Background(), connect.NewRequest(&sessionv1.RetrySessionCreationRequest{
				Id: "retry-race-test",
			}))
			results <- err
		}()
	}
	wg.Wait()
	close(results)

	var successes, failedPreconditions int
	for err := range results {
		if err == nil {
			successes++
			continue
		}
		connectErr, ok := err.(*connect.Error)
		require.True(t, ok, "expected *connect.Error, got %T", err)
		assert.Equal(t, connect.CodeFailedPrecondition, connectErr.Code())
		failedPreconditions++
	}
	assert.Equal(t, 1, successes, "exactly one concurrent retry call must succeed")
	assert.Equal(t, 1, failedPreconditions, "exactly one concurrent retry call must observe FailedPrecondition")

	// GetStatus(), not StatusAndFailureReason() -- same no-live-actor
	// rationale as newRetryTestInstance's doc comment: this polls
	// concurrently with the winning call's freshly-spawned pipeline goroutine.
	require.Eventually(t, func() bool {
		status := session.Status(inst.GetStatus())
		return status == session.Active || status == session.Failed
	}, 10*time.Second, 20*time.Millisecond, "the one spawned pipeline must reach a terminal status")

	assert.Equal(t, int32(1), pipelineEntries.Load(), "exactly one pipeline goroutine must have run for this instance")
}
