package prwatch

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/server/prwatch/inbox"
)

const (
	dSession = "sess-1"
	dRepo    = "o/r"
	dPR      = 7
)

type deliverLogin struct {
	login string
	ok    bool
}

func (l deliverLogin) Cached() (string, bool) { return l.login, l.ok }

// deliverHarness wires the engine to the port fakes.
type deliverHarness struct {
	inbox     *fakeInbox
	sessions  *fakeSessions
	steerer   *fakeSteerer
	coalescer *fakeCoalescer
	recorder  *fakeRecorder
	metrics   *fakeMetrics
	log       *fakeLog
	clock     *fakeClock
	notifier  *fakeNotifier
	login     *deliverLogin
	d         *Deliverer
}

func newDeliverHarness(t *testing.T, capacity int) *deliverHarness {
	t.Helper()
	h := &deliverHarness{
		inbox:     &fakeInbox{},
		sessions:  &fakeSessions{programs: map[string]string{dSession: "claude"}},
		steerer:   &fakeSteerer{},
		coalescer: &fakeCoalescer{},
		recorder:  &fakeRecorder{},
		metrics:   &fakeMetrics{},
		log:       &fakeLog{},
		clock:     newFakeClock(),
		notifier:  &fakeNotifier{},
		login:     &deliverLogin{login: "me", ok: true},
	}
	h.sessions.Set(SessionRef{UUID: dSession, Repo: dRepo, PR: dPR, State: StateLive})
	notes := NewNotifications(h.notifier, h.clock, testNotifySettings(), nil)
	h.d = NewDeliverer(DeliverDeps{
		Inbox: h.inbox, Sessions: h.sessions, Steerer: h.steerer, Coalescer: h.coalescer,
		Recorder: h.recorder, Metrics: h.metrics, Clock: h.clock, Log: h.log,
		Filter:        NewActorFilter(Settings{HumanFeedbackMode: ModeAll}, h.login),
		Notes:         notes,
		DedupCapacity: capacity,
	})
	return h
}

func (h *deliverHarness) run(items ...inbox.Item) []DeliverResult {
	h.inbox.items = append([]inbox.Item(nil), items...)
	return h.d.Process(context.Background(), items)
}

func (h *deliverHarness) ackCalls() []string {
	var out []string
	for _, c := range h.inbox.Calls() {
		if len(c) > 4 && c[:4] == "ack:" {
			out = append(out, c[4:])
		}
	}
	return out
}

func (h *deliverHarness) key(class KindClass) CoalesceKey {
	return CoalesceKey{SessionUUID: dSession, Repo: dRepo, PR: dPR, Class: class}
}

func dItem(id, kind, action, actor, details string) inbox.Item {
	ev := inbox.Event{
		Version: 1, ID: "evt-" + id, Repository: dRepo, PullRequest: dPR,
		Kind: kind, Action: action, Actor: actor,
	}
	if details != "" {
		ev.Details = []byte(details)
	}
	return inbox.Item{ID: id, SubscriptionID: "sub-1", CreatedAt: time.Date(2026, 10, 28, 11, 59, 0, 0, time.UTC), Event: ev}
}

func comment(id string) inbox.Item { return dItem(id, "comment", "created", "alice", "") }

func ciFailure(id, sha string) inbox.Item {
	it := dItem(id, "ci", "completed", "", `{"ci_kind":"check_suite","conclusion":"failure"}`)
	if sha != "" {
		it.Event.HeadSHA = &sha
	}
	return it
}

func TestDedup_should_SteerOnce_When_SameEventIDRedeliveredAcrossTicks(t *testing.T) {
	h := newDeliverHarness(t, 0)
	h.inbox.errs = map[string]error{"ack": fmt.Errorf("%w: status 500", inbox.Err5xx)}
	item := comment("i1")
	for range 3 {
		res := h.run(item)
		require.Len(t, res, 1)
		assert.Equal(t, ActionAck, res[0].Action)
		assert.Error(t, res[0].AckErr)
	}
	assert.Len(t, h.steerer.Requests(), 1)
	assert.Len(t, h.ackCalls(), 3, "each tick retries the ack")

	h.inbox.errs = nil
	res := h.run(item)
	assert.NoError(t, res[0].AckErr)
	assert.True(t, res[0].Redelivered)
	assert.Len(t, h.steerer.Requests(), 1)
}

func TestDedup_should_SteerOnce_When_SameEventIDArrivesUnderTwoInboxItemIDs(t *testing.T) {
	h := newDeliverHarness(t, 0)
	a, b := comment("i1"), comment("i2")
	b.Event.ID = a.Event.ID
	res := h.run(a, b)
	assert.Len(t, h.steerer.Requests(), 1)
	assert.ElementsMatch(t, []string{"i1", "i2"}, h.ackCalls())
	assert.Equal(t, ActionAck, res[0].Action)
	assert.Equal(t, ActionAck, res[1].Action)
	assert.Len(t, h.coalescer.steers, 1)
}

func TestDedup_should_EvictOldestAndStayBounded_When_MoreThanCapacityIDsSeen(t *testing.T) {
	h := newDeliverHarness(t, 3)
	var first inbox.Item
	for i := range 10 {
		it := comment(fmt.Sprintf("i%d", i))
		if i == 0 {
			first = it
		}
		h.run(it)
		assert.LessOrEqual(t, h.d.HandledSize(), 3)
	}
	assert.Equal(t, 3, h.d.HandledSize())
	before := len(h.steerer.Requests())
	assert.NotPanics(t, func() { h.run(first) })
	assert.Equal(t, before+1, len(h.steerer.Requests()), "evicted id is handled afresh; the guard window decides")
}

func TestDeliver_should_LeaveItemUnackedAndWriteNothing_When_GuardedSteerReportsSessionNotReady(t *testing.T) {
	h := newDeliverHarness(t, 0)
	h.steerer.outcome = map[string]Outcome{dSession: OutcomeBusy}
	res := h.run(comment("i1"))
	assert.Len(t, h.steerer.Requests(), 1)
	assert.Empty(t, h.ackCalls())
	assert.Equal(t, ActionHold, res[0].Action)
	assert.Equal(t, HoldBusy, res[0].Hold)
	assert.Empty(t, h.coalescer.steers, "no window starts for a steer that did not land")
}

func TestDeliver_should_HoldWithoutAck_When_GuardedSteerReturnsHoldOutcome(t *testing.T) {
	cases := map[Outcome]HoldReason{
		OutcomeBusy: HoldBusy, OutcomeNoStatusSource: HoldNoStatusSource,
		OutcomeGuardBusy: HoldGuardBusy, OutcomeCoolingDown: HoldCoolingDown,
	}
	for outcome, reason := range cases {
		t.Run(string(outcome), func(t *testing.T) {
			h := newDeliverHarness(t, 0)
			h.steerer.outcome = map[string]Outcome{dSession: outcome}
			for range 2 {
				res := h.run(comment("i1"))
				assert.Equal(t, reason, res[0].Hold)
			}
			assert.Empty(t, h.ackCalls())
			assert.Len(t, h.steerer.Requests(), 2, "retried next tick")
		})
	}
}

func TestDeliver_should_UseProgramFromSessionsPort_When_BuildingMessage(t *testing.T) {
	h := newDeliverHarness(t, 0)
	h.sessions.programs = map[string]string{}
	res := h.run(comment("i1"))
	assert.Empty(t, h.steerer.Requests())
	assert.Equal(t, DropSessionNotLive, res[0].Drop)
	assert.Equal(t, []string{"i1"}, h.ackCalls())

	h.sessions.programs = map[string]string{dSession: "claude"}
	h.run(comment("i2"))
	assert.Len(t, h.steerer.Requests(), 1)
}

func TestDeliver_should_AckOnlyAfterDeliveredDuplicateCoveredOrDeliberateDrop_When_EachOutcomeReturned(t *testing.T) {
	type tc struct {
		name    string
		item    inbox.Item
		outcome Outcome
		action  CoalesceAction
		ack     bool
		hold    HoldReason
		drop    DropReason
		setup   func(*deliverHarness)
	}
	humanKey := CoalesceKey{SessionUUID: dSession, Repo: dRepo, PR: dPR, Class: ClassHumanFeedback}
	cases := []tc{
		{name: "delivered", item: comment("i"), outcome: OutcomeDelivered, ack: true},
		{name: "duplicate", item: comment("i"), outcome: OutcomeDuplicate, ack: true},
		{name: "covered_same_sha", item: ciFailure("i", "abc"), action: CoalesceCoveredSameSHA, ack: true, drop: DropCoveredSameHeadSHA},
		{name: "folded_over_cap", item: comment("i"), action: CoalesceFoldOverCap, ack: true, drop: DropFoldedOverCap},
		{name: "hidden", item: comment("i"), ack: true, drop: DropHiddenSession,
			setup: func(h *deliverHarness) { h.sessions.hidden = map[string]bool{dSession: true} }},
		{name: "session_gone", item: comment("i"), ack: true, drop: DropSessionNotLive,
			setup: func(h *deliverHarness) {
				h.sessions.Set(SessionRef{UUID: dSession, Repo: dRepo, PR: dPR, State: StateGone})
			}},
		{name: "busy", item: comment("i"), outcome: OutcomeBusy, hold: HoldBusy},
		{name: "no_status_source", item: comment("i"), outcome: OutcomeNoStatusSource, hold: HoldNoStatusSource},
		{name: "guard_busy", item: comment("i"), outcome: OutcomeGuardBusy, hold: HoldGuardBusy},
		{name: "cooling_down", item: comment("i"), outcome: OutcomeCoolingDown, hold: HoldCoolingDown},
		{name: "failed", item: comment("i"), outcome: OutcomeFailed, hold: HoldSteerFailed},
		{name: "unspecified", item: comment("i"), outcome: OutcomeUnspecified, hold: HoldUnspecified},
		{name: "window_batching", item: comment("i"), action: CoalesceHold, hold: HoldWindowBatching},
		{name: "login_unresolved", item: comment("i"), hold: HoldLoginUnresolved,
			setup: func(h *deliverHarness) { h.login.ok = false }},
		{name: "login_unresolved_ci_unaffected", item: ciFailure("i", "abc"), outcome: OutcomeDelivered, ack: true,
			setup: func(h *deliverHarness) { h.login.ok = false }},
		{name: "not_tracked_live", item: comment("i"), outcome: OutcomeNotTracked, hold: HoldNotTracked},
		{name: "suspended_comment", item: comment("i"), hold: HoldSessionSuspended,
			setup: func(h *deliverHarness) {
				h.sessions.Set(SessionRef{UUID: dSession, Repo: dRepo, PR: dPR, State: StateSuspended})
			}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newDeliverHarness(t, 0)
			if c.setup != nil {
				c.setup(h)
			}
			if c.outcome != "" {
				h.steerer.outcome = map[string]Outcome{dSession: c.outcome}
			}
			if c.action != "" {
				h.coalescer.actions = map[CoalesceKey]CoalesceAction{
					humanKey: c.action, h.key(ClassCI): c.action,
				}
			}
			res := h.run(c.item)
			require.Len(t, res, 1)
			if c.ack {
				assert.Equal(t, ActionAck, res[0].Action)
				assert.Equal(t, []string{"i"}, h.ackCalls())
				assert.Equal(t, c.drop, res[0].Drop)
			} else {
				assert.Equal(t, ActionHold, res[0].Action)
				assert.Empty(t, h.ackCalls())
				assert.Equal(t, c.hold, res[0].Hold)
			}
		})
	}
}

func TestDeliver_should_HoldNotAck_When_NewSHACIEventGetsGuardDuplicate(t *testing.T) {
	h := newDeliverHarness(t, 0)
	h.steerer.outcome = map[string]Outcome{dSession: OutcomeDuplicate}
	res := h.run(ciFailure("c1", "newsha"))
	assert.Equal(t, ActionHold, res[0].Action)
	assert.Equal(t, HoldCIDuplicateNewSHA, res[0].Hold)
	assert.Empty(t, h.ackCalls())

	// Guard window passes: the same item steers and is acked.
	h.steerer.outcome = nil
	res = h.run(ciFailure("c1", "newsha"))
	assert.Equal(t, ActionAck, res[0].Action)
	assert.True(t, res[0].Steered)

	// An already-delivered event.id is acked from the dedup set, never re-held.
	h.steerer.outcome = map[string]Outcome{dSession: OutcomeDuplicate}
	res = h.run(ciFailure("c1", "newsha"))
	assert.Equal(t, ActionAck, res[0].Action)
	assert.True(t, res[0].Redelivered)
}

func TestDeliver_should_AckDuplicateWithDebugLogAndCounter_When_HumanFeedbackHitsSharedGuard(t *testing.T) {
	h := newDeliverHarness(t, 0)
	h.steerer.outcome = map[string]Outcome{dSession: OutcomeDuplicate}
	res := h.run(comment("i1"))
	assert.Equal(t, ActionAck, res[0].Action)
	assert.Equal(t, 1, h.metrics.Count(MetricDuplicateSharedGuard))
	var debug bool
	for _, l := range h.log.Lines() {
		debug = debug || (l.Level == "debug" && l.Msg == "prwatch.duplicate_shared_guard")
	}
	assert.True(t, debug)
	assert.Empty(t, h.coalescer.steers, "a duplicate is not a steer of ours")
}

type orderLog struct {
	mu  sync.Mutex
	seq []string
}

func (o *orderLog) add(s string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.seq = append(o.seq, s)
}

type orderSteerer struct {
	Steerer
	log *orderLog
}

func (s orderSteerer) Steer(ctx context.Context, r SteerRequest) (Outcome, error) {
	out, err := s.Steerer.Steer(ctx, r)
	s.log.add("steer-returned")
	return out, err
}

type orderInbox struct {
	Inbox
	log *orderLog
}

func (i orderInbox) Ack(ctx context.Context, id string) error {
	i.log.add("ack-begin:" + id)
	return i.Inbox.Ack(ctx, id)
}

func TestDeliver_should_CallAckStrictlyAfterSteerReturns_When_Delivered(t *testing.T) {
	h := newDeliverHarness(t, 0)
	log := &orderLog{}
	h.d.Steerer = orderSteerer{Steerer: h.steerer, log: log}
	h.d.Inbox = orderInbox{Inbox: h.inbox, log: log}
	h.run(comment("i1"), comment("i2"))
	assert.Equal(t, []string{"steer-returned", "ack-begin:i1", "ack-begin:i2"}, log.seq)
}

func TestDeliver_should_NotAck_When_SteerReturnsErrorOrUnspecifiedOutcomeWithNilError(t *testing.T) {
	h := newDeliverHarness(t, 0)
	h.steerer.err = fmt.Errorf("pane write failed")
	res := h.run(comment("i1"))
	assert.Equal(t, HoldSteerFailed, res[0].Hold)

	h = newDeliverHarness(t, 0)
	h.steerer.outcome = map[string]Outcome{dSession: OutcomeUnspecified}
	res = h.run(comment("i1"))
	assert.Equal(t, HoldUnspecified, res[0].Hold)
	assert.Empty(t, h.ackCalls())
}

func TestDeliver_should_TreatAck404AsDone_When_ItemWasAlreadyAckedOrRemoved(t *testing.T) {
	h := newDeliverHarness(t, 0)
	h.inbox.errs = map[string]error{"ack": fmt.Errorf("%w: POST /v1/events/i1/ack: status 404: no such item", inbox.ErrNotFound)}
	item := comment("i1")
	res := h.run(item)
	assert.NoError(t, res[0].AckErr)
	res = h.run(item)
	assert.NoError(t, res[0].AckErr)
	assert.Len(t, h.steerer.Requests(), 1, "no further steer")
}

func TestDeliver_should_SurfaceNotificationAndLeavePending_When_SteerFailsOutright(t *testing.T) {
	for name, setup := range map[string]func(*deliverHarness){
		"outcome": func(h *deliverHarness) { h.steerer.outcome = map[string]Outcome{dSession: OutcomeFailed} },
		"error":   func(h *deliverHarness) { h.steerer.err = fmt.Errorf("boom") },
	} {
		t.Run(name, func(t *testing.T) {
			h := newDeliverHarness(t, 0)
			setup(h)
			res := h.run(comment("i1"))
			sent := h.notifier.Sent()
			require.Len(t, sent, 1)
			assert.Equal(t, NotifySteerFailed, sent[0].Kind)
			assert.Equal(t, dSession, sent[0].SessionUUID)
			assert.Equal(t, ActionHold, res[0].Action)
			assert.Empty(t, h.ackCalls())
		})
	}
}

func TestDeliver_should_RaiseNoNotification_When_OutcomeIsBusyGuardBusyCoolingDownOrNoStatusSource(t *testing.T) {
	for _, o := range []Outcome{OutcomeBusy, OutcomeGuardBusy, OutcomeCoolingDown, OutcomeNoStatusSource, OutcomeUnspecified, OutcomeNotTracked} {
		h := newDeliverHarness(t, 0)
		h.steerer.outcome = map[string]Outcome{dSession: o}
		h.run(comment("i1"))
		assert.Empty(t, h.notifier.Sent(), string(o))
		recs := h.recorder.Records()
		require.Len(t, recs, 1, string(o))
		assert.Equal(t, DispositionHeld, recs[0].Disposition)
	}
}

func TestDeliver_should_RecordHoldOncePerReason_When_ItemStaysHeldAcrossTicks(t *testing.T) {
	h := newDeliverHarness(t, 0)
	h.steerer.outcome = map[string]Outcome{dSession: OutcomeBusy}
	item := comment("i1")
	for range 3 {
		h.run(item)
	}
	assert.Len(t, h.recorder.Records(), 1)
	assert.Equal(t, 1, h.metrics.Count(MetricHeld))

	h.steerer.outcome = map[string]Outcome{dSession: OutcomeCoolingDown}
	h.run(item)
	recs := h.recorder.Records()
	require.Len(t, recs, 2)
	assert.Equal(t, string(HoldCoolingDown), recs[1].Reason)
}

func TestDeliver_should_AckDropWithoutSteer_When_SessionIsHidden(t *testing.T) {
	h := newDeliverHarness(t, 0)
	h.sessions.hidden = map[string]bool{dSession: true}
	res := h.run(comment("i1"))
	assert.Empty(t, h.steerer.Requests())
	assert.Equal(t, []string{"i1"}, h.ackCalls())
	assert.Equal(t, DropHiddenSession, res[0].Drop)
	assert.Equal(t, 1, h.metrics.Count(MetricDropped))
	assert.Empty(t, h.coalescer.decides, "hidden is checked before the coalescer")
}

func TestDeliver_should_AckDropAndUnsubscribeOnReconcile_When_SessionIsGone(t *testing.T) {
	h := newDeliverHarness(t, 0)
	// Live while listed: NotTracked is a transient hold.
	h.steerer.outcome = map[string]Outcome{dSession: OutcomeNotTracked}
	res := h.run(comment("i1"))
	assert.Equal(t, HoldNotTracked, res[0].Hold)

	// Once the port lists the session gone, the same item is ack-dropped.
	h.sessions.Set(SessionRef{UUID: dSession, Repo: dRepo, PR: dPR, State: StateGone})
	res = h.run(comment("i1"))
	assert.Equal(t, DropSessionNotLive, res[0].Drop)
	assert.Equal(t, []string{"i1"}, h.ackCalls())
	assert.Len(t, h.steerer.Requests(), 1)

	// No session for the PR at all is the same.
	h.sessions.Set()
	res = h.run(comment("i2"))
	assert.Equal(t, DropSessionNotLive, res[0].Drop)
}

func TestDeliver_should_AckDrop_When_EventIsForClosedPR(t *testing.T) {
	h := newDeliverHarness(t, 0)
	res := h.run(dItem("i1", "pull_request", "closed", "alice", ""))
	assert.Equal(t, DropPullRequestEvent, res[0].Drop)
	assert.Empty(t, h.steerer.Requests())
	assert.Equal(t, []string{"i1"}, h.ackCalls())
}

func TestDeliver_should_AckDropWithCounter_When_SessionIsStoppedPermanentlyFailedOrRemoved(t *testing.T) {
	h := newDeliverHarness(t, 0)
	h.sessions.Set(SessionRef{UUID: dSession, Repo: dRepo, PR: dPR, State: StateGone})
	h.run(comment("i1"), ciFailure("i2", "abc"))
	assert.Empty(t, h.steerer.Requests())
	assert.Equal(t, []string{"i1", "i2"}, h.ackCalls())
	assert.Equal(t, 2, h.metrics.Count(MetricDropped))
	for _, c := range h.metrics.calls {
		assert.Equal(t, string(DropSessionNotLive), c.Labels["drop_reason"])
	}
}

func TestDeliver_should_HoldNotTracked_When_SessionsPortStillListsSessionLive(t *testing.T) {
	h := newDeliverHarness(t, 0)
	h.steerer.outcome = map[string]Outcome{dSession: OutcomeNotTracked}
	res := h.run(comment("i1"))
	assert.Equal(t, HoldNotTracked, res[0].Hold)
	assert.Empty(t, h.ackCalls())
}

func TestDeliver_should_HoldNotAck_When_GuardReturnsCoolingDownAndWindowIsClosed(t *testing.T) {
	h := newDeliverHarness(t, 0)
	h.coalescer.actions = map[CoalesceKey]CoalesceAction{h.key(ClassHumanFeedback): CoalesceSteer}
	h.steerer.outcome = map[string]Outcome{dSession: OutcomeCoolingDown}
	res := h.run(comment("i1"))
	assert.Equal(t, HoldCoolingDown, res[0].Hold)
	assert.Empty(t, h.ackCalls())
}

func TestDeliver_should_MergeKindsIntoOneSteerAndAckAll_When_SeveralItemsDueInOneTick(t *testing.T) {
	h := newDeliverHarness(t, 0)
	review := dItem("r1", "review", "submitted", "alice", `{"state":"changes_requested"}`)
	res := h.run(review, ciFailure("c1", "abc"), comment("m1"), comment("m2"))
	reqs := h.steerer.Requests()
	require.Len(t, reqs, 1)
	assert.Equal(t, []Kind{KindReview, KindComment, KindCI}, reqs[0].Kinds)
	assert.Contains(t, reqs[0].Text, "review, comment, CI failure")
	assert.ElementsMatch(t, []string{"r1", "c1", "m1", "m2"}, h.ackCalls())
	for _, r := range res {
		assert.True(t, r.Steered)
	}
	assert.Len(t, h.coalescer.steers, 2, "one RecordSteer per class key")
}

func TestDeliver_should_AckFoldedItemsOnlyAfterClosingSteer_When_WindowHeldSiblings(t *testing.T) {
	h := newDeliverHarness(t, 0)
	hf := h.key(ClassHumanFeedback)
	h.coalescer.actions = map[CoalesceKey]CoalesceAction{hf: CoalesceHold}
	res := h.run(comment("m1"), comment("m2"))
	assert.Empty(t, h.steerer.Requests())
	assert.Empty(t, h.ackCalls())
	assert.Equal(t, HoldWindowBatching, res[0].Hold)
	// The second Decide saw one already-held sibling.
	assert.Len(t, h.coalescer.decides, 2)

	h.coalescer.actions = nil // window closed
	h.run(comment("m1"), comment("m2"))
	assert.Len(t, h.steerer.Requests(), 1)
	assert.ElementsMatch(t, []string{"m1", "m2"}, h.ackCalls())
}

func TestDeliver_should_ApplyActorFilterBeforeCoalescer_When_CommentIsFromSelfOrBot(t *testing.T) {
	h := newDeliverHarness(t, 0)
	h.d.Filter = NewActorFilter(Settings{HumanFeedbackMode: ModeAll, IgnoreBotActors: true}, h.login)
	res := h.run(dItem("i1", "comment", "created", "ME", ""), dItem("i2", "comment", "created", "ci[bot]", ""))
	assert.Equal(t, DropSelfActor, res[0].Drop)
	assert.Equal(t, DropBotActor, res[1].Drop)
	assert.Empty(t, h.coalescer.decides)
	recs := h.recorder.Records()
	require.Len(t, recs, 2)
	assert.Equal(t, ActorSelf, recs[0].ActorClass)
}

func TestDeliver_should_FailClosed_When_SessionListErrors(t *testing.T) {
	h := newDeliverHarness(t, 0)
	h.sessions.listErr = fmt.Errorf("db down")
	res := h.run(comment("i1"))
	assert.Nil(t, res)
	assert.Empty(t, h.ackCalls())
	assert.Empty(t, h.steerer.Requests())
}

func TestDeliver_should_RecordSteeredDispositionWithoutLoginOrText_When_Delivered(t *testing.T) {
	h := newDeliverHarness(t, 0)
	stranger := &fakeStrangerWatch{}
	followup := &fakeFollowup{}
	h.d.Stranger, h.d.Followup = stranger, followup
	h.run(comment("i1"), ciFailure("c1", "abc"))
	recs := h.recorder.Records()
	require.Len(t, recs, 2)
	assert.Equal(t, DispositionSteered, recs[0].Disposition)
	assert.Equal(t, ActorOther, recs[0].ActorClass)
	assert.Empty(t, recs[1].ActorClass)
	assert.Equal(t, 1, stranger.OtherCount(dRepo), "only the human-feedback item counts")
	assert.Equal(t, 2, h.metrics.Count(MetricSteers))
}
