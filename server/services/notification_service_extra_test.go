package services

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/events"
)

// setupNotificationService creates a NotificationService with no store wired
// (store is nil), so all three RPCs exercise the nil-store fast paths.
func setupNotificationService() *NotificationService {
	bus := events.NewEventBus(32)
	return NewNotificationService(NewNotificationRateLimiter(10, 20), bus)
}

// TestGetNotificationHistory_ReturnsEmpty verifies that a fresh NotificationService
// with no store wired returns an empty list without error.
func TestGetNotificationHistory_ReturnsEmpty(t *testing.T) {
	svc := setupNotificationService()

	resp, err := svc.GetNotificationHistory(
		context.Background(),
		connect.NewRequest(&sessionv1.GetNotificationHistoryRequest{}),
	)

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Empty(t, resp.Msg.Notifications)
	assert.Equal(t, int32(0), resp.Msg.TotalCount)
}

// TestMarkNotificationRead_EmptyIDs verifies that passing an empty
// notification_ids slice returns success with marked_count == 0 (no-op).
func TestMarkNotificationRead_EmptyIDs(t *testing.T) {
	svc := setupNotificationService()

	resp, err := svc.MarkNotificationRead(
		context.Background(),
		connect.NewRequest(&sessionv1.MarkNotificationReadRequest{
			NotificationIds: []string{},
		}),
	)

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.True(t, resp.Msg.Success)
	assert.Equal(t, int32(0), resp.Msg.MarkedCount)
}

// TestClearNotificationHistory_Success verifies that ClearNotificationHistory
// returns success when there is nothing to clear (nil store).
func TestClearNotificationHistory_Success(t *testing.T) {
	svc := setupNotificationService()

	resp, err := svc.ClearNotificationHistory(
		context.Background(),
		connect.NewRequest(&sessionv1.ClearNotificationHistoryRequest{}),
	)

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.True(t, resp.Msg.Success)
	assert.Equal(t, int32(0), resp.Msg.ClearedCount)
}
