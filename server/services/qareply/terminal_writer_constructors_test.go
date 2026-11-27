package qareply

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/server/services/access"
	"github.com/tstapler/stapler-squad/server/services/questionkit"
	"github.com/tstapler/stapler-squad/session/instance"
	"github.com/tstapler/stapler-squad/session/panereply"
)

func hiddenInstance(t *testing.T, hidden bool) *instance.Instance {
	t.Helper()
	inst, err := instance.NewInstance(instance.InstanceOptions{
		Title:   "access-" + t.Name(),
		Path:    t.TempDir(),
		Program: "claude",
		Hidden:  hidden,
	})
	require.NoError(t, err)
	return inst
}

func validClaim(t *testing.T) questionkit.PendingQuestionClaim {
	t.Helper()
	s := questionkit.NewPendingQuestionStore(nil)
	s.Register(testQuestion("s1", "q1"))
	claim, res := s.Claim(questionkit.QuestionKey{SessionUUID: "s1", QuestionID: "q1"})
	require.Equal(t, questionkit.ClaimOK, res)
	return claim
}

func TestNewQuestionReplyWriter_ShouldRefuseAccessOfTheWrongKind_WhenTargetDoesNotAdmitIt(t *testing.T) {
	t.Parallel()
	claim := validClaim(t)
	inst := hiddenInstance(t, true)
	noAfter := func(time.Duration) <-chan time.Time { return nil }

	cases := []struct {
		name   string
		target AnswerTarget
		access access.TerminalAccess
		want   error
	}{
		{"hidden target, read-write access", AnswerTargetHidden, access.TerminalReadWrite, errAnswerTargetMismatch},
		{"visible target, read-only access", AnswerTargetVisible, access.TerminalReadOnly, errAnswerTargetMismatch},
		{"zero target admits nothing", AnswerTarget(0), access.TerminalReadOnly, errAnswerTargetMismatch},
		{"zero target, read-write access", AnswerTarget(0), access.TerminalReadWrite, errAnswerTargetMismatch},
	}
	for _, tc := range cases {
		w, err := newQuestionReplyWriter(tc.access, tc.target, claim, inst, noAfter)
		assert.Nil(t, w, tc.name)
		assert.ErrorIs(t, err, tc.want, tc.name)
	}

	w, err := newQuestionReplyWriter(access.TerminalReadOnly, AnswerTargetHidden, claim, inst, noAfter)
	require.NoError(t, err)
	assert.NotNil(t, w, "hidden target admits read-only access")
	w, err = newQuestionReplyWriter(access.TerminalReadWrite, AnswerTargetVisible, claim, inst, noAfter)
	require.NoError(t, err)
	assert.NotNil(t, w, "visible target admits read-write access")
}

func TestNewQuestionReplyWriter_ShouldReturnErrReadOnly_WhenClaimIsZeroOrInstanceIsNil(t *testing.T) {
	t.Parallel()
	inst := hiddenInstance(t, true)

	w, err := newQuestionReplyWriter(access.TerminalReadOnly, AnswerTargetHidden, questionkit.PendingQuestionClaim{}, inst, nil)
	assert.Nil(t, w)
	assert.ErrorIs(t, err, access.ErrReadOnly, "a never-claimed question yields no writer")

	w, err = newQuestionReplyWriter(access.TerminalReadOnly, AnswerTargetHidden, validClaim(t), nil, nil)
	assert.Nil(t, w)
	assert.ErrorIs(t, err, access.ErrReadOnly, "no instance yields no writer")
}

func testQuestion(sessionUUID, id string) questionkit.PendingQuestion {
	return questionkit.PendingQuestion{
		QuestionID: id, SessionUUID: sessionUUID,
		Dialog: panereply.QuestionDialog{Question: "Q " + id + "?", Header: "H", Labels: []string{"A", "B"}},
	}
}
