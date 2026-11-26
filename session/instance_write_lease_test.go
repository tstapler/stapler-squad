package session

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/session/instance"
	"github.com/tstapler/stapler-squad/session/internal/fixturetest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// leaseInstance returns an instance with its own flag, so tests never share
// DefaultLeaseFlag state.
func leaseInstance(t *testing.T, uuid string) *instance.Instance {
	t.Helper()
	inst := &instance.Instance{Title: "lease-" + uuid, UUID: uuid}
	inst.SetLeaseFlag(&instance.LeaseFlag{})
	return inst
}

// T-WL-01
func TestWriteLease_ShouldRefuseNilZeroAndForeignLeaseWithZeroWrites_WhenAnyWritePrimitiveIsCalled(t *testing.T) {
	t.Parallel()
	foreign := leaseInstance(t, "someone-else")

	cases := []struct {
		name    string
		lease   func() *instance.HeldLease
		wantErr error
	}{
		{"nil", func() *instance.HeldLease { return nil }, instance.ErrNoLease},
		{"zero value", func() *instance.HeldLease { return &instance.HeldLease{} }, instance.ErrNoLease},
		{"foreign instance", func() *instance.HeldLease {
			l, ok := foreign.TryTerminalWriteLease(instance.LeaseWriterOther)
			require.True(t, ok)
			return l
		}, instance.ErrLeaseMismatch},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			pane := fixturetest.NewFakePaneSubmitter()

			l := tc.lease()
			err := instance.SubmitDriverContent(ctx, pane, l, "x", time.Millisecond, time.Millisecond)
			require.ErrorIs(t, err, tc.wantErr)

			// A refused lease is not reused by the next call: take a fresh one.
			l = tc.lease()
			err = instance.SubmitContentWithEnter(ctx, pane, l, "x")
			require.ErrorIs(t, err, tc.wantErr)

			l = tc.lease()
			err = instance.SendKeysWithTimeout(ctx, pane, l, "x", time.Second)
			require.ErrorIs(t, err, tc.wantErr)

			assert.Empty(t, pane.SendCalls, "a refused lease must perform 0 writes")
		})
	}
	// Every refusal released the foreign lease it was handed.
	instance.AssertLeaseFree(t, foreign)
}

// T-WL-02
func TestWriteLease_ShouldReturnFalseAtOnceForATryAndBusyAfterTheBoundedWaitForAnAcquireAndNeverHang_WhenAFixtureChainAcquiresTwiceForTheSameInstance(t *testing.T) {
	t.Parallel()
	inst := leaseInstance(t, "nest")
	outer, ok := inst.TryTerminalWriteLease(instance.LeaseWriterSteer)
	require.True(t, ok)

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, ok := inst.TryTerminalWriteLease(instance.LeaseWriterSteer)
		assert.False(t, ok, "a nested Try must return false at once")

		start := time.Now()
		_, err := inst.AcquireTerminalWriteLease(context.Background(), instance.LeaseWriterSteer, 30*time.Millisecond)
		assert.ErrorIs(t, err, instance.ErrLeaseBusy)
		assert.GreaterOrEqual(t, time.Since(start), 30*time.Millisecond)

		_, err = inst.AcquireTerminalWriteLease(context.Background(), instance.LeaseWriterSteer, 0)
		assert.ErrorIs(t, err, instance.ErrLeaseBusy, "a zero wait is a Try")
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("a nested acquire hung instead of returning busy")
	}

	outer.Release()
	outer.Release() // idempotent backstop
	instance.AssertLeaseFree(t, inst)
}

func TestWriteLease_ShouldReturnCtxErr_WhenAcquireIsCancelledWhileWaiting(t *testing.T) {
	t.Parallel()
	inst := leaseInstance(t, "cancel")
	held, _ := inst.TryTerminalWriteLease(instance.LeaseWriterSteer)
	defer held.Release()

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		_, err := inst.AcquireTerminalWriteLease(ctx, instance.LeaseWriterDriver, time.Minute)
		errCh <- err
	}()
	cancel()
	require.ErrorIs(t, <-errCh, context.Canceled)
}

func TestWriteLease_ShouldHandTheLeaseToAWaiter_WhenTheHolderReleases(t *testing.T) {
	t.Parallel()
	inst := leaseInstance(t, "handoff")
	held, _ := inst.TryTerminalWriteLease(instance.LeaseWriterSteer)

	got := make(chan *instance.HeldLease, 1)
	go func() {
		l, err := inst.AcquireTerminalWriteLease(context.Background(), instance.LeaseWriterAutonomous, 10*time.Second)
		assert.NoError(t, err)
		got <- l
	}()
	held.Release()
	l := <-got
	require.NotNil(t, l)
	l.Release()
	instance.AssertLeaseFree(t, inst)
}

// T-WL-10: every exit path of the three primitives leaves the lease free.
func TestEveryAcquirer_ShouldLeaveTheLeaseFreeOnEveryExitPath_WhenSuccessBusyWriteErrorCancelledContextAndEarlyReturnAreExercised(t *testing.T) {
	t.Parallel()
	// Each case gets its own instance bound to the fake pane's lease identity.
	setup := func(t *testing.T, failOnCall int) (*instance.Instance, *fixturetest.FakePaneSubmitter, *instance.HeldLease) {
		t.Helper()
		inst := leaseInstance(t, fixturetest.FakeLeaseOwner)
		pane := fixturetest.NewFakePaneSubmitter()
		pane.FailOnCall = failOnCall
		lease, ok := inst.TryTerminalWriteLease(instance.LeaseWriterOther)
		require.True(t, ok)
		return inst, pane, lease
	}
	ctx := context.Background()

	t.Run("SubmitDriverContent success", func(t *testing.T) {
		t.Parallel()
		inst, pane, lease := setup(t, -1)
		require.NoError(t, instance.SubmitDriverContent(ctx, pane, lease, "x", time.Millisecond, 20*time.Millisecond))
		instance.AssertLeaseFree(t, inst)
	})
	t.Run("SubmitDriverContent write error", func(t *testing.T) {
		t.Parallel()
		inst, pane, lease := setup(t, 0)
		require.Error(t, instance.SubmitDriverContent(ctx, pane, lease, "x", time.Millisecond, 20*time.Millisecond))
		instance.AssertLeaseFree(t, inst)
	})
	t.Run("SubmitDriverContent cancelled context", func(t *testing.T) {
		t.Parallel()
		inst, pane, lease := setup(t, -1)
		cancelled, cancel := context.WithCancel(ctx)
		cancel()
		require.Error(t, instance.SubmitDriverContent(cancelled, pane, lease, "x", time.Millisecond, 20*time.Millisecond))
		instance.AssertLeaseFree(t, inst)
	})
	t.Run("SubmitDriverContent pane owner refusal", func(t *testing.T) {
		t.Parallel()
		inst, pane, lease := setup(t, -1)
		sub := &ownerCheckedSubmitter{FakePaneSubmitter: pane, ownerErr: errors.New("owner mismatch")}
		require.Error(t, instance.SubmitDriverContent(ctx, sub, lease, "x", time.Millisecond, 20*time.Millisecond))
		instance.AssertLeaseFree(t, inst)
		assert.Empty(t, pane.SendCalls)
	})
	t.Run("SubmitContentWithEnter success", func(t *testing.T) {
		t.Parallel()
		inst, pane, lease := setup(t, -1)
		require.NoError(t, instance.SubmitContentWithEnter(ctx, pane, lease, "x"))
		waitLeaseFree(t, inst)
	})
	t.Run("SubmitContentWithEnter write error", func(t *testing.T) {
		t.Parallel()
		inst, pane, lease := setup(t, 0)
		require.Error(t, instance.SubmitContentWithEnter(ctx, pane, lease, "x"))
		waitLeaseFree(t, inst)
	})
	t.Run("SendKeysWithTimeout success", func(t *testing.T) {
		t.Parallel()
		inst, pane, lease := setup(t, -1)
		require.NoError(t, instance.SendKeysWithTimeout(ctx, pane, lease, "x", time.Second))
		waitLeaseFree(t, inst)
	})
	t.Run("SendKeysWithTimeout write error", func(t *testing.T) {
		t.Parallel()
		inst, pane, lease := setup(t, 0)
		require.Error(t, instance.SendKeysWithTimeout(ctx, pane, lease, "x", time.Second))
		waitLeaseFree(t, inst)
	})
	t.Run("busy acquire leaves the holder's lease alone", func(t *testing.T) {
		t.Parallel()
		inst, _, holder := setup(t, -1)
		_, ok := inst.TryTerminalWriteLease(instance.LeaseWriterOther)
		require.False(t, ok)
		holder.Release()
		instance.AssertLeaseFree(t, inst)
	})
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

// include is caller code: it must run outside leaseRegistry.mu, or a filter
// that touches another lease (here, taking one) deadlocks the scan.
func TestScanWedgedLeases_ShouldRunIncludeOutsideTheRegistryLock(t *testing.T) {
	inst := leaseInstance(t, "include-unlocked")
	other := leaseInstance(t, "include-unlocked-other")
	lease, ok := inst.TryTerminalWriteLease(instance.LeaseWriterReply)
	require.True(t, ok)
	t.Cleanup(lease.Release)

	done := make(chan struct{})
	go func() {
		defer close(done)
		instance.ScanWedgedLeases(lease.AcquiredAtForTest().Add(instance.LeaseWedgeWarnAfter), instance.LeaseWedgeWarnAfter, func(i *instance.Instance) bool {
			if l, got := other.TryTerminalWriteLease(instance.LeaseWriterNudge); got {
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
