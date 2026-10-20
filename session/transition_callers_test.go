package session

import (
	"fmt"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/domain"
)

// expandEdge turns the AnyStatus wildcard into one edge per built-in status.
func expandEdge(e SystemEdge) []SystemEdge {
	if e.From != AnyStatus {
		return []SystemEdge{e}
	}
	out := make([]SystemEdge, 0, len(builtInStageOrder))
	for _, s := range builtInStageOrder {
		out = append(out, SystemEdge{From: s, To: e.To})
	}
	return out
}

func isKnownOffGraph(name TransitionCaller, e SystemEdge) bool {
	for _, k := range knownOffGraphCallerEdges {
		if k.Caller == name && k.Edge == e {
			return true
		}
	}
	return false
}

func sortedSetKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func TestTransitionCallers_HaveUniqueNamesValidFieldsAndNoRepeatedSite(t *testing.T) {
	t.Parallel()
	seen := map[TransitionCaller]bool{}
	for _, c := range TransitionCallers() {
		require.NotEmpty(t, c.Name)
		require.Falsef(t, seen[c.Name], "duplicate caller %s", c.Name)
		seen[c.Name] = true
		require.Containsf(t, []CallerClass{ClassOperator, ClassAgent, ClassSystem}, c.Class, "%s", c.Name)
		require.Containsf(t, []LegacyPolicy{LegacyEngine, LegacyUnchecked}, c.Legacy, "%s", c.Name)
		require.Containsf(t, []GuardPolicy{GuardsBuiltIn, GuardsNone}, c.Guards, "%s", c.Name)

		keys := map[string]bool{}
		for _, s := range append(append([]CallSite(nil), c.Sites...), c.Origins...) {
			k := s.File + " " + s.Func
			require.Falsef(t, keys[k], "%s lists %s twice", c.Name, k)
			keys[k] = true
		}
		for _, s := range c.Sites {
			require.Positivef(t, s.Calls, "%s site %s.%s", c.Name, s.File, s.Func)
		}
		if len(c.Sites) == 0 {
			require.NotEmptyf(t, c.Origins, "%s owns neither a primitive call site nor an origin", c.Name)
		}
	}
}

func TestTransitionCallerAudit_ForwardingIsExactlyTheMCPDelegateAndContributesNoEdge(t *testing.T) {
	t.Parallel()
	var forwarding []TransitionCaller
	for _, c := range TransitionCallers() {
		if c.Forwarding {
			forwarding = append(forwarding, c.Name)
			require.Emptyf(t, c.Edges, "forwarding %s declares no edge of its own", c.Name)
			require.Equal(t, ClassAgent, c.Class)
		}
	}
	require.Equal(t, []TransitionCaller{CallerMCPTransitionDelegate}, forwarding,
		"a second generic forwarder needs a deliberate registry change")
	require.False(t, IsSystemRequired(BacklogStatusDone, BacklogStatusRefining), "delegate-reachable edges stay disableable")
}

func TestTransitionCallerAudit_EveryAgentAndSystemEdgeIsSystemRequired(t *testing.T) {
	t.Parallel()
	for _, c := range TransitionCallers() {
		if c.Forwarding || c.Class == ClassOperator {
			continue
		}
		require.NotEmptyf(t, c.Edges, "%s", c.Name)
		for _, declared := range c.Edges {
			for _, e := range expandEdge(declared) {
				if isKnownOffGraph(c.Name, e) {
					continue
				}
				require.Truef(t, IsSystemRequired(e.From, e.To), "%s requests off-graph edge %s->%s that is not system-required", c.Name, e.From, e.To)
			}
		}
	}
}

func TestTransitionCallerAudit_OperatorEdgesAreInGraphOrSystemRequired(t *testing.T) {
	t.Parallel()
	valid := domain.ValidTransitions()
	for _, c := range TransitionCallers() {
		if c.Class != ClassOperator {
			continue
		}
		require.NotEmptyf(t, c.Edges, "%s", c.Name)
		for _, declared := range c.Edges {
			for _, e := range expandEdge(declared) {
				require.Truef(t, valid[e.From][e.To] || IsSystemRequired(e.From, e.To), "%s requests off-graph edge %s->%s", c.Name, e.From, e.To)
			}
		}
	}
}

func TestTransitionCallerAudit_NoSystemRequiredCoreEdgeIsOrphaned(t *testing.T) {
	t.Parallel()
	declared := map[string]bool{}
	for _, c := range TransitionCallers() {
		if c.Forwarding || c.Class == ClassOperator {
			continue
		}
		for _, d := range c.Edges {
			for _, e := range expandEdge(d) {
				declared[edgeKey(e.From, e.To)] = true
			}
		}
	}
	for _, group := range [][]SystemEdge{systemRequiredCore, systemRequiredOffGraph} {
		for _, e := range group {
			require.Truef(t, declared[edgeKey(e.From, e.To)], "%s->%s is system-required but no agent or system caller declares it", e.From, e.To)
		}
	}
}

// The off-graph decisions of Task 1.5.1b. A new off-graph edge fails here until
// it is either declared system-required (recorded in ADR-007) or fixed in the caller.
func TestTransitionCallerAudit_OffGraphEdgesAreExactlyTheDecidedOnes(t *testing.T) {
	t.Parallel()
	valid := domain.ValidTransitions()
	got := map[string]bool{}
	for _, c := range TransitionCallers() {
		for _, d := range c.Edges {
			for _, e := range expandEdge(d) {
				if !valid[e.From][e.To] && e.To != BacklogStatusArchived {
					got[fmt.Sprintf("%s: %s", c.Name, edgeKey(e.From, e.To))] = true
				}
			}
		}
	}
	want := map[string]bool{
		"mcp_request_review: in_progress->done":                 true,
		"lifecycle_session_exited: in_progress->done":           true,
		"lifecycle_push_create_pr_fallback: ready->done":        true,
		"lifecycle_recover_drifted_pr: idea->pr_pending":        true,
		"lifecycle_recover_drifted_pr: refining->pr_pending":    true,
		"lifecycle_recover_drifted_pr: ready->pr_pending":       true,
		"lifecycle_recover_drifted_pr: queued->pr_pending":      true,
		"lifecycle_recover_drifted_pr: in_progress->pr_pending": true,
		"mcp_report_blocked: review->review":                    true,
	}
	require.Equal(t, sortedSetKeys(want), sortedSetKeys(got))

	require.Len(t, KnownOffGraphCallerEdges(), 1, "tolerated caller bugs are pinned; fix the caller instead of growing this")
	k := KnownOffGraphCallerEdges()[0]
	require.False(t, IsSystemRequired(k.Edge.From, k.Edge.To), "a tolerated caller bug must not leak into the system-required set")
}

func TestTransitionCallerAudit_ArchiveWildcardOnlyTargetsArchived(t *testing.T) {
	t.Parallel()
	for _, c := range TransitionCallers() {
		for _, e := range c.Edges {
			if e.From == AnyStatus {
				require.Equalf(t, BacklogStatusArchived, e.To, "%s", c.Name)
			}
		}
	}
}

func TestTransitionCallerAudit_AutonomousCompleteEdgeSetIsIdeaToReady(t *testing.T) {
	t.Parallel()
	c, ok := LookupTransitionCaller(CallerAutonomousComplete)
	require.True(t, ok)
	require.Equal(t, []SystemEdge{{BacklogStatusIdea, BacklogStatusReady}}, c.Edges)
	require.Equal(t, ClassSystem, c.Class)
}

func TestTransitionCallers_ExemptSitesAreTheThreeRollbacksAndTwoDebugWrites(t *testing.T) {
	t.Parallel()
	counts := map[ExemptKind]int{}
	for _, s := range ExemptTransitionSites() {
		counts[s.Kind] += s.Calls
		require.NotEmpty(t, s.Reason)
	}
	require.Equal(t, map[ExemptKind]int{ExemptRollback: 3, ExemptDebug: 2}, counts)
}
