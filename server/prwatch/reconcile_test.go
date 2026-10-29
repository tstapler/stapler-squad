package prwatch

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/server/prwatch/inbox"
)

const testConsumer = "squad:testhost"

// upsertInbox is a fakeInbox whose Watch registers a subscription the way the
// real inbox does (idempotent per consumer/repo/PR).
type upsertInbox struct{ *fakeInbox }

func (u upsertInbox) Watch(ctx context.Context, consumerID, repo string, pr int) (inbox.WatchResult, error) {
	if err := u.record("watch:" + consumerID + ":" + repo + "#" + itoa(pr)); err != nil {
		return inbox.WatchResult{}, err
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	for _, s := range u.subs {
		if s.ConsumerID == consumerID && s.Repository == repo && s.PullRequest == pr {
			return inbox.WatchResult{Coverage: inbox.CoverageExisting, Subscription: s}, nil
		}
	}
	sub := inbox.Subscription{ID: "sub-" + repo + "#" + itoa(pr), ConsumerID: consumerID, Repository: repo, PullRequest: pr, Active: true}
	u.subs = append(u.subs, sub)
	return inbox.WatchResult{Coverage: inbox.CoverageExisting, Subscription: sub}, nil
}

type reconcileEnv struct {
	in        upsertInbox
	sessions  *fakeSessions
	coalescer *fakeCoalescer
	clock     *fakeClock
	log       *fakeLog
	rec       *Reconciler
}

func newReconcileEnv(t *testing.T) *reconcileEnv {
	t.Helper()
	e := &reconcileEnv{
		in:        upsertInbox{&fakeInbox{errs: map[string]error{}}},
		sessions:  &fakeSessions{},
		coalescer: &fakeCoalescer{},
		clock:     newFakeClock(),
		log:       &fakeLog{},
	}
	e.rec = e.newReconciler()
	return e
}

func (e *reconcileEnv) newReconciler() *Reconciler {
	s := Settings{ConsumerID: testConsumer, ReconcileInterval: 60 * time.Second}
	cov := NewCoverage(e.in, &fakeNotifier{}, e.clock, e.log, s)
	return NewReconciler(e.in, e.sessions, cov, e.coalescer, e.clock, e.log, s)
}

func (e *reconcileEnv) callsWithPrefix(p string) []string {
	var out []string
	for _, c := range e.in.Calls() {
		if strings.HasPrefix(c, p) {
			out = append(out, c)
		}
	}
	return out
}

func liveSession(uuid, repo string, pr int) SessionRef {
	return SessionRef{UUID: uuid, Repo: repo, PR: pr, State: StateLive}
}

func TestReconcile_should_SubscribeWithHostConsumerID_When_NewSessionWithPRAppears(t *testing.T) {
	e := newReconcileEnv(t)
	e.sessions.Set(liveSession("a", "acme/widget", 7), liveSession("b", "acme/gadget", 9))
	require.NoError(t, e.rec.Reconcile(context.Background()))
	require.ElementsMatch(t, []string{
		"watch:" + testConsumer + ":acme/widget#7",
		"watch:" + testConsumer + ":acme/gadget#9",
	}, e.callsWithPrefix("watch:"))

	// Unchanged set: no further watch calls between listings.
	e.clock.Advance(5 * time.Second)
	require.NoError(t, e.rec.Reconcile(context.Background()))
	require.Len(t, e.callsWithPrefix("watch:"), 2)
	require.Len(t, e.callsWithPrefix("subscriptions:"), 1)
}

func TestReconcile_should_SubscribeLater_When_PRNumberAppearsAfterSessionStart(t *testing.T) {
	e := newReconcileEnv(t)
	e.sessions.Set(SessionRef{UUID: "a", State: StateLive, SkipReason: SkipNoPR})
	require.NoError(t, e.rec.Reconcile(context.Background()))
	require.Empty(t, e.callsWithPrefix("watch:"))

	e.clock.Advance(5 * time.Second)
	e.sessions.Set(liveSession("a", "acme/widget", 7))
	require.NoError(t, e.rec.Reconcile(context.Background()))
	require.Equal(t, []string{"watch:" + testConsumer + ":acme/widget#7"}, e.callsWithPrefix("watch:"))
}

func TestReconcile_should_UnsubscribeUsingSubscriptionID_When_SessionStopsOrIsArchived(t *testing.T) {
	e := newReconcileEnv(t)
	e.sessions.Set(liveSession("a", "acme/widget", 7), liveSession("b", "acme/widget", 7), liveSession("c", "acme/gadget", 9))
	require.NoError(t, e.rec.Reconcile(context.Background()))

	// One of two sessions on the shared PR leaves: subscription stays.
	e.sessions.Set(liveSession("b", "acme/widget", 7), liveSession("c", "acme/gadget", 9))
	require.NoError(t, e.rec.Reconcile(context.Background()))
	require.Empty(t, e.callsWithPrefix("unsubscribe:"))
	require.Equal(t, []string{"a"}, e.coalescer.evicted)

	// The rest leave: both subscriptions are removed by ID.
	e.sessions.Set()
	require.NoError(t, e.rec.Reconcile(context.Background()))
	require.ElementsMatch(t, []string{"unsubscribe:sub-acme/widget#7", "unsubscribe:sub-acme/gadget#9"}, e.callsWithPrefix("unsubscribe:"))
	require.ElementsMatch(t, []string{"a", "b", "c"}, e.coalescer.evicted)
}

func TestReconcile_should_Unsubscribe_When_PRMergedOrClosed(t *testing.T) {
	e := newReconcileEnv(t)
	e.sessions.Set(liveSession("a", "acme/widget", 7))
	require.NoError(t, e.rec.Reconcile(context.Background()))

	// The Sessions adapter reports a terminal PR as not watched.
	e.sessions.Set(SessionRef{UUID: "a", Repo: "acme/widget", PR: 7, State: StateLive, SkipReason: SkipNotLive})
	require.NoError(t, e.rec.Reconcile(context.Background()))
	require.Equal(t, []string{"unsubscribe:sub-acme/widget#7"}, e.callsWithPrefix("unsubscribe:"))
	require.Equal(t, []string{"a"}, e.coalescer.evicted)
}

func TestReconcile_should_RebuildSubscriptionsFromLiveSessions_When_ProcessRestarts(t *testing.T) {
	e := newReconcileEnv(t)
	e.sessions.Set(liveSession("a", "acme/widget", 7))
	require.NoError(t, e.rec.Reconcile(context.Background()))

	// New process: empty in-memory state, inbox still holds the subscription.
	restarted := e.newReconciler()
	require.NoError(t, restarted.Reconcile(context.Background()))
	require.Len(t, e.in.subs, 1, "no duplicate subscription")
	require.Empty(t, e.callsWithPrefix("unsubscribe:"), "nothing orphaned")
	require.Len(t, e.callsWithPrefix("subscriptions:"), 2)
	require.Len(t, e.callsWithPrefix("watch:"), 2, "idempotent re-watch")
	for _, l := range e.log.Lines() {
		if strings.HasPrefix(l.Msg, "now watching") {
			require.Equal(t, "now watching PR #7 (acme/widget)", l.Msg)
		}
	}
	count := 0
	for _, l := range e.log.Lines() {
		if strings.HasPrefix(l.Msg, "now watching") {
			count++
		}
	}
	require.Equal(t, 1, count, "restart re-adoption is not a new 'now watching'")
}

func TestReconcile_should_UnsubscribeOrphans_When_SessionEndedWhileSquadWasDown(t *testing.T) {
	e := newReconcileEnv(t)
	e.in.subs = []inbox.Subscription{
		{ID: "orphan-1", ConsumerID: testConsumer, Repository: "acme/old", PullRequest: 3, Active: true},
		{ID: "other-consumer", ConsumerID: "squad:elsewhere", Repository: "acme/x", PullRequest: 1, Active: true},
	}
	e.sessions.Set(liveSession("a", "acme/widget", 7))
	require.NoError(t, e.rec.Reconcile(context.Background()))
	require.Equal(t, []string{"unsubscribe:orphan-1"}, e.callsWithPrefix("unsubscribe:"))
}

func TestReconcile_should_KeepSubscription_When_SessionPausedOrHibernatedAndNotArchived(t *testing.T) {
	e := newReconcileEnv(t)
	e.sessions.Set(liveSession("a", "acme/widget", 7))
	require.NoError(t, e.rec.Reconcile(context.Background()))

	e.sessions.Set(SessionRef{UUID: "a", Repo: "acme/widget", PR: 7, State: StateSuspended})
	for i := 0; i < 3; i++ {
		e.clock.Advance(61 * time.Second) // forces a listing diff each pass
		require.NoError(t, e.rec.Reconcile(context.Background()))
	}
	require.Empty(t, e.callsWithPrefix("unsubscribe:"))
	require.Empty(t, e.coalescer.evicted)

	e.sessions.Set(SessionRef{UUID: "a", Repo: "acme/widget", PR: 7, State: StateGone, SkipReason: SkipNotLive})
	require.NoError(t, e.rec.Reconcile(context.Background()))
	require.Len(t, e.callsWithPrefix("unsubscribe:"), 1)
}

func TestReconcile_should_UnsubscribeRepo_When_AddedToIgnoreReposAfterSubscribe(t *testing.T) {
	e := newReconcileEnv(t)
	e.sessions.Set(liveSession("a", "acme/widget", 7))
	require.NoError(t, e.rec.Reconcile(context.Background()))

	// ComputeWatchSet marks the ignored repo's session SkipIgnoreRepos.
	e.sessions.Set(SessionRef{UUID: "a", Repo: "acme/widget", PR: 7, State: StateLive, SkipReason: SkipIgnoreRepos})
	require.NoError(t, e.rec.Reconcile(context.Background()))
	require.Equal(t, []string{"unsubscribe:sub-acme/widget#7"}, e.callsWithPrefix("unsubscribe:"))
}

func TestReconcile_should_ResubscribeWithNewRepository_When_SessionPRChanges(t *testing.T) {
	e := newReconcileEnv(t)
	e.sessions.Set(liveSession("a", "acme/widget", 7))
	require.NoError(t, e.rec.Reconcile(context.Background()))

	e.sessions.Set(liveSession("a", "acme/gadget", 9))
	require.NoError(t, e.rec.Reconcile(context.Background()))
	require.Equal(t, []string{"unsubscribe:sub-acme/widget#7"}, e.callsWithPrefix("unsubscribe:"))
	require.Contains(t, e.callsWithPrefix("watch:"), "watch:"+testConsumer+":acme/gadget#9")
	require.Equal(t, []string{"a"}, e.coalescer.evicted)
}

func TestReconcile_should_RetryNextTick_When_SubscribeFails(t *testing.T) {
	e := newReconcileEnv(t)
	e.in.errs["watch"] = errors.New("500")
	e.sessions.Set(liveSession("a", "acme/widget", 7))
	require.NoError(t, e.rec.Reconcile(context.Background()))
	require.Len(t, e.callsWithPrefix("watch:"), 1)

	delete(e.in.errs, "watch")
	e.clock.Advance(5 * time.Second)
	require.NoError(t, e.rec.Reconcile(context.Background()))
	require.Len(t, e.callsWithPrefix("watch:"), 2)
	require.Len(t, e.in.subs, 1)
}

func TestReconcile_should_ChangeNothing_When_SessionListErrors(t *testing.T) {
	e := newReconcileEnv(t)
	e.sessions.Set(liveSession("a", "acme/widget", 7))
	require.NoError(t, e.rec.Reconcile(context.Background()))
	before := len(e.in.Calls())

	e.sessions.listErr = errors.New("db down")
	e.clock.Advance(2 * time.Minute)
	require.Error(t, e.rec.Reconcile(context.Background()))
	require.Len(t, e.in.Calls(), before, "no inbox call of any kind")
	require.Empty(t, e.coalescer.evicted)
}

func TestReconcile_should_ChangeNothing_When_SubscriptionListingErrors(t *testing.T) {
	e := newReconcileEnv(t)
	e.in.subs = []inbox.Subscription{{ID: "s1", ConsumerID: testConsumer, Repository: "acme/old", PullRequest: 3, Active: true}}
	e.in.errs["subscriptions"] = errors.New("503")
	e.sessions.Set(liveSession("a", "acme/widget", 7))
	require.Error(t, e.rec.Reconcile(context.Background()))
	require.Empty(t, e.callsWithPrefix("watch:"))
	require.Empty(t, e.callsWithPrefix("unsubscribe:"))

	delete(e.in.errs, "subscriptions")
	require.NoError(t, e.rec.Reconcile(context.Background()))
	require.Equal(t, []string{"unsubscribe:s1"}, e.callsWithPrefix("unsubscribe:"))
}

func TestReconcile_should_ListSubscriptionsOnReconcileIntervalOnly(t *testing.T) {
	e := newReconcileEnv(t)
	e.sessions.Set(liveSession("a", "acme/widget", 7))
	for i := 0; i < 5; i++ {
		require.NoError(t, e.rec.Reconcile(context.Background()))
		e.clock.Advance(10 * time.Second)
	}
	require.Len(t, e.callsWithPrefix("subscriptions:"), 1)
	e.clock.Advance(30 * time.Second)
	require.NoError(t, e.rec.Reconcile(context.Background()))
	require.Len(t, e.callsWithPrefix("subscriptions:"), 2)
}

func TestReconcile_should_RetryUnsubscribeNextTick_When_UnsubscribeFails(t *testing.T) {
	e := newReconcileEnv(t)
	e.sessions.Set(liveSession("a", "acme/widget", 7))
	require.NoError(t, e.rec.Reconcile(context.Background()))
	e.in.errs["unsubscribe"] = errors.New("500")
	e.sessions.Set()
	require.NoError(t, e.rec.Reconcile(context.Background()))
	require.Len(t, e.in.subs, 1)

	delete(e.in.errs, "unsubscribe")
	require.NoError(t, e.rec.Reconcile(context.Background()))
	require.Empty(t, e.in.subs)
}

func TestReconcile_should_LogNowWatchingAndDiscoveredToSubscribedLatency_When_Subscribing(t *testing.T) {
	e := newReconcileEnv(t)
	e.in.errs["watch"] = errors.New("500")
	e.sessions.Set(liveSession("a", "acme-corp/widget-api", 123))
	require.NoError(t, e.rec.Reconcile(context.Background()))
	delete(e.in.errs, "watch")
	e.clock.Advance(7 * time.Second)
	require.NoError(t, e.rec.Reconcile(context.Background()))

	var found *LogLine
	for _, l := range e.log.Lines() {
		if l.Level == "info" && l.Msg == "now watching PR #123 (acme-corp/widget-api)" {
			l := l
			found = &l
		}
	}
	require.NotNil(t, found)
	require.Equal(t, []any{"discovered_to_subscribed_seconds", 7.0}, found.KV)
}
