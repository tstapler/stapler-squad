package services

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	githubpkg "github.com/tstapler/stapler-squad/github"
	"github.com/tstapler/stapler-squad/session"
)

// ---- fakes ----

type steerCall struct {
	inst *session.Instance
	sig  string
	msg  string
}

// fakeNudger records steers. With guard set it runs the real sessionNudgeGuard
// around the recorded write, like SteerInstanceGuarded does.
type fakeNudger struct {
	mu       sync.Mutex
	insts    []*session.Instance
	finds    int
	steers   []steerCall
	outcome  SteerOutcome
	err      error
	guard    *sessionNudgeGuard
	inWrite  chan struct{} // closed on first write entry when non-nil
	holdOpen chan struct{} // writes block until closed when non-nil
	onSteer  func()
	notIdle  bool // InstanceReadyForSteer reports false
}

func (f *fakeNudger) FindLiveInstance(id string) *session.Instance {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.finds++
	for _, i := range f.insts {
		if i.MatchesID(id) {
			return i
		}
	}
	return nil
}

func (f *fakeNudger) InstanceReadyForSteer(*session.Instance) bool { return !f.notIdle }

func (f *fakeNudger) SteerInstanceGuarded(_ context.Context, inst *session.Instance, sig, msg string) (SteerOutcome, error) {
	release := func(bool) {}
	if f.guard != nil {
		rel, g := f.guard.TryBegin(inst.GetStableID(), sig)
		switch g {
		case GuardBusy:
			return SteerGuardBusy, nil
		case GuardDuplicate:
			return SteerDuplicate, nil
		}
		release = rel
	}
	if f.inWrite != nil {
		select {
		case <-f.inWrite:
		default:
			close(f.inWrite)
		}
	}
	if f.holdOpen != nil {
		<-f.holdOpen
	}
	if f.onSteer != nil {
		f.onSteer()
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.outcome != SteerDelivered || f.err != nil {
		release(false)
		return f.outcome, f.err
	}
	f.steers = append(f.steers, steerCall{inst: inst, sig: sig, msg: msg})
	release(true)
	return SteerDelivered, nil
}

func (f *fakeNudger) steerCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.steers)
}

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

// fakePRs is the PR snapshot, token resolver and nudge tracker in one.
type fakePRs struct {
	prs       []githubpkg.UserPR
	tokens    map[string][2]string // PRKey.String() -> token, login
	recorded  []githubpkg.NudgeReasonKind
	firstSeen time.Time
}

func (f *fakePRs) GetAll() []githubpkg.UserPR { return f.prs }
func (f *fakePRs) TokenForPR(k githubpkg.PRKey) (string, string, bool) {
	t, ok := f.tokens[k.String()]
	return t[0], t[1], ok
}
func (f *fakePRs) RecordNudge(_ githubpkg.PRKey, r []githubpkg.NudgeReasonKind, _ time.Time) {
	f.recorded = append(f.recorded, r...)
}
func (f *fakePRs) FirstSeenAttention(githubpkg.PRKey) (time.Time, bool) {
	return f.firstSeen, !f.firstSeen.IsZero()
}

// fakeNudgeClock never sleeps: Advance moves time and fires due timeouts.
type fakeNudgeClock struct {
	mu     sync.Mutex
	now    time.Time
	timers []*fakeTimeout
}

type fakeTimeout struct {
	at     time.Time
	cancel context.CancelCauseFunc
}

func newFakeNudgeClock() *fakeNudgeClock {
	return &fakeNudgeClock{now: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
}

func (c *fakeNudgeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeNudgeClock) WithTimeout(parent context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancelCause(parent)
	c.mu.Lock()
	c.timers = append(c.timers, &fakeTimeout{at: c.now.Add(d), cancel: cancel})
	c.mu.Unlock()
	return ctx, func() { cancel(context.Canceled) }
}

func (c *fakeNudgeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	var due []*fakeTimeout
	rest := c.timers[:0:0]
	for _, t := range c.timers {
		if !t.at.After(c.now) {
			due = append(due, t)
		} else {
			rest = append(rest, t)
		}
	}
	c.timers = rest
	c.mu.Unlock()
	for _, t := range due {
		t.cancel(context.DeadlineExceeded)
	}
}

type logLine struct {
	msg    string
	fields map[string]any
}

type logCapture struct {
	mu    sync.Mutex
	lines []logLine
}

func (l *logCapture) logf(msg string, args ...any) {
	f := map[string]any{}
	for i := 0; i+1 < len(args); i += 2 {
		f[fmt.Sprint(args[i])] = args[i+1]
	}
	l.mu.Lock()
	l.lines = append(l.lines, logLine{msg, f})
	l.mu.Unlock()
}

func (l *logCapture) byMsg(msg string) []logLine {
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []logLine
	for _, ln := range l.lines {
		if ln.msg == msg {
			out = append(out, ln)
		}
	}
	return out
}

// ---- fixture ----

type nudgeFixture struct {
	svc     *GitHubUserService
	nudger  *fakeNudger
	fetcher *fakeFetcher
	prs     *fakePRs
	clock   *fakeNudgeClock
	logs    *logCapture
	inst    *session.Instance
}

func linkedPR(host, owner, repo string, number int, account string, links ...githubpkg.LinkedSession) githubpkg.UserPR {
	return githubpkg.UserPR{Host: host, Owner: owner, Repo: repo, Number: number, AccountLogin: account, LinkedSessions: links}
}

func failingLintDetail(t *testing.T, host, owner, repo string, number int) githubpkg.PRNudgeDetail {
	t.Helper()
	key, err := githubpkg.NewPRKey(host, owner, repo, number)
	require.NoError(t, err)
	return githubpkg.PRNudgeDetail{
		Key: key, State: "open",
		FailingChecks: []githubpkg.FailingCheck{{Name: "lint", URL: "https://github.com/acme/api/runs/1", Conclusion: "failure"}},
	}
}

// newNudgeFixture wires PR github.com/acme/api#42 owned by alice, linked to an
// idle claude session "fix-ci", with one failing check "lint".
func newNudgeFixture(t *testing.T) *nudgeFixture {
	t.Helper()
	inst := &session.Instance{Title: "fix-ci", UUID: "uuid-fix-ci", Program: "claude"}
	f := &nudgeFixture{
		nudger:  &fakeNudger{insts: []*session.Instance{inst}, outcome: SteerDelivered},
		fetcher: &fakeFetcher{detail: failingLintDetail(t, "github.com", "acme", "api", 42)},
		prs: &fakePRs{
			prs: []githubpkg.UserPR{linkedPR("github.com", "acme", "api", 42, "alice",
				githubpkg.LinkedSession{SessionID: "fix-ci"})},
			tokens: map[string][2]string{"github.com/acme/api#42": {"tok-alice", "alice"}},
		},
		clock: newFakeNudgeClock(),
		logs:  &logCapture{},
		inst:  inst,
	}
	f.svc = &GitHubUserService{}
	f.svc.SetPRNudger(f.nudger)
	f.svc.SetPRDetailFetcher(f.fetcher)
	f.svc.SetPRTokenResolver(f.prs)
	f.svc.nudge.snapshot = f.prs
	f.svc.nudge.tracker = f.prs
	f.svc.nudge.clock = f.clock
	f.svc.nudge.logf = f.logs.logf
	return f
}

func nudgeReq(host, owner, repo string, number int32, sessionID string) *connect.Request[sessionv1.NudgeSessionForPRRequest] {
	return connect.NewRequest(&sessionv1.NudgeSessionForPRRequest{
		Pr:        &sessionv1.PRKey{Host: host, Owner: owner, Repo: repo, Number: number},
		SessionId: sessionID,
	})
}

func (f *nudgeFixture) call(sessionID string) (*sessionv1.NudgeSessionForPRResponse, error) {
	resp, err := f.svc.NudgeSessionForPR(context.Background(), nudgeReq("", "acme", "api", 42, sessionID))
	if err != nil {
		return nil, err
	}
	return resp.Msg, nil
}

func requireCode(t *testing.T, err error, want connect.Code) {
	t.Helper()
	require.Error(t, err)
	assert.Equal(t, want, connect.CodeOf(err), err.Error())
}

// ---- tests ----

func TestNudgeSessionForPR_should_DeliverOnceToResolvedInstance_When_IdleLinkedSessionAndFailingCheck(t *testing.T) {
	f := newNudgeFixture(t)

	resp, err := f.call("fix-ci")

	require.NoError(t, err)
	assert.Equal(t, sessionv1.NudgeOutcome_NUDGE_OUTCOME_DELIVERED, resp.Outcome)
	assert.Equal(t, []sessionv1.NudgeReason{sessionv1.NudgeReason_NUDGE_REASON_FAILING_CHECKS}, resp.Reasons)
	assert.Equal(t, "uuid-fix-ci", resp.SessionId)
	require.Len(t, f.nudger.steers, 1)
	assert.Same(t, f.inst, f.nudger.steers[0].inst, "steer must target the instance resolved once")
	assert.Equal(t, 1, f.nudger.finds, "instance is looked up exactly once")
	assert.Contains(t, f.nudger.steers[0].msg, "lint")
	assert.Equal(t, []githubpkg.NudgeReasonKind{githubpkg.NudgeReasonFailingChecks}, f.prs.recorded)
}

func TestNudgeSessionForPR_should_ReturnSessionNotLinkedWithZeroSteers_When_CrossRepoOrUnlinkedSession(t *testing.T) {
	for _, sessionID := range []string{"s-web", "other"} {
		t.Run(sessionID, func(t *testing.T) {
			f := newNudgeFixture(t)
			web := &session.Instance{Title: "s-web", UUID: "uuid-web", Program: "claude"}
			other := &session.Instance{Title: "other", UUID: "uuid-other", Program: "claude"}
			f.nudger.insts = append(f.nudger.insts, web, other)
			// s-web is linked to acme/web#42 (same number, other repo), not to acme/api#42.
			f.prs.prs = append(f.prs.prs, linkedPR("github.com", "acme", "web", 42, "alice", githubpkg.LinkedSession{SessionID: "s-web"}))

			resp, err := f.call(sessionID)

			require.NoError(t, err)
			assert.Equal(t, sessionv1.NudgeOutcome_NUDGE_OUTCOME_SESSION_NOT_LINKED, resp.Outcome)
			assert.Zero(t, f.nudger.steerCount())
			assert.Zero(t, f.fetcher.callCount(), "rejected before any GitHub read")
		})
	}
}

func TestNudgeSessionForPR_should_ReturnSessionNotLinkedUsingStrictKey_When_SessionLinkedOnlyViaLegacyFallback(t *testing.T) {
	f := newNudgeFixture(t)
	f.prs.prs[0].LinkedSessions = []githubpkg.LinkedSession{{SessionID: "fix-ci", LegacyFallback: true}}

	resp, err := f.call("fix-ci")

	require.NoError(t, err)
	assert.Equal(t, sessionv1.NudgeOutcome_NUDGE_OUTCOME_SESSION_NOT_LINKED, resp.Outcome)
	assert.Zero(t, f.nudger.steerCount())
}

func TestNudgeSessionForPR_should_UseOwningAccountTokenAndLogHostWithoutToken_When_GHEPROwnedByBob(t *testing.T) {
	f := newNudgeFixture(t)
	f.prs.prs = []githubpkg.UserPR{linkedPR("ghe.corp", "acme", "api", 7, "bob", githubpkg.LinkedSession{SessionID: "fix-ci"})}
	f.prs.tokens = map[string][2]string{"ghe.corp/acme/api#7": {"tok-bob-secret", "bob"}}
	f.fetcher.detail = failingLintDetail(t, "ghe.corp", "acme", "api", 7)

	resp, err := f.svc.NudgeSessionForPR(context.Background(), nudgeReq("ghe.corp", "acme", "api", 7, "fix-ci"))

	require.NoError(t, err)
	assert.Equal(t, sessionv1.NudgeOutcome_NUDGE_OUTCOME_DELIVERED, resp.Msg.Outcome)
	require.Len(t, f.fetcher.calls, 1)
	assert.Equal(t, "tok-bob-secret", f.fetcher.calls[0].token)
	assert.Equal(t, "ghe.corp", f.fetcher.calls[0].key.Host())
	exit := f.logs.byMsg("nudge_outcome")
	require.Len(t, exit, 1)
	assert.Equal(t, "ghe.corp", exit[0].fields["host"])
	assert.Equal(t, "bob", exit[0].fields["account"])
	for _, ln := range f.logs.lines {
		assert.NotContains(t, fmt.Sprint(ln.fields), "tok-bob-secret", "tokens are never logged")
	}
}

func TestNudgeSessionForPR_should_ReturnPRNotFoundWithoutTryingOtherAccount_When_OwningAccountCannotSeePR(t *testing.T) {
	f := newNudgeFixture(t)
	f.fetcher.err = fmt.Errorf("%w: not visible", githubpkg.ErrGitHubRefNotFound)
	f.prs.tokens["github.com/acme/api#42"] = [2]string{"tok-alice", "alice"}

	resp, err := f.call("fix-ci")

	require.NoError(t, err)
	assert.Equal(t, sessionv1.NudgeOutcome_NUDGE_OUTCOME_PR_NOT_FOUND, resp.Outcome)
	require.Equal(t, 1, f.fetcher.callCount())
	assert.Equal(t, "tok-alice", f.fetcher.calls[0].token)
	assert.Zero(t, f.nudger.steerCount())
}

func TestNudgeSessionForPR_should_ReturnPausedWithDistinctCopyForPausedVsNotTracked_When_StatusPausedOrFindLiveInstanceNil(t *testing.T) {
	f := newNudgeFixture(t)
	f.inst.Status = session.Paused
	paused, err := f.call("fix-ci")
	require.NoError(t, err)
	assert.Equal(t, sessionv1.NudgeOutcome_NUDGE_OUTCOME_PAUSED, paused.Outcome)
	assert.Equal(t, "Session paused. Open it to resume", paused.Detail)

	f.nudger.insts = nil
	untracked, err := f.call("fix-ci")
	require.NoError(t, err)
	assert.Equal(t, sessionv1.NudgeOutcome_NUDGE_OUTCOME_PAUSED, untracked.Outcome)
	assert.Equal(t, "Session is not running or not tracked. Open its page to restart it.", untracked.Detail)
	assert.NotEqual(t, paused.Detail, untracked.Detail)
	assert.Zero(t, f.nudger.steerCount())
	assert.Zero(t, f.fetcher.callCount())
}

func TestNudgeSessionForPR_should_MapBusyPausedUntrackedAndNoControllerToDistinctDetail_When_GateFalse(t *testing.T) {
	tests := []struct {
		name    string
		outcome SteerOutcome
		want    sessionv1.NudgeOutcome
		detail  string
	}{
		{"queued command or non-idle", SteerBusy, sessionv1.NudgeOutcome_NUDGE_OUTCOME_BUSY, "Session is busy. Try again when it is idle."},
		{"another delivery in flight", SteerGuardBusy, sessionv1.NudgeOutcome_NUDGE_OUTCOME_BUSY, "Session is busy. Try again when it is idle."},
		{"no controller", SteerNoStatusSource, sessionv1.NudgeOutcome_NUDGE_OUTCOME_BUSY, "Session isn't being monitored, so it can't safely take a request. Open it to restart it."},
		{"not tracked at steer time", SteerNotTracked, sessionv1.NudgeOutcome_NUDGE_OUTCOME_PAUSED, "Session is not running or not tracked. Open its page to restart it."},
		{"duplicate", SteerDuplicate, sessionv1.NudgeOutcome_NUDGE_OUTCOME_DUPLICATE, "Already requested recently."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newNudgeFixture(t)
			f.nudger.outcome = tt.outcome

			resp, err := f.call("fix-ci")

			require.NoError(t, err)
			assert.Equal(t, tt.want, resp.Outcome)
			assert.Equal(t, tt.detail, resp.Detail)
			assert.Zero(t, f.nudger.steerCount())
			assert.Empty(t, f.prs.recorded, "only a delivery is recorded for follow-up")
		})
	}
}

// Real SessionService as PRNudger: the handler reports exactly what the shared
// readiness gate (the one auto-steer uses) decides.
func TestNudgeSessionForPR_should_ReturnDistinctNoControllerBusyDetailAndNotGenericBusyCopy_When_NoStatusSource(t *testing.T) {
	fix := setupForkTestFixture(t)
	inst := &session.Instance{Title: "fix-ci", UUID: "uuid-fix-ci", Program: "claude"}
	addInstanceToPoller(fix.poller, inst)
	writes := 0
	fix.svc.guardedSteer.write = func(context.Context, *session.Instance, string) error { writes++; return nil }
	fix.svc.guardedSteer.verifyPane = func(context.Context, *session.Instance) error { return nil }
	fix.svc.guardedSteer.guard.now = newFakeNudgeClock().Now

	f := newNudgeFixture(t)
	f.svc.SetPRNudger(fix.svc)

	gates := []struct {
		ready  notReadyReason
		detail string
	}{
		{notReadyNoStatusSource, nudgeNoControllerDetail},
		{notReadyBusy, nudgeBusyDetail},
	}
	details := make([]string, 0, len(gates))
	for _, g := range gates {
		g := g
		fix.svc.guardedSteer.ready = func(*session.Instance) notReadyReason { return g.ready }
		resp, err := f.call("fix-ci")
		require.NoError(t, err)
		assert.Equal(t, sessionv1.NudgeOutcome_NUDGE_OUTCOME_BUSY, resp.Outcome)
		assert.Equal(t, g.detail, resp.Detail)
		details = append(details, resp.Detail)
	}
	assert.NotEqual(t, details[0], details[1], "the no-controller copy must differ from generic busy")
	assert.Zero(t, writes)
}

func TestNudgeSessionForPR_should_ReturnNothingToFixWithZeroSteers_When_FreshDetailGreenOrMergedClosedDraft(t *testing.T) {
	green := failingLintDetail(t, "github.com", "acme", "api", 42)
	green.FailingChecks = nil
	merged := failingLintDetail(t, "github.com", "acme", "api", 42)
	merged.State = "merged"
	closed := failingLintDetail(t, "github.com", "acme", "api", 42)
	closed.State = "closed"
	draft := failingLintDetail(t, "github.com", "acme", "api", 42)
	draft.IsDraft = true

	for name, d := range map[string]githubpkg.PRNudgeDetail{"green": green, "merged": merged, "closed": closed, "draft": draft} {
		t.Run(name, func(t *testing.T) {
			f := newNudgeFixture(t)
			f.fetcher.detail = d
			resp, err := f.call("fix-ci")
			require.NoError(t, err)
			assert.Equal(t, sessionv1.NudgeOutcome_NUDGE_OUTCOME_NOTHING_TO_FIX, resp.Outcome)
			assert.Zero(t, f.nudger.steerCount())
		})
	}
}

func TestNudgeSessionForPR_should_ReturnNothingToFixWithZeroSteers_When_OnlyChangesRequested(t *testing.T) {
	// Changes-requested is not a nudge reason: fresh detail has no check, thread or conflict.
	f := newNudgeFixture(t)
	f.fetcher.detail = githubpkg.PRNudgeDetail{Key: f.fetcher.detail.Key, State: "open"}

	resp, err := f.call("fix-ci")

	require.NoError(t, err)
	assert.Equal(t, sessionv1.NudgeOutcome_NUDGE_OUTCOME_NOTHING_TO_FIX, resp.Outcome)
	assert.Zero(t, f.nudger.steerCount())
}

func TestNudgeSessionForPR_should_ReturnDuplicate20sAfterDelivery_And_OneDeliveredOneBusyOnSimultaneousCalls(t *testing.T) {
	f := newNudgeFixture(t)
	guardClock := newFakeNudgeClock()
	f.nudger.guard = &sessionNudgeGuard{now: guardClock.Now}

	first, err := f.call("fix-ci")
	require.NoError(t, err)
	assert.Equal(t, sessionv1.NudgeOutcome_NUDGE_OUTCOME_DELIVERED, first.Outcome)
	guardClock.Advance(20 * time.Second)
	again, err := f.call("fix-ci")
	require.NoError(t, err)
	assert.Equal(t, sessionv1.NudgeOutcome_NUDGE_OUTCOME_DUPLICATE, again.Outcome)
	assert.Equal(t, 1, f.nudger.steerCount())

	// Double-click race: the first call is held inside the write; the second arrives meanwhile.
	g := newNudgeFixture(t)
	g.nudger.guard = &sessionNudgeGuard{now: newFakeNudgeClock().Now}
	g.nudger.inWrite = make(chan struct{})
	g.nudger.holdOpen = make(chan struct{})
	done := make(chan *sessionv1.NudgeSessionForPRResponse, 1)
	go func() {
		resp, _ := g.call("fix-ci")
		done <- resp
	}()
	<-g.nudger.inWrite
	second, err := g.call("fix-ci")
	require.NoError(t, err)
	assert.Equal(t, sessionv1.NudgeOutcome_NUDGE_OUTCOME_BUSY, second.Outcome)
	close(g.nudger.holdOpen)
	winner := <-done
	assert.Equal(t, sessionv1.NudgeOutcome_NUDGE_OUTCOME_DELIVERED, winner.Outcome)
	assert.Equal(t, 1, g.nudger.steerCount(), "exactly one steer for two simultaneous calls")
}

func TestNudgeSessionForPR_should_WriteExactlyOnceThroughRealGuard_When_RealSessionNudgeGuardAndFakePRNudger(t *testing.T) {
	f := newNudgeFixture(t)
	f.nudger.guard = &sessionNudgeGuard{now: newFakeNudgeClock().Now}

	for i := 0; i < 3; i++ {
		_, err := f.call("fix-ci")
		require.NoError(t, err)
	}

	assert.Equal(t, 1, f.nudger.steerCount())
	assert.Contains(t, f.nudger.steers[0].sig, "FAILING_CHECKS")
	assert.Contains(t, f.nudger.steers[0].sig, "github.com/acme/api#42")
}

func TestNudgeSessionForPR_should_ReturnFailedPreconditionWithZeroWrites_When_PaneOwnershipMismatch(t *testing.T) {
	fix := setupForkTestFixture(t)
	inst := &session.Instance{Title: "fix-ci", UUID: "uuid-fix-ci", Program: "claude"}
	addInstanceToPoller(fix.poller, inst)
	writes := 0
	verifyErr := errors.New("pane ownership mismatch")
	fix.svc.guardedSteer.ready = func(*session.Instance) notReadyReason { return notReadyNone }
	fix.svc.guardedSteer.verifyPane = func(context.Context, *session.Instance) error { return verifyErr }
	fix.svc.guardedSteer.write = func(context.Context, *session.Instance, string) error { writes++; return nil }
	clk := newFakeNudgeClock()
	fix.svc.guardedSteer.guard.now = clk.Now
	f := newNudgeFixture(t)
	f.svc.SetPRNudger(fix.svc)

	_, err := f.call("fix-ci")
	requireCode(t, err, connect.CodeFailedPrecondition)
	assert.Zero(t, writes)

	// Identity fixed; the failure cooldown is 10s, so a retry after it delivers once.
	verifyErr = nil
	clk.Advance(11 * time.Second)
	resp, err := f.call("fix-ci")
	require.NoError(t, err)
	assert.Equal(t, sessionv1.NudgeOutcome_NUDGE_OUTCOME_DELIVERED, resp.Outcome)
	assert.Equal(t, 1, writes)
}

func TestNudgeSessionForPR_should_ReturnFailedPrecondition_When_ProgramUnsupported(t *testing.T) {
	f := newNudgeFixture(t)
	f.inst.Program = "aider --model x"

	_, err := f.call("fix-ci")

	requireCode(t, err, connect.CodeFailedPrecondition)
	assert.Zero(t, f.nudger.steerCount())
	assert.Zero(t, f.fetcher.callCount())
}

func TestNudgeSessionForPR_should_ReturnInternalWithoutDelivered_When_WriteFails(t *testing.T) {
	f := newNudgeFixture(t)
	f.nudger.outcome = SteerFailed
	f.nudger.err = errors.New("tmux write failed")

	_, err := f.call("fix-ci")

	requireCode(t, err, connect.CodeInternal)
	assert.NotContains(t, err.Error(), "tmux write failed", "internal errors are not echoed to the client")
	logged := f.logs.byMsg("nudge_steer_failed")
	require.Len(t, logged, 1)
	assert.Equal(t, "tmux write failed", logged[0].fields["err"], "the underlying error is logged server-side")
	assert.Empty(t, f.prs.recorded)
}

func TestNudgeSessionForPR_should_ReturnInternalAndLogErr_When_OutcomeUnspecified(t *testing.T) {
	f := newNudgeFixture(t)
	f.nudger.outcome = SteerUnspecified

	_, err := f.call("fix-ci")

	requireCode(t, err, connect.CodeInternal)
	assert.Empty(t, f.prs.recorded, "an unspecified outcome is not a delivery")
	require.Len(t, f.logs.byMsg("nudge_steer_unexpected"), 1)
}

func TestNudgeSessionForPR_should_ReturnPRNotFoundOrResourceExhausted_When_FetcherNotFoundOrErrRateLimited(t *testing.T) {
	f := newNudgeFixture(t)
	f.fetcher.err = githubpkg.ErrGitHubRefNotFound
	resp, err := f.call("fix-ci")
	require.NoError(t, err)
	assert.Equal(t, sessionv1.NudgeOutcome_NUDGE_OUTCOME_PR_NOT_FOUND, resp.Outcome)

	resume := time.Date(2026, 1, 1, 13, 45, 0, 0, time.UTC)
	f.svc.nudge.rateLimitResume = func() (bool, time.Time) { return true, resume }
	f.fetcher.err = fmt.Errorf("wrapped: %w", githubpkg.ErrRateLimited)
	_, err = f.call("fix-ci")
	requireCode(t, err, connect.CodeResourceExhausted)
	assert.Contains(t, err.Error(), "2026-01-01T13:45:00Z")
	assert.Zero(t, f.nudger.steerCount())
}

func TestNudgeSessionForPR_should_ReturnPRNotFound_When_OwningAccountNoLongerConnectedOrPRNotPolled(t *testing.T) {
	f := newNudgeFixture(t)
	f.prs.tokens = nil
	resp, err := f.call("fix-ci")
	require.NoError(t, err)
	assert.Equal(t, sessionv1.NudgeOutcome_NUDGE_OUTCOME_PR_NOT_FOUND, resp.Outcome)
	assert.Zero(t, f.fetcher.callCount(), "no fallback to another account")

	f.prs.prs = nil
	resp, err = f.call("fix-ci")
	require.NoError(t, err)
	assert.Equal(t, sessionv1.NudgeOutcome_NUDGE_OUTCOME_PR_NOT_FOUND, resp.Outcome)
}

func TestNudgeSessionForPR_should_ReturnDeadlineErrorWithZeroSteersAndNoRealSleep_When_FakeClockPassesFetchTimeout(t *testing.T) {
	f := newNudgeFixture(t)
	f.fetcher.block = true
	f.fetcher.started = make(chan struct{})
	errCh := make(chan error, 1)
	go func() {
		_, err := f.call("fix-ci")
		errCh <- err
	}()

	<-f.fetcher.started
	f.clock.Advance(nudgeFetchTimeout)

	requireCode(t, <-errCh, connect.CodeDeadlineExceeded)
	assert.Zero(t, f.nudger.steerCount())

	// Instant fakes never advance the clock.
	g := newNudgeFixture(t)
	before := g.clock.Now()
	_, err := g.call("fix-ci")
	require.NoError(t, err)
	assert.Equal(t, before, g.clock.Now())
}

func TestNudgeSessionForPR_should_LogOneNudgeOutcomeLineWithFieldsAndNoPromptText_When_AnyOutcome(t *testing.T) {
	f := newNudgeFixture(t)
	f.prs.firstSeen = f.clock.Now().Add(-90 * time.Second)

	_, err := f.call("fix-ci")
	require.NoError(t, err)

	exit := f.logs.byMsg("nudge_outcome")
	require.Len(t, exit, 1)
	fields := exit[0].fields
	assert.Equal(t, "DELIVERED", fields["outcome"])
	assert.Equal(t, "FAILING_CHECKS", fields["reasons"])
	assert.Equal(t, "uuid-fix-ci", fields["session_id"])
	assert.Equal(t, "github.com/acme/api#42", fields["pr"])
	assert.EqualValues(t, 0, fields["latency_ms"])
	assert.EqualValues(t, int64(90), fields["attention_age_s"])
	assert.Greater(t, fields["prompt_bytes"], 0)
	body := f.nudger.steers[0].msg
	for _, ln := range f.logs.lines {
		assert.NotContains(t, fmt.Sprint(ln.fields), "lint", "check text must not be logged")
		assert.NotContains(t, fmt.Sprint(ln.fields), body)
	}

	// An error exit also logs exactly one line.
	f.logs.lines = nil
	f.fetcher.err = githubpkg.ErrRateLimited
	_, err = f.call("fix-ci")
	require.Error(t, err)
	exit = f.logs.byMsg("nudge_outcome")
	require.Len(t, exit, 1)
	assert.Equal(t, "ERROR", exit[0].fields["outcome"])
	assert.Equal(t, "resource_exhausted", exit[0].fields["code"])
	_, hasAge := exit[0].fields["attention_age_s"]
	assert.True(t, hasAge)
}

func TestParsePRKey_should_DefaultHostAndRejectInvalid_When_EmptyHostOrBadOwnerRepoNumber(t *testing.T) {
	key, err := parsePRKey(&sessionv1.PRKey{Host: "", Owner: "acme", Repo: "api", Number: 42})
	require.NoError(t, err)
	assert.Equal(t, "github.com", key.Host())

	bad := []*sessionv1.PRKey{
		nil,
		{Owner: "", Repo: "api", Number: 42},
		{Owner: "acme", Repo: "", Number: 42},
		{Owner: "acme", Repo: "api", Number: 0},
		{Owner: "acme", Repo: "api", Number: -3},
	}
	for i, k := range bad {
		_, err := parsePRKey(k)
		requireCode(t, err, connect.CodeInvalidArgument)
		_ = i
	}

	f := newNudgeFixture(t)
	_, err = f.svc.NudgeSessionForPR(context.Background(), nudgeReq("", "acme", "api", 0, "fix-ci"))
	requireCode(t, err, connect.CodeInvalidArgument)
	assert.Equal(t, 1, len(f.logs.byMsg("nudge_outcome")))
}

func TestAttentionAge_should_ReturnWholeSecondsOrAbsent_When_FirstSeenKnownOrNot(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name      string
		firstSeen time.Time
		want      int64
		ok        bool
	}{
		{"never seen", time.Time{}, 0, false},
		{"seen 40 minutes ago", now.Add(-40 * time.Minute), 2400, true},
		{"seen just now", now, 0, true},
		{"clock skew (future)", now.Add(time.Second), 0, false},
		{"sub-second truncates", now.Add(-1500 * time.Millisecond), 1, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := attentionAge(now, tt.firstSeen)
			assert.Equal(t, tt.ok, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCanSteer_should_AcceptOnlyAgentsWithAStatusSource_When_ProgramGiven(t *testing.T) {
	for program, want := range map[string]bool{
		"": true, "claude": true, "claude --resume": true, "/usr/local/bin/claude": true, "env -u X claude": true,
		"pi": true, "/usr/bin/pi": true,
		"aider": false, "pipenv": false, "claude-squad": false, "bash": false,
	} {
		assert.Equal(t, want, CanSteer(program), strings.TrimSpace(program))
	}
}

func TestNudgeSessionForPR_should_ReturnUnavailable_When_PortsNotWired(t *testing.T) {
	svc := &GitHubUserService{}
	_, err := svc.NudgeSessionForPR(context.Background(), nudgeReq("", "acme", "api", 42, "fix-ci"))
	requireCode(t, err, connect.CodeUnavailable)
}
