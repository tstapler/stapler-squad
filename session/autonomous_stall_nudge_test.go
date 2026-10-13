package session

// These tests live in the session package because only here can a cancelled
// ctx, a held lease and a pane that records keystrokes be observed together. A
// service test with a fake nudger cannot prove promptness or no-write.

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/domain"
)

const (
	nudgeTestIdlePane      = "I've fixed the null check in validator.go.\n> "
	nudgeTestExecutingPane = "  esc to interrupt\n"
)

// silentPaneProcessManager never reports a pane change, so a submit is never
// confirmed.
type silentPaneProcessManager struct{ fakeSendKeysProcessManager }

func (s *silentPaneProcessManager) HasUpdated() (bool, bool, string) { return false, false, "" }

type nudgePane interface {
	ProcessManager
	recorded() []string
}

func newNudgeWriterInstance(t *testing.T, paneContent string, pm nudgePane) *Instance {
	t.Helper()
	cc, _ := newControllerWithMock(paneContent)
	inst := minimalInstance(t)
	inst.ForceStatus(Active)
	inst.controllerManager.SetController(cc)
	inst.processManager = pm
	inst.started.Store(true)
	return inst
}

// testNudgeSeams skips the settle window and shrinks the pane waits.
func testNudgeSeams(inst *Instance) nudgeSeams {
	return nudgeSeams{
		poll:     time.Millisecond,
		tryLease: inst.TryTerminalWriteLease,
		submit: func(ctx context.Context, i paneSubmitter, l *HeldLease, content string, _, _ time.Duration) error {
			return SubmitDriverContent(ctx, i, l, content, time.Millisecond, 20*time.Millisecond)
		},
	}
}

type claimCounter struct {
	calls   atomic.Int32
	granted bool
}

func (c *claimCounter) claim() (bool, error) {
	c.calls.Add(1)
	return c.granted, nil
}

// cancelAfterDoneCalls cancels itself on its Nth Done() call, i.e. inside the
// settle loop's select, without any real sleep.
type cancelAfterDoneCalls struct {
	context.Context
	after int32
	calls atomic.Int32
	done  chan struct{}
	once  sync.Once
}

func newCancelAfterDoneCalls(after int32) *cancelAfterDoneCalls {
	return &cancelAfterDoneCalls{Context: context.Background(), after: after, done: make(chan struct{})}
}

func (c *cancelAfterDoneCalls) Done() <-chan struct{} {
	if c.calls.Add(1) >= c.after {
		c.once.Do(func() { close(c.done) })
	}
	return c.done
}

func (c *cancelAfterDoneCalls) Err() error {
	select {
	case <-c.done:
		return context.Canceled
	default:
		return nil
	}
}

// cancelAfterErrCalls reports nil from Err() for the first `ok` calls, then
// context.Canceled, so a test can cancel between two specific checks.
type cancelAfterErrCalls struct {
	context.Context
	ok    int32
	calls atomic.Int32
}

func (c *cancelAfterErrCalls) Err() error {
	if c.calls.Add(1) > c.ok {
		return context.Canceled
	}
	return nil
}

func runWithWatchdog(t *testing.T, fn func() error) error {
	t.Helper()
	errCh := make(chan error, 1)
	go func() { errCh <- fn() }()
	select {
	case err := <-errCh:
		return err
	case <-time.After(30 * time.Second):
		t.Fatal("nudge writer did not return: it is not honouring ctx cancellation")
		return nil
	}
}

// NDG-1
func TestNudgeStalledAutonomousSession_NotIdle_ClaimUntouched(t *testing.T) {
	t.Parallel()
	pm := &fakeSendKeysProcessManager{}
	inst := newNudgeWriterInstance(t, nudgeTestExecutingPane, pm)
	cc := &claimCounter{granted: true}

	err := nudgeStalledWith(context.Background(), inst, cc.claim, testNudgeSeams(inst))

	require.ErrorIs(t, err, ErrNudgeTargetNotIdle)
	assert.Zero(t, cc.calls.Load())
	assert.Empty(t, pm.recorded())
	AssertLeaseFree(t, inst)
}

// NDG-2
func TestNudgeStalledAutonomousSession_LeaseBusy_ClaimUntouched(t *testing.T) {
	t.Parallel()
	pm := &fakeSendKeysProcessManager{}
	inst := newNudgeWriterInstance(t, nudgeTestIdlePane, pm)
	held, ok := inst.TryTerminalWriteLease(LeaseWriterDriver)
	require.True(t, ok)
	cc := &claimCounter{granted: true}

	err := nudgeStalledWith(context.Background(), inst, cc.claim, testNudgeSeams(inst))

	require.ErrorIs(t, err, ErrLeaseBusy)
	assert.Zero(t, cc.calls.Load())
	assert.Empty(t, pm.recorded())
	held.Release()
	AssertLeaseFree(t, inst)
}

// NDG-3
func TestNudgeStalledAutonomousSession_ClaimLost_ReleasesLeaseNoWrite(t *testing.T) {
	t.Parallel()
	pm := &fakeSendKeysProcessManager{}
	inst := newNudgeWriterInstance(t, nudgeTestIdlePane, pm)
	cc := &claimCounter{granted: false}

	err := nudgeStalledWith(context.Background(), inst, cc.claim, testNudgeSeams(inst))

	require.ErrorIs(t, err, ErrNudgeSlotUnavailable)
	assert.EqualValues(t, 1, cc.calls.Load())
	assert.Empty(t, pm.recorded())
	AssertLeaseFree(t, inst)
}

// NDG-4
func TestNudgeStalledAutonomousSession_RefusesNonActiveArchivedPaused(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		setup func(*Instance)
	}{
		{"not active", func(i *Instance) { i.ForceStatus(Creating) }},
		{"archived", func(i *Instance) { i.SetArchivedAtIfNil(time.Now()) }},
		{"paused", func(i *Instance) { i.ForceStatus(Paused) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			pm := &fakeSendKeysProcessManager{}
			inst := newNudgeWriterInstance(t, nudgeTestIdlePane, pm)
			tt.setup(inst)
			cc := &claimCounter{granted: true}

			err := nudgeStalledWith(context.Background(), inst, cc.claim, testNudgeSeams(inst))

			require.Error(t, err)
			assert.NotErrorIs(t, err, ErrNudgeTargetNotIdle)
			assert.Zero(t, cc.calls.Load())
			assert.Empty(t, pm.recorded())
			AssertLeaseFree(t, inst)
		})
	}
}

// NDG-5: an hour-long window with a 1ms poll means a regression to
// time.Sleep hangs here and trips the watchdog instead of passing by luck.
func TestNudgeStalledAutonomousSession_CancelMidSettle_NoWriteNoLeaseNoClaim(t *testing.T) {
	t.Parallel()
	pm := &fakeSendKeysProcessManager{}
	inst := newNudgeWriterInstance(t, nudgeTestIdlePane, pm)
	cc := &claimCounter{granted: true}
	var leaseTaken atomic.Int32
	seams := testNudgeSeams(inst)
	seams.settleWindow = time.Hour
	seams.tryLease = func(w string) (*HeldLease, bool) {
		leaseTaken.Add(1)
		return inst.TryTerminalWriteLease(w)
	}
	ctx := newCancelAfterDoneCalls(3)

	err := runWithWatchdog(t, func() error { return nudgeStalledWith(ctx, inst, cc.claim, seams) })

	require.ErrorIs(t, err, context.Canceled)
	assert.Empty(t, pm.recorded())
	assert.Zero(t, leaseTaken.Load())
	assert.Zero(t, cc.calls.Load())
	AssertLeaseFree(t, inst)
}

// NDG-6: Err() passes inside the gate, then reports cancelled before the lease.
func TestNudgeStalledAutonomousSession_CancelAfterGateBeforeLease(t *testing.T) {
	t.Parallel()
	pm := &fakeSendKeysProcessManager{}
	inst := newNudgeWriterInstance(t, nudgeTestIdlePane, pm)
	cc := &claimCounter{granted: true}
	var leaseTaken atomic.Int32
	seams := testNudgeSeams(inst)
	seams.tryLease = func(w string) (*HeldLease, bool) {
		leaseTaken.Add(1)
		return inst.TryTerminalWriteLease(w)
	}
	ctx := &cancelAfterErrCalls{Context: context.Background(), ok: 1}

	err := nudgeStalledWith(ctx, inst, cc.claim, seams)

	require.ErrorIs(t, err, context.Canceled)
	assert.Empty(t, pm.recorded())
	assert.Zero(t, leaseTaken.Load())
	assert.Zero(t, cc.calls.Load())
	AssertLeaseFree(t, inst)
}

// NDG-7: an unconfirmed submit keeps the slot (the row count stays
// incremented) and wraps ErrNudgeSubmitUncertain.
func TestNudgeStalledAutonomousSession_SubmitNotConfirmed_UncertainNoRefund(t *testing.T) {
	t.Parallel()
	storage, _, itemID, ctx := newDiagnoseNudgeTestFixture(t)
	pm := &silentPaneProcessManager{}
	inst := newNudgeWriterInstance(t, nudgeTestIdlePane, pm)
	claim := func() (bool, error) {
		res, err := storage.ClaimDiagnoseNudgeSlot(ctx, itemID, domain.StuckReasonBouncing, 1)
		return res.Claimed, err
	}

	err := nudgeStalledWith(ctx, inst, claim, testNudgeSeams(inst))

	require.ErrorIs(t, err, ErrNudgeSubmitUncertain)
	require.ErrorIs(t, err, ErrSubmitNotConfirmed)
	rows, findErr := storage.FindOpenStuckStates(ctx)
	require.NoError(t, findErr)
	require.Len(t, rows, 1)
	assert.Equal(t, int32(1), rows[0].DiagnoseNudgeCount, "no refund after an uncertain delivery")
	AssertLeaseFree(t, inst)
}

// NDG-8
func TestNudgeStalledAutonomousSession_FailureAfterTextSent_Uncertain(t *testing.T) {
	t.Parallel()
	pm := &fakeSendKeysProcessManager{failOn: map[int]bool{2: true}} // the Enter keystroke
	inst := newNudgeWriterInstance(t, nudgeTestIdlePane, pm)
	cc := &claimCounter{granted: true}

	err := nudgeStalledWith(context.Background(), inst, cc.claim, testNudgeSeams(inst))

	require.ErrorIs(t, err, ErrNudgeSubmitUncertain)
	require.ErrorIs(t, err, errTestSendKeysFailure)
	assert.Equal(t, []string{autonomousStallNudgeText}, pm.recorded())
	AssertLeaseFree(t, inst)
}

// NDG-9
func TestAutonomousStallNudgeText_ExactBytes(t *testing.T) {
	t.Parallel()
	const want = "The autonomous driver noticed this session stopped. If the task is finished, call request_review now. If it is not, continue working on it and call report_progress when you reach a milestone. Do not ask for confirmation."
	assert.Equal(t, want, autonomousStallNudgeText)
	assert.NotContains(t, autonomousStallNudgeText, "%")

	pm := &fakeSendKeysProcessManager{}
	inst := newNudgeWriterInstance(t, nudgeTestIdlePane, pm)
	cc := &claimCounter{granted: true}

	require.NoError(t, nudgeStalledWith(context.Background(), inst, cc.claim, testNudgeSeams(inst)))

	assert.Equal(t, []string{want, EnterKeySequence}, pm.recorded())
	AssertLeaseFree(t, inst)
}
