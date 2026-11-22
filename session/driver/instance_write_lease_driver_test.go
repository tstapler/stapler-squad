package driver

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/session/instance"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/detection"
	"github.com/tstapler/stapler-squad/session/domain"
)

// leaseInstance returns an instance with its own flag, so tests never share
// DefaultLeaseFlag state.
func leaseInstance(t *testing.T, uuid string) *instance.Instance {
	t.Helper()
	inst := &instance.Instance{Title: "lease-" + uuid, UUID: uuid}
	inst.SetLeaseFlag(&instance.LeaseFlag{})
	return inst
}

// waitLeaseFree tolerates the write goroutine's release landing just after the
// primitive returned its result.
func waitLeaseFree(t *testing.T, inst *instance.Instance) {
	t.Helper()
	require.Eventually(t, func() bool {
		l, ok := inst.TryTerminalWriteLease(instance.LeaseWriterOther)
		if ok {
			l.Release()
		}
		return ok
	}, 5*time.Second, time.Millisecond, "the writing goroutine never released the lease")
}

// T-WL-05: never acquired under i.mu or the actor; -race.
func TestWriteLease_ShouldNotDeadlockOrInvertLockOrder_WhenDriverSteerNudgeAndStatusPollerRunConcurrently(t *testing.T) {
	t.Parallel()
	pm := &stuckDialogProcessManager{dialogText: "idle"}
	inst := (&instance.Instance{Title: "lock-order", Status: domain.Active}).WithProcessManagerForTest(pm)
	inst.MarkStarted()
	inst.SetLeaseFlag(&instance.LeaseFlag{})

	var wg sync.WaitGroup
	run := func(f func()) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 300; i++ {
				f()
			}
		}()
	}
	run(func() { _ = sendAnswerKeyUnderLease(inst) }) // driver tick
	run(func() {                                      // steer: acquire, then a status read while held
		if l, err := inst.AcquireTerminalWriteLease(context.Background(), instance.LeaseWriterSteer, time.Millisecond); err == nil {
			_ = inst.Snapshot()
			l.Release()
		}
	})
	run(func() { // nudge
		if l, ok := inst.TryTerminalWriteLease(instance.LeaseWriterNudge); ok {
			l.Release()
		}
	})
	run(func() { // status poller
		_, _ = inst.HasUpdated()
		_, _ = inst.Preview()
		_ = inst.Snapshot()
	})

	joined := make(chan struct{})
	go func() { wg.Wait(); close(joined) }()
	select {
	case <-joined:
	case <-time.After(10 * time.Second):
		t.Fatal("deadlock: lease acquirers and the status poller did not finish")
	}
	instance.AssertLeaseFree(t, inst)
}

type warnRecord struct {
	msg     string
	session string
}

type warnCapture struct {
	mu   *sync.Mutex
	recs *[]warnRecord
}

func (warnCapture) Enabled(context.Context, slog.Level) bool { return true }
func (h warnCapture) Handle(_ context.Context, r slog.Record) error {
	rec := warnRecord{msg: r.Message}
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == "session" {
			rec.session = a.Value.String()
		}
		return true
	})
	h.mu.Lock()
	*h.recs = append(*h.recs, rec)
	h.mu.Unlock()
	return nil
}
func (h warnCapture) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h warnCapture) WithGroup(string) slog.Handler      { return h }

func (h warnCapture) count(msg, session string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	n := 0
	for _, r := range *h.recs {
		if r.msg == msg && r.session == session {
			n++
		}
	}
	return n
}

// T-WL-06: not parallel; it swaps the process-wide slog default.
func TestWriteLease_ShouldRaiseTheHeldSecondsGaugeAndLogOneWedgeWarnPerMinuteAndNeverForceRelease_WhenAWriteNeverReturns(t *testing.T) {
	cap := warnCapture{mu: &sync.Mutex{}, recs: &[]warnRecord{}}
	prev := log.SetSlogDefaultForTest(slog.New(cap))
	t.Cleanup(func() { log.SetSlogDefaultForTest(prev) })

	inst := leaseInstance(t, "wedge")
	lease, ok := inst.TryTerminalWriteLease(instance.LeaseWriterReply)
	require.True(t, ok)
	t0 := lease.AcquiredAtForTest()
	scanMine := func(now time.Time, warnAfter time.Duration) []instance.WedgedLease {
		return instance.ScanWedgedLeases(now, warnAfter, func(i *instance.Instance) bool { return i == inst })
	}
	mine := func(ws []instance.WedgedLease) int { return len(ws) }
	title := inst.Title

	assert.InDelta(t, 45, instance.LeaseHeldSeconds(instance.LeaseWriterReply, t0.Add(45*time.Second)), 1.0)
	assert.Zero(t, instance.LeaseHeldSeconds(instance.LeaseWriterMCP, t0.Add(45*time.Second)), "other writers' gauge is unaffected")

	assert.Zero(t, mine(scanMine(t0.Add(instance.LeaseWedgeWarnAfter-time.Second), instance.LeaseWedgeWarnAfter)))
	assert.Zero(t, cap.count("terminal_write_lease_wedged", title))

	assert.Equal(t, 1, mine(scanMine(t0.Add(instance.LeaseWedgeWarnAfter), instance.LeaseWedgeWarnAfter)), "first sighting")
	assert.Equal(t, 1, cap.count("terminal_write_lease_wedged", title))

	assert.Zero(t, mine(scanMine(t0.Add(instance.LeaseWedgeWarnAfter+30*time.Second), instance.LeaseWedgeWarnAfter)), "same episode is not a first sighting")
	assert.Equal(t, 1, cap.count("terminal_write_lease_wedged", title), "at most one WARN a minute")

	assert.Zero(t, mine(scanMine(t0.Add(instance.LeaseWedgeWarnAfter+instance.LeaseWedgeWarnEvery), instance.LeaseWedgeWarnAfter)))
	assert.Equal(t, 2, cap.count("terminal_write_lease_wedged", title))

	// Never force-released: the driver tick skips, the steer and nudge get a retryable busy.
	busyBefore := instance.LeaseBusyTotal(instance.LeaseWriterDriver)
	assert.ErrorIs(t, sendAnswerKeyUnderLease(inst), instance.ErrLeaseBusy)
	assert.Greater(t, instance.LeaseBusyTotal(instance.LeaseWriterDriver), busyBefore, "busy counter increments")
	_, ok = inst.TryTerminalWriteLease(instance.LeaseWriterNudge)
	assert.False(t, ok)

	lease.Release()
	instance.AssertLeaseFree(t, inst)
	assert.Zero(t, instance.LeaseHeldSeconds(instance.LeaseWriterReply, t0.Add(time.Hour)), "a released lease leaves the gauge")
}

// T-WL-04 (latch half): a busy lease is not a dialog attempt.
func TestAutomatedWriters_ShouldNotCountABusyLeaseAsADialogAttemptAndStillAnswerAfterThreeBusyTicks_WhenTheLeaseIsBusy(t *testing.T) {
	t.Parallel()
	pm := &stuckDialogProcessManager{dialogText: trustDialogText}
	inst := (&instance.Instance{Title: "latch", Status: domain.Active}).WithProcessManagerForTest(pm)
	inst.MarkStarted()
	inst.SetLeaseFlag(&instance.LeaseFlag{})

	held, ok := inst.TryTerminalWriteLease(instance.LeaseWriterReply)
	require.True(t, ok)

	var latch dialogAnswerState
	send := func() error { return sendAnswerKeyUnderLease(inst) }
	for tick := 0; tick < 3; tick++ {
		st := answerDialogOnce(&latch, trustDialogText, send, inst.Title, "startup dialog")
		require.Equal(t, dialogUnanswered, st, "tick %d", tick)
		require.Zero(t, latch.attempts, "a busy lease must not burn an attempt")
	}
	require.Zero(t, pm.sendKeysCount.Load(), "nothing written while the lease is held")

	held.Release()
	st := answerDialogOnce(&latch, trustDialogText, send, inst.Title, "startup dialog")
	require.Equal(t, dialogAwaitingDismissal, st)
	require.EqualValues(t, 1, pm.sendKeysCount.Load(), "answered once after the lease freed")
	instance.AssertLeaseFree(t, inst)
}

// T-WL-04 (nudge half): the idle nudge skips the tick and does not latch.
func TestAttemptBacklogNudge_ShouldSkipTheTickAndWriteNothing_WhenTheLeaseIsBusy(t *testing.T) {
	t.Parallel()
	pm := &stuckDialogProcessManager{dialogText: "idle"}
	inst := (&instance.Instance{Title: "nudge-busy", Status: domain.Active}).WithProcessManagerForTest(pm)
	inst.MarkStarted()
	inst.SetLeaseFlag(&instance.LeaseFlag{})

	held, _ := inst.TryTerminalWriteLease(instance.LeaseWriterReply)
	got := attemptBacklogNudge(context.Background(), inst, time.Hour)
	assert.True(t, got.IsZero(), "a skipped tick leaves nudgeSentAt unset so the next tick nudges")
	assert.Zero(t, pm.sendKeysCount.Load())
	held.Release()
	instance.AssertLeaseFree(t, inst)
}

// T-WL-09
func TestInitialPrompt_ShouldNotCountALeaseApiErrorTowardTheAttemptLimitNorMarkSent_WhenSubmitDriverContentReturnsErrNoLeaseOrErrLeaseMismatch(t *testing.T) {
	for _, leaseErr := range []error{instance.ErrNoLease, instance.ErrLeaseMismatch} {
		t.Run(leaseErr.Error(), func(t *testing.T) {
			prev := submitInitialPrompt
			submitInitialPrompt = func(_ context.Context, _ *instance.Instance, l *instance.HeldLease, _ string) error {
				l.Release()
				return leaseErr
			}
			t.Cleanup(func() { submitInitialPrompt = prev })

			pm := &stuckDialogProcessManager{dialogText: "> "}
			inst := (&instance.Instance{Title: "initial-lease-err", Status: domain.Active}).WithProcessManagerForTest(pm)
			inst.MarkStarted()
			inst.SetLeaseFlag(&instance.LeaseFlag{})

			var sent bool
			var sentAt time.Time
			attempts := 2 // one failure from the limit
			sendInitialPromptTick(context.Background(), inst, "do the thing", "> ", detection.StatusIdle, time.Now().Add(-time.Hour), &sent, &sentAt, &attempts)

			assert.False(t, sent, "must not markSent")
			assert.Equal(t, 2, attempts, "must not count toward the attempt limit")
			instance.AssertLeaseFree(t, inst)
		})
	}
}

// T-WL-04 (initial prompt half): a busy lease consumes no attempt.
func TestInitialPrompt_ShouldConsumeNoAttempt_WhenTheLeaseIsBusy(t *testing.T) {
	pm := &stuckDialogProcessManager{dialogText: "> "}
	inst := (&instance.Instance{Title: "initial-busy", Status: domain.Active}).WithProcessManagerForTest(pm)
	inst.MarkStarted()
	inst.SetLeaseFlag(&instance.LeaseFlag{})
	held, _ := inst.TryTerminalWriteLease(instance.LeaseWriterReply)

	var sent bool
	var sentAt time.Time
	attempts := 2
	sendInitialPromptTick(context.Background(), inst, "do the thing", "> ", detection.StatusIdle, time.Now().Add(-time.Hour), &sent, &sentAt, &attempts)

	assert.False(t, sent, "the prompt must not be dropped")
	assert.Equal(t, 2, attempts, "a busy lease must not burn an attempt")
	assert.Zero(t, pm.sendKeysCount.Load())
	held.Release()
}
