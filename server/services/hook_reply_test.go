package services

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/internal/hookipc"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

const skipUntilS1Reviewed = "blocked on S1 owner review: research/spike-S1.md"

// askToolInput is the real AskUserQuestion tool_input shape (spike S1, 2.1.303).
func askToolInput(question string, labels ...string) map[string]interface{} {
	opts := make([]interface{}, len(labels))
	for i, l := range labels {
		opts[i] = map[string]interface{}{"label": l, "description": "d"}
	}
	return map[string]interface{}{"questions": []interface{}{map[string]interface{}{
		"question": question, "header": "Cache", "multiSelect": false, "options": opts,
	}}}
}

type hookReplyEnv struct {
	storage  *session.Storage
	acks     *recordingAcks
	resolver *hookReplyResolver
	flags    *fakeActionFlags
}

func newHookReplyEnv(t *testing.T) *hookReplyEnv {
	t.Helper()
	e := &hookReplyEnv{storage: createTestStorage(t), acks: &recordingAcks{}, flags: &fakeActionFlags{off: map[string]bool{}}}
	e.resolver = &hookReplyResolver{rows: e.storage, flags: e.flags, acks: e.acks}
	return e
}

// openRow stores an unanswered single-question hook row for sessionUUID.
func (e *hookReplyEnv) openRow(t *testing.T, sessionUUID, question string, multi bool, labels ...string) string {
	t.Helper()
	opts := make([]domain.Option, len(labels))
	for i, l := range labels {
		opts[i] = domain.Option{Label: l}
	}
	payload, err := domain.NewQuestionPayload([]domain.Question{{Text: question, Header: "Cache", Options: opts, MultiSelect: multi}})
	require.NoError(t, err)
	gr, err := domain.NewGuidanceRequest(domain.GuidanceRequestParams{
		Scope: domain.RequestScopeSession, SessionUUID: sessionUUID, Source: domain.QuestionSourceHook,
		AskerSessionUUID: sessionUUID, Payload: payload, HookCallID: domain.HookCallID("toolu_" + uuid.NewString()),
	})
	require.NoError(t, err)
	row, err := e.storage.CreateMirroredGuidanceRequest(context.Background(), gr, 0)
	require.NoError(t, err)
	return row.ID
}

func (e *hookReplyEnv) row(t *testing.T, id string) *session.GuidanceRequestData {
	t.Helper()
	row, err := e.storage.GetGuidanceRequest(context.Background(), uuid.MustParse(id))
	require.NoError(t, err)
	return row
}

func TestHookReply_ShouldReturnAllowWithAnswers_WhenARowWasPreAnswered(t *testing.T) {
	e := newHookReplyEnv(t)
	id := e.openRow(t, "s1", "Which cache?", false, "Redis", "Memcached")
	pre, err := e.resolver.PreAnswer(context.Background(), id, "Memcached")
	require.NoError(t, err)
	require.True(t, pre)
	input := askToolInput("Which cache?", "Redis", "Memcached")

	out := e.resolver.Reply(context.Background(), "s1", input)

	require.NotNil(t, out)
	body, err := json.Marshal(out)
	require.NoError(t, err)
	var got struct {
		HookSpecificOutput struct {
			HookEventName      string `json:"hookEventName"`
			PermissionDecision string `json:"permissionDecision"`
			UpdatedInput       struct {
				Questions []map[string]any  `json:"questions"`
				Answers   map[string]string `json:"answers"`
			} `json:"updatedInput"`
		} `json:"hookSpecificOutput"`
	}
	require.NoError(t, json.Unmarshal(body, &got))
	h := got.HookSpecificOutput
	assert.Equal(t, "PreToolUse", h.HookEventName)
	assert.Equal(t, "allow", h.PermissionDecision)
	assert.Equal(t, map[string]string{"Which cache?": "Memcached"}, h.UpdatedInput.Answers, "keyed by the full question text")
	assert.Len(t, h.UpdatedInput.Questions, 1, "the questions are echoed back")
	assert.Equal(t, "Which cache?", h.UpdatedInput.Questions[0]["question"])

	row := e.row(t, id)
	assert.Equal(t, domain.DeliveryStateSent, row.DeliveryState)
	assert.Equal(t, domain.DeliveryRouteHookReply, row.DeliveryRoute)
	sent := e.acks.all()
	require.Len(t, sent, 1)
	assert.Equal(t, domain.DeliveryRouteHookReply, sent[0].Route)
	assert.Nil(t, e.resolver.Reply(context.Background(), "s1", input), "an answer is served once")
}

func TestHookReply_ShouldReturnNothing_WhenNoRowMatchesTheQuestionSessionOrLabel(t *testing.T) {
	e := newHookReplyEnv(t)
	id := e.openRow(t, "s1", "Which cache?", false, "Redis", "Memcached")
	_, err := e.resolver.PreAnswer(context.Background(), id, "Redis")
	require.NoError(t, err)

	tests := []struct {
		name    string
		session string
		input   map[string]interface{}
	}{
		{"another session", "s2", askToolInput("Which cache?", "Redis", "Memcached")},
		{"another question", "s1", askToolInput("Which region?", "Redis", "Memcached")},
		{"the answer is not an option now", "s1", askToolInput("Which cache?", "Memcached", "None")},
		{"not a replyable shape", "s1", map[string]interface{}{"prompt": "?"}},
		{"no session", "", askToolInput("Which cache?", "Redis", "Memcached")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Nil(t, e.resolver.Reply(context.Background(), tt.session, tt.input))
			assert.Equal(t, domain.DeliveryStatePending, e.row(t, id).DeliveryState, "the row is untouched")
		})
	}
}

func TestHookReply_ShouldLeaveAnUnansweredRowToTheDialog(t *testing.T) {
	e := newHookReplyEnv(t)
	id := e.openRow(t, "s1", "Which cache?", false, "Redis", "Memcached")

	assert.Nil(t, e.resolver.Reply(context.Background(), "s1", askToolInput("Which cache?", "Redis", "Memcached")))
	assert.Empty(t, e.row(t, id).DeliveryState)
}

func TestHookReply_ShouldStayClosed_WhenTheAnswerFlagIsOff(t *testing.T) {
	e := newHookReplyEnv(t)
	id := e.openRow(t, "s1", "Which cache?", false, "Redis", "Memcached")
	_, err := e.resolver.PreAnswer(context.Background(), id, "Redis")
	require.NoError(t, err)
	e.flags.answerOff = true

	assert.Nil(t, e.resolver.Reply(context.Background(), "s1", askToolInput("Which cache?", "Redis", "Memcached")))
	assert.Equal(t, domain.DeliveryStatePending, e.row(t, id).DeliveryState)
}

func TestHookReply_PreAnswer_ShouldRefuseASecondAnswer(t *testing.T) {
	e := newHookReplyEnv(t)
	id := e.openRow(t, "s1", "Which cache?", false, "Redis", "Memcached")

	first, err := e.resolver.PreAnswer(context.Background(), id, "Redis")
	require.NoError(t, err)
	second, err := e.resolver.PreAnswer(context.Background(), id, "Memcached")
	require.NoError(t, err)

	assert.True(t, first)
	assert.False(t, second)
	assert.Equal(t, "Redis", e.row(t, id).Answer)
}

// All stages are configured open, but the compiled S1 evidence is false, so the
// production flag composition keeps the rung closed.
func TestHookReply_ShouldStayClosedUnderProductionFlags_WhileS1IsNotRecorded(t *testing.T) {
	withIsolatedStateDir(t)
	cfg := config.LoadConfig()
	for _, sc := range []string{scopeStageRender, scopeStageAnswer, scopeStageHookReply} {
		require.NoError(t, cfg.SetFeatureFlagScope(askUserQuestionStructuredFlagName, sc, true))
	}
	require.NoError(t, cfg.SetFeatureFlagScope(notificationInlineActionsFlagName, scopePrefixAction+actionKindAnswerQuestion, true))
	e := newHookReplyEnv(t)
	e.resolver.flags = configActionFlags{ev: compiledFlagEvidence}
	id := e.openRow(t, "s1", "Which cache?", false, "Redis")
	_, err := e.resolver.PreAnswer(context.Background(), id, "Redis")
	require.NoError(t, err)

	out := e.resolver.Reply(context.Background(), "s1", askToolInput("Which cache?", "Redis"))

	if spikeS1HookReplyPassed {
		require.NotNil(t, out, "S1 is recorded and every stage is open")
		return
	}
	assert.Nil(t, out)
}

// HookReply_* from the plan: the end-to-end rung through the real flag chain.
func TestHookReply_ShouldServeThroughTheRealFlagChain_WhenS1IsRecorded(t *testing.T) {
	if !spikeS1HookReplyPassed {
		t.Skip(skipUntilS1Reviewed)
	}
	withIsolatedStateDir(t)
	cfg := config.LoadConfig()
	for _, sc := range []string{scopeStageRender, scopeStageAnswer, scopeStageHookReply} {
		require.NoError(t, cfg.SetFeatureFlagScope(askUserQuestionStructuredFlagName, sc, true))
	}
	require.NoError(t, cfg.SetFeatureFlagScope(notificationInlineActionsFlagName, scopePrefixAction+actionKindAnswerQuestion, true))
	e := newHookReplyEnv(t)
	e.resolver.flags = configActionFlags{ev: compiledFlagEvidence}
	id := e.openRow(t, "s1", "Which cache?", false, "Redis")
	_, err := e.resolver.PreAnswer(context.Background(), id, "Redis")
	require.NoError(t, err)

	assert.NotNil(t, e.resolver.Reply(context.Background(), "s1", askToolInput("Which cache?", "Redis")))
}

type stubReplier struct {
	out     *hookipc.HookOutput
	session string
	calls   int
}

func (s *stubReplier) Reply(_ context.Context, sessionUUID string, _ map[string]interface{}) *hookipc.HookOutput {
	s.calls++
	s.session = sessionUUID
	return s.out
}

func TestHookClassifier_Classify_ShouldReturnTheReplierAnswer_WhenAskUserQuestionIsAnswered(t *testing.T) {
	answer := hookAllowWithAnswers(askToolInput("Q?", "A"), map[string]string{"Q?": "A"})
	replier := &stubReplier{out: answer}
	service := NewHookClassifier(&hookPolicyStub{}, nil)
	service.SetHookReplier(replier)
	request := hookRequest("AskUserQuestion")
	request.Context.Env = map[string]string{SessionUUIDEnv: "uuid-1"}

	reply, err := service.Classify(context.Background(), request)

	require.NoError(t, err)
	assert.Same(t, answer, reply.Output)
	assert.Equal(t, hookipc.DecisionSourcePrimary, reply.Source)
	assert.Equal(t, "uuid-1", replier.session)
}

func TestHookClassifier_Classify_ShouldDefer_WhenTheReplierHasNoAnswerOrIsAbsent(t *testing.T) {
	withReplier := NewHookClassifier(&hookPolicyStub{}, nil)
	withReplier.SetHookReplier(&stubReplier{})
	for name, svc := range map[string]*HookClassifier{"no answer": withReplier, "no replier": NewHookClassifier(&hookPolicyStub{}, nil)} {
		reply, err := svc.Classify(context.Background(), hookRequest("AskUserQuestion"))
		require.NoError(t, err, name)
		assert.Nil(t, reply.Output, name)
		assert.Equal(t, hookipc.DecisionSourceDefer, reply.Source, name)
	}
}
