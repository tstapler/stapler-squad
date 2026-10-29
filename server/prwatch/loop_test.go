package prwatch

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/tstapler/stapler-squad/server/prwatch/inbox"
)

// pagedInbox caps Events at the inbox page size, like the real inbox, and
// makes Watch register a subscription so the reconcile listing sees it.
type pagedInbox struct{ *fakeInbox }

func (p pagedInbox) Watch(ctx context.Context, consumerID, repo string, pr int) (inbox.WatchResult, error) {
	res, err := p.fakeInbox.Watch(ctx, consumerID, repo, pr)
	if err != nil {
		return res, err
	}
	p.mu.Lock()
	p.subs = append(p.subs, inbox.Subscription{
		ID: "sub-" + repo + "#" + itoa(pr), ConsumerID: consumerID, Repository: repo, PullRequest: pr, Active: true,
	})
	p.mu.Unlock()
	return res, nil
}

func (p pagedInbox) Events(ctx context.Context, consumerID string) ([]inbox.Item, error) {
	items, err := p.fakeInbox.Events(ctx, consumerID)
	if len(items) > EventsPageSize {
		items = items[:EventsPageSize]
	}
	return items, err
}

type loopHarness struct {
	inbox     *fakeInbox
	sessions  *fakeSessions
	steerer   *fakeSteerer
	coalescer *fakeCoalescer
	recorder  *fakeRecorder
	metrics   *fakeMetrics
	log       *fakeLog
	clock     *fakeClock
	notifier  *fakeNotifier
	settings  Settings
	notes     *Notifications
	engine    *Deliverer
	poller    *Poller
	deps      *LoopDeps
}

func loopSettings() Settings {
	s := testNotifySettings()
	s.ConsumerID = "squad:test"
	s.PollInterval = 5 * time.Second
	s.ReconcileInterval = 60 * time.Second
	s.HeldNoticeAfter = time.Hour
	s.HeldAgeCeiling = 24 * time.Hour
	s.HumanFeedbackMode = ModeAll
	return s
}

func newLoopHarness(t *testing.T, mut func(*loopHarness)) *loopHarness {
	t.Helper()
	h := &loopHarness{
		inbox:     &fakeInbox{},
		sessions:  &fakeSessions{programs: map[string]string{dSession: "claude"}},
		steerer:   &fakeSteerer{},
		coalescer: &fakeCoalescer{},
		recorder:  &fakeRecorder{},
		metrics:   &fakeMetrics{},
		log:       &fakeLog{},
		clock:     newFakeClock(),
		notifier:  &fakeNotifier{},
		settings:  loopSettings(),
	}
	h.sessions.Set(SessionRef{UUID: dSession, Repo: dRepo, PR: dPR, State: StateLive})
	if mut != nil {
		mut(h)
	}
	h.notes = NewNotifications(h.notifier, h.clock, h.settings, nil)
	h.engine = NewDeliverer(DeliverDeps{
		Inbox: pagedInbox{h.inbox}, Sessions: h.sessions, Steerer: h.steerer, Coalescer: h.coalescer,
		Recorder: h.recorder, Metrics: h.metrics, Clock: h.clock, Log: h.log,
		Filter: NewActorFilter(Settings{HumanFeedbackMode: ModeAll}, deliverLogin{login: "me", ok: true}),
		Notes:  h.notes, Mode: ModeAll,
	})
	h.deps = &LoopDeps{
		Inbox: pagedInbox{h.inbox}, Sessions: h.sessions, Engine: h.engine, Notifier: h.notifier,
		Coalescer: h.coalescer, Metrics: h.metrics, Log: h.log, Clock: h.clock, Settings: h.settings,
		HeldAge: NewHeldAge(h.clock, h.settings, h.notes),
	}
	return h
}

// start builds the poller after any deps tweaks.
func (h *loopHarness) start(mut func(*LoopDeps)) *Poller {
	if mut != nil {
		mut(h.deps)
	}
	h.poller = NewPoller(*h.deps)
	return h.poller
}

func (h *loopHarness) tick() { h.poller.Tick(context.Background()) }

func (h *loopHarness) count(prefix string) int {
	n := 0
	for _, c := range h.inbox.Calls() {
		if strings.HasPrefix(c, prefix) {
			n++
		}
	}
	return n
}

func (h *loopHarness) warns(msg string) int {
	n := 0
	for _, l := range h.log.Lines() {
		if l.Level == "warn" && l.Msg == msg {
			n++
		}
	}
	return n
}

func (h *loopHarness) metricLabelled(name, key, val string) int {
	h.metrics.mu.Lock()
	defer h.metrics.mu.Unlock()
	n := 0
	for _, c := range h.metrics.calls {
		if c.Name == name && c.Labels[key] == val {
			n++
		}
	}
	return n
}

func TestPoller_should_SteerOwningSessionOnceAndAck_When_CommentEventPendingForWatchedPR(t *testing.T) {
	h := newLoopHarness(t, nil)
	h.inbox.items = []inbox.Item{comment("i1")}
	h.start(nil)
	h.tick()

	reqs := h.steerer.Requests()
	require.Len(t, reqs, 1)
	assert.Equal(t, dSession, reqs[0].SessionUUID)
	calls := h.inbox.Calls()
	steerAt, ackAt := -1, -1
	for i, c := range calls {
		if c == "ack:i1" {
			ackAt = i
		}
		if strings.HasPrefix(c, "events:") && steerAt < 0 {
			steerAt = i
		}
	}
	assert.Greater(t, ackAt, steerAt, "ack follows the events read that produced the steer")
	assert.Equal(t, 1, h.count("ack:"))
}

func TestPoller_should_RetryAckWithoutResteering_When_AckFailsAfterConfirmedSteer(t *testing.T) {
	h := newLoopHarness(t, nil)
	h.inbox.items = []inbox.Item{comment("i1")}
	h.inbox.errs = map[string]error{"ack": fmt.Errorf("%w: status 500", inbox.Err5xx)}
	h.start(nil)
	h.tick()
	h.inbox.mu.Lock()
	h.inbox.errs = nil
	h.inbox.mu.Unlock()
	h.tick()

	assert.Len(t, h.steerer.Requests(), 1)
	assert.Equal(t, 2, h.count("ack:i1"))
	assert.Empty(t, h.inbox.items)
}

func TestPoller_should_NotBlockOtherSessions_When_OneSessionIsNotReady(t *testing.T) {
	h := newLoopHarness(t, func(h *loopHarness) {
		h.sessions.programs["s2"] = "claude"
		h.sessions.Set(
			SessionRef{UUID: dSession, Repo: dRepo, PR: dPR, State: StateLive},
			SessionRef{UUID: "s2", Repo: dRepo, PR: 8, State: StateLive},
		)
		h.steerer.outcome = map[string]Outcome{dSession: OutcomeBusy}
	})
	other := comment("i2")
	other.Event.PullRequest = 8
	h.inbox.items = []inbox.Item{comment("i1"), other}
	h.start(nil)
	h.tick()

	assert.Equal(t, []string{"i2"}, ackedIDs(h))
	assert.Len(t, h.inbox.items, 1, "the busy session's item stays pending")
}

func ackedIDs(h *loopHarness) []string {
	var out []string
	for _, c := range h.inbox.Calls() {
		if strings.HasPrefix(c, "ack:") {
			out = append(out, c[4:])
		}
	}
	return out
}

func TestPoller_should_WarnOnceAndKeepTicking_When_InboxUnreachable(t *testing.T) {
	h := newLoopHarness(t, nil)
	h.inbox.errs = map[string]error{"events": fmt.Errorf("%w: refused", inbox.ErrTransport)}
	h.start(nil)
	for range 3 {
		h.tick()
		h.clock.Advance(time.Second)
	}
	assert.Equal(t, 1, h.warns("prwatch.inbox_error"))
	assert.Equal(t, 3, h.count("events:"))
	assert.Empty(t, h.steerer.Requests())
}

func TestPoller_should_RateLimitWarnings_When_FailureRepeatsForManyTicks(t *testing.T) {
	h := newLoopHarness(t, nil)
	h.inbox.errs = map[string]error{"events": fmt.Errorf("%w: refused", inbox.ErrTransport)}
	h.start(nil)
	for range 1000 {
		h.tick()
		h.clock.Advance(time.Second)
	}
	// 1000s span: first warning at 0s, then one per 5 minutes.
	assert.Equal(t, 4, h.warns("prwatch.inbox_error"))
	assert.Equal(t, 1000, h.metricLabelled(MetricInboxErrors, "error_class", errClassTransport))
}

func TestPoller_should_LogRecoveryOnceAndResumeSteering_When_InboxComesBack(t *testing.T) {
	h := newLoopHarness(t, nil)
	h.inbox.errs = map[string]error{"events": fmt.Errorf("%w: refused", inbox.ErrTransport)}
	h.start(nil)
	h.tick()
	h.inbox.mu.Lock()
	h.inbox.errs = nil
	h.inbox.items = []inbox.Item{comment("i1")}
	h.inbox.mu.Unlock()
	h.tick()
	h.tick()

	recovered := 0
	for _, l := range h.log.Lines() {
		if l.Msg == "prwatch.inbox_recovered" {
			recovered++
		}
	}
	assert.Equal(t, 1, recovered)
	assert.Len(t, h.steerer.Requests(), 1)
}

func TestPoller_should_WarnAndSkipTick_When_TokenFileMissing(t *testing.T) {
	h := newLoopHarness(t, nil)
	h.inbox.errs = map[string]error{"events": fmt.Errorf("%w: /tmp/token", inbox.ErrTokenMissing)}
	h.start(nil)
	h.tick()
	h.tick()
	assert.Equal(t, 1, h.warns("prwatch.inbox_error"))
	assert.Equal(t, 2, h.metricLabelled(MetricInboxErrors, "error_class", errClassTokenMissing))
	assert.Empty(t, h.steerer.Requests())
}

func TestPoller_should_WarnAndBackOff_When_TokenRejectedWith401(t *testing.T) {
	h := newLoopHarness(t, nil)
	h.inbox.errs = map[string]error{"events": fmt.Errorf("%w: status 401", inbox.ErrUnauthorized)}
	h.start(nil)
	h.tick()
	assert.Equal(t, 1, h.count("events:"), "no retry within the tick")
	h.clock.Advance(5 * time.Second)
	h.tick()
	assert.Equal(t, 1, h.count("events:"), "paused after a 401")

	h.inbox.mu.Lock()
	h.inbox.errs = nil
	h.inbox.items = []inbox.Item{comment("i1")}
	h.inbox.mu.Unlock()
	h.clock.Advance(unauthorizedBackoff)
	h.tick()
	assert.Len(t, h.steerer.Requests(), 1, "rotated token picked up after the pause")
}

func TestPoller_should_IssueOneEventsRequestPerTick_When_NSessionsWatched(t *testing.T) {
	h := newLoopHarness(t, func(h *loopHarness) {
		refs := make([]SessionRef, 0, 5)
		for i := range 5 {
			refs = append(refs, SessionRef{UUID: fmt.Sprintf("s%d", i), Repo: dRepo, PR: 10 + i, State: StateLive})
		}
		h.sessions.Set(refs...)
	})
	h.start(nil)
	h.tick()
	h.tick()
	assert.Equal(t, 2, h.count("events:squad:test"))
}

func TestPoller_should_IssueNoEventsRequest_When_NoSessionHasAWatchedPR(t *testing.T) {
	h := newLoopHarness(t, func(h *loopHarness) {
		h.sessions.Set(
			SessionRef{UUID: "a", Repo: dRepo, PR: 0, State: StateLive, SkipReason: SkipNoPR},
			SessionRef{UUID: "b", Repo: dRepo, PR: 3, State: StateGone},
		)
	})
	h.start(nil)
	h.tick()
	h.tick()
	assert.Zero(t, h.count("events:"))
}

func TestPoller_should_ListSubscriptionsEveryReconcileIntervalAndCallWatchOnlyOnSetChange_When_SetIsStable(t *testing.T) {
	h := newLoopHarness(t, nil)
	h.start(nil)
	for range 37 { // 36 ticks of 5s = 180s, tick 0 included
		h.tick()
		h.clock.Advance(5 * time.Second)
	}
	assert.Equal(t, 4, h.count("subscriptions:"), "start, 60s, 120s, 180s")
	assert.Equal(t, 1, h.count("watch:"))

	h.sessions.Set(
		SessionRef{UUID: dSession, Repo: dRepo, PR: dPR, State: StateLive},
		SessionRef{UUID: "s2", Repo: dRepo, PR: 9, State: StateLive},
	)
	h.tick()
	assert.Equal(t, 2, h.count("watch:"), "exactly one new watch for the added session")
}

func TestPoller_should_ProcessFurtherPages_When_FirstPageHas200ItemsAndAcksFreeSlots(t *testing.T) {
	h := newLoopHarness(t, nil)
	for i := range 1300 {
		h.inbox.items = append(h.inbox.items, comment(fmt.Sprintf("i%d", i)))
	}
	h.start(nil)
	h.tick()
	assert.Equal(t, MaxPagesPerTick, h.count("events:"))
	assert.Len(t, h.inbox.items, 1300-MaxPagesPerTick*EventsPageSize, "remainder waits for the next tick")
	h.tick()
	assert.Empty(t, h.inbox.items)
}

func TestPoller_should_StopFetchingPages_When_FullPageIsAllHeld(t *testing.T) {
	h := newLoopHarness(t, func(h *loopHarness) {
		h.steerer.outcome = map[string]Outcome{dSession: OutcomeBusy}
	})
	for i := range 250 {
		h.inbox.items = append(h.inbox.items, comment(fmt.Sprintf("i%d", i)))
	}
	h.start(nil)
	h.tick()
	assert.Equal(t, 1, h.count("events:"), "nothing acked means the next page would be identical")
}

func TestPoller_should_RespectContextDeadlineAndPageCap_When_ManyPagesOrSlowInbox(t *testing.T) {
	h := newLoopHarness(t, nil)
	for i := range 1300 {
		h.inbox.items = append(h.inbox.items, comment(fmt.Sprintf("i%d", i)))
	}
	slow := &slowInbox{Inbox: pagedInbox{h.inbox}, delay: 200 * time.Millisecond}
	h.engine.Inbox = slow
	h.start(func(d *LoopDeps) {
		d.Inbox = slow
		d.Settings.PollInterval = 700 * time.Millisecond
	})
	start := time.Now()
	h.tick()
	assert.Less(t, time.Since(start), 1500*time.Millisecond, "the tick stops at its deadline")
	assert.LessOrEqual(t, h.count("events:"), MaxPagesPerTick)
}

// slowInbox adds latency to every Events and Ack, honouring the context.
type slowInbox struct {
	Inbox
	delay time.Duration
}

func (s *slowInbox) Events(ctx context.Context, c string) ([]inbox.Item, error) {
	if err := sleepCtx(ctx, s.delay); err != nil {
		return nil, err
	}
	return s.Inbox.Events(ctx, c)
}

func (s *slowInbox) Ack(ctx context.Context, id string) error {
	if err := sleepCtx(ctx, s.delay); err != nil {
		return err
	}
	return s.Inbox.Ack(ctx, id)
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	select {
	case <-time.After(d):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestPoller_should_MakeNoRequestsAndKeepSubscriptions_When_FlagFlippedOffMidRun(t *testing.T) {
	h := newLoopHarness(t, nil)
	var mu sync.Mutex
	on := true
	h.start(func(d *LoopDeps) {
		d.Enabled = func() bool { mu.Lock(); defer mu.Unlock(); return on }
	})
	h.tick()
	before := len(h.inbox.Calls())
	require.Positive(t, before)

	mu.Lock()
	on = false
	mu.Unlock()
	h.inbox.items = []inbox.Item{comment("i1")}
	h.clock.Advance(2 * time.Minute)
	h.tick()
	assert.Len(t, h.inbox.Calls(), before, "no request while off")
	assert.Zero(t, h.count("unsubscribe:"))
	assert.Empty(t, h.steerer.Requests())

	mu.Lock()
	on = true
	mu.Unlock()
	h.tick()
	assert.Len(t, h.steerer.Requests(), 1, "resumes when on again")
}

func TestPoller_should_MakeNoRequests_When_LeaderNotHeld(t *testing.T) {
	h := newLoopHarness(t, nil)
	guard := &fakeLeader{}
	state := &fakeInboxStateTracker{}
	h.start(func(d *LoopDeps) {
		d.Leader = NewLeadership(guard, h.clock, h.log, 30*time.Second)
		d.State = state
	})
	h.inbox.items = []inbox.Item{comment("i1")}
	h.tick()
	assert.Empty(t, h.inbox.Calls())
	assert.Equal(t, InboxStateLeaderNotHeld, state.Current())

	guard.mu.Lock()
	guard.grant = true
	guard.mu.Unlock()
	h.clock.Advance(31 * time.Second)
	h.tick()
	assert.Len(t, h.steerer.Requests(), 1)
	assert.Equal(t, InboxStateOK, state.Current())
}

func TestPoller_should_SkipTickAndCountOverrun_When_PreviousTickStillRunning(t *testing.T) {
	h := newLoopHarness(t, nil)
	block := &blockingSteerer{entered: make(chan struct{}), release: make(chan struct{})}
	h.engine.Steerer = block
	h.inbox.items = []inbox.Item{comment("i1")}
	h.start(nil)

	done := make(chan struct{})
	go func() { h.tick(); close(done) }()
	<-block.entered
	assert.False(t, h.poller.Tick(context.Background()), "single in-flight tick")
	assert.Equal(t, 1, h.metrics.Count(MetricTickOverrun))
	close(block.release)
	<-done
	assert.True(t, h.poller.Tick(context.Background()))
}

type blockingSteerer struct {
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (b *blockingSteerer) Steer(context.Context, SteerRequest) (Outcome, error) {
	b.once.Do(func() { close(b.entered) })
	<-b.release
	return OutcomeDelivered, nil
}

func TestPoller_should_StopPromptly_When_ContextCancelled(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())
	h := newLoopHarness(t, nil)
	h.start(func(d *LoopDeps) { d.Settings.PollInterval = 50 * time.Millisecond })
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { h.poller.Run(ctx); close(done) }()
	time.Sleep(120 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not return after cancel")
	}
}

func TestPoller_should_NotPanicOrStop_When_PortsReturnNilOrPanic(t *testing.T) {
	h := newLoopHarness(t, nil)
	h.start(func(d *LoopDeps) { d.Sessions = &panicOnceSessions{fakeSessions: h.sessions} })
	h.inbox.items = []inbox.Item{comment("i1")}
	assert.NotPanics(t, h.tick)
	assert.Equal(t, 1, h.warns("prwatch.tick_panic"))
	h.tick()
	assert.Len(t, h.steerer.Requests(), 1, "next tick proceeds")
}

type panicOnceSessions struct {
	*fakeSessions
	mu      sync.Mutex
	paniced bool
}

func (p *panicOnceSessions) List(ctx context.Context) ([]SessionRef, error) {
	p.mu.Lock()
	first := !p.paniced
	p.paniced = true
	p.mu.Unlock()
	if first {
		panic("adapter bug")
	}
	return p.fakeSessions.List(ctx)
}

func TestPoller_should_AckAndDropHeldItemPastCeiling_And_RaiseHeldLongBefore(t *testing.T) {
	h := newLoopHarness(t, func(h *loopHarness) {
		h.steerer.outcome = map[string]Outcome{dSession: OutcomeBusy}
	})
	old := comment("i1")
	old.CreatedAt = h.clock.Now().Add(-90 * time.Minute)
	h.inbox.items = []inbox.Item{old}
	h.start(nil)
	h.tick()
	assert.Len(t, h.inbox.items, 1, "held, not dropped, before the ceiling")
	sent := h.notifier.Sent()
	require.NotEmpty(t, sent)
	assert.Equal(t, NotifyHeldLong, sent[len(sent)-1].Kind)

	h.clock.Advance(24 * time.Hour)
	h.tick()
	assert.Empty(t, h.inbox.items, "ceiling acks the stuck item")
	assert.Equal(t, 1, h.metricLabelled(MetricDropped, "drop_reason", string(DropHeldAgeCeiling)))
	sent = h.notifier.Sent()
	assert.Equal(t, NotifyHeldDropped, sent[len(sent)-1].Kind)
}

func TestPoller_should_RunCatchUpOnFirstSubscribe_When_PRChecksAlreadyFailing(t *testing.T) {
	h := newLoopHarness(t, nil)
	snaps := &fakePRSnapshot{snapshots: map[string]PRSnapshot{
		dSession: {CheckConclusion: "failure", CheckedAt: h.clock.Now()},
	}}
	h.start(func(d *LoopDeps) {
		d.Catchup = NewCatchup(CatchupDeps{
			Snapshots: snaps, Steerer: h.steerer, Coalescer: h.coalescer,
			Metrics: h.metrics, Log: h.log, Settings: h.settings,
		})
	})
	h.tick()
	require.Len(t, h.steerer.Requests(), 1)
	assert.Equal(t, []Kind{KindCI}, h.steerer.Requests()[0].Kinds)
	assert.Equal(t, 1, h.metrics.Count(MetricCatchupSteers))
	h.tick()
	assert.Len(t, h.steerer.Requests(), 1, "catch-up runs once")
}

func TestPoller_should_PublishStatus_When_TickRuns(t *testing.T) {
	h := newLoopHarness(t, nil)
	status := NewStatusTracker(h.clock, nil)
	h.inbox.items = []inbox.Item{comment("i1")}
	h.engine.Recorder = status
	h.start(func(d *LoopDeps) { d.Status = status })
	h.tick()
	st := status.Snapshot(h.coalescer)
	assert.True(t, st.Enabled)
	assert.True(t, st.LeaderHeld)
	assert.Equal(t, InboxStateOK, st.InboxState)
	require.NotNil(t, st.LastSteer)
	assert.Equal(t, 1, st.Counts.Watched)
}

func TestPoller_should_RaiseCIOnlyNoticeOnce_When_EffectiveModeIsCIOnly(t *testing.T) {
	cases := []struct {
		name string
		mut  func(*Settings)
		want int
	}{
		{"off", func(s *Settings) { s.HumanFeedbackMode = ModeOff }, 1},
		{"allowlist_empty", func(s *Settings) { s.HumanFeedbackMode = ModeAllowlist }, 1},
		{"allowlist_listed", func(s *Settings) { s.HumanFeedbackMode = ModeAllowlist; s.ActorAllowlist = []string{"bob"} }, 0},
		{"all", func(s *Settings) { s.HumanFeedbackMode = ModeAll }, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newLoopHarness(t, func(h *loopHarness) { tc.mut(&h.settings) })
			h.start(nil)
			h.tick()
			h.tick()
			got := 0
			for _, n := range h.notifier.Sent() {
				if n.Title == TitleCIOnly {
					got++
				}
			}
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestPoller_should_PublishCoverageReasonToStatus_When_WatchUnavailable(t *testing.T) {
	h := newLoopHarness(t, nil)
	h.inbox.watchResult = inbox.WatchResult{Coverage: inbox.CoverageUnavailable, Reason: "no admin access"}
	status := NewStatusTracker(h.clock, nil)
	h.start(func(d *LoopDeps) { d.Status = status })
	h.tick()
	st := status.Snapshot(h.coalescer)
	require.Len(t, st.Repos, 1)
	assert.Equal(t, CoverageStateUnavailable, st.Repos[0].Coverage)
	assert.Equal(t, "no admin access", st.Repos[0].Reason)
}

func TestDeliver_should_ApplySessionGate_When_SessionIsSuspendedOrNotTracked(t *testing.T) {
	cases := []struct {
		name     string
		item     inbox.Item
		state    SessionState
		outcome  Outcome
		wantAck  bool
		wantDrop DropReason
		wantHold HoldReason
	}{
		{"suspended_ci_dropped", ciFailure("i", "sha1"), StateSuspended, "", true, DropSessionNotLive, ""},
		{"suspended_comment_held", comment("i"), StateSuspended, "", false, "", HoldSessionSuspended},
		{"suspended_changes_requested_dropped", dItem("i", "review", "submitted", "alice", `{"state":"changes_requested"}`), StateSuspended, "", true, DropSessionNotLive, ""},
		{"suspended_other_review_held", dItem("i", "review", "submitted", "alice", `{"state":"commented"}`), StateSuspended, "", false, "", HoldSessionSuspended},
		{"gone_comment_dropped", comment("i"), StateGone, "", true, DropSessionNotLive, ""},
		{"not_tracked_live_held", comment("i"), StateLive, OutcomeNotTracked, false, "", HoldNotTracked},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newDeliverHarness(t, 0)
			h.d.Mode = ModeAll
			h.sessions.Set(SessionRef{UUID: dSession, Repo: dRepo, PR: dPR, State: tc.state})
			if tc.outcome != "" {
				h.steerer.outcome = map[string]Outcome{dSession: tc.outcome}
			}
			res := h.run(tc.item)
			require.Len(t, res, 1)
			assert.Equal(t, tc.wantAck, res[0].Action == ActionAck)
			assert.Equal(t, tc.wantDrop, res[0].Drop)
			assert.Equal(t, tc.wantHold, res[0].Hold)
		})
	}
}

func TestDeliver_should_DropNotTracked_When_SessionListedGoneAfterSteerAttempt(t *testing.T) {
	h := newDeliverHarness(t, 0)
	h.steerer.outcome = map[string]Outcome{dSession: OutcomeNotTracked}
	h.sessions.Set(SessionRef{UUID: dSession, Repo: dRepo, PR: dPR, State: StateLive})
	res := h.run(comment("i1"))
	assert.Equal(t, HoldNotTracked, res[0].Hold, "transient while listed live")
	assert.Equal(t, ActionHold, res[0].Action)
}

// logText flattens every fakeLog line for sentinel scans.
func (h *loopHarness) logText() string {
	var b strings.Builder
	for _, l := range h.log.Lines() {
		b.WriteString(l.Level + " " + l.Msg + " " + fmt.Sprint(l.KV...) + "\n")
	}
	return b.String()
}

func (h *loopHarness) metricText() string {
	h.metrics.mu.Lock()
	defer h.metrics.mu.Unlock()
	var b strings.Builder
	for _, c := range h.metrics.calls {
		b.WriteString(c.Name + fmt.Sprint(c.Labels) + "\n")
	}
	return b.String()
}

func logKV(l LogLine, key string) (any, bool) {
	for i := 0; i+1 < len(l.KV); i += 2 {
		if l.KV[i] == key {
			return l.KV[i+1], true
		}
	}
	return nil, false
}

func (h *loopHarness) linesNamed(msg string) []LogLine {
	var out []LogLine
	for _, l := range h.log.Lines() {
		if l.Msg == msg {
			out = append(out, l)
		}
	}
	return out
}

func TestPoller_should_NeverLogOrReturnToken_When_EveryErrorPathRuns(t *testing.T) {
	const sentinel = "tok-SENTINEL-9f3a"
	errs := []error{
		fmt.Errorf("%w: status 401 Authorization: Bearer %s", inbox.ErrUnauthorized, sentinel),
		fmt.Errorf("%w: status 502 body echoes Authorization: Bearer %s", inbox.Err5xx, sentinel),
		fmt.Errorf("%w: dial %s", inbox.ErrTransport, sentinel),
		fmt.Errorf("%w: bad json near %s", inbox.ErrDecode, sentinel),
		fmt.Errorf("%w: /path/%s mode 0644", inbox.ErrTokenPermissions, sentinel),
		fmt.Errorf("%w: status 400 %s", inbox.ErrRejected, sentinel),
	}
	h := newLoopHarness(t, nil)
	status := NewStatusTracker(h.clock, nil)
	h.start(func(d *LoopDeps) { d.Status = status })
	for _, e := range errs {
		h.inbox.mu.Lock()
		h.inbox.errs = map[string]error{"events": e}
		h.inbox.mu.Unlock()
		h.clock.Advance(unauthorizedBackoff + warnInterval)
		h.tick()
	}
	// Ack failure after a confirmed steer.
	h.inbox.mu.Lock()
	h.inbox.errs = map[string]error{"ack": fmt.Errorf("%w: Bearer %s", inbox.Err5xx, sentinel)}
	h.inbox.items = []inbox.Item{comment("i1")}
	h.inbox.mu.Unlock()
	h.clock.Advance(unauthorizedBackoff)
	h.tick()

	require.NotEmpty(t, h.log.Lines())
	assert.NotContains(t, h.logText(), sentinel)
	assert.NotContains(t, h.metricText(), sentinel)
	st := status.Snapshot(h.coalescer)
	assert.NotContains(t, fmt.Sprintf("%+v", st), sentinel)
	assert.NotContains(t, RenderText(st, h.clock.Now(), "host"), sentinel)
	for _, n := range h.notifier.Sent() {
		assert.NotContains(t, fmt.Sprintf("%+v", n), sentinel)
	}
}

func TestPoller_should_CountSteeredHeldDroppedAndInboxErrorsByType_When_ScenariosRun(t *testing.T) {
	h := newLoopHarness(t, func(h *loopHarness) {
		for _, u := range []string{"s2", "s3"} {
			h.sessions.programs[u] = "claude"
		}
		h.sessions.hidden = map[string]bool{"s3": true}
		h.sessions.Set(
			SessionRef{UUID: dSession, Repo: dRepo, PR: dPR, State: StateLive},
			SessionRef{UUID: "s2", Repo: dRepo, PR: 8, State: StateLive},
			SessionRef{UUID: "s3", Repo: dRepo, PR: 9, State: StateLive},
		)
		h.steerer.outcome = map[string]Outcome{"s2": OutcomeBusy}
	})
	onPR := func(it inbox.Item, pr int) inbox.Item { it.Event.PullRequest = pr; return it }
	unknown := dItem("i4", "mystery", "created", "alice", "")
	h.inbox.items = []inbox.Item{comment("i1"), onPR(comment("i2"), 8), onPR(comment("i3"), 9), unknown}
	h.start(nil)
	h.tick()

	assert.Equal(t, 1, h.metricLabelled(MetricSteers, "kind", string(KindComment)))
	assert.Equal(t, 1, h.metrics.Count(MetricSteers))
	assert.Equal(t, 1, h.metricLabelled(MetricHeld, "hold_reason", string(HoldBusy)))
	assert.Equal(t, 1, h.metrics.Count(MetricHeld))
	assert.Equal(t, 1, h.metricLabelled(MetricDropped, "drop_reason", string(DropHiddenSession)))
	assert.Equal(t, 1, h.metricLabelled(MetricDropped, "drop_reason", string(DropUnknownKind)))
	assert.Equal(t, 2, h.metrics.Count(MetricDropped))

	h.inbox.mu.Lock()
	h.inbox.errs = map[string]error{"events": fmt.Errorf("%w: status 401", inbox.ErrUnauthorized)}
	h.inbox.mu.Unlock()
	h.clock.Advance(time.Minute)
	h.tick()
	h.inbox.mu.Lock()
	h.inbox.errs = map[string]error{"events": fmt.Errorf("%w: refused", inbox.ErrTransport)}
	h.inbox.mu.Unlock()
	h.clock.Advance(time.Minute)
	h.tick()
	assert.Equal(t, 1, h.metricLabelled(MetricInboxErrors, "error_class", errClassUnauthorized))
	assert.Equal(t, 1, h.metricLabelled(MetricInboxErrors, "error_class", errClassTransport))
	assert.Equal(t, 2, h.metrics.Count(MetricInboxErrors))
}

func TestLog_should_EmitActorClassAndNeverALoginOrText_When_HumanFeedbackEventsAreSteeredDroppedOrHeld(t *testing.T) {
	const login, body = "SENTINEL-login-ab12", "SENTINEL-body-cd34"
	h := newLoopHarness(t, func(h *loopHarness) {
		h.steerer.outcome = map[string]Outcome{}
	})
	mk := func(id, kind, actor string) inbox.Item {
		it := dItem(id, kind, "created", actor, `{"summary":"`+body+`","body":"`+body+`","title":"`+body+`"}`)
		it.Event.Summary = body
		return it
	}
	// steered (other), dropped (self), held (busy).
	h.inbox.items = []inbox.Item{mk("i1", "comment", login), mk("i2", "comment", "me")}
	h.start(nil)
	h.tick()
	h.steerer.outcome[dSession] = OutcomeBusy
	h.inbox.mu.Lock()
	h.inbox.items = append(h.inbox.items, mk("i3", "review_comment", login+"-2"))
	h.inbox.mu.Unlock()
	h.tick()

	for msg, class := range map[string]string{"prwatch.steer": string(ActorOther), "prwatch.drop": string(ActorSelf), "prwatch.held": string(ActorOther)} {
		lines := h.linesNamed(msg)
		require.Len(t, lines, 1, msg)
		got, ok := logKV(lines[0], "actor_class")
		require.True(t, ok, msg)
		assert.Equal(t, class, got, msg)
	}
	for _, s := range []string{login, "me\n", body} {
		assert.NotContains(t, h.logText(), s)
		assert.NotContains(t, h.metricText(), s)
	}
	assert.NotContains(t, h.metricText(), login)
}

func TestLog_should_EmitStructuredSteerAndHeldLinesWithoutPRText_When_SteerAndHoldHappen(t *testing.T) {
	const hostile = "SENTINEL-hostile-ef56"
	h := newLoopHarness(t, nil)
	it := comment("i1")
	it.Event.Summary = hostile
	it.Event.Actor = hostile
	held := comment("i2")
	held.Event.PullRequest = 8
	held.Event.Summary = hostile
	h.sessions.programs["s2"] = "claude"
	h.sessions.Set(
		SessionRef{UUID: dSession, Repo: dRepo, PR: dPR, State: StateLive},
		SessionRef{UUID: "s2", Repo: dRepo, PR: 8, State: StateLive},
	)
	h.steerer.outcome = map[string]Outcome{"s2": OutcomeBusy}
	h.inbox.items = []inbox.Item{it, held}
	h.start(nil)
	h.tick()
	h.tick()
	h.tick()

	steer := h.linesNamed("prwatch.steer")
	require.Len(t, steer, 1)
	for _, k := range []string{"session", "repo", "pr", "kind", "outcome"} {
		_, ok := logKV(steer[0], k)
		assert.True(t, ok, "steer line has %s", k)
	}
	heldLines := h.linesNamed("prwatch.held")
	require.Len(t, heldLines, 1, "held logged on first hold only")
	for _, k := range []string{"session", "repo", "pr", "kind", "outcome"} {
		_, ok := logKV(heldLines[0], k)
		assert.True(t, ok, "held line has %s", k)
	}
	assert.NotContains(t, h.logText(), hostile)
}

func TestLog_should_EmitItemToSteerSecondsOnEveryDeliveredSteer_When_SteersAreDelivered(t *testing.T) {
	h := newLoopHarness(t, nil)
	older := comment("i1")
	older.CreatedAt = h.clock.Now().Add(-9 * time.Second)
	newer := dItem("i2", "comment", "created", "bob", "")
	newer.CreatedAt = h.clock.Now().Add(-4 * time.Second)
	h.inbox.items = []inbox.Item{newer}
	h.start(nil)
	h.tick()
	lines := h.linesNamed("prwatch.steer")
	require.Len(t, lines, 1)
	v, ok := logKV(lines[0], "item_to_steer_seconds")
	require.True(t, ok)
	assert.EqualValues(t, 4, v)

	// A folded steer logs the oldest folded item's delta.
	h2 := newLoopHarness(t, nil)
	older.CreatedAt = h2.clock.Now().Add(-9 * time.Second)
	newer.CreatedAt = h2.clock.Now().Add(-4 * time.Second)
	h2.inbox.items = []inbox.Item{newer, older}
	h2.start(nil)
	h2.tick()
	lines = h2.linesNamed("prwatch.steer")
	require.Len(t, lines, 1)
	v, _ = logKV(lines[0], "item_to_steer_seconds")
	assert.EqualValues(t, 9, v)
}

func TestLog_should_ClassifyAlt4_When_CommentInlineReviewAndCISteersAreDelivered(t *testing.T) {
	cases := []struct {
		name string
		item inbox.Item
		age  time.Duration
		want Alt4Class
	}{
		{"comment", comment("i"), time.Second, Alt4TextEvent},
		{"inline_comment", dItem("i", "review_comment", "created", "alice", ""), time.Second, Alt4TextEvent},
		{"commented_review", dItem("i", "review", "submitted", "alice", `{"state":"commented"}`), time.Second, Alt4TextEvent},
		{"ci_fast", ciFailure("i", "sha1"), 10 * time.Second, Alt4CISubminute},
		{"ci_slow", ciFailure("i", "sha1"), 30 * time.Second, Alt4CIOther},
		{"changes_requested", dItem("i", "review", "submitted", "alice", `{"state":"changes_requested"}`), time.Second, Alt4CIOther},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newLoopHarness(t, nil)
			tc.item.CreatedAt = h.clock.Now().Add(-tc.age)
			h.inbox.items = []inbox.Item{tc.item}
			h.start(nil)
			h.tick()
			lines := h.linesNamed("prwatch.steer")
			require.Len(t, lines, 1)
			got, ok := logKV(lines[0], "alt4_class")
			require.True(t, ok)
			assert.Equal(t, string(tc.want), got)
		})
	}
}

func TestPoller_should_ExcludeCatchUpSteersFromLatencyAndAlt4_When_CatchUpDelivers(t *testing.T) {
	h := newLoopHarness(t, nil)
	snaps := &fakePRSnapshot{snapshots: map[string]PRSnapshot{
		dSession: {CheckConclusion: "failure", CheckedAt: h.clock.Now()},
	}}
	h.start(func(d *LoopDeps) {
		d.Catchup = NewCatchup(CatchupDeps{
			Snapshots: snaps, Steerer: h.steerer, Coalescer: h.coalescer,
			Metrics: h.metrics, Log: h.log, Settings: h.settings,
		})
	})
	h.tick()
	assert.Equal(t, 1, h.metrics.Count(MetricCatchupSteers))
	assert.Zero(t, h.metrics.Count(MetricSteers))
	assert.NotContains(t, h.logText(), "alt4_class")
	assert.NotContains(t, h.logText(), "item_to_steer_seconds")
}

func TestPoller_should_ApplySelfActorFilter_When_BuiltThroughNewPoller(t *testing.T) {
	h := newLoopHarness(t, nil)
	h.inbox.items = []inbox.Item{dItem("i1", "comment", "created", "me", "")}
	h.start(nil)
	h.tick()
	assert.Empty(t, h.steerer.Requests())
	assert.Equal(t, []string{"i1"}, ackedIDs(h))
	assert.Equal(t, 1, h.metricLabelled(MetricDropped, "drop_reason", string(DropSelfActor)))
}
