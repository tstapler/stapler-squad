package services

import (
	"context"
	"sync"
	"testing"
	"time"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
)

// Epic 3.2 (async-session-creation): CancelSessionCreation RPC. See
// project_plans/async-session-creation/implementation/plan.md's Story 3.2.1
// and validation.md's "Epic 3.2" test mapping.

// newCancelTestInstance builds a *session.Instance registered with both
// storage (so CancelSessionCreation's storage.DeleteInstance call has a row
// to remove) and svc's live poller (so FindLiveInstance resolves it), mirroring
// newAwaitTestInstance (session_creation_await_test.go) but additionally
// persisting to storage — CancelSessionCreation, unlike AwaitCreationTerminal,
// deletes the storage row on success. wrapLive controls whether the instance
// is wrapped in a session.LiveInstance: the race test needs the actor mailbox
// for a real ordering guarantee between BumpCreationEpoch and a concurrent
// TryForceStatusIfEpoch; the simpler unit tests don't race anything and use
// the unlocked direct-call fallback sendSyncErr takes with no live actor
// registered.
func newCancelTestInstance(t *testing.T, svc *SessionService, storage *session.Storage, title string, wrapLive bool) *session.Instance {
	t.Helper()
	inst, err := session.NewInstance(session.InstanceOptions{
		Title:   title,
		Path:    t.TempDir(),
		Program: "claude",
	})
	require.NoError(t, err)
	require.NoError(t, storage.AddInstance(inst))
	if wrapLive {
		live := session.NewLiveInstance(inst)
		t.Cleanup(live.Stop)
	}
	svc.reviewQueuePoller.AddInstance(inst)
	return inst
}

// TestCancelSessionCreation_should_RemoveInstanceAndCleanUp_When_StatusIsCreating
// covers the happy path (Story 3.2.1's first acceptance criterion): a
// Creating instance is removed from the poller and storage, and a subsequent
// GetSession returns NotFound.
func TestCancelSessionCreation_should_RemoveInstanceAndCleanUp_When_StatusIsCreating(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	svc := newCreateTestService(t, storage)
	newCancelTestInstance(t, svc, storage, "cancel-happy-path", false)

	resp, err := svc.CancelSessionCreation(context.Background(), connect.NewRequest(&sessionv1.CancelSessionCreationRequest{
		Id: "cancel-happy-path",
	}))
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.True(t, resp.Msg.Success)

	assert.Nil(t, svc.reviewQueuePoller.FindInstance("cancel-happy-path"), "instance should be removed from the live poller")

	_, getErr := svc.GetSession(context.Background(), connect.NewRequest(&sessionv1.GetSessionRequest{
		Id: "cancel-happy-path",
	}))
	require.Error(t, getErr)
	connectErr, ok := getErr.(*connect.Error)
	require.True(t, ok, "expected *connect.Error, got %T", getErr)
	assert.Equal(t, connect.CodeNotFound, connectErr.Code())
}

// TestCancelSessionCreation_should_SucceedWithoutPanic_When_CancelFuncIsNil
// covers Task 3.2.1b-2: an instance whose pipeline goroutine (and CancelFunc)
// never existed in this process — the post-restart case — must still cancel
// cleanly rather than nil-pointer-panicking on the CancelFunc call.
func TestCancelSessionCreation_should_SucceedWithoutPanic_When_CancelFuncIsNil(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	svc := newCreateTestService(t, storage)
	inst := newCancelTestInstance(t, svc, storage, "cancel-nil-cancelfunc", false)

	require.Nil(t, inst.CreationCancelFunc(), "precondition: CancelFunc must be nil (never spawned in this process)")

	require.NotPanics(t, func() {
		resp, err := svc.CancelSessionCreation(context.Background(), connect.NewRequest(&sessionv1.CancelSessionCreationRequest{
			Id: "cancel-nil-cancelfunc",
		}))
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.True(t, resp.Msg.Success)
	})

	assert.Nil(t, svc.reviewQueuePoller.FindInstance("cancel-nil-cancelfunc"))
}

// TestCancelSessionCreation_should_ReturnFailedPrecondition_When_StatusIsActive
// covers Story 3.2.1's non-Creating rejection: cancel only ever applies to a
// Creating instance (per the plan's Non-Goals), and the instance must be left
// untouched.
func TestCancelSessionCreation_should_ReturnFailedPrecondition_When_StatusIsActive(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	svc := newCreateTestService(t, storage)
	inst := newCancelTestInstance(t, svc, storage, "cancel-already-active", false)
	epoch := inst.CreationEpoch()
	require.True(t, inst.TryForceStatusIfEpoch(epoch, session.Active, ""))

	_, err := svc.CancelSessionCreation(context.Background(), connect.NewRequest(&sessionv1.CancelSessionCreationRequest{
		Id: "cancel-already-active",
	}))
	require.Error(t, err)
	connectErr, ok := err.(*connect.Error)
	require.True(t, ok, "expected *connect.Error, got %T", err)
	assert.Equal(t, connect.CodeFailedPrecondition, connectErr.Code())

	// Instance must be untouched: still present, still Active.
	require.NotNil(t, svc.reviewQueuePoller.FindInstance("cancel-already-active"))
	status, _ := inst.StatusAndFailureReason()
	assert.Equal(t, session.Active, status)
}

// TestCancelSessionCreation_should_ResolveDeterministically_When_RacingPipelineSuccess
// covers Task 3.2.1c/d: cancel bumps creationEpoch as its own actor mailbox
// round-trip before re-reading status, so a concurrent pipeline success
// (TryForceStatusIfEpoch with the pre-bump epoch) and the cancel call resolve
// to exactly one deterministic outcome — either cancel wins (instance
// removed) or the pipeline's terminal write already landed first (cancel
// observes Active and returns FailedPrecondition without touching the
// instance) — never a torn/inconsistent state. Run with `-race -count=50`.
func TestCancelSessionCreation_should_ResolveDeterministically_When_RacingPipelineSuccess(t *testing.T) {
	storage := createTestStorage(t)
	svc := newCreateTestService(t, storage)
	inst := newCancelTestInstance(t, svc, storage, "cancel-race-test", true)
	epoch := inst.CreationEpoch()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		inst.TryForceStatusIfEpoch(epoch, session.Active, "")
	}()

	resp, err := svc.CancelSessionCreation(context.Background(), connect.NewRequest(&sessionv1.CancelSessionCreationRequest{
		Id: "cancel-race-test",
	}))
	wg.Wait()

	stillPresent := svc.reviewQueuePoller.FindInstance("cancel-race-test") != nil

	if err == nil {
		// Cancel won the race.
		require.NotNil(t, resp)
		assert.True(t, resp.Msg.Success)
		assert.False(t, stillPresent, "cancel won: instance should be removed from the poller")

		_, getErr := svc.GetSession(context.Background(), connect.NewRequest(&sessionv1.GetSessionRequest{
			Id: "cancel-race-test",
		}))
		require.Error(t, getErr)
		connectErr, ok := getErr.(*connect.Error)
		require.True(t, ok, "expected *connect.Error, got %T", getErr)
		assert.Equal(t, connect.CodeNotFound, connectErr.Code())
		return
	}

	// The pipeline's terminal write won the race: cancel must be a no-op —
	// the instance stays present and Active, never deleted mid-flight.
	connectErr, ok := err.(*connect.Error)
	require.True(t, ok, "expected *connect.Error, got %T", err)
	assert.Equal(t, connect.CodeFailedPrecondition, connectErr.Code())
	assert.True(t, stillPresent, "pipeline won: instance must remain present, not deleted")
	status, _ := inst.StatusAndFailureReason()
	assert.Equal(t, session.Active, status, "pipeline won: status must be Active, not left mid-transition")
}

// TestCancelSessionCreation_should_ReturnNotFound_When_SessionDoesNotExist
// pins the not-found path so a bad ID doesn't silently no-op.
func TestCancelSessionCreation_should_ReturnNotFound_When_SessionDoesNotExist(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	svc := newCreateTestService(t, storage)

	_, err := svc.CancelSessionCreation(context.Background(), connect.NewRequest(&sessionv1.CancelSessionCreationRequest{
		Id: "no-such-session",
	}))
	require.Error(t, err)
	connectErr, ok := err.(*connect.Error)
	require.True(t, ok, "expected *connect.Error, got %T", err)
	assert.Equal(t, connect.CodeNotFound, connectErr.Code())
}

// TestCancelSessionCreation_should_ReturnInvalidArgument_When_IdIsEmpty pins
// the same fast-fail-on-empty-ID convention DeleteSession uses.
func TestCancelSessionCreation_should_ReturnInvalidArgument_When_IdIsEmpty(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	svc := newCreateTestService(t, storage)

	_, err := svc.CancelSessionCreation(context.Background(), connect.NewRequest(&sessionv1.CancelSessionCreationRequest{
		Id: "",
	}))
	require.Error(t, err)
	connectErr, ok := err.(*connect.Error)
	require.True(t, ok, "expected *connect.Error, got %T", err)
	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}

// TestCancelSessionCreation_should_ResolveAwaitCreationTerminalPromptly_When_RacingALiveWait
// is Task 3.2.1f: the real CancelSessionCreation RPC racing a live
// AwaitCreationTerminal wait, not just AwaitCreationTerminal's own
// direct-store-deletion vanish-path unit test
// (TestAwaitCreationTerminal_should_ReturnErrCreationVanished_When_InstanceIsRemovedMidWait
// in session_creation_await_test.go), which removes the instance from the
// poller directly rather than going through the real RPC an MCP-style caller
// actually calls. Given a caller blocked inside AwaitCreationTerminal for an
// instance, when a concurrent CancelSessionCreation call removes it, the
// waiter must observe ErrCreationVanished within roughly one poll interval
// of the deletion, not hang until its own (much longer) timeout.
func TestCancelSessionCreation_should_ResolveAwaitCreationTerminalPromptly_When_RacingALiveWait(t *testing.T) {
	storage := createTestStorage(t)
	svc := newCreateTestService(t, storage)
	newCancelTestInstance(t, svc, storage, "cancel-await-race", true)

	type awaitResult struct {
		outcome CreationOutcome
		err     error
	}
	awaitDone := make(chan awaitResult, 1)
	start := time.Now()
	go func() {
		outcome, err := svc.awaitCreationTerminal(context.Background(), "cancel-await-race", 5*time.Second, 10*time.Millisecond)
		awaitDone <- awaitResult{outcome, err}
	}()

	// Give the awaiter time to actually start polling before cancel fires --
	// this is what makes it a race against a LIVE wait, not a wait started
	// after the deletion already happened.
	time.Sleep(30 * time.Millisecond)

	resp, err := svc.CancelSessionCreation(context.Background(), connect.NewRequest(&sessionv1.CancelSessionCreationRequest{
		Id: "cancel-await-race",
	}))
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.True(t, resp.Msg.Success)

	select {
	case res := <-awaitDone:
		elapsed := time.Since(start)
		require.ErrorIs(t, res.err, ErrCreationVanished)
		assert.Equal(t, CreationOutcome{}, res.outcome)
		assert.Less(t, elapsed, 2*time.Second,
			"the waiter must observe the cancel-driven deletion promptly, not hang until its own 5s timeout")
	case <-time.After(2 * time.Second):
		t.Fatal("AwaitCreationTerminal did not return promptly after a concurrent CancelSessionCreation removed the instance")
	}
}

// TestDeleteSession_should_CancelInFlightPipeline_When_StatusIsCreating
// verifies DeleteSession bumps the creation epoch and invokes the pipeline's
// CancelFunc before cleanup, mirroring CancelSessionCreation/
// RetrySessionCreation's identical ordering, so a concurrent
// SetGitHubResolution write can't win against DeleteSession's cleanup path.
// Both the epoch bump and the CancelFunc call happen synchronously inside the
// RPC handler, before the async cleanup goroutine is even dispatched
// (session_service.go's trackCleanup call), so observing them true
// immediately after DeleteSession returns is already proof they ran first.
func TestDeleteSession_should_CancelInFlightPipeline_When_StatusIsCreating(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	svc := newCreateTestService(t, storage)
	t.Cleanup(func() { svc.Shutdown() })
	inst := newCancelTestInstance(t, svc, storage, "delete-cancels-creating", false)

	var mu sync.Mutex
	cancelled := false
	inst.SetCreationCancelFunc(func() {
		mu.Lock()
		cancelled = true
		mu.Unlock()
	})

	resp, err := svc.DeleteSession(context.Background(), connect.NewRequest(&sessionv1.DeleteSessionRequest{
		Id: "delete-cancels-creating",
	}))
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.True(t, resp.Msg.Success)

	mu.Lock()
	defer mu.Unlock()
	assert.True(t, cancelled, "DeleteSession must cancel an in-flight Background Resolution Pipeline for a Creating session")
	assert.Equal(t, uint64(1), inst.CreationEpoch(), "DeleteSession must bump the creation epoch so a late pipeline write can't win after cleanup starts")
}

// TestDeleteSession_should_SucceedWithoutPanic_When_CreatingWithNilCancelFunc
// covers the post-restart case (mirroring
// TestCancelSessionCreation_should_SucceedWithoutPanic_When_CancelFuncIsNil):
// a Creating instance whose pipeline goroutine never existed in this process
// has a nil CancelFunc, and DeleteSession must not nil-pointer-panic on it.
func TestDeleteSession_should_SucceedWithoutPanic_When_CreatingWithNilCancelFunc(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	svc := newCreateTestService(t, storage)
	t.Cleanup(func() { svc.Shutdown() })
	inst := newCancelTestInstance(t, svc, storage, "delete-nil-cancelfunc", false)
	require.Nil(t, inst.CreationCancelFunc(), "precondition: CancelFunc must be nil (never spawned in this process)")

	require.NotPanics(t, func() {
		resp, err := svc.DeleteSession(context.Background(), connect.NewRequest(&sessionv1.DeleteSessionRequest{
			Id: "delete-nil-cancelfunc",
		}))
		require.NoError(t, err)
		require.True(t, resp.Msg.Success)
	})
}

// TestDeleteSession_should_NotTouchCancelFunc_When_StatusIsNotCreating is a
// regression guard: DeleteSession's new fence-out step must stay scoped to
// Creating sessions, the same guard CancelSessionCreation/RetrySessionCreation
// apply, so deleting an already-Active session doesn't invoke a stale
// CancelFunc left over from a finished pipeline.
func TestDeleteSession_should_NotTouchCancelFunc_When_StatusIsNotCreating(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	svc := newCreateTestService(t, storage)
	t.Cleanup(func() { svc.Shutdown() })
	inst := newCancelTestInstance(t, svc, storage, "delete-active-session", false)
	require.True(t, inst.TryForceStatusIfEpoch(inst.CreationEpoch(), session.Active, ""))

	var mu sync.Mutex
	cancelled := false
	inst.SetCreationCancelFunc(func() {
		mu.Lock()
		cancelled = true
		mu.Unlock()
	})

	resp, err := svc.DeleteSession(context.Background(), connect.NewRequest(&sessionv1.DeleteSessionRequest{
		Id: "delete-active-session",
	}))
	require.NoError(t, err)
	require.True(t, resp.Msg.Success)

	mu.Lock()
	defer mu.Unlock()
	assert.False(t, cancelled, "DeleteSession must not invoke the pipeline CancelFunc for a session that already finished creating")
}
