package services

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/services/auditkit"
	"github.com/tstapler/stapler-squad/server/services/flagsvc"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
)

// Dispatcher fixtures duplicated from server/services/notifyactions' tests, for
// the answer-router and hook-reply tests that stay here.

const (
	outApplied  = sessionv1.ActionOutcome_ACTION_OUTCOME_APPLIED
	outResolved = sessionv1.ActionOutcome_ACTION_OUTCOME_ALREADY_RESOLVED
	outInvalid  = sessionv1.ActionOutcome_ACTION_OUTCOME_INVALID
	outFlagOff  = sessionv1.ActionOutcome_ACTION_OUTCOME_FLAG_OFF
	outForbid   = sessionv1.ActionOutcome_ACTION_OUTCOME_FORBIDDEN
	outStale    = sessionv1.ActionOutcome_ACTION_OUTCOME_STALE
)

// fakeActionFlags resolves every point true unless listed in off.
type fakeActionFlags struct {
	off       map[string]bool
	answerOff bool
}

func (f fakeActionFlags) Inline(dp string) bool { return !f.off[dp] }
func (f fakeActionFlags) Answer(flagsvc.AnswerRoute) bool {
	return !f.answerOff
}

type recordingAudit struct {
	mu    sync.Mutex
	lines []auditkit.AuditLine
	fail  bool
}

func (a *recordingAudit) Append(l auditkit.AuditLine) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.lines = append(a.lines, l)
	return nil
}

func (a *recordingAudit) AppendBounded(_ context.Context, l auditkit.AuditLine) error {
	if a.fail {
		return auditkit.ErrAuditFailed
	}
	return a.Append(l)
}

func answerReq(key string) *sessionv1.InvokeNotificationActionRequest {
	return &sessionv1.InvokeNotificationActionRequest{
		ActionKind: sessionv1.ActionKind_ACTION_KIND_ANSWER_QUESTION, TargetKeyType: sessionv1.ActionTargetKey_ACTION_TARGET_KEY_QUESTION_ID,
		TargetKey: "q1", SessionId: "s1", IdempotencyKey: key,
		Params: &sessionv1.NotificationActionParams{Answer: "Redis"},
	}
}

// enableInline turns on the inline-action flag for the given scopes and the
// answer stages, in an isolated state dir.
func enableInline(t *testing.T, scopes ...string) {
	t.Helper()
	svctest.WithIsolatedStateDir(t)
	cfg := config.LoadConfig()
	for _, sc := range scopes {
		require.NoError(t, cfg.SetFeatureFlagScope(flagsvc.NotificationInlineActionsFlagName, sc, true))
	}
	for _, st := range []string{flagsvc.ScopeStageRender, flagsvc.ScopeStageAnswer} {
		require.NoError(t, cfg.SetFeatureFlagScope(flagsvc.AskUserQuestionStructuredFlagName, st, true))
	}
}
