package mcp

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/instance"
)

// blockedNotifyFixture returns a handler wired to an event bus, a work session
// linked to a new item at status, and the bus's notification channel.
func blockedNotifyFixture(t *testing.T, status domain.BacklogStatus, notes string) (*backlogHandlers, context.Context, string, <-chan *events.Event) {
	t.Helper()
	storage := newTestBacklogStorage(t)
	ctx := context.Background()
	item, err := storage.CreateBacklogItem(ctx, backlogdto.BacklogItemData{Title: "Blocked item", Status: string(status), Notes: notes})
	require.NoError(t, err)
	sessionUUID := uuid.New().String()
	_, err = storage.CreateItemSession(ctx, backlogdto.ItemSessionData{ItemID: item.ID, SessionUUID: sessionUUID, SessionRole: instance.SessionRoleWork})
	require.NoError(t, err)

	bus := events.NewEventBus(32)
	ch, subID := bus.Subscribe(ctx)
	t.Cleanup(func() { bus.Unsubscribe(subID) })
	return &backlogHandlers{storage: storage, eventBus: bus}, WithSessionUUID(ctx, sessionUUID), item.ID, ch
}

func blockOnce(t *testing.T, h *backlogHandlers, ctx context.Context, itemID string) {
	t.Helper()
	res, err := h.reportBlocked(ctx, makeToolReq(map[string]interface{}{"item_id": itemID, "rationale": "AC 2 cannot pass: upstream API missing"}))
	require.NoError(t, err)
	requireToolTextResult(t, res)
}

func drain(ch <-chan *events.Event) []*events.Event {
	var out []*events.Event
	for {
		select {
		case e := <-ch:
			out = append(out, e)
		case <-time.After(200 * time.Millisecond):
			return out
		}
	}
}

func TestReportBlocked_should_NotifyOperatorOnce_When_ThresholdEscalatesToReview(t *testing.T) {
	t.Parallel()
	notes := blockedNoteMarker + " one\n" + rejectedNoteMarker + " two"
	h, ctx, itemID, ch := blockedNotifyFixture(t, domain.BacklogStatusInProgress, notes)

	blockOnce(t, h, ctx, itemID)

	got := drain(ch)
	require.Len(t, got, 1, "escalation must publish exactly one notification")
	n := got[0]
	require.Equal(t, events.EventNotification, n.Type)
	require.Equal(t, int32(sessionv1.NotificationType_NOTIFICATION_TYPE_INPUT_REQUIRED), n.NotificationType)
	require.Equal(t, int32(sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_HIGH), n.NotificationPriority)
	require.Equal(t, itemID, n.NotificationMetadata["item_id"])
	require.Contains(t, n.NotificationMessage, "AC 2 cannot pass")
}

func TestReportBlocked_should_NotNotify_When_BelowThreshold(t *testing.T) {
	t.Parallel()
	h, ctx, itemID, ch := blockedNotifyFixture(t, domain.BacklogStatusInProgress, "")

	blockOnce(t, h, ctx, itemID)

	require.Empty(t, drain(ch), "a plain return-to-ready is not a decision request")
}

func TestReportBlocked_should_NotRenotify_When_ItemAlreadyInReviewPastThreshold(t *testing.T) {
	t.Parallel()
	// Two prior cycles + this one crosses the threshold while already in review: first crossing notifies.
	notes := blockedNoteMarker + " one\n" + blockedNoteMarker + " two"
	h, ctx, itemID, ch := blockedNotifyFixture(t, domain.BacklogStatusReview, notes)

	blockOnce(t, h, ctx, itemID)
	require.Len(t, drain(ch), 1, "first threshold crossing announces even without a status change")

	blockOnce(t, h, ctx, itemID)
	require.Empty(t, drain(ch), "repeat blocks on an item already waiting in review must not re-notify")
}
