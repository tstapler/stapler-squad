package session

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/detection"
)

// leaseInstance returns an instance with its own flag, so tests never share
// DefaultLeaseFlag state.
func leaseInstance(t *testing.T, uuid string) *Instance {
	t.Helper()
	inst := &Instance{Title: "lease-" + uuid, UUID: uuid}
	inst.SetLeaseFlag(&LeaseFlag{})
	return inst
}

// T-WL-01
func TestWriteLease_ShouldRefuseNilZeroAndForeignLeaseWithZeroWrites_WhenAnyWritePrimitiveIsCalled(t *testing.T) {
	t.Parallel()
	foreign := leaseInstance(t, "someone-else")

	cases := []struct {
		name    string
		lease   func() *HeldLease
		wantErr error
	}{
		{"nil", func() *HeldLease { return nil }, ErrNoLease},
		{"zero value", func() *HeldLease { return &HeldLease{} }, ErrNoLease},
		{"foreign instance", func() *HeldLease {
			l, ok := foreign.TryTerminalWriteLease(LeaseWriterOther)
			require.True(t, ok)
			return l
		}, ErrLeaseMismatch},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			pane := newFakePaneSubmitter()

			l := tc.lease()
			err := SubmitDriverContent(ctx, pane, l, "x", time.Millisecond, time.Millisecond)
			require.ErrorIs(t, err, tc.wantErr)

			// A refused lease is not reused by the next call: take a fresh one.
			l = tc.lease()
			err = SubmitContentWithEnter(ctx, pane, l, "x")
			require.ErrorIs(t, err, tc.wantErr)

			l = tc.lease()
			err = SendKeysWithTimeout(ctx, pane, l, "x", time.Second)
			require.ErrorIs(t, err, tc.wantErr)

			assert.Empty(t, pane.sendCalls, "a refused lease must perform 0 writes")
		})
	}
	// Every refusal released the foreign lease it was handed.
	AssertLeaseFree(t, foreign)
}

// T-WL-02
func TestWriteLease_ShouldReturnFalseAtOnceForATryAndBusyAfterTheBoundedWaitForAnAcquireAndNeverHang_WhenAFixtureChainAcquiresTwiceForTheSameInstance(t *testing.T) {
	t.Parallel()
	inst := leaseInstance(t, "nest")
	outer, ok := inst.TryTerminalWriteLease(LeaseWriterSteer)
	require.True(t, ok)

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, ok := inst.TryTerminalWriteLease(LeaseWriterSteer)
		assert.False(t, ok, "a nested Try must return false at once")

		start := time.Now()
		_, err := inst.AcquireTerminalWriteLease(context.Background(), LeaseWriterSteer, 30*time.Millisecond)
		assert.ErrorIs(t, err, ErrLeaseBusy)
		assert.GreaterOrEqual(t, time.Since(start), 30*time.Millisecond)

		_, err = inst.AcquireTerminalWriteLease(context.Background(), LeaseWriterSteer, 0)
		assert.ErrorIs(t, err, ErrLeaseBusy, "a zero wait is a Try")
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("a nested acquire hung instead of returning busy")
	}

	outer.Release()
	outer.Release() // idempotent backstop
	AssertLeaseFree(t, inst)
}

func TestWriteLease_ShouldReturnCtxErr_WhenAcquireIsCancelledWhileWaiting(t *testing.T) {
	t.Parallel()
	inst := leaseInstance(t, "cancel")
	held, _ := inst.TryTerminalWriteLease(LeaseWriterSteer)
	defer held.Release()

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		_, err := inst.AcquireTerminalWriteLease(ctx, LeaseWriterDriver, time.Minute)
		errCh <- err
	}()
	cancel()
	require.ErrorIs(t, <-errCh, context.Canceled)
}

func TestWriteLease_ShouldHandTheLeaseToAWaiter_WhenTheHolderReleases(t *testing.T) {
	t.Parallel()
	inst := leaseInstance(t, "handoff")
	held, _ := inst.TryTerminalWriteLease(LeaseWriterSteer)

	got := make(chan *HeldLease, 1)
	go func() {
		l, err := inst.AcquireTerminalWriteLease(context.Background(), LeaseWriterAutonomous, 10*time.Second)
		assert.NoError(t, err)
		got <- l
	}()
	held.Release()
	l := <-got
	require.NotNil(t, l)
	l.Release()
	AssertLeaseFree(t, inst)
}

// T-WL-05: never acquired under i.mu or the actor; -race.
func TestWriteLease_ShouldNotDeadlockOrInvertLockOrder_WhenDriverSteerNudgeAndStatusPollerRunConcurrently(t *testing.T) {
	t.Parallel()
	pm := &stuckDialogProcessManager{dialogText: "idle"}
	inst := &Instance{Title: "lock-order", Status: Ready, processManager: pm}
	inst.started.Store(true)
	inst.SetLeaseFlag(&LeaseFlag{})

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
		if l, err := inst.AcquireTerminalWriteLease(context.Background(), LeaseWriterSteer, time.Millisecond); err == nil {
			_ = inst.Snapshot()
			l.Release()
		}
	})
	run(func() { // nudge
		if l, ok := inst.TryTerminalWriteLease(LeaseWriterNudge); ok {
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
	AssertLeaseFree(t, inst)
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
	lease, ok := inst.TryTerminalWriteLease(LeaseWriterReply)
	require.True(t, ok)
	t0 := lease.acquiredAt
	scanMine := func(now time.Time, warnAfter time.Duration) []WedgedLease {
		return ScanWedgedLeases(now, warnAfter, func(i *Instance) bool { return i == inst })
	}
	mine := func(ws []WedgedLease) int { return len(ws) }
	title := inst.Title

	assert.InDelta(t, 45, LeaseHeldSeconds(LeaseWriterReply, t0.Add(45*time.Second)), 1.0)
	assert.Zero(t, LeaseHeldSeconds(LeaseWriterMCP, t0.Add(45*time.Second)), "other writers' gauge is unaffected")

	assert.Zero(t, mine(scanMine(t0.Add(LeaseWedgeWarnAfter-time.Second), LeaseWedgeWarnAfter)))
	assert.Zero(t, cap.count("terminal_write_lease_wedged", title))

	assert.Equal(t, 1, mine(scanMine(t0.Add(LeaseWedgeWarnAfter), LeaseWedgeWarnAfter)), "first sighting")
	assert.Equal(t, 1, cap.count("terminal_write_lease_wedged", title))

	assert.Zero(t, mine(scanMine(t0.Add(LeaseWedgeWarnAfter+30*time.Second), LeaseWedgeWarnAfter)), "same episode is not a first sighting")
	assert.Equal(t, 1, cap.count("terminal_write_lease_wedged", title), "at most one WARN a minute")

	assert.Zero(t, mine(scanMine(t0.Add(LeaseWedgeWarnAfter+LeaseWedgeWarnEvery), LeaseWedgeWarnAfter)))
	assert.Equal(t, 2, cap.count("terminal_write_lease_wedged", title))

	// Never force-released: the driver tick skips, the steer and nudge get a retryable busy.
	busyBefore := LeaseBusyTotal(LeaseWriterDriver)
	assert.ErrorIs(t, sendAnswerKeyUnderLease(inst), ErrLeaseBusy)
	assert.Greater(t, LeaseBusyTotal(LeaseWriterDriver), busyBefore, "busy counter increments")
	_, ok = inst.TryTerminalWriteLease(LeaseWriterNudge)
	assert.False(t, ok)

	lease.Release()
	AssertLeaseFree(t, inst)
	assert.Zero(t, LeaseHeldSeconds(LeaseWriterReply, t0.Add(time.Hour)), "a released lease leaves the gauge")
}

// T-WL-04 (latch half): a busy lease is not a dialog attempt.
func TestAutomatedWriters_ShouldNotCountABusyLeaseAsADialogAttemptAndStillAnswerAfterThreeBusyTicks_WhenTheLeaseIsBusy(t *testing.T) {
	t.Parallel()
	pm := &stuckDialogProcessManager{dialogText: trustDialogText}
	inst := &Instance{Title: "latch", Status: Ready, processManager: pm}
	inst.started.Store(true)
	inst.SetLeaseFlag(&LeaseFlag{})

	held, ok := inst.TryTerminalWriteLease(LeaseWriterReply)
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
	AssertLeaseFree(t, inst)
}

// T-WL-04 (nudge half): the idle nudge skips the tick and does not latch.
func TestAttemptBacklogNudge_ShouldSkipTheTickAndWriteNothing_WhenTheLeaseIsBusy(t *testing.T) {
	t.Parallel()
	pm := &stuckDialogProcessManager{dialogText: "idle"}
	inst := &Instance{Title: "nudge-busy", Status: Ready, processManager: pm}
	inst.started.Store(true)
	inst.SetLeaseFlag(&LeaseFlag{})

	held, _ := inst.TryTerminalWriteLease(LeaseWriterReply)
	got := attemptBacklogNudge(context.Background(), inst, time.Hour)
	assert.True(t, got.IsZero(), "a skipped tick leaves nudgeSentAt unset so the next tick nudges")
	assert.Zero(t, pm.sendKeysCount.Load())
	held.Release()
	AssertLeaseFree(t, inst)
}

// T-WL-09
func TestInitialPrompt_ShouldNotCountALeaseApiErrorTowardTheAttemptLimitNorMarkSent_WhenSubmitDriverContentReturnsErrNoLeaseOrErrLeaseMismatch(t *testing.T) {
	for _, leaseErr := range []error{ErrNoLease, ErrLeaseMismatch} {
		t.Run(leaseErr.Error(), func(t *testing.T) {
			prev := submitInitialPrompt
			submitInitialPrompt = func(_ context.Context, _ *Instance, l *HeldLease, _ string) error {
				l.Release()
				return leaseErr
			}
			t.Cleanup(func() { submitInitialPrompt = prev })

			pm := &stuckDialogProcessManager{dialogText: "> "}
			inst := &Instance{Title: "initial-lease-err", Status: Ready, processManager: pm}
			inst.started.Store(true)
			inst.SetLeaseFlag(&LeaseFlag{})

			var sent bool
			var sentAt time.Time
			attempts := 2 // one failure from the limit
			sendInitialPromptTick(context.Background(), inst, "do the thing", "> ", detection.StatusIdle, time.Now().Add(-time.Hour), &sent, &sentAt, &attempts)

			assert.False(t, sent, "must not markSent")
			assert.Equal(t, 2, attempts, "must not count toward the attempt limit")
			AssertLeaseFree(t, inst)
		})
	}
}

// T-WL-04 (initial prompt half): a busy lease consumes no attempt.
func TestInitialPrompt_ShouldConsumeNoAttempt_WhenTheLeaseIsBusy(t *testing.T) {
	pm := &stuckDialogProcessManager{dialogText: "> "}
	inst := &Instance{Title: "initial-busy", Status: Ready, processManager: pm}
	inst.started.Store(true)
	inst.SetLeaseFlag(&LeaseFlag{})
	held, _ := inst.TryTerminalWriteLease(LeaseWriterReply)

	var sent bool
	var sentAt time.Time
	attempts := 2
	sendInitialPromptTick(context.Background(), inst, "do the thing", "> ", detection.StatusIdle, time.Now().Add(-time.Hour), &sent, &sentAt, &attempts)

	assert.False(t, sent, "the prompt must not be dropped")
	assert.Equal(t, 2, attempts, "a busy lease must not burn an attempt")
	assert.Zero(t, pm.sendKeysCount.Load())
	held.Release()
}

// T-WL-11
func TestTerminalWriteLeaseFlag_ShouldSerializeWhenOnAndReturnANonExclusiveLeaseWhenOffAndFailClosedOnAnUnreadableConfig(t *testing.T) {
	t.Parallel()
	var nilFlag *LeaseFlag
	assert.True(t, nilFlag.Enabled(), "an unreadable config (no flag value) keeps the lease on")
	assert.True(t, (&LeaseFlag{}).Enabled(), "default on")

	flag := &LeaseFlag{}
	inst := &Instance{Title: "flag", UUID: fakeLeaseOwner}
	inst.SetLeaseFlag(flag)

	a, ok := inst.TryTerminalWriteLease(LeaseWriterSteer)
	require.True(t, ok)
	_, ok = inst.TryTerminalWriteLease(LeaseWriterMCP)
	assert.False(t, ok, "on: exclusive")
	a.Release()

	flag.SetEnabled(false)
	b, ok := inst.TryTerminalWriteLease(LeaseWriterSteer)
	require.True(t, ok)
	c, ok := inst.TryTerminalWriteLease(LeaseWriterMCP)
	require.True(t, ok, "off: non-exclusive, today's behavior")
	d, err := inst.AcquireTerminalWriteLease(context.Background(), LeaseWriterMCP, 0)
	require.NoError(t, err)

	// The capability types keep working with the flag off; a zero value is still refused.
	pane := newFakePaneSubmitter()
	pane.updates = []bool{true}
	require.ErrorIs(t, SubmitDriverContent(context.Background(), pane, &HeldLease{}, "x", time.Millisecond, time.Millisecond), ErrNoLease)
	require.NoError(t, SendKeysWithTimeout(context.Background(), pane, b, "x", time.Second))
	c.Release()
	d.Release()

	flag.SetEnabled(true)
	AssertLeaseFree(t, inst)
	e, ok := inst.TryTerminalWriteLease(LeaseWriterSteer)
	require.True(t, ok)
	_, ok = inst.TryTerminalWriteLease(LeaseWriterMCP)
	assert.False(t, ok, "back on: exclusive again")
	e.Release()
}

// T-WL-10: every exit path of the three primitives leaves the lease free.
func TestEveryAcquirer_ShouldLeaveTheLeaseFreeOnEveryExitPath_WhenSuccessBusyWriteErrorCancelledContextAndEarlyReturnAreExercised(t *testing.T) {
	t.Parallel()
	// Each case gets its own instance bound to the fake pane's lease identity.
	setup := func(t *testing.T, failOnCall int) (*Instance, *fakePaneSubmitter, *HeldLease) {
		t.Helper()
		inst := leaseInstance(t, fakeLeaseOwner)
		pane := newFakePaneSubmitter()
		pane.failOnCall = failOnCall
		lease, ok := inst.TryTerminalWriteLease(LeaseWriterOther)
		require.True(t, ok)
		return inst, pane, lease
	}
	ctx := context.Background()

	t.Run("SubmitDriverContent success", func(t *testing.T) {
		t.Parallel()
		inst, pane, lease := setup(t, -1)
		require.NoError(t, SubmitDriverContent(ctx, pane, lease, "x", time.Millisecond, 20*time.Millisecond))
		AssertLeaseFree(t, inst)
	})
	t.Run("SubmitDriverContent write error", func(t *testing.T) {
		t.Parallel()
		inst, pane, lease := setup(t, 0)
		require.Error(t, SubmitDriverContent(ctx, pane, lease, "x", time.Millisecond, 20*time.Millisecond))
		AssertLeaseFree(t, inst)
	})
	t.Run("SubmitDriverContent cancelled context", func(t *testing.T) {
		t.Parallel()
		inst, pane, lease := setup(t, -1)
		cancelled, cancel := context.WithCancel(ctx)
		cancel()
		require.Error(t, SubmitDriverContent(cancelled, pane, lease, "x", time.Millisecond, 20*time.Millisecond))
		AssertLeaseFree(t, inst)
	})
	t.Run("SubmitDriverContent pane owner refusal", func(t *testing.T) {
		t.Parallel()
		inst, pane, lease := setup(t, -1)
		sub := &ownerCheckedSubmitter{fakePaneSubmitter: pane, ownerErr: errors.New("owner mismatch")}
		require.Error(t, SubmitDriverContent(ctx, sub, lease, "x", time.Millisecond, 20*time.Millisecond))
		AssertLeaseFree(t, inst)
		assert.Empty(t, pane.sendCalls)
	})
	t.Run("SubmitContentWithEnter success", func(t *testing.T) {
		t.Parallel()
		inst, pane, lease := setup(t, -1)
		require.NoError(t, SubmitContentWithEnter(ctx, pane, lease, "x"))
		waitLeaseFree(t, inst)
	})
	t.Run("SubmitContentWithEnter write error", func(t *testing.T) {
		t.Parallel()
		inst, pane, lease := setup(t, 0)
		require.Error(t, SubmitContentWithEnter(ctx, pane, lease, "x"))
		waitLeaseFree(t, inst)
	})
	t.Run("SendKeysWithTimeout success", func(t *testing.T) {
		t.Parallel()
		inst, pane, lease := setup(t, -1)
		require.NoError(t, SendKeysWithTimeout(ctx, pane, lease, "x", time.Second))
		waitLeaseFree(t, inst)
	})
	t.Run("SendKeysWithTimeout write error", func(t *testing.T) {
		t.Parallel()
		inst, pane, lease := setup(t, 0)
		require.Error(t, SendKeysWithTimeout(ctx, pane, lease, "x", time.Second))
		waitLeaseFree(t, inst)
	})
	t.Run("busy acquire leaves the holder's lease alone", func(t *testing.T) {
		t.Parallel()
		inst, _, holder := setup(t, -1)
		_, ok := inst.TryTerminalWriteLease(LeaseWriterOther)
		require.False(t, ok)
		holder.Release()
		AssertLeaseFree(t, inst)
	})
}

// waitLeaseFree tolerates the write goroutine's release landing just after the
// primitive returned its result.
func waitLeaseFree(t *testing.T, inst *Instance) {
	t.Helper()
	require.Eventually(t, func() bool {
		l, ok := inst.TryTerminalWriteLease(LeaseWriterOther)
		if ok {
			l.Release()
		}
		return ok
	}, 5*time.Second, time.Millisecond, "the writing goroutine never released the lease")
}

// include is caller code: it must run outside leaseRegistry.mu, or a filter
// that touches another lease (here, taking one) deadlocks the scan.
func TestScanWedgedLeases_ShouldRunIncludeOutsideTheRegistryLock(t *testing.T) {
	inst := leaseInstance(t, "include-unlocked")
	other := leaseInstance(t, "include-unlocked-other")
	lease, ok := inst.TryTerminalWriteLease(LeaseWriterReply)
	require.True(t, ok)
	t.Cleanup(lease.Release)

	done := make(chan struct{})
	go func() {
		defer close(done)
		ScanWedgedLeases(lease.acquiredAt.Add(LeaseWedgeWarnAfter), LeaseWedgeWarnAfter, func(i *Instance) bool {
			if l, got := other.TryTerminalWriteLease(LeaseWriterNudge); got {
				l.Release()
			}
			return i == inst
		})
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("ScanWedgedLeases deadlocked: include ran under leaseRegistry.mu")
	}
}
