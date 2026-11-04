package session

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/domain"
)

// Task 1.5.1c: the archive (and off-graph recovery) edges join the system-required
// set without touching validTransitions, the seed or the UI-facing allowed list.
// The empty-cache and nil-Configured fallback is pinned by
// TestSwitchableWorkflowEngine_FallsBackToDefaultGraph_WhenConfiguredCacheEmptyOrNil.
func TestSystemRequiredEdges_ArchiveAndRecoveryGroupsStayOutOfTableSeedAndAllowedList(t *testing.T) {
	t.Parallel()
	valid := domain.ValidTransitions()
	h := newSeededSelectorHarness(t, ModeEnforce)

	for _, from := range []BacklogStatus{BacklogStatusInProgress, BacklogStatusReview, BacklogStatusPRPending} {
		require.Falsef(t, valid[from][BacklogStatusArchived], "validTransitions must not gain %s->archived", from)
	}
	require.False(t, valid[BacklogStatusArchived][BacklogStatusArchived])

	// The seeded database (create-if-count-zero, never re-run) lacks these edges,
	// so this is the stale-seed case: the union alone must let archive through.
	for _, from := range []BacklogStatus{BacklogStatusInProgress, BacklogStatusReview, BacklogStatusPRPending, BacklogStatusDone, BacklogStatusArchived} {
		require.Truef(t, h.sel.CanTransition(from, BacklogStatusArchived, nil), "stale seed: %s->archived", from)
	}
	for _, from := range []BacklogStatus{BacklogStatusInProgress, BacklogStatusReview, BacklogStatusPRPending} {
		require.NotContainsf(t, h.sel.AllowedTransitions(from, nil), BacklogStatusArchived, "%s allowed list", from)
	}
	for _, e := range systemRequiredOffGraph {
		require.Falsef(t, valid[e.From][e.To], "%s->%s is off-graph by definition", e.From, e.To)
		require.Truef(t, h.sel.CanTransition(e.From, e.To, nil), "stale seed: %s->%s", e.From, e.To)
		require.NotContainsf(t, h.sel.AllowedTransitions(e.From, nil), e.To, "%s allowed list", e.From)
	}
}
