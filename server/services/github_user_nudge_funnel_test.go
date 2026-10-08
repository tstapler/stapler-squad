package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	githubpkg "github.com/tstapler/stapler-squad/github"
	"github.com/tstapler/stapler-squad/session"
)

func steerReadyOf(t *testing.T, f *nudgeFixture) bool {
	t.Helper()
	out := f.svc.userPRsToProto(f.prs.prs)
	require.Len(t, out, 1)
	require.Len(t, out[0].LinkedSessions, 1)
	return out[0].LinkedSessions[0].SteerReady
}

func TestUserPRsToProto_should_MarkSteerReady_When_LiveIdleSteerableSession(t *testing.T) {
	f := newNudgeFixture(t)
	assert.True(t, steerReadyOf(t, f))
}

func TestUserPRsToProto_should_NotMarkSteerReady_When_SessionCannotTakeANudge(t *testing.T) {
	tests := []struct {
		name  string
		setup func(f *nudgeFixture)
	}{
		{"not idle", func(f *nudgeFixture) { f.nudger.notIdle = true }},
		{"paused", func(f *nudgeFixture) { f.inst.Status = session.Paused }},
		{"unsteerable program", func(f *nudgeFixture) { f.inst.Program = "aider" }},
		{"not tracked live", func(f *nudgeFixture) { f.nudger.insts = nil }},
		{"legacy fallback link", func(f *nudgeFixture) { f.prs.prs[0].LinkedSessions[0].LegacyFallback = true }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newNudgeFixture(t)
			tc.setup(f)
			assert.False(t, steerReadyOf(t, f))
		})
	}
}

func TestLogUpNextFunnel_should_CountAttentionLinkedAndLive_When_MixedPRs(t *testing.T) {
	f := newNudgeFixture(t)
	failing := []githubpkg.FailingCheck{{Name: "lint", Conclusion: "failure"}}
	idle := linkedPR("github.com", "acme", "api", 42, "alice", githubpkg.LinkedSession{SessionID: "fix-ci"})
	idle.FailingChecks = failing
	unlinked := linkedPR("github.com", "acme", "api", 43, "alice")
	unlinked.FailingChecks = failing
	healthy := linkedPR("github.com", "acme", "api", 44, "alice", githubpkg.LinkedSession{SessionID: "fix-ci"})
	healthy.FailingChecks = nil
	draft := linkedPR("github.com", "acme", "api", 45, "alice", githubpkg.LinkedSession{SessionID: "fix-ci"})
	draft.FailingChecks, draft.IsDraft = failing, true

	f.svc.logUpNextFunnel(f.svc.userPRsToProto([]githubpkg.UserPR{idle, unlinked, healthy, draft}))

	lines := f.logs.byMsg("up_next_funnel")
	require.Len(t, lines, 1)
	assert.EqualValues(t, 2, lines[0].fields["prs_needing_attention"])
	assert.EqualValues(t, 1, lines[0].fields["with_linked_session"])
	assert.EqualValues(t, 1, lines[0].fields["with_live_session"])
}

func TestNudgeOutcomeLog_should_FlagSessionLive_When_SessionResolvesVsNotTracked(t *testing.T) {
	f := newNudgeFixture(t)
	_, err := f.call("fix-ci")
	require.NoError(t, err)
	assert.Equal(t, true, f.logs.byMsg("nudge_outcome")[0].fields["session_live"])

	f.logs.lines = nil
	f.nudger.insts = nil
	_, err = f.call("fix-ci")
	require.NoError(t, err)
	assert.Equal(t, false, f.logs.byMsg("nudge_outcome")[0].fields["session_live"])
}

func TestLogUpNextFunnel_should_LogOnlyOnChange_When_SameSnapshotServedRepeatedly(t *testing.T) {
	f := newNudgeFixture(t)
	protoPRs := f.svc.userPRsToProto(f.prs.prs)

	f.svc.logUpNextFunnel(protoPRs)
	f.svc.logUpNextFunnel(protoPRs)
	require.Len(t, f.logs.byMsg("up_next_funnel"), 1)

	f.prs.prs[0].FailingChecks = nil
	f.svc.logUpNextFunnel(f.svc.userPRsToProto(f.prs.prs))
	assert.Len(t, f.logs.byMsg("up_next_funnel"), 2, "counts changed")
}

func TestLogUpNextFunnel_should_CountFailingRollupWithoutItemisedCheck_When_WebBadgeWould(t *testing.T) {
	f := newNudgeFixture(t)
	pr := linkedPR("github.com", "acme", "api", 42, "alice")
	pr.FailingChecks, pr.CheckConclusion = nil, "failure"
	f.svc.logUpNextFunnel(f.svc.userPRsToProto([]githubpkg.UserPR{pr}))
	assert.EqualValues(t, 1, f.logs.byMsg("up_next_funnel")[0].fields["prs_needing_attention"])
}

func TestUserPRsToProto_should_NotProbeSessions_When_PRIsNotNudgeableOrSessionRepeats(t *testing.T) {
	f := newNudgeFixture(t)
	healthy := linkedPR("github.com", "acme", "api", 44, "alice", githubpkg.LinkedSession{SessionID: "fix-ci"})
	healthy.FailingChecks = nil
	a := linkedPR("github.com", "acme", "api", 42, "alice", githubpkg.LinkedSession{SessionID: "fix-ci"})
	b := linkedPR("github.com", "acme", "api", 43, "alice", githubpkg.LinkedSession{SessionID: "fix-ci"})

	f.nudger.finds = 0
	f.svc.userPRsToProto([]githubpkg.UserPR{healthy, a, b})
	assert.Equal(t, 1, f.nudger.finds, "healthy PR skipped; the shared session probed once")
}
