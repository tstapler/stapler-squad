package qareply

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/services/access"
	"github.com/tstapler/stapler-squad/server/services/flagsvc"
	"github.com/tstapler/stapler-squad/server/services/notifyactions"
	"github.com/tstapler/stapler-squad/session/domain"
)

// Story 2.8: a live-keystroke answer from the authenticated remote listener
// (:8444) goes through the real AnswerRouter, DialogMatch included.
func remoteTierBEnv(t *testing.T, tierB bool) (*routerEnv, *notifyactions.NotificationActionDispatcher) {
	t.Helper()
	e := newRouterEnv(t) // isolates the state dir, so flags are set after it
	enableInline(t, "action:ANSWER_QUESTION", flagsvc.ScopeChannelRemote)
	if tierB {
		require.NoError(t, config.LoadConfig().SetFeatureFlagScope(flagsvc.NotificationInlineActionsFlagName, flagsvc.ScopeChannelTierB, true))
	}
	audit := &recordingAudit{}
	d := notifyactions.NewNotificationActionDispatcher(notifyactions.ActionDelegates{Questions: e.router}, func() notifyactions.ActionAudit { return audit }, testFlagRegistry())
	return e, d
}

func remoteAnswer(t *testing.T, e *routerEnv, d *notifyactions.NotificationActionDispatcher, sessionID, qid string) *sessionv1.InvokeNotificationActionResponse {
	t.Helper()
	req := answerReq("k-" + qid)
	req.SessionId, req.TargetKey = sessionID, qid
	req.Params = &sessionv1.NotificationActionParams{Answer: replyColorLabel}
	resp, err := d.Invoke(e.requestCtx(access.ListenerRemote, true, "onyx:8444", nil), notifyactions.ActionCall{Msg: req, Peer: "192.0.2.7:50000"})
	require.NoError(t, err)
	return resp
}

func TestRemoteTierB_ShouldDeliverKeystrokeAfterDialogMatch_WhenTierBScopeOn(t *testing.T) {
	e, d := remoteTierBEnv(t, true)
	inst, pm := e.addVisible("remote-live")
	qid := e.hookRow(inst, colorDialog)

	resp := remoteAnswer(t, e, d, inst.UUID, qid)

	assert.Equal(t, outApplied, resp.Outcome, resp.Detail)
	assert.Equal(t, []string{"2"}, pm.writes(), "exactly one digit, no Enter")
	assert.Equal(t, domain.DeliveryStateSent, e.row(qid).DeliveryState)
}

func TestRemoteTierB_ShouldWriteNothing_WhenDialogNoLongerMatches(t *testing.T) {
	e, d := remoteTierBEnv(t, true)
	inst, pm := e.addVisible("remote-stale")
	qid := e.hookRow(inst, colorDialog)
	pm.dialog = "unrelated pane content\n❯ \n"

	resp := remoteAnswer(t, e, d, inst.UUID, qid)

	assert.Equal(t, outStale, resp.Outcome, resp.Detail)
	assert.Empty(t, pm.writes())
}

func TestRemoteTierB_ShouldForbidWithZeroWrites_WhenTierBScopeOff(t *testing.T) {
	e, d := remoteTierBEnv(t, false)
	inst, pm := e.addVisible("remote-off")
	qid := e.hookRow(inst, colorDialog)

	resp := remoteAnswer(t, e, d, inst.UUID, qid)

	assert.Equal(t, outForbid, resp.Outcome)
	assert.Equal(t, "desktop_only", resp.Detail)
	assert.Empty(t, pm.writes())
}
