package wakes

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// slotAdmission admits while fewer than cap sessions are live; the revive's
// Start flips the owner to live, like a real revived session joining the count.
type slotAdmission struct {
	cap   int
	live  atomic.Int32
	calls atomic.Int32
	err   error
}

func (a *slotAdmission) Admit(context.Context) (bool, error) {
	a.calls.Add(1)
	if a.err != nil {
		return false, a.err
	}
	return int(a.live.Load()) < a.cap, nil
}

func stoppedOwner(owners *fakeOwners, a *slotAdmission, name string) *spyTarget {
	target := &spyTarget{}
	target.onStart = func() {
		a.live.Add(1)
		owners.set(name, OwnerFacts{Exists: true, Live: true})
	}
	owners.set(name, OwnerFacts{Exists: true, Revive: target})
	return target
}

func TestReviver_should_NotCallRecoverOrStart_When_OwnerArchivedBetweenClaimAndRevive(t *testing.T) {
	t.Parallel()
	owners := newFakeOwners()
	target := &spyTarget{archived: true}
	// The owner lookup still reports a stopped, non-archived owner; the
	// instance flipped to archived after that read.
	owners.set("o1", OwnerFacts{Exists: true, Revive: target})
	rv := NewReviver(&slotAdmission{cap: 2}, owners, NewFakeClock(t0), 0)

	out, err := rv.TryRevive(context.Background(), "o1")

	require.NoError(t, err)
	assert.Equal(t, ReviveOwnerGone, out)
	recovered, started := target.calls()
	assert.Zero(t, recovered)
	assert.Zero(t, started)
}

func TestReviver_should_ReviveExactlyOne_When_ThreeRaceForOneSlot(t *testing.T) {
	t.Parallel()
	owners := newFakeOwners()
	adm := &slotAdmission{cap: 1}
	targets := map[string]*spyTarget{}
	for _, n := range []string{"a", "b", "c"} {
		targets[n] = stoppedOwner(owners, adm, n)
	}
	rv := NewReviver(adm, owners, NewFakeClock(t0), 0)

	var wg sync.WaitGroup
	outcomes := make(chan ReviveOutcome, 3)
	for n := range targets {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, err := rv.TryRevive(context.Background(), n)
			assert.NoError(t, err)
			outcomes <- out
		}()
	}
	wg.Wait()
	close(outcomes)

	live, notDone := 0, 0
	for o := range outcomes {
		switch o {
		case ReviveOwnerLive:
			live++
		case ReviveNotDone:
			notDone++
		}
	}
	assert.Equal(t, 1, live, "the revive lock spans Admit through live")
	assert.Equal(t, 2, notDone)
	started := 0
	for _, tg := range targets {
		if _, s := tg.calls(); s == 1 {
			started++
		}
	}
	assert.Equal(t, 1, started)
}

func TestReviver_should_ReleaseLockAtBudget_When_StartBlocks(t *testing.T) {
	t.Parallel()
	owners := newFakeOwners()
	adm := &slotAdmission{cap: 5}
	release := make(chan struct{})
	slow := &spyTarget{onStart: func() { <-release }}
	owners.set("slow", OwnerFacts{Exists: true, Revive: slow})
	clock := NewFakeClock(t0)
	rv := NewReviver(adm, owners, clock, 0)

	first := make(chan ReviveOutcome, 1)
	go func() {
		out, err := rv.TryRevive(context.Background(), "slow")
		assert.NoError(t, err)
		first <- out
	}()
	clock.BlockUntilTimerAt(t0.Add(DefaultReviveLockHold))
	clock.Advance(31 * time.Second)

	select {
	case out := <-first:
		assert.Equal(t, ReviveNotDone, out, "row stays needs_revive")
	case <-time.After(5 * time.Second):
		t.Fatal("revive lock was not released at the budget")
	}

	// The lock is free again: a second owner is admitted without waiting for the hung spawn.
	other := stoppedOwner(owners, adm, "other")
	out, err := rv.TryRevive(context.Background(), "other")
	require.NoError(t, err)
	assert.Equal(t, ReviveOwnerLive, out)
	_, started := other.calls()
	assert.Equal(t, 1, started)

	// Next cycle: the slow session did come up, so it counts as live.
	owners.set("slow", OwnerFacts{Exists: true, Live: true})
	out, err = rv.TryRevive(context.Background(), "slow")
	require.NoError(t, err)
	assert.Equal(t, ReviveOwnerLive, out)
	close(release)
}

func TestReviver_should_SkipAdmit_When_OwnerAlreadyLive(t *testing.T) {
	t.Parallel()
	owners := newFakeOwners()
	adm := &slotAdmission{cap: 0}
	rv := NewReviver(adm, owners, NewFakeClock(t0), 0)

	out, err := rv.TryRevive(context.Background(), "up") // unknown owners read as live

	require.NoError(t, err)
	assert.Equal(t, ReviveOwnerLive, out)
	assert.Zero(t, adm.calls.Load())
}

func TestReviver_should_ReportNotDone_When_AdmitRefusesOrErrors(t *testing.T) {
	t.Parallel()
	owners := newFakeOwners()
	full := &slotAdmission{cap: 0}
	target := stoppedOwner(owners, full, "o")
	rv := NewReviver(full, owners, NewFakeClock(t0), 0)

	out, err := rv.TryRevive(context.Background(), "o")
	require.NoError(t, err)
	assert.Equal(t, ReviveNotDone, out)
	recovered, started := target.calls()
	assert.Zero(t, recovered+started, "no slot, nothing revived")

	full.err = errBoom
	_, err = rv.TryRevive(context.Background(), "o")
	assert.ErrorIs(t, err, errBoom)
}

func TestReviver_should_ReturnError_When_StartFails(t *testing.T) {
	t.Parallel()
	owners := newFakeOwners()
	adm := &slotAdmission{cap: 3}
	target := &spyTarget{startErr: errBoom}
	owners.set("o", OwnerFacts{Exists: true, Revive: target})
	rv := NewReviver(adm, owners, NewFakeClock(t0), 0)

	out, err := rv.TryRevive(context.Background(), "o")

	assert.ErrorIs(t, err, errBoom)
	assert.Equal(t, ReviveNotDone, out)
	recovered, started := target.calls()
	assert.Equal(t, 1, recovered)
	assert.Equal(t, 1, started)
}

func TestReviver_should_ReportGone_When_OwnerNoLongerQualifies(t *testing.T) {
	t.Parallel()
	owners := newFakeOwners()
	owners.set("o", OwnerFacts{Exists: true, ItemTerminal: true})
	rv := NewReviver(&slotAdmission{cap: 3}, owners, NewFakeClock(t0), 0)

	out, err := rv.TryRevive(context.Background(), "o")

	require.NoError(t, err)
	assert.Equal(t, ReviveOwnerGone, out)
}

func TestReviver_should_NotStart_When_OwnerIsStillStarting(t *testing.T) {
	t.Parallel()
	owners := newFakeOwners()
	target := &spyTarget{}
	owners.set("o1", OwnerFacts{Exists: true, Starting: true, Revive: target})
	adm := &slotAdmission{cap: 2}
	rv := NewReviver(adm, owners, NewFakeClock(t0), 0)

	out, err := rv.TryRevive(context.Background(), "o1")

	require.NoError(t, err)
	assert.Equal(t, ReviveNotDone, out)
	recovered, started := target.calls()
	assert.Zero(t, recovered)
	assert.Zero(t, started)
	assert.Zero(t, adm.calls.Load())
}
