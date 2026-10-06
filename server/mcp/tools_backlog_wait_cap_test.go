package mcp

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/session"
)

func newReviewItem(t *testing.T, storage *session.Storage) string {
	t.Helper()
	item, err := storage.CreateBacklogItem(context.Background(), session.BacklogItemData{
		Title:  "Waiting for verdict",
		Status: string(session.BacklogStatusReview),
	})
	require.NoError(t, err)
	return item.ID
}

// Pins the timeout text: it must tell the session to stop, not to poll.
func TestWaitForBacklogEvent_TimeoutMessage_NoPollingAdvice(t *testing.T) {
	storage := newTestBacklogStorage(t)
	itemID := newReviewItem(t, storage)
	handler := &backlogHandlers{storage: storage, eventBus: events.NewEventBus(32)}

	result, err := handler.waitForBacklogEvent(context.Background(), makeToolReq(map[string]interface{}{"item_id": itemID, "timeout_seconds": float64(1)}))
	require.NoError(t, err)
	out := decodeWaitResult(t, result)

	require.Equal(t, "WAIT_TIMEOUT", out.Error.Code)
	require.Contains(t, out.Error.Message, "end your turn and stay idle")
	require.Contains(t, out.Error.Message, "do not use ScheduleWakeup or /loop")
	require.False(t, strings.Contains(out.Error.Message, "call ScheduleWakeup"))
}

func TestWaitForBacklogEvent_ParksSessionAfterRepeatedTimeouts(t *testing.T) {
	storage := newTestBacklogStorage(t)
	itemID := newReviewItem(t, storage)
	bus := events.NewEventBus(32)
	notifCh, subID := bus.Subscribe(context.Background())
	defer bus.Unsubscribe(subID)
	handler := &backlogHandlers{storage: storage, eventBus: bus}
	ctx := WithSessionUUID(context.Background(), uuid.New().String())
	req := makeToolReq(map[string]interface{}{"item_id": itemID, "timeout_seconds": float64(1)})

	// Two prior timeouts; the next one reaches the cap.
	handler.waitTimeouts = map[string]int{waitCapKey(ctx, itemID): maxWaitTimeoutsPerSession - 1}
	result, err := handler.waitForBacklogEvent(ctx, req)
	require.NoError(t, err)
	require.Equal(t, "WAIT_CAP_REACHED", decodeWaitResult(t, result).Error.Code)

	select {
	case evt := <-notifCh:
		require.Equal(t, events.EventNotification, evt.Type, "a visible operator notification must accompany parking")
	case <-time.After(2 * time.Second):
		t.Fatal("no notification published when the wait cap was reached")
	}

	// Subsequent waits refuse immediately instead of blocking.
	start := time.Now()
	result, err = handler.waitForBacklogEvent(ctx, req)
	require.NoError(t, err)
	require.Equal(t, "WAIT_CAP_REACHED", decodeWaitResult(t, result).Error.Code)
	require.Less(t, time.Since(start), 500*time.Millisecond)

	// Another session on the same item is unaffected.
	other := WithSessionUUID(context.Background(), uuid.New().String())
	require.False(t, handler.waitCapReached(other, itemID))
}

// A capped session can still read state that already satisfies the wait.
func TestWaitForBacklogEvent_CappedSession_StillSeesExistingState(t *testing.T) {
	storage := newTestBacklogStorage(t)
	item, err := storage.CreateBacklogItem(context.Background(), session.BacklogItemData{
		Title:  "Already archived",
		Status: string(session.BacklogStatusArchived),
	})
	require.NoError(t, err)
	handler := &backlogHandlers{storage: storage, eventBus: events.NewEventBus(32)}
	ctx := WithSessionUUID(context.Background(), uuid.New().String())
	handler.waitTimeouts = map[string]int{waitCapKey(ctx, item.ID): maxWaitTimeoutsPerSession}

	result, err := handler.waitForBacklogEvent(ctx, makeToolReq(map[string]interface{}{"item_id": item.ID, "timeout_seconds": float64(1)}))
	require.NoError(t, err)
	out := decodeWaitResult(t, result)
	require.True(t, out.EventReceived)
	require.True(t, out.FromCurrentState)
}
