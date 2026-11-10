package services

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/github"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/session"
)

type staticPRTokens map[string]string

func (m staticPRTokens) TokenForPR(k github.PRKey) (string, string, bool) {
	tok, ok := m[k.String()]
	return tok, "alice", ok
}

type askFixFixture struct {
	svc     *SessionService
	inst    *session.Instance
	fetcher *fakeFetcher
	rec     *guardedSteerRecorder
}

// newAskFixFixture wires a running claude session holding acme/app#11 and #12,
// a fetcher reporting one failing check, and a token for #12 only.
func newAskFixFixture(t *testing.T, status session.Status) *askFixFixture {
	t.Helper()
	fix := setupForkTestFixture(t)
	inst := &session.Instance{Title: "ask-fix", UUID: "uuid-ask-fix", Program: "claude", Status: status,
		CreatedAt: time.Now(), UpdatedAt: time.Now()}
	for _, n := range []int{11, 12} {
		ref, err := session.NewPRRef("", "acme", "app", n)
		require.NoError(t, err)
		require.NoError(t, inst.AttachPR(ref, session.AttachSourceManual, session.PRInitial{State: session.PRStateOpen}))
	}
	svctest.AddInstanceToPoller(fix.poller, inst)
	rec := &guardedSteerRecorder{}
	rec.install(fix.svc, notReadyNone, &svctest.FakeGuardClock{T: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)})
	key, err := github.NewPRKey("", "acme", "app", 12)
	require.NoError(t, err)
	f := &fakeFetcher{detail: github.PRNudgeDetail{Key: key, State: "open",
		FailingChecks: []github.FailingCheck{{Name: "lint", URL: "https://github.com/acme/app/runs/1", Conclusion: "failure"}}}}
	fix.svc.SetPRAskToFix(f, staticPRTokens{key.String(): "tok"})
	return &askFixFixture{svc: fix.svc, inst: inst, fetcher: f, rec: rec}
}

func (f *askFixFixture) ask(pr *sessionv1.PullRequestRef) (*sessionv1.AskSessionToFixPullRequestResponse, error) {
	resp, err := f.svc.AskSessionToFixPullRequest(context.Background(), connect.NewRequest(
		&sessionv1.AskSessionToFixPullRequestRequest{Id: "uuid-ask-fix", Pr: pr}))
	if err != nil {
		return nil, err
	}
	return resp.Msg, nil
}

func TestAskSessionToFixPullRequest_ShouldDeliverOnceToTargetPR_WhenIdleAndFailingCheck(t *testing.T) {
	t.Parallel()
	f := newAskFixFixture(t, session.Active)

	got, err := f.ask(protoRef(12))
	require.NoError(t, err)
	assert.Equal(t, sessionv1.NudgeOutcome_NUDGE_OUTCOME_DELIVERED, got.Outcome)
	assert.Equal(t, []sessionv1.NudgeReason{sessionv1.NudgeReason_NUDGE_REASON_FAILING_CHECKS}, got.Reasons)
	assert.EqualValues(t, 12, got.Pr.Number)
	assert.Equal(t, 1, f.rec.writes)
	assert.Equal(t, 1, f.fetcher.callCount())

	again, err := f.ask(protoRef(12))
	require.NoError(t, err)
	assert.Equal(t, sessionv1.NudgeOutcome_NUDGE_OUTCOME_DUPLICATE, again.Outcome, "a repeat within the window is reported, not re-sent")
	assert.Equal(t, 1, f.rec.writes)
}

func TestAskSessionToFixPullRequest_ShouldFailPrecondition_WhenPRNotAttached(t *testing.T) {
	t.Parallel()
	f := newAskFixFixture(t, session.Active)

	_, err := f.ask(protoRef(99))
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
	assert.Equal(t, 0, f.fetcher.callCount())
	assert.Equal(t, 0, f.rec.writes)
}

func TestAskSessionToFixPullRequest_ShouldReportPRNotFound_WhenNoAccountTokenForPR(t *testing.T) {
	t.Parallel()
	f := newAskFixFixture(t, session.Active)

	got, err := f.ask(protoRef(11)) // only #12 has a token
	require.NoError(t, err)
	assert.Equal(t, sessionv1.NudgeOutcome_NUDGE_OUTCOME_PR_NOT_FOUND, got.Outcome)
	assert.Equal(t, nudgeNoAccountDetail, got.Detail)
	assert.Equal(t, 0, f.fetcher.callCount())
	assert.Equal(t, 0, f.rec.writes)
}

func TestAskSessionToFixPullRequest_ShouldReportNothingToFix_WhenPRClean(t *testing.T) {
	t.Parallel()
	f := newAskFixFixture(t, session.Active)
	f.fetcher.detail.FailingChecks = nil

	got, err := f.ask(protoRef(12))
	require.NoError(t, err)
	assert.Equal(t, sessionv1.NudgeOutcome_NUDGE_OUTCOME_NOTHING_TO_FIX, got.Outcome)
	assert.Equal(t, 0, f.rec.writes)
}

func TestAskSessionToFixPullRequest_ShouldReportPausedWithoutFetching_WhenSessionSuspended(t *testing.T) {
	t.Parallel()
	f := newAskFixFixture(t, session.Paused)

	got, err := f.ask(protoRef(12))
	require.NoError(t, err)
	assert.Equal(t, sessionv1.NudgeOutcome_NUDGE_OUTCOME_PAUSED, got.Outcome)
	assert.Equal(t, 0, f.fetcher.callCount())
	assert.Equal(t, 0, f.rec.writes)
}

func TestAskSessionToFixPullRequest_ShouldTargetActivePR_WhenSelectorUnset(t *testing.T) {
	t.Parallel()
	f := newAskFixFixture(t, session.Active)

	got, err := f.ask(nil)
	require.NoError(t, err)
	assert.EqualValues(t, 12, got.Pr.Number, "the newest open PR is the active one")
	assert.Equal(t, sessionv1.NudgeOutcome_NUDGE_OUTCOME_DELIVERED, got.Outcome)
}

func TestAttachSessionPullRequest_ShouldReportAlreadyTrackedWithoutLookup_WhenPRAlreadyAttached(t *testing.T) {
	t.Parallel()
	fix := setupForkTestFixture(t)
	addLivePRSession(t, fix, "u1", "uuid-u1", prSpec{11, session.PRStateOpen, false})
	lookups := 0
	fix.svc.prRefs.lookup = func(_ context.Context, _ github.RepoRef, n int) (*github.PRInfo, error) {
		lookups++
		return &github.PRInfo{Number: n, State: "OPEN", Title: "Add thing", HTMLURL: "https://github.com/acme/app/pull/30"}, nil
	}
	attach := func(num string) *sessionv1.AttachSessionPullRequestResponse {
		resp, err := fix.svc.AttachSessionPullRequest(context.Background(), connect.NewRequest(&sessionv1.AttachSessionPullRequestRequest{
			Id: "uuid-u1", Url: "https://github.com/acme/app/pull/" + num}))
		require.NoError(t, err)
		return resp.Msg
	}

	dup := attach("11")
	assert.Equal(t, sessionv1.AttachPullRequestResult_ATTACH_PULL_REQUEST_RESULT_ALREADY_TRACKED, dup.Result)
	assert.EqualValues(t, 11, dup.Pr.Number)
	assert.Equal(t, 0, lookups, "an already-tracked PR costs no GitHub call")

	added := attach("30")
	assert.Equal(t, sessionv1.AttachPullRequestResult_ATTACH_PULL_REQUEST_RESULT_ATTACHED, added.Result)
	assert.EqualValues(t, 30, added.Pr.Number)
	assert.Equal(t, 1, lookups)
	for _, p := range added.PullRequests {
		if p.Number == 30 {
			assert.Equal(t, "Add thing", p.Title, "the lookup's title is carried onto the new link")
		}
	}
}
