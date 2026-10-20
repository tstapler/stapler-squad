package services

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

// enableInline turns on the inline-action flag for the given scopes and the
// answer stages, in an isolated state dir.
func enableInline(t *testing.T, scopes ...string) {
	t.Helper()
	withIsolatedStateDir(t)
	cfg := config.LoadConfig()
	for _, sc := range scopes {
		require.NoError(t, cfg.SetFeatureFlagScope(notificationInlineActionsFlagName, sc, true))
	}
	for _, st := range []string{scopeStageRender, scopeStageAnswer} {
		require.NoError(t, cfg.SetFeatureFlagScope(askUserQuestionStructuredFlagName, st, true))
	}
}

func tierFixture(ev flagEvidence) *dispatchFixture {
	f := newDispatchFixture()
	f.d.flags = configActionFlags{ev: ev}
	return f
}

func remoteCtx(t *testing.T) context.Context {
	t.Helper()
	return stampedContext(t, nil, ListenerRemote, true, "onyx:8444", nil)
}

// ADR-004 / S6 finding 3: the dispatcher itself refuses a live keystroke on the
// remote listener; EvaluateLocalWrite would allow an authenticated remote caller.
func TestTierPolicy_ShouldForbidLiveKeystrokeWithZeroWrites_WhenListenerRemoteAndTierBNotEffective(t *testing.T) {
	enableInline(t, "action:ANSWER_QUESTION", scopeChannelRemote, scopeChannelTierB)

	// Scope on but the S6 gate (compiled evidence) still closed.
	f := tierFixture(flagEvidence{})
	resp := f.invoke(t, remoteCtx(t), answerReq("k1"), nil)
	assert.Equal(t, outForbid, resp.Outcome)
	assert.Equal(t, "desktop_only", resp.Detail)
	assert.Zero(t, f.questions.writes.Load(), "no pane write")

	// S6 passed but the tierB scope is off.
	require.NoError(t, config.LoadConfig().SetFeatureFlagScope(notificationInlineActionsFlagName, scopeChannelTierB, false))
	g := tierFixture(flagEvidence{s6TierBPass: true})
	assert.Equal(t, outForbid, g.invoke(t, remoteCtx(t), answerReq("k2"), nil).Outcome)
	assert.Zero(t, g.questions.writes.Load())
}

func TestTierPolicy_ShouldAllowLiveKeystrokeOnRemote_WhenTierBScopeAndS6BothEffective(t *testing.T) {
	enableInline(t, "action:ANSWER_QUESTION", scopeChannelRemote, scopeChannelTierB)
	f := tierFixture(flagEvidence{s6TierBPass: true})
	assert.Equal(t, outApplied, f.invoke(t, remoteCtx(t), answerReq("k1"), nil).Outcome)
	assert.EqualValues(t, 1, f.questions.writes.Load())
}

func TestTierPolicy_ShouldRequireRemoteChannelFlag_WhenTierBScopeOnButChannelOff(t *testing.T) {
	enableInline(t, "action:ANSWER_QUESTION", scopeChannelTierB)
	f := tierFixture(flagEvidence{s6TierBPass: true})
	assert.Equal(t, outForbid, f.invoke(t, remoteCtx(t), answerReq("k1"), nil).Outcome, "tierB needs channel:remote")
	assert.Zero(t, f.questions.writes.Load())
}

func TestTierPolicy_ShouldAllowTierAOnRemote_WhenRemoteChannelOn(t *testing.T) {
	enableInline(t, "action:APPROVE", "action:BACKLOG_DECISION", "action:ANSWER_GUIDANCE", scopeChannelRemote)
	f := tierFixture(flagEvidence{})
	assert.Equal(t, outApplied, f.invoke(t, remoteCtx(t), approveReq("k1"), nil).Outcome)
	assert.EqualValues(t, 1, f.approvals.calls.Load())
	assert.Zero(t, f.questions.writes.Load())
}

func TestTierPolicy_ShouldReturnFlagOffForTierAOnRemote_WhenRemoteChannelNotEnabled(t *testing.T) {
	enableInline(t, "action:APPROVE")
	f := tierFixture(flagEvidence{})
	assert.Equal(t, outFlagOff, f.invoke(t, remoteCtx(t), approveReq("k1"), nil).Outcome)
	assert.Zero(t, f.approvals.calls.Load())
}

func TestTierPolicy_ShouldFailClosedOnLocal_WhenRequestWasNeverStamped(t *testing.T) {
	enableInline(t, "action:ANSWER_QUESTION")
	f := tierFixture(flagEvidence{})
	// Unstamped context is treated as remote: Tier B is refused without a write.
	assert.Equal(t, outForbid, f.invoke(t, context.Background(), answerReq("k1"), nil).Outcome)
	assert.Zero(t, f.questions.writes.Load())
}

func TestTierPolicy_ShouldRunLocalVerdict_WhenListenerLocalAndTierB(t *testing.T) {
	enableInline(t, "action:ANSWER_QUESTION")
	f := tierFixture(flagEvidence{})
	assert.Equal(t, outApplied, f.invoke(t, localCtx(t), answerReq("k1"), nil).Outcome)
	assert.EqualValues(t, 1, f.questions.writes.Load())

	// A local-listener request whose Host is not loopback is the rebinding case.
	cfg := verdictServiceConfig("localhost:8543", nil)
	rebinding := stampedContext(t, &cfg, ListenerLocal, false, "evil.example:8543", nil)
	g := tierFixture(flagEvidence{})
	assert.Equal(t, outForbid, g.invoke(t, rebinding, answerReq("k2"), nil).Outcome)
	assert.Zero(t, g.questions.writes.Load())
}

func TestTierPolicy_ShouldRefuseTierB_WhenAuditLogUnavailable(t *testing.T) {
	enableInline(t, "action:ANSWER_QUESTION")
	f := tierFixture(flagEvidence{})
	f.audit.fail = true
	resp := f.invoke(t, localCtx(t), answerReq("k1"), nil)
	assert.Equal(t, sessionv1.ActionOutcome_ACTION_OUTCOME_NOT_SENT, resp.Outcome)
	assert.Zero(t, f.questions.writes.Load(), "the request line is written before any byte")
}

func TestTierPolicy_ShouldTreatDurableAnswerAsTierA_WhenRouteIsNotAKeystroke(t *testing.T) {
	t.Parallel()
	live := tierOf(sessionv1.ActionKind_ACTION_KIND_ANSWER_QUESTION, routeLiveKeystrokeHidden)
	durable := tierOf(sessionv1.ActionKind_ACTION_KIND_ANSWER_QUESTION, routeDurableRecord)
	assert.Equal(t, sessionv1.ActionTier_ACTION_TIER_B, live)
	assert.Equal(t, sessionv1.ActionTier_ACTION_TIER_A, durable)
	assert.Equal(t, sessionv1.ActionTier_ACTION_TIER_A, tierOf(sessionv1.ActionKind_ACTION_KIND_BACKLOG_DECISION, routeDurableRecord))
}
