package backlog

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tstapler/stapler-squad/session"
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
	var got []string
	for _, d := range (&BacklogLifecycleListener{}).defaultStuckDetectors() {
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

type recordingDetector struct{}

func (recordingDetector) Name() string                                { return "extra" }
func (recordingDetector) Run(context.Context, *session.EntRepository) {}

func TestAppendStuckDetectors_RunsAfterDefaults(t *testing.T) {
	t.Parallel()
	l := &BacklogLifecycleListener{}
	l.AppendStuckDetectors(recordingDetector{})
	ds := l.stuckDetectors()
	assert.Equal(t, "extra", ds[len(ds)-1].Name())
	assert.Len(t, ds, len(l.defaultStuckDetectors())+1)
}
