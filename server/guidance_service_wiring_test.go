package server

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/server/services/backlogsvc"
	"github.com/tstapler/stapler-squad/server/services/guidance"
	"github.com/tstapler/stapler-squad/server/services/notify"
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
		assert.Equal(t, notify.DerivePriority(true, true), ev.NotificationPriority)
	default:
		t.Fatal("no notification published")
	}
}

// newWaiverItem is an item in status with one pending criterion, and the request id of an
// owner-action question that asks to waive it.
func newWaiverItem(t *testing.T, storage *session.Storage, status domain.BacklogStatus) (itemID, requestID string) {
	t.Helper()
	ctx := context.Background()
	criteria, err := domain.SerializeAcCriteria([]domain.AcCriterion{{Index: 0, Text: "a", Status: domain.AcStatusPending}})
	require.NoError(t, err)
	item, err := storage.CreateBacklogItem(ctx, backlogdto.BacklogItemData{Title: "w", Status: string(status), AcceptanceCriteria: criteria})
	require.NoError(t, err)
	id := uuid.MustParse(item.ID)
	row, err := storage.CreateGuidanceRequest(ctx, domain.CreateGuidanceRequestInput{
		Scope: domain.RequestScopeBacklogItem, ItemID: &id, QuestionType: domain.QuestionTypeMultipleChoice,
		QuestionText: domain.OwnerActionQuestionPrefix + domain.FormatOwnerActionWaiver(0) + "AC unattainable",
		Options:      `["Waive criterion","Keep"]`,
	})
	require.NoError(t, err)
	return item.ID, row.ID
}

func newGuidanceWiringStorage(t *testing.T) *session.Storage {
	t.Helper()
	storage, err := session.NewStorageWithRepository(session.NewTestEntRepository(t))
	require.NoError(t, err)
	return storage
}

func statusOf(t *testing.T, storage *session.Storage, itemID string) domain.BacklogStatus {
	t.Helper()
	item, err := storage.GetBacklogItem(context.Background(), itemID)
	require.NoError(t, err)
	return domain.BacklogStatus(item.Status)
}

func TestNewGuidanceRequestService_should_ResumeThroughTheBacklogService_When_OneIsWired(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	storage := newGuidanceWiringStorage(t)
	backlog := backlogsvc.NewBacklogService(storage, nil, &config.Config{}, nil, nil, nil)
	t.Cleanup(backlog.Shutdown)
	svc := newGuidanceRequestService(&ServerDependencies{Storage: storage, EventBus: events.NewEventBus(8), BacklogService: backlog})

	// The waiver resumer reopens a reviewed item before AnswerAs returns.
	// With a live work session the reopen reuses it instead of spawning one.
	reviewed, waiver := newWaiverItem(t, storage, domain.BacklogStatusReview)
	_, err := storage.CreateItemSession(ctx, backlogdto.ItemSessionData{ItemID: reviewed, SessionUUID: "live-work", SessionRole: "work"})
	require.NoError(t, err)
	_, applied, err := svc.AnswerAs(ctx, waiver, domain.OwnerActionWaiveResponse, guidance.HumanAnswerer())
	require.NoError(t, err)
	require.True(t, applied)
	assert.Equal(t, domain.BacklogStatusInProgress, statusOf(t, storage, reviewed), "waiver resumer not wired")

	// The triage respawner moves a queued item back to idea before triaging it again.
	queued, question := newWaiverItem(t, storage, domain.BacklogStatusQueued)
	_, applied, err = svc.AnswerAs(ctx, question, "Keep", guidance.HumanAnswerer())
	require.NoError(t, err)
	require.True(t, applied)
	require.Eventually(t, func() bool { return statusOf(t, storage, queued) == domain.BacklogStatusIdea },
		10*time.Second, 10*time.Millisecond, "triage respawner not wired")
}

// A method value on a nil *BacklogService only panics when it is called.
func TestNewGuidanceRequestService_should_LeaveTheResumersUnset_When_ThereIsNoBacklogService(t *testing.T) {
	t.Parallel()
	storage := newGuidanceWiringStorage(t)
	svc := newGuidanceRequestService(&ServerDependencies{Storage: storage, EventBus: events.NewEventBus(8)})
	itemID, waiver := newWaiverItem(t, storage, domain.BacklogStatusReview)

	_, applied, err := svc.AnswerAs(context.Background(), waiver, domain.OwnerActionWaiveResponse, guidance.HumanAnswerer())

	require.NoError(t, err)
	assert.True(t, applied)
	assert.Equal(t, domain.BacklogStatusReview, statusOf(t, storage, itemID))
}
