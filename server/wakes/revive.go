package wakes

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// DefaultReviveLockHold bounds how long the revive lock may be held (the
// revive_lock row of the Wake limits table).
const DefaultReviveLockHold = 30 * time.Second

// livePollInterval is how often the revive path re-reads the owner while it
// waits for a started session to count as live.
const livePollInterval = time.Second

// ReviveOutcome is what one TryRevive pass achieved.
type ReviveOutcome uint8

const (
	// ReviveNotDone is the zero value: nothing was revived (refused by
	// Admission, an error, or the lock budget ran out); the row stays needs_revive.
	ReviveNotDone ReviveOutcome = iota
	// ReviveOwnerLive: the owner is running (already, or after our restart), so
	// the wake can be delivered.
	ReviveOwnerLive
	// ReviveOwnerGone: the owner no longer qualifies for a revive (archived,
	// one-shot, terminal item or deleted); the caller re-runs Decide and skips.
	ReviveOwnerGone
)

var errOwnerArchived = errors.New("wakes: owner archived; not reviving")

// Reviver resumes stopped, non-archived owners behind Admission. Only revives
// take reviveMu, so create-time and claim-time cap checks never wait on a slow
// tmux and worktree spawn (ADR-007 rule 5).
type Reviver struct {
	admission Admission
	owners    OwnerLookup
	clock     Clock
	hold      time.Duration

	reviveMu sync.Mutex
}

// NewReviver builds a Reviver; hold <= 0 means DefaultReviveLockHold.
func NewReviver(admission Admission, owners OwnerLookup, clock Clock, hold time.Duration) *Reviver {
	if hold <= 0 {
		hold = DefaultReviveLockHold
	}
	return &Reviver{admission: admission, owners: owners, clock: clock, hold: hold}
}

// TryRevive revives ownerUUID if it is stopped, allowed and admitted. The revive
// lock is taken before Admit and held until the revived session counts as
// live, the revive failed, or the hold budget ran out: Admit reserves no slot,
// so without the lock two wakes could both pass at cap-1.
func (r *Reviver) TryRevive(ctx context.Context, ownerUUID string) (ReviveOutcome, error) {
	r.reviveMu.Lock()
	defer r.reviveMu.Unlock()

	facts, err := r.owners.Owner(ctx, ownerUUID)
	if err != nil {
		return ReviveNotDone, fmt.Errorf("revive %s: owner lookup: %w", ownerUUID, err)
	}
	if v := Decide(true, facts); v.Kind == VerdictSkip {
		return ReviveOwnerGone, nil
	}
	if facts.Live {
		return ReviveOwnerLive, nil
	}
	if facts.Revive == nil {
		return ReviveNotDone, fmt.Errorf("revive %s: stopped owner has no revive target", ownerUUID)
	}
	budget := r.clock.NewTimer(r.hold)
	defer budget.Stop()

	admitted, err := r.admission.Admit(ctx)
	if err != nil {
		return ReviveNotDone, fmt.Errorf("revive %s: admit: %w", ownerUUID, err)
	}
	if !admitted {
		return ReviveNotDone, nil
	}
	return r.startAndAwaitLive(ctx, ownerUUID, facts.Revive, budget)
}

func (r *Reviver) startAndAwaitLive(ctx context.Context, ownerUUID string, target ReviveTarget, budget Timer) (ReviveOutcome, error) {
	// Start spawns tmux and a worktree and can hang; it runs on its own
	// goroutine so the budget can release the lock. The goroutine finishes on
	// its own and its result is picked up by the next cycle's owner re-read.
	started := make(chan error, 1)
	go func() { started <- reviveStoppedOwner(target) }()
	select {
	case err := <-started:
		if errors.Is(err, errOwnerArchived) {
			return ReviveOwnerGone, nil
		}
		if err != nil {
			return ReviveNotDone, fmt.Errorf("revive %s: start: %w", ownerUUID, err)
		}
	case <-budget.C():
		return ReviveNotDone, nil
	case <-ctx.Done():
		return ReviveNotDone, ctx.Err()
	}
	return r.awaitLive(ctx, ownerUUID, budget)
}

func (r *Reviver) awaitLive(ctx context.Context, ownerUUID string, budget Timer) (ReviveOutcome, error) {
	for {
		facts, err := r.owners.Owner(ctx, ownerUUID)
		if err != nil {
			return ReviveNotDone, fmt.Errorf("revive %s: re-read owner: %w", ownerUUID, err)
		}
		if facts.Live {
			return ReviveOwnerLive, nil
		}
		poll := r.clock.NewTimer(livePollInterval)
		select {
		case <-poll.C():
		case <-budget.C():
			poll.Stop()
			return ReviveNotDone, nil
		case <-ctx.Done():
			poll.Stop()
			return ReviveNotDone, ctx.Err()
		}
	}
}

// reviveStoppedOwner is the only function in this package allowed to call
// RecoverFromStopped and Start(false) (tools/lint/noarchivedrevival lints this
// file). It checks archival itself, immediately before: the analyzer is
// lexical, so a guard in a caller would not count. RecoverFromStopped only
// resets status to Creating; Start(false) is the warm restart that follows it
// (session/instance_claude.go, session/retry_state.go).
func reviveStoppedOwner(target ReviveTarget) error {
	if target.IsArchived() {
		return errOwnerArchived
	}
	target.RecoverFromStopped()
	return target.Start(false)
}
