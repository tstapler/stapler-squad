package services

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/tmux"
)

// recordingAcks is a deliverySink that remembers what was sent.
type recordingAcks struct {
	mu   sync.Mutex
	sent []SentDelivery
}

func (r *recordingAcks) Sent(d SentDelivery) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sent = append(r.sent, d)
}

func (r *recordingAcks) all() []SentDelivery {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]SentDelivery(nil), r.sent...)
}

// routerEnv is a reply env plus a router over real storage, a fake clock for
// the human_active guard and a recording acknowledgement sink.
type routerEnv struct {
	*replyEnv
	router   *AnswerRouter
	acks     *recordingAcks
	activity *HumanActivity
	clients  []tmux.ClientActivity
	flags    *fakeActionFlags
}

func newRouterEnv(t *testing.T) *routerEnv {
	t.Helper()
	e := &routerEnv{replyEnv: newReplyEnv(t), acks: &recordingAcks{}, flags: &fakeActionFlags{off: map[string]bool{}}}
	e.activity = NewHumanActivity(e.clock.Now)
	e.router = e.fix.svc.WireAnswerRouter(AnswerRouterDeps{Rows: e.fix.storage, Acks: e.acks})
	e.router.flags = e.flags
	e.router.guard = NewHumanActiveGuard(e.activity, func(context.Context, string) ([]tmux.ClientActivity, error) {
		return e.clients, nil
	})
	return e
}

// addVisible registers a visible session whose pane shows the colour dialog.
func (e *routerEnv) addVisible(title string, tweak ...func(*session.Instance)) (*session.Instance, *replyPM) {
	e.t.Helper()
	dialog := strings.ReplaceAll(replyDialogFixture(e.t, "question_single_select_3opts.txt"), "2. Green", "2. "+replyColorLabel)
	pm := &replyPM{dialog: dialog, after: "● User answered Claude's questions\n❯ \n"}
	inst := session.NewStartedInstanceForTest(e.t, title, namedPM{replyPM: pm, name: "ssq_" + title})
	inst.UUID = title + "-uuid"
	inst.Path = e.t.TempDir()
	inst.Status = session.Active
	inst.Program = "claude"
	inst.CreatedAt, inst.UpdatedAt = time.Now(), time.Now()
	for _, f := range tweak {
		f(inst)
	}
	addInstanceToPoller(e.fix.poller, inst)
	return inst, pm
}

// hookRow stores a hook-mirrored row for dialog d and registers it as pending.
func (e *routerEnv) hookRow(inst *session.Instance, d session.QuestionDialog) string {
	e.t.Helper()
	opts := make([]domain.Option, len(d.Labels))
	for i, l := range d.Labels {
		opts[i] = domain.Option{Label: l}
	}
	payload, err := domain.NewQuestionPayload([]domain.Question{{Text: d.Question, Header: d.Header, Options: opts}})
	require.NoError(e.t, err)
	gr, err := domain.NewGuidanceRequest(domain.GuidanceRequestParams{
		Scope: domain.RequestScopeSession, SessionUUID: inst.UUID, Source: domain.QuestionSourceHook,
		AskerSessionUUID: inst.UUID, Payload: payload, HookCallID: domain.HookCallID("toolu_" + uuid.NewString()),
		DeliveryRoute: domain.DeliveryRouteLiveKeystroke,
	})
	require.NoError(e.t, err)
	row, err := e.fix.storage.CreateMirroredGuidanceRequest(context.Background(), gr, 0)
	require.NoError(e.t, err)
	e.fix.svc.PendingQuestions().Register(PendingQuestion{
		QuestionID: row.ID, SessionUUID: inst.UUID, Dialog: d, Token: QuestionToken(d.Question),
	})
	return row.ID
}

// agentRow stores an agent-created session-scope question.
func (e *routerEnv) agentRow(inst *session.Instance) string {
	e.t.Helper()
	row, err := e.fix.storage.CreateGuidanceRequest(context.Background(), session.CreateGuidanceRequestInput{
		Scope: domain.RequestScopeSession, SessionUUID: inst.UUID, QuestionText: "Which cache?",
		QuestionType: domain.QuestionTypeMultipleChoice, Options: `["Redis","Memcached"]`,
	})
	require.NoError(e.t, err)
	return row.ID
}

func (e *routerEnv) row(id string) *session.GuidanceRequestData {
	e.t.Helper()
	row, err := e.fix.storage.GetGuidanceRequest(context.Background(), uuid.MustParse(id))
	require.NoError(e.t, err)
	return row
}

func (e *routerEnv) answer(inst *session.Instance, qid, label string) ActionResult {
	e.t.Helper()
	res, err := e.router.ReplyToQuestion(e.localCtx(), QuestionAnswer{SessionID: inst.UUID, QuestionID: qid, OptionLabel: label, ReplyID: "r-" + uuid.NewString()})
	require.NoError(e.t, err)
	return res
}

func TestAnswerRouter_ShouldWriteOneDigitAndTrackDelivery_WhenVisibleSessionShowsTheDialog(t *testing.T) {
	e := newRouterEnv(t)
	inst, pm := e.addVisible("visible-ok")
	qid := e.hookRow(inst, colorDialog)

	res := e.answer(inst, qid, replyColorLabel)

	assert.Equal(t, outApplied, res.Outcome)
	assert.Equal(t, []string{"2"}, pm.writes(), "exactly one byte, no Enter")
	row := e.row(qid)
	assert.Equal(t, domain.DeliveryStateSent, row.DeliveryState)
	assert.Equal(t, domain.DeliveryRouteLiveKeystroke, row.DeliveryRoute)
	assert.Equal(t, 1, row.DeliveryAttempts)
	assert.Equal(t, replyColorLabel, row.Answer)
	require.NotNil(t, row.AnsweredAt)
	sent := e.acks.all()
	require.Len(t, sent, 1)
	assert.Equal(t, qid, sent[0].RowID)
	assert.False(t, sent[0].Indeterminate)
	lines := replyLines(e.auditLines())
	require.Len(t, lines, 2, "the visible write is audited like the hidden one")
}

func TestAnswerRouter_ShouldReturnFlagOffWithZeroWrites_WhenAnswerStageIsOffForVisibleSession(t *testing.T) {
	e := newRouterEnv(t)
	inst, pm := e.addVisible("visible-off")
	qid := e.hookRow(inst, colorDialog)
	e.flags.answerOff = true

	res := e.answer(inst, qid, replyColorLabel)

	assert.Equal(t, outFlagOff, res.Outcome)
	assert.Empty(t, pm.writes())
	assert.Empty(t, e.row(qid).DeliveryState, "no state moved")
}

func TestAnswerRouter_ShouldRefuseWithHumanActive_WhenWebTerminalTypedWithinFiveSeconds(t *testing.T) {
	e := newRouterEnv(t)
	inst, pm := e.addVisible("visible-typing")
	qid := e.hookRow(inst, colorDialog)
	e.activity.Record("ssq_visible-typing")
	e.clock.Advance(2 * time.Second)
	refusalsBefore := HumanActiveRefusals()

	res := e.answer(inst, qid, replyColorLabel)

	assert.Equal(t, sessionv1.ActionOutcome_ACTION_OUTCOME_NOT_SENT, res.Outcome)
	assert.Equal(t, "human_active", res.Detail)
	assert.Empty(t, pm.writes(), "zero bytes written")
	assert.Empty(t, e.row(qid).DeliveryState, "a refusal before any attempt moves no state")
	assert.Equal(t, refusalsBefore+1, HumanActiveRefusals(), "human_active_refusal_total counted the refusal once")

	e.clock.Advance(4 * time.Second)
	assert.Equal(t, outApplied, e.answer(inst, qid, replyColorLabel).Outcome, "6 s after the last input the answer proceeds")
	assert.Equal(t, []string{"2"}, pm.writes())
}

func TestAnswerRouter_ShouldRefuseWithHumanActive_WhenAHumanIsAttachedAndActive(t *testing.T) {
	e := newRouterEnv(t)
	inst, pm := e.addVisible("visible-attached")
	qid := e.hookRow(inst, colorDialog)

	e.clients = []tmux.ClientActivity{{Session: "ssq_visible-attached", ControlMode: true, Activity: e.clock.Now().Add(-time.Second)}}
	probe := e.answer(inst, qid, "not an option")
	assert.Equal(t, outInvalid, probe.Outcome, "a control-mode client alone does not block; the bad label is what is refused")

	e.clients = append(e.clients, tmux.ClientActivity{Session: "ssq_visible-attached", Activity: e.clock.Now().Add(-2 * time.Second)})
	res := e.answer(inst, qid, replyColorLabel)

	assert.Equal(t, sessionv1.ActionOutcome_ACTION_OUTCOME_NOT_SENT, res.Outcome)
	assert.Equal(t, "human_active", res.Detail)
	assert.Empty(t, pm.writes())
}

func TestAnswerRouter_ShouldMarkDeliveryFailedAndAllowRetry_WhenNothingWasWritten(t *testing.T) {
	e := newRouterEnv(t)
	inst, pm := e.addVisible("visible-retry")
	qid := e.hookRow(inst, colorDialog)
	pm.sendErr = assert.AnError

	first := e.answer(inst, qid, replyColorLabel)

	assert.Equal(t, sessionv1.ActionOutcome_ACTION_OUTCOME_NOT_SENT, first.Outcome)
	assert.Equal(t, domain.DeliveryStateFailed, e.row(qid).DeliveryState)
	assert.Empty(t, e.acks.all(), "nothing sent, nothing to acknowledge")

	pm.sendErr = nil
	e.clock.Advance(10 * time.Second) // past the per-session reply interval
	second := e.answer(inst, qid, replyColorLabel)

	assert.Equal(t, outApplied, second.Outcome)
	row := e.row(qid)
	assert.Equal(t, domain.DeliveryStateSent, row.DeliveryState, "failed -> pending -> sent")
	assert.Equal(t, 2, row.DeliveryAttempts)
}

func TestAnswerRouter_ShouldTrackAnIndeterminateWriteAsUnconfirmed_WhenTheDialogStaysOpen(t *testing.T) {
	e := newRouterEnv(t)
	inst, pm := e.addVisible("visible-indet")
	qid := e.hookRow(inst, colorDialog)
	pm.keepOpen = true

	res := e.answer(inst, qid, replyColorLabel)

	assert.Equal(t, sessionv1.ActionOutcome_ACTION_OUTCOME_SEND_INDETERMINATE, res.Outcome)
	assert.Equal(t, domain.DeliveryStateSent, e.row(qid).DeliveryState)
	sent := e.acks.all()
	require.Len(t, sent, 1)
	assert.True(t, sent[0].Indeterminate)
}

func TestAnswerRouter_ShouldMarkTheRowStale_WhenTheDialogChanged(t *testing.T) {
	e := newRouterEnv(t)
	inst, pm := e.addVisible("visible-stale")
	qid := e.hookRow(inst, colorDialog)
	pm.dialog = "unrelated pane content\n❯ \n"

	res := e.answer(inst, qid, replyColorLabel)

	assert.Equal(t, outStale, res.Outcome)
	assert.Equal(t, "dialog_replaced", res.StaleReason)
	assert.Empty(t, pm.writes())
	assert.Equal(t, domain.DeliveryStateStale, e.row(qid).DeliveryState)
}

func TestAnswerRouter_ShouldDelegateToTheHiddenHandler_WhenSessionIsHidden(t *testing.T) {
	e := newRouterEnv(t)
	qid := e.hookRow(e.inst, colorDialog)

	res := e.answer(e.inst, qid, replyColorLabel)

	assert.Equal(t, outApplied, res.Outcome)
	assert.Equal(t, []string{"2"}, e.pm.writes())
	assert.Equal(t, domain.DeliveryStateSent, e.row(qid).DeliveryState)
}

func TestAnswerRouter_ShouldReportTheRouteOfEachSessionState(t *testing.T) {
	e := newRouterEnv(t)
	visible, _ := e.addVisible("rt-visible")
	stopped, _ := e.addVisible("rt-stopped", func(i *session.Instance) { i.Status = session.Stopped })
	archived, _ := e.addVisible("rt-archived", func(i *session.Instance) {
		now := time.Now()
		i.ArchivedAt = &now
	})

	tests := []struct {
		name      string
		sessionID string
		want      answerRoute
		known     bool
	}{
		{"hidden live", e.inst.UUID, routeLiveKeystrokeHidden, true},
		{"visible live", visible.UUID, routeLiveKeystrokeVisible, true},
		{"stopped", stopped.UUID, routeDurableRecord, true},
		{"archived", archived.UUID, routeDurableRecord, true},
		{"unknown", "nope", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, known := e.router.RouteFor(tt.sessionID)
			assert.Equal(t, tt.known, known)
			if known {
				assert.Equal(t, tt.want, got)
			}
		})
	}
	res, err := e.router.ReplyToQuestion(e.localCtx(), QuestionAnswer{SessionID: "nope", QuestionID: "q", OptionLabel: "x", ReplyID: "r"})
	require.NoError(t, err)
	assert.Equal(t, outStale, res.Outcome)
	assert.Equal(t, "session_ended", res.StaleReason)
}

// lifecycleUntouched asserts an answer neither revived nor wrote to the session.
func lifecycleUntouched(t *testing.T, inst *session.Instance, pm *replyPM, status session.Status, archived bool) {
	t.Helper()
	assert.Equal(t, status, inst.Snapshot().Status, "no Start, RecoverFromStopped, restart or transition to Active")
	assert.Equal(t, archived, inst.IsArchived())
	assert.Empty(t, pm.writes(), "no pane write")
}

func TestAnswerRouter_ShouldOnlyStoreTheAnswer_WhenSessionIsStopped(t *testing.T) {
	e := newRouterEnv(t)
	inst, pm := e.addVisible("stopped-store", func(i *session.Instance) { i.Status = session.Stopped })
	qid := e.agentRow(inst)

	res := e.answer(inst, qid, "Redis")

	assert.Equal(t, outApplied, res.Outcome)
	row := e.row(qid)
	assert.Equal(t, "Redis", row.Answer)
	require.NotNil(t, row.AnsweredAt)
	assert.Equal(t, domain.DeliveryStateSavedForLater, row.DeliveryState)
	assert.Equal(t, domain.DeliveryRouteResume, row.DeliveryRoute)
	lifecycleUntouched(t, inst, pm, session.Stopped, false)
	assert.Empty(t, e.acks.all())
}

func TestAnswerRouter_ShouldOnlyStoreTheAnswer_WhenSessionIsArchived(t *testing.T) {
	e := newRouterEnv(t)
	inst, pm := e.addVisible("archived-store", func(i *session.Instance) {
		now := time.Now()
		i.ArchivedAt = &now
		i.Status = session.Stopped
	})
	qid := e.agentRow(inst)

	res := e.answer(inst, qid, "Memcached")

	assert.Equal(t, outApplied, res.Outcome)
	row := e.row(qid)
	assert.Equal(t, "Memcached", row.Answer)
	require.NotNil(t, row.AnsweredAt)
	assert.Equal(t, domain.DeliveryStateSavedForLater, row.DeliveryState)
	assert.Equal(t, domain.DeliveryRouteReadOnly, row.DeliveryRoute)
	lifecycleUntouched(t, inst, pm, session.Stopped, true)
}

func TestAnswerRouter_ShouldReportAlreadyResolvedWithTheStoredValue_WhenAnsweredTwice(t *testing.T) {
	e := newRouterEnv(t)
	inst, _ := e.addVisible("stopped-twice", func(i *session.Instance) { i.Status = session.Stopped })
	qid := e.agentRow(inst)
	require.Equal(t, outApplied, e.answer(inst, qid, "Redis").Outcome)

	second := e.answer(inst, qid, "Memcached")

	assert.Equal(t, outResolved, second.Outcome)
	assert.Equal(t, "Redis", second.Detail, "the value that won")
	assert.Equal(t, "Redis", e.row(qid).Answer)
}

func TestAnswerRouter_ShouldReportSessionEnded_WhenTheReaperCancelledAHookRow(t *testing.T) {
	e := newRouterEnv(t)
	inst, pm := e.addVisible("stopped-hook", func(i *session.Instance) { i.Status = session.Stopped })
	qid := e.hookRow(inst, colorDialog)
	_, err := e.fix.storage.CancelGuidanceRequest(context.Background(), uuid.MustParse(qid), domain.CancelReasonSessionEnded.String())
	require.NoError(t, err)

	res := e.answer(inst, qid, replyColorLabel)

	assert.Equal(t, outStale, res.Outcome)
	assert.Equal(t, "session_ended", res.StaleReason)
	lifecycleUntouched(t, inst, pm, session.Stopped, false)
}

func TestAnswerRouter_ShouldRefuseTheDurableRoute_WhenStageIsOff(t *testing.T) {
	e := newRouterEnv(t)
	inst, _ := e.addVisible("stopped-off", func(i *session.Instance) { i.Status = session.Stopped })
	qid := e.agentRow(inst)
	e.flags.answerOff = true

	res := e.answer(inst, qid, "Redis")

	assert.Equal(t, outFlagOff, res.Outcome)
	assert.Nil(t, e.row(qid).AnsweredAt)
}
