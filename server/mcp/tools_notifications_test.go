package mcp

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/server/notifications"
	"github.com/tstapler/stapler-squad/server/services"
)

// newTestNotificationHandlers wires a *notificationHandlers backed by a real
// *services.SessionService with a disk-backed NotificationHistoryStore
// attached, mirroring newTestWorkflowHandlers' shape.
func newTestNotificationHandlers(t *testing.T) *notificationHandlers {
	t.Helper()
	storage := newTestBacklogStorage(t)
	svc := services.NewSessionService(storage, events.NewEventBus(100))

	store, err := notifications.NewNotificationHistoryStore(filepath.Join(t.TempDir(), "notifications.json"))
	require.NoError(t, err)
	svc.SetNotificationStore(store)

	return &notificationHandlers{svc: svc}
}

func appendNotification(t *testing.T, store *notifications.NotificationHistoryStore, sessionID string, notifType int32, isRead bool) {
	t.Helper()
	id := uuid.New().String()
	require.NoError(t, store.Append(&notifications.NotificationRecord{
		ID:               id,
		SessionID:        sessionID,
		SessionName:      sessionID,
		NotificationType: notifType,
		Title:            "notification",
		Message:          "notification body",
		CreatedAt:        time.Now(),
	}))
	if isRead {
		records, _, listErr := store.List(notifications.ListOptions{SessionID: sessionID})
		require.NoError(t, listErr)
		require.NotEmpty(t, records)
		_, err := store.MarkRead([]string{records[0].ID})
		require.NoError(t, err)
	}
}

func TestGetNotificationHistory_ReturnsFilteredRecords_When_TypeFilterApplied(t *testing.T) {
	h := newTestNotificationHandlers(t)
	store := h.svc.GetNotificationStore()

	appendNotification(t, store, "sess-1", int32(sessionv1.NotificationType_NOTIFICATION_TYPE_TASK_COMPLETE), false)
	appendNotification(t, store, "sess-2", int32(sessionv1.NotificationType_NOTIFICATION_TYPE_ERROR), false)

	res, err := h.getNotificationHistory(context.Background(), makeToolReq(map[string]interface{}{
		"type_filter": "TASK_COMPLETE",
	}))
	require.NoError(t, err)
	out := parseResult(t, res)
	require.True(t, out["success"].(bool))
	notifs := out["notifications"].([]interface{})
	require.Len(t, notifs, 1)
	require.Equal(t, "sess-1", notifs[0].(map[string]interface{})["session_id"])
}

func TestGetNotificationHistory_ReturnsInvalidArgument_When_TypeFilterUnknown(t *testing.T) {
	h := newTestNotificationHandlers(t)

	res, err := h.getNotificationHistory(context.Background(), makeToolReq(map[string]interface{}{
		"type_filter": "NOT_A_REAL_TYPE",
	}))
	require.NoError(t, err)
	out := parseResult(t, res)
	require.False(t, out["success"].(bool))
	require.Equal(t, ErrInvalidArgument, out["error"].(map[string]interface{})["code"])
}

func TestGetNotificationHistory_ReturnsAndedResults_When_UnreadOnlyAndTypeFilterCombined(t *testing.T) {
	h := newTestNotificationHandlers(t)
	store := h.svc.GetNotificationStore()

	for i := 0; i < 5; i++ {
		appendNotification(t, store, uuid.New().String(), int32(sessionv1.NotificationType_NOTIFICATION_TYPE_TASK_COMPLETE), false)
	}
	for i := 0; i < 3; i++ {
		appendNotification(t, store, uuid.New().String(), int32(sessionv1.NotificationType_NOTIFICATION_TYPE_ERROR), true)
	}

	res, err := h.getNotificationHistory(context.Background(), makeToolReq(map[string]interface{}{
		"type_filter": "TASK_COMPLETE",
		"unread_only": true,
		"limit":       float64(50),
	}))
	require.NoError(t, err)
	out := parseResult(t, res)
	require.True(t, out["success"].(bool))
	notifs := out["notifications"].([]interface{})
	require.Len(t, notifs, 5)
	require.Equal(t, float64(5), out["unread_count"])
}

func TestGetNotificationHistory_ReturnsDefaultLimitOf10_When_NoLimitArgGiven(t *testing.T) {
	h := newTestNotificationHandlers(t)
	store := h.svc.GetNotificationStore()
	for i := 0; i < 100; i++ {
		appendNotification(t, store, uuid.New().String(), int32(sessionv1.NotificationType_NOTIFICATION_TYPE_TASK_COMPLETE), false)
	}

	res, err := h.getNotificationHistory(context.Background(), makeToolReq(map[string]interface{}{}))
	require.NoError(t, err)
	out := parseResult(t, res)
	require.True(t, out["success"].(bool))
	notifs := out["notifications"].([]interface{})
	require.Len(t, notifs, 10)
	require.True(t, out["has_more"].(bool))
}

func TestGetNotificationHistory_ReturnsFilteredRecords_When_SessionIDFilterApplied(t *testing.T) {
	h := newTestNotificationHandlers(t)
	store := h.svc.GetNotificationStore()

	appendNotification(t, store, "sess-a", int32(sessionv1.NotificationType_NOTIFICATION_TYPE_TASK_COMPLETE), false)
	appendNotification(t, store, "sess-b", int32(sessionv1.NotificationType_NOTIFICATION_TYPE_TASK_COMPLETE), false)

	res, err := h.getNotificationHistory(context.Background(), makeToolReq(map[string]interface{}{
		"session_id": "sess-a",
	}))
	require.NoError(t, err)
	out := parseResult(t, res)
	require.True(t, out["success"].(bool))
	notifs := out["notifications"].([]interface{})
	require.Len(t, notifs, 1)
	require.Equal(t, "sess-a", notifs[0].(map[string]interface{})["session_id"])
}

func TestGetNotificationHistory_ClampsLimitAboveMax(t *testing.T) {
	h := newTestNotificationHandlers(t)
	store := h.svc.GetNotificationStore()
	for i := 0; i < 60; i++ {
		appendNotification(t, store, uuid.New().String(), int32(sessionv1.NotificationType_NOTIFICATION_TYPE_TASK_COMPLETE), false)
	}

	res, err := h.getNotificationHistory(context.Background(), makeToolReq(map[string]interface{}{
		"limit": float64(1000),
	}))
	require.NoError(t, err)
	out := parseResult(t, res)
	notifs := out["notifications"].([]interface{})
	require.Len(t, notifs, 50, "limit must be clamped to the tool's max of 50")
}

func TestGetNotificationHistory_ReturnsEmptySuccess_When_NotificationStoreNil(t *testing.T) {
	storage := newTestBacklogStorage(t)
	svc := services.NewSessionService(storage, events.NewEventBus(100))
	h := &notificationHandlers{svc: svc} // no notification store wired

	res, err := h.getNotificationHistory(context.Background(), makeToolReq(map[string]interface{}{}))
	require.NoError(t, err)
	out := parseResult(t, res)
	require.True(t, out["success"].(bool))
	require.Empty(t, out["notifications"])
}
