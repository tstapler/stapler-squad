package prwatch

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/tstapler/stapler-squad/server/prwatch/inbox"
)

// DefaultDedupCapacity bounds the handled-event set (event.id).
const DefaultDedupCapacity = 4096

// DeliveryNotes is the slice of Notifications the engine uses. *Notifications
// satisfies it.
type DeliveryNotes interface {
	SteerFailed(session, repo string, pr int) error
	SteerDelivered(session, repo string, pr int)
}

// DeliverAction is the engine's verdict for one inbox item (D5).
type DeliverAction int

const (
	// ActionHold leaves the item unacked.
	ActionHold DeliverAction = iota
	// ActionAck acks the item: delivered, covered, folded or deliberately dropped.
	ActionAck
)

// DeliverResult reports what the engine did with one item, so the loop can
// age holds and the status surface can show the reason.
type DeliverResult struct {
	ItemID      string
	EventID     string
	SessionUUID string
	Repo        string
	PR          int
	CreatedAt   time.Time
	Kinds       []Kind
	Action      DeliverAction
	Steered     bool
	// Drop is set for an ack that was not a delivered steer and not a
	// redelivery of an already handled event.
	Drop DropReason
	// Hold is set when Action is ActionHold.
	Hold HoldReason
	// Redelivered marks an item whose event.id was already handled; it is
	// only re-acked.
	Redelivered bool
	// AckErr is the error from the ack call, nil when acked or held.
	AckErr error
}

// DeliverDeps are the engine's collaborators. Notes, Stranger, Followup,
// Filter-less operation and DedupCapacity are optional.
type DeliverDeps struct {
	Inbox     Inbox
	Sessions  Sessions
	Steerer   Steerer
	Coalescer Coalescer
	Recorder  Recorder
	Metrics   Metrics
	Clock     Clock
	Log       Logger
	// Filter applies the actor rules; nil lets every actor through.
	Filter   *ActorFilter
	Notes    DeliveryNotes
	Stranger StrangerWatch
	Followup FollowupObserver
	// DedupCapacity defaults to DefaultDedupCapacity.
	DedupCapacity int
}

// Deliverer turns pending inbox items into steers and applies the D5
// ack/hold table. It is for one loop (not safe for concurrent Process calls).
type Deliverer struct {
	DeliverDeps
	handled  *boundedIndex[struct{}]
	lastHold *boundedIndex[HoldReason]
}

// NewDeliverer builds the engine.
func NewDeliverer(deps DeliverDeps) *Deliverer {
	c := deps.DedupCapacity
	if c <= 0 {
		c = DefaultDedupCapacity
	}
	return &Deliverer{
		DeliverDeps: deps,
		handled:     newBoundedIndex[struct{}](c),
		lastHold:    newBoundedIndex[HoldReason](c),
	}
}

// HandledSize is the number of event ids in the dedup set (<= capacity).
func (d *Deliverer) HandledSize() int { return d.handled.len() }

type candidate struct {
	idx   int
	kind  Kind
	sha   string
	class ActorClass
	key   CoalesceKey
}

type steerBatch struct {
	ref   SessionRef
	cands []candidate
}

// Process handles one page of pending items in order and returns one result
// per item. It never acks an item it holds. A Sessions.List error changes
// nothing (fail closed): every item stays pending and no result is returned.
func (d *Deliverer) Process(ctx context.Context, items []inbox.Item) []DeliverResult {
	refs, err := d.Sessions.List(ctx)
	if err != nil {
		d.Log.Warn("prwatch.deliver_sessions_unavailable")
		return nil
	}
	results := make([]DeliverResult, len(items))
	twinOf := map[int]int{}
	firstByEvent := map[string]int{}
	heldPerKey := map[CoalesceKey]int{}
	var batches []*steerBatch
	batchByUUID := map[string]*steerBatch{}

	for i, it := range items {
		results[i] = DeliverResult{
			ItemID: it.ID, EventID: it.Event.ID, Repo: it.Event.Repository,
			PR: it.Event.PullRequest, CreatedAt: it.CreatedAt,
		}
		if j, ok := firstByEvent[it.Event.ID]; ok && it.Event.ID != "" {
			twinOf[i] = j
			continue
		}
		firstByEvent[it.Event.ID] = i
		ref, found := findSession(refs, it.Event)
		c, ok := d.screen(&results[i], it, ref, found)
		if !ok {
			continue
		}
		action := d.Coalescer.Decide(c.key, c.sha, heldPerKey[c.key])
		switch action {
		case CoalesceSteer:
			b := batchByUUID[ref.UUID]
			if b == nil {
				b = &steerBatch{ref: ref}
				batchByUUID[ref.UUID] = b
				batches = append(batches, b)
			}
			c.idx = i
			b.cands = append(b.cands, c)
		case CoalesceCoveredSameSHA:
			d.drop(&results[i], DropCoveredSameHeadSHA, c.class)
		case CoalesceFoldOverCap:
			d.drop(&results[i], DropFoldedOverCap, c.class)
		default: // CoalesceHold and anything unknown
			heldPerKey[c.key]++
			d.hold(&results[i], HoldWindowBatching, c.class)
		}
	}

	for _, b := range batches {
		d.steerBatch(ctx, items, results, b, refs)
	}

	for i, j := range twinOf {
		r := results[j]
		results[i].SessionUUID = r.SessionUUID
		results[i].Kinds = r.Kinds
		results[i].Action = r.Action
		results[i].Drop = r.Drop
		results[i].Hold = r.Hold
		results[i].Redelivered = r.Action == ActionAck
	}

	for i := range results {
		if results[i].Action != ActionAck {
			continue
		}
		results[i].AckErr = d.ack(ctx, results[i].ItemID)
	}
	return results
}

// screen runs the checks before the coalescer. ok is true when the item is a
// steer candidate; otherwise res already holds its ack or hold.
func (d *Deliverer) screen(res *DeliverResult, it inbox.Item, ref SessionRef, found bool) (candidate, bool) {
	ev := it.Event
	if found {
		res.SessionUUID = ref.UUID
		if ref.SkipReason == SkipHidden || d.Sessions.IsHidden(ref.UUID) {
			d.drop(res, DropHiddenSession, "")
			return candidate{}, false
		}
	}
	if d.handled.has(ev.ID) {
		res.Action = ActionAck
		res.Redelivered = true
		return candidate{}, false
	}
	kinds, reason, ok := classify(ev)
	if !ok {
		d.drop(res, reason, "")
		return candidate{}, false
	}
	res.Kinds = kinds
	kind := kinds[0]

	var class ActorClass
	if d.Filter != nil {
		fr := d.Filter.Evaluate(kind, ev.Actor)
		class = fr.Class
		switch fr.Verdict {
		case VerdictDrop:
			d.drop(res, fr.Drop, class)
			return candidate{}, false
		case VerdictHold:
			d.hold(res, fr.Hold, class)
			return candidate{}, false
		}
	}

	switch {
	case !found, ref.SkipReason == SkipNotLive, ref.State == StateGone:
		d.drop(res, DropSessionNotLive, class)
		return candidate{}, false
	case ref.SkipReason != "":
		d.drop(res, DropRepoMismatch, class)
		return candidate{}, false
	case ref.State != StateLive:
		d.hold(res, HoldSessionSuspended, class)
		return candidate{}, false
	}

	sha := ""
	if ev.HeadSHA != nil {
		sha = *ev.HeadSHA
	}
	return candidate{
		kind: kind, sha: sha, class: class,
		key: CoalesceKey{SessionUUID: ref.UUID, Repo: ref.Repo, PR: ref.PR, Class: kind.Class()},
	}, true
}

func (d *Deliverer) steerBatch(ctx context.Context, items []inbox.Item, results []DeliverResult, b *steerBatch, refs []SessionRef) {
	dropAll := func(reason DropReason) {
		for _, c := range b.cands {
			d.drop(&results[c.idx], reason, c.class)
		}
	}
	holdAll := func(reason HoldReason) {
		for _, c := range b.cands {
			d.hold(&results[c.idx], reason, c.class)
		}
	}

	kinds := unionKinds(b.cands)
	text, err := render(kinds, b.ref.Repo, b.ref.PR)
	if err != nil {
		dropAll(DropRepoMismatch)
		return
	}
	if d.Sessions.Program(b.ref.UUID) == "" {
		dropAll(DropSessionNotLive)
		return
	}

	out, err := d.Steerer.Steer(ctx, SteerRequest{
		SessionUUID: b.ref.UUID, Repo: b.ref.Repo, PR: b.ref.PR, Kinds: kinds, Text: text,
	})
	if err != nil || out == OutcomeFailed {
		d.Log.Warn("prwatch.steer_failed", "session", b.ref.UUID, "pr", b.ref.PR)
		holdAll(HoldSteerFailed)
		if d.Notes != nil {
			if nerr := d.Notes.SteerFailed(b.ref.UUID, b.ref.Repo, b.ref.PR); nerr != nil {
				d.Log.Warn("prwatch.notify_failed", "session", b.ref.UUID)
			}
		}
		return
	}

	switch out {
	case OutcomeDelivered:
		d.delivered(items, results, b, kinds)
	case OutcomeDuplicate:
		d.duplicate(results, b)
	case OutcomeBusy:
		holdAll(HoldBusy)
	case OutcomeGuardBusy:
		holdAll(HoldGuardBusy)
	case OutcomeCoolingDown:
		holdAll(HoldCoolingDown)
	case OutcomeNoStatusSource:
		holdAll(HoldNoStatusSource)
	case OutcomeNotTracked:
		switch stateOf(refs, b.ref.UUID) {
		case StateGone:
			dropAll(DropSessionNotLive)
		case StateSuspended:
			holdAll(HoldSessionSuspended)
		default:
			holdAll(HoldNotTracked)
		}
	default: // OutcomeUnspecified and unknown values
		holdAll(HoldUnspecified)
	}
}

func (d *Deliverer) delivered(items []inbox.Item, results []DeliverResult, b *steerBatch, kinds []Kind) {
	now := d.Clock.Now()
	var oldest time.Time
	seen := map[CoalesceKey]bool{}
	for _, c := range b.cands {
		it := items[c.idx]
		if oldest.IsZero() || it.CreatedAt.Before(oldest) {
			oldest = it.CreatedAt
		}
		if !seen[c.key] {
			seen[c.key] = true
			d.Coalescer.RecordSteer(c.key, c.sha)
		}
		r := &results[c.idx]
		r.Action, r.Steered = ActionAck, true
		d.handled.put(it.Event.ID, struct{}{})
		d.lastHold.put(it.ID, "")
		d.Recorder.Record(Disposition{
			At: now, Kind: c.kind, Repo: b.ref.Repo, PR: b.ref.PR, SessionUUID: b.ref.UUID,
			ActorClass: c.class, Disposition: DispositionSteered,
		})
		cls := steerClassCI
		if c.class != "" {
			cls = string(c.class)
		}
		d.Metrics.Inc(MetricSteers, map[string]string{"kind": string(c.kind), "actor_class": cls})
		if d.Stranger != nil && c.kind.Class() == ClassHumanFeedback {
			d.Stranger.RecordSteer(b.ref.Repo, b.ref.PR, c.class)
		}
	}
	if d.Notes != nil {
		d.Notes.SteerDelivered(b.ref.UUID, b.ref.Repo, b.ref.PR)
	}
	if d.Followup != nil {
		d.Followup.Track(b.ref.UUID, b.ref.Repo, b.ref.PR)
	}
	d.Log.Info("prwatch.steer", "session", b.ref.UUID, "pr", b.ref.PR, "items", len(b.cands),
		"kinds", len(kinds), "item_to_steer_seconds", int64(now.Sub(oldest)/time.Second))
}

// duplicate applies the Duplicate row of D5: ack, except a CI item with a head
// SHA, which reached the guard only because its SHA is new to the coalescer
// (the guard window keys on PR plus reason set, not SHA), so it is held.
func (d *Deliverer) duplicate(results []DeliverResult, b *steerBatch) {
	for _, c := range b.cands {
		r := &results[c.idx]
		if c.kind == KindCI && c.sha != "" {
			d.hold(r, HoldCIDuplicateNewSHA, c.class)
			continue
		}
		r.Action = ActionAck
		d.markHandled(r)
		d.Metrics.Inc(MetricDuplicateSharedGuard, nil)
		d.Log.Debug("prwatch.duplicate_shared_guard", "session", b.ref.UUID, "pr", b.ref.PR)
	}
}

func (d *Deliverer) markHandled(r *DeliverResult) {
	d.handled.put(r.EventID, struct{}{})
	d.lastHold.put(r.ItemID, "")
}

func (d *Deliverer) drop(r *DeliverResult, reason DropReason, class ActorClass) {
	r.Action, r.Drop, r.Hold = ActionAck, reason, ""
	d.markHandled(r)
	var kind Kind
	if len(r.Kinds) > 0 {
		kind = r.Kinds[0]
	}
	d.Metrics.Inc(MetricDropped, map[string]string{"drop_reason": string(reason)})
	d.Recorder.Record(Disposition{
		At: d.Clock.Now(), Kind: kind, Repo: r.Repo, PR: r.PR, SessionUUID: r.SessionUUID,
		ActorClass: class, Disposition: DispositionDropped, Reason: string(reason),
	})
}

// hold leaves the item pending. The disposition and counter are recorded when
// the item's hold reason is new, not on every tick it stays held.
func (d *Deliverer) hold(r *DeliverResult, reason HoldReason, class ActorClass) {
	r.Action, r.Hold, r.Drop = ActionHold, reason, ""
	if prev, ok := d.lastHold.get(r.ItemID); ok && prev == reason {
		return
	}
	d.lastHold.put(r.ItemID, reason)
	var kind Kind
	if len(r.Kinds) > 0 {
		kind = r.Kinds[0]
	}
	d.Metrics.Inc(MetricHeld, map[string]string{"hold_reason": string(reason)})
	d.Recorder.Record(Disposition{
		At: d.Clock.Now(), Kind: kind, Repo: r.Repo, PR: r.PR, SessionUUID: r.SessionUUID,
		ActorClass: class, Disposition: DispositionHeld, Reason: string(reason),
	})
}

// ack treats a 404 as done: the item was already acked or removed.
func (d *Deliverer) ack(ctx context.Context, itemID string) error {
	err := d.Inbox.Ack(ctx, itemID)
	if err == nil || isAckNotFound(err) {
		return nil
	}
	d.Log.Warn("prwatch.ack_failed", "item", itemID)
	return err
}

// isAckNotFound matches the client's 404 on ack. The client has no typed
// not-found error (it folds 404 into ErrRejected), so the status text is used.
func isAckNotFound(err error) bool {
	return errors.Is(err, inbox.ErrRejected) && strings.Contains(err.Error(), "status 404")
}

// findSession matches the event's repo and PR, preferring a live session.
func findSession(refs []SessionRef, ev inbox.Event) (SessionRef, bool) {
	var best SessionRef
	found := false
	for _, r := range refs {
		if r.PR != ev.PullRequest || !strings.EqualFold(r.Repo, ev.Repository) {
			continue
		}
		if !found || (best.State != StateLive && r.State == StateLive) {
			best, found = r, true
		}
	}
	return best, found
}

func stateOf(refs []SessionRef, uuid string) SessionState {
	for _, r := range refs {
		if r.UUID == uuid {
			return r.State
		}
	}
	return StateGone
}

func unionKinds(cands []candidate) []Kind {
	present := map[Kind]bool{}
	for _, c := range cands {
		present[c.kind] = true
	}
	var out []Kind
	for _, k := range renderOrder {
		if present[k] {
			out = append(out, k)
		}
	}
	return out
}

// boundedIndex is a FIFO-evicting map with a fixed capacity.
type boundedIndex[V any] struct {
	m    map[string]V
	ring []string
	pos  int
}

func newBoundedIndex[V any](capacity int) *boundedIndex[V] {
	return &boundedIndex[V]{m: make(map[string]V, capacity), ring: make([]string, capacity)}
}

func (b *boundedIndex[V]) len() int { return len(b.m) }

func (b *boundedIndex[V]) has(k string) bool { _, ok := b.m[k]; return ok }

func (b *boundedIndex[V]) get(k string) (V, bool) { v, ok := b.m[k]; return v, ok }

// put stores k; a new key evicts the oldest once the index is full.
func (b *boundedIndex[V]) put(k string, v V) {
	if _, ok := b.m[k]; ok {
		b.m[k] = v
		return
	}
	if len(b.m) >= len(b.ring) {
		delete(b.m, b.ring[b.pos])
	}
	b.ring[b.pos] = k
	b.pos = (b.pos + 1) % len(b.ring)
	b.m[k] = v
}
