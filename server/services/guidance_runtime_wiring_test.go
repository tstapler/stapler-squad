package services

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

func TestGetGuidanceRequest_should_DeriveUnacknowledgedSessionEndedAndCancelReason(t *testing.T) {
	t.Parallel()
	m, storage, _, _ := newTestMirror(t)
	svc := NewGuidanceRequestService(storage, nil, nil)
	ended := false
	unconfirmed := true
	svc.SetDeliveryProbes(DeliveryProbes{
		Unconfirmed:     func(string) bool { return unconfirmed },
		SessionEnded:    func(string) bool { return ended },
		SessionArchived: func(string) bool { return ended },
	})
	ctx := context.Background()
	get := func(id string) *sessionv1.GuidanceRequest {
		resp, err := svc.GetGuidanceRequest(ctx, connect.NewRequest(&sessionv1.GetGuidanceRequestRequest{Id: id}))
		require.NoError(t, err)
		return resp.Msg.GetRequest()
	}

	sent, _ := m.Mirror(mirrorReq(t, "toolu_sent", "Sent?", true))
	_, err := storage.AnswerGuidanceRequest(ctx, uuid.MustParse(sent), "Red")
	require.NoError(t, err)
	for _, state := range []domain.DeliveryState{domain.DeliveryStatePending, domain.DeliveryStateSent} {
		applied, serr := storage.SetGuidanceDelivery(ctx, uuid.MustParse(sent), domain.DeliveryRouteLiveKeystroke, state, 1)
		require.NoError(t, serr)
		require.True(t, applied, state)
	}

	got := get(sent)
	assert.True(t, got.GetUnacknowledged())
	assert.False(t, got.GetSessionEnded())
	ended, unconfirmed = true, false
	got = get(sent)
	assert.False(t, got.GetUnacknowledged())
	assert.True(t, got.GetSessionEnded(), "row 17: the session ended while the answer was sent")

	open, _ := m.Mirror(mirrorReq(t, "toolu_open", "Open?", true))
	assert.False(t, get(open).GetSessionEnded(), "only sent or failed rows carry the ended probe")
	assert.True(t, get(open).GetSessionArchived(), "an open card on an archived session says so before the operator acts")
	require.NoError(t, m.Clear(ctx, open, domain.CancelReasonAnsweredInTerminal))
	assert.Equal(t, "answered_in_terminal", get(open).GetCancelReason())
}

func TestListSavedGuidanceAnswers_should_ReturnOnlyAgentCreatedSavedRows(t *testing.T) {
	t.Parallel()
	m, storage, _, _ := newTestMirror(t)
	svc := NewGuidanceRequestService(storage, nil, nil)
	ctx := context.Background()
	save := func(id string, route domain.DeliveryRoute) {
		_, err := storage.AnswerGuidanceRequest(ctx, uuid.MustParse(id), "Redis")
		require.NoError(t, err)
		_, err = storage.SetGuidanceDelivery(ctx, uuid.MustParse(id), route, domain.DeliveryStateSavedForLater, 0)
		require.NoError(t, err)
	}
	agent, err := storage.CreateGuidanceRequest(ctx, session.CreateGuidanceRequestInput{
		Scope: domain.RequestScopeSession, SessionUUID: mirrorSession, QuestionText: "Which cache?",
		QuestionType: domain.QuestionTypeMultipleChoice, Options: `["Redis","Memcached"]`,
	})
	require.NoError(t, err)
	save(agent.ID, domain.DeliveryRouteResume)
	hook, _ := m.Mirror(mirrorReq(t, "toolu_h", "Hook?", true))
	save(hook, domain.DeliveryRouteReadOnly)

	resp, err := svc.ListSavedGuidanceAnswers(ctx, connect.NewRequest(&sessionv1.ListSavedGuidanceAnswersRequest{}))

	require.NoError(t, err)
	require.Len(t, resp.Msg.GetRequests(), 1)
	got := resp.Msg.GetRequests()[0]
	assert.Equal(t, agent.ID, got.GetId())
	assert.Equal(t, "resume", got.GetDeliveryRoute(), "stopped, not archived")
}

func TestAnswerAs_should_SaveAnAgentAnswerForLater_WhenItsSessionIsStoppedOrArchived_AndNeverRevive(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	svc := NewGuidanceRequestService(storage, nil, nil)
	ended, archived := false, false
	svc.SetDeliveryProbes(DeliveryProbes{
		SessionEnded:    func(string) bool { return ended },
		SessionArchived: func(string) bool { return archived },
	})
	ctx := context.Background()
	ask := func(text string) string {
		row, err := storage.CreateGuidanceRequest(ctx, session.CreateGuidanceRequestInput{
			Scope: domain.RequestScopeSession, SessionUUID: mirrorSession, QuestionText: text,
			QuestionType: domain.QuestionTypeMultipleChoice, Options: `["Redis","Memcached"]`,
		})
		require.NoError(t, err)
		return row.ID
	}
	answer := func(id string) *session.GuidanceRequestData {
		_, applied, err := svc.AnswerAs(ctx, id, "Redis", HumanAnswerer())
		require.NoError(t, err)
		require.True(t, applied)
		row, err := storage.GetGuidanceRequest(ctx, uuid.MustParse(id))
		require.NoError(t, err)
		return row
	}

	live := answer(ask("live?"))
	assert.Empty(t, live.DeliveryState, "a live session's agent reads the answer itself")

	ended = true
	stopped := answer(ask("stopped?"))
	assert.Equal(t, domain.DeliveryStateSavedForLater, stopped.DeliveryState)
	assert.Equal(t, domain.DeliveryRouteResume, stopped.DeliveryRoute)

	archived = true
	gone := answer(ask("archived?"))
	assert.Equal(t, domain.DeliveryStateSavedForLater, gone.DeliveryState)
	assert.Equal(t, domain.DeliveryRouteReadOnly, gone.DeliveryRoute, "archived: stored, never delivered unless the operator resumes it")
}
