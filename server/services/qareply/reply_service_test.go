package qareply

import (
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/services/access"
	"github.com/tstapler/stapler-squad/server/services/questionkit"
	"github.com/tstapler/stapler-squad/session/panereply"
)

// With no PeerAddr port the router installs the in-process peer: without it the
// hidden reply handler would refuse the router's own request, which has no peer.
func TestNewAnswerRouter_ShouldInstallTheInProcessPeer_WhenNoPeerReaderIsGiven(t *testing.T) {
	e := newBareEnv(t)
	p := e.ports()
	p.PeerAddr = nil
	e.svc = New(p)
	assert.Nil(t, e.svc.peerAddr)

	e.svc.NewAnswerRouter(AnswerRouterDeps{}, fakeActionFlags{off: map[string]bool{}})

	require.NotNil(t, e.svc.peerAddr)
	assert.Equal(t, inProcessPeerAddr, e.svc.peerAddr(connect.NewRequest(&sessionv1.ReplyToPendingQuestionRequest{})))
}

func TestNewAnswerRouter_ShouldKeepTheConfiguredPeerReader(t *testing.T) {
	e := newBareEnv(t)
	e.svc = New(e.ports())

	e.svc.NewAnswerRouter(AnswerRouterDeps{}, fakeActionFlags{off: map[string]bool{}})

	assert.Equal(t, replyLoopbackPeer, e.svc.peerAddr(nil))
}

// SessionDeleted drops the session's rate-limit state with its questions.
func TestSessionDeleted_ShouldForgetTheSessionsLimiterAndQuestions(t *testing.T) {
	e := newReplyEnv(t)
	assert.Equal(t, sessionv1.ReplyOutcome_REPLY_OUTCOME_SENT, e.replyOK("1", "r-before").Outcome)
	require.Len(t, e.svc.limiters.session, 1)
	left := e.register(e.inst, colorDialog)

	e.svc.SessionDeleted(e.inst.UUID)

	assert.Empty(t, e.svc.limiters.session)
	_, ok := e.svc.PendingQuestions().Peek(questionkit.QuestionKey{SessionUUID: e.inst.UUID, QuestionID: left})
	assert.False(t, ok)
}

// The After port is the timer of the send and of the closed-check polls.
func TestReply_ShouldUseTheAfterPort(t *testing.T) {
	e := newBareEnv(t)
	p := e.ports()
	var timers atomic.Int32
	after := p.After
	p.After = func(d time.Duration) <-chan time.Time {
		timers.Add(1)
		return after(d)
	}
	e.svc = New(p)
	e.svc.flag.SetEnabled(true)
	tgt := e.addSession("after-port", true)
	tgt.pm.dialog, tgt.pm.after = replyDialogFixture(t, "question_single_select_3opts.txt"), closedComposer
	e.inst = tgt.inst
	e.qid = e.register(tgt.inst, panereply.QuestionDialog{Question: colorDialog.Question, Header: "Color", Labels: []string{"Red", "Green", "Blue"}})

	assert.Equal(t, sessionv1.ReplyOutcome_REPLY_OUTCOME_SENT, e.replyOK("1", "r-after").Outcome)

	assert.Positive(t, timers.Load())
}

// The router's own guard, not a test's: input recorded for the pane on the
// process-wide activity record refuses the write.
func TestNewAnswerRouter_ShouldRefuseToType_WhileAHumanIsActiveInThePane(t *testing.T) {
	e := newRouterEnv(t)
	e.router = e.svc.NewAnswerRouter(AnswerRouterDeps{Rows: e.storage}, e.flags)
	inst, pm := e.addVisible("default-guard")
	qid := e.hookRow(inst, colorDialog)
	access.DefaultHumanActivity().Record("ssq_default-guard")

	res := e.answer(inst, qid, replyColorLabel)

	assert.Equal(t, sessionv1.ActionOutcome_ACTION_OUTCOME_NOT_SENT, res.Outcome)
	assert.Equal(t, detailHumanActive, res.Detail)
	assert.Empty(t, pm.writes())
}
