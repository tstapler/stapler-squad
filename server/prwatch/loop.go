package prwatch

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tstapler/stapler-squad/server/prwatch/inbox"
)

const (
	// EventsPageSize is the inbox's page size: GET /v1/events returns at most
	// this many of the oldest pending items.
	EventsPageSize = 200
	// MaxPagesPerTick caps the page loop; any remainder waits for the next
	// tick (D13.4).
	MaxPagesPerTick = 5
	// warnInterval rate-limits inbox warnings to one per error class (D16).
	warnInterval = 5 * time.Minute
	// unauthorizedBackoff pauses polling after a 401 so a rejected token is
	// not hammered; a rotated token is picked up when the pause ends.
	unauthorizedBackoff = 30 * time.Second
	// defaultPollInterval applies when Settings.PollInterval is unset.
	defaultPollInterval = 5 * time.Second

	// MetricInboxErrors counts failed inbox calls by error_class.
	MetricInboxErrors = "prwatch_inbox_errors_total"
)

// Inbox error classes used for the counter label, the status surface and the
// warning rate limit. They never carry a message.
const (
	errClassUnauthorized = "unauthorized"
	errClassTokenMissing = "token_missing"
	errClassTokenPerms   = "token_permissions"
	errClassTransport    = "transport"
	errClassServer       = "server_error"
	errClassDecode       = "decode"
	errClassRejected     = "rejected"
	errClassOther        = "other"
)

func inboxErrClass(err error) string {
	switch {
	case errors.Is(err, inbox.ErrUnauthorized):
		return errClassUnauthorized
	case errors.Is(err, inbox.ErrTokenMissing):
		return errClassTokenMissing
	case errors.Is(err, inbox.ErrTokenPermissions):
		return errClassTokenPerms
	case errors.Is(err, inbox.ErrTransport):
		return errClassTransport
	case errors.Is(err, inbox.Err5xx):
		return errClassServer
	case errors.Is(err, inbox.ErrDecode):
		return errClassDecode
	case errors.Is(err, inbox.ErrRejected):
		return errClassRejected
	}
	return errClassOther
}

// LoopDeps are the poll loop's collaborators. Inbox, Sessions, Engine,
// Notifier, Coalescer, Metrics, Log and Clock are required; the rest are
// optional and skipped when nil.
type LoopDeps struct {
	Inbox     Inbox
	Sessions  Sessions
	Engine    *Deliverer
	Notifier  Notifier
	Coalescer Coalescer
	Metrics   Metrics
	Log       Logger
	Clock     Clock
	Settings  Settings

	Catchup  *Catchup
	Leader   *Leadership
	State    InboxStateTracker
	Followup FollowupObserver
	HeldAge  *HeldAge
	Status   *StatusTracker
	// Login reports whether the operator's GitHub login resolved (status only;
	// resolution runs in its own goroutine, never in a tick).
	Login interface{ Cached() (string, bool) }
	// Enabled is the live feature flag, read at the start of every tick. Nil
	// means on.
	Enabled func() bool
}

// Poller is the PR watch loop. One tick: flag, leader, session view, catch-up,
// subscription reconcile, then (only when something is watched) the events
// pages. A single tick is in flight at a time.
type Poller struct {
	LoopDeps
	coverage   *Coverage
	reconciler *Reconciler
	watched    *watchObserver
	sessions   *tickSessions

	inFlight atomic.Bool

	// Touched only by the goroutine holding inFlight.
	lastWarn     map[string]time.Time
	failing      bool
	backoffUntil time.Time
}

// NewPoller builds the loop and the reconcile and coverage collaborators it
// owns, so that every successful watch is observed for the catch-up.
func NewPoller(d LoopDeps) *Poller {
	if d.Settings.PollInterval <= 0 {
		d.Settings.PollInterval = defaultPollInterval
	}
	p := &Poller{LoopDeps: d, lastWarn: map[string]time.Time{}}
	p.watched = &watchObserver{Inbox: d.Inbox, done: map[string]bool{}}
	p.sessions = &tickSessions{Sessions: d.Sessions}
	p.coverage = NewCoverage(p.watched, d.Notifier, d.Clock, d.Log, d.Settings)
	p.reconciler = NewReconciler(p.watched, p.sessions, p.coverage, d.Coalescer, d.Clock, d.Log, d.Settings)
	return p
}

// Coverage exposes the webhook coverage tracker the loop drives.
func (p *Poller) Coverage() *Coverage { return p.coverage }

// Run ticks every PollInterval until ctx ends. A tick that outlasts the
// interval is counted as an overrun and the slots it covered are skipped (no
// queueing, no overlap). The wait is real time; tests call Tick directly.
func (p *Poller) Run(ctx context.Context) {
	interval := p.Settings.PollInterval
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		start := time.Now()
		p.Tick(ctx)
		elapsed := time.Since(start)
		wait := interval - elapsed
		if elapsed >= interval {
			p.Metrics.Inc(MetricTickOverrun, nil)
			wait = interval - elapsed%interval
		}
		timer.Reset(wait)
	}
}

// Tick runs one tick. It returns false, counting an overrun, when the
// previous tick is still in flight. A panic in a port is recovered and the
// loop continues on the next tick.
func (p *Poller) Tick(ctx context.Context) bool {
	if !p.inFlight.CompareAndSwap(false, true) {
		p.Metrics.Inc(MetricTickOverrun, nil)
		return false
	}
	defer p.inFlight.Store(false)
	defer func() {
		if r := recover(); r != nil {
			p.Log.Warn("prwatch.tick_panic")
		}
	}()
	tctx, cancel := context.WithTimeout(ctx, p.Settings.PollInterval)
	defer cancel()
	p.tick(tctx)
	return true
}

func (p *Poller) tick(ctx context.Context) {
	if p.Enabled != nil && !p.Enabled() {
		if p.Status != nil {
			p.Status.SetEnabled(false, 0)
		}
		return
	}
	if p.Leader != nil && !p.Leader.Held() {
		if p.Status != nil {
			p.Status.SetLeaderHeld(false)
		}
		if p.State != nil {
			p.State.Observe(InboxStateLeaderNotHeld)
		}
		return
	}
	if p.Status != nil {
		p.Status.SetLeaderHeld(true)
	}
	if p.Followup != nil {
		p.Followup.Tick()
	}
	if p.Clock.Now().Before(p.backoffUntil) {
		return
	}

	p.sessions.reset()
	refs, err := p.sessions.List(ctx)
	if err != nil {
		p.Log.Warn("prwatch.sessions_unavailable")
		return
	}
	retained, _ := retainedSet(refs)
	if p.Status != nil {
		p.Status.SetEnabled(true, len(retained))
		p.Status.SetSessions(refs)
		if p.Login != nil {
			_, ok := p.Login.Cached()
			p.Status.SetLoginResolved(ok)
		}
	}

	rerr := p.reconciler.Reconcile(ctx)
	p.catchUp(ctx, refs)
	p.publishSubscriptionStatus(refs)
	if rerr != nil {
		p.inboxFailed(rerr)
		return // a second request would only stack another timeout on the bound
	}
	if len(retained) == 0 {
		p.inboxHealthy()
		return
	}
	p.pollEvents(ctx)
}

// catchUp runs the first-subscribe catch-up for PRs watched this tick, then
// the resume and retry pass.
func (p *Poller) catchUp(ctx context.Context, refs []SessionRef) {
	if p.Catchup == nil {
		return
	}
	for _, key := range p.watched.take() {
		for _, r := range refs {
			if r.SkipReason == "" && r.State != StateGone && watchKey(r.Repo, r.PR) == key {
				p.Catchup.Subscribed(ctx, r)
			}
		}
	}
	p.Catchup.Tick(ctx, refs)
}

func (p *Poller) publishSubscriptionStatus(refs []SessionRef) {
	if p.Status == nil {
		return
	}
	seenRepo := map[string]bool{}
	for _, r := range refs {
		if r.SkipReason != "" || r.PR <= 0 {
			continue
		}
		p.Status.SetSubscribed(r.UUID, p.watched.has(watchKey(r.Repo, r.PR)))
		if !seenRepo[r.Repo] {
			seenRepo[r.Repo] = true
			if st, ok := p.coverage.State(r.Repo); ok {
				p.Status.SetCoverage(r.Repo, st, "")
			}
		}
	}
}

// pollEvents fetches and processes up to MaxPagesPerTick pages. A further
// page is fetched only when the last one was full and something was acked,
// because acked items are what free the slots (D13.4).
func (p *Poller) pollEvents(ctx context.Context) {
	var held []HeldItem
	for page := 0; page < MaxPagesPerTick; page++ {
		items, err := p.Inbox.Events(ctx, p.Settings.ConsumerID)
		if err != nil {
			p.inboxFailed(err)
			return
		}
		p.inboxHealthy()
		if len(items) == 0 {
			break
		}
		results := p.Engine.Process(ctx, items)
		if results == nil {
			break
		}
		acked, pageHeld := p.applyResults(ctx, results)
		held = append(held, pageHeld...)
		if len(items) < EventsPageSize || acked == 0 || ctx.Err() != nil {
			break
		}
	}
	if p.Status != nil {
		p.Status.SetHeld(held)
	}
}

// applyResults records activity, applies the held-age rules to held items and
// returns the number of items acked (including ceiling drops) and the held
// items that remain.
func (p *Poller) applyResults(ctx context.Context, results []DeliverResult) (int, []HeldItem) {
	now := p.Clock.Now()
	acked := 0
	var held []HeldItem
	for _, r := range results {
		if p.Status != nil && r.SessionUUID != "" {
			p.Status.NoteEvent(r.SessionUUID, now)
		}
		switch r.Action {
		case ActionAck:
			if r.AckErr == nil {
				acked++
			}
		case ActionHold:
			if p.ceilingDrop(ctx, r) {
				acked++
				continue
			}
			var kind Kind
			if len(r.Kinds) > 0 {
				kind = r.Kinds[0]
			}
			held = append(held, HeldItem{
				SessionUUID: r.SessionUUID, Repo: r.Repo, PR: r.PR,
				Kind: kind, Reason: r.Hold, Since: r.CreatedAt,
			})
		}
	}
	return acked, held
}

// ceilingDrop applies the D13.1 notice and ceiling to one held item and acks
// it when the ceiling is reached. It reports whether the item was acked.
func (p *Poller) ceilingDrop(ctx context.Context, r DeliverResult) bool {
	if p.HeldAge == nil || r.CreatedAt.IsZero() {
		return false
	}
	verdict := p.HeldAge.Check(HeldEvent{
		SessionUUID: r.SessionUUID, Repo: r.Repo, PR: r.PR, CreatedAt: r.CreatedAt, Reason: r.Hold,
	})
	if verdict != HeldDrop {
		return false
	}
	if err := p.Inbox.Ack(ctx, r.ItemID); err != nil && !errors.Is(err, inbox.ErrNotFound) {
		p.Log.Warn("prwatch.ack_failed", "item", r.ItemID)
		return false
	}
	var kind Kind
	if len(r.Kinds) > 0 {
		kind = r.Kinds[0]
	}
	p.Metrics.Inc(MetricDropped, map[string]string{"drop_reason": string(DropHeldAgeCeiling)})
	if p.Engine.Recorder != nil {
		p.Engine.Recorder.Record(Disposition{
			At: p.Clock.Now(), Kind: kind, Repo: r.Repo, PR: r.PR, SessionUUID: r.SessionUUID,
			Disposition: DispositionDropped, Reason: string(DropHeldAgeCeiling),
		})
	}
	p.Log.Info("prwatch.drop", "reason", string(DropHeldAgeCeiling), "session", r.SessionUUID, "pr", r.PR)
	return true
}

// inboxFailed counts the failure, warns at most once per error class per
// warnInterval, and tells the state tracker and status surface.
func (p *Poller) inboxFailed(err error) {
	if errors.Is(err, context.Canceled) {
		return
	}
	class := inboxErrClass(err)
	p.Metrics.Inc(MetricInboxErrors, map[string]string{"error_class": class})
	now := p.Clock.Now()
	if last, ok := p.lastWarn[class]; !ok || now.Sub(last) >= warnInterval {
		p.lastWarn[class] = now
		p.Log.Warn("prwatch.inbox_error", "error_class", class)
	}
	p.failing = true
	if class == errClassUnauthorized {
		p.backoffUntil = now.Add(unauthorizedBackoff)
	}
	state := StateFromError(err)
	if p.State != nil {
		p.State.Observe(state)
	}
	if p.Status != nil {
		p.Status.SetInbox(state, class)
	}
}

func (p *Poller) inboxHealthy() {
	if p.failing {
		p.failing = false
		p.lastWarn = map[string]time.Time{}
		p.Log.Info("prwatch.inbox_recovered")
	}
	if p.State != nil {
		p.State.Observe(InboxStateOK)
	}
	if p.Status != nil {
		p.Status.SetInbox(InboxStateOK, "")
	}
}

// watchObserver records successful watch and subscribe calls so the loop can
// trigger the first-subscribe catch-up. The reconciler only watches a PR it
// has not yet watched in this process.
type watchObserver struct {
	Inbox
	mu    sync.Mutex
	done  map[string]bool
	fresh []string
}

func (w *watchObserver) note(repo string, pr int) {
	key := watchKey(repo, pr)
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.done[key] {
		w.done[key] = true
		w.fresh = append(w.fresh, key)
	}
}

func (w *watchObserver) Watch(ctx context.Context, consumerID, repo string, pr int) (inbox.WatchResult, error) {
	res, err := w.Inbox.Watch(ctx, consumerID, repo, pr)
	if err == nil {
		w.note(repo, pr)
	}
	return res, err
}

func (w *watchObserver) Subscribe(ctx context.Context, consumerID, repo string, pr int) (inbox.Subscription, error) {
	sub, err := w.Inbox.Subscribe(ctx, consumerID, repo, pr)
	if err == nil {
		w.note(repo, pr)
	}
	return sub, err
}

// take returns the keys first watched since the last call.
func (w *watchObserver) take() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := w.fresh
	w.fresh = nil
	return out
}

func (w *watchObserver) has(key string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.done[key]
}

// tickSessions caches one List result per tick so the loop and the reconciler
// share a single session read.
type tickSessions struct {
	Sessions
	mu     sync.Mutex
	cached []SessionRef
	valid  bool
}

func (s *tickSessions) reset() {
	s.mu.Lock()
	s.valid = false
	s.mu.Unlock()
}

func (s *tickSessions) List(ctx context.Context) ([]SessionRef, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.valid {
		return append([]SessionRef(nil), s.cached...), nil
	}
	refs, err := s.Sessions.List(ctx)
	if err != nil {
		return nil, err
	}
	s.cached, s.valid = refs, true
	return append([]SessionRef(nil), refs...), nil
}
