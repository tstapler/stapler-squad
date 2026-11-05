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
	Filter *ActorFilter
	// Mode is the human_feedback_mode; it decides whether a changes-requested
	// review on a suspended session is covered by the on-resume catch-up.
	Mode     HumanFeedbackMode
	Notes    DeliveryNotes
	Stranger StrangerWatch
	Followup FollowupObserver
	// DedupCapacity defaults to DefaultDedupCapacity.
	DedupCapacity int
	// OnPRClosed, when set, is told about a pull_request closed event so the
	// reconciler can unsubscribe the PR.
	OnPRClosed func(ctx context.Context, repo string, pr int)
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
	// changesRequested marks a changes-requested review (alt4 ci_other).
	changesRequested bool
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
	// Each (item, session) pair is one unit, so every session subscribed to a
	// PR is steered and an item is acked only when every unit is.
	type unit struct {
		ref   SessionRef
		found bool
	}
	results := make([]DeliverResult, len(items))
	twinOf := map[int]int{}
	firstByEvent := map[string]int{}
	heldPerKey := map[CoalesceKey]int{}
	var batches []*steerBatch
	batchByUUID := map[string]*steerBatch{}
	var units []*unit
	var uItems []inbox.Item
	var uResults []DeliverResult
	unitsOf := make([][]int, len(items))

	for i, it := range items {
		results[i] = DeliverResult{
			ItemID: it.ID, EventID: it.Event.ID, Repo: it.Event.Repository,
			PR: it.Event.PullRequest, CreatedAt: it.CreatedAt,
		}
		// A merged or closed PR is done: retire its subscription (D7). The
		// event itself is still dropped by classify below.
		if d.OnPRClosed != nil && isPullRequestClosed(it.Event) && validRepo(it.Event.Repository) {
			d.OnPRClosed(ctx, it.Event.Repository, it.Event.PullRequest)
		}
		if j, ok := firstByEvent[it.Event.ID]; ok && it.Event.ID != "" {
			twinOf[i] = j
			continue
		}
		firstByEvent[it.Event.ID] = i
		if !validRepo(it.Event.Repository) {
			// Never let an unvalidated repository reach logs, the ring buffer or status.
			results[i].Repo = ""
			d.drop(&results[i], DropRepoMismatch, "")
			continue
		}
		matches := findSessions(refs, it.Event)
		found := len(matches) > 0
		if !found {
			matches = []SessionRef{{}}
		}
		for _, m := range matches {
			unitsOf[i] = append(unitsOf[i], len(units))
			units = append(units, &unit{ref: m, found: found})
			uItems = append(uItems, it)
			uResults = append(uResults, results[i])
		}
	}

	for k, u := range units {
		i, ref := k, u.ref
		c, ok := d.screen(&uResults[i], uItems[i], ref, u.found)
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
			d.drop(&uResults[i], DropCoveredSameHeadSHA, c.class)
		case CoalesceFoldOverCap:
			d.drop(&uResults[i], DropFoldedOverCap, c.class)
		default: // CoalesceHold and anything unknown
			heldPerKey[c.key]++
			d.hold(&uResults[i], HoldWindowBatching, c.class)
		}
	}

	for _, b := range batches {
		d.steerBatch(ctx, uItems, uResults, b, refs)
	}

	for i, idxs := range unitsOf {
		if len(idxs) > 0 {
			results[i] = mergeUnits(results[i], uResults, idxs)
		}
	}

	for i, j := range twinOf {
		r := results[j]
		results[i].SessionUUID = r.SessionUUID
		results[i].Kinds = r.Kinds
		results[i].Action = r.Action
		results[i].Drop = r.Drop
		results[i].Hold = r.Hold
		results[i].Steered = r.Steered
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

// mergeUnits folds one item's per-session results into the item result: hold
// if any session is held (the first held session's reason), else ack.
func mergeUnits(base DeliverResult, units []DeliverResult, idxs []int) DeliverResult {
	out := base
	out.Action = ActionAck
	out.SessionUUID, out.Kinds = units[idxs[0]].SessionUUID, units[idxs[0]].Kinds
	redelivered := true
	var drop DropReason
	for _, k := range idxs {
		u := units[k]
		if u.Action == ActionHold {
			if out.Action != ActionHold {
				out.Action, out.Hold = ActionHold, u.Hold
				out.SessionUUID, out.Kinds = u.SessionUUID, u.Kinds
			}
			redelivered = false
			continue
		}
		out.Steered = out.Steered || u.Steered
		if !u.Redelivered {
			redelivered = false
		}
		if drop == "" {
			drop = u.Drop
		}
	}
	if out.Action == ActionHold {
		out.Redelivered = false
		return out
	}
	if !out.Steered {
		out.Drop = drop
	}
	out.Redelivered = redelivered
	return out
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
	if d.handled.has(deliveryKey(ev.ID, ref.UUID)) {
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
	}
	changesRequested := kind == KindReview && detailString(ev.Details, "state") == "changes_requested"
	switch verdict, reason := SessionGate(ref.State, kind, changesRequested, d.Mode); verdict {
	case SessionDrop:
		d.drop(res, DropSessionNotLive, class)
		return candidate{}, false
	case SessionHold:
		d.hold(res, reason, class)
		return candidate{}, false
	}

	sha := ""
	if ev.HeadSHA != nil {
		sha = *ev.HeadSHA
	}
	return candidate{
		kind: kind, sha: sha, class: class, changesRequested: changesRequested,
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
		if verdict, reason := NotTrackedGate(stateOf(refs, b.ref.UUID)); verdict == SessionDrop {
			dropAll(DropSessionNotLive)
		} else {
			holdAll(reason)
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
		d.handled.put(deliveryKey(it.Event.ID, b.ref.UUID), struct{}{})
		d.lastHold.put(deliveryKey(it.ID, b.ref.UUID), "")
		d.Recorder.Record(Disposition{
			At: now, Kind: c.kind, Repo: b.ref.Repo, PR: b.ref.PR, SessionUUID: b.ref.UUID,
			ActorClass: c.class, Disposition: DispositionSteered,
		})
		cls := actorClassLabel(c.class)
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
	latency := now.Sub(oldest)
	d.Log.Info("prwatch.steer", "session", b.ref.UUID, "repo", b.ref.Repo, "pr", b.ref.PR,
		"kind", string(b.cands[0].kind), "outcome", string(OutcomeDelivered),
		"actor_class", actorClassLabel(b.cands[0].class), "items", len(b.cands), "kinds", len(kinds), "item_to_steer_seconds", int64(latency/time.Second),
		"alt4_class", string(alt4Class(b.cands, latency)))
}

// actorClassLabel is the only actor form that reaches logs and counters.
func actorClassLabel(c ActorClass) string {
	if c == "" {
		return steerClassCI
	}
	return string(c)
}

// Alt4Class buckets a delivered steer for the alt-4 (60s poller) counterfactual.
type Alt4Class string

const (
	Alt4TextEvent   Alt4Class = "text_event"
	Alt4CISubminute Alt4Class = "ci_subminute"
	Alt4CIOther     Alt4Class = "ci_other"

	// ciSubminuteBelow is the latency under which a CI steer would not have
	// gained from a faster path than a 60s poller.
	ciSubminuteBelow = 30 * time.Second
)

// alt4Class: any plain text event makes the steer text_event; otherwise a
// changes-requested review or a slow CI steer is ci_other. Catch-up steers
// never reach here (no inbox timestamp).
func alt4Class(cands []candidate, latency time.Duration) Alt4Class {
	changes := false
	for _, c := range cands {
		switch {
		case c.changesRequested:
			changes = true
		case c.kind != KindCI:
			return Alt4TextEvent
		}
	}
	if changes || latency >= ciSubminuteBelow {
		return Alt4CIOther
	}
	return Alt4CISubminute
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
	d.handled.put(deliveryKey(r.EventID, r.SessionUUID), struct{}{})
	d.lastHold.put(deliveryKey(r.ItemID, r.SessionUUID), "")
}

func (d *Deliverer) drop(r *DeliverResult, reason DropReason, class ActorClass) {
	r.Action, r.Drop, r.Hold = ActionAck, reason, ""
	d.markHandled(r)
	var kind Kind
	if len(r.Kinds) > 0 {
		kind = r.Kinds[0]
	}
	d.Metrics.Inc(MetricDropped, map[string]string{"drop_reason": string(reason)})
	d.Log.Info("prwatch.drop", "session", r.SessionUUID, "repo", r.Repo, "pr", r.PR,
		"kind", string(kind), "outcome", "dropped", "reason", string(reason), "actor_class", actorClassLabel(class))
	d.Recorder.Record(Disposition{
		At: d.Clock.Now(), Kind: kind, Repo: r.Repo, PR: r.PR, SessionUUID: r.SessionUUID,
		ActorClass: class, Disposition: DispositionDropped, Reason: string(reason),
	})
}

// hold leaves the item pending. The disposition and counter are recorded when
// the item's hold reason is new, not on every tick it stays held.
func (d *Deliverer) hold(r *DeliverResult, reason HoldReason, class ActorClass) {
	r.Action, r.Hold, r.Drop = ActionHold, reason, ""
	if prev, ok := d.lastHold.get(deliveryKey(r.ItemID, r.SessionUUID)); ok && prev == reason {
		return
	}
	d.lastHold.put(deliveryKey(r.ItemID, r.SessionUUID), reason)
	var kind Kind
	if len(r.Kinds) > 0 {
		kind = r.Kinds[0]
	}
	d.Metrics.Inc(MetricHeld, map[string]string{"hold_reason": string(reason)})
	d.Log.Info("prwatch.held", "session", r.SessionUUID, "repo", r.Repo, "pr", r.PR,
		"kind", string(kind), "outcome", "held", "reason", string(reason), "actor_class", actorClassLabel(class))
	d.Recorder.Record(Disposition{
		At: d.Clock.Now(), Kind: kind, Repo: r.Repo, PR: r.PR, SessionUUID: r.SessionUUID,
		ActorClass: class, Disposition: DispositionHeld, Reason: string(reason),
	})
}

// ack treats a 404 as done: the item was already acked or removed.
func (d *Deliverer) ack(ctx context.Context, itemID string) error {
	err := d.Inbox.Ack(ctx, itemID)
	if err == nil || errors.Is(err, inbox.ErrNotFound) {
		return nil
	}
	d.Log.Warn("prwatch.ack_failed", "item", itemID)
	return err
}

// findSessions returns every session on the event's repo and PR, so each is steered.
func findSessions(refs []SessionRef, ev inbox.Event) []SessionRef {
	var out []SessionRef
	for _, r := range refs {
		if r.PR == ev.PullRequest && strings.EqualFold(r.Repo, ev.Repository) {
			out = append(out, r)
		}
	}
	return out
}

// deliveryKey scopes dedup and hold memory to one (id, session) delivery.
func deliveryKey(id, sessionUUID string) string { return id + "|" + sessionUUID }

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
