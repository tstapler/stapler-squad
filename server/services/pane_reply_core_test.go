package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

func TestAnswerTarget_Admits(t *testing.T) {
	tests := []struct {
		name   string
		target AnswerTarget
		access TerminalAccess
		want   bool
	}{
		{"hidden target, hidden session", AnswerTargetHidden, TerminalReadOnly, true},
		{"hidden target, visible session", AnswerTargetHidden, TerminalReadWrite, false},
		{"visible target, visible session", AnswerTargetVisible, TerminalReadWrite, true},
		{"visible target, hidden session", AnswerTargetVisible, TerminalReadOnly, false},
		{"unset target, hidden session", 0, TerminalReadOnly, false},
		{"unset target, visible session", 0, TerminalReadWrite, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.target.admits(tt.access))
		})
	}
}

func TestPaneReplyCore_ShouldRefuseWithZeroWritesAndKeepTheQuestion_WhenTargetIsNotTheSessionKind(t *testing.T) {
	tests := []struct {
		name        string
		target      AnswerTarget
		sessionHide bool
		wantOutcome sessionv1.ReplyOutcome
		wantStage   string
	}{
		{"visible target on a hidden session", AnswerTargetVisible, true, sessionv1.ReplyOutcome_REPLY_OUTCOME_NOT_SENT, replyStageTargetMismatch},
		{"hidden target on a visible session", AnswerTargetHidden, false, sessionv1.ReplyOutcome_REPLY_OUTCOME_NOT_HIDDEN, replyStageNotHidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newReplyEnv(t)
			tgt := e.addSession("core-target", tt.sessionHide, "")
			qid := e.register(tgt.inst, colorDialog)
			in := replyInput{sessionID: tgt.inst.UUID, sessionUUID: tgt.inst.GetStableID(), questionID: qid, replyID: "core-1", digit: '2'}

			res, cache := e.fix.svc.paneReplyCore().Run(e.localCtx(), tt.target, in, tgt.inst, steerRequestFacts{})

			require.NoError(t, res.Err)
			assert.Equal(t, tt.wantOutcome, res.Outcome)
			assert.False(t, cache, "nothing was written, so a retry may try again")
			assert.Empty(t, tgt.pm.writes())
			lines := replyLines(e.auditLines())
			require.Len(t, lines, 2)
			assert.Equal(t, tt.wantStage, lines[1].Stage)
			_, still := e.fix.svc.PendingQuestions().Peek(QuestionKey{SessionUUID: tgt.inst.GetStableID(), QuestionID: qid})
			assert.True(t, still, "the released claim leaves the question pending")
		})
	}
}
