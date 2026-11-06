package services

import (
	"context"
	"fmt"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
)

func leaseTestInstance(t *testing.T, title string) (*session.Instance, *steerRecordingPM) {
	t.Helper()
	pm := &steerRecordingPM{}
	inst := session.NewStartedInstanceForTest(t, title, pm)
	inst.SetLeaseFlag(&session.LeaseFlag{})
	return inst, pm
}

// T-WL-04 (steer half): the visible steer returns a retryable error with 0
// writes while the lease is held, and writes once it frees.
func TestSteerInstance_ShouldReturnRetryableBusyWithZeroWrites_WhenTheLeaseIsHeld(t *testing.T) {
	fix := setupForkTestFixture(t)
	inst, pm := leaseTestInstance(t, "steer-lease")
	held, ok := inst.TryTerminalWriteLease(session.LeaseWriterDriver)
	require.True(t, ok)

	err := fix.svc.steerInternal(context.Background(), inst, "fix it")
	require.ErrorIs(t, err, session.ErrLeaseBusy)
	assert.Empty(t, pm.writes())

	held.Release()
	require.NoError(t, fix.svc.steerInternal(context.Background(), inst, "fix it"))
	assert.Equal(t, []string{"fix it", session.EnterKeySequence}, pm.writes())
	session.AssertLeaseFree(t, inst)
}

// A held lease is SteerBusy with no failure cooldown, so the same signature
// delivers as soon as the lease frees.
func TestSteerInstanceGuarded_ShouldReturnSteerBusyWithoutFailureCooldown_WhenTheLeaseIsHeld(t *testing.T) {
	fix := setupForkTestFixture(t)
	inst, pm := leaseTestInstance(t, "guarded-lease")
	fix.svc.guardedSteer.ready = func(*session.Instance) notReadyReason { return notReadyNone }
	fix.svc.guardedSteer.verifyPane = func(context.Context, *session.Instance) error { return nil }
	held, ok := inst.TryTerminalWriteLease(session.LeaseWriterDriver)
	require.True(t, ok)

	out, err := fix.svc.SteerInstanceGuarded(context.Background(), inst, "sigA", "msg")
	require.NoError(t, err)
	assert.Equal(t, SteerBusy, out)
	assert.Empty(t, pm.writes())

	held.Release()
	out, err = fix.svc.SteerInstanceGuarded(context.Background(), inst, "sigA", "msg")
	require.NoError(t, err)
	assert.Equal(t, SteerDelivered, out, "no failure cooldown was recorded for the busy attempt")
}

func writeToSessionReq(id string) *connect.Request[sessionv1.WriteToSessionRequest] {
	return connect.NewRequest(&sessionv1.WriteToSessionRequest{SessionId: id, Input: "ls", PressEnter: true})
}

func newLeaseTerminalService(t *testing.T, inst *session.Instance) *TerminalService {
	t.Helper()
	poller := session.NewReviewQueuePoller(session.NewReviewQueue(), session.NewInstanceStatusManager(), nil)
	poller.SetInstances([]*session.Instance{inst})
	ts := NewTerminalService()
	ts.SetPoller(poller)
	return ts
}

// T-WL-04 (WriteToSession half) and T-WL-11's flag-off path.
func TestWriteToSession_ShouldReturnFailedPreconditionWithZeroWrites_WhenTheLeaseIsHeldAndWriteWhenTheFlagIsOff(t *testing.T) {
	inst, pm := leaseTestInstance(t, "wts-lease")
	ts := newLeaseTerminalService(t, inst)
	held, ok := inst.TryTerminalWriteLease(session.LeaseWriterDriver)
	require.True(t, ok)

	_, err := ts.WriteToSession(context.Background(), writeToSessionReq("wts-lease"))
	require.Error(t, err)
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
	assert.Empty(t, pm.writes())

	// Flag off: non-exclusive leases, so the write proceeds exactly as before.
	flag := &session.LeaseFlag{}
	flag.SetEnabled(false)
	inst.SetLeaseFlag(flag)
	_, err = ts.WriteToSession(context.Background(), writeToSessionReq("wts-lease"))
	require.NoError(t, err)
	assert.Equal(t, []string{"ls", session.EnterKeySequence}, pm.writes())

	held.Release()
	flag.SetEnabled(true)
	_, err = ts.WriteToSession(context.Background(), writeToSessionReq("wts-lease"))
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		l, free := inst.TryTerminalWriteLease(session.LeaseWriterOther)
		if free {
			l.Release()
		}
		return free
	}, 5*time.Second, time.Millisecond)
}

// The default compactor is the chain's acquirer: a busy lease is an error the
// monitor already logs, not a blocked cycle.
func TestCapacityMonitorDefaultCompactor_ShouldReturnErrLeaseBusyWithZeroWrites_WhenTheLeaseIsHeld(t *testing.T) {
	inst, pm := leaseTestInstance(t, "compactor-lease")
	m := NewCapacityMonitor(CapacityMonitorParams{})
	held, ok := inst.TryTerminalWriteLease(session.LeaseWriterDriver)
	require.True(t, ok)

	err := m.compactor(context.Background(), inst, "/compact")
	require.ErrorIs(t, err, session.ErrLeaseBusy)
	assert.Empty(t, pm.writes())

	held.Release()
	require.NoError(t, m.compactor(context.Background(), inst, "/compact"))
	assert.Equal(t, []string{"/compact", session.EnterKeySequence}, pm.writes(), fmt.Sprint(pm.writes()))
}
