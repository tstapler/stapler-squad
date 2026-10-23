package services

// Nudge-versus-respawn decision tests (Tasks 3.2.3b-3.2.3d): every observed
// state, every flag combination, every nudge failure class, and a stop that
// arrives mid-recovery. The nudger and driver restarter are fakes; the stuck
// row, the nudge slot claim and the instance state are real.

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

// fakeStallNudger runs the real claim (so the slot cap and cooldown are exercised) and
// then returns err.
type fakeStallNudger struct {
	mu      sync.Mutex
	calls   int
	err     error
	preFn   func(ctx context.Context) error // runs before the claim; may block
	granted []bool
}

func (n *fakeStallNudger) NudgeStalledSession(ctx context.Context, _ *session.Instance, claim func() (bool, error)) error {
	n.mu.Lock()
	n.calls++
	pre, err := n.preFn, n.err
	n.mu.Unlock()
	if pre != nil {
		if perr := pre(ctx); perr != nil {
			return perr
		}
	}
	if err != nil {
		return err
	}
	ok, cerr := claim()
	n.mu.Lock()
	n.granted = append(n.granted, ok)
	n.mu.Unlock()
	if cerr != nil {
		return cerr
	}
	if !ok {
		return session.ErrNudgeSlotUnavailable
	}
	return nil
}

func (n *fakeStallNudger) callCount() int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.calls
}

type stallNudgeFixture struct {
	*charFixture
	nudger    *fakeStallNudger
	respawner *signalingRespawner
	restarts  int
	restartFn func() error
}

func newStallNudgeFixture(t *testing.T, title string, itemStatus session.BacklogStatus, diagnoseOn, stallOn bool) *stallNudgeFixture {
	t.Helper()
	base := newUnlinkedCharFixtureWithStatus(t, title, session.Active)
	item, err := base.storage.CreateBacklogItem(t.Context(), session.BacklogItemData{Title: title + " item", Status: string(itemStatus)})
	require.NoError(t, err)
	is, err := base.storage.CreateItemSession(t.Context(), session.ItemSessionData{ItemID: item.ID, SessionUUID: base.inst.UUID, SessionRole: session.SessionRoleWork})
	require.NoError(t, err)
	base.itemID, base.itemSes = item.ID, is.ID

	f := &stallNudgeFixture{charFixture: base, nudger: &fakeStallNudger{}, respawner: newSignalingRespawner()}
	svc := base.svc.autonomousSvc
	svc.SetStallNudger(f.nudger)
	svc.SetNudgeFlagReaders(func() bool { return diagnoseOn }, func() bool { return stallOn })
	svc.SetAutonomousStuckRespawner(f.respawner)
	svc.SetDriverRestarter(func(*session.Instance, time.Duration) error {
		f.restarts++
		if f.restartFn != nil {
			return f.restartFn()
		}
		return nil
	})
	return f
}

func (f *stallNudgeFixture) mode() bool { return f.inst.Snapshot().Autonomous.AutonomousMode }

// requireRespawn waits for the async respawn dispatch. requireNoRespawn uses a
// synchronous oracle: respawnStalledWork ends the ItemSession before it spawns the
// dispatch goroutine, so an open ItemSession proves no respawn was started.
func (f *stallNudgeFixture) requireRespawn(t *testing.T) {
	t.Helper()
	select {
	case <-f.respawner.called:
	case <-time.After(5 * time.Second):
		t.Fatal("expected a respawn dispatch")
	}
}

func (f *stallNudgeFixture) requireNoRespawn(t *testing.T) {
	t.Helper()
	is, err := f.storage.GetItemSessionBySessionUUID(context.Background(), f.inst.UUID)
	require.NoError(t, err)
	assert.Nil(t, is.EndedAt, "no respawn expected: the ItemSession must stay open")
}

func (f *stallNudgeFixture) requireRespawnIf(t *testing.T, want bool) {
	t.Helper()
	if want {
		f.requireRespawn(t)
	} else {
		f.requireNoRespawn(t)
	}
}

func (f *stallNudgeFixture) rowContext(t *testing.T) string {
	t.Helper()
	return f.openStuckRow(t).Context
}

func snapshotFor(state domain.ObservedState) domain.StopSnapshot {
	s := idleSnapshot
	switch state {
	case domain.ObservedIdlePrompt:
	case domain.ObservedWorking:
		s.Bucket = domain.BucketActive
	case domain.ObservedNeedsApproval:
		s.Bucket = domain.BucketNeedsInput
	case domain.ObservedError:
		s.Bucket = domain.BucketError
	case domain.ObservedIdle:
		s.Bucket = domain.BucketIdle
	case domain.ObservedExited:
		s.TmuxAlive = false
	case domain.ObservedCrashed:
		s.PaneDead, s.ExitCode = true, 2
	case domain.ObservedUnknown:
		s.Bucket = domain.BucketUnknown
	}
	return s
}

func stallIn(state domain.ObservedState, reason domain.StopReason) session.AutonomousDriverOutcome {
	return session.AutonomousDriverOutcome{StopReason: reason, Turns: 2, Snapshot: snapshotFor(state), RunID: 9}
}

func TestStallRecovery_IdlePromptNudgesAndKeepsAutonomy(t *testing.T) {
	f := newStallNudgeFixture(t, "nudge-happy", session.BacklogStatusInProgress, true, true)

	f.complete(stallIn(domain.ObservedIdlePrompt, domain.StopReasonOrchestratorError))

	assert.Equal(t, 1, f.nudger.callCount())
	assert.Equal(t, 1, f.restarts, "the driver is re-attached after a delivered nudge")
	f.requireNoRespawn(t)
	assert.True(t, f.mode(), "autonomy stays on across a nudge")
	assert.Empty(t, f.inst.Snapshot().Autonomous.AutonomousStopReason, "a re-attached driver clears the observed stop reason")
	row := f.rowContext(t)
	assert.Contains(t, row, "action=nudged nudge=1/2")
	assert.Contains(t, row, "nudge_outcome=success")
	assert.Equal(t, []string{"Autonomous session nudged"}, notificationTitles(f.drain()),
		"one INFO notification, no 'stuck' alarm")
}

func TestStallRecovery_StateDecidesNudgeOrRespawn(t *testing.T) {
	cases := []struct {
		state       domain.ObservedState
		wantNudge   bool
		wantRespawn bool
	}{
		{domain.ObservedIdlePrompt, true, false},
		{domain.ObservedWorking, false, true},
		{domain.ObservedNeedsApproval, false, true},
		{domain.ObservedError, false, true},
		{domain.ObservedIdle, false, true},
		{domain.ObservedExited, false, true},
		{domain.ObservedCrashed, false, true},
		{domain.ObservedUnknown, false, true},
	}
	for _, tc := range cases {
		t.Run(string(tc.state), func(t *testing.T) {
			f := newStallNudgeFixture(t, "state-"+string(tc.state), session.BacklogStatusInProgress, true, true)

			f.complete(stallIn(tc.state, domain.StopReasonOrchestratorError))

			assert.Equal(t, tc.wantNudge, f.nudger.callCount() == 1)
			f.requireRespawnIf(t, tc.wantRespawn)
			if tc.wantRespawn {
				assert.False(t, f.mode(), "a respawn ends autonomy on the old session")
				assert.Equal(t, 0, f.restarts)
			}
		})
	}
}

func TestStallRecovery_NudgeNotChosen(t *testing.T) {
	cases := []struct {
		name       string
		diagnose   bool
		stall      bool
		status     session.BacklogStatus
		reason     domain.StopReason
		wantOutcom string
	}{
		{"diagnose flag off", false, true, session.BacklogStatusInProgress, domain.StopReasonOrchestratorError, "nudge_outcome=disabled"},
		{"stall flag off", true, false, session.BacklogStatusInProgress, domain.StopReasonOrchestratorError, "nudge_outcome=disabled"},
		{"both flags off", false, false, session.BacklogStatusInProgress, domain.StopReasonOrchestratorError, "nudge_outcome=disabled"},
		{"item already in review", true, true, session.BacklogStatusReview, domain.StopReasonOrchestratorError, "nudge_outcome=n/a"},
		{"turn cap", true, true, session.BacklogStatusInProgress, domain.StopReasonMaxTurns, "nudge_outcome=n/a"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newStallNudgeFixture(t, "nochoice-"+fmt.Sprint(len(tc.name)), tc.status, tc.diagnose, tc.stall)

			f.complete(stallIn(domain.ObservedIdlePrompt, tc.reason))

			assert.Zero(t, f.nudger.callCount(), "no pane write when the nudge is not chosen")
			f.requireRespawn(t)
			assert.Contains(t, f.rowContext(t), tc.wantOutcom)
		})
	}
}

func TestStallRecovery_NudgeFailureFallsBackOrLeavesPane(t *testing.T) {
	cases := []struct {
		name        string
		err         error
		wantRespawn bool
		wantOutcome string
		wantAction  string
	}{
		{"not idle", session.ErrNudgeTargetNotIdle, true, "nudge_outcome=not_idle", "action=respawned"},
		{"lease busy", session.ErrLeaseBusy, true, "nudge_outcome=lease_busy", "action=respawned"},
		{"slot unavailable", session.ErrNudgeSlotUnavailable, true, "nudge_outcome=slot_unavailable", "action=respawned"},
		{"ownership", errors.New("pane owned by another session"), true, "nudge_outcome=ownership", "action=respawned"},
		{"uncertain delivery", fmt.Errorf("%w: boom", session.ErrNudgeSubmitUncertain), false, "nudge_outcome=uncertain", "action=none"},
		{"cancelled", context.Canceled, false, "nudge_outcome=ctx_cancelled", "action=none"},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newStallNudgeFixture(t, fmt.Sprintf("fail-%d", i), session.BacklogStatusInProgress, true, true)
			f.nudger.err = tc.err

			f.complete(stallIn(domain.ObservedIdlePrompt, domain.StopReasonOrchestratorError))

			f.requireRespawnIf(t, tc.wantRespawn)
			assert.Zero(t, f.restarts, "no driver re-attach without a delivered nudge")
			assert.False(t, f.mode(), "no driver is left running, so autonomy reads off")
			row := f.rowContext(t)
			assert.Contains(t, row, tc.wantOutcome)
			assert.Contains(t, row, tc.wantAction)
		})
	}
}

func TestStallRecovery_RestartFailureFallsBackToRespawn(t *testing.T) {
	f := newStallNudgeFixture(t, "restart-fails", session.BacklogStatusInProgress, true, true)
	f.restartFn = func() error { return errors.New("pool unavailable") }

	f.complete(stallIn(domain.ObservedIdlePrompt, domain.StopReasonOrchestratorError))

	f.requireRespawn(t)
	assert.Contains(t, f.rowContext(t), "nudge_outcome=restart_failed")
	assert.False(t, f.mode())
}

func TestStallRecovery_SecondStallInCooldownRespawns(t *testing.T) {
	f := newStallNudgeFixture(t, "cooldown", session.BacklogStatusInProgress, true, true)
	f.complete(stallIn(domain.ObservedIdlePrompt, domain.StopReasonOrchestratorError))
	f.requireNoRespawn(t)

	f.complete(stallIn(domain.ObservedIdlePrompt, domain.StopReasonOrchestratorError))

	f.requireRespawn(t)
	assert.Equal(t, 1, f.restarts, "only the first stall re-attached a driver")
}

func TestStallRecovery_ArchivedSessionIsNeverRecovered(t *testing.T) {
	f := newStallNudgeFixture(t, "archived", session.BacklogStatusInProgress, true, true)
	now := time.Now()
	f.inst.SetArchivedAt(&now)

	f.complete(stallIn(domain.ObservedIdlePrompt, domain.StopReasonOrchestratorError))

	assert.Zero(t, f.nudger.callCount())
	f.requireNoRespawn(t)
	assert.Zero(t, f.restarts)
}

func TestStallRecovery_StopDuringNudgeCancelsIt(t *testing.T) {
	f := newStallNudgeFixture(t, "stop-mid-nudge", session.BacklogStatusInProgress, true, true)
	started := make(chan struct{})
	var startedOnce sync.Once
	f.nudger.preFn = func(ctx context.Context) error {
		startedOnce.Do(func() { close(started) })
		<-ctx.Done()
		return ctx.Err()
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		f.complete(stallIn(domain.ObservedIdlePrompt, domain.StopReasonOrchestratorError))
	}()
	select {
	case <-started:
	case <-time.After(10 * time.Second):
		t.Fatal("the nudger was never reached")
	}

	f.svc.autonomousSvc.StopDriverForSession(f.title)

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("recovery did not return after the stop was requested")
	}
	f.requireNoRespawn(t)
	assert.Zero(t, f.restarts)
	assert.False(t, f.mode())
}

func TestStallNudgeFlag_ShippedDefaultIsOff(t *testing.T) {
	assert.False(t, config.AutonomousStallNudgeDefault, "flipping the shipped default must edit this test")
	assert.False(t, config.EffectiveAutonomousStallNudgeEnabled(&config.Config{}))
	assert.True(t, config.EffectiveAutonomousStallNudgeEnabled(&config.Config{FeatureFlags: map[string]bool{config.AutonomousStallNudgeFeatureFlag: true}}))
}

func TestClassifyNudgeError(t *testing.T) {
	cases := []struct {
		name        string
		err         error
		wantOutcome nudgeOutcome
		wantLeave   bool
	}{
		{"nil", nil, nudgeOutcomeSuccess, false},
		{"not idle", fmt.Errorf("x: %w", session.ErrNudgeTargetNotIdle), nudgeOutcomeNotIdle, false},
		{"lease busy", session.ErrLeaseBusy, nudgeOutcomeLeaseBusy, false},
		{"slot unavailable", session.ErrNudgeSlotUnavailable, nudgeOutcomeSlotUnavailable, false},
		{"claim failed", fmt.Errorf("%w: db down", session.ErrNudgeClaimFailed), nudgeOutcomeSlotUnavailable, false},
		{"uncertain", fmt.Errorf("%w: %w", session.ErrNudgeSubmitUncertain, errors.New("io")), nudgeOutcomeUncertain, true},
		{"uncertain wrapping cancel", fmt.Errorf("%w: %w", session.ErrNudgeSubmitUncertain, context.Canceled), nudgeOutcomeUncertain, true},
		{"deadline", context.DeadlineExceeded, nudgeOutcomeCtxCancelled, true},
		{"cancelled", context.Canceled, nudgeOutcomeCtxCancelled, true},
		{"session not active", fmt.Errorf("%w: uuid", session.ErrNudgeSessionNotActive), nudgeOutcomeNotRestartable, true},
		{"not restartable", errNotRestartable, nudgeOutcomeNotRestartable, true},
		{"other", errors.New("anything else"), nudgeOutcomeOwnership, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, leave := classifyNudgeError(tc.err)
			assert.Equal(t, tc.wantOutcome, got)
			assert.Equal(t, tc.wantLeave, leave)
		})
	}
}

// A session that is no longer restartable (autonomy turned off before the nudge)
// must never fall through to a respawn, and a requested stop raises no alarm.
func TestStallRecovery_NotRestartableLeavesPaneAndStaysQuiet(t *testing.T) {
	f := newStallNudgeFixture(t, "not-restartable", session.BacklogStatusInProgress, true, true)
	f.inst.SetAutonomousMode(false, "")

	f.complete(stallIn(domain.ObservedIdlePrompt, domain.StopReasonOrchestratorError))

	assert.Zero(t, f.nudger.callCount())
	f.requireNoRespawn(t)
	assert.Zero(t, f.restarts)
	assert.Contains(t, f.rowContext(t), "nudge_outcome=not_restartable")
	assert.Empty(t, notificationTitles(f.drain()), "a requested stop is not a stuck alarm")
}

// Cancelled-class stops reach no nudge and no re-attach even when every flag is on
// and the pane sits at an idle prompt.
func TestStallRecovery_CancelledStopsNeverNudgeOrRestart(t *testing.T) {
	for _, reason := range cancelledReasons {
		t.Run(string(reason), func(t *testing.T) {
			f := newStallNudgeFixture(t, "cancel-nudge-"+string(reason), session.BacklogStatusInProgress, true, true)
			out := cancelledOutcome(reason)
			out.Snapshot = snapshotFor(domain.ObservedIdlePrompt)

			f.complete(out)

			assert.Zero(t, f.nudger.callCount())
			assert.Zero(t, f.restarts)
			f.requireNoRespawn(t)
		})
	}
}

// The flags are re-read at the claim: one turned off after planning falls back to respawn.
func TestStallRecovery_FlagTurnedOffBeforeClaimRespawns(t *testing.T) {
	f := newStallNudgeFixture(t, "flag-flips", session.BacklogStatusInProgress, true, true)
	var reads int
	f.svc.autonomousSvc.SetNudgeFlagReaders(func() bool { return true }, func() bool {
		reads++
		return reads == 1 // on for planning, off at the claim
	})

	f.complete(stallIn(domain.ObservedIdlePrompt, domain.StopReasonOrchestratorError))

	assert.Equal(t, []bool{false}, f.nudger.granted, "the claim saw the flag off and granted nothing")
	assert.Zero(t, f.restarts)
	f.requireRespawn(t)
}

// A stop landing between the last canRestart and driver registration must drop the new driver.
func TestStallRecovery_StopDuringReattachDropsNewDriver(t *testing.T) {
	f := newStallNudgeFixture(t, "stop-in-reattach", session.BacklogStatusInProgress, true, true)
	svc := f.svc.autonomousSvc
	f.restartFn = func() error {
		svc.registerDriver(f.title, session.NewAutonomousDriver(f.inst, nil, "goal", 1))
		f.inst.SetAutonomousMode(false, "")
		return nil
	}

	f.complete(stallIn(domain.ObservedIdlePrompt, domain.StopReasonOrchestratorError))

	svc.mu.Lock()
	_, registered := svc.drivers[f.title]
	svc.mu.Unlock()
	assert.False(t, registered, "the driver started after the stop must be removed")
	f.requireNoRespawn(t)
	assert.False(t, f.mode())
}

// A delivered nudge followed by a stop before re-attach reports the delivery and attaches nothing.
func TestStallRecovery_StopAfterDeliveryDetaches(t *testing.T) {
	f := newStallNudgeFixture(t, "detached", session.BacklogStatusInProgress, true, true)
	f.svc.autonomousSvc.SetStallNudger(stopAfterNudge{inner: f.nudger, inst: f.inst})

	f.complete(stallIn(domain.ObservedIdlePrompt, domain.StopReasonOrchestratorError))

	assert.Zero(t, f.restarts)
	f.requireNoRespawn(t)
	assert.False(t, f.mode())
	assert.Contains(t, f.rowContext(t), "action=nudged")
	assert.Equal(t, []string{"Autonomous session nudged"}, notificationTitles(f.drain()))
}

type stopAfterNudge struct {
	inner *fakeStallNudger
	inst  *session.Instance
}

func (n stopAfterNudge) NudgeStalledSession(ctx context.Context, inst *session.Instance, claim func() (bool, error)) error {
	err := n.inner.NudgeStalledSession(ctx, inst, claim)
	n.inst.SetAutonomousMode(false, "")
	return err
}

// Stuck-row write failures fail open to the respawn body; a moved-on item status skips recovery.
func TestStallRecovery_MarkStuckFailureFailsOpenToRespawn(t *testing.T) {
	f := newStallNudgeFixture(t, "markstuck-fails", session.BacklogStatusInProgress, true, true)
	session.TestFailMarkStuckWrites(t, f.storage)

	f.complete(stallIn(domain.ObservedIdlePrompt, domain.StopReasonOrchestratorError))

	assert.Zero(t, f.nudger.callCount(), "a failed plan-time write must not nudge")
	f.requireRespawn(t)
	assert.False(t, f.mode())
}

func TestStallRecovery_ItemMovedOnBeforeMarkStuckSkipsRecovery(t *testing.T) {
	f := newStallNudgeFixture(t, "item-moved", session.BacklogStatusInProgress, true, true)
	moved := false
	// The flag reader runs during planning, after the item was loaded and before MarkStuck.
	f.svc.autonomousSvc.SetNudgeFlagReaders(func() bool {
		if !moved {
			moved = true
			_, err := f.storage.RawTransitionBacklogItemStatus(context.Background(), f.itemID, session.BacklogStatusReview, nil, "test")
			require.NoError(t, err)
		}
		return true
	}, func() bool { return true })

	f.complete(stallIn(domain.ObservedIdlePrompt, domain.StopReasonOrchestratorError))

	assert.Zero(t, f.nudger.callCount())
	f.requireNoRespawn(t)
	assert.Zero(t, f.restarts)
}

// A replaced driver's late completion callback must not stop, remove or rewrite the new run.
func TestOnAutonomousDriverComplete_ReplacedDriverCallbackLeavesNewDriver(t *testing.T) {
	f := newCharFixture(t, "replaced-driver", session.SessionRoleWork, session.BacklogStatusInProgress)
	respawner := newSignalingRespawner()
	f.svc.SetAutonomousStuckRespawner(respawner)
	svc := f.svc.autonomousSvc
	oldDriver := session.NewAutonomousDriver(f.inst, nil, "goal", 1)
	newDriver := session.NewAutonomousDriver(f.inst, nil, "goal", 1)
	session.TestSetAutonomousDriverRunID(t, oldDriver, 1)
	session.TestSetAutonomousDriverRunID(t, newDriver, 2)
	svc.registerDriver(f.title, oldDriver)
	svc.registerDriver(f.title, newDriver)

	out := stallIn(domain.ObservedIdlePrompt, domain.StopReasonOrchestratorError)
	out.RunID = 1
	f.complete(out)

	svc.mu.Lock()
	registered := svc.drivers[f.title]
	svc.mu.Unlock()
	assert.Same(t, newDriver, registered, "the new driver stays registered")
	assert.True(t, f.inst.Snapshot().Autonomous.AutonomousMode, "the instance must not be stopped")
	assert.Empty(t, f.drain(), "no notification or update for a superseded run")
	open, err := f.storage.FindOpenStuckStates(context.Background())
	require.NoError(t, err)
	assert.Empty(t, open)
	is, err := f.storage.GetItemSessionBySessionUUID(context.Background(), f.inst.UUID)
	require.NoError(t, err)
	assert.Nil(t, is.EndedAt)

	out.RunID = 2
	f.complete(out)
	svc.mu.Lock()
	_, still := svc.drivers[f.title]
	svc.mu.Unlock()
	assert.False(t, still, "the current run's own completion deregisters it")
}
