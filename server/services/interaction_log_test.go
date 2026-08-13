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

// setupReviewQueueServiceWithT creates a ReviewQueueService using the test's
// cleanup helpers so that the underlying storage is properly closed.
func setupReviewQueueServiceWithT(t *testing.T) *ReviewQueueService {
	t.Helper()
	queue := session.NewReviewQueue()
	storage := createTestStorage(t)
	bus := events.NewEventBus(32)
	t.Cleanup(bus.Close)
	return NewReviewQueueService(queue, storage, bus)
}

// TestLogInteraction_Success verifies that LogUserInteraction returns success
// for a well-formed request with all optional fields populated.
func TestLogInteraction_Success(t *testing.T) {
	svc := setupReviewQueueServiceWithT(t)

	sessionID := "test-session-id"
	ctx := "notification_panel"
	notifID := "notif-123"

	resp, err := svc.LogUserInteraction(
		context.Background(),
		connect.NewRequest(&sessionv1.LogUserInteractionRequest{
			SessionId:       &sessionID,
			InteractionType: sessionv1.UserInteractionEvent_INTERACTION_TYPE_NOTIFICATION_VIEWED,
			Context:         &ctx,
			NotificationId:  &notifID,
		}),
	)

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.True(t, resp.Msg.Success)
}

// TestLogInteraction_NoSessionID verifies that LogUserInteraction succeeds when
// session_id is not set (panel-level actions have no session context).
func TestLogInteraction_NoSessionID(t *testing.T) {
	svc := setupReviewQueueServiceWithT(t)

	resp, err := svc.LogUserInteraction(
		context.Background(),
		connect.NewRequest(&sessionv1.LogUserInteractionRequest{
			InteractionType: sessionv1.UserInteractionEvent_INTERACTION_TYPE_NOTIFICATION_PANEL_OPENED,
		}),
	)

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.True(t, resp.Msg.Success)
}

// TestGetLogs_EmptySessionID verifies that GetLogs with an empty session_id
// pointer (nil) returns a valid (possibly empty) response for the global log.
// This exercises the "no session ID" code path in UtilityService.GetLogs.
func TestGetLogs_EmptySessionID(t *testing.T) {
	svc := setupUtilityService()

	// Passing a nil session_id — exercises the global log path.
	resp, err := svc.GetLogs(
		context.Background(),
		connect.NewRequest(&sessionv1.GetLogsRequest{}),
	)

	// Either succeeds (empty log) or errors; must not panic.
	if err != nil {
		var connectErr *connect.Error
		require.ErrorAs(t, err, &connectErr)
		assert.NotEqual(t, connect.CodeNotFound, connectErr.Code())
	} else {
		require.NotNil(t, resp)
	}
}
