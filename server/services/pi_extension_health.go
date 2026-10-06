package services

import (
	"time"

	"github.com/puzpuzpuz/xsync/v4"
	"github.com/tstapler/stapler-squad/log"
)

// PiExtensionHealth is a three-state signal of whether a pi session's globally
// installed approval extension (~/.pi/agent/extensions/ssq-approval.ts) has
// confirmed it loaded and is actively enforcing approval rules. Three states,
// not a bool: project_plans/pi-support/implementation/plan.md's Domain
// Glossary and research/ux.md §4 are explicit that "unknown" must stay
// visually distinct from a confirmed failure, and must never present as
// "loaded" before a real signal has arrived — defaulting to "assume healthy"
// is exactly the silent-enforcement-gap failure mode PITFALL-1 warns about.
type PiExtensionHealth int

const (
	// PiExtensionHealthUnknown is the default state: no ping has been received
	// yet, and the grace window has not elapsed. Never render this as "loaded."
	PiExtensionHealthUnknown PiExtensionHealth = iota
	// PiExtensionHealthLoaded means at least one health ping has been received
	// within the grace window (either the extension's initial load-time ping or
	// Story 4.2.3's periodic re-ping).
	PiExtensionHealthLoaded
	// PiExtensionHealthFailed means the grace window elapsed with no ping —
	// tool calls for this session are unenforced.
	PiExtensionHealthFailed
)

// String renders the health state for logging; matches the lowercase words used
// throughout plan.md/design/ux.md ("loaded"/"failed"/"unknown").
func (h PiExtensionHealth) String() string {
	switch h {
	case PiExtensionHealthLoaded:
		return "loaded"
	case PiExtensionHealthFailed:
		return "failed"
	default:
		return "unknown"
	}
}

// piExtensionRepingInterval is how often ssqApprovalExtensionTemplate
// (cmd/ssq-hooks/main.go) re-sends its health ping for the lifetime of the pi
// process (Story 4.2.3), so a stapler-squad server restart doesn't permanently
// strand a live, still-enforcing session's badge at Unknown.
const piExtensionRepingInterval = 2 * time.Minute

// piExtensionHealthGraceWindow bounds how long PiExtensionHealthTracker waits
// after a session is first observed, or after its last successful ping, before
// reporting Failed instead of Unknown/Loaded. Sized at >= 2x
// piExtensionRepingInterval (Story 4.2.3's AC) so a single dropped re-ping from
// ordinary network jitter never flips a healthy session to Failed — the tracker
// tolerates one full missed re-ping cycle before giving up on it.
const piExtensionHealthGraceWindow = 2 * piExtensionRepingInterval

// piExtensionHealthEntry is PiExtensionHealthTracker's per-session bookkeeping.
type piExtensionHealthEntry struct {
	// state is the last state HealthFor computed/logged for this session —
	// used only to detect a transition worth logging exactly once (Surface 6
	// AC3: no log spam on every poll tick with no ping arriving).
	state PiExtensionHealth
	// firstObservedAt is when this session was first seen (first HealthFor or
	// RecordPing call) — the grace window for the *initial* ping is measured
	// from here, not from process start, since sessions can appear at any time.
	firstObservedAt time.Time
	// lastPingAt is the timestamp of the most recent successful health ping.
	// Zero until the first ping arrives.
	lastPingAt time.Time
}

// PiExtensionHealthTracker records pi approval-extension health pings
// (server/services/approval_handler.go's HandlePiExtensionLoaded) and answers
// "is enforcement actually active for this session right now?" via HealthFor —
// pi-support Epic 4.2, closing PITFALL-1's "installed != enforcing" gap.
//
// State is in-memory only and deliberately NOT persisted: a server restart
// legitimately resets every session back to Unknown until the extension's next
// periodic re-ping re-arms it (Story 4.2.3) — simpler and safer than trying to
// durably persist "was enforcing as of some past instant," which could lie
// about the present.
//
// HealthFor is safe to call from multiple goroutines and computes the current
// state lazily at read time (mirroring this codebase's existing convention for
// other derived-at-read-time proto fields, e.g. SubStatus in
// server/adapters/instance_adapter.go) rather than running a background ticker
// — every session-list poll already calls it, which is a tight enough loop to
// notice a transition promptly without needing a second goroutine per session.
type PiExtensionHealthTracker struct {
	sessions *xsync.Map[string, *piExtensionHealthEntry]

	// graceWindow overrides piExtensionHealthGraceWindow; zero means "use the
	// default." Exists so tests don't have to sleep for real minutes.
	graceWindow time.Duration
	// now overrides time.Now; nil means "use the real clock." Test-only.
	now func() time.Time
}

// NewPiExtensionHealthTracker constructs a tracker using the real clock and
// the default grace window.
func NewPiExtensionHealthTracker() *PiExtensionHealthTracker {
	return &PiExtensionHealthTracker{
		sessions: xsync.NewMap[string, *piExtensionHealthEntry](),
	}
}

func (t *PiExtensionHealthTracker) graceWindowOrDefault() time.Duration {
	if t.graceWindow > 0 {
		return t.graceWindow
	}
	return piExtensionHealthGraceWindow
}

func (t *PiExtensionHealthTracker) nowFn() time.Time {
	if t.now != nil {
		return t.now()
	}
	return time.Now()
}

// Forget removes sessionID's tracked state, e.g. when its Instance is
// destroyed (session.Instance.Destroy(), via SetPiExtensionHealthForgetter).
// Without this, the tracker's map grows unboundedly for the life of the
// process — nothing else ever evicts an entry.
func (t *PiExtensionHealthTracker) Forget(sessionID string) {
	if sessionID == "" {
		return
	}
	t.sessions.Delete(sessionID)
}

// RecordPing marks sessionID as having just successfully pinged the health
// endpoint. A late ping arriving after the state already flipped to Failed
// still flips it back to Loaded — per plan.md Task 4.2.1e's decision, a
// late-but-successful load is still real enforcement, and the badge should
// reflect current truth, not the worst historical state (design/ux.md's
// edge-case table makes the same call explicitly).
//
// Uses xsync.Map's atomic Compute (read-modify-write under a single bucket
// lock) rather than a separate Load then Store: the previous two-step version
// let a concurrent RecordPing/HealthFor for the same sessionID interleave
// between the Load and the Store, losing whichever update landed second and
// sometimes double-logging the same transition.
func (t *PiExtensionHealthTracker) RecordPing(sessionID string) {
	if sessionID == "" {
		return
	}
	now := t.nowFn()
	var transitionedToLoaded bool
	t.sessions.Compute(sessionID, func(prev *piExtensionHealthEntry, existed bool) (*piExtensionHealthEntry, xsync.ComputeOp) {
		transitionedToLoaded = !existed || prev.state != PiExtensionHealthLoaded
		firstObservedAt := now
		if existed {
			firstObservedAt = prev.firstObservedAt
		}
		return &piExtensionHealthEntry{
			state:           PiExtensionHealthLoaded,
			firstObservedAt: firstObservedAt,
			lastPingAt:      now,
		}, xsync.UpdateOp
	})
	if transitionedToLoaded {
		log.Info("[PiExtensionHealthTracker] transitioned to loaded", "session_id", sessionID)
	}
}

// HealthFor returns sessionID's current health state. The very first call for
// a given sessionID starts that session's grace-window clock and returns
// Unknown (never Loaded — see PiExtensionHealthUnknown's doc comment).
// Subsequent calls after the grace window elapses with no ping return Failed,
// logging the Unknown->Failed or Loaded->Failed transition exactly once
// (Surface 6 AC1/AC3) rather than once per call.
//
// Uses xsync.Map's atomic Compute instead of a separate Load then
// Load-or-Store-then-conditional-Store: the previous version raced a
// concurrent RecordPing/HealthFor for the same sessionID between its Load and
// its Store, which could lose an update or log the same transition twice.
func (t *PiExtensionHealthTracker) HealthFor(sessionID string) PiExtensionHealth {
	if sessionID == "" {
		return PiExtensionHealthUnknown
	}
	now := t.nowFn()
	var transitionedToFailed bool
	actual, _ := t.sessions.Compute(sessionID, func(prev *piExtensionHealthEntry, existed bool) (*piExtensionHealthEntry, xsync.ComputeOp) {
		entry := prev
		if !existed {
			entry = &piExtensionHealthEntry{state: PiExtensionHealthUnknown, firstObservedAt: now}
		}

		sinceReference := now.Sub(entry.firstObservedAt)
		if !entry.lastPingAt.IsZero() {
			sinceReference = now.Sub(entry.lastPingAt)
		}

		computed := entry.state
		switch entry.state {
		case PiExtensionHealthLoaded, PiExtensionHealthUnknown:
			if sinceReference > t.graceWindowOrDefault() {
				computed = PiExtensionHealthFailed
			}
		case PiExtensionHealthFailed:
			// Stays Failed until a ping arrives via RecordPing — HealthFor never
			// transitions a session back to Loaded on its own.
		}

		if existed && computed == entry.state {
			// Nothing changed: cancel the write rather than churning the map
			// entry (and its atomic pointer) on every no-op poll.
			return entry, xsync.CancelOp
		}
		transitionedToFailed = computed != entry.state && computed == PiExtensionHealthFailed
		updated := *entry
		updated.state = computed
		return &updated, xsync.UpdateOp
	})
	if transitionedToFailed {
		log.Warn("[PiExtensionHealthTracker] transitioned to failed",
			"session_id", sessionID,
			"grace_window_s", int(t.graceWindowOrDefault().Seconds()),
		)
	}
	return actual.state
}
