package services

import (
	"context"
	"fmt"
	"sort"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

type fakeApprovals struct {
	ids   map[string]bool
	calls atomic.Int32
}

func (f *fakeApprovals) PendingApprovalIDs() map[string]bool {
	f.calls.Add(1)
	return f.ids
}

type fakeQuestions struct {
	live  map[QuestionKey]PendingQuestion
	calls atomic.Int32
}

func (f *fakeQuestions) PeekQuestions(keys []QuestionKey) map[QuestionKey]PendingQuestion {
	f.calls.Add(1)
	out := map[QuestionKey]PendingQuestion{}
	for _, k := range keys {
		if q, ok := f.live[k]; ok {
			out[k] = q
		}
	}
	return out
}

type fakeBacklog struct {
	states map[string]BacklogDecisionState
	calls  atomic.Int32
}

func (f *fakeBacklog) DecisionStates(_ context.Context, ids []string) (map[string]BacklogDecisionState, error) {
	f.calls.Add(1)
	out := map[string]BacklogDecisionState{}
	for _, id := range ids {
		if s, ok := f.states[id]; ok {
			out[id] = s
		}
	}
	return out, nil
}

type fakeGuidance struct {
	open  map[string]OpenGuidance
	calls atomic.Int32
}

func (f *fakeGuidance) OpenGuidance(_ context.Context, ids []string) (map[string]OpenGuidance, error) {
	f.calls.Add(1)
	out := map[string]OpenGuidance{}
	for _, id := range ids {
		if g, ok := f.open[id]; ok {
			out[id] = g
		}
	}
	return out, nil
}

type descriptorFixture struct {
	approvals *fakeApprovals
	questions *fakeQuestions
	backlog   *fakeBacklog
	guidance  *fakeGuidance
	flags     map[string]bool
}

func newDescriptorFixture() *descriptorFixture {
	return &descriptorFixture{
		approvals: &fakeApprovals{ids: map[string]bool{}},
		questions: &fakeQuestions{live: map[QuestionKey]PendingQuestion{}},
		backlog:   &fakeBacklog{states: map[string]BacklogDecisionState{}},
		guidance:  &fakeGuidance{open: map[string]OpenGuidance{}},
		flags:     map[string]bool{},
	}
}

// allOn turns every decision point on (the default for these tests).
func (f *descriptorFixture) builder() *DescriptorBuilder {
	b := NewDescriptorBuilder(DescriptorSources{
		Approvals: f.approvals, Questions: f.questions, Backlog: f.backlog, Guidance: f.guidance,
	})
	b.flag = func(dp string) bool {
		if v, ok := f.flags[dp]; ok {
			return v
		}
		return true
	}
	return b
}

func kinds(r interface {
	GetActions() []*sessionv1.NotificationActionDescriptor
}) []string {
	var out []string
	for _, d := range r.GetActions() {
		out = append(out, kindName(d.GetKind()))
	}
	return out
}

func questionRecord(id, session, qid string) *sessionv1.NotificationHistoryRecord {
	return &sessionv1.NotificationHistoryRecord{
		Id: id, SessionId: session,
		Metadata: map[string]string{metaQuestionID: qid},
	}
}

func TestBuildDescriptors_ShouldOfferApproveDenyOpen_WhenApprovalPending(t *testing.T) {
	t.Parallel()
	f := newDescriptorFixture()
	f.approvals.ids["a1"] = true
	rec := &sessionv1.NotificationHistoryRecord{Id: "a1", SessionId: "s1", Metadata: map[string]string{metaApprovalID: "a1"}}
	f.builder().Annotate(context.Background(), []*sessionv1.NotificationHistoryRecord{rec}, ListenerClassLocal)
	assert.Equal(t, []string{"APPROVE", "DENY", "OPEN"}, kinds(rec))
	assert.Equal(t, "a1", rec.Actions[0].TargetKey)
	assert.Equal(t, sessionv1.ActionTargetKey_ACTION_TARGET_KEY_APPROVAL_ID, rec.Actions[0].TargetKeyType)
}

func TestBuildDescriptors_ShouldOfferOnlyOpen_WhenApprovalAlreadyResolved(t *testing.T) {
	t.Parallel()
	f := newDescriptorFixture() // approval no longer in the store
	rec := &sessionv1.NotificationHistoryRecord{Id: "a1", Metadata: map[string]string{metaApprovalID: "a1"}}
	read := &sessionv1.NotificationHistoryRecord{Id: "a2", IsRead: true, Metadata: map[string]string{metaApprovalID: "a2"}}
	f.approvals.ids["a2"] = true
	f.builder().Annotate(context.Background(), []*sessionv1.NotificationHistoryRecord{rec, read}, ListenerClassLocal)
	assert.Equal(t, []string{"OPEN"}, kinds(rec))
	assert.Equal(t, []string{"OPEN"}, kinds(read))
}

func TestBuildDescriptors_ShouldCarryOptionLabelsTierB_WhenQuestionPendingAndLocal(t *testing.T) {
	t.Parallel()
	f := newDescriptorFixture()
	f.questions.live[QuestionKey{SessionUUID: "s1", QuestionID: "q1"}] = PendingQuestion{QuestionID: "q1", SessionUUID: "s1"}
	q := f.questions.live[QuestionKey{SessionUUID: "s1", QuestionID: "q1"}]
	q.Dialog.Labels = []string{"Redis", "Memcached"}
	f.questions.live[QuestionKey{SessionUUID: "s1", QuestionID: "q1"}] = q

	rec := questionRecord("n1", "s1", "q1")
	f.builder().Annotate(context.Background(), []*sessionv1.NotificationHistoryRecord{rec}, ListenerClassLocal)

	assert.Equal(t, []string{"ANSWER_QUESTION", "OPEN"}, kinds(rec))
	d := rec.Actions[0]
	assert.Equal(t, sessionv1.ActionTier_ACTION_TIER_B, d.Tier)
	assert.True(t, d.Available)
	require.Len(t, d.Options, 2)
	assert.Equal(t, "Redis", d.Options[0].Label)
	assert.Equal(t, "Memcached", d.Options[1].Label)
	assert.Equal(t, "q1", d.TargetKey)
	assert.Equal(t, "s1", d.SessionId)
}

func TestBuildDescriptors_ShouldOfferOnlyOpen_WhenQuestionAnsweredOrReplyUnavailable(t *testing.T) {
	t.Parallel()
	f := newDescriptorFixture() // store has no q1: answered
	answered := questionRecord("n1", "s1", "q1")
	unavailable := questionRecord("n2", "s1", "q2")
	unavailable.Metadata[metaReplyUnavailable] = "true"
	f.questions.live[QuestionKey{SessionUUID: "s1", QuestionID: "q2"}] = PendingQuestion{}
	f.builder().Annotate(context.Background(), []*sessionv1.NotificationHistoryRecord{answered, unavailable}, ListenerClassLocal)
	assert.Equal(t, []string{"OPEN"}, kinds(answered))
	assert.Equal(t, []string{"OPEN"}, kinds(unavailable))
}

func TestBuildDescriptors_ShouldMarkTierBDesktopOnly_WhenListenerRemote(t *testing.T) {
	t.Parallel()
	f := newDescriptorFixture()
	f.questions.live[QuestionKey{SessionUUID: "s1", QuestionID: "q1"}] = PendingQuestion{}
	page := func() []*sessionv1.NotificationHistoryRecord {
		return []*sessionv1.NotificationHistoryRecord{questionRecord("n1", "s1", "q1")}
	}

	local, remote := page(), page()
	f.builder().Annotate(context.Background(), local, ListenerClassLocal)
	f.builder().Annotate(context.Background(), remote, ListenerClassRemote)

	assert.True(t, local[0].Actions[0].Available)
	assert.Empty(t, local[0].Actions[0].UnavailableReason)
	assert.False(t, remote[0].Actions[0].Available)
	assert.Equal(t, "desktop_only", remote[0].Actions[0].UnavailableReason)
	assert.Equal(t, sessionv1.ActionTier_ACTION_TIER_B, remote[0].Actions[0].Tier)
}

func TestBuildDescriptors_ShouldOfferAnswerGuidance_WhenGuidanceOpen(t *testing.T) {
	t.Parallel()
	f := newDescriptorFixture()
	f.guidance.open["g1"] = OpenGuidance{Options: []string{"yes", "no"}}
	open := &sessionv1.NotificationHistoryRecord{Id: "n1", Metadata: map[string]string{metaGuidanceRequestID: "g1"}}
	closed := &sessionv1.NotificationHistoryRecord{Id: "n2", Metadata: map[string]string{metaGuidanceRequestID: "g2"}}
	f.builder().Annotate(context.Background(), []*sessionv1.NotificationHistoryRecord{open, closed}, ListenerClassRemote)

	assert.Equal(t, []string{"ANSWER_GUIDANCE", "OPEN"}, kinds(open))
	assert.True(t, open.Actions[0].Available, "guidance is Tier A and works remotely")
	assert.Equal(t, []string{"OPEN"}, kinds(closed))
}

func TestBuildDescriptors_ShouldOfferDecisionChoices_WhenItemInReviewWithSchema(t *testing.T) {
	t.Parallel()
	f := newDescriptorFixture()
	f.backlog.states["b608ab1e"] = BacklogDecisionState{Status: "review"}
	f.backlog.states["pr1"] = BacklogDecisionState{Status: "pr_pending"}
	f.backlog.states["done1"] = BacklogDecisionState{Status: "done"}
	schema := EncodeDecisionSchema(DefaultDecisionSchema())
	mk := func(id string) *sessionv1.NotificationHistoryRecord {
		return &sessionv1.NotificationHistoryRecord{Id: "n-" + id, Metadata: map[string]string{metaItemID: id, metaDecisionSchema: schema}}
	}
	review, pr, done := mk("b608ab1e"), mk("pr1"), mk("done1")
	f.builder().Annotate(context.Background(), []*sessionv1.NotificationHistoryRecord{review, pr, done}, ListenerClassRemote)

	assert.Equal(t, []string{"BACKLOG_DECISION", "OPEN"}, kinds(review))
	var values []string
	for _, o := range review.Actions[0].Options {
		values = append(values, o.Value)
	}
	assert.Equal(t, []string{"accept", "rework", "merge"}, values)
	assert.Equal(t, []string{"merge"}, optionValues(pr.Actions[0]))
	assert.Equal(t, []string{"OPEN"}, kinds(done))
}

func optionValues(d *sessionv1.NotificationActionDescriptor) []string {
	var out []string
	for _, o := range d.GetOptions() {
		out = append(out, o.GetValue())
	}
	return out
}

func TestBuildDescriptors_ShouldDropUnknownChoice_WhenSchemaNamesChoiceWithoutRPC(t *testing.T) {
	t.Parallel()
	f := newDescriptorFixture()
	f.backlog.states["i1"] = BacklogDecisionState{Status: "review"}
	schema := EncodeDecisionSchema(DecisionSchema{Choices: []DecisionChoice{{Value: "accept"}, {Value: "teleport"}}})
	rec := &sessionv1.NotificationHistoryRecord{Metadata: map[string]string{metaItemID: "i1", metaDecisionSchema: schema}}
	f.builder().Annotate(context.Background(), []*sessionv1.NotificationHistoryRecord{rec}, ListenerClassLocal)
	assert.Equal(t, []string{"accept"}, optionValues(rec.Actions[0]))
}

func TestBuildDescriptors_ShouldReduceToOpen_WhenActionFlagOff(t *testing.T) {
	t.Parallel()
	f := newDescriptorFixture()
	f.approvals.ids["a1"] = true
	f.guidance.open["g1"] = OpenGuidance{}
	f.flags["action:APPROVE"] = false
	f.flags["action:DENY"] = false
	appr := &sessionv1.NotificationHistoryRecord{Metadata: map[string]string{metaApprovalID: "a1"}}
	guid := &sessionv1.NotificationHistoryRecord{Metadata: map[string]string{metaGuidanceRequestID: "g1"}}
	f.builder().Annotate(context.Background(), []*sessionv1.NotificationHistoryRecord{appr, guid}, ListenerClassLocal)
	assert.Equal(t, []string{"OPEN"}, kinds(appr))
	assert.Equal(t, []string{"ANSWER_GUIDANCE", "OPEN"}, kinds(guid))
}

func TestBuildDescriptors_ShouldReduceToOpen_WhenRemoteChannelOff(t *testing.T) {
	t.Parallel()
	f := newDescriptorFixture()
	f.approvals.ids["a1"] = true
	f.flags["channel:remote"] = false
	rec := &sessionv1.NotificationHistoryRecord{Metadata: map[string]string{metaApprovalID: "a1"}}
	local := &sessionv1.NotificationHistoryRecord{Metadata: map[string]string{metaApprovalID: "a1"}}
	f.builder().Annotate(context.Background(), []*sessionv1.NotificationHistoryRecord{rec}, ListenerClassRemote)
	f.builder().Annotate(context.Background(), []*sessionv1.NotificationHistoryRecord{local}, ListenerClassLocal)
	assert.Equal(t, []string{"OPEN"}, kinds(rec))
	assert.Equal(t, []string{"APPROVE", "DENY", "OPEN"}, kinds(local))
}

func TestListenerClassFrom_ShouldDeriveFromStampedRequest_WhenLocalRemoteOrUnstamped(t *testing.T) {
	t.Parallel()
	assert.Equal(t, ListenerClassLocal, ListenerClassFrom(stampedContext(t, nil, ListenerLocal, false, "localhost:8543", nil)))
	assert.Equal(t, ListenerClassRemote, ListenerClassFrom(stampedContext(t, nil, ListenerRemote, true, "onyx:8444", nil)))
	assert.Equal(t, ListenerClassRemote, ListenerClassFrom(context.Background()), "unstamped fails closed")
}

// pageOf builds n records over sessions sessions: a mix of approval, question,
// guidance and decision records so every store is needed.
func pageOf(f *descriptorFixture, n, sessions int) []*sessionv1.NotificationHistoryRecord {
	schema := EncodeDecisionSchema(DefaultDecisionSchema())
	out := make([]*sessionv1.NotificationHistoryRecord, 0, n)
	for i := 0; i < n; i++ {
		sess := fmt.Sprintf("s%d", i%sessions)
		id := fmt.Sprintf("n%d", i)
		switch i % 4 {
		case 0:
			f.approvals.ids[id] = true
			out = append(out, &sessionv1.NotificationHistoryRecord{Id: id, SessionId: sess, Metadata: map[string]string{metaApprovalID: id}})
		case 1:
			f.questions.live[QuestionKey{SessionUUID: sess, QuestionID: id}] = PendingQuestion{}
			out = append(out, questionRecord(id, sess, id))
		case 2:
			f.guidance.open[id] = OpenGuidance{}
			out = append(out, &sessionv1.NotificationHistoryRecord{Id: id, SessionId: sess, Metadata: map[string]string{metaGuidanceRequestID: id}})
		default:
			f.backlog.states[id] = BacklogDecisionState{Status: "review"}
			out = append(out, &sessionv1.NotificationHistoryRecord{Id: id, SessionId: sess, Metadata: map[string]string{metaItemID: id, metaDecisionSchema: schema}})
		}
	}
	return out
}

func TestBuildDescriptors_ShouldCallEachStoreOncePerPage_When300RecordsOver40Sessions(t *testing.T) {
	t.Parallel()
	f := newDescriptorFixture()
	page := pageOf(f, 300, 40)
	f.builder().Annotate(context.Background(), page, ListenerClassLocal)
	assert.EqualValues(t, 1, f.approvals.calls.Load())
	assert.EqualValues(t, 1, f.questions.calls.Load())
	assert.EqualValues(t, 1, f.backlog.calls.Load())
	assert.EqualValues(t, 1, f.guidance.calls.Load())
	for _, r := range page {
		assert.Greater(t, len(r.Actions), 1, "record %s has its action plus OPEN", r.Id)
	}
}

func TestBuildDescriptors_ShouldSkipStores_WhenEveryRecordResolved(t *testing.T) {
	t.Parallel()
	f := newDescriptorFixture()
	page := pageOf(f, 40, 4)
	for _, r := range page {
		r.IsRead = true
	}
	f.builder().Annotate(context.Background(), page, ListenerClassLocal)
	assert.Zero(t, f.approvals.calls.Load()+f.questions.calls.Load()+f.backlog.calls.Load()+f.guidance.calls.Load())
	for _, r := range page {
		assert.Equal(t, []string{"OPEN"}, kinds(r))
	}
}

func BenchmarkBuildDescriptors_300RecordsWith40Sessions(b *testing.B) {
	f := newDescriptorFixture()
	builder := f.builder()
	ctx := context.Background()
	durations := make([]time.Duration, 0, b.N)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		page := pageOf(f, 300, 40)
		b.StartTimer()
		start := time.Now()
		builder.Annotate(ctx, page, ListenerClassLocal)
		durations = append(durations, time.Since(start))
	}
	b.StopTimer()
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	p95 := durations[len(durations)*95/100]
	b.ReportMetric(float64(p95.Microseconds()), "p95-us")
	if p95 > 100*time.Millisecond {
		b.Fatalf("p95 %s exceeds the 100 ms budget", p95)
	}
}

// 2.8c: every ActionKind has exactly one tier row and remote availability.
func TestEveryActionKindHasTierAndRemoteAvailability(t *testing.T) {
	t.Parallel()
	assert.Empty(t, kindsWithoutPolicy(sessionv1.ActionKind_name), "add a row to actionPolicies for each new ActionKind")
	for kind, p := range actionPolicies {
		assert.Equal(t, p.Tier == sessionv1.ActionTier_ACTION_TIER_A, p.RemoteAvailable,
			"%s: remote availability must follow the tier (Tier B is desktop-only)", kind)
		assert.NotEqual(t, sessionv1.ActionTier_ACTION_TIER_UNSPECIFIED, p.Tier, kind.String())
	}
}

func TestEveryActionKindHasTierAndRemoteAvailability_ShouldFail_WhenFixtureKindHasNoRow(t *testing.T) {
	t.Parallel()
	withFixture := map[int32]string{}
	for v, n := range sessionv1.ActionKind_name {
		withFixture[v] = n
	}
	withFixture[9999] = "ACTION_KIND_FIXTURE_ONLY"
	assert.Equal(t, []int32{9999}, kindsWithoutPolicy(withFixture))
}

func TestControlPolicies_ShouldMakeResumeScheduledLocalOnly(t *testing.T) {
	t.Parallel()
	assert.True(t, controlPolicies[ControlRemindMe].RemoteAvailable)
	assert.False(t, controlPolicies[ControlResumeScheduled].RemoteAvailable, "remote shows Resume from desktop")
}

func TestAnswerTier_ShouldBeTierBOnlyForLiveKeystroke(t *testing.T) {
	t.Parallel()
	assert.Equal(t, sessionv1.ActionTier_ACTION_TIER_B, answerTier(routeLiveKeystrokeHidden))
	assert.Equal(t, sessionv1.ActionTier_ACTION_TIER_B, answerTier(routeLiveKeystrokeVisible))
	assert.Equal(t, sessionv1.ActionTier_ACTION_TIER_A, answerTier(routeDurableRecord))
	assert.Equal(t, sessionv1.ActionTier_ACTION_TIER_A, answerTier(routeHookReply))
}

// kindsWithoutPolicy lists the non-zero kind values of names with no table row.
func kindsWithoutPolicy(names map[int32]string) []int32 {
	var missing []int32
	for v := range names {
		if v == 0 {
			continue
		}
		if _, ok := actionPolicies[sessionv1.ActionKind(v)]; !ok {
			missing = append(missing, v)
		}
	}
	sort.Slice(missing, func(i, j int) bool { return missing[i] < missing[j] })
	return missing
}
