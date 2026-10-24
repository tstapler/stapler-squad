package session

import (
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/session/detection"
)

// stubDeterminer returns a fixed result so the post-Determine chain in checkSession can be
// pinned independently of terminal detection.
type stubDeterminer struct{ result DetectionResult }

func (s stubDeterminer) Determine(*Instance, string, InstanceStatusInfo, detection.TerminalDetector) DetectionResult {
	return s.result
}

// chainOutcome is what is left in the queue after one checkSession pass.
type chainOutcome struct {
	present  bool
	priority Priority
	reason   AttentionReason
}

var (
	chainGone = chainOutcome{}
	chainKept = func(p Priority, r AttentionReason) chainOutcome {
		return chainOutcome{present: true, priority: p, reason: r}
	}
)

// chainCase pins today's behavior of the gates that run after Determine (acknowledgment
// snooze, post-ack grace period, re-add spam limit). It documents, not endorses, the result.
type chainCase struct {
	name             string
	result           DetectionResult
	controllerActive bool
	// ack and addedAgo are offsets before "now"; zero means unset.
	ackAgo      time.Duration
	outputAgo   time.Duration
	addedAgo    time.Duration
	existing    *ReviewItem
	wantOutcome chainOutcome
}

func chainAdd(p Priority, r AttentionReason) DetectionResult {
	return DetectionResult{Action: DetectionActionAdd, Priority: p, Reason: r, Context: "ctx"}
}

func chainCases() []chainCase {
	const sec = time.Second
	prompt := &ReviewItem{SessionID: "s", Reason: ReasonApprovalPending, Priority: PriorityHigh, Context: "old"}
	low := &ReviewItem{SessionID: "s", Reason: ReasonIdle, Priority: PriorityLow, Context: "old"}
	med := &ReviewItem{SessionID: "s", Reason: ReasonInputRequired, Priority: PriorityMedium, Context: "ctx"}

	return []chainCase{
		// The :901 gap: Medium (input required) is not "active high priority", so an
		// acknowledgment newer than the last output snoozes a live input prompt.
		{name: "medium_add_ack_after_output_is_snoozed", result: chainAdd(PriorityMedium, ReasonInputRequired),
			controllerActive: true, outputAgo: 60 * sec, ackAgo: 30 * sec, wantOutcome: chainGone},
		{name: "high_add_ack_after_output_bypasses_snooze", result: chainAdd(PriorityHigh, ReasonApprovalPending),
			controllerActive: true, outputAgo: 60 * sec, ackAgo: 30 * sec,
			wantOutcome: chainKept(PriorityHigh, ReasonApprovalPending)},
		{name: "urgent_add_ack_after_output_bypasses_snooze", result: chainAdd(PriorityUrgent, ReasonErrorState),
			controllerActive: true, outputAgo: 60 * sec, ackAgo: 30 * sec,
			wantOutcome: chainKept(PriorityUrgent, ReasonErrorState)},
		{name: "low_add_ack_after_output_is_snoozed", result: chainAdd(PriorityLow, ReasonIdle),
			controllerActive: true, outputAgo: 60 * sec, ackAgo: 30 * sec, wantOutcome: chainGone},
		{name: "skip_with_ack_after_output_removes_existing_item", result: DetectionResult{Action: DetectionActionSkip},
			controllerActive: true, outputAgo: 60 * sec, ackAgo: 30 * sec, existing: low, wantOutcome: chainGone},
		{name: "skip_without_ack_leaves_existing_item", result: DetectionResult{Action: DetectionActionSkip},
			controllerActive: true, existing: low, wantOutcome: chainKept(PriorityLow, ReasonIdle)},

		// Grace period: ack before the last output, but under 5 minutes ago.
		{name: "low_add_within_grace_period_is_removed", result: chainAdd(PriorityLow, ReasonIdle),
			controllerActive: true, outputAgo: 10 * sec, ackAgo: 60 * sec, wantOutcome: chainGone},
		{name: "low_add_after_grace_period_is_added", result: chainAdd(PriorityLow, ReasonIdle),
			controllerActive: true, outputAgo: 10 * sec, ackAgo: 10 * time.Minute,
			wantOutcome: chainKept(PriorityLow, ReasonIdle)},
		{name: "high_add_without_controller_within_grace_period_is_removed", result: chainAdd(PriorityHigh, ReasonApprovalPending),
			controllerActive: false, outputAgo: 10 * sec, ackAgo: 60 * sec, wantOutcome: chainGone},
		{name: "high_add_with_controller_ignores_grace_period", result: chainAdd(PriorityHigh, ReasonApprovalPending),
			controllerActive: true, outputAgo: 10 * sec, ackAgo: 60 * sec,
			wantOutcome: chainKept(PriorityHigh, ReasonApprovalPending)},
		{name: "medium_add_with_controller_ignores_grace_period", result: chainAdd(PriorityMedium, ReasonInputRequired),
			controllerActive: true, outputAgo: 10 * sec, ackAgo: 60 * sec,
			wantOutcome: chainKept(PriorityMedium, ReasonInputRequired)},

		// Re-add spam limit: only applies when the item is already queued.
		{name: "readd_inside_interval_when_queued_is_dropped", result: chainAdd(PriorityMedium, ReasonInputRequired),
			controllerActive: true, addedAgo: 30 * sec, existing: &ReviewItem{SessionID: "s", Reason: ReasonWaitingForUser, Priority: PriorityMedium, Context: "kept"},
			wantOutcome: chainKept(PriorityMedium, ReasonWaitingForUser)},
		{name: "readd_inside_interval_with_escalation_replaces_item", result: chainAdd(PriorityHigh, ReasonApprovalPending),
			controllerActive: true, addedAgo: 30 * sec, existing: low,
			wantOutcome: chainKept(PriorityHigh, ReasonApprovalPending)},
		{name: "readd_inside_interval_when_not_queued_is_added", result: chainAdd(PriorityMedium, ReasonInputRequired),
			controllerActive: true, addedAgo: 30 * sec, wantOutcome: chainKept(PriorityMedium, ReasonInputRequired)},
		{name: "readd_outside_interval_updates_item", result: chainAdd(PriorityMedium, ReasonInputRequired),
			controllerActive: true, addedAgo: 5 * time.Minute, existing: low,
			wantOutcome: chainKept(PriorityMedium, ReasonInputRequired)},

		// Determiner Remove: prompt-reason items survive on the no-controller path only.
		{name: "remove_without_controller_keeps_prompt_item", result: DetectionResult{Action: DetectionActionRemove},
			controllerActive: false, existing: prompt, wantOutcome: chainKept(PriorityHigh, ReasonApprovalPending)},
		{name: "remove_with_controller_drops_prompt_item", result: DetectionResult{Action: DetectionActionRemove},
			controllerActive: true, existing: prompt, wantOutcome: chainGone},
		{name: "remove_without_controller_drops_non_prompt_item", result: DetectionResult{Action: DetectionActionRemove},
			controllerActive: false, existing: low, wantOutcome: chainGone},
		{name: "skip_leaves_medium_item", result: DetectionResult{Action: DetectionActionSkip},
			controllerActive: true, existing: med, wantOutcome: chainKept(PriorityMedium, ReasonInputRequired)},
	}
}

func TestCheckSession_Should_PreserveCharacterizedGateChain_When_RefactoredToSuppressionPolicy(t *testing.T) {
	t.Parallel()
	for _, tc := range chainCases() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			now := time.Now()
			inst := &Instance{Title: "s", UUID: "uuid-s", Status: Running}
			inst.started.Store(true)
			inst.UpdatedAt = now
			if tc.outputAgo != 0 {
				inst.LastMeaningfulOutput = now.Add(-tc.outputAgo)
			}
			if tc.ackAgo != 0 {
				inst.LastAcknowledged = now.Add(-tc.ackAgo)
			}
			if tc.addedAgo != 0 {
				inst.LastAddedToQueue = now.Add(-tc.addedAgo)
			}
			inst.SyncAtomicTimestamps()

			poller := NewReviewQueuePoller(NewReviewQueue(), newFakeStatusProvider(map[string]InstanceStatusInfo{
				"s": {IsControllerActive: tc.controllerActive},
			}), nil)
			poller.statusDeterminer = stubDeterminer{result: tc.result}
			poller.contentProvider = newFakeContentProvider(map[string]string{})
			if tc.existing != nil {
				seed := *tc.existing
				seed.SessionID, seed.SessionName = "s", "s"
				poller.queue.Add(&seed)
			}

			poller.checkSession(inst, nil)

			item, present := poller.queue.Get("s")
			got := chainOutcome{present: present}
			if present {
				got.priority, got.reason = item.Priority, item.Reason
			}
			if got != tc.wantOutcome {
				t.Errorf("queue after checkSession = %+v, want %+v", got, tc.wantOutcome)
			}
		})
	}
}
