package services

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

func nudTotals(r *session.NudgeOutcomeRecorder) map[string]int64 {
	m := map[string]int64{}
	for _, c := range r.Snapshot().Totals {
		m[string(c.Kind)+"/"+string(c.Outcome)] = c.Value
	}
	return m
}

func TestStallRecovery_RecordsDurableNudgeOutcomes(t *testing.T) {
	t.Run("delivered then cooldown counts skipped_cap", func(t *testing.T) {
		rec := session.NewNudgeOutcomeRecorder("", nil)
		defer session.SwapNudgeOutcomeRecorder(rec)()
		f := newStallNudgeFixture(t, "nud-counted", session.BacklogStatusInProgress, true, true)

		f.complete(stallIn(domain.ObservedIdlePrompt, domain.StopReasonOrchestratorError))
		assert.Equal(t, map[string]int64{"stall/delivered": 1}, nudTotals(rec))

		f.complete(stallIn(domain.ObservedIdlePrompt, domain.StopReasonOrchestratorError))
		assert.Equal(t, map[string]int64{"stall/delivered": 1, "stall/skipped_cap": 1}, nudTotals(rec))
	})
	t.Run("uncertain submit", func(t *testing.T) {
		rec := session.NewNudgeOutcomeRecorder("", nil)
		defer session.SwapNudgeOutcomeRecorder(rec)()
		f := newStallNudgeFixture(t, "nud-uncertain", session.BacklogStatusInProgress, true, true)
		f.nudger.err = fmt.Errorf("%w: boom", session.ErrNudgeSubmitUncertain)

		f.complete(stallIn(domain.ObservedIdlePrompt, domain.StopReasonOrchestratorError))

		assert.Equal(t, map[string]int64{"stall/uncertain": 1}, nudTotals(rec))
	})
	t.Run("flags off records nothing", func(t *testing.T) {
		rec := session.NewNudgeOutcomeRecorder("", nil)
		defer session.SwapNudgeOutcomeRecorder(rec)()
		f := newStallNudgeFixture(t, "nud-off", session.BacklogStatusInProgress, false, false)

		f.complete(stallIn(domain.ObservedIdlePrompt, domain.StopReasonOrchestratorError))

		assert.Empty(t, nudTotals(rec))
	})
}
