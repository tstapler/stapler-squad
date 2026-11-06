package wakes

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

// DefaultClaimLease is how old a claimed row must be before startup reconcile
// re-queues it (ADR-006 rule 2).
const DefaultClaimLease = 2 * time.Minute

// lastFiredPruneAt bounds the debounce map: past this many entries, expired ones are dropped.
const lastFiredPruneAt = 1024

// Config wires a Dispatcher. Store, Steerer, Owners and Admission are required.
type Config struct {
	Store     Store
	Clock     Clock
	Steerer   Steerer
	Admission Admission
	Owners    OwnerLookup
	Notifier  Notifier
	// Events is the in-process bus; nil disables event-type triggers.
	Events  EventSource
	Backlog BacklogState
	// Sessions, Guidance and PRs back the session_state, guidance_answered and
	// pr_event/ci_event sources; a nil one makes its trigger type unavailable.
	Sessions SessionStates
	Guidance GuidanceState
	PRs      PRSource
	// Peers enables the PeerHelpEscalator over the open peer-help requests.
	Peers   PeerRequests
	Leader  LeaderGuard
	Metrics *Metrics

	// Limits are the operator-set values of the Wake limits table; zero values
	// keep the defaults and everything is clamped to the compiled ceilings.
	Limits LimitSettings

	// TypeEnabled reports whether a trigger type's flag scope is on, read at
	// fire time (ADR-008). Nil means every type is on. A pending row of an off
	// type is neither fired nor marked missed, except by expires_at.
	TypeEnabled func(domain.WakeTriggerType) bool

	ClaimLease    time.Duration
	RetryInterval time.Duration
}

// Dispatcher is the single admission path for wakes (ADR-006 rule 3): one bus
// consumer indexing pending triggers by (type, key), one timer over the
// durable table, guarded claims, and SteerDelivery for the pane writes. It
// subscribes before reading state, reconciles from the database before it
// reports ready, and holds the single-leader guard.
type Dispatcher struct {
	cfg      Config
	delivery *SteerDelivery
	reviver  *Reviver
	timer    *Timekeeper
	index    *triggerIndex

	// escalator pulls the operator into stalled peer-help requests; nil when
	// Config.Peers is unset.
	escalator *Escalator

	limits  Limits
	limiter *limiter
	admit   AdmitFunc // claim-time gate; nil admits everything

	ready     chan struct{}
	changed   chan struct{}
	startedAt time.Time

	mu            sync.Mutex
	running       bool
	cancel        context.CancelFunc
	release       func()
	lastFired     map[string]time.Time
	digest        *digest
	lastDigest    map[string]time.Time
	reviveAlerted map[string]struct{}
	reviveRunning map[string]struct{} // needs_revive rows with a revive in flight
	started       bool                // Start ran once; a Dispatcher is single-use
	matchers      []EventMatcher
	predicates    map[domain.WakeTriggerType]Predicate

	wg sync.WaitGroup // long-running loops
	bg idleGroup      // spawned one-shot work (revives)

	// testBeforeRead runs between the bus subscription and the first state read.
	testBeforeRead func()
}

// NewDispatcher builds a stopped Dispatcher.
func NewDispatcher(cfg Config) *Dispatcher {
	if cfg.Clock == nil {
		cfg.Clock = RealClock{}
	}
	if cfg.Leader == nil {
		cfg.Leader = NoLeader{}
	}
	if cfg.Metrics == nil {
		cfg.Metrics = NewMetrics()
	}
	if cfg.ClaimLease <= 0 {
		cfg.ClaimLease = DefaultClaimLease
	}
	limits := ResolveLimits(cfg.Limits)
	d := &Dispatcher{
		cfg:        cfg,
		limits:     limits,
		limiter:    newLimiter(),
		index:      newTriggerIndex(),
		ready:      make(chan struct{}),
		changed:    make(chan struct{}, 1),
		lastFired:  map[string]time.Time{},
		lastDigest: map[string]time.Time{},
		predicates: map[domain.WakeTriggerType]Predicate{},
	}
	d.reviver = NewReviver(cfg.Admission, cfg.Owners, cfg.Clock, limits.ReviveLockHold)
	d.admit = d.rateGate
	d.delivery = NewSteerDelivery(SteerDeliveryConfig{
		Store: cfg.Store, Steerer: cfg.Steerer, Owners: cfg.Owners, Clock: cfg.Clock, Metrics: cfg.Metrics,
		RetryInterval: cfg.RetryInterval,
		BusyHold:      limits.BusyHold,
		OnEnded:       d.ended,
		OnNeedsRevive: func(ctx context.Context, row *session.WakeTriggerData) {
			d.spawnRevive(ctx, row)
		},
	})
	d.timer = NewTimekeeper(cfg.Store, cfg.Clock, d.onClaimed,
		WithMissedFunc(d.timerMissed),
		WithHoldFunc(func(row *session.WakeTriggerData) bool { return d.typeOff(row.Type) }),
		WithAdmitFunc(func(ctx context.Context, row *session.WakeTriggerData, now time.Time) bool {
			return d.allowClaim(ctx, row, now)
		}),
	)
	if cfg.Peers != nil {
		d.escalator = NewEscalator(EscalatorConfig{
			Requests: cfg.Peers, Owners: cfg.Owners, Notifier: cfg.Notifier,
			Events: cfg.Events, Clock: cfg.Clock, Metrics: cfg.Metrics,
		})
	}
	d.registerBacklogSource()
	d.registerSessionSource()
	d.registerGuidanceSource()
	d.registerPRSource()
	return d
}

// ChangeHook is what the repository calls after any wake-row change: it
// re-arms the timer and asks the loop to refresh the index.
func (d *Dispatcher) ChangeHook() func() {
	return func() {
		d.timer.Rearm()
		if d.escalator != nil {
			d.escalator.Rearm()
		}
		select {
		case d.changed <- struct{}{}:
		default:
		}
	}
}

// Ready is closed once startup reconcile finished; events are consumed only
// after it.
func (d *Dispatcher) Ready() <-chan struct{} { return d.ready }

// Delivery exposes the SteerDelivery cache (reconcile and tests).
func (d *Dispatcher) Delivery() *SteerDelivery { return d.delivery }

// Metrics returns the dispatcher's counters.
func (d *Dispatcher) Metrics() *Metrics { return d.cfg.Metrics }

func (d *Dispatcher) typeOff(t domain.WakeTriggerType) bool {
	return d.cfg.TypeEnabled != nil && !d.cfg.TypeEnabled(t)
}

func (d *Dispatcher) allowClaim(ctx context.Context, row *session.WakeTriggerData, now time.Time) bool {
	return d.admit == nil || d.admit(ctx, row, now)
}

// idleGroup counts running work and lets callers wait for zero. Unlike
// sync.WaitGroup, Add may race with Wait: the event loop spawns revives while
// WaitIdle callers (tests, Stop) wait, which the WaitGroup contract forbids and
// the race detector reports.
type idleGroup struct {
	mu      sync.Mutex
	n       int
	waiters []chan struct{}
}

func (g *idleGroup) Add() {
	g.mu.Lock()
	g.n++
	g.mu.Unlock()
}

func (g *idleGroup) Done() {
	g.mu.Lock()
	g.n--
	if g.n == 0 {
		for _, w := range g.waiters {
			close(w)
		}
		g.waiters = nil
	}
	g.mu.Unlock()
}

// Wait returns once no work is running.
func (g *idleGroup) Wait() {
	g.mu.Lock()
	if g.n == 0 {
		g.mu.Unlock()
		return
	}
	w := make(chan struct{})
	g.waiters = append(g.waiters, w)
	g.mu.Unlock()
	<-w
}

// spawn runs fn on a goroutine joined by Stop (revives block up to the revive
// lock budget, which must not stall the timer or event loop).
func (d *Dispatcher) spawn(fn func()) {
	d.bg.Add()
	go func() {
		defer d.bg.Done()
		fn()
	}()
}

// spawnRevive runs processNeedsRevive for row unless one is already in flight
// for it, so a slow revive does not collect a goroutine per reconcile tick.
func (d *Dispatcher) spawnRevive(ctx context.Context, row *session.WakeTriggerData) {
	d.mu.Lock()
	if _, busy := d.reviveRunning[row.ID]; busy {
		d.mu.Unlock()
		return
	}
	if d.reviveRunning == nil {
		d.reviveRunning = map[string]struct{}{}
	}
	d.reviveRunning[row.ID] = struct{}{}
	d.mu.Unlock()
	d.spawn(func() {
		defer func() {
			d.mu.Lock()
			delete(d.reviveRunning, row.ID)
			d.mu.Unlock()
		}()
		d.processNeedsRevive(ctx, row)
	})
}

// WaitIdle blocks until the spawned background work (revives) has finished.
func (d *Dispatcher) WaitIdle() { d.bg.Wait() }

// Start takes the leader guard, subscribes to the bus, reads and reconciles
// state, then starts the loops. It returns ErrNotLeader when another process
// leads. The subscription comes first so an event published during the initial
// read is queued, not lost; the guarded claim makes a row that both the event
// and the read find fire once.
func (d *Dispatcher) Start(ctx context.Context) error {
	d.mu.Lock()
	if d.running {
		d.mu.Unlock()
		return errors.New("wakes: dispatcher already running")
	}
	if d.started {
		d.mu.Unlock()
		return ErrDispatcherReused
	}
	release, ok, err := d.cfg.Leader.TryAcquire()
	if err != nil {
		d.mu.Unlock()
		return fmt.Errorf("wakes: leader guard: %w", err)
	}
	if !ok {
		d.mu.Unlock()
		return ErrNotLeader
	}
	loopCtx, cancel := context.WithCancel(ctx)
	d.running, d.started, d.cancel, d.release = true, true, cancel, release
	d.startedAt = d.cfg.Clock.Now()
	d.mu.Unlock()

	var evCh <-chan *events.Event
	if d.cfg.Events != nil {
		evCh, _ = d.cfg.Events.Subscribe(loopCtx)
	}
	if d.testBeforeRead != nil {
		d.testBeforeRead()
	}
	d.reconcileStartup(loopCtx)
	close(d.ready)

	d.wg.Add(2)
	go func() { defer d.wg.Done(); d.delivery.Run(loopCtx) }()
	go func() { defer d.wg.Done(); d.loop(loopCtx, evCh) }()
	d.startPRWatcher(loopCtx)
	if err := d.timer.Start(loopCtx); err != nil {
		d.shutdown()
		return fmt.Errorf("wakes: start timer: %w", err)
	}
	if d.escalator != nil {
		if err := d.escalator.Start(loopCtx); err != nil {
			log.Warn("wake dispatcher: peer help escalator not started", errKey, err)
		}
	}
	return nil
}

// Stop cancels the loops, joins them and releases the leader guard.
func (d *Dispatcher) Stop(ctx context.Context) error {
	d.mu.Lock()
	if !d.running {
		d.mu.Unlock()
		return nil
	}
	d.mu.Unlock()
	if err := d.timer.Stop(ctx); err != nil {
		log.Warn("wake dispatcher: timer stop", errKey, err)
	}
	d.shutdown()
	if d.escalator != nil {
		if err := d.escalator.Stop(ctx); err != nil {
			log.Warn("wake dispatcher: escalator stop", errKey, err)
		}
	}
	done := make(chan struct{})
	go func() { d.wg.Wait(); d.bg.Wait(); close(done) }()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("wakes: dispatcher did not stop: %w", ctx.Err())
	}
}

func (d *Dispatcher) shutdown() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.running {
		return
	}
	d.cancel()
	d.release()
	d.running = false
}

// loop consumes bus events and index-refresh requests until ctx ends.
func (d *Dispatcher) loop(ctx context.Context, evCh <-chan *events.Event) {
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-evCh:
			if !ok {
				evCh = nil
				continue
			}
			d.handleEvent(ctx, ev)
		case <-d.changed:
			d.reindex(ctx)
		}
	}
}

func (d *Dispatcher) reindex(ctx context.Context) {
	rows, err := d.cfg.Store.ListPendingWakeTriggers(ctx)
	if err != nil {
		if ctx.Err() == nil {
			log.Warn("wake dispatcher: reindex failed", errKey, err)
		}
		return
	}
	d.index.replace(rows)
}

func (d *Dispatcher) handleEvent(ctx context.Context, ev *events.Event) {
	for _, m := range d.matchers {
		for _, ref := range m(ev) {
			d.fireCell(ctx, ref, "")
		}
	}
}

// fireCell fires the pending rows in ref's cell that the match satisfies; detail,
// when set, is appended to the wake text (what the event was).
func (d *Dispatcher) fireCell(ctx context.Context, ref EventMatch, detail string) {
	for _, row := range d.index.lookup(ref.Type, ref.Key) {
		if !d.cellRowMatches(ctx, row, ref) {
			continue
		}
		fired := *row // the shared index row stays unmodified if this fire is deferred
		withEventDetail(&fired, detail)
		d.fire(ctx, &fired)
	}
}

func (d *Dispatcher) cellRowMatches(ctx context.Context, row *session.WakeTriggerData, ref EventMatch) bool {
	if !ref.Check {
		return slices.Contains(ref.Kinds, specKind(row.Spec))
	}
	holds, err := d.conditionHolds(ctx, row)
	if err != nil {
		log.Warn("wake dispatcher: predicate failed", "trigger_id", row.ID, errKey, err)
	}
	return holds
}

// withEventDetail appends a one-line description of the firing event to the
// in-memory row's payload; the stored payload is untouched, so a re-queue after
// a restart delivers the plain text.
func withEventDetail(row *session.WakeTriggerData, detail string) {
	if detail != "" {
		row.Action.Payload += "\n[event] " + detail
	}
}

// fire claims a pending event-type row and hands it on. Several events for one
// trigger inside the debounce window collapse into the first.
func (d *Dispatcher) fire(ctx context.Context, row *session.WakeTriggerData) {
	id, err := uuid.Parse(row.ID)
	if err != nil {
		log.Error("wake dispatcher: unparseable trigger id", "id", row.ID, errKey, err)
		return
	}
	now := d.cfg.Clock.Now()
	if row.ExpiresAt != nil && !row.ExpiresAt.After(now) {
		d.expire(ctx, id, row, now)
		return
	}
	if d.typeOff(row.Type) || d.debounced(row.ID, now) {
		return
	}
	if !d.allowClaim(ctx, row, now) {
		return
	}
	applied, err := d.cfg.Store.ClaimWakeTrigger(ctx, id, now)
	if err != nil {
		log.Warn("wake dispatcher: claim failed", "trigger_id", row.ID, errKey, err)
		return
	}
	if !applied {
		return
	}
	d.index.remove(row)
	row = d.withGuidanceAnswer(ctx, row)
	row.Status, row.ClaimedAt = domain.WakeStatusClaimed, &now
	d.onClaimed(ctx, row)
}

func (d *Dispatcher) debounced(id string, now time.Time) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if last, ok := d.lastFired[id]; ok && now.Sub(last) < d.limits.Debounce {
		return true
	}
	d.lastFired[id] = now
	if len(d.lastFired) > lastFiredPruneAt {
		for k, t := range d.lastFired {
			if now.Sub(t) >= d.limits.Debounce {
				delete(d.lastFired, k)
			}
		}
	}
	return false
}

// expire ends a pending row that passed expires_at before it could fire.
func (d *Dispatcher) expire(ctx context.Context, id uuid.UUID, row *session.WakeTriggerData, now time.Time) {
	applied, err := d.cfg.Store.MarkWakeMissed(ctx, id, domain.WakeReasonExpired, now)
	if err != nil {
		log.Warn("wake dispatcher: mark missed failed", "trigger_id", row.ID, errKey, err)
		return
	}
	d.index.remove(row)
	if applied {
		d.timerMissed(ctx, row)
	}
}

// timerMissed is told of a row that expired before it could be claimed.
func (d *Dispatcher) timerMissed(ctx context.Context, row *session.WakeTriggerData) {
	d.cfg.Metrics.Inc(MetricTrigger, string(row.Type), string(domain.WakeStatusMissed))
	d.ended(ctx, row, domain.WakeStatusMissed, domain.WakeReasonExpired)
}

// onClaimed routes a freshly claimed (or re-queued) row by action kind.
func (d *Dispatcher) onClaimed(ctx context.Context, row *session.WakeTriggerData) {
	d.cfg.Metrics.Inc(MetricTrigger, string(row.Type), string(domain.WakeStatusClaimed))
	d.index.remove(row)
	d.dispatchClaimed(ctx, row)
}

// dispatchClaimed hands a claimed row to its action: reminders are raised at
// once, steer_text rows go to SteerDelivery.
func (d *Dispatcher) dispatchClaimed(ctx context.Context, row *session.WakeTriggerData) {
	if row.Action.Kind == domain.WakeActionNotifyOperator {
		d.deliverReminder(ctx, row)
		return
	}
	d.delivery.Enqueue(ctx, row)
}

// deliverReminder raises the INFO notification of a notify_operator wake. It
// is exempt from every owner-state guard: nothing is written to the owner.
func (d *Dispatcher) deliverReminder(ctx context.Context, row *session.WakeTriggerData) {
	if d.cfg.Notifier == nil {
		log.Warn("wake dispatcher: no notifier wired; reminder stays claimed", "trigger_id", row.ID)
		return
	}
	id, err := uuid.Parse(row.ID)
	if err != nil {
		return
	}
	exists := true
	if facts, err := d.cfg.Owners.Owner(ctx, row.OwnerSessionUUID); err == nil {
		exists = facts.Exists
	}
	d.cfg.Notifier.Notify(reminderNotification(row, exists))
	now := d.cfg.Clock.Now()
	applied, err := d.cfg.Store.MarkWakeDelivered(ctx, id, now, nextFireAfter(row, now))
	if err != nil {
		log.Error("wake dispatcher: reminder raised but MarkWakeDelivered failed", "trigger_id", row.ID, errKey, err)
		return
	}
	if applied {
		d.cfg.Metrics.Inc(MetricTrigger, string(row.Type), string(domain.WakeStatusDelivered))
	}
}

// ended is the one place a missed or skipped row reaches the operator: a
// failure-class notification, or a digest entry while a reconcile run is open.
func (d *Dispatcher) ended(_ context.Context, row *session.WakeTriggerData, state domain.WakeStatus, reason domain.WakeReason) {
	if row.Status == domain.WakeStatusClaimed {
		d.limiter.release(row.ID) // a claim that never delivered gives its hourly slot back
	}
	d.mu.Lock()
	dg := d.digest
	d.mu.Unlock()
	if dg != nil {
		dg.add(row.OwnerSessionUUID, row.ID)
		return
	}
	if d.cfg.Notifier != nil {
		d.cfg.Notifier.Notify(failureNotification(row, state, reason))
	}
}

// processNeedsRevive tries to bring a needs_revive row's stopped owner back
// and, once it is live, re-admits the row into delivery.
func (d *Dispatcher) processNeedsRevive(ctx context.Context, row *session.WakeTriggerData) {
	id, err := uuid.Parse(row.ID)
	if err != nil {
		return
	}
	now := d.cfg.Clock.Now()
	if row.ExpiresAt != nil && !row.ExpiresAt.After(now) {
		d.expire(ctx, id, row, now)
		return
	}
	// Re-read: the user may have cancelled the wake since the reconcile snapshot.
	if cur, gerr := d.cfg.Store.GetWakeTrigger(ctx, id); gerr == nil && (cur == nil || cur.Status != domain.WakeStatusNeedsRevive) {
		return
	}
	outcome, err := d.reviver.TryRevive(ctx, row.OwnerSessionUUID)
	if err != nil {
		log.Warn("wake dispatcher: revive failed; row stays needs_revive", "trigger_id", row.ID, errKey, err)
	}
	switch outcome {
	case ReviveOwnerLive:
		d.readmit(ctx, id, row)
	case ReviveOwnerGone:
		d.skipGone(ctx, id, row)
	case ReviveNotDone:
	}
}

func (d *Dispatcher) readmit(ctx context.Context, id uuid.UUID, row *session.WakeTriggerData) {
	now := d.cfg.Clock.Now()
	applied, err := d.cfg.Store.ReadmitWakeTrigger(ctx, id, now)
	if err != nil {
		log.Warn("wake dispatcher: readmit failed", "trigger_id", row.ID, errKey, err)
		return
	}
	if !applied {
		return
	}
	row.Status, row.ClaimedAt = domain.WakeStatusClaimed, &now
	d.delivery.Enqueue(ctx, row)
}

// skipGone ends a needs_revive row whose owner stopped qualifying meanwhile.
func (d *Dispatcher) skipGone(ctx context.Context, id uuid.UUID, row *session.WakeTriggerData) {
	facts, err := d.cfg.Owners.Owner(ctx, row.OwnerSessionUUID)
	if err != nil {
		return
	}
	v := Decide(true, facts)
	if v.Kind != VerdictSkip {
		return
	}
	now := d.cfg.Clock.Now()
	applied, err := d.cfg.Store.MarkWakeSkipped(ctx, id, v.Reason, now)
	if err != nil || !applied {
		return
	}
	d.cfg.Metrics.Inc(MetricTrigger, string(row.Type), string(domain.WakeStatusSkipped))
	d.ended(ctx, row, domain.WakeStatusSkipped, v.Reason)
}

// timerClaim claims row through the same path the timer uses; the entry point
// tests use to drive one due row without a running loop.
func (d *Dispatcher) timerClaim(ctx context.Context, row *session.WakeTriggerData) {
	now := d.cfg.Clock.Now()
	if !d.allowClaim(ctx, row, now) {
		return
	}
	id, err := uuid.Parse(row.ID)
	if err != nil {
		return
	}
	applied, err := d.cfg.Store.ClaimWakeTrigger(ctx, id, now)
	if err != nil || !applied {
		return
	}
	row.Status, row.ClaimedAt = domain.WakeStatusClaimed, &now
	if row.NextFireAt != nil {
		d.cfg.Metrics.ObserveFireLag(string(row.Type), now.Sub(*row.NextFireAt))
	}
	d.onClaimed(ctx, row)
}
