package services

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/services/access"
	"github.com/tstapler/stapler-squad/server/services/featureflags"
	"github.com/tstapler/stapler-squad/server/services/flagsvc"
	"github.com/tstapler/stapler-squad/server/services/guidance"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/server/services/qareply"
	"github.com/tstapler/stapler-squad/server/services/questionkit"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/instance"
)

// Each test here fails when one line of the Reply wiring is removed.

// FindInstance, AuditSink and Count, plus the RPC delegator and PendingQuestions.
func TestReplyWiring_ShouldReachTheLivePollerAuditSinkAndMetrics(t *testing.T) {
	e := newReplyEnv(t)

	assert.Equal(t, sessionv1.ReplyOutcome_REPLY_OUTCOME_SENT, e.replyOK("2", "r-wired").Outcome)

	assert.Equal(t, []string{"2"}, e.pm.writes())
	assert.EqualValues(t, 1, e.counter("sent"))
	assert.Len(t, e.replyLines(), 2)
}

// Canary: replies that miss the dialog degrade the canary installed on the service.
func TestReplyWiring_ShouldRecordDialogMatchesOnTheServiceCanary(t *testing.T) {
	e := newReplyEnv(t)
	canary := e.fix.svc.wireDialogCanary()
	e.pm.dialog = "user@host:~$ \n"

	for i := range questionkit.CanaryMinAttempts {
		e.clock.Advance(time.Minute)
		assert.Equal(t, sessionv1.ReplyOutcome_REPLY_OUTCOME_STALE_PROMPT, e.replyOK("1", "r-miss-"+string(rune('a'+i))).Outcome)
	}

	assert.True(t, canary.Tripped())
}

// InstalledMirror: SetGuidanceMirror after the router was built is still used to
// re-register an expired question (covered end to end by
// TestAnswerRouter_should_ReVerifyDialogMatchAndWriteOneByte_When_AnsweredPastTheCacheTTL).
func TestReplyWiring_ShouldReadTheMirrorInstalledOnTheService(t *testing.T) {
	e := newReplyEnv(t)
	assert.Nil(t, e.fix.svc.replyPorts().InstalledMirror())
	mirror := questionkit.NewGuidanceHookMirror(e.fix.storage, e.fix.svc.PendingQuestions(), nil, nil, func() bool { return true })

	e.fix.svc.SetGuidanceMirror(mirror)

	assert.Same(t, mirror, e.fix.svc.replyPorts().InstalledMirror())
}

// The guards flag has no effect on Reply in either state (T-RP-21).
func TestReply_ShouldIgnoreTheGuardsFlag_WhenItIsOffAndWhenOn(t *testing.T) {
	e := newReplyEnv(t)
	e.fix.svc.guards = &access.UnaryGuardsFlag{}
	e.fix.svc.guards.SetEnabled(false)
	assert.Equal(t, sessionv1.ReplyOutcome_REPLY_OUTCOME_SENT, e.replyOK("1", "r-g-off").Outcome)
}

// wireReplyFlag and the audit policy, as the gated constructor wires them.
func TestReplyWiring_ShouldGateReplyBehindTheFlagService_WhenConstructedWithAGate(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	svc := newGatedSessionService(svctest.CreateTestStorage(t), testFlagRegistry())
	t.Cleanup(svc.Shutdown)
	detail := func() string {
		resp, err := svc.GetFeatureFlags(context.Background(), connect.NewRequest(&sessionv1.GetFeatureFlagsRequest{}))
		require.NoError(t, err)
		for _, f := range resp.Msg.GetFlags() {
			if f.GetName() == featureflags.HiddenSessionReplyFlagName {
				return f.GetStatusDetail()
			}
		}
		t.Fatal("flag not listed")
		return ""
	}

	require.NoError(t, flagUpdateCtx(context.Background(), svc.featureFlagSvc, featureflags.HiddenSessionReplyFlagName, false))
	assert.Contains(t, detail(), "Hidden-session Reply is OFF")
	require.NoError(t, flagUpdateCtx(context.Background(), svc.featureFlagSvc, featureflags.HiddenSessionReplyFlagName, true))
	assert.NotContains(t, detail(), "Hidden-session Reply is OFF")
}

// Opening the Reply path is refused when its audit line cannot be made durable.
func TestReplyWiring_ShouldRefuseToEnableReply_WhenTheAuditSinkIsDown(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	dir, err := config.GetConfigDir()
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "audit"), []byte("not a directory"), 0o600))
	svc := newGatedSessionService(svctest.CreateTestStorage(t), testFlagRegistry())
	t.Cleanup(svc.Shutdown)

	err = flagUpdateCtx(context.Background(), svc.featureFlagSvc, featureflags.HiddenSessionReplyFlagName, true)

	assert.Error(t, err)
}

// The ack detector drops what it tracks for a session that can take no keystroke.
func TestReplyWiring_ShouldTellTheAckDetectorWhenASessionEnded(t *testing.T) {
	e := newReplyEnv(t)
	acks := e.fix.svc.wireGuidanceAcks(e.fix.storage)
	t.Cleanup(e.fix.svc.stopGuidanceAcks)
	acks.Sent(questionkit.SentDelivery{RowID: "7c1e0000-0000-4000-8000-00000000c001", SessionUUID: "gone", Indeterminate: true})

	acks.Tick(context.Background())

	assert.False(t, acks.Unconfirmed("7c1e0000-0000-4000-8000-00000000c001"))
}

// SessionDeleted: a deleted session's questions are dropped.
func TestReplyWiring_ShouldDropPendingQuestions_WhenASessionIsDeleted(t *testing.T) {
	e := newReplyEnv(t)
	stored, err := instance.NewInstance(instance.InstanceOptions{Title: "reply-deleted", Path: t.TempDir(), Program: "claude"})
	require.NoError(t, err)
	stored.UUID = "reply-deleted-uuid"
	require.NoError(t, e.fix.storage.AddInstance(stored))
	qid := e.register(stored, colorDialog)

	_, err = e.fix.svc.DeleteSession(context.Background(), connect.NewRequest(&sessionv1.DeleteSessionRequest{Id: stored.UUID, Force: true}))
	require.NoError(t, err)

	_, ok := e.fix.svc.PendingQuestions().Peek(questionkit.QuestionKey{SessionUUID: stored.UUID, QuestionID: qid})
	assert.False(t, ok)
}

// A nil detector or storage must stay a nil interface, never a boxed nil pointer.
func TestAnswerRouterDeps_ShouldLeaveRowsAndAcksNilInterfaces_WhenNotWired(t *testing.T) {
	d := answerRouterDeps(NotificationActionWiring{}, nil)

	assert.True(t, d.Rows == nil, "Rows")
	assert.True(t, d.Acks == nil, "Acks")
	assert.True(t, ackSink(nil) == nil, "hook reply sink")
}

func TestAnswerRouterDeps_ShouldCarryStorageMirrorAndDetector_WhenWired(t *testing.T) {
	e := newReplyEnv(t)
	acks := e.fix.svc.wireGuidanceAcks(e.fix.storage)
	t.Cleanup(e.fix.svc.stopGuidanceAcks)
	mirror := &questionkit.GuidanceHookMirror{}

	d := answerRouterDeps(NotificationActionWiring{Storage: e.fix.storage, Mirror: mirror}, acks)

	assert.Equal(t, qareply.AnswerRouterDeps{Rows: e.fix.storage, Mirror: mirror, Acks: acks}, d)
}

// SessionEnded and SessionArchived back the ack detector and the delivery probes.
func TestDeliveryProbes_ShouldClassifySessionsFromTheLivePoller(t *testing.T) {
	e := newReplyEnv(t)
	stopped := e.addPane("probe-stopped", &replyPM{}, func(i *instance.Instance) { i.Status = domain.Stopped })
	archived := e.addPane("probe-archived", &replyPM{}, archiveNow)

	p := e.fix.svc.deliveryProbes(nil)

	for _, tc := range []struct {
		id              string
		ended, archived bool
	}{{e.inst.UUID, false, false}, {stopped.UUID, true, false}, {archived.UUID, true, true}, {"gone", true, false}} {
		assert.Equal(t, tc.ended, p.SessionEnded(tc.id), "ended %s", tc.id)
		assert.Equal(t, tc.archived, p.SessionArchived(tc.id), "archived %s", tc.id)
	}
	assert.True(t, p.Unconfirmed == nil)
}

// A hook-served answer is handed to the detector WireNotificationActions built.
// The hook route is closed by compiled evidence, which this test opens.
func TestReplyWiring_ShouldHandAHookServedAnswerToTheAckDetector(t *testing.T) {
	e := newSteerEnv(t)
	compiled := flagsvc.CompiledFlagEvidence
	flagsvc.CompiledFlagEvidence.S1HookReplyPass = true
	t.Cleanup(func() { flagsvc.CompiledFlagEvidence = compiled })
	enableAnswerFlags(t)
	require.NoError(t, config.LoadConfig().SetFeatureFlagScope(flagsvc.AskUserQuestionStructuredFlagName, flagsvc.ScopeStageHookReply, true))
	rows := &routerEnv{replyEnv: &replyEnv{steerEnv: e}}
	qid := rows.mirroredRow("hook-uuid", domain.Question{Text: "Which cache?", Header: "Cache", Options: options("Redis", "None")})
	e.fix.svc.WireNotificationActions(NotificationActionWiring{Storage: e.fix.storage})
	acks := e.fix.svc.GuidanceAcks()
	require.NotNil(t, acks)
	e.fix.svc.stopGuidanceAcks() // joins the loop, so only the hook path can start tracking the row
	stored, err := e.fix.svc.hookReply.Load().PreAnswer(context.Background(), qid, "Redis")
	require.NoError(t, err)
	require.True(t, stored)

	out := e.fix.svc.HookReplier().Reply(context.Background(), "hook-uuid", map[string]interface{}{"questions": []interface{}{map[string]interface{}{
		"question": "Which cache?", "header": "Cache", "multiSelect": false,
		"options": []interface{}{map[string]interface{}{"label": "Redis"}, map[string]interface{}{"label": "None"}},
	}}})
	require.NotNil(t, out, "the hook served the answer")
	require.Equal(t, domain.DeliveryStateSent, rows.row(qid).DeliveryState)
	acks.Receipt(context.Background(), "hook-uuid", guidance.AskUserQuestionTool)

	assert.Equal(t, domain.DeliveryStateAcknowledged, rows.row(qid).DeliveryState, "only a tracked answer is acknowledged")
}
