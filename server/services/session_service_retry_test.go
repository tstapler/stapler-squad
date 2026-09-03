package services

import (
	"context"
	"testing"

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
