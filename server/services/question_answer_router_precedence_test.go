package services

import (
	"context"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/services/questionkit"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/instance"
)

// Characterization of the ADR-005 route precedence and of racing answers.

var suspendedStatuses = []domain.Status{domain.Paused, domain.Hibernated, domain.Stopped, domain.Crashed, domain.PermanentlyFailed}

func TestAnswerRouter_ShouldNeverTypeOrRevive_WhenAnArchivedSessionIsActiveOrSuspended(t *testing.T) {
	statuses := append([]domain.Status{domain.Active}, suspendedStatuses...)
	for _, status := range statuses {
		for _, hidden := range []bool{false, true} {
			t.Run(status.String()+map[bool]string{false: "/visible", true: "/hidden"}[hidden], func(t *testing.T) {
				e := newRouterEnv(t)
				inst, pm := e.addVisible("archived-any", func(i *instance.Instance) {
					now := time.Now()
					i.ArchivedAt, i.Status, i.Hidden = &now, status, hidden
				})
				qid := e.hookRow(inst, colorDialog)

				first := e.answer(inst, qid, replyColorLabel)
				retry, err := e.router.ReplyToQuestion(e.localCtx(), QuestionAnswer{
					SessionID: inst.UUID, QuestionID: qid, OptionLabel: replyColorLabel, ReplyID: "r-retry", Confirm: true,
				})
				require.NoError(t, err)

				assert.Equal(t, outApplied, first.Outcome)
				assert.Equal(t, outResolved, retry.Outcome, "Retry anyway does not reach the pane either")
				row := e.row(qid)
				assert.Equal(t, domain.DeliveryRouteReadOnly, row.DeliveryRoute)
				assert.Equal(t, domain.DeliveryStateSavedForLater, row.DeliveryState)
				lifecycleUntouched(t, inst, pm, status, true)
				assert.Empty(t, e.acks.all())
			})
		}
	}
}

func TestAnswerRouter_ShouldStoreOnTheResumeRoute_WhenSessionIsSuspendedAndNotArchived(t *testing.T) {
	for _, status := range suspendedStatuses {
		t.Run(status.String(), func(t *testing.T) {
			e := newRouterEnv(t)
			inst, pm := e.addVisible("suspended-any", func(i *instance.Instance) { i.Status = status })
			qid := e.hookRow(inst, colorDialog)

			res := e.answer(inst, qid, replyColorLabel)

			assert.Equal(t, outApplied, res.Outcome)
			row := e.row(qid)
			assert.Equal(t, domain.DeliveryRouteResume, row.DeliveryRoute)
			assert.Equal(t, domain.DeliveryStateSavedForLater, row.DeliveryState)
			lifecycleUntouched(t, inst, pm, status, false)
		})
	}
}

// race runs n answers at once and returns their results.
func (e *routerEnv) race(n int, inst *instance.Instance, qid string, labels ...string) []questionkit.ActionResult {
	e.t.Helper()
	results := make([]questionkit.ActionResult, n)
	errs := make([]error, n)
	start := make(chan struct{})
	var wg sync.WaitGroup
	ctx := e.localCtx()
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			results[i], errs[i] = e.router.ReplyToQuestion(ctx, QuestionAnswer{
				SessionID: inst.UUID, QuestionID: qid, OptionLabel: labels[i%len(labels)], ReplyID: "r-" + uuid.NewString(),
			})
		}()
	}
	close(start)
	wg.Wait()
	for _, err := range errs {
		require.NoError(e.t, err)
	}
	return results
}

func TestAnswerRouter_ShouldStoreOneWinner_WhenStoredAnswersRace(t *testing.T) {
	e := newRouterEnv(t)
	inst, _ := e.addVisible("stored-race", func(i *instance.Instance) { i.Status = domain.Stopped })
	qid := e.agentRow(inst)

	results := e.race(8, inst, qid, "Redis", "Memcached")

	winner := e.row(qid).Answer
	applied := 0
	for _, res := range results {
		if res.Outcome == outApplied {
			applied++
			continue
		}
		assert.Equal(t, outResolved, res.Outcome)
		assert.Equal(t, winner, res.Detail, "a loser is told the value that won")
	}
	assert.Equal(t, 1, applied)
}

func TestAnswerRouter_ShouldTypeOneDigit_WhenLiveAnswersRace(t *testing.T) {
	e := newRouterEnv(t)
	inst, pm := e.addVisible("live-race")
	qid := e.hookRow(inst, colorDialog)

	results := e.race(6, inst, qid, replyColorLabel)

	applied := 0
	for _, res := range results {
		if res.Outcome == outApplied {
			applied++
		}
	}
	assert.Equal(t, 1, applied)
	assert.Equal(t, []string{"2"}, pm.writes())
	assert.Equal(t, replyColorLabel, e.row(qid).Answer)
}

// A question whose write is in flight is no longer offered by the pending store.
func TestAnswerRouter_ShouldReportTheDialogGone_WhenASecondAnswerArrivesDuringTheWrite(t *testing.T) {
	e := newRouterEnv(t)
	inst, pm := e.addVisible("live-inflight")
	qid := e.hookRow(inst, colorDialog)
	pm.gate, pm.entered = make(chan struct{}), make(chan struct{}, 2)
	release := sync.OnceFunc(func() { close(pm.gate) })
	t.Cleanup(release)
	ctx := e.localCtx()
	answer := func() <-chan questionkit.ActionResult {
		done := make(chan questionkit.ActionResult, 1)
		go func() {
			res, err := e.router.ReplyToQuestion(ctx, QuestionAnswer{
				SessionID: inst.UUID, QuestionID: qid, OptionLabel: replyColorLabel, ReplyID: "r-" + uuid.NewString(),
			})
			assert.NoError(t, err)
			done <- res
		}()
		return done
	}
	wait := func(done <-chan questionkit.ActionResult) questionkit.ActionResult {
		select {
		case res := <-done:
			return res
		case <-time.After(10 * time.Second):
			t.Fatal("answer did not return")
			return questionkit.ActionResult{}
		}
	}

	first := answer()
	select {
	case <-pm.entered:
	case <-time.After(10 * time.Second):
		t.Fatal("the first answer never reached the write")
	}
	second := wait(answer())
	release()

	assert.Equal(t, outStale, second.Outcome)
	assert.Equal(t, "dialog_gone", second.StaleReason, "the claimed question is not offered to a second writer")
	assert.Equal(t, outApplied, wait(first).Outcome)
	assert.Equal(t, []string{"2"}, pm.writes())
}

// The frozen clock lets the rate limiter reject racers before they meet the claim,
// so this one holds the first write open and moves past the per-session interval.
func TestReply_ShouldFindNothingToClaim_WhenAnotherReplyIDArrivesDuringTheWrite(t *testing.T) {
	e := newReplyEnv(t)
	e.pm.gate, e.pm.entered = make(chan struct{}), make(chan struct{}, 2)
	release := sync.OnceFunc(func() { close(e.pm.gate) })
	t.Cleanup(release)
	ctx := e.localCtx()
	reply := func(replyID string) <-chan sessionv1.ReplyOutcome {
		done := make(chan sessionv1.ReplyOutcome, 1)
		go func() {
			resp, err := e.fix.svc.ReplyToPendingQuestion(ctx, connect.NewRequest(&sessionv1.ReplyToPendingQuestionRequest{
				SessionId: e.inst.UUID, QuestionId: e.qid, ReplyText: "1", ReplyId: replyID,
			}))
			assert.NoError(t, err)
			done <- resp.Msg.GetOutcome()
		}()
		return done
	}
	wait := func(done <-chan sessionv1.ReplyOutcome) sessionv1.ReplyOutcome {
		select {
		case o := <-done:
			return o
		case <-time.After(10 * time.Second):
			t.Fatal("reply did not return")
			return 0
		}
	}

	first := reply("r-first")
	select {
	case <-e.pm.entered:
	case <-time.After(10 * time.Second):
		t.Fatal("the first reply never reached the write")
	}
	e.clock.Advance(10 * time.Second)
	second := wait(reply("r-second"))
	release()

	assert.Equal(t, sessionv1.ReplyOutcome_REPLY_OUTCOME_NO_PENDING, second)
	assert.Equal(t, sessionv1.ReplyOutcome_REPLY_OUTCOME_SENT, wait(first))
	assert.Equal(t, []string{"1"}, e.pm.writes())
}

func TestHookReply_ShouldServeOnce_WhenTwoHooksAskAtOnce(t *testing.T) {
	t.Parallel()
	e := newHookReplyEnv(t)
	id := e.openRow(t, "s1", "Which cache?", false, "Redis", "Memcached")
	stored, err := e.resolver.PreAnswer(context.Background(), id, "Redis")
	require.NoError(t, err)
	require.True(t, stored)

	var served int
	var mu sync.Mutex
	var wg sync.WaitGroup
	start := make(chan struct{})
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if e.resolver.Reply(context.Background(), "s1", askToolInput("Which cache?", "Redis", "Memcached")) != nil {
				mu.Lock()
				served++
				mu.Unlock()
			}
		}()
	}
	close(start)
	wg.Wait()

	assert.Equal(t, 1, served)
	assert.Len(t, e.acks.all(), 1)
}

func TestAnswerRouter_ShouldRefuseAsInvalid_WhenTheOperatorLabelIsNotOnTheDialog(t *testing.T) {
	e := newRouterEnv(t)
	inst, pm := e.addVisible("live-bad-label")
	qid := e.hookRow(inst, colorDialog)

	res := e.answer(inst, qid, "2")

	assert.Equal(t, sessionv1.ActionOutcome_ACTION_OUTCOME_INVALID, res.Outcome, "a raw digit is not an option label")
	assert.Empty(t, pm.writes())
}
