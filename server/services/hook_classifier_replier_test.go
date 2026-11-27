package services

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/internal/hookipc"
	"github.com/tstapler/stapler-squad/server/services/approvals"
)

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
	answer := &hookipc.HookOutput{HookSpecificOutput: hookipc.HookSpecificOutput{HookEventName: "PreToolUse", PermissionDecision: "allow"}}
	replier := &stubReplier{out: answer}
	service := approvals.NewHookClassifier(&hookPolicyStub{}, nil)
	service.SetHookReplier(replier)
	request := hookRequest("AskUserQuestion")
	request.Context.Env = map[string]string{approvals.SessionUUIDEnv: "uuid-1"}

	reply, err := service.Classify(context.Background(), request)

	require.NoError(t, err)
	assert.Same(t, answer, reply.Output)
	assert.Equal(t, hookipc.DecisionSourcePrimary, reply.Source)
	assert.Equal(t, "uuid-1", replier.session)
}

func TestHookClassifier_Classify_ShouldDefer_WhenTheReplierHasNoAnswerOrIsAbsent(t *testing.T) {
	withReplier := approvals.NewHookClassifier(&hookPolicyStub{}, nil)
	withReplier.SetHookReplier(&stubReplier{})
	for name, svc := range map[string]*approvals.HookClassifier{"no answer": withReplier, "no replier": approvals.NewHookClassifier(&hookPolicyStub{}, nil)} {
		reply, err := svc.Classify(context.Background(), hookRequest("AskUserQuestion"))
		require.NoError(t, err, name)
		assert.Nil(t, reply.Output, name)
		assert.Equal(t, hookipc.DecisionSourceDefer, reply.Source, name)
	}
}
