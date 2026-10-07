package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session"
)

// guardedSteerRecorder fakes the verify and write hooks and records their order.
type guardedSteerRecorder struct {
	order     []string
	writes    int
	verifyErr error
	writeErr  error
}

func (r *guardedSteerRecorder) install(svc *SessionService, ready notReadyReason, clk *fakeGuardClock) {
	svc.guardedSteer.guard.now = clk.Now
	svc.guardedSteer.ready = func(*session.Instance) notReadyReason { return ready }
	svc.guardedSteer.verifyPane = func(context.Context, *session.Instance) error {
		r.order = append(r.order, "verify")
		return r.verifyErr
	}
	svc.guardedSteer.write = func(context.Context, *session.Instance, string) error {
		r.order = append(r.order, "write")
		r.writes++
		return r.writeErr
	}
}

func newGuardedSteerFixture(t *testing.T, ready notReadyReason) (*SessionService, *session.Instance, *guardedSteerRecorder, *fakeGuardClock) {
	t.Helper()
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)
	inst := &session.Instance{
		Title:     "guarded-steer-session",
		Program:   "claude",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	addInstanceToPoller(fix.poller, inst)
	rec := &guardedSteerRecorder{}
	clk := &fakeGuardClock{t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	rec.install(fix.svc, ready, clk)
	return fix.svc, inst, rec, clk
}

func TestSteerInstanceGuarded_should_WriteOnce_When_IdleAndPaneOwned(t *testing.T) {
	svc, inst, rec, _ := newGuardedSteerFixture(t, notReadyNone)

	out, err := svc.SteerInstanceGuarded(context.Background(), inst, "sigA", "msg")

	require.NoError(t, err)
	assert.Equal(t, SteerDelivered, out)
	assert.Equal(t, 1, rec.writes)
}

func TestSteerInstanceGuarded_should_CallVerifyPaneOwnershipExactlyOncePerWriteBeforeWrite_When_InstanceIdle(t *testing.T) {
	svc, inst, rec, _ := newGuardedSteerFixture(t, notReadyNone)

	_, err := svc.SteerInstanceGuarded(context.Background(), inst, "sigA", "msg")
	require.NoError(t, err)
	assert.Equal(t, []string{"verify", "write"}, rec.order)

	// Same through the UUID entry on a distinct signature.
	rec.order = nil
	out, err := svc.SteerSessionGuarded(context.Background(), inst.GetStableID(), "sigB", "msg")
	require.NoError(t, err)
	assert.Equal(t, SteerDelivered, out)
	assert.Equal(t, []string{"verify", "write"}, rec.order)
}

func TestSteerInstanceGuarded_should_WriteZeroBytesAndNotRecord_When_PaneOwnershipMismatch(t *testing.T) {
	svc, inst, rec, _ := newGuardedSteerFixture(t, notReadyNone)
	rec.verifyErr = errors.New("pane ownership mismatch")

	out, err := svc.SteerInstanceGuarded(context.Background(), inst, "sigA", "msg")
	require.Error(t, err)
	assert.Equal(t, SteerFailed, out)
	assert.Zero(t, rec.writes)

	out, err = svc.SteerSessionGuarded(context.Background(), inst.GetStableID(), "sigB", "msg")
	require.Error(t, err)
	assert.Equal(t, SteerFailed, out)
	assert.Zero(t, rec.writes)

	// Identity fixed: a retry of the same signature delivers exactly once after the 10s cooldown.
	rec.verifyErr = nil
	rec.order = nil
	svc.guardedSteer.guard.now = func() time.Time { return time.Date(2026, 1, 1, 0, 0, 11, 0, time.UTC) }
	out, err = svc.SteerInstanceGuarded(context.Background(), inst, "sigA", "msg")
	require.NoError(t, err)
	assert.Equal(t, SteerDelivered, out)
	assert.Equal(t, 1, rec.writes)
}

func TestSteerInstanceGuarded_should_ReleaseWithFailureAndSkipDuplicateRecord_When_PaneOwnershipMismatch(t *testing.T) {
	svc, inst, rec, clk := newGuardedSteerFixture(t, notReadyNone)
	rec.verifyErr = errors.New("pane ownership mismatch")
	_, err := svc.SteerInstanceGuarded(context.Background(), inst, "sigA", "msg")
	require.Error(t, err)
	rec.verifyErr = nil

	clk.Advance(5 * time.Second)
	out, _ := svc.SteerInstanceGuarded(context.Background(), inst, "sigA", "msg")
	assert.Equal(t, SteerDuplicate, out, "inside the 10s failure cooldown")

	clk.Advance(6 * time.Second)
	out, err = svc.SteerInstanceGuarded(context.Background(), inst, "sigA", "msg")
	require.NoError(t, err)
	assert.Equal(t, SteerDelivered, out, "cooldown, not the 60s delivery window, applied")
}

func TestSteerInstanceGuarded_should_NotVerifyWriteOrRecord_When_NotReady(t *testing.T) {
	for name, tc := range map[string]struct {
		reason notReadyReason
		want   SteerOutcome
	}{
		"busy":             {notReadyBusy, SteerBusy},
		"no status source": {notReadyNoStatusSource, SteerNoStatusSource},
	} {
		t.Run(name, func(t *testing.T) {
			svc, inst, rec, _ := newGuardedSteerFixture(t, tc.reason)

			out, err := svc.SteerInstanceGuarded(context.Background(), inst, "sigA", "msg")

			require.NoError(t, err)
			assert.Equal(t, tc.want, out)
			assert.Empty(t, rec.order)

			// Not-ready leaves no cooldown behind: once idle, the same signature delivers.
			svc.guardedSteer.ready = func(*session.Instance) notReadyReason { return notReadyNone }
			out, err = svc.SteerInstanceGuarded(context.Background(), inst, "sigA", "msg")
			require.NoError(t, err)
			assert.Equal(t, SteerDelivered, out)
		})
	}
}

func TestSteerInstanceGuarded_should_ReturnGuardBusyOrDuplicateWithoutWrite_When_ClaimedOrJustDelivered(t *testing.T) {
	svc, inst, rec, _ := newGuardedSteerFixture(t, notReadyNone)
	_, err := svc.SteerInstanceGuarded(context.Background(), inst, "sigA", "msg")
	require.NoError(t, err)

	out, err := svc.SteerInstanceGuarded(context.Background(), inst, "sigA", "msg")
	require.NoError(t, err)
	assert.Equal(t, SteerDuplicate, out)
	assert.Equal(t, 1, rec.writes)

	release, g := svc.guardedSteer.guard.TryBegin(inst.GetStableID(), "sigZ")
	require.Equal(t, GuardOK, g)
	defer release(false)
	out, err = svc.SteerInstanceGuarded(context.Background(), inst, "sigB", "msg")
	require.NoError(t, err)
	assert.Equal(t, SteerGuardBusy, out)
	assert.Equal(t, 1, rec.writes)
}

func TestSteerSessionGuarded_should_ReturnNotTrackedWithoutWrite_When_FindLiveInstanceNil(t *testing.T) {
	svc, _, rec, _ := newGuardedSteerFixture(t, notReadyNone)

	out, err := svc.SteerSessionGuarded(context.Background(), "no-such-session", "sigA", "msg")

	require.NoError(t, err)
	assert.Equal(t, SteerNotTracked, out)
	assert.Zero(t, rec.writes)
}

func TestSteerInstanceGuarded_should_ReleaseClaim_When_WritePanics(t *testing.T) {
	svc, inst, _, _ := newGuardedSteerFixture(t, notReadyNone)
	svc.guardedSteer.write = func(context.Context, *session.Instance, string) error { panic("boom") }

	require.Panics(t, func() {
		_, _ = svc.SteerInstanceGuarded(context.Background(), inst, "sigA", "msg")
	})

	svc.guardedSteer.write = func(context.Context, *session.Instance, string) error { return nil }
	out, err := svc.SteerInstanceGuarded(context.Background(), inst, "sigB", "msg")
	require.NoError(t, err)
	assert.Equal(t, SteerDelivered, out, "a panic must not leave the session GuardBusy")
}

func TestSteerOutcome_should_NotBeDelivered_When_ZeroValue(t *testing.T) {
	var o SteerOutcome
	assert.Equal(t, SteerUnspecified, o)
	assert.NotEqual(t, SteerDelivered, o)
}

func TestSteerOutcome_String_should_NameEveryOutcome(t *testing.T) {
	for o, want := range map[SteerOutcome]string{
		SteerUnspecified: "unspecified", SteerDelivered: "delivered", SteerGuardBusy: "guard_busy",
		SteerDuplicate: "duplicate", SteerBusy: "busy", SteerNoStatusSource: "no_status_source",
		SteerNotTracked: "not_tracked", SteerFailed: "failed", SteerOutcome(99): "SteerOutcome(99)",
	} {
		assert.Equal(t, want, o.String())
	}
}
