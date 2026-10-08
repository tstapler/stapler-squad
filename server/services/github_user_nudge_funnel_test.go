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
