package services

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
)

func createPeerRequest(t *testing.T, storage *session.Storage, asker, answerer string) *session.GuidanceRequestData {
	t.Helper()
	req, err := domain.NewGuidanceRequest(domain.GuidanceRequestParams{
		Scope: domain.RequestScopeSession, SessionUUID: asker, QuestionText: "How do I run the linter?",
		QuestionType: domain.QuestionTypeShortAnswer, Source: domain.QuestionSourcePeer, AskerSessionUUID: asker,
		AnswererKind: domain.AnswererKindSession, AnswererSessionUUID: answerer,
	})
	require.NoError(t, err)
	row, err := storage.CreateMirroredGuidanceRequest(context.Background(), req, 3)
	require.NoError(t, err)
	return row
}

func TestAnswerAs_should_AuthorizeOnlyTheNamedSessionOrTheHuman(t *testing.T) {
	t.Parallel()
	storage := svctest.CreateTestStorage(t)
	svc := NewGuidanceRequestService(storage, nil, nil)
	ctx := context.Background()
	peer := createPeerRequest(t, storage, "asker-a", "peer-b")
	human, err := storage.CreateGuidanceRequest(ctx, session.CreateGuidanceRequestInput{
		Scope: domain.RequestScopeSession, SessionUUID: "asker-a", QuestionText: "ok?", QuestionType: domain.QuestionTypeShortAnswer,
	})
	require.NoError(t, err)

	tests := []struct {
		name string
		id   string
		who  Answerer
		ok   bool
	}{
		{"another session on a peer row", peer.ID, SessionAnswerer("peer-c"), false},
		{"the asker on its own peer row", peer.ID, SessionAnswerer("asker-a"), false},
		{"an empty session id", peer.ID, SessionAnswerer(""), false},
		{"a session on a human-addressed row", human.ID, SessionAnswerer("peer-b"), false},
		{"the zero value", peer.ID, Answerer{}, false},
		{"the named session", peer.ID, SessionAnswerer("peer-b"), true},
		{"the operator on a peer row", createPeerRequest(t, storage, "asker-a", "peer-d").ID, HumanAnswerer(), true},
		{"the operator on a human row", human.ID, HumanAnswerer(), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, applied, err := svc.AnswerAs(ctx, tt.id, "answer", tt.who)
			if !tt.ok {
				require.ErrorIs(t, err, ErrAnswererForbidden)
				assert.False(t, applied)
				return
			}
			require.NoError(t, err)
			assert.True(t, applied)
			assert.Equal(t, "answer", data.Answer)
		})
	}
	row, err := storage.GetGuidanceRequest(ctx, uuid.MustParse(peer.ID))
	require.NoError(t, err)
	assert.Equal(t, "answer", row.Answer, "the forbidden attempts changed nothing; only peer-b's answer landed")
}

func TestAnswerAs_should_SkipTheHumanNotification_When_RowIsAPeerRequest(t *testing.T) {
	t.Parallel()
	storage := svctest.CreateTestStorage(t)
	bus := events.NewEventBus(10)
	svc := NewGuidanceRequestService(storage, bus, nil)
	ctx := context.Background()
	peer := createPeerRequest(t, storage, "asker-a", "peer-b")
	var heard []string
	svc.SetAnswerListener(func(_ context.Context, id string) { heard = append(heard, id) })
	ch, subID := bus.Subscribe(ctx)
	defer bus.Unsubscribe(subID)

	_, applied, err := svc.AnswerAs(ctx, peer.ID, "use approach 2", SessionAnswerer("peer-b"))

	require.NoError(t, err)
	require.True(t, applied)
	select {
	case ev := <-ch:
		t.Fatalf("a peer answer must not raise a human-guidance notification, got %+v", ev)
	default:
	}
	assert.Equal(t, []string{peer.ID}, heard, "the asker is reached through the answer listener's wake instead")
}

func TestAnswerAs_should_ReturnNotFound_When_RequestDoesNotExist(t *testing.T) {
	t.Parallel()
	svc := NewGuidanceRequestService(svctest.CreateTestStorage(t), nil, nil)
	_, _, err := svc.AnswerAs(context.Background(), "5d1c8ec3-3f6b-4c0e-9a3e-0a6b4d6b1f11", "x", HumanAnswerer())
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestAnswerAs_should_WaiveCriterion_Only_When_OwnerPicksWaiveResponse(t *testing.T) {
	t.Parallel()
	storage := svctest.CreateTestStorage(t)
	svc := NewGuidanceRequestService(storage, nil, nil)
	ctx := context.Background()
	raw, err := domain.SerializeAcCriteria([]domain.AcCriterion{
		{Index: 0, Text: "a", Status: domain.AcStatusPending},
		{Index: 1, Text: "b", Status: domain.AcStatusPending},
	})
	require.NoError(t, err)
	item, err := storage.CreateBacklogItem(ctx, backlogdto.BacklogItemData{Title: "w", Status: string(domain.BacklogStatusInProgress), AcceptanceCriteria: raw})
	require.NoError(t, err)
	itemID := uuid.MustParse(item.ID)
	ask := func(idx int) string {
		row, err := storage.CreateGuidanceRequest(ctx, session.CreateGuidanceRequestInput{
			Scope: domain.RequestScopeBacklogItem, ItemID: &itemID, QuestionType: domain.QuestionTypeMultipleChoice,
			QuestionText: domain.OwnerActionQuestionPrefix + domain.FormatOwnerActionWaiver(idx) + "AC unattainable",
			Options:      `["Waive criterion","Keep"]`,
		})
		require.NoError(t, err)
		return row.ID
	}
	statuses := func() []domain.AcStatus {
		got, err := storage.GetBacklogItem(ctx, item.ID)
		require.NoError(t, err)
		cs, err := domain.ParseAcCriteria(got.AcceptanceCriteria)
		require.NoError(t, err)
		out := make([]domain.AcStatus, len(cs))
		for i, c := range cs {
			out[i] = c.Status
		}
		return out
	}

	_, applied, err := svc.AnswerAs(ctx, ask(1), "Keep", HumanAnswerer())
	require.NoError(t, err)
	require.True(t, applied)
	assert.Equal(t, []domain.AcStatus{domain.AcStatusPending, domain.AcStatusPending}, statuses(), "other answers waive nothing")

	_, applied, err = svc.AnswerAs(ctx, ask(1), domain.OwnerActionWaiveResponse, HumanAnswerer())
	require.NoError(t, err)
	require.True(t, applied)
	assert.Equal(t, []domain.AcStatus{domain.AcStatusPending, domain.AcStatusWaived}, statuses())

	// A second waive of the same criterion is refused by storage; the answer still lands.
	_, applied, err = svc.AnswerAs(ctx, ask(1), domain.OwnerActionWaiveResponse, HumanAnswerer())
	require.NoError(t, err)
	assert.True(t, applied)
}
