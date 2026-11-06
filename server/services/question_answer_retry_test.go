package services

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

// answeredThenShowing answers a hook row once, then makes the pane show the
// dialog again (frames[n] is the pane after n writes) and puts the row in state.
func (e *routerEnv) answeredThenShowing(title string, state domain.DeliveryState, afterRetry string) (*session.Instance, *replyPM, string) {
	e.t.Helper()
	inst, pm := e.addVisible(title)
	qid := e.hookRow(inst, colorDialog)
	require.Equal(e.t, outApplied, e.answer(inst, qid, replyColorLabel).Outcome)
	pm.frames = []string{pm.dialog, pm.dialog, afterRetry}
	if state == domain.DeliveryStateFailed {
		moved, err := e.fix.storage.SetGuidanceDelivery(context.Background(), uuid.MustParse(qid), domain.DeliveryRouteLiveKeystroke, state, 0)
		require.NoError(e.t, err)
		require.True(e.t, moved)
	}
	e.clock.Advance(10 * time.Second) // past the per-session reply interval
	return inst, pm, qid
}

func (e *routerEnv) retryAnyway(inst *session.Instance, qid string) ActionResult {
	e.t.Helper()
	res, err := e.router.ReplyToQuestion(e.localCtx(), QuestionAnswer{
		SessionID: inst.UUID, QuestionID: qid, OptionLabel: replyColorLabel, ReplyID: "r-" + uuid.NewString(), Confirm: true,
	})
	require.NoError(e.t, err)
	return res
}

func TestAnswerRouter_ShouldReverifyAndRewriteOneDigit_WhenRetryAnywayOnStillShowingRow(t *testing.T) {
	e := newRouterEnv(t)
	inst, pm, qid := e.answeredThenShowing("retry-still", domain.DeliveryStateFailed, "● answered\n❯ \n")

	res := e.retryAnyway(inst, qid)

	assert.Equal(t, outApplied, res.Outcome)
	assert.Equal(t, []string{"2", "2"}, pm.writes(), "exactly one more digit, no Enter")
	row := e.row(qid)
	assert.Equal(t, domain.DeliveryStateSent, row.DeliveryState, "failed -> pending -> sent")
	assert.Equal(t, 2, row.DeliveryAttempts)
	assert.Len(t, e.acks.all(), 2, "the second send is tracked for acknowledgement again")
}

func TestAnswerRouter_ShouldWriteNothingAndReportStale_WhenRetryAnywayFindsTheDialogGone(t *testing.T) {
	e := newRouterEnv(t)
	inst, pm, qid := e.answeredThenShowing("retry-gone", domain.DeliveryStateFailed, "● answered\n❯ \n")
	pm.frames = []string{pm.dialog, "● answered\n❯ \n"}

	res := e.retryAnyway(inst, qid)

	assert.Equal(t, outStale, res.Outcome)
	assert.Equal(t, []string{"2"}, pm.writes(), "only the original digit")
	assert.Equal(t, domain.DeliveryStateStale, e.row(qid).DeliveryState)
}

func TestAnswerRouter_ShouldNotRewrite_WhenAnAnsweredRowIsRetriedWithoutConfirm(t *testing.T) {
	e := newRouterEnv(t)
	inst, pm, qid := e.answeredThenShowing("retry-noconfirm", domain.DeliveryStateFailed, "● answered\n❯ \n")

	res := e.answer(inst, qid, replyColorLabel)

	assert.Equal(t, outStale, res.Outcome)
	assert.Equal(t, "dialog_gone", res.StaleReason)
	assert.Equal(t, []string{"2"}, pm.writes())
}

func TestAnswerRouter_ShouldReportAlreadyResolved_WhenRetryAnywayRacesAnAcknowledgement(t *testing.T) {
	e := newRouterEnv(t)
	inst, pm, qid := e.answeredThenShowing("retry-acked", domain.DeliveryStateSent, "● answered\n❯ \n")
	moved, err := e.fix.storage.SetGuidanceDelivery(context.Background(), uuid.MustParse(qid), domain.DeliveryRouteLiveKeystroke, domain.DeliveryStateAcknowledged, 0)
	require.NoError(t, err)
	require.True(t, moved)

	res := e.retryAnyway(inst, qid)

	assert.Equal(t, sessionv1.ActionOutcome_ACTION_OUTCOME_ALREADY_RESOLVED, res.Outcome)
	assert.Equal(t, replyColorLabel, res.Detail)
	assert.Equal(t, []string{"2"}, pm.writes())
}

func TestAnswerRouter_ShouldRewrite_WhenRetryAnywayOnAnUnconfirmedSentRow(t *testing.T) {
	e := newRouterEnv(t)
	inst, pm, qid := e.answeredThenShowing("retry-unconfirmed", domain.DeliveryStateSent, "● answered\n❯ \n")

	res := e.retryAnyway(inst, qid)

	assert.Equal(t, outApplied, res.Outcome)
	assert.Equal(t, []string{"2", "2"}, pm.writes())
	assert.Equal(t, domain.DeliveryStateSent, e.row(qid).DeliveryState)
	assert.Equal(t, 2, e.row(qid).DeliveryAttempts)
}
