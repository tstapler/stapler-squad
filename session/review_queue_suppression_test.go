package session

import (
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

var suppressionNow = time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)

func noExisting() (*ReviewItem, bool) { return nil, false }

func existingItem(p Priority) func() (*ReviewItem, bool) {
	return func() (*ReviewItem, bool) { return &ReviewItem{Priority: p}, true }
}

func addCtx() ruleCtx {
	return ruleCtx{action: DetectionActionAdd, priority: PriorityMedium, reason: ReasonInputRequired,
		controllerActive: true, now: suppressionNow, existing: noExisting}
}

func TestSuppressionPolicy_Should_EnforceRuleOrder_When_TwoRulesWouldFire(t *testing.T) {
	t.Parallel()

	var names []string
	for _, r := range suppressionRules {
		names = append(names, r.name)
	}
	// PR B inserts episodeAck after readdLimit and extends this list.
	if want := []string{"ackSnooze", "grace", "readdLimit"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("rule order = %v, want %v", names, want)
	}

	both := func(mut func(*ruleCtx)) ruleCtx {
		c := addCtx()
		c.priority = PriorityLow
		mut(&c)
		return c
	}
	tests := []struct {
		name string
		ctx  ruleCtx
		want SuppressionDecision
	}{
		{"ackSnooze_beats_grace", both(func(c *ruleCtx) {
			c.ackedAfterOutput, c.lastAcknowledged = true, suppressionNow.Add(-time.Minute)
		}), removeDecision("ackSnooze")},
		{"grace_beats_readdLimit", both(func(c *ruleCtx) {
			c.lastAcknowledged = suppressionNow.Add(-time.Minute)
			c.lastAddedToQueue, c.existing = suppressionNow.Add(-time.Second), existingItem(PriorityLow)
		}), removeDecision("grace")},
		{"readdLimit_decides_alone", both(func(c *ruleCtx) {
			c.lastAddedToQueue, c.existing = suppressionNow.Add(-time.Second), existingItem(PriorityLow)
		}), suppressDecision("readdLimit")},
		{"no_rule_fires_keeps", addCtx(), keepDecision()},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := evaluateRules(suppressionRules, tc.ctx); got != tc.want {
				t.Errorf("decision = %+v, want %+v", got, tc.want)
			}
			if tc.want.kind == suppressionKeep {
				return
			}
			reversed := []suppressionRule{}
			for i := len(suppressionRules) - 1; i >= 0; i-- {
				reversed = append(reversed, suppressionRules[i])
			}
			if tc.name != "readdLimit_decides_alone" && evaluateRules(reversed, tc.ctx) == tc.want {
				t.Errorf("reversing the rules did not change the decision; order is not exercised")
			}
		})
	}
}

func TestAckSnooze_Should_FollowCharacterizedBehavior_When_BypassFixOff(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		action       DetectionAction
		priority     Priority
		acked        bool
		wantDecided  bool
		wantDecision SuppressionDecision
	}{
		{"low_add_acked_is_removed", DetectionActionAdd, PriorityLow, true, true, removeDecision("ackSnooze")},
		{"skip_acked_is_removed", DetectionActionSkip, PriorityLow, true, true, removeDecision("ackSnooze")},
		{"high_add_acked_bypasses", DetectionActionAdd, PriorityHigh, true, false, SuppressionDecision{}},
		{"urgent_add_acked_bypasses", DetectionActionAdd, PriorityUrgent, true, false, SuppressionDecision{}},
		{"low_add_not_acked_defers", DetectionActionAdd, PriorityLow, false, false, SuppressionDecision{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := addCtx()
			c.action, c.priority, c.ackedAfterOutput = tc.action, tc.priority, tc.acked
			got, decided := ackSnooze(c)
			if decided != tc.wantDecided || got != tc.wantDecision {
				t.Errorf("ackSnooze = (%+v, %v), want (%+v, %v)", got, decided, tc.wantDecision, tc.wantDecided)
			}
		})
	}
}

func TestGrace_Should_NotRemoveItem_When_NewEpisodeWithin5MinutesOfAck(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		action     DetectionAction
		priority   Priority
		controller bool
		ackAgo     time.Duration
		episodeNew bool
		wantRemove bool
	}{
		{"same_episode_low_within_grace_removes", DetectionActionAdd, PriorityLow, true, time.Minute, false, true},
		{"new_episode_low_within_grace_keeps", DetectionActionAdd, PriorityLow, true, time.Minute, true, false},
		{"no_controller_high_within_grace_removes", DetectionActionAdd, PriorityHigh, false, time.Minute, false, true},
		{"controller_high_ignores_grace", DetectionActionAdd, PriorityHigh, true, time.Minute, false, false},
		{"after_grace_defers", DetectionActionAdd, PriorityLow, true, 10 * time.Minute, false, false},
		{"never_acked_defers", DetectionActionAdd, PriorityLow, true, 0, false, false},
		{"skip_never_removes", DetectionActionSkip, PriorityLow, true, time.Minute, false, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := addCtx()
			c.action, c.priority, c.controllerActive, c.episodeChanged = tc.action, tc.priority, tc.controller, tc.episodeNew
			if tc.ackAgo != 0 {
				c.lastAcknowledged = suppressionNow.Add(-tc.ackAgo)
			}
			got, decided := grace(c)
			if decided != tc.wantRemove || (decided && got != removeDecision("grace")) {
				t.Errorf("grace = (%+v, %v), want remove=%v", got, decided, tc.wantRemove)
			}
		})
	}
}

func TestReaddLimit_Should_NotSuppress_When_NewEpisodeWithin2Minutes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		priority     Priority
		existing     func() (*ReviewItem, bool)
		addedAgo     time.Duration
		episodeNew   bool
		wantSuppress bool
	}{
		{"same_episode_same_priority_suppresses", PriorityMedium, existingItem(PriorityMedium), time.Second, false, true},
		{"new_episode_same_priority_passes", PriorityMedium, existingItem(PriorityMedium), time.Second, true, false},
		{"priority_escalation_passes", PriorityHigh, existingItem(PriorityMedium), time.Second, false, false},
		{"not_queued_passes", PriorityMedium, noExisting, time.Second, false, false},
		{"outside_interval_passes", PriorityMedium, existingItem(PriorityMedium), 3 * time.Minute, false, false},
		{"never_added_passes", PriorityMedium, existingItem(PriorityMedium), 0, false, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := addCtx()
			c.priority, c.existing, c.episodeChanged = tc.priority, tc.existing, tc.episodeNew
			if tc.addedAgo != 0 {
				c.lastAddedToQueue = suppressionNow.Add(-tc.addedAgo)
			}
			got, decided := readdLimit(c)
			if decided != tc.wantSuppress || (decided && got != suppressDecision("readdLimit")) {
				t.Errorf("readdLimit = (%+v, %v), want suppress=%v", got, decided, tc.wantSuppress)
			}
		})
	}
}

// pollerFor builds a poller whose determiner returns result for a started "demo" session.
func pollerFor(result DetectionResult, mutate func(*Instance)) (*ReviewQueuePoller, *Instance) {
	now := time.Now()
	inst := &Instance{Title: "demo", UUID: "uuid-demo", Status: Running}
	inst.started.Store(true)
	inst.UpdatedAt = now
	mutate(inst)
	inst.SyncAtomicTimestamps()
	poller := NewReviewQueuePoller(NewReviewQueue(), newFakeStatusProvider(map[string]InstanceStatusInfo{
		"demo": {IsControllerActive: true},
	}), nil)
	poller.statusDeterminer = stubDeterminer{result: result}
	poller.contentProvider = newFakeContentProvider(map[string]string{})
	return poller, inst
}

type lineRecorder struct {
	mu    sync.Mutex
	lines []string
}

func (r *lineRecorder) log(msg string, args ...any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lines = append(r.lines, logLine(msg, args))
}

// logLine renders a structured log call as "msg k=v k=v".
func logLine(msg string, kv []any) string {
	var b strings.Builder
	b.WriteString(msg)
	for i := 0; i+1 < len(kv); i += 2 {
		fmt.Fprintf(&b, " %v=%v", kv[i], kv[i+1])
	}
	return b.String()
}

func TestSuppressionLogging_Should_EmitOneDebugLine_When_GraceSuppressesThreeConsecutivePolls(t *testing.T) {
	t.Parallel()
	now := time.Now()
	poller, inst := pollerFor(chainAdd(PriorityLow, ReasonIdle), func(i *Instance) {
		i.LastMeaningfulOutput, i.LastAcknowledged = now.Add(-10*time.Second), now.Add(-time.Minute)
	})
	rec := &lineRecorder{}
	poller.explain.debug = rec.log

	for range 3 {
		poller.checkSession(inst, nil)
	}

	if want := []string{"suppressed session=demo rule=grace"}; !reflect.DeepEqual(rec.lines, want) {
		t.Errorf("debug lines = %q, want %q", rec.lines, want)
	}
}

func TestSuppressionLogging_Should_EmitAgainOnDecisionChange_When_RuleChanges(t *testing.T) {
	t.Parallel()
	e := &suppressionExplainer{}
	rec := &lineRecorder{}
	e.debug = rec.log

	e.noteDecision("demo", removeDecision("grace"))
	e.noteDecision("demo", removeDecision("grace"))
	e.noteDecision("demo", suppressDecision("readdLimit"))
	e.noteDecision("demo", keepDecision())
	e.noteDecision("demo", removeDecision("grace"))

	want := []string{
		"suppressed session=demo rule=grace",
		"suppressed session=demo rule=readdLimit",
		"suppressed session=demo rule=grace",
	}
	if !reflect.DeepEqual(rec.lines, want) {
		t.Errorf("debug lines = %q, want %q", rec.lines, want)
	}
}

func ackedCtx(p Priority, r AttentionReason, bypassFix bool) ruleCtx {
	c := addCtx()
	c.priority, c.reason, c.ackedAfterOutput = p, r, true
	c.opts = RuleOptions{AckBypassFix: bypassFix}
	return c
}

func TestAckSnooze_Should_KeepInputRequired_When_BypassFixOnAndAckAfterOutput(t *testing.T) {
	t.Parallel()
	for _, reason := range NeedsHumanReasons() {
		if _, decided := ackSnooze(ackedCtx(PriorityMedium, reason, true)); decided {
			t.Errorf("reason %v at Medium was snoozed with the bypass fix on", reason)
		}
	}
	if _, decided := ackSnooze(ackedCtx(PriorityMedium, ReasonWaitingForUser, true)); !decided {
		t.Error("a non-needs-human reason must still be snoozed with the bypass fix on")
	}
}

func TestAckSnooze_Should_RemoveInputRequired_When_BypassFixOff(t *testing.T) {
	t.Parallel()
	got, decided := ackSnooze(ackedCtx(PriorityMedium, ReasonInputRequired, false))
	if !decided || got != removeDecision("ackSnooze") {
		t.Errorf("ackSnooze = (%+v, %v), want remove by ackSnooze", got, decided)
	}
}

func TestAckSnooze_Should_KeepApprovalPending_When_BypassFixOnOrOff(t *testing.T) {
	t.Parallel()
	for _, flag := range []bool{false, true} {
		for _, p := range []Priority{PriorityUrgent, PriorityHigh} {
			if _, decided := ackSnooze(ackedCtx(p, ReasonApprovalPending, flag)); decided {
				t.Errorf("approval at %v snoozed with bypassFix=%v", p, flag)
			}
		}
		if _, decided := ackSnooze(ackedCtx(PriorityUrgent, ReasonErrorState, flag)); decided {
			t.Errorf("error state snoozed with bypassFix=%v", flag)
		}
	}
}

// PR B appends ReasonProseQuestion to this expectation when the constant exists.
func TestNeedsHumanReasons_Should_ListBlockingReasons_When_Queried(t *testing.T) {
	t.Parallel()
	want := []AttentionReason{ReasonApprovalPending, ReasonInputRequired, ReasonErrorState}
	if got := NeedsHumanReasons(); !reflect.DeepEqual(got, want) {
		t.Errorf("NeedsHumanReasons = %v, want %v", got, want)
	}
}
