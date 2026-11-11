package backlog

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultStuckDetectors_Order(t *testing.T) {
	t.Parallel()
	want := []string{
		"transition_denied_redrive", "work_commit_refresh", "stale_work",
		"rework_blocked_stale", "respawn_blocked_active", "abandoned_review",
		"orphaned_agent_pr", "unprocessed_review_verdict", "bouncing",
		"repeated_noop_dispatch", "push_failed", "orphaned_triage",
		"orphaned_triage_remediation", "gate_timeout", "plan_not_approved",
		"self_heal", "multi_reason_escalation", "auto_archive_done",
		"archive_terminal_sessions", "cleanup_terminal_worktrees",
		"pr_drift_recovery", "pr_pending_no_pr", "pr_ready+merge_detection",
		"blocked_by_dependency", "dequeue_backlog_items", "trigger_chain_reconcile",
	}
	detectors := (&BacklogLifecycleListener{}).defaultStuckDetectors()
	got := make([]string, 0, len(detectors))
	for _, d := range detectors {
		got = append(got, d.Name())
	}
	assert.Equal(t, want, got)

	idx := map[string]int{}
	for i, n := range got {
		idx[n] = i
	}
	// BUG-047: fresh commit SHAs must precede merge detection's supersede check.
	assert.Equal(t, 0, idx["transition_denied_redrive"])
	assert.Equal(t, 1, idx["work_commit_refresh"])
	assert.Less(t, idx["work_commit_refresh"], idx["pr_ready+merge_detection"])
}
