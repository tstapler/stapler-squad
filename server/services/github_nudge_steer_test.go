package services

// Up Next's nudge against the real SessionService steer gates: the handler lives
// in githubrpc, the gates stay here.

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	githubpkg "github.com/tstapler/stapler-squad/github"
	"github.com/tstapler/stapler-squad/server/services/githubrpc"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/server/services/prs"
	"github.com/tstapler/stapler-squad/session/instance"
)

var hubNudgeEpoch = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

type fetchCall struct {
	key   githubpkg.PRKey
	token string
}

type fakeFetcher struct {
	mu      sync.Mutex
	detail  githubpkg.PRNudgeDetail
	err     error
	calls   []fetchCall
	started chan struct{} // closed on first entry when non-nil
	block   bool          // wait for ctx cancellation, return its cause
}

func (f *fakeFetcher) FetchPRNudgeDetail(ctx context.Context, key githubpkg.PRKey, token string) (githubpkg.PRNudgeDetail, error) {
	f.mu.Lock()
	f.calls = append(f.calls, fetchCall{key, token})
	f.mu.Unlock()
	if f.started != nil {
		close(f.started)
	}
	if f.block {
		<-ctx.Done()
		return githubpkg.PRNudgeDetail{}, context.Cause(ctx)
	}
	return f.detail, f.err
}

func (f *fakeFetcher) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

type fixedUserPRs []githubpkg.UserPR

func (f fixedUserPRs) GetAll() []githubpkg.UserPR { return f }

type hubNudgeFixture struct{ svc *githubrpc.GitHubUserService }

// newHubNudgeFixture wires acme/api#42, linked to session "fix-ci" with one
// failing check, to nudger.
func newHubNudgeFixture(t *testing.T, nudger githubrpc.PRNudger) *hubNudgeFixture {
	t.Helper()
	key, err := githubpkg.NewPRKey("", "acme", "api", 42)
	require.NoError(t, err)
	failing := []githubpkg.FailingCheck{{Name: "lint", URL: "https://github.com/acme/api/runs/1", Conclusion: "failure"}}
	svc := githubrpc.NewGitHubUserService(nil, nil)
	svc.SetPRNudger(nudger)
	svc.SetPRDetailFetcher(&fakeFetcher{detail: githubpkg.PRNudgeDetail{Key: key, State: "open", FailingChecks: failing}})
	svc.SetPRTokenResolver(staticPRTokens{key.String(): "tok-alice"})
	svc.SetPRSnapshotSource(fixedUserPRs{{Host: "github.com", Owner: "acme", Repo: "api", Number: 42, FailingChecks: failing,
		LinkedSessions: []githubpkg.LinkedSession{{SessionID: "fix-ci"}}}})
	return &hubNudgeFixture{svc: svc}
}

func (f *hubNudgeFixture) call(sessionID string) (*sessionv1.NudgeSessionForPRResponse, error) {
	resp, err := f.svc.NudgeSessionForPR(context.Background(), connect.NewRequest(&sessionv1.NudgeSessionForPRRequest{
		Pr: &sessionv1.PRKey{Owner: "acme", Repo: "api", Number: 42}, SessionId: sessionID}))
	if err != nil {
		return nil, err
	}
	return resp.Msg, nil
}

// Real SessionService as PRNudger: the handler reports exactly what the shared
// readiness gate (the one auto-steer uses) decides.
func TestNudgeSessionForPR_should_ReturnDistinctNoControllerBusyDetailAndNotGenericBusyCopy_When_NoStatusSource(t *testing.T) {
	fix := setupForkTestFixture(t)
	inst := &instance.Instance{Title: "fix-ci", UUID: "uuid-fix-ci", Program: "claude"}
	svctest.AddInstanceToPoller(fix.poller, inst)
	writes := 0
	fix.svc.guardedSteer.write = func(context.Context, *instance.Instance, string) error { writes++; return nil }
	fix.svc.guardedSteer.verifyPane = func(context.Context, *instance.Instance) error { return nil }
	fix.svc.guardedSteer.guard.SetClock((&svctest.FakeGuardClock{T: hubNudgeEpoch}).Now)

	f := newHubNudgeFixture(t, fix.svc)

	gates := []struct {
		ready  notReadyReason
		detail string
	}{
		{notReadyNoStatusSource, prs.NudgeNoControllerDetail},
		{notReadyBusy, prs.NudgeBusyDetail},
	}
	details := make([]string, 0, len(gates))
	for _, g := range gates {
		g := g
		fix.svc.guardedSteer.ready = func(*instance.Instance) notReadyReason { return g.ready }
		resp, err := f.call("fix-ci")
		require.NoError(t, err)
		assert.Equal(t, sessionv1.NudgeOutcome_NUDGE_OUTCOME_BUSY, resp.Outcome)
		assert.Equal(t, g.detail, resp.Detail)
		details = append(details, resp.Detail)
	}
	assert.NotEqual(t, details[0], details[1], "the no-controller copy must differ from generic busy")
	assert.Zero(t, writes)
}

func TestNudgeSessionForPR_should_ReturnFailedPreconditionWithZeroWrites_When_PaneOwnershipMismatch(t *testing.T) {
	fix := setupForkTestFixture(t)
	inst := &instance.Instance{Title: "fix-ci", UUID: "uuid-fix-ci", Program: "claude"}
	svctest.AddInstanceToPoller(fix.poller, inst)
	writes := 0
	verifyErr := errors.New("pane ownership mismatch")
	fix.svc.guardedSteer.ready = func(*instance.Instance) notReadyReason { return notReadyNone }
	fix.svc.guardedSteer.verifyPane = func(context.Context, *instance.Instance) error { return verifyErr }
	fix.svc.guardedSteer.write = func(context.Context, *instance.Instance, string) error { writes++; return nil }
	clk := &svctest.FakeGuardClock{T: hubNudgeEpoch}
	fix.svc.guardedSteer.guard.SetClock(clk.Now)
	f := newHubNudgeFixture(t, fix.svc)

	_, err := f.call("fix-ci")
	require.Error(t, err)
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err), err.Error())
	assert.Zero(t, writes)

	// Identity fixed; the failure cooldown is 10s, so a retry after it delivers once.
	verifyErr = nil
	clk.Advance(11 * time.Second)
	resp, err := f.call("fix-ci")
	require.NoError(t, err)
	assert.Equal(t, sessionv1.NudgeOutcome_NUDGE_OUTCOME_DELIVERED, resp.Outcome)
	assert.Equal(t, 1, writes)
}
