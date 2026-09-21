package services

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/pkg/classifier"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
)

// TestNotificationPushGateClassification is a table-driven regression test mirroring the
// classification table validated against real notification-history data (2026-09,
// notification push-gate redesign). Each case drives the actual production call site and
// asserts the derived NotificationPriority, so a reviewer changing a call site's
// urgent/important flags without updating this test sees it fail. See also:
//   - session/backlog_lifecycle_test.go / backlog_lifecycle_stuck_test.go for the
//     session-package rows of the same table (Multiple stuck reasons open, PR creation
//     failed, Bounce cap exhausted, Review session ended without a verdict, Review item
//     needs attention, Work session may be stuck, Triage may be stuck).
//   - server/server_test.go for Fork Pressure / Tmux Server Recovered.
//   - stale_session_notifier_test.go / memory_pressure_notifier_test.go for the other two
//     rows (Session went stale, Memory usage near limit), asserted alongside their
//     existing fire/re-arm coverage rather than duplicated here.
func TestNotificationPushGateClassification(t *testing.T) {
	t.Run("Claude has a question", func(t *testing.T) {
		bus := events.NewEventBus(4)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		ch, _ := bus.Subscribe(ctx)

		h := NewApprovalHandler(NewApprovalStore(""), nil, bus)
		h.broadcastQuestionNotification("sess-1", classifier.PermissionRequestPayload{
			ToolName:  "AskUserQuestion",
			ToolInput: map[string]interface{}{"prompt": "Which approach?"},
		})

		ev := requireOneNotification(t, ch)
		assert.Equal(t, "Claude has a question", ev.NotificationTitle)
		assert.Equal(t, int32(sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_URGENT), ev.NotificationPriority,
			"urgent=true, important=true — Claude is blocked waiting on the user right now")
	})

	t.Run("Permission Required", func(t *testing.T) {
		bus := events.NewEventBus(4)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		ch, _ := bus.Subscribe(ctx)

		h := NewApprovalHandler(NewApprovalStore(""), nil, bus)
		h.broadcastApprovalNotification("sess-1", &PendingApproval{
			ID:        "approval-1",
			SessionID: "sess-1",
			ToolName:  "Bash",
			ToolInput: map[string]interface{}{"command": "rm -rf /tmp/x"},
		})

		ev := requireOneNotification(t, ch)
		assert.Contains(t, ev.NotificationTitle, "Permission Required")
		assert.Equal(t, int32(sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_URGENT), ev.NotificationPriority,
			"urgent=true, important=true — a pending permission request blocks the session right now")
	})

	t.Run("Auto-rework stopped — repeated failure", func(t *testing.T) {
		storage := createTestStorage(t)
		svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
		bus := events.NewEventBus(4)
		svc.SetEventBus(bus)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		ch, _ := bus.Subscribe(ctx)

		svc.notifyRepeatedFailure(ctx, "item-1", "Item title", session.BacklogStatusInProgress, 3, "same test failure")

		ev := requireOneNotification(t, ch)
		assert.Equal(t, "Auto-rework stopped — repeated failure", ev.NotificationTitle)
		assert.Equal(t, int32(sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_URGENT), ev.NotificationPriority,
			"urgent=true, important=true")
	})

	t.Run("Rework blocked by a stale-but-alive session", func(t *testing.T) {
		storage := createTestStorage(t)
		svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
		bus := events.NewEventBus(4)
		svc.SetEventBus(bus)
		svc.SetSessionStopper(&mockSessionStopper{
			liveUUIDs: map[string]bool{"active-work-uuid": true},
			staleFor:  map[string]time.Duration{"active-work-uuid": 20 * time.Minute},
		})
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		ch, _ := bus.Subscribe(ctx)

		item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
			Title:  "Item idle 20m — over the 15m threshold",
			Status: string(session.BacklogStatusReview),
		})
		require.NoError(t, err)
		_, err = storage.CreateItemSession(ctx, session.ItemSessionData{
			ItemID: item.ID, SessionUUID: "active-work-uuid", SessionRole: session.SessionRoleWork,
		})
		require.NoError(t, err)
		sessions, err := storage.ListItemSessions(ctx, item.ID)
		require.NoError(t, err)

		svc.notifyIfActiveWorkSessionStale(ctx, item.ID, item.Title, sessions)

		ev := requireOneNotification(t, ch)
		assert.Equal(t, "Rework blocked by a stale-but-alive session", ev.NotificationTitle)
		assert.Equal(t, int32(sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_HIGH), ev.NotificationPriority,
			"urgent=false, important=true")
	})
}

// requireOneNotification waits briefly for exactly one EventNotification on ch and
// returns it, failing the test otherwise.
func requireOneNotification(t *testing.T, ch <-chan *events.Event) *events.Event {
	t.Helper()
	select {
	case ev := <-ch:
		require.Equal(t, events.EventNotification, ev.Type)
		return ev
	case <-time.After(2 * time.Second):
		t.Fatal("expected a notification event, got none")
		return nil
	}
}
