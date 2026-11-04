package prwatch

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/server/prwatch/inbox"
)

// Compile-time port checks.
var (
	_ Inbox             = (*fakeInbox)(nil)
	_ Sessions          = (*fakeSessions)(nil)
	_ Steerer           = (*fakeSteerer)(nil)
	_ Notifier          = (*fakeNotifier)(nil)
	_ Clock             = (*fakeClock)(nil)
	_ Logger            = (*fakeLog)(nil)
	_ Metrics           = (*fakeMetrics)(nil)
	_ Recorder          = (*fakeRecorder)(nil)
	_ LeaderGuard       = (*fakeLeader)(nil)
	_ PRSnapshotSource  = (*fakePRSnapshot)(nil)
	_ SelfLogin         = (*fakeSelfLogin)(nil)
	_ Coalescer         = (*fakeCoalescer)(nil)
	_ StrangerWatch     = (*fakeStrangerWatch)(nil)
	_ InboxStateTracker = (*fakeInboxStateTracker)(nil)
	_ FollowupObserver  = (*fakeFollowup)(nil)
)

func TestFakes_should_SatisfyPorts_When_Compiled(t *testing.T) {
	var c Clock = newFakeClock()
	start := c.Now()
	c.(*fakeClock).Advance(time.Minute)
	if got := c.Now().Sub(start); got != time.Minute {
		t.Fatalf("clock advanced %v, want 1m", got)
	}
}

// fakeInbox is an in-memory Inbox. Errors are scripted per method; calls are
// recorded in order as "method:arg".
type fakeInbox struct {
	mu          sync.Mutex
	calls       []string
	items       []inbox.Item
	subs        []inbox.Subscription
	watchResult inbox.WatchResult
	health      inbox.Health
	errs        map[string]error // method name -> error returned
}

func (f *fakeInbox) record(call string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, call)
	return f.errs[call[:indexColon(call)]]
}

func indexColon(s string) int {
	for i := range s {
		if s[i] == ':' {
			return i
		}
	}
	return len(s)
}

func (f *fakeInbox) Calls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

func (f *fakeInbox) Watch(_ context.Context, consumerID, repo string, pr int) (inbox.WatchResult, error) {
	if err := f.record("watch:" + consumerID + ":" + repo + "#" + itoa(pr)); err != nil {
		return inbox.WatchResult{}, err
	}
	return f.watchResult, nil
}

func (f *fakeInbox) Subscribe(_ context.Context, consumerID, repo string, pr int) (inbox.Subscription, error) {
	if err := f.record("subscribe:" + consumerID + ":" + repo + "#" + itoa(pr)); err != nil {
		return inbox.Subscription{}, err
	}
	sub := inbox.Subscription{ID: "sub-" + repo + "#" + itoa(pr), ConsumerID: consumerID, Repository: repo, PullRequest: pr, Active: true}
	f.mu.Lock()
	f.subs = append(f.subs, sub)
	f.mu.Unlock()
	return sub, nil
}

func (f *fakeInbox) Subscriptions(_ context.Context, consumerID string) ([]inbox.Subscription, error) {
	if err := f.record("subscriptions:" + consumerID); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]inbox.Subscription(nil), f.subs...), nil
}

func (f *fakeInbox) Events(_ context.Context, consumerID string) ([]inbox.Item, error) {
	if err := f.record("events:" + consumerID); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]inbox.Item(nil), f.items...), nil
}

func (f *fakeInbox) Ack(_ context.Context, itemID string) error {
	if err := f.record("ack:" + itemID); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	kept := f.items[:0:0]
	for _, it := range f.items {
		if it.ID != itemID {
			kept = append(kept, it)
		}
	}
	f.items = kept
	return nil
}

func (f *fakeInbox) Unsubscribe(_ context.Context, subscriptionID string) error {
	if err := f.record("unsubscribe:" + subscriptionID); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	kept := f.subs[:0:0]
	for _, s := range f.subs {
		if s.ID != subscriptionID {
			kept = append(kept, s)
		}
	}
	f.subs = kept
	return nil
}

func (f *fakeInbox) Health(context.Context) (inbox.Health, error) {
	if err := f.record("health:"); err != nil {
		return inbox.Health{}, err
	}
	return f.health, nil
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b []byte
	for ; n > 0; n /= 10 {
		b = append([]byte{byte('0' + n%10)}, b...)
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}

// fakeSessions is a mutable session list.
type fakeSessions struct {
	mu       sync.Mutex
	sessions []SessionRef
	programs map[string]string
	hidden   map[string]bool
	busy     map[string]bool
	listErr  error
	listN    int
}

func (f *fakeSessions) Set(sessions ...SessionRef) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sessions = append([]SessionRef(nil), sessions...)
}

func (f *fakeSessions) ListCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.listN
}

func (f *fakeSessions) List(context.Context) ([]SessionRef, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.listN++
	if f.listErr != nil {
		return nil, f.listErr
	}
	return append([]SessionRef(nil), f.sessions...), nil
}

func (f *fakeSessions) Program(uuid string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.programs[uuid]
}

func (f *fakeSessions) IsHidden(uuid string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.hidden[uuid]
}

func (f *fakeSessions) IsBusy(uuid string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.busy[uuid]
}

// fakeSteerer records requests and returns a scripted outcome per session
// (default OutcomeDelivered).
type fakeSteerer struct {
	mu       sync.Mutex
	requests []SteerRequest
	outcome  map[string]Outcome
	err      error
}

func (f *fakeSteerer) Steer(_ context.Context, req SteerRequest) (Outcome, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, req)
	if f.err != nil {
		return OutcomeFailed, f.err
	}
	if o, ok := f.outcome[req.SessionUUID]; ok {
		return o, nil
	}
	return OutcomeDelivered, nil
}

func (f *fakeSteerer) Requests() []SteerRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]SteerRequest(nil), f.requests...)
}

type fakeNotifier struct {
	mu   sync.Mutex
	sent []Notification
	err  error
}

func (f *fakeNotifier) Notify(n Notification) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, n)
	return f.err
}

func (f *fakeNotifier) Sent() []Notification {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Notification(nil), f.sent...)
}

// fakeClock is a manually advanced clock.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newFakeClock() *fakeClock {
	return &fakeClock{now: time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// LogLine is one captured log call.
type LogLine struct {
	Level string
	Msg   string
	KV    []any
}

type fakeLog struct {
	mu    sync.Mutex
	lines []LogLine
}

func (l *fakeLog) add(level, msg string, kv []any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lines = append(l.lines, LogLine{Level: level, Msg: msg, KV: kv})
}

func (l *fakeLog) Debug(msg string, kv ...any) { l.add("debug", msg, kv) }
func (l *fakeLog) Info(msg string, kv ...any)  { l.add("info", msg, kv) }
func (l *fakeLog) Warn(msg string, kv ...any)  { l.add("warn", msg, kv) }

func (l *fakeLog) Lines() []LogLine {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]LogLine(nil), l.lines...)
}

// MetricCall is one recorded Inc.
type MetricCall struct {
	Name   string
	Labels map[string]string
}

type fakeMetrics struct {
	mu    sync.Mutex
	calls []MetricCall
}

func (m *fakeMetrics) Inc(name string, labels map[string]string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make(map[string]string, len(labels))
	for k, v := range labels {
		cp[k] = v
	}
	m.calls = append(m.calls, MetricCall{Name: name, Labels: cp})
}

// Count returns how many Inc calls used name.
func (m *fakeMetrics) Count(name string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, c := range m.calls {
		if c.Name == name {
			n++
		}
	}
	return n
}

type fakeRecorder struct {
	mu      sync.Mutex
	records []Disposition
}

func (r *fakeRecorder) Record(d Disposition) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.records = append(r.records, d)
}

func (r *fakeRecorder) Records() []Disposition {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]Disposition(nil), r.records...)
}

// fakeLeader grants or refuses leadership and counts releases.
type fakeLeader struct {
	mu       sync.Mutex
	grant    bool
	err      error
	attempts int
	released int
}

func (l *fakeLeader) TryAcquire() (func(), bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.attempts++
	if l.err != nil || !l.grant {
		return func() {}, false, l.err
	}
	return func() {
		l.mu.Lock()
		defer l.mu.Unlock()
		l.released++
	}, true, nil
}

type fakePRSnapshot struct {
	mu        sync.Mutex
	snapshots map[string]PRSnapshot
	calls     []string
}

func (f *fakePRSnapshot) Snapshot(uuid string) (PRSnapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, uuid)
	s, ok := f.snapshots[uuid]
	if !ok {
		return PRSnapshot{}, errUnknownSession
	}
	return s, nil
}

type fakeSelfLogin struct {
	mu    sync.Mutex
	login string
	err   error
	calls int
}

func (f *fakeSelfLogin) Login(context.Context) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return f.login, f.err
}

// fakeCoalescer returns scripted actions (default CoalesceSteer) and records
// every call. NextEligible reads from the next map.
type fakeCoalescer struct {
	mu      sync.Mutex
	actions map[CoalesceKey]CoalesceAction
	next    map[CoalesceKey]time.Time
	decides []CoalesceKey
	steers  []CoalesceKey
	evicted []string
}

func (f *fakeCoalescer) Decide(key CoalesceKey, _ string, _ int) CoalesceAction {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.decides = append(f.decides, key)
	if a, ok := f.actions[key]; ok {
		return a
	}
	return CoalesceSteer
}

func (f *fakeCoalescer) RecordSteer(key CoalesceKey, _ string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.steers = append(f.steers, key)
}

func (f *fakeCoalescer) Evict(sessionUUID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.evicted = append(f.evicted, sessionUUID)
}

func (f *fakeCoalescer) NextEligible(key CoalesceKey) (time.Time, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	at, ok := f.next[key]
	return at, ok
}

type fakeStrangerWatch struct {
	mu     sync.Mutex
	counts map[string]int
	steers []ActorClass
}

func (f *fakeStrangerWatch) RecordSteer(repo string, _ int, class ActorClass) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.counts == nil {
		f.counts = map[string]int{}
	}
	f.steers = append(f.steers, class)
	if class == ActorOther {
		f.counts[repo]++
	}
}

func (f *fakeStrangerWatch) OtherCount(repo string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.counts[repo]
}

type fakeInboxStateTracker struct {
	mu       sync.Mutex
	observed []InboxState
}

func (f *fakeInboxStateTracker) Observe(s InboxState) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.observed = append(f.observed, s)
}

func (f *fakeInboxStateTracker) Current() InboxState {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.observed) == 0 {
		return InboxStateOK
	}
	return f.observed[len(f.observed)-1]
}

type fakeFollowup struct {
	mu      sync.Mutex
	tracked []string
	ticks   int
}

func (f *fakeFollowup) Track(uuid, repo string, pr int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tracked = append(f.tracked, uuid+":"+repo+"#"+itoa(pr))
}

func (f *fakeFollowup) Tick() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ticks++
}

var errUnknownSession = errors.New("unknown session")
