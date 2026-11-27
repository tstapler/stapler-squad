package notify

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/services/featureflags"
	"github.com/tstapler/stapler-squad/server/services/flagsvc"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/server/services/questionkit"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/panereply"
)

var inlineApprove = flagsvc.ScopePrefixAction + "APPROVE"

// inlineOnRegistry is the shared test registry with the inline-action defaults flipped.
func inlineOnRegistry() *featureflags.Registry {
	specs := testFlagRegistry().Specs()
	for i := range specs {
		if specs[i].Name == flagsvc.NotificationInlineActionsFlagName || specs[i].Name == flagsvc.AskUserQuestionStructuredFlagName {
			specs[i].Default = !specs[i].Default
		}
	}
	return featureflags.NewRegistry(specs)
}

func TestBuildDescriptors_ShouldDegradeHookQuestionToReadOnly_WhenCanaryTripped_ExceptWithHookReply(t *testing.T) {
	t.Parallel()
	f := NewDescriptorFixture()
	key := questionkit.QuestionKey{SessionUUID: "s1", QuestionID: "q1"}
	f.questions.live[key] = questionkit.PendingQuestion{QuestionID: "q1", SessionUUID: "s1"}
	clock := time.Unix(1_800_000_000, 0)
	canary := questionkit.NewDialogMatchCanary(func() time.Time { return clock }, nil, nil)
	record := func(matches ...bool) {
		for _, m := range matches {
			canary.Record(questionkit.DialogMatchResultOf(m))
			clock = clock.Add(time.Minute)
		}
	}
	record(false, false, false, false, false)
	require.True(t, canary.Tripped())
	annotate := func(hookReply bool) *sessionv1.NotificationActionDescriptor {
		b := NewDescriptorBuilder(DescriptorSources{Questions: f.questions, Canary: canary}, testFlagRegistry())
		b.flag = func(string) bool { return true }
		b.hookReplyOn = func() bool { return hookReply }
		rec := QuestionRecord("n1", "s1", "q1")
		b.Annotate(context.Background(), []*sessionv1.NotificationHistoryRecord{rec}, ListenerClassLocal)
		return rec.Actions[0]
	}

	degraded := annotate(false)
	assert.False(t, degraded.Available)
	assert.Equal(t, "read_only", degraded.UnavailableReason)

	assert.True(t, annotate(true).Available, "the hook_reply route is not keystroke-based and keeps answering")

	record(true, true, true, true, true, true, true, true, true, true, true)
	require.False(t, canary.Tripped())
	assert.True(t, annotate(false).Available, "the descriptor returns once the share recovers")
}

func tabbedGuidance() OpenGuidance {
	return OpenGuidance{Source: domain.QuestionSourceHook, Tabbed: []panereply.TabbedQuestion{
		{Text: "Which cache?", Header: "Cache", Labels: []string{"Redis", "None"}},
		{Text: "Which letters?", Header: "Letters", Labels: []string{"A", "B"}, MultiSelect: true},
	}}
}

func tabbedRecord() *sessionv1.NotificationHistoryRecord {
	return &sessionv1.NotificationHistoryRecord{Id: "n1", SessionId: "s1", Metadata: map[string]string{
		questionkit.MetaGuidanceRequestID: "row1", questionkit.MetaQuestionID: "row1", "question_shape": string(questionkit.QuestionShapeMulti), questionkit.MetaReplyUnavailable: "shape",
	}}
}

func TestBuildDescriptors_should_OfferAnswerQuestionWithPerQuestionOptions_WhenATabbedHookRowIsOpen(t *testing.T) {
	t.Parallel()
	f := NewDescriptorFixture()
	f.guidance.open["row1"] = tabbedGuidance()
	rec := tabbedRecord()

	f.builder().Annotate(context.Background(), []*sessionv1.NotificationHistoryRecord{rec}, ListenerClassLocal)

	assert.Equal(t, []string{"ANSWER_QUESTION", "OPEN"}, Kinds(rec))
	d := rec.Actions[0]
	assert.Equal(t, sessionv1.ActionTier_ACTION_TIER_B, d.Tier)
	assert.True(t, d.Available)
	assert.Equal(t, "row1", d.TargetKey)
	assert.Equal(t, "s1", d.SessionId)
	got := make([]string, len(d.Options))
	for i, o := range d.Options {
		got[i] = o.Value + ":" + o.Label
	}
	assert.Equal(t, []string{"0:Redis", "0:None", "1:A", "1:B"}, got, "the option value is the index of its question")
}

func TestBuildDescriptors_should_ReduceATabbedRowToOpen_WhenItIsClosedOrNotAHookTabbedRow(t *testing.T) {
	t.Parallel()
	f := NewDescriptorFixture()
	closed := tabbedRecord() // the row is no longer open: absent from the lookup
	notTabbed := tabbedRecord()
	notTabbed.Metadata[questionkit.MetaGuidanceRequestID] = "row2"
	f.guidance.open["row2"] = OpenGuidance{Source: domain.QuestionSourceAgentMCP}

	f.builder().Annotate(context.Background(), []*sessionv1.NotificationHistoryRecord{closed, notTabbed}, ListenerClassLocal)

	assert.Equal(t, []string{"OPEN"}, Kinds(closed))
	assert.Equal(t, []string{"ANSWER_GUIDANCE", "OPEN"}, Kinds(notTabbed), "an agent row keeps its stored-answer action and gains no keystroke one")
}

func TestBuildDescriptors_should_DegradeATabbedAnswer_WhenRemoteOrTheLayoutCanaryTripped(t *testing.T) {
	t.Parallel()
	f := NewDescriptorFixture()
	f.guidance.open["row1"] = tabbedGuidance()
	remote := tabbedRecord()
	f.builder().Annotate(context.Background(), []*sessionv1.NotificationHistoryRecord{remote}, ListenerClassRemote)
	assert.False(t, remote.Actions[0].Available)
	assert.Equal(t, "desktop_only", remote.Actions[0].UnavailableReason)

	canary := questionkit.NewDialogMatchCanary(nil, nil, nil)
	for range questionkit.CanaryMinAttempts {
		canary.Record(questionkit.DialogMismatch)
	}
	b := f.builder()
	b.sources.Canary = canary
	b.hookReplyOn = func() bool { return false }
	local := tabbedRecord()
	b.Annotate(context.Background(), []*sessionv1.NotificationHistoryRecord{local}, ListenerClassLocal)
	assert.False(t, local.Actions[0].Available)
	assert.Equal(t, unavailableReadOnly, local.Actions[0].UnavailableReason)
}

func TestGuidanceDescriptors_should_OfferCancelRequestWithConfirmAndPeerLabel_When_PeerHelpIsOpen(t *testing.T) {
	b := &DescriptorBuilder{
		sources: DescriptorSources{Guidance: &FakeGuidance{open: map[string]OpenGuidance{"g1": {PeerHelp: true}}}},
		flag:    func(string) bool { return true },
	}
	rec := &sessionv1.NotificationHistoryRecord{Metadata: map[string]string{questionkit.MetaGuidanceRequestID: "g1", questionkit.MetaPeerTitle: "diagnose:abc"}}

	b.Annotate(context.Background(), []*sessionv1.NotificationHistoryRecord{rec}, ListenerClassLocal)

	kinds := make([]sessionv1.ActionKind, 0, len(rec.Actions))
	for _, a := range rec.Actions {
		kinds = append(kinds, a.Kind)
	}
	assert.Equal(t, []sessionv1.ActionKind{
		sessionv1.ActionKind_ACTION_KIND_ANSWER_GUIDANCE, sessionv1.ActionKind_ACTION_KIND_CANCEL_PEER_HELP, sessionv1.ActionKind_ACTION_KIND_OPEN,
	}, kinds)
	assert.Equal(t, "Answer for diagnose:abc", rec.Actions[0].Label)
	assert.True(t, rec.Actions[1].RequiresConfirm)
	assert.Equal(t, sessionv1.ActionTier_ACTION_TIER_A, rec.Actions[1].Tier)
}

func TestNewDescriptorBuilder_ShouldFlagOnInjectedRegistry(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	assert.True(t, NewDescriptorBuilder(DescriptorSources{}, inlineOnRegistry()).flag(inlineApprove))
	assert.False(t, NewDescriptorBuilder(DescriptorSources{}, testFlagRegistry()).flag(inlineApprove))
}
