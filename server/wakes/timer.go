package wakes

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

// storeRetryDelay is how long the loop waits after a store error before
// re-reading, so a failing database cannot spin it.
const storeRetryDelay = 5 * time.Second

const errKey = "error"

// ErrNotLeader is returned by Start when another process holds the leader guard.
var ErrNotLeader = errors.New("wakes: another process holds the wake dispatcher leader guard")

// Store is the slice of the wake repository the timer needs; *session.Storage
// and *session.EntRepository satisfy it.
type Store interface {
	NextWakeFireAt(ctx context.Context) (time.Time, bool, error)
	ListDueWakeTriggers(ctx context.Context, now time.Time) ([]*session.WakeTriggerData, error)
	ClaimWakeTrigger(ctx context.Context, id uuid.UUID, now time.Time) (bool, error)
	MarkWakeMissed(ctx context.Context, id uuid.UUID, reason domain.WakeReason, now time.Time) (bool, error)
}

// ClaimedFunc receives a row the timer just claimed (pending -> claimed). The
// dispatcher (Story 4.3) delivers it and later confirms with MarkWakeDelivered.
type ClaimedFunc func(ctx context.Context, claimed *session.WakeTriggerData)

// MissedFunc is told about a row that expired before the timer could claim it.
type MissedFunc func(ctx context.Context, missed *session.WakeTriggerData)

// Timekeeper owns the single timer on min(next_fire_at) over the durable
// table. It sleeps until the next due row (no row-scanning ticker), claims
// due rows, and re-arms when a row is created, cancelled or changed (Rearm,
// fed by the repository change hook) and on start.
type Timekeeper struct {
	store   Store
	clock   Clock
	claimed ClaimedFunc
	missed  MissedFunc
	leader  LeaderGuard
	rearm   chan struct{}

	mu      sync.Mutex
	running bool
	cancel  context.CancelFunc
	done    chan struct{}
	release func()
}

// TimekeeperOption customizes a Timekeeper.
type TimekeeperOption func(*Timekeeper)

// WithLeaderGuard sets the single-leader guard (default: always leader).
func WithLeaderGuard(g LeaderGuard) TimekeeperOption { return func(t *Timekeeper) { t.leader = g } }

// WithMissedFunc registers a callback for rows that expired unclaimed.
func WithMissedFunc(fn MissedFunc) TimekeeperOption { return func(t *Timekeeper) { t.missed = fn } }

// NewTimekeeper builds a Timekeeper; claimed is called for every row it claims.
func NewTimekeeper(store Store, clock Clock, claimed ClaimedFunc, opts ...TimekeeperOption) *Timekeeper {
	t := &Timekeeper{
		store:   store,
		clock:   clock,
		claimed: claimed,
		leader:  NoLeader{},
		rearm:   make(chan struct{}, 1),
	}
	for _, o := range opts {
		o(t)
	}
	return t
}

// Rearm asks the loop to recompute min(next_fire_at). It never blocks and
// coalesces bursts.
func (t *Timekeeper) Rearm() {
	select {
	case t.rearm <- struct{}{}:
	default:
	}
}

// Start acquires the leader guard and launches the loop. It returns
// ErrNotLeader without starting when another process is the leader, and an
// error when already running. The loop ends when ctx is cancelled or Stop is called.
func (t *Timekeeper) Start(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.running {
		return errors.New("wakes: timekeeper already running")
	}
	release, ok, err := t.leader.TryAcquire()
	if err != nil {
		return fmt.Errorf("wakes: leader guard: %w", err)
	}
	if !ok {
		return ErrNotLeader
	}
	loopCtx, cancel := context.WithCancel(ctx)
	t.running, t.cancel, t.release = true, cancel, release
	t.done = make(chan struct{})
	go t.loop(loopCtx, t.done)
	return nil
}

// Stop cancels the loop and waits for it to exit (or ctx to expire), then
// releases the leader guard. Safe to call when not running.
func (t *Timekeeper) Stop(ctx context.Context) error {
	t.mu.Lock()
	if !t.running {
		t.mu.Unlock()
		return nil
	}
	cancel, done, release := t.cancel, t.done, t.release
	t.mu.Unlock()

	cancel()
	select {
	case <-done:
	case <-ctx.Done():
		return fmt.Errorf("wakes: timekeeper did not stop: %w", ctx.Err())
	}
	t.mu.Lock()
	t.running = false
	t.mu.Unlock()
	release()
	return nil
}

func (t *Timekeeper) loop(ctx context.Context, done chan<- struct{}) {
	defer close(done)
	backoff := false
	for {
		wait, armed := t.nextWait(ctx)
		if backoff {
			wait, armed, backoff = storeRetryDelay, true, false
		}
		var tm Timer
		var fire <-chan time.Time
		if armed {
			tm = t.clock.NewTimer(wait)
			fire = tm.C()
		}
		select {
		case <-ctx.Done():
			stopTimer(tm)
			return
		case <-t.rearm:
			stopTimer(tm)
		case <-fire:
			backoff = !t.fireDue(ctx)
		}
	}
}

func stopTimer(tm Timer) {
	if tm != nil {
		tm.Stop()
	}
}

// nextWait is the delay until the earliest pending row, or storeRetryDelay
// after a store error; armed=false means nothing is pending.
func (t *Timekeeper) nextWait(ctx context.Context) (wait time.Duration, armed bool) {
	next, ok, err := t.store.NextWakeFireAt(ctx)
	if err != nil {
		if ctx.Err() == nil {
			log.Warn("wake timer: read next fire time failed", errKey, err)
		}
		return storeRetryDelay, true
	}
	if !ok {
		return 0, false
	}
	return max(next.Sub(t.clock.Now()), 0), true
}

// fireDue claims every due row once. An expired row becomes missed instead:
// claiming it would fire a stale wake, and leaving it pending would make the
// loop re-select it forever.
func (t *Timekeeper) fireDue(ctx context.Context) (ok bool) {
	now := t.clock.Now()
	due, err := t.store.ListDueWakeTriggers(ctx, now)
	if err != nil {
		if ctx.Err() == nil {
			log.Warn("wake timer: list due triggers failed", errKey, err)
		}
		return false
	}
	ok = true
	for _, row := range due {
		if ctx.Err() != nil {
			return false
		}
		ok = t.handleDue(ctx, row, now) && ok
	}
	return ok
}

// handleDue reports false when the row stayed pending because of a store
// error, so the loop backs off instead of re-selecting it immediately.
func (t *Timekeeper) handleDue(ctx context.Context, row *session.WakeTriggerData, now time.Time) bool {
	id, err := uuid.Parse(row.ID)
	if err != nil {
		log.Error("wake timer: unparseable trigger id", "id", row.ID, errKey, err)
		return false
	}
	if row.ExpiresAt != nil && !row.ExpiresAt.After(now) {
		return t.markMissed(ctx, id, row, now)
	}
	applied, err := t.store.ClaimWakeTrigger(ctx, id, now)
	if err != nil {
		log.Warn("wake timer: claim failed", "trigger_id", row.ID, errKey, err)
		return false
	}
	if applied {
		row.Status, row.ClaimedAt = domain.WakeStatusClaimed, &now
		if t.claimed != nil {
			t.claimed(ctx, row)
		}
	}
	return true
}

func (t *Timekeeper) markMissed(ctx context.Context, id uuid.UUID, row *session.WakeTriggerData, now time.Time) bool {
	applied, err := t.store.MarkWakeMissed(ctx, id, domain.WakeReasonExpired, now)
	if err != nil {
		log.Warn("wake timer: mark missed failed", "trigger_id", row.ID, errKey, err)
		return false
	}
	if applied && t.missed != nil {
		t.missed(ctx, row)
	}
	return true
}
