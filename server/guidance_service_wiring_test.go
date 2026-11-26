package server

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/server/services/guidance"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
)

// The guidance leaf publishes through a port it cannot fill itself; a nil notifier
// silently drops every "your request was answered" notification.
func TestNewGuidanceRequestService_should_NotifyThroughTheEventBus_When_AnAnswerIsApplied(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	storage, err := session.NewStorageWithRepository(session.NewTestEntRepository(t))
	require.NoError(t, err)
	bus := events.NewEventBus(8)
	svc := newGuidanceRequestService(&ServerDependencies{Storage: storage, EventBus: bus})
	ch, subID := bus.Subscribe(ctx)
	defer bus.Unsubscribe(subID)

	item, err := storage.CreateBacklogItem(ctx, backlogdto.BacklogItemData{Title: "x"})
	require.NoError(t, err)
	itemID := uuid.MustParse(item.ID)
	row, err := storage.CreateGuidanceRequest(ctx, domain.CreateGuidanceRequestInput{
		Scope: domain.RequestScopeBacklogItem, ItemID: &itemID, QuestionText: "ok?", QuestionType: domain.QuestionTypeYesNo,
	})
	require.NoError(t, err)
	_, applied, err := svc.AnswerAs(ctx, row.ID, "yes", guidance.HumanAnswerer())
	require.NoError(t, err)
	require.True(t, applied)

	select {
	case ev := <-ch:
		assert.Equal(t, events.EventNotification, ev.Type)
		assert.Equal(t, item.ID, ev.SessionID)
		assert.Equal(t, int32(sessionv1.NotificationType_NOTIFICATION_TYPE_INPUT_REQUIRED), ev.NotificationType)
		assert.Equal(t, services.DerivePriority(true, true), ev.NotificationPriority)
	default:
		t.Fatal("no notification published")
	}
}
