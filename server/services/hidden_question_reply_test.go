package services

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/deliverygate"
	"github.com/tstapler/stapler-squad/session"
)

const (
	replyLoopbackPeer = "127.0.0.1:50000"
	replyColorLabel   = "Green-label-sentinel"
)

// replyDialogFixture loads a verbatim capture of a real Claude Code dialog.
func replyDialogFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "session", "testdata", "reply_dialogs", name))
	require.NoError(t, err)
	return string(b)
}

// replyPM is a recording pane: it shows `dialog` until a key was written, then
// `after` (or still `dialog` when keepOpen), and notes how many audit lines
// were durable at the instant of each write.
type replyPM struct {
	session.ProcessManager
	mu       sync.Mutex
	sent     []string
	dialog   string
	after    string
	keepOpen bool
	sendErr  error
	sendN    int
	probe    func() int // audit lines visible when a write happens
	probed   []int
	gate     chan struct{} // when set, SendKeys blocks until closed
	entered  chan struct{}
	capErr   error
}

func (r *replyPM) GetSessionIdentifier() string     { return "" }
func (r *replyPM) GetPanePID() (int32, error)       { return 0, errors.New("no pane") }
func (r *replyPM) HasSession() bool                 { return true }
func (r *replyPM) IsAlive() bool                    { return true }
func (r *replyPM) HasMeaningfulContent(string) bool { return false }

func (r *replyPM) SendKeys(keys string) (int, error) {
	if r.entered != nil {
		r.entered <- struct{}{}
	}
	if r.gate != nil {
		<-r.gate
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.probe != nil {
		r.probed = append(r.probed, r.probe())
	}
	if r.sendErr != nil && r.sendN == 0 {
		return 0, r.sendErr // provably nothing reached the pane
	}
	r.sent = append(r.sent, keys)
	if r.sendErr != nil {
		return r.sendN, r.sendErr
	}
	return len(keys), nil
}

func (r *replyPM) CapturePaneContent() (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.capErr != nil {
		return "", r.capErr
	}
	if len(r.sent) > 0 && !r.keepOpen {
		return r.after, nil
	}
	return r.dialog, nil
}

func (r *replyPM) writes() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.sent...)
}

// replyEnv is a steer env plus a registered single-select question on a hidden
// session whose pane shows the real captured dialog.
type replyEnv struct {
	*steerEnv
	clock *gateTestClock
	pm    *replyPM
	inst  *session.Instance
	qid   string
}

var colorDialog = session.QuestionDialog{
	Question: "Which color should the spike use?",
	Header:   "Color",
	Labels:   []string{"Red", replyColorLabel, "Blue"},
}

func newReplyEnv(t *testing.T) *replyEnv {
	t.Helper()
	e := newSteerEnv(t)
	clock := newGateTestClock()
	e.fix.svc.replyNow = clock.Now
	e.fix.svc.replyPeerAddr = func(connect.AnyRequest) string { return replyLoopbackPeer }
	e.fix.svc.replyControllerActive = func(*session.Instance) bool { return true }
	// Poll timers fire at once; the 5s send timeout never does (nothing sleeps).
	e.fix.svc.replyAfter = func(d time.Duration) <-chan time.Time {
		if d >= time.Second {
			return nil
		}
		ch := make(chan time.Time, 1)
		ch <- time.Time{}
		return ch
	}
	e.fix.svc.replyState().flag.SetEnabled(true)

	// The fixture's own labels are Red/Green/Blue; the sentinel label proves the
	// log never carries a label, so the dialog under test is built from it.
	dialog := strings.ReplaceAll(replyDialogFixture(t, "question_single_select_3opts.txt"), "2. Green", "2. "+replyColorLabel)
	pm := &replyPM{dialog: dialog, after: "● User answered Claude's questions\n❯ \n"}
	pm.probe = func() int { return len(e.readLines()) }
	inst := session.NewStartedInstanceForTest(t, "reply-hidden", pm)
	inst.UUID = "reply-hidden-uuid"
	inst.Path = t.TempDir()
	inst.Status = session.Active
	inst.Program = "claude"
	inst.Hidden = true
	inst.CreatedAt, inst.UpdatedAt = time.Now(), time.Now()
	addInstanceToPoller(e.fix.poller, inst)

	re := &replyEnv{steerEnv: e, clock: clock, pm: pm, inst: inst}
	re.qid = re.register(inst, colorDialog)
	return re
}

func (e *replyEnv) register(inst *session.Instance, d session.QuestionDialog) string {
	id := NewQuestionID()
	e.fix.svc.PendingQuestions().Register(PendingQuestion{
		QuestionID: id, SessionUUID: inst.UUID, Dialog: d, Token: QuestionToken(d.Question),
	})
	return id
}

func (e *replyEnv) reply(sessionID, questionID, text, replyID string) (*sessionv1.ReplyToPendingQuestionResponse, error) {
	resp, err := e.fix.svc.ReplyToPendingQuestion(e.localCtx(), connect.NewRequest(&sessionv1.ReplyToPendingQuestionRequest{
		SessionId: sessionID, QuestionId: questionID, ReplyText: text, ReplyId: replyID,
	}))
	if err != nil {
		return nil, err
	}
	return resp.Msg, nil
}

func (e *replyEnv) replyOK(text, replyID string) *sessionv1.ReplyToPendingQuestionResponse {
	e.t.Helper()
	resp, err := e.reply(e.inst.UUID, e.qid, text, replyID)
	require.NoError(e.t, err)
	return resp
}

func (e *replyEnv) counter(label string) uint64 {
	return e.gate.Metrics().Value(deliverygate.CounterReply, label)
}

func replyLines(all []AuditLine) []AuditLine { return steerLines(all, auditKindReply) }

// T-RP-02, T-RP-07, T-RP-10
func TestReply_ShouldClaimAuditThenWriteOneDigitOnceAndReportSent_WhenHiddenSessionHasASingleSelectQuestionAndTheDialogCloses(t *testing.T) {
	e := newReplyEnv(t)

	resp := e.replyOK("2", "r-1")

	assert.Equal(t, sessionv1.ReplyOutcome_REPLY_OUTCOME_SENT, resp.Outcome)
	assert.Equal(t, []string{"2"}, e.pm.writes(), "exactly one one-byte write, no Enter")
	assert.Equal(t, []int{1}, e.pm.probed, "the fsynced requested line was durable when the digit was written, and the result line was not yet")
	session.AssertLeaseFree(t, e.inst)
	assert.EqualValues(t, 1, e.counter("sent"))
	_, still := e.fix.svc.PendingQuestions().Peek(QuestionKey{SessionUUID: e.inst.UUID, QuestionID: e.qid})
	assert.False(t, still, "the answered question is consumed")

	lines := replyLines(e.auditLines())
	require.Len(t, lines, 2)
	req, res := lines[0], lines[1]
	assert.Equal(t, auditPhaseRequest, req.Phase)
	assert.Equal(t, auditPhaseResult, res.Phase)
	assert.Equal(t, req.ChangeID, res.ChangeID)
	assert.Equal(t, "sent", res.Outcome)
	assert.Equal(t, replyStageDialogClosed, res.Stage)
	assert.Equal(t, 2, req.OptionIndex)
	assert.Equal(t, replyColorLabel, req.OptionLabel)
	assert.Equal(t, "Which color should the spike use", req.QuestionToken[:32])
	assert.Equal(t, e.qid, req.QuestionID)
	assert.Equal(t, "r-1", req.ReplyID)
	assert.Equal(t, e.inst.UUID, req.SessionUUID)
	assert.Equal(t, ListenerLocal, req.Listener)
	assert.Equal(t, AuthModeNone, req.AuthMode)
	assert.Equal(t, "localhost:8543", req.Host)
	assert.Empty(t, req.MessageSHA256, "a digit is brute-forceable: no hash")
	assert.Empty(t, req.MessagePreview)
}

// T-RP-03, T-RP-04, T-RP-20, T-RP-29, T-RP-28, T-RP-44, T-RP-48: each refusal is a typed
// outcome with zero terminal writes and the question left claimable.
func TestReply_ShouldReturnDistinctOutcomeWithZeroWrites_WhenFlagOffNotHiddenNoPendingStaleNotWaiting(t *testing.T) {
	e := newReplyEnv(t)

	t.Run("flag off", func(t *testing.T) {
		e.fix.svc.replyState().flag.SetEnabled(false)
		defer e.fix.svc.replyState().flag.SetEnabled(true)
		assert.Equal(t, sessionv1.ReplyOutcome_REPLY_OUTCOME_DISABLED, e.replyOK("1", "r-flag").Outcome)
		assert.Empty(t, e.pm.writes())
	})
	t.Run("no pending: unknown question and another session's question", func(t *testing.T) {
		resp, err := e.reply(e.inst.UUID, "not-a-question", "1", "r-unknown")
		require.NoError(t, err)
		assert.Equal(t, sessionv1.ReplyOutcome_REPLY_OUTCOME_NO_PENDING, resp.Outcome)

		other := e.addSession("reply-other", true, "")
		oq := e.register(other.inst, colorDialog)
		resp, err = e.reply(e.inst.UUID, oq, "1", "r-other")
		require.NoError(t, err)
		assert.Equal(t, sessionv1.ReplyOutcome_REPLY_OUTCOME_NO_PENDING, resp.Outcome)
		assert.Empty(t, e.pm.writes())
		assert.Empty(t, other.pm.writes())
	})
	t.Run("not hidden", func(t *testing.T) {
		vis := e.addSession("reply-visible", false, "")
		vq := e.register(vis.inst, colorDialog)
		resp, err := e.reply(vis.inst.UUID, vq, "1", "r-vis")
		require.NoError(t, err)
		assert.Equal(t, sessionv1.ReplyOutcome_REPLY_OUTCOME_NOT_HIDDEN, resp.Outcome)
		assert.Empty(t, vis.pm.writes())
	})
	t.Run("not waiting: no active controller, status value never consulted", func(t *testing.T) {
		e.fix.svc.replyControllerActive = func(*session.Instance) bool { return false }
		defer func() { e.fix.svc.replyControllerActive = func(*session.Instance) bool { return true } }()
		assert.Equal(t, sessionv1.ReplyOutcome_REPLY_OUTCOME_NOT_WAITING, e.replyOK("1", "r-nw").Outcome)
		assert.Empty(t, e.pm.writes())
	})
	t.Run("pane cannot be captured", func(t *testing.T) {
		e.pm.capErr = errors.New("capture failed")
		defer func() { e.pm.capErr = nil }()
		assert.Equal(t, sessionv1.ReplyOutcome_REPLY_OUTCOME_NOT_WAITING, e.replyOK("1", "r-cap").Outcome)
		assert.Empty(t, e.pm.writes())
	})
	t.Run("stale: a Bash permission dialog is on screen after the question was answered", func(t *testing.T) {
		e.pm.dialog = replyDialogFixture(t, "permission_bash.txt")
		assert.Equal(t, sessionv1.ReplyOutcome_REPLY_OUTCOME_STALE_PROMPT, e.replyOK("1", "r-stale").Outcome)
		assert.Empty(t, e.pm.writes(), "digits 1 to 4 are never written into a permission dialog")
	})
	t.Run("stale: a shell prompt", func(t *testing.T) {
		e.pm.dialog = "user@host:~$ \n"
		assert.Equal(t, sessionv1.ReplyOutcome_REPLY_OUTCOME_STALE_PROMPT, e.replyOK("1", "r-shell").Outcome)
		assert.Empty(t, e.pm.writes())
	})

	// Every refusal left the question claimable and released the lease.
	_, ok := e.fix.svc.PendingQuestions().Peek(QuestionKey{SessionUUID: e.inst.UUID, QuestionID: e.qid})
	assert.True(t, ok)
	session.AssertLeaseFree(t, e.inst)
}

// T-RP-06: one ASCII digit 1..N, never N+1 or N+2.
func TestReply_ShouldReturnInvalidArgumentWithZeroWrites_WhenReplyTextIsNotExactlyOneAsciiDigitInOneToN(t *testing.T) {
	e := newReplyEnv(t) // N = 3
	for _, text := range []string{"2 weeks", "", "0", "4", "5", "10", "２", " 2", "2\n", "\x1b", "a", "-1"} {
		_, err := e.reply(e.inst.UUID, e.qid, text, "r-"+text)
		require.Error(t, err, "%q", text)
		assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err), "%q", text)
	}
	assert.Empty(t, e.pm.writes(), "no digit above N, no non-digit, nothing is ever written")
	assert.EqualValues(t, 12, e.counter("invalid"))
}

// T-RP-05, T-RP-19, T-RP-25, T-RP-55: the same reply_id returns the first result
// and writes once; a different payload under the same id is InvalidArgument.
func TestReply_ShouldWriteOnceAndReturnFirstResult_WhenSameReplyIDSentAgainAfterATerminalResult(t *testing.T) {
	e := newReplyEnv(t)
	first := e.replyOK("2", "r-same")
	again := e.replyOK("2", "r-same")
	assert.Equal(t, sessionv1.ReplyOutcome_REPLY_OUTCOME_SENT, first.Outcome)
	assert.Equal(t, first.Outcome, again.Outcome)
	assert.Equal(t, []string{"2"}, e.pm.writes())

	_, err := e.reply(e.inst.UUID, e.qid, "3", "r-same")
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err), "the id is bound to its payload")
	assert.Equal(t, []string{"2"}, e.pm.writes())
}

// T-RP-33: a repeated reply_id while the first is in flight waits and returns
// the first result: one write, never NO_PENDING, never RATE_LIMITED.
func TestReply_ShouldMakeInFlightReplyIdWaitForTheFirstResult_WhenTwoCallersSendTheSameReplyId(t *testing.T) {
	e := newReplyEnv(t)
	e.pm.gate = make(chan struct{})
	e.pm.entered = make(chan struct{}, 1)

	results := make(chan *sessionv1.ReplyToPendingQuestionResponse, 2)
	go func() { results <- e.replyOK("1", "r-flight") }()
	<-e.pm.entered // the first caller is inside the write
	go func() { results <- e.replyOK("1", "r-flight") }()
	require.Eventually(t, func() bool { return e.fix.svc.replyState().flights.size() == 1 }, time.Second, time.Millisecond)
	close(e.pm.gate)

	r1, r2 := <-results, <-results
	assert.Equal(t, sessionv1.ReplyOutcome_REPLY_OUTCOME_SENT, r1.Outcome)
	assert.Equal(t, sessionv1.ReplyOutcome_REPLY_OUTCOME_SENT, r2.Outcome)
	assert.Equal(t, []string{"1"}, e.pm.writes())
}

// T-RP-12: two goroutines, two reply ids: exactly one claim, one write.
func TestReply_ShouldWriteExactlyOnce_WhenTwoGoroutinesReplyWithDifferentReplyIDs(t *testing.T) {
	e := newReplyEnv(t)
	var wg sync.WaitGroup
	outs := make([]sessionv1.ReplyOutcome, 2)
	start := make(chan struct{})
	for i := range outs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			outs[i] = e.replyOK("1", "r-race-"+string(rune('a'+i))).Outcome
		}(i)
	}
	close(start)
	wg.Wait()
	assert.Len(t, e.pm.writes(), 1)
	sent := 0
	for _, o := range outs {
		if o == sessionv1.ReplyOutcome_REPLY_OUTCOME_SENT {
			sent++
		}
	}
	assert.Equal(t, 1, sent, "outcomes: %v", outs)
}

// T-RP-11, T-RP-54: a second reply with a different id inside 5s is rate limited
// before any claim; the clock is injected.
func TestReply_ShouldReturnRateLimitedWithRetryAfterBeforeTakingAnyClaim_WhenSecondReplyWithinFiveSeconds(t *testing.T) {
	e := newReplyEnv(t)
	assert.Equal(t, sessionv1.ReplyOutcome_REPLY_OUTCOME_SENT, e.replyOK("1", "r-1").Outcome)

	q2 := e.register(e.inst, session.QuestionDialog{Question: "Which color should the spike use?", Header: "Color", Labels: []string{"Red", replyColorLabel, "Blue", "Teal"}})
	e.clock.Advance(time.Second)
	resp, err := e.reply(e.inst.UUID, q2, "1", "r-2")
	require.NoError(t, err)
	assert.Equal(t, sessionv1.ReplyOutcome_REPLY_OUTCOME_RATE_LIMITED, resp.Outcome)
	assert.EqualValues(t, 4, resp.RetryAfterSeconds)
	assert.Len(t, e.pm.writes(), 1)
	_, claimable := e.fix.svc.PendingQuestions().Peek(QuestionKey{SessionUUID: e.inst.UUID, QuestionID: q2})
	assert.True(t, claimable, "rate limiting happens before the claim")

	// A retry of the first id inside the window is the first result, not RATE_LIMITED.
	assert.Equal(t, sessionv1.ReplyOutcome_REPLY_OUTCOME_SENT, e.replyOK("1", "r-1").Outcome)
}

// T-RP-08: no audit, no write.
func TestReply_ShouldReturnInternalWithZeroWritesAndReleaseTheClaim_WhenPreWriteAuditAppendFails(t *testing.T) {
	e := newReplyEnv(t)
	e.mfs.failMkdir = errors.New("disk full")

	_, err := e.reply(e.inst.UUID, e.qid, "2", "r-audit")

	require.Error(t, err)
	assert.Equal(t, connect.CodeInternal, connect.CodeOf(err))
	assert.Empty(t, e.pm.writes())
	assert.EqualValues(t, 1, e.counter("audit_failed"))
	_, ok := e.fix.svc.PendingQuestions().Peek(QuestionKey{SessionUUID: e.inst.UUID, QuestionID: e.qid})
	assert.True(t, ok, "the claim was released: Retry is safe")
	session.AssertLeaseFree(t, e.inst)

	// With the sink back, the same reply_id attempts the write again.
	e.mfs.failMkdir = nil
	assert.Equal(t, sessionv1.ReplyOutcome_REPLY_OUTCOME_SENT, e.replyOK("2", "r-audit").Outcome)
	assert.Equal(t, []string{"2"}, e.pm.writes())
}

// T-RP-24, T-RP-25: a dialog that stays open is indeterminate, consumed and cached;
// a Retry writes nothing more.
func TestReply_ShouldReturnSendIndeterminateConsumeClaimAndCacheResult_WhenDialogStaysOpen(t *testing.T) {
	e := newReplyEnv(t)
	e.pm.keepOpen = true
	first := e.replyOK("1", "r-open")
	assert.Equal(t, sessionv1.ReplyOutcome_REPLY_OUTCOME_SEND_INDETERMINATE, first.Outcome)
	assert.Equal(t, []string{"1"}, e.pm.writes())
	again := e.replyOK("1", "r-open")
	assert.Equal(t, sessionv1.ReplyOutcome_REPLY_OUTCOME_SEND_INDETERMINATE, again.Outcome)
	assert.Equal(t, []string{"1"}, e.pm.writes(), "no second byte")
	res := replyLines(e.auditLines())
	require.Len(t, res, 2)
	assert.Equal(t, replyStageDialogStillOpen, res[1].Stage)
}

// T-RP-23: a write that provably wrote zero bytes is NOT_SENT and the claim is
// released; a Retry with the same reply_id attempts it again.
func TestReply_ShouldReturnNotSentAndAllowRetry_WhenTheWriteProvablyWroteNothing(t *testing.T) {
	e := newReplyEnv(t)
	e.pm.sendErr, e.pm.sendN = errors.New("pty not initialized"), 0

	first := e.replyOK("2", "r-ns")
	assert.Equal(t, sessionv1.ReplyOutcome_REPLY_OUTCOME_NOT_SENT, first.Outcome)
	_, ok := e.fix.svc.PendingQuestions().Peek(QuestionKey{SessionUUID: e.inst.UUID, QuestionID: e.qid})
	assert.True(t, ok)

	e.pm.mu.Lock()
	e.pm.sendErr = nil
	e.pm.mu.Unlock()
	assert.Equal(t, sessionv1.ReplyOutcome_REPLY_OUTCOME_SENT, e.replyOK("2", "r-ns").Outcome)
}

// T-RP-78: a busy lease is NOT_SENT with zero writes and the claim released.
func TestReply_ShouldReturnNotSentWithZeroWrites_WhenTheLeaseIsBusy(t *testing.T) {
	e := newReplyEnv(t)
	held, ok := e.inst.TryTerminalWriteLease(session.LeaseWriterDriver)
	require.True(t, ok)
	defer held.Release()

	assert.Equal(t, sessionv1.ReplyOutcome_REPLY_OUTCOME_NOT_SENT, e.replyOK("1", "r-busy").Outcome)
	assert.Empty(t, e.pm.writes())
	_, claimable := e.fix.svc.PendingQuestions().Peek(QuestionKey{SessionUUID: e.inst.UUID, QuestionID: e.qid})
	assert.True(t, claimable)
}

// T-RP-31: a question of a session that shares a path can never write into the
// other session, whichever of uuid, title or tmux name the request carries.
func TestReply_ShouldNeverWriteToTheWrongInstance_WhenTwoInstancesSharePath(t *testing.T) {
	e := newReplyEnv(t)
	vis := e.addSession("reply-sharing", false, "", func(i *session.Instance) { i.Path = e.inst.Path })
	for _, id := range []string{vis.inst.UUID, vis.inst.Title} {
		resp, err := e.reply(id, e.qid, "1", "r-share-"+id)
		require.NoError(t, err)
		assert.NotEqual(t, sessionv1.ReplyOutcome_REPLY_OUTCOME_SENT, resp.Outcome)
	}
	assert.Empty(t, vis.pm.writes())
	assert.Empty(t, e.pm.writes())

	// The hidden session's own title resolves to the same UUID and is accepted.
	resp, err := e.reply(e.inst.Title, e.qid, "1", "r-title")
	require.NoError(t, err)
	assert.Equal(t, sessionv1.ReplyOutcome_REPLY_OUTCOME_SENT, resp.Outcome)
	assert.Equal(t, []string{"1"}, e.pm.writes())
}

// T-RP-38: auth off and a remote or proxied caller is refused with PermissionDenied.
func TestReply_ShouldReturnPermissionDeniedAndCountRefused_WhenAuthModeNoneAndPeerNotLoopback(t *testing.T) {
	e := newReplyEnv(t)
	for _, peer := range []string{"192.168.1.20:50000", ""} {
		e.fix.svc.replyPeerAddr = func(connect.AnyRequest) string { return peer }
		_, err := e.reply(e.inst.UUID, e.qid, "1", "r-peer-"+peer)
		require.Error(t, err)
		assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
	}
	// A loopback peer behind a proxy header is remote too.
	e.fix.svc.replyPeerAddr = func(connect.AnyRequest) string { return replyLoopbackPeer }
	ctx := e.requestCtx(ListenerLocal, false, "localhost:8543", map[string]string{"X-Forwarded-For": "203.0.113.9"})
	_, err := e.fix.svc.ReplyToPendingQuestion(ctx, connect.NewRequest(&sessionv1.ReplyToPendingQuestionRequest{
		SessionId: e.inst.UUID, QuestionId: e.qid, ReplyText: "1", ReplyId: "r-proxy",
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
	assert.EqualValues(t, 3, e.counter("refused"))
	assert.Empty(t, e.pm.writes())
}

// T-RP-21: the guards flag has no effect on Reply in either state.
func TestReply_ShouldIgnoreTheGuardsFlag_WhenItIsOffAndWhenOn(t *testing.T) {
	e := newReplyEnv(t)
	e.fix.svc.guards = &UnaryGuardsFlag{}
	e.fix.svc.guards.SetEnabled(false)
	assert.Equal(t, sessionv1.ReplyOutcome_REPLY_OUTCOME_SENT, e.replyOK("1", "r-g-off").Outcome)
}

// T-RP-36: no option label or question text reaches the INFO log; the audit file
// carries the label.
func TestReply_ShouldPutNoOptionLabelInTheLogLine_WhenReplySent(t *testing.T) {
	e := newReplyEnv(t)
	logs := captureInfoLog()

	assert.Equal(t, sessionv1.ReplyOutcome_REPLY_OUTCOME_SENT, e.replyOK("2", "r-log").Outcome)

	out := logs()
	assert.NotContains(t, out, replyColorLabel)
	assert.Contains(t, out, "hidden_session_reply")
	lines := replyLines(e.auditLines())
	assert.Equal(t, replyColorLabel, lines[0].OptionLabel)
}

// Task 5.6c: the handler is a no-op for context cancel before anything started.
func TestReply_ShouldWaitNoLonger_WhenFollowerContextIsCancelled(t *testing.T) {
	e := newReplyEnv(t)
	e.pm.gate = make(chan struct{})
	e.pm.entered = make(chan struct{}, 1)
	go func() { _ = func() *sessionv1.ReplyToPendingQuestionResponse { return e.replyOK("1", "r-cancel") }() }()
	<-e.pm.entered

	ctx, cancel := context.WithCancel(e.localCtx())
	cancel()
	_, err := e.fix.svc.ReplyToPendingQuestion(ctx, connect.NewRequest(&sessionv1.ReplyToPendingQuestionRequest{
		SessionId: e.inst.UUID, QuestionId: e.qid, ReplyText: "1", ReplyId: "r-cancel",
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeCanceled, connect.CodeOf(err))
	close(e.pm.gate)
}
