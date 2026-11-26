package services

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/services/questionkit"
	"github.com/tstapler/stapler-squad/session/instance"
)

func (e *replyEnv) assertRefusedAsArchived(resp *sessionv1.ReplyToPendingQuestionResponse) {
	e.t.Helper()
	assert.Equal(e.t, sessionv1.ReplyOutcome_REPLY_OUTCOME_NO_PENDING, resp.Outcome)
	assert.Empty(e.t, e.pm.writes(), "no keystroke is written to an archived session")
	_, ok := e.fix.svc.PendingQuestions().Peek(questionkit.QuestionKey{SessionUUID: e.inst.UUID, QuestionID: e.qid})
	assert.True(e.t, ok, "the question is not consumed")
	instance.AssertLeaseFree(e.t, e.inst)
}

func TestReply_ShouldRefuseWithZeroWrites_WhenSessionIsArchived(t *testing.T) {
	e := newReplyEnv(t)
	now := time.Now()
	e.inst.SetArchivedAt(&now)

	e.assertRefusedAsArchived(e.replyOK("1", "r-archived"))
	assert.Empty(t, replyLines(e.auditLines()), "refused before any attempt starts")
}

// Archival can land after the request was admitted; the last check before the write sees it.
func TestReply_ShouldRefuseWithZeroWrites_WhenSessionIsArchivedWhileTheReplyIsInFlight(t *testing.T) {
	e := newReplyEnv(t)
	e.fix.svc.replyControllerActive = func(inst *instance.Instance) bool {
		now := time.Now()
		inst.SetArchivedAt(&now)
		return true
	}

	resp := e.replyOK("1", "r-archived-late")

	require.True(t, e.inst.IsArchived())
	e.assertRefusedAsArchived(resp)
}
