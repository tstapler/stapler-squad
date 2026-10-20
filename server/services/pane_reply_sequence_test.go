package services

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

// Story 3.5 against the Spike S1c captures: Q1 single (Redis/Memcached/None),
// Q2 multi (A/B/C), answered Redis + [A, B] by the recorded 9-key sequence.

var recordedAnswerKeys = []string{
	session.KeyEnter, session.KeyEnter, session.KeyDown, session.KeyEnter, session.KeyDown,
	session.KeyDown, session.KeyDown, session.KeyEnter, session.KeyEnter,
}

const closedComposer = "\n────────────────────────────────────────\n❯ \n────────────────────────────────────────\n  ? for shortcuts\n"

// tabbedFrames returns the pane after 0..9 keys of the recorded sequence. The
// intermediate frames are the real AB_toggled capture with only its focus and
// checkbox marks moved, since the fixtures hold five of the ten frames.
func tabbedFrames(t *testing.T) []string {
	t.Helper()
	ab := replyDialogFixture(t, "claude_auq_twoq_step2_multiselect_AB_toggled.txt")
	edit := func(pairs ...string) string {
		out := ab
		for i := 0; i < len(pairs); i += 2 {
			require.Contains(t, out, pairs[i])
			out = strings.Replace(out, pairs[i], pairs[i+1], 1)
		}
		return out
	}
	return []string{
		replyDialogFixture(t, "claude_auq_twoq_step1_single_select.txt"),
		replyDialogFixture(t, "claude_auq_twoq_step2_multiselect_initial.txt"),
		edit("❯ 2. [✔] B", "  2. [ ] B", "  1. [✔] A", "❯ 1. [✔] A"),
		edit("❯ 2. [✔] B", "❯ 2. [ ] B"),
		ab,
		edit("❯ 2. [✔] B", "  2. [✔] B", "  3. [ ] C", "❯ 3. [ ] C"),
		edit("❯ 2. [✔] B", "  2. [✔] B", "  4. [ ] Type something", "❯ 4. [ ] Type something"),
		replyDialogFixture(t, "claude_auq_twoq_step2_multiselect_submit_focused.txt"),
		replyDialogFixture(t, "claude_auq_twoq_review_submit.txt"),
		closedComposer,
	}
}

const (
	redisAndAB     = `[["Redis"],["A","B"]]`
	partialStaleAt = "answer_aborted_partial"
)

// addTabbed registers a visible session whose pane replays frames.
func (e *routerEnv) addTabbed(title string, frames []string) (*session.Instance, *replyPM) {
	e.t.Helper()
	inst, pm := e.addVisible(title)
	pm.frames = frames
	return inst, pm
}

// twoQuestionRow stores the hook-mirrored row of the two-question dialog.
func (e *routerEnv) twoQuestionRow(inst *session.Instance, q1MultiSelect bool) string {
	e.t.Helper()
	opts := func(labels ...string) []domain.Option {
		o := make([]domain.Option, len(labels))
		for i, l := range labels {
			o[i] = domain.Option{Label: l}
		}
		return o
	}
	payload, err := domain.NewQuestionPayload([]domain.Question{
		{Text: "Which cache?", Header: "Cache", Options: opts("Redis", "Memcached", "None"), MultiSelect: q1MultiSelect},
		{Text: "Which letters?", Header: "Letters", Options: opts("A", "B", "C"), MultiSelect: true},
	})
	require.NoError(e.t, err)
	gr, err := domain.NewGuidanceRequest(domain.GuidanceRequestParams{
		Scope: domain.RequestScopeSession, SessionUUID: inst.UUID, Source: domain.QuestionSourceHook,
		AskerSessionUUID: inst.UUID, Payload: payload, HookCallID: domain.HookCallID("toolu_" + uuid.NewString()),
		DeliveryRoute: domain.DeliveryRouteLiveKeystroke,
	})
	require.NoError(e.t, err)
	row, err := e.fix.storage.CreateMirroredGuidanceRequest(context.Background(), gr, 0)
	require.NoError(e.t, err)
	return row.ID
}

func TestSequence_should_replayExactFixtureKeySequenceAndReturnBothAnswers_When_twoQuestionDialogWithMultiSelect(t *testing.T) {
	e := newRouterEnv(t)
	inst, pm := e.addTabbed("seq-ok", tabbedFrames(t))
	qid := e.twoQuestionRow(inst, false)

	res := e.answer(inst, qid, redisAndAB)

	assert.Equal(t, outApplied, res.Outcome)
	assert.Equal(t, recordedAnswerKeys, pm.writes())
	row := e.row(qid)
	assert.Equal(t, domain.DeliveryStateSent, row.DeliveryState)
	assert.Equal(t, redisAndAB, row.Answer)
	require.NotNil(t, row.AnsweredAt)
	sent := e.acks.all()
	require.Len(t, sent, 1)
	assert.Equal(t, qid, sent[0].RowID)
	assert.False(t, sent[0].Indeterminate)
	lines := replyLines(e.auditLines())
	require.Len(t, lines, 2)
	assert.Equal(t, replyStageDialogClosed, lines[1].Stage)
}

func TestSequence_should_abortWithStaleAndNoFurtherWrites_When_dialogMatchFailsAfterStep1(t *testing.T) {
	e := newRouterEnv(t)
	frames := tabbedFrames(t)
	frames[1] = "unrelated pane content\n" + closedComposer // the pane changed after Q1 was answered
	inst, pm := e.addTabbed("seq-stale", frames)
	qid := e.twoQuestionRow(inst, false)

	res := e.answer(inst, qid, redisAndAB)

	assert.Equal(t, outStale, res.Outcome)
	assert.Equal(t, partialStaleAt, res.StaleReason)
	assert.Equal(t, "1 of 9 keys sent", res.Detail)
	assert.Equal(t, []string{session.KeyEnter}, pm.writes(), "no write after the failed check")
	row := e.row(qid)
	assert.Equal(t, domain.DeliveryStateAbortedPartial, row.DeliveryState)
	assert.Nil(t, row.AnsweredAt, "the row stays unanswered, so it stays in Needs you")
	assert.Equal(t, session.GuidanceRequestStatusPending, row.Status())
	assert.Empty(t, e.acks.all(), "nothing to acknowledge for an aborted answer")
	lines := replyLines(e.auditLines())
	require.Len(t, lines, 2)
	assert.Equal(t, replyStageAbortedPartial, lines[1].Stage)
}

func TestSequence_should_writeNothing_When_theFirstFrameIsNotTheDialog(t *testing.T) {
	e := newRouterEnv(t)
	frames := tabbedFrames(t)
	frames[0] = frames[1] // already on Q2: the plan starts at Q1
	inst, pm := e.addTabbed("seq-first", frames)
	qid := e.twoQuestionRow(inst, false)

	res := e.answer(inst, qid, redisAndAB)

	assert.Equal(t, outStale, res.Outcome)
	assert.Equal(t, "dialog_replaced", res.StaleReason)
	assert.Empty(t, pm.writes())
	assert.Equal(t, domain.DeliveryStateStale, e.row(qid).DeliveryState, "nothing was typed: an ordinary stale, not a partial")
}

func TestSequence_should_reportNotSentVsSendIndeterminatePerStage_When_faultInjectedAtEachStep(t *testing.T) {
	for k := 1; k <= len(recordedAnswerKeys); k++ {
		t.Run(fmt.Sprintf("zero-byte failure at key %d", k), func(t *testing.T) {
			e := newRouterEnv(t)
			inst, pm := e.addTabbed("seq-fault", tabbedFrames(t))
			qid := e.twoQuestionRow(inst, false)
			pm.sendErr, pm.failAt = assert.AnError, k

			res := e.answer(inst, qid, redisAndAB)

			assert.Len(t, pm.writes(), k-1)
			row := e.row(qid)
			if k == 1 {
				assert.Equal(t, sessionv1.ActionOutcome_ACTION_OUTCOME_NOT_SENT, res.Outcome, "no byte left: Retry is safe")
				assert.Equal(t, domain.DeliveryStateFailed, row.DeliveryState)
				return
			}
			assert.Equal(t, outStale, res.Outcome, "earlier keys landed: never NOT_SENT")
			assert.Equal(t, partialStaleAt, res.StaleReason)
			assert.Equal(t, domain.DeliveryStateAbortedPartial, row.DeliveryState)
			assert.Nil(t, row.AnsweredAt)
		})
		t.Run(fmt.Sprintf("partial write at key %d", k), func(t *testing.T) {
			e := newRouterEnv(t)
			inst, pm := e.addTabbed("seq-short", tabbedFrames(t))
			qid := e.twoQuestionRow(inst, false)
			pm.sendErr, pm.sendN, pm.failAt = assert.AnError, 1, k

			res := e.answer(inst, qid, redisAndAB)

			assert.Len(t, pm.writes(), k, "the short key is recorded as written")
			if k == 1 {
				assert.Equal(t, sessionv1.ActionOutcome_ACTION_OUTCOME_SEND_INDETERMINATE, res.Outcome)
				assert.Equal(t, domain.DeliveryStateSent, e.row(qid).DeliveryState, "unconfirmed, tracked for acknowledgement")
				return
			}
			assert.Equal(t, partialStaleAt, res.StaleReason, "an unfinished answer is never a plain SEND_INDETERMINATE")
			assert.Equal(t, domain.DeliveryStateAbortedPartial, e.row(qid).DeliveryState)
		})
	}
}

func TestSequence_should_reportIndeterminateAndKeepTheRowOpen_When_theDialogStaysAfterTheLastKey(t *testing.T) {
	e := newRouterEnv(t)
	frames := tabbedFrames(t)
	frames[9] = frames[8] // the last Enter did nothing visible
	inst, _ := e.addTabbed("seq-open", frames)
	qid := e.twoQuestionRow(inst, false)

	res := e.answer(inst, qid, redisAndAB)

	assert.Equal(t, sessionv1.ActionOutcome_ACTION_OUTCOME_SEND_INDETERMINATE, res.Outcome)
	sent := e.acks.all()
	require.Len(t, sent, 1)
	assert.True(t, sent[0].Indeterminate)
}

func TestSequence_should_fallThroughToTheExistingPath_When_theShapeHasNoMeasuredKeySequence(t *testing.T) {
	e := newRouterEnv(t)
	inst, pm := e.addTabbed("seq-unmeasured", tabbedFrames(t))
	qid := e.twoQuestionRow(inst, true) // multi-select first: never captured

	res := e.answer(inst, qid, redisAndAB)

	assert.Equal(t, outStale, res.Outcome)
	assert.Equal(t, "dialog_gone", res.StaleReason)
	assert.Empty(t, pm.writes())
}

func TestSequence_should_refuseABadAnswerWithoutWriting(t *testing.T) {
	for name, label := range map[string]string{
		"not json":       "Redis",
		"unknown label":  `[["Valkey"],["A"]]`,
		"empty multi":    `[["Redis"],[]]`,
		"one list only":  `[["Redis"]]`,
		"free text pick": `[["Redis"],["Type something"]]`,
	} {
		t.Run(name, func(t *testing.T) {
			e := newRouterEnv(t)
			inst, pm := e.addTabbed("seq-bad", tabbedFrames(t))
			qid := e.twoQuestionRow(inst, false)
			res := e.answer(inst, qid, label)
			assert.Equal(t, outInvalid, res.Outcome)
			assert.Empty(t, pm.writes())
			assert.Empty(t, e.row(qid).DeliveryState)
		})
	}
}

func TestSequence_should_refuseWithHumanActive_WhenWebTerminalTypedWithinFiveSeconds(t *testing.T) {
	e := newRouterEnv(t)
	inst, pm := e.addTabbed("seq-typing", tabbedFrames(t))
	qid := e.twoQuestionRow(inst, false)
	e.activity.Record("ssq_seq-typing")
	e.clock.Advance(2 * time.Second)

	res := e.answer(inst, qid, redisAndAB)

	assert.Equal(t, sessionv1.ActionOutcome_ACTION_OUTCOME_NOT_SENT, res.Outcome)
	assert.Equal(t, "human_active", res.Detail)
	assert.Empty(t, pm.writes())
}

func TestSequence_should_keepAHiddenSessionOnTheOpenSessionPath(t *testing.T) {
	e := newRouterEnv(t)
	inst, pm := e.addTabbed("seq-hidden", tabbedFrames(t))
	inst.Hidden = true
	qid := e.twoQuestionRow(inst, false)

	res := e.answer(inst, qid, redisAndAB)

	assert.Equal(t, outInvalid, res.Outcome)
	assert.Contains(t, res.Detail, "open session")
	assert.Empty(t, pm.writes())
}

func TestSequencePicks_should_roundTrip(t *testing.T) {
	picks := [][]string{{"Redis"}, {"A", "C"}}
	got, err := DecodeSequencePicks(EncodeSequencePicks(picks))
	require.NoError(t, err)
	assert.Equal(t, picks, got)
}
