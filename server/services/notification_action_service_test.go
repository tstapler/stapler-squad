package services

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
)

const (
	outApplied  = sessionv1.ActionOutcome_ACTION_OUTCOME_APPLIED
	outResolved = sessionv1.ActionOutcome_ACTION_OUTCOME_ALREADY_RESOLVED
	outInvalid  = sessionv1.ActionOutcome_ACTION_OUTCOME_INVALID
	outFlagOff  = sessionv1.ActionOutcome_ACTION_OUTCOME_FLAG_OFF
	outForbid   = sessionv1.ActionOutcome_ACTION_OUTCOME_FORBIDDEN
	outStale    = sessionv1.ActionOutcome_ACTION_OUTCOME_STALE
)

type fakeActionApprovals struct {
	calls  atomic.Int32
	last   ApprovalChoice
	result ActionResult
}

func (f *fakeActionApprovals) ResolveApproval(_ context.Context, c ApprovalChoice) (ActionResult, error) {
	f.calls.Add(1)
	f.last = c
	if f.result.Outcome == 0 {
		return applied(), nil
	}
	return f.result, nil
}

type fakeQuestionReplier struct {
	route  answerRoute
	known  bool
	writes atomic.Int32
	result ActionResult
}

func (f *fakeQuestionReplier) RouteFor(string) (answerRoute, bool) { return f.route, f.known }
func (f *fakeQuestionReplier) ReplyToQuestion(context.Context, QuestionAnswer) (ActionResult, error) {
	f.writes.Add(1)
	if f.result.Outcome == 0 {
		return applied(), nil
	}
	return f.result, nil
}

type fakeGuidanceAnswerer struct{ calls atomic.Int32 }

func (f *fakeGuidanceAnswerer) AnswerGuidance(context.Context, string, string) (ActionResult, error) {
	f.calls.Add(1)
	return applied(), nil
}

type fakeBacklogDecider struct {
	calls  atomic.Int32
	result ActionResult
}

func (f *fakeBacklogDecider) Decide(context.Context, BacklogDecision) (ActionResult, error) {
	f.calls.Add(1)
	if f.result.Outcome == 0 {
		return applied(), nil
	}
	return f.result, nil
}

type fakeResolver struct {
	mu        sync.Mutex
	items     []string
	questions []string
	approvals []string
}

func (f *fakeResolver) ResolveQuestionNotifications(id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.questions = append(f.questions, id)
}

func (f *fakeResolver) ResolveApprovalNotifications(id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.approvals = append(f.approvals, id)
}

func (f *fakeResolver) ResolveItemNotifications(id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.items = append(f.items, id)
}

// fakeActionFlags resolves every point true unless listed in off.
type fakeActionFlags struct {
	off       map[string]bool
	answerOff bool
}

func (f fakeActionFlags) Inline(dp string) bool { return !f.off[dp] }
func (f fakeActionFlags) Answer(answerRoute) bool {
	return !f.answerOff
}

type recordingMetrics struct {
	mu      sync.Mutex
	samples []ActionSample
}

func (m *recordingMetrics) Record(s ActionSample) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.samples = append(m.samples, s)
}

type recordingAudit struct {
	mu    sync.Mutex
	lines []AuditLine
	fail  bool
}

func (a *recordingAudit) Append(l AuditLine) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.lines = append(a.lines, l)
	return nil
}

func (a *recordingAudit) AppendBounded(_ context.Context, l AuditLine) error {
	if a.fail {
		return ErrAuditFailed
	}
	return a.Append(l)
}

func (a *recordingAudit) snapshot() []AuditLine {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]AuditLine(nil), a.lines...)
}

type dispatchFixture struct {
	d         *NotificationActionDispatcher
	approvals *fakeActionApprovals
	questions *fakeQuestionReplier
	guidance  *fakeGuidanceAnswerer
	backlog   *fakeBacklogDecider
	resolver  *fakeResolver
	metrics   *recordingMetrics
	audit     *recordingAudit
	flags     *fakeActionFlags
}

func newDispatchFixture() *dispatchFixture {
	f := &dispatchFixture{
		approvals: &fakeActionApprovals{}, questions: &fakeQuestionReplier{route: routeLiveKeystrokeHidden, known: true},
		guidance: &fakeGuidanceAnswerer{}, backlog: &fakeBacklogDecider{}, resolver: &fakeResolver{},
		metrics: &recordingMetrics{}, audit: &recordingAudit{}, flags: &fakeActionFlags{off: map[string]bool{}},
	}
	f.d = NewNotificationActionDispatcher(ActionDelegates{
		Approvals: f.approvals, Questions: f.questions, Guidance: f.guidance, Backlog: f.backlog, Resolver: f.resolver,
	}, func() actionAudit { return f.audit })
	f.d.flags, f.d.metrics = f.flags, f.metrics
	return f
}

func (f *dispatchFixture) invoke(t *testing.T, ctx context.Context, m *sessionv1.InvokeNotificationActionRequest, hdr http.Header) *sessionv1.InvokeNotificationActionResponse {
	t.Helper()
	resp, err := f.d.Invoke(ctx, ActionCall{Msg: m, Peer: "127.0.0.1:50000", Header: hdr})
	require.NoError(t, err)
	return resp
}

func approveReq(key string) *sessionv1.InvokeNotificationActionRequest {
	return &sessionv1.InvokeNotificationActionRequest{
		ActionKind: sessionv1.ActionKind_ACTION_KIND_APPROVE, TargetKeyType: sessionv1.ActionTargetKey_ACTION_TARGET_KEY_APPROVAL_ID,
		TargetKey: "a1", IdempotencyKey: key,
	}
}

func answerReq(key string) *sessionv1.InvokeNotificationActionRequest {
	return &sessionv1.InvokeNotificationActionRequest{
		ActionKind: sessionv1.ActionKind_ACTION_KIND_ANSWER_QUESTION, TargetKeyType: sessionv1.ActionTargetKey_ACTION_TARGET_KEY_QUESTION_ID,
		TargetKey: "q1", SessionId: "s1", IdempotencyKey: key,
		Params: &sessionv1.NotificationActionParams{Answer: "Redis"},
	}
}

func localCtx(t *testing.T) context.Context {
	t.Helper()
	cfg := verdictServiceConfig("localhost:8543", nil)
	return stampedContext(t, &cfg, ListenerLocal, false, "localhost:8543", nil)
}

func TestInvokeNotificationAction_ShouldResolveOnce_WhenSameKeyAndPayloadRepeated(t *testing.T) {
	t.Parallel()
	f := newDispatchFixture()
	ctx := localCtx(t)
	first := f.invoke(t, ctx, approveReq("k1"), nil)
	second := f.invoke(t, ctx, approveReq("k1"), nil)
	assert.Equal(t, outApplied, first.Outcome)
	assert.Equal(t, outApplied, second.Outcome)
	assert.EqualValues(t, 1, f.approvals.calls.Load(), "ResolveApproval ran once")
}

func TestInvokeNotificationAction_ShouldReturnInvalid_WhenKeyReusedWithDifferentPayload(t *testing.T) {
	t.Parallel()
	f := newDispatchFixture()
	ctx := localCtx(t)
	f.invoke(t, ctx, approveReq("k1"), nil)
	other := approveReq("k1")
	other.TargetKey = "a2"
	assert.Equal(t, outInvalid, f.invoke(t, ctx, other, nil).Outcome)
	assert.EqualValues(t, 1, f.approvals.calls.Load())
}

func TestInvokeNotificationAction_ShouldRetryAgain_WhenFirstAttemptWasNotCacheable(t *testing.T) {
	t.Parallel()
	f := newDispatchFixture()
	f.approvals.result = outcome(sessionv1.ActionOutcome_ACTION_OUTCOME_RATE_LIMITED, "")
	ctx := localCtx(t)
	f.invoke(t, ctx, approveReq("k1"), nil)
	f.approvals.result = ActionResult{}
	assert.Equal(t, outApplied, f.invoke(t, ctx, approveReq("k1"), nil).Outcome)
	assert.EqualValues(t, 2, f.approvals.calls.Load())
}

func TestInvokeNotificationAction_ShouldRejectMalformedRequests_WhenKeyKindOrTargetMissing(t *testing.T) {
	t.Parallel()
	f := newDispatchFixture()
	ctx := localCtx(t)
	noKey := approveReq("")
	noTarget := approveReq("k")
	noTarget.TargetKey = ""
	unknownKind := approveReq("k")
	unknownKind.ActionKind = sessionv1.ActionKind(99)
	for name, m := range map[string]*sessionv1.InvokeNotificationActionRequest{"no key": noKey, "no target": noTarget, "unknown kind": unknownKind} {
		assert.Equal(t, outInvalid, f.invoke(t, ctx, m, nil).Outcome, name)
	}
	assert.Zero(t, f.approvals.calls.Load())
}

func TestInvokeNotificationAction_ShouldReturnFlagOffWithZeroWrites_WhenAnswerStageOffOnVisibleSession(t *testing.T) {
	t.Parallel()
	f := newDispatchFixture()
	f.questions.route = routeLiveKeystrokeVisible
	f.flags.answerOff = true
	resp := f.invoke(t, localCtx(t), answerReq("k1"), nil)
	assert.Equal(t, outFlagOff, resp.Outcome)
	assert.Equal(t, flagOffMessage, resp.Detail)
	assert.Zero(t, f.questions.writes.Load())
}

func TestInvokeNotificationAction_ShouldReturnFlagOff_WhenActionScopeOffOrRemoteChannelOff(t *testing.T) {
	t.Parallel()
	f := newDispatchFixture()
	f.flags.off["action:APPROVE"] = true
	assert.Equal(t, outFlagOff, f.invoke(t, localCtx(t), approveReq("k1"), nil).Outcome)

	g := newDispatchFixture()
	g.flags.off[scopeChannelRemote] = true
	remote := stampedContext(t, nil, ListenerRemote, true, "onyx:8444", nil)
	assert.Equal(t, outFlagOff, g.invoke(t, remote, approveReq("k1"), nil).Outcome)
	assert.Zero(t, g.approvals.calls.Load())
}

func TestInvokeNotificationAction_ShouldReturnStale_WhenQuestionSessionUnknown(t *testing.T) {
	t.Parallel()
	f := newDispatchFixture()
	f.questions.known = false
	resp := f.invoke(t, localCtx(t), answerReq("k1"), nil)
	assert.Equal(t, outStale, resp.Outcome)
	assert.Equal(t, "session_ended", resp.StaleReason)
}

func TestInvokeNotificationAction_ShouldRetireDecisionNotifications_WhenBacklogDecisionApplied(t *testing.T) {
	t.Parallel()
	f := newDispatchFixture()
	m := &sessionv1.InvokeNotificationActionRequest{
		ActionKind: sessionv1.ActionKind_ACTION_KIND_BACKLOG_DECISION, TargetKey: "item1", IdempotencyKey: "k1",
		Params: &sessionv1.NotificationActionParams{Choice: choiceAccept},
	}
	assert.Equal(t, outApplied, f.invoke(t, localCtx(t), m, nil).Outcome)
	assert.Equal(t, []string{"item1"}, f.resolver.items)

	f.backlog.result = outcome(outResolved, "")
	m.IdempotencyKey = "k2"
	assert.Equal(t, outResolved, f.invoke(t, localCtx(t), m, nil).Outcome)
	assert.Equal(t, []string{"item1"}, f.resolver.items, "an already-resolved decision does not re-retire")
}

func TestInvokeNotificationAction_ShouldAuditChannelAndCallerKind_WhenSPAHeaderPresentOrAbsent(t *testing.T) {
	t.Parallel()
	f := newDispatchFixture()
	ctx := localCtx(t)
	f.invoke(t, ctx, approveReq("k-spa"), http.Header{actionClientHeader: {actionClientWebUI}})
	f.invoke(t, ctx, approveReq("k-raw"), nil)

	lines := f.audit.snapshot()
	require.Len(t, lines, 2)
	assert.Equal(t, auditKindNotificationAction, lines[0].Kind)
	assert.Equal(t, channelWebLocal, lines[0].Channel)
	assert.Equal(t, callerOperatorUI, lines[0].CallerKind)
	assert.Equal(t, channelUnknown, lines[1].Channel)
	assert.Equal(t, callerUnknown, lines[1].CallerKind)

	var rawSamples int
	for _, s := range f.metrics.samples {
		if s.Channel == channelUnknown {
			rawSamples++
			assert.Equal(t, "approve", s.Kind)
			assert.Equal(t, "applied", s.Outcome)
		}
	}
	assert.Equal(t, 1, rawSamples, "the headerless call counts under channel=unknown")
}

func TestInvokeNotificationAction_ShouldAuditAndCountRefusals_WhenForbiddenOrFlagOff(t *testing.T) {
	t.Parallel()
	f := newDispatchFixture()
	f.flags.off["action:APPROVE"] = true
	f.invoke(t, localCtx(t), approveReq("k1"), nil)
	lines := f.audit.snapshot()
	require.Len(t, lines, 1)
	assert.Equal(t, "flag_off", lines[0].Outcome)
	require.Len(t, f.metrics.samples, 1)
	assert.Equal(t, "flag_off", f.metrics.samples[0].Outcome)
}

func TestInvokeNotificationAction_ShouldNotBlockOnTimers_WhenFakeDelegatesUsed(t *testing.T) {
	t.Parallel()
	f := newDispatchFixture()
	ctx := localCtx(t)
	for i := 0; i < 100; i++ {
		m := approveReq(fmt.Sprintf("seq-%d", i))
		require.Equal(t, outApplied, f.invoke(t, ctx, m, nil).Outcome)
	}
	assert.EqualValues(t, 100, f.approvals.calls.Load())
}

// Opt-in (make benchmark-tier1): p95 of 100 sequential APPROVE calls over fake
// delegates must stay under the 1 s round-trip SLO (REQ-19).
func BenchmarkInvokeNotificationAction_Approve100Sequential(b *testing.B) {
	f := newDispatchFixture()
	ctx := context.Background() // Tier A needs no listener facts
	durations := make([]time.Duration, 0, b.N*100)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for j := 0; j < 100; j++ {
			m := approveReq(fmt.Sprintf("k-%d-%d", i, j))
			start := time.Now()
			if _, err := f.d.Invoke(ctx, ActionCall{Msg: m, Peer: "127.0.0.1:1"}); err != nil {
				b.Fatal(err)
			}
			durations = append(durations, time.Since(start))
		}
	}
	b.StopTimer()
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	p95 := durations[len(durations)*95/100]
	b.ReportMetric(float64(p95.Microseconds()), "p95-us")
	if p95 > time.Second {
		b.Fatalf("p95 %s exceeds the 1 s SLO", p95)
	}
}

func TestActionIdempotency_ShouldEvictOldestKey_WhenOverCapacity(t *testing.T) {
	t.Parallel()
	c := newActionIdempotency(3)
	for _, k := range []string{"a", "b", "c", "d"} {
		f, role := c.begin(k, "h")
		require.Equal(t, flightLeader, role)
		c.settle(f, applied())
	}
	assert.Equal(t, 3, c.len())
	_, role := c.begin("a", "h")
	assert.Equal(t, flightLeader, role, "the oldest key was evicted, so it runs again")
}

// Delegate checks stay in force: a CI-blocked approval is INVALID without confirm.
func TestApprovalResolver_ShouldBeInvalidWithoutConfirmAndAppliedWithIt_WhenCIBlocksApproval(t *testing.T) {
	withIsolatedStateDir(t)
	require.NoError(t, config.LoadConfig().SetFeatureFlag(blockApprovalOnCIFailureFlagName, true))
	store := NewApprovalStore("")
	svc := NewApprovalService(store)
	svc.SetLiveInstanceFinder(&fakeApprovalLiveInstanceFinder{inst: &session.Instance{
		Title: "session-X", UUID: "session-X", GitHubPRNumber: 42, GitHubCheckConclusion: ciConclusionFailure,
	}})
	require.NoError(t, store.Create(newTestPendingApproval("appr-1", "session-X", "Bash")))
	resolver := NewApprovalResolver(svc, store)

	blocked, err := resolver.ResolveApproval(context.Background(), ApprovalChoice{ApprovalID: "appr-1", Allow: true})
	require.NoError(t, err)
	assert.Equal(t, outInvalid, blocked.Outcome)
	_, pending := store.Get("appr-1")
	assert.True(t, pending, "the approval is unresolved")

	confirmed, err := resolver.ResolveApproval(context.Background(), ApprovalChoice{ApprovalID: "appr-1", Allow: true, Override: true})
	require.NoError(t, err)
	assert.Equal(t, outApplied, confirmed.Outcome)

	again, err := resolver.ResolveApproval(context.Background(), ApprovalChoice{ApprovalID: "appr-1", Allow: true})
	require.NoError(t, err)
	assert.Equal(t, outResolved, again.Outcome)
}

func TestGuidanceAnswerer_ShouldReportAlreadyResolved_WhenAnswerNotApplied(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	svc := NewGuidanceRequestService(storage, nil, nil)
	created, err := svc.CreateGuidanceRequest(context.Background(), connect.NewRequest(&sessionv1.CreateGuidanceRequestRequest{
		Scope: "standalone", QuestionText: "Which target?", QuestionType: "short-answer",
	}))
	require.NoError(t, err)
	id := created.Msg.GetRequest().GetId()

	answerer := NewGuidanceAnswerer(svc)
	first, err := answerer.AnswerGuidance(context.Background(), id, "prod")
	require.NoError(t, err)
	second, err := answerer.AnswerGuidance(context.Background(), id, "staging")
	require.NoError(t, err)
	assert.Equal(t, outApplied, first.Outcome)
	assert.Equal(t, outResolved, second.Outcome, "applied=false maps to ALREADY_RESOLVED")
}

func TestMapReplyOutcome_ShouldCoverEveryReplyOutcome(t *testing.T) {
	t.Parallel()
	want := map[sessionv1.ReplyOutcome]sessionv1.ActionOutcome{
		sessionv1.ReplyOutcome_REPLY_OUTCOME_SENT:               outApplied,
		sessionv1.ReplyOutcome_REPLY_OUTCOME_SEND_INDETERMINATE: sessionv1.ActionOutcome_ACTION_OUTCOME_SEND_INDETERMINATE,
		sessionv1.ReplyOutcome_REPLY_OUTCOME_NOT_SENT:           sessionv1.ActionOutcome_ACTION_OUTCOME_NOT_SENT,
		sessionv1.ReplyOutcome_REPLY_OUTCOME_NO_PENDING:         outStale,
		sessionv1.ReplyOutcome_REPLY_OUTCOME_STALE_PROMPT:       outStale,
		sessionv1.ReplyOutcome_REPLY_OUTCOME_RATE_LIMITED:       sessionv1.ActionOutcome_ACTION_OUTCOME_RATE_LIMITED,
		sessionv1.ReplyOutcome_REPLY_OUTCOME_DISABLED:           outFlagOff,
	}
	for r, o := range want {
		assert.Equal(t, o, mapReplyOutcome(&sessionv1.ReplyToPendingQuestionResponse{Outcome: r}).Outcome, r.String())
	}
	assert.Equal(t, "dialog_gone", mapReplyOutcome(&sessionv1.ReplyToPendingQuestionResponse{Outcome: sessionv1.ReplyOutcome_REPLY_OUTCOME_NO_PENDING}).StaleReason)
	assert.Equal(t, "dialog_replaced", mapReplyOutcome(&sessionv1.ReplyToPendingQuestionResponse{Outcome: sessionv1.ReplyOutcome_REPLY_OUTCOME_STALE_PROMPT}).StaleReason)
}

func TestInvokeNotificationAction_ShouldRetireTheDecidedCard_WhenAnswerOrApprovalApplied(t *testing.T) {
	t.Parallel()
	f := newDispatchFixture()
	ctx := localCtx(t)

	f.invoke(t, ctx, answerReq("k-q"), nil)
	f.invoke(t, ctx, approveReq("k-a"), nil)

	assert.Equal(t, []string{"q1"}, f.resolver.questions)
	assert.Equal(t, []string{"a1"}, f.resolver.approvals)
}

func TestInvokeNotificationAction_ShouldKeepTheCard_WhenAnswerWasNotApplied(t *testing.T) {
	t.Parallel()
	f := newDispatchFixture()
	f.questions.result = outcome(sessionv1.ActionOutcome_ACTION_OUTCOME_NOT_SENT, "")

	f.invoke(t, localCtx(t), answerReq("k-q"), nil)

	assert.Empty(t, f.resolver.questions)
}
