package questionkit

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/panereply"
)

// The members below exist because staying callers cross the package boundary.

func TestForgetPending_should_DropTheEntryFromTheMirrorsPendingStore(t *testing.T) {
	t.Parallel()
	pending := NewPendingQuestionStore(nil)
	key := QuestionKey{SessionUUID: "s", QuestionID: "q"}
	pending.Register(PendingQuestion{QuestionID: "q", SessionUUID: "s", Dialog: panereply.QuestionDialog{Question: "Q?", Header: "H", Labels: []string{"A"}}})
	m := NewGuidanceHookMirror(nil, pending, nil, nil, func() bool { return true })

	m.ForgetPending(key)

	_, ok := pending.Peek(key)
	assert.False(t, ok)
}

func TestNewGuidanceHookMirror_should_ReadTheEnabledPortOnEveryCall(t *testing.T) {
	t.Parallel()
	on := false
	m := NewGuidanceHookMirror(nil, nil, nil, nil, func() bool { return on })
	assert.False(t, m.Enabled())
	on = true
	assert.True(t, m.Enabled())
	assert.True(t, NewGuidanceHookMirror(nil, nil, nil, nil, nil).Enabled(), "a nil port is the legacy always-on gate")
}

func TestMirror_should_WriteNothing_WhenThePortSaysOff(t *testing.T) {
	t.Parallel()
	m := NewGuidanceHookMirror(nil, nil, nil, nil, func() bool { return false })
	id, ok := m.Mirror(HookMirrorRequest{SessionUUID: "s"})
	assert.Empty(t, id)
	assert.False(t, ok)
}

func TestSetGuidanceMirrorFields_should_SendNoQuestions_WhenQuestionsJSONDoesNotParse(t *testing.T) {
	t.Parallel()
	out := &sessionv1.GuidanceRequest{}
	SetGuidanceMirrorFields(out, &domain.GuidanceRequestData{ID: "x", QuestionsJSON: "{nope", Source: domain.QuestionSourceHook, DeliveryAttempts: 2})
	assert.Empty(t, out.Questions)
	assert.Equal(t, string(domain.QuestionSourceHook), out.Source)
	assert.Equal(t, int32(2), out.DeliveryAttempts)
	assert.Nil(t, out.ExpiresAt)
}

func TestActionResultHelpers_should_CarryTheirOutcomeAndDetail(t *testing.T) {
	t.Parallel()
	assert.Equal(t, sessionv1.ActionOutcome_ACTION_OUTCOME_APPLIED, Applied().Outcome)
	r := Outcome(sessionv1.ActionOutcome_ACTION_OUTCOME_STALE, "why")
	assert.Equal(t, "why", r.Detail)
	assert.Equal(t, sessionv1.ActionOutcome_ACTION_OUTCOME_STALE, r.Outcome)
	nw := NotWired()
	assert.Equal(t, sessionv1.ActionOutcome_ACTION_OUTCOME_INVALID, nw.Outcome)
	assert.Equal(t, "this action is not available on this server", nw.Detail)
}

func TestMetaKeys_should_KeepTheirWireValues(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "question_id", MetaQuestionID)
	assert.Equal(t, "guidance_request_id", MetaGuidanceRequestID)
	assert.Equal(t, "reply_unavailable", MetaReplyUnavailable)
}

func TestSanitizeNotificationText_should_ReplaceControlCharactersWithSpaces(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "a b c d", SanitizeNotificationText("a\nb\rc\x01d"))
	assert.Equal(t, "ok 🎉", SanitizeNotificationText("ok 🎉"))
}

func TestTabbedQuestions_should_CopyHeaderTextLabelsAndMultiSelect(t *testing.T) {
	t.Parallel()
	p, err := domain.NewQuestionPayload([]domain.Question{
		{Text: "One?", Header: "One", Options: []domain.Option{{Label: "A"}, {Label: "B"}}},
		{Text: "Two?", Header: "Two", MultiSelect: true, Options: []domain.Option{{Label: "X"}}},
	})
	require.NoError(t, err)
	got := tabbedQuestions(p)
	require.Len(t, got, 2)
	assert.Equal(t, panereply.TabbedQuestion{Text: "One?", Header: "One", Labels: []string{"A", "B"}}, got[0])
	assert.Equal(t, panereply.TabbedQuestion{Text: "Two?", Header: "Two", Labels: []string{"X"}, MultiSelect: true}, got[1])
}

func TestSetBound_should_ReplaceTheWriteBound(t *testing.T) {
	t.Parallel()
	m := NewGuidanceHookMirror(nil, nil, nil, nil, nil)
	assert.Equal(t, guidanceMirrorBound, m.bound)
	m.SetBound(time.Second)
	assert.Equal(t, time.Second, m.bound)
}

func TestObserveScheduleWakeup_should_NotPanic_WhenNoLookupIsWired(t *testing.T) {
	t.Parallel()
	ObserveScheduleWakeup(context.Background(), ScheduleWakeupRoles{}, "sess")
	RecordAnswerUnconfirmed()
}
