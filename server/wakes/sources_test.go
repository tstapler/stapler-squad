package wakes

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

type fakeSessions struct {
	mu     sync.Mutex
	states map[string]SessionFacts
}

func (f *fakeSessions) set(id, state string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.states[id] = SessionFacts{Exists: true, State: state}
}

func (f *fakeSessions) SessionState(_ context.Context, id string) (SessionFacts, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.states[id], nil
}

type fakeGuidance struct {
	mu   sync.Mutex
	rows map[string]GuidanceFacts
}

func (f *fakeGuidance) answer(id string, facts GuidanceFacts) {
	f.mu.Lock()
	defer f.mu.Unlock()
	facts.Exists, facts.Answered = true, true
	f.rows[id] = facts
}

func (f *fakeGuidance) Guidance(_ context.Context, id string) (GuidanceFacts, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.rows[id], nil
}

// fakePRs is a PRSource whose snapshots the test pushes by hand.
type fakePRs struct {
	mu        sync.Mutex
	open      map[PRKey]PRView
	lookups   map[PRKey]PRFacts
	lookupErr error
	ch        chan []PRView
	lookupN   int
}

func newFakePRs() *fakePRs {
	return &fakePRs{open: map[PRKey]PRView{}, lookups: map[PRKey]PRFacts{}, ch: make(chan []PRView, 8)}
}

func (f *fakePRs) Subscribe(context.Context) (<-chan []PRView, error) { return f.ch, nil }

func (f *fakePRs) Open(k PRKey) (PRView, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.open[k]
	return v, ok
}

func (f *fakePRs) Lookup(_ context.Context, k PRKey) (PRFacts, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lookupN++
	if f.lookupErr != nil {
		return PRFacts{}, f.lookupErr
	}
	return f.lookups[k], nil
}

// poll replaces the open list and delivers it as one poller snapshot.
func (f *fakePRs) poll(views ...PRView) {
	f.mu.Lock()
	f.open = map[PRKey]PRView{}
	for _, v := range views {
		f.open[v.PRKey] = v
	}
	f.mu.Unlock()
	f.ch <- views
}

func (f *fakePRs) setLookupErr(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lookupErr = err
}

func (f *fakePRs) lookupCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lookupN
}

type sourceRig struct {
	*dispatcherRig
	sessions *fakeSessions
	guidance *fakeGuidance
	prs      *fakePRs
}

func newSourceRig(t *testing.T) *sourceRig {
	t.Helper()
	s := &sourceRig{
		sessions: &fakeSessions{states: map[string]SessionFacts{}},
		guidance: &fakeGuidance{rows: map[string]GuidanceFacts{}},
		prs:      newFakePRs(),
	}
	s.dispatcherRig = newDispatcherRig(t, func(c *Config) {
		c.Sessions, c.Guidance, c.PRs = s.sessions, s.guidance, s.prs
	})
	return s
}

func (s *sourceRig) schedule(spec domain.WakeSpec, payload string) *session.WakeTriggerData {
	s.t.Helper()
	action, err := domain.NewWakeAction(domain.WakeActionSteerText, payload)
	require.NoError(s.t, err)
	row, created, err := s.d.Schedule(context.Background(), ScheduleRequest{Owner: "owner-1", Spec: spec, Action: action})
	require.NoError(s.t, err)
	require.True(s.t, created)
	return row
}

func (s *sourceRig) delivered(id string) func() bool {
	return func() bool { return s.status(id) == domain.WakeStatusDelivered }
}

func TestSchedule_should_RejectBareStatusTransitionAndCreateNoRow_When_NoReconcilePredicate(t *testing.T) {
	t.Parallel()
	r := newSourceRig(t)
	action, err := domain.NewWakeAction(domain.WakeActionSteerText, "x")
	require.NoError(t, err)

	_, _, err = r.d.Schedule(context.Background(), ScheduleRequest{
		Owner: "owner-1", Spec: domain.BacklogEventSpec{ItemID: itemX, Kind: "status_transition"}, Action: action,
	})
	require.ErrorIs(t, err, ErrNoReconcilePredicate)
	live, cerr := r.repo.CountLiveWakeTriggersByOwner(context.Background(), "owner-1")
	require.NoError(t, cerr)
	assert.Zero(t, live)

	row := r.schedule(domain.BacklogEventSpec{ItemID: itemX, Kind: "status_reached:review"}, "x")
	assert.Equal(t, domain.WakeStatusPending, row.Status)
}

func TestBacklogSource_should_FireStatusReached_When_EventCarriesTargetStatus(t *testing.T) {
	t.Parallel()
	r := newSourceRig(t)
	r.start(r.d)
	row := r.schedule(domain.BacklogEventSpec{ItemID: itemX, Kind: "status_reached:review"}, "review is ready")
	r.eventually(func() bool { return len(r.d.index.lookup(domain.WakeTriggerBacklogEvent, itemX)) == 1 })

	other := &events.Event{Type: events.EventBacklogItemChanged, BacklogItemPayload: &events.BacklogItemEventPayload{
		Kind: events.BacklogChangeStatusTransition, NewStatus: "in_progress", Item: &session.BacklogItemData{ID: itemX}}}
	r.bus.Publish(other)
	hit := &events.Event{Type: events.EventBacklogItemChanged, BacklogItemPayload: &events.BacklogItemEventPayload{
		Kind: events.BacklogChangeStatusTransition, NewStatus: "review", Item: &session.BacklogItemData{ID: itemX}}}
	r.bus.Publish(hit)

	r.eventually(r.delivered(row.ID))
	assert.Len(t, r.steerer.pane.written(), 1)
}

func TestBacklogSource_should_FireOnce_When_VerdictBecameTrueWhileDown(t *testing.T) {
	t.Parallel()
	r := newSourceRig(t)
	row := r.schedule(domain.BacklogEventSpec{ItemID: itemX, Kind: "verdict_recorded"}, "verdict")
	r.backlog.items[itemX] = ItemState{Exists: true, HasVerdict: true}

	restarted := r.newDispatcher(func(c *Config) { c.Sessions, c.Guidance, c.PRs = r.sessions, r.guidance, r.prs })
	restarted.reconcileStartup(context.Background())
	restarted.reconcileStartup(context.Background())

	assert.Equal(t, domain.WakeStatusDelivered, r.status(row.ID))
	assert.Len(t, r.steerer.pane.written(), 1)
}

func TestSessionSource_should_FireOnce_When_PeerBecomesIdle(t *testing.T) {
	t.Parallel()
	r := newSourceRig(t)
	r.sessions.set("p1", "active")
	r.start(r.d)
	row := r.schedule(domain.SessionStateSpec{SessionUUID: "p1", State: "idle"}, "p1 is idle")
	r.eventually(func() bool { return len(r.d.index.lookup(domain.WakeTriggerSessionState, "p1")) == 1 })
	assert.Equal(t, domain.WakeStatusPending, r.status(row.ID))

	r.sessions.set("p1", "idle")
	r.bus.Publish(events.NewSessionDeletedEvent("p1")) // any session event nudges a re-read

	r.eventually(r.delivered(row.ID))
	assert.Len(t, r.steerer.pane.written(), 1)
}

func TestSessionSource_should_FireImmediately_When_PeerAlreadyAtTargetAtSchedule(t *testing.T) {
	t.Parallel()
	r := newSourceRig(t)
	r.sessions.set("p1", "idle")
	r.start(r.d)

	row := r.schedule(domain.SessionStateSpec{SessionUUID: "p1", State: "idle"}, "p1 is idle")

	r.eventually(r.delivered(row.ID))
	assert.Len(t, r.steerer.pane.written(), 1)
}

func TestGuidanceSource_should_ClaimOnce_When_AnswerListenerRunsThenRestartReconciles(t *testing.T) {
	t.Parallel()
	r := newSourceRig(t)
	r.start(r.d)
	row := r.schedule(domain.GuidanceAnsweredSpec{GuidanceRequestID: "g1"}, "your question was answered")
	r.eventually(func() bool { return len(r.d.index.lookup(domain.WakeTriggerGuidanceAnswered, "g1")) == 1 })

	r.guidance.answer("g1", GuidanceFacts{Answer: "use approach 2", AnswererKind: "session", AnswererSessionUUID: "peer-b"})
	r.d.OnGuidanceAnswered(context.Background(), "g1")
	r.d.OnGuidanceAnswered(context.Background(), "g1")

	r.eventually(r.delivered(row.ID))
	assert.Len(t, r.steerer.pane.written(), 1)
	text := r.steerer.pane.written()[0]
	assert.Contains(t, text, "answered by session peer-b: use approach 2")
	assert.Equal(t, 1, strings.Count(text, "<wake "))
}

func TestGuidanceSource_should_DeliverViaReconcile_When_AnsweredBeforeDispatcherRan(t *testing.T) {
	t.Parallel()
	r := newSourceRig(t)
	row := r.schedule(domain.GuidanceAnsweredSpec{GuidanceRequestID: "g2"}, "answered?")
	r.guidance.answer("g2", GuidanceFacts{Answer: "yes"})

	r.d.OnGuidanceAnswered(context.Background(), "g2") // not ready: ignored
	assert.Equal(t, domain.WakeStatusPending, r.status(row.ID))
	restarted := r.newDispatcher(func(c *Config) { c.Sessions, c.Guidance, c.PRs = r.sessions, r.guidance, r.prs })
	restarted.reconcileStartup(context.Background())

	assert.Equal(t, domain.WakeStatusDelivered, r.status(row.ID))
	assert.Contains(t, r.steerer.pane.written()[0], "answered by the operator: yes")
}

func TestPRSource_should_FireCIEvent_When_ChecksCompleteBetweenPolls(t *testing.T) {
	t.Parallel()
	r := newSourceRig(t)
	key := PRKey{Repo: "o/r", Number: 901}
	r.prs.open[key] = PRView{PRKey: key, CheckConclusion: "pending"}
	r.start(r.d)
	row := r.schedule(domain.CIEventSpec{Repo: "o/r", Number: 901, Kind: CIKindCompleted}, "checks done")
	r.eventually(func() bool { return len(r.d.index.lookup(domain.WakeTriggerCIEvent, "o/r#901")) == 1 })

	r.prs.poll(PRView{PRKey: key, CheckConclusion: "pending"}) // seeds the diff
	r.prs.poll(PRView{PRKey: key, CheckConclusion: "success"})

	r.eventually(r.delivered(row.ID))
	text := r.steerer.pane.written()[0]
	assert.Contains(t, text, `"pr":901`)
	assert.Contains(t, text, `"conclusion":"success"`)
}

func TestPRSource_should_ConfirmMergeByLookup_And_RetryWhenLookupFails(t *testing.T) {
	t.Parallel()
	r := newSourceRig(t)
	key := PRKey{Repo: "o/r", Number: 7}
	r.prs.open[key] = PRView{PRKey: key, CheckConclusion: "success"}
	r.prs.lookups[key] = PRFacts{State: PRStateMerged}
	r.start(r.d)
	row := r.schedule(domain.PREventSpec{Repo: "o/r", Number: 7, Kind: PRKindMerged}, "merged")
	r.eventually(func() bool { return len(r.d.index.lookup(domain.WakeTriggerPREvent, "o/r#7")) == 1 })

	r.prs.poll(PRView{PRKey: key, CheckConclusion: "success"})
	r.prs.setLookupErr(errors.New("github 502"))
	r.prs.poll() // PR vanished, confirmation fails
	r.eventually(func() bool { return r.prs.lookupCount() >= 1 })
	assert.Equal(t, domain.WakeStatusPending, r.status(row.ID), "a failed lookup keeps the wake pending")

	r.prs.setLookupErr(nil)
	r.prs.poll() // next poll retries the confirmation

	r.eventually(r.delivered(row.ID))
	assert.Contains(t, r.steerer.pane.written()[0], `"state":"merged"`)
}

func TestPRSource_should_NotLookUp_When_VanishedPRHasNoArmedWake(t *testing.T) {
	t.Parallel()
	r := newSourceRig(t)
	key := PRKey{Repo: "o/r", Number: 8}
	r.start(r.d)
	r.prs.poll(PRView{PRKey: key})
	r.prs.poll()
	r.prs.poll() // a third snapshot proves the watcher processed the second
	r.eventually(func() bool { return len(r.prs.ch) == 0 })
	assert.Zero(t, r.prs.lookupCount())
}

func TestSchedule_should_ReturnUnavailable_When_PRSourceCannotServeTheTrigger(t *testing.T) {
	t.Parallel()
	action, err := domain.NewWakeAction(domain.WakeActionSteerText, "x")
	require.NoError(t, err)
	spec := domain.CIEventSpec{Repo: "o/r", Number: 901, Kind: CIKindSuccess}

	noPoller := newDispatcherRig(t)
	_, _, err = noPoller.d.Schedule(context.Background(), ScheduleRequest{Owner: "owner-1", Spec: spec, Action: action})
	assert.ErrorIs(t, err, ErrUnavailable)

	r := newSourceRig(t)
	_, _, err = r.d.Schedule(context.Background(), ScheduleRequest{Owner: "owner-1", Spec: spec, Action: action})
	assert.ErrorIs(t, err, ErrUnavailable, "the poller does not hold PR 901")
	live, cerr := r.repo.CountLiveWakeTriggersByOwner(context.Background(), "owner-1")
	require.NoError(t, cerr)
	assert.Zero(t, live)
}
