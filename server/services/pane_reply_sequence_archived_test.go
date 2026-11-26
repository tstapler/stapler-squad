package services

import (
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/server/services/questionkit"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/instance"
)

const staleDialogGone = "dialog_gone"

func archiveNow(inst *instance.Instance) {
	now := time.Now()
	inst.SetArchivedAt(&now)
}

// The router read the session as live; it was archived before the sequence started.
func TestSequence_should_writeNothingAndStartNoAttempt_When_theSessionIsAlreadyArchived(t *testing.T) {
	e := newRouterEnv(t)
	inst, pm := e.addTabbed("seq-archived", tabbedFrames(t))
	qid := e.twoQuestionRow(inst, false)
	qs, ok := e.router.sequenceQuestions(e.localCtx(), qid)
	require.True(t, ok)
	archiveNow(inst)

	res, err := e.router.liveSequence(e.localCtx(), routedSession{inst: inst, phase: phaseLive},
		QuestionAnswer{SessionID: inst.UUID, QuestionID: qid, OptionLabel: redisAndAB, ReplyID: "r-" + uuid.NewString()}, qs)

	require.NoError(t, err)
	assert.Equal(t, outStale, res.Outcome)
	assert.Equal(t, staleDialogGone, res.StaleReason)
	assert.Empty(t, pm.writes())
	assert.Empty(t, replyLines(e.auditLines()))
	instance.AssertLeaseFree(t, inst)
}

func TestSequence_should_writeNothing_When_theSessionIsArchivedBeforeTheFirstKey(t *testing.T) {
	e := newRouterEnv(t)
	inst, pm := e.addTabbed("seq-archived-first", tabbedFrames(t))
	qid := e.twoQuestionRow(inst, false)
	e.fix.svc.replyControllerActive = func(i *instance.Instance) bool {
		archiveNow(i)
		return true
	}

	res := e.answer(inst, qid, redisAndAB)

	assert.Equal(t, outStale, res.Outcome)
	assert.Equal(t, staleDialogGone, res.StaleReason)
	assert.Empty(t, pm.writes())
	lines := replyLines(e.auditLines())
	require.Len(t, lines, 2)
	assert.Equal(t, replyStageArchived, lines[1].Stage)
	assert.Equal(t, "no_pending", lines[1].Outcome)
	instance.AssertLeaseFree(t, inst)
}

func TestSequence_should_writeNoFurtherKey_When_theSessionIsArchivedDuringTheSequence(t *testing.T) {
	for k := 1; k < len(recordedAnswerKeys); k++ {
		t.Run(fmt.Sprintf("archived during key %d", k), func(t *testing.T) {
			e := newRouterEnv(t)
			inst, pm := e.addTabbed("seq-archived-mid", tabbedFrames(t))
			qid := e.twoQuestionRow(inst, false)
			writes := 0
			pm.probe = func() int {
				if writes++; writes == k {
					archiveNow(inst)
				}
				return 0
			}

			res := e.answer(inst, qid, redisAndAB)

			assert.Equal(t, recordedAnswerKeys[:k], pm.writes())
			assert.Equal(t, outStale, res.Outcome)
			assert.Equal(t, questionkit.StaleReasonAnswerPartial, res.StaleReason)
			assert.Equal(t, domain.DeliveryStateAbortedPartial, e.row(qid).DeliveryState)
			lines := replyLines(e.auditLines())
			require.Len(t, lines, 2)
			assert.Equal(t, replyStageArchived, lines[1].Stage)
			assert.Equal(t, "no_pending", lines[1].Outcome)
			instance.AssertLeaseFree(t, inst)
		})
	}
}
