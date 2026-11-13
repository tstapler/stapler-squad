package session

import (
	"fmt"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/domain"
)

// expandEdge turns the AnyStatus wildcard into one edge per built-in status.
func expandEdge(e domain.SystemEdge) []domain.SystemEdge {
	if e.From != domain.AnyStatus {
		return []domain.SystemEdge{e}
	}
	out := make([]domain.SystemEdge, 0, len(BuiltInStageOrder))
	for _, s := range BuiltInStageOrder {
		out = append(out, domain.SystemEdge{From: s, To: e.To})
	}
	return out
}

func isKnownOffGraph(name domain.TransitionCaller, e domain.SystemEdge) bool {
	for _, k := range domain.KnownOffGraphCallerEdges() {
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
	seen := map[domain.TransitionCaller]bool{}
	for _, c := range domain.TransitionCallers() {
		require.NotEmpty(t, c.Name)
		require.Falsef(t, seen[c.Name], "duplicate caller %s", c.Name)
		seen[c.Name] = true
		require.Containsf(t, []domain.CallerClass{domain.ClassOperator, domain.ClassAgent, domain.ClassSystem}, c.Class, "%s", c.Name)
		require.Containsf(t, []domain.LegacyPolicy{domain.LegacyEngine, domain.LegacyUnchecked}, c.Legacy, "%s", c.Name)
		require.Containsf(t, []domain.GuardPolicy{domain.GuardsBuiltIn, domain.GuardsNone}, c.Guards, "%s", c.Name)

		keys := map[string]bool{}
		for _, s := range append(append([]domain.CallSite(nil), c.Sites...), c.Origins...) {
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
	var forwarding []domain.TransitionCaller
	for _, c := range domain.TransitionCallers() {
		if c.Forwarding {
			forwarding = append(forwarding, c.Name)
			require.Emptyf(t, c.Edges, "forwarding %s declares no edge of its own", c.Name)
			require.Equal(t, domain.ClassAgent, c.Class)
		}
	}
	require.Equal(t, []domain.TransitionCaller{domain.CallerMCPTransitionDelegate}, forwarding,
		"a second generic forwarder needs a deliberate registry change")
	require.False(t, IsSystemRequired(domain.BacklogStatusDone, domain.BacklogStatusRefining), "delegate-reachable edges stay disableable")
}

func TestTransitionCallerAudit_EveryAgentAndSystemEdgeIsSystemRequired(t *testing.T) {
	t.Parallel()
	for _, c := range domain.TransitionCallers() {
		if c.Forwarding || c.Class == domain.ClassOperator {
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
	for _, c := range domain.TransitionCallers() {
		if c.Class != domain.ClassOperator {
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
	for _, c := range domain.TransitionCallers() {
		if c.Forwarding || c.Class == domain.ClassOperator {
			continue
		}
		for _, d := range c.Edges {
			for _, e := range expandEdge(d) {
				declared[edgeKey(e.From, e.To)] = true
			}
		}
	}
	for _, group := range [][]domain.SystemEdge{systemRequiredCore, systemRequiredOffGraph} {
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
	for _, c := range domain.TransitionCallers() {
		for _, d := range c.Edges {
			for _, e := range expandEdge(d) {
				if !valid[e.From][e.To] && e.To != domain.BacklogStatusArchived {
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
	}
	require.Equal(t, sortedSetKeys(want), sortedSetKeys(got))

	require.Empty(t, domain.KnownOffGraphCallerEdges(), "tolerated caller bugs are pinned; fix the caller instead of growing this")
}

func TestTransitionCallerAudit_ArchiveWildcardOnlyTargetsArchived(t *testing.T) {
	t.Parallel()
	for _, c := range domain.TransitionCallers() {
		for _, e := range c.Edges {
			if e.From == domain.AnyStatus {
				require.Equalf(t, domain.BacklogStatusArchived, e.To, "%s", c.Name)
			}
		}
	}
}

func TestTransitionCallerAudit_AutonomousCompleteEdgeSetIsIdeaToReady(t *testing.T) {
	t.Parallel()
	c, ok := domain.LookupTransitionCaller(domain.CallerAutonomousComplete)
	require.True(t, ok)
	require.Equal(t, []domain.SystemEdge{{From: domain.BacklogStatusIdea, To: domain.BacklogStatusReady}}, c.Edges)
	require.Equal(t, domain.ClassSystem, c.Class)
}

func TestTransitionCallers_ExemptSitesAreTheThreeRollbacksAndTwoDebugWrites(t *testing.T) {
	t.Parallel()
	counts := map[domain.ExemptKind]int{}
	for _, s := range domain.ExemptTransitionSites() {
		counts[s.Kind] += s.Calls
		require.NotEmpty(t, s.Reason)
	}
	require.Equal(t, map[domain.ExemptKind]int{domain.ExemptRollback: 3, domain.ExemptDebug: 2}, counts)
}

// The strict bypass inventory (server/services, Story 1.5.9a) accepts the
// re-drive writer's computed Caller because it can only be one of these.
func TestRedriveByWrite_OnlyNamesRegisteredSystemCallers(t *testing.T) {
	require.NotEmpty(t, redriveByWrite)
	for caller := range redriveByWrite {
		spec, ok := domain.LookupTransitionCaller(caller)
		require.True(t, ok, "%s is not registered", caller)
		require.Equal(t, domain.ClassSystem, spec.Class, caller)
	}
}
