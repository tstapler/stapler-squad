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
