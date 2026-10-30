package session

import (
	"testing"
	"time"
)

func episodeCtx(reason AttentionReason, ep, acked EpisodeID) ruleCtx {
	c := addCtx()
	c.reason, c.episode, c.ackedEpisode = reason, ep, acked
	c.priority = reasonToPriority(reason)
	c.episodeChanged = ep != "" && ep != acked
	return c
}

func TestEpisodeAck_Should_DropOnlyTheAckedEpisode_When_ReasonIsEpisodeKeyed(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		ctx  ruleCtx
		want bool
	}{
		{"prose_acked_episode_drops", episodeCtx(ReasonProseQuestion, "E1", "E1"), true},
		{"awaiting_acked_episode_drops", episodeCtx(ReasonAwaitingDirection, "E1", "E1"), true},
		{"prose_new_episode_passes", episodeCtx(ReasonProseQuestion, "E2", "E1"), false},
		{"prose_never_acked_passes", episodeCtx(ReasonProseQuestion, "E1", ""), false},
		{"non_episode_reason_ignores_ack", episodeCtx(ReasonInputRequired, "E1", "E1"), false},
		{"no_episode_passes", episodeCtx(ReasonProseQuestion, "", ""), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, decided := episodeAck(tc.ctx)
			if decided != tc.want || (decided && got != removeDecision("episodeAck")) {
				t.Errorf("episodeAck = (%+v, %v), want drop=%v", got, decided, tc.want)
			}
		})
	}
}

func TestAckSnooze_Should_NeverSnoozeEpisodeReasons_When_BypassFixOff(t *testing.T) {
	t.Parallel()
	for _, r := range []AttentionReason{ReasonProseQuestion, ReasonAwaitingDirection} {
		if _, decided := ackSnooze(ackedCtx(reasonToPriority(r), r, false)); decided {
			t.Errorf("%v was snoozed by output-based ack with attention_ack_bypass_fix off", r)
		}
	}
}

func TestAckSnooze_Should_Expire_When_SnoozeOlderThanTTLAndBypassFixOn(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		ackedAgo   time.Duration
		bypassFix  bool
		wantRemove bool
	}{
		{"fresh_snooze_holds", time.Hour, true, true},
		{"just_under_ttl_holds", snoozeTTL - time.Second, true, true},
		{"at_ttl_expires", snoozeTTL, true, false},
		{"four_hours_ago_expires", 4 * time.Hour, true, false},
		{"flag_off_never_expires", 10 * time.Hour, false, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := ackedCtx(PriorityLow, ReasonStale, tc.bypassFix)
			c.lastAcknowledged = c.now.Add(-tc.ackedAgo)
			_, decided := ackSnooze(c)
			if decided != tc.wantRemove {
				t.Errorf("snooze removed = %v, want %v", decided, tc.wantRemove)
			}
		})
	}
}

func TestGrace_Should_PassNewEpisode_When_AckedWithinFiveMinutes(t *testing.T) {
	t.Parallel()
	c := episodeCtx(ReasonAwaitingDirection, "E2", "E1")
	c.controllerActive = false
	c.lastAcknowledged = c.now.Add(-time.Minute)
	if _, decided := grace(c); decided {
		t.Error("grace dropped a new episode shortly after an ack")
	}
	c.episodeChanged = false
	if _, decided := grace(c); !decided {
		t.Error("grace should still drop a non-episode re-add shortly after an ack")
	}
}

func snapshotAckedAt(title string, acked EpisodeID, ackedAt, outputAt time.Time) *InstanceSnapshot {
	snap := &InstanceSnapshot{Title: title, AckedEpisodeID: acked}
	snap.LastAcknowledged, snap.LastMeaningfulOutput = ackedAt, outputAt
	return snap
}

func TestSuppressionPolicy_Should_KeepAckAcrossSpinnerThenRaiseNewEpisode_When_E1AckedThenE2(t *testing.T) {
	t.Parallel()
	now := time.Now()
	queue := NewReviewQueue()
	add := func(ep EpisodeID) DetectionResult {
		return DetectionResult{Action: DetectionActionAdd, Reason: ReasonProseQuestion, Priority: PriorityMedium, Context: "q?", Episode: ep}
	}
	info := InstanceStatusInfo{IsControllerActive: true}
	// E1 acked; spinner output arrived afterwards (LastMeaningfulOutput > LastAcknowledged).
	snap := snapshotAckedAt("demo", "E1", now.Add(-time.Minute), now)

	if d := suppressionPolicy(snap, add("E1"), info, queue, RuleOptions{}); d != removeDecision("episodeAck") {
		t.Errorf("E1 after ack + spinner = %+v, want removed by episodeAck", d)
	}
	if d := suppressionPolicy(snap, add("E2"), info, queue, RuleOptions{}); d != keepDecision() {
		t.Errorf("E2 after E1 ack = %+v, want keep (new un-acked item)", d)
	}
}

func TestSuppressionPolicy_Should_LimitSameEpisodeButPassNewOne_When_QueuedWithinTwoMinutes(t *testing.T) {
	t.Parallel()
	now := time.Now()
	queue := NewReviewQueue()
	e1 := &ReviewItem{SessionID: "demo", SessionName: "demo", Reason: ReasonProseQuestion, Priority: PriorityMedium}
	setEpisodeMetadata(e1, "E1")
	queue.Add(e1)
	snap := &InstanceSnapshot{Title: "demo"}
	snap.LastAddedToQueue = now.Add(-time.Second)
	res := func(ep EpisodeID) DetectionResult {
		return DetectionResult{Action: DetectionActionAdd, Reason: ReasonProseQuestion, Priority: PriorityMedium, Episode: ep}
	}
	info := InstanceStatusInfo{IsControllerActive: true}
	if d := suppressionPolicy(snap, res("E1"), info, queue, RuleOptions{}); d != suppressDecision("readdLimit") {
		t.Errorf("same episode re-add = %+v, want readdLimit suppress", d)
	}
	if d := suppressionPolicy(snap, res("E2"), info, queue, RuleOptions{}); d != keepDecision() {
		t.Errorf("new episode re-add = %+v, want keep", d)
	}
}
