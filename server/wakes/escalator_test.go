package wakes

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/session/domain"
)

// fakePeerRequests is a durable-store stand-in shared across escalator "restarts".
type fakePeerRequests struct {
	mu        sync.Mutex
	open      map[string]PeerRequest
	escalated map[string]bool
}

func newFakePeerRequests(reqs ...PeerRequest) *fakePeerRequests {
	f := &fakePeerRequests{open: map[string]PeerRequest{}, escalated: map[string]bool{}}
	for _, r := range reqs {
		f.open[r.ID] = r
	}
	return f
}

func (f *fakePeerRequests) OpenPeerRequests(context.Context) ([]PeerRequest, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]PeerRequest, 0, len(f.open))
	for _, r := range f.open {
		out = append(out, r)
	}
	return out, nil
}

func (f *fakePeerRequests) MarkEscalated(_ context.Context, id string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.escalated[id] {
		return false, nil
	}
	f.escalated[id] = true
	return true, nil
}

func (f *fakePeerRequests) answer(id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.open, id)
}

type escalatorRig struct {
	store    *fakePeerRequests
	owners   *fakeOwners
	notifier *recordingNotifier
	clock    *FakeClock
	bus      *events.EventBus
	esc      *Escalator
}

func newEscalatorRig(t *testing.T, store *fakePeerRequests, clock *FakeClock) *escalatorRig {
	t.Helper()
	r := &escalatorRig{store: store, owners: newFakeOwners(), notifier: &recordingNotifier{}, clock: clock, bus: events.NewEventBus(16)}
	r.esc = NewEscalator(EscalatorConfig{Requests: store, Owners: r.owners, Notifier: r.notifier, Events: r.bus, Clock: clock})
	return r
}

func (r *escalatorRig) start(t *testing.T) {
	t.Helper()
	require.NoError(t, r.esc.Start(context.Background()))
	t.Cleanup(func() { _ = r.esc.Stop(context.Background()) })
}

func peerReq(created time.Time) PeerRequest {
	return PeerRequest{ID: "req-1", Asker: "asker", Answerer: "peer", Question: "Which approach?", CreatedAt: created, AskerTitle: "review:pr-1", AnswererTitle: "diagnose:abc"}
}

func (r *escalatorRig) waitNotifications(t *testing.T, n int) []Notification {
	t.Helper()
	require.Eventually(t, func() bool { return len(r.notifier.all()) >= n }, 2*time.Second, 5*time.Millisecond)
	return r.notifier.all()
}

func TestEscalator_should_raiseExactlyOneStalledNotificationWithRequestId_When_10MinutesWithoutAnswerOnFakeClock(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())
	clock := NewFakeClock(t0)
	rig := newEscalatorRig(t, newFakePeerRequests(peerReq(t0)), clock)
	rig.owners.set("peer", OwnerFacts{Exists: true, Live: true, WorkspaceKey: "repo"})
	rig.owners.set("asker", OwnerFacts{Exists: true, Live: true, WorkspaceKey: "repo"})
	rig.start(t)

	clock.BlockUntilTimerAt(t0.Add(PeerEscalationTimeout))
	assert.Empty(t, rig.notifier.all(), "nothing before the deadline")
	clock.Advance(PeerEscalationTimeout)

	got := rig.waitNotifications(t, 1)
	require.Len(t, got, 1)
	assert.Equal(t, NotifyPeerStalled, got[0].Kind)
	assert.Equal(t, "req-1", got[0].GuidanceRequestID)
	assert.Equal(t, "asker", got[0].OwnerSessionUUID)
	assert.Contains(t, got[0].Message, "review:pr-1 asked diagnose:abc: no reply in 10m")
	assert.EqualValues(t, 1, rig.esc.cfg.Metrics.Value(MetricPeerHelp, "escalated"))

	rig.esc.Rearm()
	clock.Advance(time.Hour)
	assert.Len(t, rig.notifier.all(), 1, "a second pass never duplicates")
	require.NoError(t, rig.esc.Stop(context.Background()))
}

func TestEscalator_should_notDuplicate_When_RestartedAfterEscalating(t *testing.T) {
	clock := NewFakeClock(t0)
	store := newFakePeerRequests(peerReq(t0))
	first := newEscalatorRig(t, store, clock)
	first.start(t)
	clock.BlockUntilTimers(1)
	clock.Advance(PeerEscalationTimeout)
	first.waitNotifications(t, 1)
	require.NoError(t, first.esc.Stop(context.Background()))

	second := newEscalatorRig(t, store, clock)
	second.start(t)
	require.Eventually(t, func() bool { _, ok := second.esc.Pass(context.Background()); return !ok }, time.Second, 5*time.Millisecond)
	assert.Empty(t, second.notifier.all(), "the durable marker stops the restart's startup pass")
}

func TestEscalator_should_escalateOverdueRequestsOnStartupPass_When_ProcessWasDown(t *testing.T) {
	clock := NewFakeClock(t0.Add(time.Hour))
	rig := newEscalatorRig(t, newFakePeerRequests(peerReq(t0)), clock)
	rig.start(t)
	assert.Len(t, rig.waitNotifications(t, 1), 1)
}

func TestEscalator_should_escalateAtOnceWithPlainReason_When_AnswererStoppedArchivedOrDeleted(t *testing.T) {
	cases := map[string]struct {
		facts  OwnerFacts
		reason string
	}{
		"stopped":  {OwnerFacts{Exists: true, Live: false}, "Peer is stopped. Not delivered unless resumed."},
		"archived": {OwnerFacts{Exists: true, Archived: true}, "peer is archived"},
		"deleted":  {OwnerFacts{}, "peer no longer exists"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			clock := NewFakeClock(t0)
			rig := newEscalatorRig(t, newFakePeerRequests(peerReq(t0)), clock)
			rig.start(t)
			clock.BlockUntilTimers(1)
			assert.Empty(t, rig.notifier.all())

			rig.owners.set("peer", c.facts)
			rig.bus.Publish(events.NewSessionDeletedEvent("peer"))
			got := rig.waitNotifications(t, 1)
			assert.Contains(t, got[0].Message, c.reason)
			require.Eventually(t, func() bool { return clock.ActiveTimers() == 0 }, time.Second, 5*time.Millisecond,
				"the deadline is dropped with the request")
		})
	}
}

func TestEscalator_should_notEscalateAndDropDeadline_When_PeerAnswersAtMinute4(t *testing.T) {
	clock := NewFakeClock(t0)
	store := newFakePeerRequests(peerReq(t0))
	rig := newEscalatorRig(t, store, clock)
	rig.start(t)
	clock.BlockUntilTimers(1)

	clock.Advance(4 * time.Minute)
	store.answer("req-1")
	rig.esc.Rearm()
	require.Eventually(t, func() bool { return clock.ActiveTimers() == 0 }, time.Second, 5*time.Millisecond)
	clock.Advance(time.Hour)
	assert.Empty(t, rig.notifier.all())
}

func TestEscalator_should_ignoreSessionEventsOfUnrelatedSessions(t *testing.T) {
	clock := NewFakeClock(t0)
	rig := newEscalatorRig(t, newFakePeerRequests(peerReq(t0)), clock)
	rig.start(t)
	clock.BlockUntilTimers(1)
	rig.owners.set("other", OwnerFacts{})
	rig.bus.Publish(events.NewSessionDeletedEvent("other"))
	assert.Never(t, func() bool { return len(rig.notifier.all()) > 0 }, 100*time.Millisecond, 10*time.Millisecond)
}

func TestPeerFaultOf_should_classifyNotSentFromStalled(t *testing.T) {
	asker := OwnerFacts{Exists: true, Live: true, WorkspaceKey: "repo"}
	_, _, faulted := PeerFaultOf(OwnerFacts{Exists: true, Live: true, WorkspaceKey: "repo"}, asker)
	assert.False(t, faulted)
	reason, notSent, _ := PeerFaultOf(OwnerFacts{Exists: true, Live: true, WorkspaceKey: "other"}, asker)
	assert.True(t, notSent)
	assert.Contains(t, reason, "different repository")
	_, notSent, faulted = PeerFaultOf(OwnerFacts{Exists: true}, asker)
	assert.True(t, faulted)
	assert.False(t, notSent, "a stopped peer is stalled, not undeliverable")
}

func (f *fakeGuidance) cancel(id, reason string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rows[id] = GuidanceFacts{Exists: true, Cancelled: true, CancelReason: reason}
}

func TestEscalator_should_deliverOnBehalfOfProvenanceToAsker_When_operatorAnswersForPeer(t *testing.T) {
	t.Parallel()
	r := newSourceRig(t)
	r.start(r.d)
	row := r.schedule(domain.GuidanceAnsweredSpec{GuidanceRequestID: "g1"}, "your help request was answered")
	r.eventually(func() bool { return len(r.d.index.lookup(domain.WakeTriggerGuidanceAnswered, "g1")) == 1 })

	// The operator answered in the peer's place: the row keeps naming the peer.
	r.guidance.answer("g1", GuidanceFacts{Answer: "use B", AnswererKind: "human", AnswererSessionUUID: "diagnose:abc"})
	r.d.OnGuidanceAnswered(context.Background(), "g1")

	r.eventually(r.delivered(row.ID))
	text := r.steerer.pane.written()[0]
	assert.Contains(t, text, "from=operator on-behalf-of=diagnose:abc")
	assert.Contains(t, text, "use B")
}

func TestGuidanceSource_should_WakeTheAskerWithCancelled_When_OperatorCancelsTheRequest(t *testing.T) {
	t.Parallel()
	r := newSourceRig(t)
	r.start(r.d)
	row := r.schedule(domain.GuidanceAnsweredSpec{GuidanceRequestID: "g3"}, "your help request was answered")
	r.eventually(func() bool { return len(r.d.index.lookup(domain.WakeTriggerGuidanceAnswered, "g3")) == 1 })

	r.guidance.cancel("g3", OperatorCancelReason)
	r.d.OnGuidanceAnswered(context.Background(), "g3")

	r.eventually(r.delivered(row.ID))
	assert.Contains(t, r.steerer.pane.written()[0], "help request g3 was cancelled (by the operator)")
}
