package services

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/server/notifications"
	"github.com/tstapler/stapler-squad/server/services/guidance"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/server/services/notify"
	"github.com/tstapler/stapler-squad/server/services/questionkit"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/instance"
	"github.com/tstapler/stapler-squad/session/queue"
)

const archivedAsker = "uuid-archived-asker"

type sourcesEnv struct {
	svc     *SessionService
	storage *session.Storage
	history *notifications.NotificationHistoryStore
}

// newSourcesEnv is an unwired service with a history store, every question
// related flag scope on, and one archived session known to the live poller.
func newSourcesEnv(t *testing.T) *sourcesEnv {
	t.Helper()
	svctest.WithIsolatedStateDir(t)
	cfg := config.LoadConfig()
	for _, sc := range []string{"action:ANSWER_QUESTION", "action:ANSWER_GUIDANCE", "action:BACKLOG_DECISION", "channel:remote"} {
		require.NoError(t, cfg.SetFeatureFlagScope("notification_inline_actions", sc, true))
	}
	for _, st := range []string{"stage:render", "stage:answer"} {
		require.NoError(t, cfg.SetFeatureFlagScope("askuserquestion_structured", st, true))
	}

	e := &sourcesEnv{storage: svctest.CreateTestStorage(t)}
	bus := events.NewEventBus(16)
	t.Cleanup(bus.Close)
	e.svc = NewSessionService(e.storage, bus)
	t.Cleanup(e.svc.Shutdown)
	history, err := notifications.NewNotificationHistoryStore(filepath.Join(t.TempDir(), "notifications.json"))
	require.NoError(t, err)
	e.history = history
	e.svc.SetNotificationStore(history)

	inst, err := instance.NewInstance(instance.InstanceOptions{Title: "archived-asker", Path: t.TempDir(), Program: "claude"})
	require.NoError(t, err)
	inst.UUID = archivedAsker
	archiveNow(inst)
	poller := session.NewReviewQueuePoller(queue.NewReviewQueue(), instance.NewInstanceStatusManager(), nil)
	poller.SetInstances([]*instance.Instance{inst})
	e.svc.SetReviewQueuePoller(poller)
	return e
}

func (e *sourcesEnv) card(t *testing.T, id, sessionID string, md map[string]string) {
	t.Helper()
	require.NoError(t, e.history.Append(&notifications.NotificationRecord{
		ID: id, SessionID: sessionID, NotificationType: 1, Title: id, CreatedAt: time.Unix(1_800_000_000, 0), Metadata: md,
	}))
}

// descriptor returns the action of kind offered on card id, or nil.
func (e *sourcesEnv) descriptor(t *testing.T, id string, kind sessionv1.ActionKind) *sessionv1.NotificationActionDescriptor {
	t.Helper()
	resp, err := e.svc.GetNotificationHistory(context.Background(), connect.NewRequest(&sessionv1.GetNotificationHistoryRequest{}))
	require.NoError(t, err)
	for _, r := range resp.Msg.GetNotifications() {
		if r.GetId() != id {
			continue
		}
		require.NotEmpty(t, r.GetActions(), "card %s was annotated", id)
		for _, a := range r.GetActions() {
			if a.GetKind() == kind {
				return a
			}
		}
		return nil
	}
	t.Fatalf("no card %s", id)
	return nil
}

// A nil *session.Storage must not reach Deps as a non-nil interface: the
// storage lookups would then dereference it while annotating these cards.
func TestWireNotificationActions_ShouldLeaveStorageLookupsOut_WhenThereIsNoStorage(t *testing.T) {
	e := newSourcesEnv(t)
	e.card(t, "decision", "s1", map[string]string{"item_id": "item-1", "decision_schema": notify.EncodeDecisionSchema(notify.DecisionSchema{
		Choices: []notify.DecisionChoice{{Value: notify.ChoiceAccept, Label: "Accept"}},
	})})
	e.card(t, "guidance", "s2", map[string]string{"guidance_request_id": "g1"})

	e.svc.WireNotificationActions(NotificationActionWiring{})

	assert.Nil(t, e.descriptor(t, "decision", sessionv1.ActionKind_ACTION_KIND_BACKLOG_DECISION))
	assert.Nil(t, e.descriptor(t, "guidance", sessionv1.ActionKind_ACTION_KIND_ANSWER_GUIDANCE))
}

// Questions supplies the card's options, Routes keeps an archived session's
// stored answer available to a remote reader, and the wired canary degrades it.
func TestWireNotificationActions_ShouldDescribeQuestionCardsFromPendingQuestionsRoutesAndCanary(t *testing.T) {
	e := newSourcesEnv(t)
	q := questionkit.PendingQuestion{QuestionID: "q1", SessionUUID: archivedAsker}
	q.Dialog.Labels = []string{"Redis", "Memcached"}
	e.svc.PendingQuestions().Register(q)
	e.card(t, "question", archivedAsker, map[string]string{"question_id": "q1"})
	e.svc.WireNotificationActions(NotificationActionWiring{Storage: e.storage})
	answer := sessionv1.ActionKind_ACTION_KIND_ANSWER_QUESTION

	d := e.descriptor(t, "question", answer)
	require.NotNil(t, d, "Questions")
	assert.Len(t, d.GetOptions(), 2)
	assert.True(t, d.GetAvailable(), "Routes: %s", d.GetUnavailableReason())
	assert.Equal(t, sessionv1.ActionTier_ACTION_TIER_A, d.GetTier(), "Routes: a stored answer")

	for range questionkit.CanaryMinAttempts {
		e.svc.wireDialogCanary().Record(questionkit.DialogMismatch)
	}
	d = e.descriptor(t, "question", answer)
	require.NotNil(t, d)
	assert.False(t, d.GetAvailable(), "Canary")
	assert.Equal(t, "read_only", d.GetUnavailableReason())
}

func TestWireNotificationActions_ShouldInstallDeliveryProbesOnTheGuidanceService(t *testing.T) {
	e := newSourcesEnv(t)
	guidanceSvc := guidance.NewGuidanceRequestService(e.storage, nil, nil)
	e.svc.WireNotificationActions(NotificationActionWiring{Storage: e.storage, Guidance: guidanceSvc})
	ctx := context.Background()
	created, err := guidanceSvc.CreateGuidanceRequest(ctx, connect.NewRequest(&sessionv1.CreateGuidanceRequestRequest{
		Scope: "session", SessionUuid: archivedAsker, CallerSessionUuid: archivedAsker,
		QuestionText: "Which target?", QuestionType: "short-answer",
	}))
	require.NoError(t, err)

	got, err := guidanceSvc.GetGuidanceRequest(ctx, connect.NewRequest(&sessionv1.GetGuidanceRequestRequest{Id: created.Msg.GetRequest().GetId()}))

	require.NoError(t, err)
	assert.True(t, got.Msg.GetRequest().GetSessionArchived())
}
