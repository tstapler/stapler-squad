package prwatch

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Reconciler keeps the inbox subscriptions for consumer squad:<hostname> in
// line with the retained set of sessions (D9). Every Reconcile call
// recomputes the desired set in memory; the inbox is contacted only when the
// set changed or on the periodic subscription listing. Any failed read
// (session list, subscription list) changes nothing: unsubscribing drops
// pending items, so it never happens on a transient failure. Shutdown does
// not unsubscribe. Not safe for concurrent use; one loop drives it.
type Reconciler struct {
	in        Inbox
	sessions  Sessions
	coverage  *Coverage // optional; nil falls back to plain Subscribe
	coalescer Coalescer // optional
	clock     Clock
	log       Logger
	settings  Settings

	subs        map[string]*trackedSub // by watchKey
	owners      map[string]string      // session UUID -> watchKey, from the last good tick
	firstSeen   map[string]time.Time   // watchKey -> first tick that saw the PR
	closed      map[string]bool        // PRs retired by a closed event, until no session lists them
	listed      bool                   // a subscription listing has succeeded
	lastListing time.Time
}

type trackedSub struct {
	id      string
	watched bool // watch/subscribe succeeded in this process
	adopted bool // found in the inbox's listing, not created here
}

// NewReconciler builds a Reconciler. coverage and coalescer may be nil.
func NewReconciler(in Inbox, sessions Sessions, coverage *Coverage, coalescer Coalescer, clock Clock, log Logger, settings Settings) *Reconciler {
	if settings.ReconcileInterval <= 0 {
		settings.ReconcileInterval = 60 * time.Second
	}
	return &Reconciler{
		in: in, sessions: sessions, coverage: coverage, coalescer: coalescer,
		clock: clock, log: log, settings: settings,
		subs: map[string]*trackedSub{}, owners: map[string]string{}, firstSeen: map[string]time.Time{}, closed: map[string]bool{},
	}
}

func watchKey(repo string, pr int) string { return strings.ToLower(repo) + "#" + strconv.Itoa(pr) }

func splitWatchKey(key string) (string, int) {
	i := strings.LastIndexByte(key, '#')
	n, _ := strconv.Atoi(key[i+1:])
	return key[:i], n
}

// Reconcile runs one pass. It returns an error only when a read failed and
// the pass was skipped; per-subscription failures are logged and retried on
// the next call.
func (r *Reconciler) Reconcile(ctx context.Context) error {
	refs, err := r.sessions.List(ctx)
	if err != nil {
		r.log.Warn("prwatch.reconcile_session_list_failed")
		return err
	}
	desired, owners := retainedSet(refs)
	held := lookupFailedKeys(refs)
	for uuid, key := range lookupFailedOwners(refs) {
		owners[uuid] = key
	}
	r.dropClosed(desired, held)
	now := r.clock.Now()

	if r.listingDue(now) {
		if err := r.adoptListing(ctx, now); err != nil {
			r.log.Warn("prwatch.reconcile_subscription_list_failed")
			return err
		}
	}
	if !r.listed {
		return nil
	}

	for key := range desired {
		if _, ok := r.firstSeen[key]; !ok {
			r.firstSeen[key] = now
		}
	}
	r.subscribeMissing(ctx, desired, now)
	r.unsubscribeUndesired(ctx, desired, held)
	r.evictChangedOwners(owners)
	r.owners = owners
	for key := range r.firstSeen {
		if _, ok := desired[key]; !ok {
			delete(r.firstSeen, key)
		}
	}
	if r.coverage != nil {
		r.coverage.Recheck(ctx)
	}
	return nil
}

func (r *Reconciler) listingDue(now time.Time) bool {
	return !r.listed || now.Sub(r.lastListing) >= r.settings.ReconcileInterval
}

// adoptListing reads the inbox's view of this consumer's subscriptions. It
// records IDs for the keys the inbox already holds (restart rebuild) and
// forgets tracked entries the inbox no longer has, so they are re-subscribed.
func (r *Reconciler) adoptListing(ctx context.Context, now time.Time) error {
	subs, err := r.in.Subscriptions(ctx, r.settings.ConsumerID)
	if err != nil {
		return err
	}
	inboxKeys := map[string]bool{}
	for _, s := range subs {
		if !s.Active || (s.ConsumerID != "" && s.ConsumerID != r.settings.ConsumerID) {
			continue
		}
		key := watchKey(s.Repository, s.PullRequest)
		inboxKeys[key] = true
		if t := r.subs[key]; t != nil {
			t.id = s.ID
		} else {
			r.subs[key] = &trackedSub{id: s.ID, adopted: true}
		}
	}
	for key := range r.subs {
		if !inboxKeys[key] {
			delete(r.subs, key)
		}
	}
	r.listed = true
	r.lastListing = now
	return nil
}

func (r *Reconciler) subscribeMissing(ctx context.Context, desired map[string]string, now time.Time) {
	for _, key := range sortedKeys(desired) {
		t := r.subs[key]
		if t != nil && t.watched {
			continue
		}
		repo, pr := splitWatchKey(desired[key])
		id, err := r.watch(ctx, repo, pr)
		if err != nil {
			r.log.Warn("prwatch.subscribe_failed", "repo", repo, "pr", pr)
			continue
		}
		fresh := t == nil
		if t == nil {
			t = &trackedSub{}
			r.subs[key] = t
		}
		if id != "" {
			t.id = id
		}
		t.watched = true
		if fresh {
			r.logNowWatching(key, repo, pr, now)
		}
	}
}

func (r *Reconciler) watch(ctx context.Context, repo string, pr int) (string, error) {
	if r.coverage != nil {
		sub, _, err := r.coverage.Watch(ctx, repo, pr)
		return sub.ID, err
	}
	sub, err := r.in.Subscribe(ctx, r.settings.ConsumerID, repo, pr)
	return sub.ID, err
}

func (r *Reconciler) logNowWatching(key, repo string, pr int, now time.Time) {
	seen, ok := r.firstSeen[key]
	if !ok {
		seen = now
	}
	r.log.Info(fmt.Sprintf("now watching PR #%d (%s)", pr, repo),
		"discovered_to_subscribed_seconds", now.Sub(seen).Seconds())
}

func (r *Reconciler) unsubscribeUndesired(ctx context.Context, desired map[string]string, held map[string]bool) {
	var drop []string
	for key := range r.subs {
		if _, ok := desired[key]; !ok && !held[key] {
			drop = append(drop, key)
		}
	}
	sort.Strings(drop)
	for _, key := range drop {
		t := r.subs[key]
		if t.id == "" {
			// Learned from the next listing; nothing to delete by yet.
			continue
		}
		if err := r.in.Unsubscribe(ctx, t.id); err != nil {
			r.log.Warn("prwatch.unsubscribe_failed")
			continue
		}
		delete(r.subs, key)
		r.log.Info("prwatch.unsubscribed", "repo", key[:strings.LastIndexByte(key, '#')])
	}
}

// evictChangedOwners drops coalescer state for sessions that left the
// retained set or moved to a different PR, including when the subscription
// stays because another session shares the PR.
func (r *Reconciler) evictChangedOwners(owners map[string]string) {
	if r.coalescer == nil {
		return
	}
	for uuid, prev := range r.owners {
		if cur, ok := owners[uuid]; !ok || cur != prev {
			r.coalescer.Evict(uuid)
		}
	}
}

// retainedSet returns the wanted subscriptions, keyed by watchKey (value is
// the key with the session's original repo casing for the wire call), and
// the session -> key map. Skipped and gone sessions are not retained.
func retainedSet(refs []SessionRef) (desired, owners map[string]string) {
	desired = map[string]string{}
	owners = map[string]string{}
	for _, s := range refs {
		if s.SkipReason != "" || s.State == StateGone || s.Repo == "" || s.PR <= 0 {
			continue
		}
		key := watchKey(s.Repo, s.PR)
		if _, ok := desired[key]; !ok {
			desired[key] = s.Repo + "#" + strconv.Itoa(s.PR)
		}
		owners[s.UUID] = key
	}
	return desired, owners
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// lookupFailedKeys returns the PRs of sessions skipped only because the
// backlog lookup failed. Their existing subscriptions are kept (a transient
// failure must not discard pending items); new ones are not made.
func lookupFailedKeys(refs []SessionRef) map[string]bool {
	keys := map[string]bool{}
	for _, key := range lookupFailedOwners(refs) {
		keys[key] = true
	}
	return keys
}

func lookupFailedOwners(refs []SessionRef) map[string]string {
	owners := map[string]string{}
	for _, s := range refs {
		if s.SkipReason == SkipBacklogLookupFailed && s.State != StateGone && s.Repo != "" && s.PR > 0 {
			owners[s.UUID] = watchKey(s.Repo, s.PR)
		}
	}
	return owners
}

// dropClosed removes PRs retired by a closed event from desired, and forgets
// the retirement once no session lists the PR any more.
func (r *Reconciler) dropClosed(desired map[string]string, held map[string]bool) {
	for key := range r.closed {
		_, listed := desired[key]
		if !listed && !held[key] {
			delete(r.closed, key)
			continue
		}
		delete(desired, key)
	}
}

// RetirePR handles a pull_request closed event (D7): it unsubscribes the PR
// and keeps it out of the desired set until no session lists it. A failed
// unsubscribe is retried by the next Reconcile.
func (r *Reconciler) RetirePR(ctx context.Context, repo string, pr int) {
	key := watchKey(repo, pr)
	r.closed[key] = true
	if t := r.subs[key]; t != nil && t.id != "" {
		if err := r.in.Unsubscribe(ctx, t.id); err != nil {
			r.log.Warn("prwatch.unsubscribe_failed")
			return
		}
		delete(r.subs, key)
		r.log.Info("prwatch.unsubscribed", "repo", repo)
	}
}
