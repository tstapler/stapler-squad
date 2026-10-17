package unfinished

import (
	"sync/atomic"
	"time"
)

// FullWalkReason says why the next scan of a worktree must do a full walk.
type FullWalkReason uint8

const (
	ReasonNone FullWalkReason = iota
	ReasonOverflow
	ReasonError
	ReasonDirEvent
	ReasonAge
	ReasonUnarmed
	numFullWalkReasons
)

// String is the metric label value of the reason.
func (r FullWalkReason) String() string {
	switch r {
	case ReasonOverflow:
		return "overflow"
	case ReasonError:
		return "error"
	case ReasonDirEvent:
		return "dir_event"
	case ReasonAge:
		return "age"
	case ReasonUnarmed:
		return "unarmed"
	default:
		return "none"
	}
}

// scanSkipAgeTicks is how many backstop ticks a clean verdict may be trusted
// for. At one tick the bound equalled the scan cadence, so every scheduled scan
// was already past it and nothing was ever served from cache (ADR-006 addendum).
const scanSkipAgeTicks = 3

// fullWalkMaxAge bounds how long a clean verdict can be trusted: a multiple of
// the backstop tick, so a missed event is corrected within scanSkipAgeTicks
// ticks (ADR-006).
const fullWalkMaxAge = scanSkipAgeTicks * defaultScanTickInterval

// EventKind classifies one watcher observation fed into DirtyGen.
type EventKind uint8

const (
	// EventTrackedPath is a change under a path the watcher maps to this worktree (HEAD, index).
	EventTrackedPath EventKind = iota
	// EventDirChange is a directory create, rename, or remove.
	EventDirChange
	EventOverflow
	EventError
)

// Pending-state bits. One bit per forcing reason, plus the dirty hint.
const (
	pendOverflow uint32 = 1 << iota
	pendError
	pendDirEvent
	pendDirtyHint
)

// FullWalkFallback is the rule deciding when a clean verdict is not
// trustworthy. Pure value type; the clock is injected for deterministic tests.
type FullWalkFallback struct {
	MaxAge time.Duration
}

// Evaluate returns the highest-priority reason forcing a full walk, or
// ReasonNone. Priority: overflow, error, dir_event, unarmed, age.
func (f FullWalkFallback) Evaluate(armed bool, lastFullWalk, now time.Time, pending uint32) FullWalkReason {
	switch {
	case pending&pendOverflow != 0:
		return ReasonOverflow
	case pending&pendError != 0:
		return ReasonError
	case pending&pendDirEvent != 0:
		return ReasonDirEvent
	case !armed || lastFullWalk.IsZero():
		// Never walked since arming counts as unarmed: clean needs proof.
		return ReasonUnarmed
	case now.Sub(lastFullWalk) >= f.MaxAge:
		return ReasonAge
	}
	return ReasonNone
}

// DirtyDecision is the policy's answer for one scan.
type DirtyDecision struct {
	// Gen is the DirtyGen value observed; pass it to CompleteFullWalk.
	Gen uint64
	// FullWalk forces a full walk; Reason says why.
	FullWalk bool
	Reason   FullWalkReason
	// DirtyHint reports a watcher event on a tracked path since the last full
	// walk. Asymmetric safety: a true hint may short-circuit to dirty=true;
	// a false hint never licenses reporting clean.
	DirtyHint bool

	pending uint32
	at      time.Time
}

// DirtyGen is the per-worktree dirty-tracking state: a generation counter
// bumped by every watcher event, plus the pending reasons that force a full
// walk. Lock-free; the zero value is unarmed (every scan is a full walk), and
// needs SetClock only for tests.
type DirtyGen struct {
	gen          atomic.Uint64
	armed        atomic.Bool
	pending      atomic.Uint32
	lastFullWalk atomic.Int64 // unix nanos; 0 = no full walk since arming
	now          func() time.Time
	rule         FullWalkFallback
}

// NewDirtyGen returns an unarmed DirtyGen using clock (time.Now if nil).
func NewDirtyGen(clock func() time.Time) *DirtyGen {
	if clock == nil {
		clock = time.Now
	}
	return &DirtyGen{now: clock, rule: FullWalkFallback{MaxAge: fullWalkMaxAge}}
}

// Gen returns the current generation.
func (d *DirtyGen) Gen() uint64 { return d.gen.Load() }

// Arm marks the watcher live. The first scan afterwards is a full walk.
// Re-arming an armed DirtyGen is a no-op: watchRepo re-registers watches on
// every AddRepo and ref event, and resetting lastFullWalk each time forced an
// "unarmed" full walk on the next scan even though the watch never lapsed.
func (d *DirtyGen) Arm() {
	if d.armed.Load() {
		return
	}
	d.lastFullWalk.Store(0) // before armed flips, so no scan trusts a pre-disarm walk
	d.armed.Store(true)
	d.gen.Add(1)
}

// Disarm marks the watcher not live; every scan is a full walk until re-armed.
func (d *DirtyGen) Disarm() {
	d.armed.Store(false)
	d.gen.Add(1)
}

// Note records a watcher observation and bumps the generation.
func (d *DirtyGen) Note(kind EventKind) {
	var bit uint32
	switch kind {
	case EventTrackedPath:
		bit = pendDirtyHint
	case EventDirChange:
		bit = pendDirEvent
	case EventOverflow:
		bit = pendOverflow
	case EventError:
		bit = pendError
	}
	for {
		old := d.pending.Load()
		if d.pending.CompareAndSwap(old, old|bit) {
			break
		}
	}
	d.gen.Add(1)
}

// ShouldRunStatPass reports whether the tracked-file stat pass must run.
// Always true: Tier A is blind to in-place edits, and Tier B is not built, so
// no DirtyGen state can license skipping it (ADR-006 hard rule).
func (d *DirtyGen) ShouldRunStatPass() bool { return true }

// Decide evaluates the fallback rule and, when it forces a full walk,
// increments dirty_full_walk_total{reason}. Call once per scan.
func (d *DirtyGen) Decide() DirtyDecision {
	dec := d.Peek()
	if dec.FullWalk {
		recordDirtyFullWalk(dec.Reason)
	}
	return dec
}

// Peek is Decide without the metric: a second consumer of the same scan (the
// diff walk) asks whether a full walk is pending without double-counting it.
func (d *DirtyGen) Peek() DirtyDecision {
	now := d.now()
	pending := d.pending.Load()
	last := time.Time{}
	if n := d.lastFullWalk.Load(); n != 0 {
		last = time.Unix(0, n)
	}
	reason := d.rule.Evaluate(d.armed.Load(), last, now, pending)
	dec := DirtyDecision{
		Gen:       d.gen.Load(),
		FullWalk:  reason != ReasonNone,
		Reason:    reason,
		DirtyHint: pending&pendDirtyHint != 0,
		pending:   pending,
		at:        now,
	}
	return dec
}

// AckHint clears the dirty-hint bit dec observed once the scan re-validated
// the state the hint pointed at (it never counts as a full walk).
func (d *DirtyGen) AckHint(dec DirtyDecision) {
	if dec.pending&pendDirtyHint == 0 {
		return
	}
	for {
		old := d.pending.Load()
		if d.pending.CompareAndSwap(old, old&^pendDirtyHint) {
			return
		}
	}
}

// CompleteFullWalk records a finished full walk that began at decision dec.
// Only the pending bits dec observed are cleared, so an event arriving during
// the walk still forces the next one.
func (d *DirtyGen) CompleteFullWalk(dec DirtyDecision) {
	for {
		old := d.pending.Load()
		if d.pending.CompareAndSwap(old, old&^dec.pending) {
			break
		}
	}
	d.lastFullWalk.Store(dec.at.UnixNano())
}
