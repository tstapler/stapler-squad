package services

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
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
)

// paneScript is a scripted pane: one capture per call, the last one repeating.
type paneScript struct {
	mu     sync.Mutex
	frames []string
	calls  int
}

func (p *paneScript) capture(context.Context, string) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	i := p.calls
	if i >= len(p.frames) {
		i = len(p.frames) - 1
	}
	p.calls++
	return p.frames[i], nil
}

func (p *paneScript) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

func idleFrame(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "session", "testdata", "dialog_presence", "idle_composer.txt"))
	require.NoError(t, err)
	return string(b)
}

// ownFrame is the fixture dialog whose option 2 carries the sentinel label.
func ownFrame(t *testing.T) string {
	t.Helper()
	return strings.ReplaceAll(replyDialogFixture(t, "question_single_select_3opts.txt"), "2. Green", "2. "+replyColorLabel)
}

func otherFrame(t *testing.T) string { return replyDialogFixture(t, "question_wrapped_2opts.txt") }

// samplerEnv is a sampler over real storage with a fake clock. Rows are created
// "now", so advancing the clock ages them.
type samplerEnv struct {
	t       *testing.T
	clock   *manualClock
	storage *session.Storage
	mirror  *GuidanceHookMirror
	counts  *mirrorCounts
	sampler *DialogGoneSampler
	pane    *paneScript
}

func newSamplerEnv(t *testing.T, frames ...string) *samplerEnv {
	t.Helper()
	storage := createTestStorage(t)
	counts := &mirrorCounts{}
	e := &samplerEnv{t: t, clock: newManualClock(), storage: storage, counts: counts, pane: &paneScript{frames: frames}}
	e.mirror = newRealStoreMirror(storage, NewPendingQuestionStore(nil), counts)
	e.sampler = NewDialogGoneSampler(storage, e.pane.capture, e.mirror, e.clock.Now)
	return e
}

// colorRow mirrors the fixture question as a replyable row created at the fake "now".
func (e *samplerEnv) colorRow(callID string) string {
	e.t.Helper()
	asked := ParseAskUserQuestion(map[string]interface{}{"questions": []interface{}{map[string]interface{}{
		"question": colorDialog.Question, "header": colorDialog.Header, "multiSelect": false,
		"options": []interface{}{map[string]interface{}{"label": "Red"}, map[string]interface{}{"label": replyColorLabel}, map[string]interface{}{"label": "Blue"}},
	}}})
	id, ok := e.mirror.Mirror(HookMirrorRequest{SessionUUID: mirrorSession, HookCallID: callID, Asked: asked, Replyable: true})
	require.True(e.t, ok)
	return id
}

func (e *samplerEnv) row(id string) *session.GuidanceRequestData {
	e.t.Helper()
	row, err := e.storage.GetGuidanceRequest(context.Background(), uuid.MustParse(id))
	require.NoError(e.t, err)
	return row
}

// tick moves the clock by d relative to the real row creation time, then ticks.
func (e *samplerEnv) tickAt(id string, age time.Duration) {
	e.t.Helper()
	e.clock.t = e.row(id).CreatedAt.Add(age)
	e.sampler.Tick(context.Background())
}

func TestDialogGoneSampler_should_CancelDialogGone_OnTwoConsecutiveAbsentSamples(t *testing.T) {
	t.Parallel()
	e := newSamplerEnv(t, idleFrame(t))
	id := e.colorRow("toolu_1")

	e.tickAt(id, 3*time.Second)
	assert.Nil(t, e.row(id).CancelledAt, "one sample proves nothing")
	e.tickAt(id, 6*time.Second)

	row := e.row(id)
	require.NotNil(t, row.CancelledAt)
	assert.Equal(t, domain.CancelReasonDialogGone, row.CancelReason)
	assert.Equal(t, []string{"dialog_gone"}, e.counts.cleared)
}

func TestDialogGoneSampler_should_CancelDialogReplaced_WhenAnotherDialogShowsTwice(t *testing.T) {
	t.Parallel()
	e := newSamplerEnv(t, otherFrame(t))
	id := e.colorRow("toolu_1")

	e.tickAt(id, 3*time.Second)
	e.tickAt(id, 6*time.Second)

	assert.Equal(t, domain.CancelReasonDialogReplaced, e.row(id).CancelReason)
}

func TestDialogGoneSampler_should_KeepTheRowOpen_WhenOneMismatchIsFollowedByAMatch(t *testing.T) {
	t.Parallel()
	e := newSamplerEnv(t, idleFrame(t), ownFrame(t), idleFrame(t), ownFrame(t))
	id := e.colorRow("toolu_1")

	for i := 1; i <= 4; i++ {
		e.tickAt(id, time.Duration(3*i)*time.Second)
	}

	assert.Nil(t, e.row(id).CancelledAt, "the streak resets on a match")
	assert.Equal(t, 4, e.pane.count())
}

func TestDialogGoneSampler_should_UseThreeSecondThenThirtySecondCadence(t *testing.T) {
	t.Parallel()
	e := newSamplerEnv(t, ownFrame(t))
	id := e.colorRow("toolu_1")

	e.tickAt(id, 1*time.Second)
	e.tickAt(id, 2*time.Second) // inside 3 s of the first sample: skipped
	e.tickAt(id, 4*time.Second)
	assert.Equal(t, 2, e.pane.count(), "3 s cadence in the first minute")

	e.tickAt(id, 61*time.Second)
	before := e.pane.count()
	e.tickAt(id, 80*time.Second) // 19 s after the last sample: skipped
	assert.Equal(t, before, e.pane.count(), "30 s cadence after the first minute")
	e.tickAt(id, 95*time.Second)
	assert.Equal(t, before+1, e.pane.count())
}

func TestDialogGoneSampler_should_StopSamplingAfterTenMinutes_AndLeaveTheRowAnswerable(t *testing.T) {
	t.Parallel()
	e := newSamplerEnv(t, ownFrame(t))
	id := e.colorRow("toolu_1")
	e.tickAt(id, 9*time.Minute)
	sampled := e.pane.count()
	require.Equal(t, 1, sampled)

	e.tickAt(id, 10*time.Minute+time.Second)
	e.tickAt(id, 40*time.Minute)

	assert.Equal(t, sampled, e.pane.count(), "no capture once the bound is reached")
	row := e.row(id)
	assert.Nil(t, row.CancelledAt)
	assert.Nil(t, row.AnsweredAt)
	assert.Empty(t, row.DeliveryState, "time alone never changes the delivery state")
	assert.Equal(t, domain.DeliveryRouteLiveKeystroke, row.DeliveryRoute)
	_, replyable := dialogFromRow(row)
	assert.True(t, replyable, "still answerable: its dialog can be rebuilt for the answer-time DialogMatch")
}

func TestDialogGoneSampler_should_SampleAtMostTwentyRowsPerPass(t *testing.T) {
	t.Parallel()
	rows := make([]*session.GuidanceRequestData, 25)
	now := time.Unix(1_800_000_000, 0)
	for i := range rows {
		rows[i] = &session.GuidanceRequestData{
			ID: fmt.Sprintf("row-%02d", i), Source: domain.QuestionSourceHook, AskerSessionUUID: mirrorSession, CreatedAt: now,
		}
	}
	pane := &paneScript{frames: []string{ownFrame(t)}}
	s := NewDialogGoneSampler(listStore(rows), pane.capture, noopClearer{}, func() time.Time { return now.Add(time.Second) })

	s.Tick(context.Background())
	assert.Equal(t, samplerMaxPerPass, pane.count())
	s.Tick(context.Background())
	assert.Equal(t, 25, pane.count(), "the five left over are sampled next, longest-unsampled first")
}

type listStore []*session.GuidanceRequestData

func (l listStore) ListAllPendingGuidanceRequests(context.Context) ([]*session.GuidanceRequestData, error) {
	return l, nil
}

type noopClearer struct{}

func (noopClearer) Clear(context.Context, string, domain.CancelReason) error { return nil }

func TestDialogGoneSampler_should_TreatAViewOnlyRowAsGoneOnlyWhenNoDialogShows(t *testing.T) {
	t.Parallel()
	e := newSamplerEnv(t, otherFrame(t), idleFrame(t), idleFrame(t))
	id, ok := e.mirror.Mirror(mirrorReq(t, "toolu_ro", "Read only?", false))
	require.True(t, ok)

	e.tickAt(id, 3*time.Second) // some dialog shows; this row cannot tell if it is its own
	e.tickAt(id, 6*time.Second)
	assert.Nil(t, e.row(id).CancelledAt)
	e.tickAt(id, 9*time.Second)
	assert.Equal(t, domain.CancelReasonDialogGone, e.row(id).CancelReason)
}

// ---- Task 3.3g: on-demand re-verification at the cap ----

func TestMirror_should_ReverifyOpenRowsAtTheCap_AndClearTheOnesWhoseDialogIsGone(t *testing.T) {
	t.Parallel()
	e := newSamplerEnv(t, idleFrame(t), idleFrame(t), idleFrame(t), ownFrame(t))
	e.mirror.SetPaneCapture(e.pane.capture)
	ids := make([]string, 4)
	for i := range ids {
		ids[i] = e.colorRow(fmt.Sprintf("toolu_%d", i))
	}

	fifth, ok := e.mirror.Mirror(mirrorReq(t, "toolu_5", "Question 5?", true))

	require.True(t, ok, "the cap freed up")
	assert.Empty(t, e.counts.failures())
	assert.Equal(t, 4, e.pane.count(), "one capture per open row, not a poll")
	cancelled := 0
	for _, id := range ids {
		if e.row(id).CancelledAt != nil {
			cancelled++
			assert.Equal(t, domain.CancelReasonDialogGone, e.row(id).CancelReason)
		}
	}
	assert.Equal(t, 3, cancelled)
	assert.Nil(t, e.row(fifth).CancelledAt)
}

func TestMirror_should_RefuseAtTheCap_WhenEveryDialogStillMatches(t *testing.T) {
	t.Parallel()
	e := newSamplerEnv(t, ownFrame(t))
	e.mirror.SetPaneCapture(e.pane.capture)
	for i := 0; i < 4; i++ {
		e.colorRow(fmt.Sprintf("toolu_%d", i))
	}

	_, ok := e.mirror.Mirror(mirrorReq(t, "toolu_5", "Question 5?", true))

	assert.False(t, ok)
	assert.Equal(t, []string{mirrorFailCapped}, e.counts.failures())
}

// ---- Task 3.3d: a PostToolUse receipt clears what the operator answered in the terminal ----

func TestClearOpenForSession_should_CancelOpenRowsOnly(t *testing.T) {
	t.Parallel()
	e := newSamplerEnv(t, idleFrame(t))
	open := e.colorRow("toolu_open")
	answered := e.colorRow("toolu_answered")
	_, err := e.storage.AnswerGuidanceRequest(context.Background(), uuid.MustParse(answered), "Red")
	require.NoError(t, err)

	require.NoError(t, e.mirror.ClearOpenForSession(context.Background(), mirrorSession, domain.CancelReasonAnsweredInTerminal))

	assert.Equal(t, domain.CancelReasonAnsweredInTerminal, e.row(open).CancelReason)
	assert.Nil(t, e.row(answered).CancelledAt, "an answer given through the pane is not cleared")
}

func TestObservePostToolUse_should_ClearTheMirroredRowsOfAnAnsweredQuestion(t *testing.T) {
	e := newRouterEnv(t)
	inst, _ := e.addVisible("terminal-answered")
	qid := e.hookRow(inst, colorDialog)
	e.fix.svc.SetGuidanceMirror(NewGuidanceHookMirror(e.fix.storage, e.fix.svc.PendingQuestions(), nil, nil))

	e.fix.svc.ObservePostToolUse(context.Background(), inst.UUID, askUserQuestion)

	row := e.row(qid)
	require.NotNil(t, row.CancelledAt)
	assert.Equal(t, domain.CancelReasonAnsweredInTerminal, row.CancelReason)
	_, still := e.fix.svc.PendingQuestions().Peek(QuestionKey{SessionUUID: inst.UUID, QuestionID: qid})
	assert.False(t, still, "the cache entry goes with the row")
}

// ---- Task 3.3g: an answer 45 minutes after the question (past the cache TTL) ----

func TestAnswerRouter_should_ReVerifyDialogMatchAndWriteOneByte_When_AnsweredPastTheCacheTTL(t *testing.T) {
	e := newRouterEnv(t)
	inst, pm := e.addVisible("answer-45m")
	qid := e.hookRow(inst, colorDialog)
	e.fix.svc.SetGuidanceMirror(NewGuidanceHookMirror(e.fix.storage, e.fix.svc.PendingQuestions(), nil, nil))
	e.clock.Advance(45 * time.Minute)
	_, cached := e.fix.svc.PendingQuestions().Peek(QuestionKey{SessionUUID: inst.UUID, QuestionID: qid})
	require.False(t, cached, "the 30-minute cache entry expired")

	res := e.answer(inst, qid, replyColorLabel)

	assert.Equal(t, sessionv1.ActionOutcome_ACTION_OUTCOME_APPLIED, res.Outcome)
	assert.Equal(t, []string{"2"}, pm.writes(), "exactly one byte after DialogMatch re-verified")
}

func TestAnswerRouter_should_ReturnStaleWithZeroWrites_When_TheDialogWasAnsweredInTheTerminalMeanwhile(t *testing.T) {
	e := newRouterEnv(t)
	inst, pm := e.addVisible("answer-45m-gone")
	qid := e.hookRow(inst, colorDialog)
	mirror := NewGuidanceHookMirror(e.fix.storage, e.fix.svc.PendingQuestions(), nil, nil)
	e.fix.svc.SetGuidanceMirror(mirror)
	e.clock.Advance(45 * time.Minute)
	require.NoError(t, mirror.Clear(context.Background(), qid, domain.CancelReasonAnsweredInTerminal))

	res := e.answer(inst, qid, replyColorLabel)

	assert.Equal(t, sessionv1.ActionOutcome_ACTION_OUTCOME_STALE, res.Outcome)
	assert.Empty(t, pm.writes())
}
