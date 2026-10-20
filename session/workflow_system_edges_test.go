package session

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/domain"
)

func edgeKey(from, to BacklogStatus) string { return fmt.Sprintf("%s->%s", from, to) }

func TestSystemRequiredEdges_IsSubsetOfValidTransitionsPlusArchiveEdges_AndContainsAutomationEdges(t *testing.T) {
	t.Parallel()
	valid := domain.ValidTransitions()
	archiveGroup := map[string]bool{edgeKey(BacklogStatusArchived, BacklogStatusArchived): true}
	for _, s := range builtInStageOrder {
		archiveGroup[edgeKey(s, BacklogStatusArchived)] = true
	}
	for _, e := range systemRequiredOffGraph {
		archiveGroup[edgeKey(e.From, e.To)] = true
	}

	got := map[string]bool{}
	for _, e := range SystemRequiredEdges() {
		k := edgeKey(e.From, e.To)
		got[k] = true
		require.Truef(t, valid[e.From][e.To] || archiveGroup[k], "%s is neither a valid transition nor in the archive group", k)
		require.Truef(t, IsSystemRequired(e.From, e.To), "%s enumerated but IsSystemRequired is false", k)
	}

	for _, want := range []BacklogStatus{BacklogStatusIdea, BacklogStatusRefining, BacklogStatusReady, BacklogStatusQueued} {
		require.Truef(t, got[edgeKey(want, BacklogStatusArchived)], "SyncLoop edge %s->archived", want)
	}
	for _, k := range []string{
		edgeKey(BacklogStatusReady, BacklogStatusQueued),
		edgeKey(BacklogStatusQueued, BacklogStatusInProgress),
		edgeKey(BacklogStatusReady, BacklogStatusInProgress),
		edgeKey(BacklogStatusInProgress, BacklogStatusArchived),
		edgeKey(BacklogStatusArchived, BacklogStatusArchived),
	} {
		require.Truef(t, got[k], "missing %s", k)
	}
	require.False(t, got[edgeKey(BacklogStatusDone, BacklogStatusRefining)], "the generic MCP delegate contributes no system edge")
	require.False(t, IsSystemRequired(BacklogStatusDone, BacklogStatusRefining))

	require.True(t, IsSystemRequired("custom-stage", BacklogStatusArchived), "custom-slug to=archived")
	require.False(t, IsSystemRequired("custom-stage", BacklogStatusReady))
}

func TestSystemRequiredTargets_ExcludesArchiveGroup(t *testing.T) {
	t.Parallel()
	for _, from := range builtInStageOrder {
		require.NotContains(t, systemRequiredTargets(from), BacklogStatusArchived, from)
	}
	require.ElementsMatch(t, []BacklogStatus{BacklogStatusReview, BacklogStatusReady}, systemRequiredTargets(BacklogStatusInProgress))
}
