package services

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session/domain"
)

func TestGetGuidanceRequest_should_DeriveUnacknowledgedSessionEndedAndCancelReason(t *testing.T) {
	t.Parallel()
	m, storage, _, _ := newTestMirror(t)
	svc := NewGuidanceRequestService(storage, nil, nil)
	ended := false
	unconfirmed := true
	svc.SetDeliveryProbes(DeliveryProbes{
		Unconfirmed:  func(string) bool { return unconfirmed },
		SessionEnded: func(string) bool { return ended },
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
	assert.False(t, get(open).GetSessionEnded(), "only sent or failed rows carry the probe")
	require.NoError(t, m.Clear(ctx, open, domain.CancelReasonAnsweredInTerminal))
	assert.Equal(t, "answered_in_terminal", get(open).GetCancelReason())
}
