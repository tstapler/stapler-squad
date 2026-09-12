package services

import (
	"context"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
)

// fakeTriageRespawner is a test double for TriageRespawner recording calls,
// used by TestAnswerGuidanceRequest_should_RespawnTriage_When_BacklogItemScopeAnswered.
type fakeTriageRespawner struct {
	mu           sync.Mutex
	respawnedIDs []string
}

func (f *fakeTriageRespawner) AutoRespawnTriage(_ context.Context, itemID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.respawnedIDs = append(f.respawnedIDs, itemID)
	return nil
}

func (f *fakeTriageRespawner) called() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.respawnedIDs...)
}

// TestCreateGuidanceRequest_should_PersistDurably_When_ScopeIsStandalone covers
// AC0: creating a question requires no live chat turn (a bare RPC call) and
// the row is durably readable back via GetGuidanceRequest.
func TestCreateGuidanceRequest_should_PersistDurably_When_ScopeIsStandalone(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	svc := NewGuidanceRequestService(storage, nil, nil)
	ctx := context.Background()

	createResp, err := svc.CreateGuidanceRequest(ctx, connect.NewRequest(&sessionv1.CreateGuidanceRequestRequest{
		Scope:        "standalone",
		QuestionText: "Which deploy target should this use?",
		QuestionType: "short-answer",
	}))
	require.NoError(t, err)
	id := createResp.Msg.GetRequest().GetId()
	require.NotEmpty(t, id)
	assert.Equal(t, "pending", createResp.Msg.GetRequest().GetStatus())

	getResp, err := svc.GetGuidanceRequest(ctx, connect.NewRequest(&sessionv1.GetGuidanceRequestRequest{Id: id}))
	require.NoError(t, err)
	assert.Equal(t, "Which deploy target should this use?", getResp.Msg.GetRequest().GetQuestionText())
}

// TestAnswerGuidanceRequest_should_PublishDurableNotification_When_ScopeIsSession
// covers AC1: answering publishes a durable (NotificationHistoryStore-bound)
// notification event addressed to the originating session, and a fresh
// GetGuidanceRequest call (simulating a resumed session) can still read the
// answer back regardless of whether anything was subscribed at answer time.
func TestAnswerGuidanceRequest_should_PublishDurableNotification_When_ScopeIsSession(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	bus := events.NewEventBus(10)
	svc := NewGuidanceRequestService(storage, bus, nil)
	ctx := context.Background()

	sessionUUID := "7c1e0000-0000-0000-0000-000000000001"
	createResp, err := svc.CreateGuidanceRequest(ctx, connect.NewRequest(&sessionv1.CreateGuidanceRequestRequest{
		Scope:             "session",
		SessionUuid:       sessionUUID,
		QuestionText:      "Approach A or B?",
		QuestionType:      "multiple-choice",
		Options:           []string{"A", "B"},
		CallerSessionUuid: sessionUUID,
	}))
	require.NoError(t, err)
	id := createResp.Msg.GetRequest().GetId()

	// Subscribe AFTER creation but BEFORE answering, mirroring a live listener
	// that happens to be connected at answer time — this just proves the event
	// carries the right addressing; durability itself doesn't depend on anyone
	// listening (see the GetGuidanceRequest call below).
	ch, subID := bus.Subscribe(ctx)
	defer bus.Unsubscribe(subID)

	answerResp, err := svc.AnswerGuidanceRequest(ctx, connect.NewRequest(&sessionv1.AnswerGuidanceRequestRequest{
		Id:     id,
		Answer: "A",
	}))
	require.NoError(t, err)
	assert.True(t, answerResp.Msg.GetApplied())
	assert.Equal(t, "A", answerResp.Msg.GetRequest().GetAnswer())

	select {
	case ev := <-ch:
		require.NotNil(t, ev)
		assert.Equal(t, events.EventNotification, ev.Type)
		assert.Equal(t, sessionUUID, ev.SessionID, "notification must be addressed to the originating session")
		assert.Equal(t, int32(sessionv1.NotificationType_NOTIFICATION_TYPE_INPUT_REQUIRED), ev.NotificationType)
	default:
		t.Fatal("expected a notification event to be published synchronously on answer")
	}

	// A fresh/resumed caller with no relation to the above subscription can
	// still retrieve the answer purely from durable storage.
	getResp, err := svc.GetGuidanceRequest(ctx, connect.NewRequest(&sessionv1.GetGuidanceRequestRequest{Id: id}))
	require.NoError(t, err)
	assert.Equal(t, "answered", getResp.Msg.GetRequest().GetStatus())
	assert.Equal(t, "A", getResp.Msg.GetRequest().GetAnswer())
}

// TestAnswerGuidanceRequest_should_NotReapplyOrRenotify_When_AlreadyAnswered
// is the RPC-level half of AC4 (the repository-level race is already covered
// by session/ent_repository_guidance_test.go): answering an already-answered
// request returns applied=false with the CURRENT persisted answer rather than
// overwriting it or erroring.
func TestAnswerGuidanceRequest_should_NotReapplyOrRenotify_When_AlreadyAnswered(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	svc := NewGuidanceRequestService(storage, nil, nil)
	ctx := context.Background()

	createResp, err := svc.CreateGuidanceRequest(ctx, connect.NewRequest(&sessionv1.CreateGuidanceRequestRequest{
		Scope:        "standalone",
		QuestionText: "Ship today?",
		QuestionType: "yes-no",
	}))
	require.NoError(t, err)
	id := createResp.Msg.GetRequest().GetId()

	first, err := svc.AnswerGuidanceRequest(ctx, connect.NewRequest(&sessionv1.AnswerGuidanceRequestRequest{Id: id, Answer: "yes"}))
	require.NoError(t, err)
	assert.True(t, first.Msg.GetApplied())

	second, err := svc.AnswerGuidanceRequest(ctx, connect.NewRequest(&sessionv1.AnswerGuidanceRequestRequest{Id: id, Answer: "no"}))
	require.NoError(t, err)
	assert.False(t, second.Msg.GetApplied(), "a second answer must not be applied")
	assert.Equal(t, "yes", second.Msg.GetRequest().GetAnswer(), "the original answer must be preserved, not overwritten")
}

// TestCreateGuidanceRequest_should_RejectPermissionDenied_When_CallerNotLinkedToItem
// covers AC5 at the RPC layer: an unlinked caller is rejected exactly like
// every other mutating backlog MCP tool.
func TestCreateGuidanceRequest_should_RejectPermissionDenied_When_CallerNotLinkedToItem(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	svc := NewGuidanceRequestService(storage, nil, nil)
	ctx := context.Background()

	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{Title: "unlinked item"})
	require.NoError(t, err)

	_, err = svc.CreateGuidanceRequest(ctx, connect.NewRequest(&sessionv1.CreateGuidanceRequestRequest{
		Scope:             "backlog-item",
		ItemId:            &item.ID,
		QuestionText:      "Merge PR first?",
		QuestionType:      "yes-no",
		CallerSessionUuid: "some-unrelated-caller",
	}))
	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodePermissionDenied, connectErr.Code())
}

// TestCreateGuidanceRequest_should_RejectPermissionDenied_When_SessionScopeMismatch
// covers AC5 for scope=session: a caller may only ask about its own session.
func TestCreateGuidanceRequest_should_RejectPermissionDenied_When_SessionScopeMismatch(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	svc := NewGuidanceRequestService(storage, nil, nil)
	ctx := context.Background()

	_, err := svc.CreateGuidanceRequest(ctx, connect.NewRequest(&sessionv1.CreateGuidanceRequestRequest{
		Scope:             "session",
		SessionUuid:       "target-session",
		QuestionText:      "Continue with plan A?",
		QuestionType:      "yes-no",
		CallerSessionUuid: "different-caller-session",
	}))
	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodePermissionDenied, connectErr.Code())
}

// TestCreateGuidanceRequest_should_RejectResourceExhausted_When_OverPendingCap
// covers AC6 at the RPC layer (the repository-level race/boundary cases are
// covered by session/ent_repository_guidance_test.go).
func TestCreateGuidanceRequest_should_RejectResourceExhausted_When_OverPendingCap(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	svc := NewGuidanceRequestService(storage, nil, nil)
	ctx := context.Background()

	for i := 0; i < session.DefaultGuidanceRequestPendingCap; i++ {
		_, err := svc.CreateGuidanceRequest(ctx, connect.NewRequest(&sessionv1.CreateGuidanceRequestRequest{
			Scope:        "standalone",
			QuestionText: "distinct question " + string(rune('A'+i)),
			QuestionType: "yes-no",
		}))
		require.NoError(t, err)
	}

	_, err := svc.CreateGuidanceRequest(ctx, connect.NewRequest(&sessionv1.CreateGuidanceRequestRequest{
		Scope:        "standalone",
		QuestionText: "one too many",
		QuestionType: "yes-no",
	}))
	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeResourceExhausted, connectErr.Code())
}

// TestAnswerGuidanceRequest_should_RespawnTriage_When_BacklogItemScopeAnswered
// covers AC2: answering a backlog-item-scoped guidance request (the shape
// automated triage creates when halting on ambiguity) resumes automated
// triage by respawning it, rather than leaving the item halted forever
// waiting for a human to manually re-trigger it.
func TestAnswerGuidanceRequest_should_RespawnTriage_When_BacklogItemScopeAnswered(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	respawner := &fakeTriageRespawner{}
	svc := NewGuidanceRequestService(storage, nil, respawner)
	ctx := context.Background()

	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{Title: "ambiguous item"})
	require.NoError(t, err)
	callerUUID := "triage-session-1"
	_, err = storage.CreateItemSession(ctx, session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: callerUUID,
		SessionRole: "triage",
	})
	require.NoError(t, err)

	createResp, err := svc.CreateGuidanceRequest(ctx, connect.NewRequest(&sessionv1.CreateGuidanceRequestRequest{
		Scope:             "backlog-item",
		ItemId:            &item.ID,
		QuestionText:      "Should this include the mobile client changes?",
		QuestionType:      "yes-no",
		CallerSessionUuid: callerUUID,
	}))
	require.NoError(t, err)
	id := createResp.Msg.GetRequest().GetId()

	_, err = svc.AnswerGuidanceRequest(ctx, connect.NewRequest(&sessionv1.AnswerGuidanceRequestRequest{Id: id, Answer: "yes"}))
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		return len(respawner.called()) == 1
	}, time.Second, 10*time.Millisecond, "expected AutoRespawnTriage to be called exactly once")
	assert.Equal(t, []string{item.ID}, respawner.called())

	notes, err := storage.ListActivityNotesForItem(ctx, item.ID)
	require.NoError(t, err)
	require.Len(t, notes, 1)
	assert.Contains(t, notes[0].Message, "yes")
}
