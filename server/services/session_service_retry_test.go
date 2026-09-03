package services

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
)

// TestSessionService_RetrySession_should_ReturnInvalidArgument_When_IDEmpty
// mirrors TestRestartSession_EmptyID's shape for the new RetrySession RPC.
func TestSessionService_RetrySession_should_ReturnInvalidArgument_When_IDEmpty(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	eventBus := events.NewEventBus(100)
	svc := NewSessionService(storage, eventBus)
	t.Cleanup(func() { svc.Shutdown() })

	_, err := svc.RetrySession(context.Background(), connect.NewRequest(&sessionv1.RetrySessionRequest{Id: ""}))
	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}

// TestSessionService_RetrySession_should_ReturnNotFound_When_SessionDoesNotExist
// mirrors TestRestartSession_SessionNotFound's shape.
func TestSessionService_RetrySession_should_ReturnNotFound_When_SessionDoesNotExist(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	eventBus := events.NewEventBus(100)
	svc := NewSessionService(storage, eventBus)
	t.Cleanup(func() { svc.Shutdown() })

	_, err := svc.RetrySession(context.Background(), connect.NewRequest(&sessionv1.RetrySessionRequest{Id: "does-not-exist"}))
	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeNotFound, connectErr.Code())
}

// TestSessionService_RetrySession_should_ReturnFailedPrecondition_When_RetryAlreadyInFlight
// verifies ErrRetryInFlight (session.ErrRetryInFlight) is mapped to
// connect.CodeFailedPrecondition, distinguishing "already retrying" from a
// generic internal error (design/ux.md Surface 4).
func TestSessionService_RetrySession_should_ReturnFailedPrecondition_When_RetryAlreadyInFlight(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	eventBus := events.NewEventBus(100)
	svc := NewSessionService(storage, eventBus)
	t.Cleanup(func() { svc.Shutdown() })

	queue := session.NewReviewQueue()
	statusMgr := session.NewInstanceStatusManager()
	poller := session.NewReviewQueuePoller(queue, statusMgr, nil)
	svc.SetReviewQueuePoller(poller)

	resp, err := svc.CreateSession(context.Background(), connect.NewRequest(&sessionv1.CreateSessionRequest{
		Title:   "retry-in-flight-test-session",
		Path:    t.TempDir(),
		Program: "true",
	}))
	require.NoError(t, err)
	t.Cleanup(func() { destroyCreatedSession(t, svc, resp.Msg.Session.Id) })

	inst := svc.FindLiveInstance(resp.Msg.Session.Id)
	require.NotNil(t, inst)
	inst.SetRetryInFlightForTest(true)

	_, retryErr := svc.RetrySession(context.Background(), connect.NewRequest(&sessionv1.RetrySessionRequest{
		Id: resp.Msg.Session.Id,
	}))
	require.Error(t, retryErr)
	var connectErr *connect.Error
	require.ErrorAs(t, retryErr, &connectErr)
	assert.Equal(t, connect.CodeFailedPrecondition, connectErr.Code())
}

// TestSessionService_RetrySession_should_RestartImmediately_When_SessionIsPermanentlyFailed
// covers the RPC's success path (AC6) — the three tests above only assert
// error-mapping branches (empty ID, not-found, retry-already-in-flight), none
// of which exercise the happy path that actually restarts a session.
func TestSessionService_RetrySession_should_RestartImmediately_When_SessionIsPermanentlyFailed(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	eventBus := events.NewEventBus(100)
	svc := NewSessionService(storage, eventBus)
	t.Cleanup(func() { svc.Shutdown() })

	queue := session.NewReviewQueue()
	statusMgr := session.NewInstanceStatusManager()
	poller := session.NewReviewQueuePoller(queue, statusMgr, nil)
	svc.SetReviewQueuePoller(poller)

	resp, err := svc.CreateSession(context.Background(), connect.NewRequest(&sessionv1.CreateSessionRequest{
		Title:   "retry-success-test-session",
		Path:    t.TempDir(),
		Program: "true",
	}))
	require.NoError(t, err)
	t.Cleanup(func() { destroyCreatedSession(t, svc, resp.Msg.Session.Id) })

	inst := svc.FindLiveInstance(resp.Msg.Session.Id)
	require.NotNil(t, inst)
	// CreateSession's own start pipeline finishes asynchronously (Creating ->
	// Active) — wait for it to settle before forcing a status transition,
	// otherwise the pipeline's own later Active transition races the forced
	// PermanentlyFailed below and the state machine (correctly) rejects it.
	// GetStatus() (not the Status field directly) is the race-safe read: it
	// goes through Snapshot(), the atomically-published value the pipeline's
	// own actor-serialized writes publish to — a direct field read/write
	// races with those writes, which don't take inst.mu at all (see
	// GetStatus's doc comment; caught by -race in CI on PR #671).
	// 10s, not 2s: this spawns a real tmux session as part of the pipeline, so
	// completion time depends on OS process-scheduling latency, not just
	// in-process work -- flaked under concurrent test-suite load with a
	// tighter window even though the pipeline always eventually converges
	// (see pipelineEventuallyTimeout's doc comment in
	// session_creation_pipeline_test.go for the same pattern/root cause).
	require.Eventually(t, func() bool {
		return session.Status(inst.GetStatus()) == session.Active
	}, 10*time.Second, 10*time.Millisecond, "session should reach Active before the test forces PermanentlyFailed")
	// Simulate a session that has exhausted its automated retries and is
	// sitting in the terminal give-up state RetryNow must be able to revive.
	// MarkPermanentlyFailedForTest routes through the same locked +
	// snapshot-republish path the production code uses, rather than a bare
	// field write.
	inst.MarkPermanentlyFailedForTest()

	retryResp, retryErr := svc.RetrySession(context.Background(), connect.NewRequest(&sessionv1.RetrySessionRequest{
		Id: resp.Msg.Session.Id,
	}))
	require.NoError(t, retryErr)
	require.True(t, retryResp.Msg.Success)
	require.NotNil(t, retryResp.Msg.Session)
	assert.NotEqual(t, "PERMANENTLY_FAILED", retryResp.Msg.Session.Status.String())
}
