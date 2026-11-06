package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/internal/claudehooks"
	"github.com/tstapler/stapler-squad/internal/hookipc"
	"github.com/tstapler/stapler-squad/pkg/classifier"
)

var askPayload = classifier.PermissionRequestPayload{
	ToolName: "AskUserQuestion", ToolUseID: "toolu_1", SessionID: "claude-session",
	ToolInput: map[string]interface{}{"questions": []interface{}{map[string]interface{}{"question": "Cache?"}}},
}

func answerOutput() *hookipc.HookOutput {
	return &hookipc.HookOutput{HookSpecificOutput: hookipc.HookSpecificOutput{
		HookEventName: "PreToolUse", PermissionDecision: "allow",
		UpdatedInput: map[string]any{"answers": map[string]any{"Cache?": "Redis"}},
	}}
}

func envOf(vars map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) { v, ok := vars[k]; return v, ok }
}

func TestAskUserQuestionViaResident_ShouldPrintTheAnswer_AndSendOnlyTheSessionUUID(t *testing.T) {
	t.Parallel()
	tr := &fakeTransport{reply: hookipc.ClassificationReply{Output: answerOutput()}}
	var out bytes.Buffer

	askUserQuestionViaResident(askPayload, fakeDialer(tr, nil), envOf(map[string]string{sessionUUIDEnv: "uuid-1", "SECRET": "x"}), &out)

	var got map[string]map[string]any
	require.NoError(t, json.Unmarshal(out.Bytes(), &got))
	assert.Equal(t, "allow", got["hookSpecificOutput"]["permissionDecision"])
	assert.Equal(t, map[string]string{sessionUUIDEnv: "uuid-1"}, tr.got.Context.Env, "nothing else from the environment is sent")
	assert.Equal(t, "PreToolUse", tr.got.Payload.HookEventName)
}

func TestAskUserQuestionViaResident_ShouldPrintNothing_WhenThereIsNoAnswerOrAnythingFails(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		tr      *fakeTransport
		dialErr error
	}{
		{"defer", &fakeTransport{}, nil},
		{"transport error", &fakeTransport{err: errors.New("boom")}, nil},
		{"dial error", &fakeTransport{}, errors.New("no instance")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			askUserQuestionViaResident(askPayload, fakeDialer(tc.tr, tc.dialErr), envOf(nil), &out)
			assert.Zero(t, out.Len(), "the native dialog stays")
		})
	}
}

// hangingTransport never answers until its context ends.
type hangingTransport struct{ release chan struct{} }

func (h hangingTransport) Classify(context.Context, hookipc.ClassificationEnvelope) (hookipc.ClassificationReply, error) {
	<-h.release
	return hookipc.ClassificationReply{Output: answerOutput()}, nil
}

func TestAskUserQuestionViaResident_ShouldReturnEmptyAtTheCap_WhenTheServerNeverAnswers(t *testing.T) {
	t.Parallel()
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	var out bytes.Buffer
	env := envOf(map[string]string{claudehooks.HookBlockMsEnv: "50"})

	start := time.Now()
	askUserQuestionViaResident(askPayload, fakeDialer(hangingTransport{release}, nil), env, &out)

	assert.Zero(t, out.Len())
	assert.Less(t, time.Since(start), 2*time.Second, "returned at the 50 ms cap, not when the server answered")
}

func TestAskUserQuestionViaResident_ShouldNotBlockLongerThanTheDefaultWait_WhenBlockingIsOff(t *testing.T) {
	t.Parallel()
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	var out bytes.Buffer

	start := time.Now()
	askUserQuestionViaResident(askPayload, fakeDialer(hangingTransport{release}, nil), envOf(nil), &out)

	assert.Zero(t, out.Len())
	assert.Less(t, time.Since(start), 2*time.Second, "the default wait is %s", defaultAskWait)
}
