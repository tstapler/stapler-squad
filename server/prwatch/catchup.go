package prwatch

import (
	"context"
	"sync"
	"time"
)

// Catch-up sources, used as the "source" label on MetricCatchupSteers.
const (
	catchupFirstSubscribe = "first_subscribe"
	catchupResume         = "resume"
)

// CatchupCounts are the informational counters for the soak review: catch-up
// steers against first subscriptions. They carry no threshold (requirements A3).
type CatchupCounts struct {
	FirstSubscriptions     int
	FirstSubscriptionSteer int // first subscriptions whose catch-up delivered a steer
	ResumeSteers           int
}

// CatchupDeps are the ports Catchup needs.
type CatchupDeps struct {
	Snapshots PRSnapshotSource
	Steerer   Steerer
	Coalescer Coalescer
	Metrics   Metrics
	Log       Logger
	Settings  Settings
}

type catchupPending struct {
	ref SessionRef
	// after is the time the snapshot must have been polled past: zero for a
	// first subscription (any poll counts), the resume time after a resume.
	after  time.Time
	source string
}

// Catchup re-reads a session's PR snapshot once after a first subscription
// and once per resume, and steers when CI is failing or (human_feedback_mode
// all) changes are requested. Events before subscription are never replayed,
// so this is the only coverage for them. Safe for concurrent use.
//
// The snapshot has no head SHA, so the steer follows the coalescer's class
// window. CheckConclusion and ChangesRequested are not persisted, so an empty
// value is trusted only once CheckedAt is non-zero and newer than the wait
// point; until then the catch-up stays armed.
type Catchup struct {
	d CatchupDeps

	mu        sync.Mutex
	excluded  map[string]bool // uuid -> was skipped as pr_pending (D4)
	pending   map[string]*catchupPending
	handled   map[string]time.Time // uuid -> ResumedAt already armed
	counts    CatchupCounts
	firstDone map[string]bool // first-subscription catch-up already armed or run
}

// NewCatchup builds a Catchup.
func NewCatchup(d CatchupDeps) *Catchup {
	return &Catchup{
		d:         d,
		excluded:  map[string]bool{},
		pending:   map[string]*catchupPending{},
		handled:   map[string]time.Time{},
		firstDone: map[string]bool{},
	}
}

// Counts returns the informational counters.
func (c *Catchup) Counts() CatchupCounts {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.counts
}

// NoteExcluded records sessions currently skipped as backlog_pr_pending. A
// later first subscription of such a session gets no catch-up (D4): #645
// already handled its failures. Call with each List result.
func (c *Catchup) NoteExcluded(refs []SessionRef) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, r := range refs {
		if r.SkipReason == SkipBacklogPRPending {
			c.excluded[r.UUID] = true
		}
	}
}

// Subscribed is called when a PR is first subscribed for the session. An
// already-subscribed PR must not call it again.
func (c *Catchup) Subscribed(ctx context.Context, ref SessionRef) {
	c.mu.Lock()
	if c.excluded[ref.UUID] {
		delete(c.excluded, ref.UUID)
		c.firstDone[ref.UUID] = true
		c.mu.Unlock()
		c.d.Log.Debug("prwatch.catchup_skipped", "reason", "pr_pending_exclusion")
		return
	}
	c.counts.FirstSubscriptions++
	c.firstDone[ref.UUID] = true
	c.pending[ref.UUID] = &catchupPending{ref: ref, source: catchupFirstSubscribe}
	c.mu.Unlock()
	c.attempt(ctx, ref.UUID)
}

// Tick arms the resume catch-up for sessions whose ResumedAt is new, retries
// armed catch-ups, and forgets sessions that left the retained set. refs is
// the latest List result.
func (c *Catchup) Tick(ctx context.Context, refs []SessionRef) {
	c.NoteExcluded(refs)
	present := map[string]bool{}
	c.mu.Lock()
	for _, r := range refs {
		if r.SkipReason != "" {
			continue
		}
		present[r.UUID] = true
		if p, ok := c.pending[r.UUID]; ok {
			p.ref = r
		}
		if !r.ResumedAt.IsZero() && r.State == StateLive && !r.ResumedAt.Equal(c.handled[r.UUID]) {
			c.handled[r.UUID] = r.ResumedAt
			c.pending[r.UUID] = &catchupPending{ref: r, after: r.ResumedAt, source: catchupResume}
		}
	}
	for uuid := range c.pending {
		if !present[uuid] {
			delete(c.pending, uuid)
		}
	}
	for uuid := range c.firstDone {
		if !present[uuid] {
			delete(c.firstDone, uuid)
			delete(c.handled, uuid)
		}
	}
	uuids := make([]string, 0, len(c.pending))
	for uuid := range c.pending {
		uuids = append(uuids, uuid)
	}
	c.mu.Unlock()
	for _, uuid := range uuids {
		c.attempt(ctx, uuid)
	}
}

// attempt runs one armed catch-up if its snapshot is fresh enough.
func (c *Catchup) attempt(ctx context.Context, uuid string) {
	c.mu.Lock()
	p := c.pending[uuid]
	c.mu.Unlock()
	if p == nil || p.ref.State != StateLive {
		return
	}
	snap, err := c.d.Snapshots.Snapshot(uuid)
	if err != nil {
		c.disarm(uuid)
		c.d.Log.Warn("prwatch.catchup_snapshot_failed")
		return
	}
	if snap.Terminal {
		c.disarm(uuid)
		return
	}
	if snap.CheckedAt.IsZero() || !snap.CheckedAt.After(p.after) {
		return // stale or never polled: an empty value cannot be trusted yet
	}

	kinds := c.wantedKinds(snap)
	steerKinds := c.steerable(p.ref, kinds)
	if len(steerKinds) == 0 {
		c.disarm(uuid)
		return
	}
	text, err := render(steerKinds, p.ref.Repo, p.ref.PR)
	if err != nil {
		c.disarm(uuid)
		c.d.Log.Warn("prwatch.catchup_render_failed")
		return
	}
	out, err := c.d.Steerer.Steer(ctx, SteerRequest{
		SessionUUID: uuid, Repo: p.ref.Repo, PR: p.ref.PR, Kinds: steerKinds, Text: text,
	})
	if err != nil {
		c.disarm(uuid)
		c.d.Log.Warn("prwatch.catchup_steer_failed")
		return
	}
	switch out {
	case OutcomeDelivered:
		seen := map[KindClass]bool{}
		for _, k := range steerKinds {
			if !seen[k.Class()] {
				seen[k.Class()] = true
				c.d.Coalescer.RecordSteer(keyFor(p.ref, k.Class()), "")
			}
		}
		c.d.Metrics.Inc(MetricCatchupSteers, map[string]string{"source": p.source})
		c.mu.Lock()
		if p.source == catchupFirstSubscribe {
			c.counts.FirstSubscriptionSteer++
		} else {
			c.counts.ResumeSteers++
		}
		c.mu.Unlock()
		c.disarm(uuid)
	case OutcomeBusy, OutcomeGuardBusy, OutcomeCoolingDown, OutcomeNoStatusSource, OutcomeUnspecified:
		// not delivered and nothing written: stay armed, retry next tick
	default:
		c.disarm(uuid)
	}
}

func (c *Catchup) disarm(uuid string) {
	c.mu.Lock()
	delete(c.pending, uuid)
	c.mu.Unlock()
}

// wantedKinds applies the failure shape and the human_feedback_mode rule: the
// changes-requested half runs only in mode all, because the snapshot cannot
// tell who requested changes.
func (c *Catchup) wantedKinds(snap PRSnapshot) []Kind {
	var kinds []Kind
	if snap.CheckConclusion == "failure" {
		kinds = append(kinds, KindCI)
	}
	if c.d.Settings.HumanFeedbackMode == ModeAll && snap.ChangesRequested > 0 {
		kinds = append(kinds, KindReview)
	}
	return kinds
}

// steerable keeps the kinds whose coalescing class may steer now; a class
// inside its window is covered by the steer that opened it.
func (c *Catchup) steerable(ref SessionRef, kinds []Kind) []Kind {
	var out []Kind
	for _, k := range kinds {
		if c.d.Coalescer.Decide(keyFor(ref, k.Class()), "", 0) == CoalesceSteer {
			out = append(out, k)
		}
	}
	return out
}

func keyFor(ref SessionRef, class KindClass) CoalesceKey {
	return CoalesceKey{SessionUUID: ref.UUID, Repo: ref.Repo, PR: ref.PR, Class: class}
}
