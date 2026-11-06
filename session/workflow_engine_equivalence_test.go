package session

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/domain"
)

// gateFixture is one guard scenario: an item input plus the transition it
// attempts. The corpus has one fixture per Err* sentinel TransitionGuard can
// return (session/domain/backlog.go), plus a passing control.
type gateFixture struct {
	name  string
	input BacklogItemTransitionInput
	to    BacklogStatus
	want  error // sentinel the Default engine must return; nil for the control
}

func oneAcCriterion() domain.AcCriteriaJSON {
	return domain.AcCriteriaJSON(`[{"id":"ac-1","text":"works","met":false}]`)
}

func gateFixtureCorpus() []gateFixture {
	return []gateFixture{
		{name: "ErrACRequired", input: BacklogItemTransitionInput{Status: BacklogStatusIdea}, to: BacklogStatusReady, want: domain.ErrACRequired},
		{name: "ErrUnresolvedBlockers", input: BacklogItemTransitionInput{Status: BacklogStatusReady, SkipPlanning: true, HasUnresolvedBlockers: true}, to: BacklogStatusInProgress, want: domain.ErrUnresolvedBlockers},
		{name: "ErrPlanRequired", input: BacklogItemTransitionInput{Status: BacklogStatusReady}, to: BacklogStatusInProgress, want: domain.ErrPlanRequired},
		{name: "ErrPlanArtifactsRequired", input: BacklogItemTransitionInput{Status: BacklogStatusReady, PlanApproved: true}, to: BacklogStatusInProgress, want: domain.ErrPlanArtifactsRequired},
		{name: "ErrPlanArtifactsMissing", input: BacklogItemTransitionInput{Status: BacklogStatusReady, PlanApproved: true, PlanArtifactsPath: "/gone", PlanArtifactsPathMissing: true}, to: BacklogStatusInProgress, want: domain.ErrPlanArtifactsMissing},
		{name: "ErrVerdictClearRequiredForReady", input: BacklogItemTransitionInput{Status: BacklogStatusReview, OverallOutcome: domain.ReviewOutcomePass}, to: BacklogStatusReady, want: domain.ErrVerdictClearRequiredForReady},
		{name: "ErrVerdictRequired", input: BacklogItemTransitionInput{Status: BacklogStatusReview}, to: BacklogStatusDone, want: domain.ErrVerdictRequired},
		{name: "ErrCodeNotOnMain", input: BacklogItemTransitionInput{Status: BacklogStatusReview, OverallOutcome: domain.ReviewOutcomePass, HasUnshippedCode: true}, to: BacklogStatusDone, want: domain.ErrCodeNotOnMain},
		{name: "control_no_guard_fires", input: BacklogItemTransitionInput{Status: BacklogStatusIdea, AcCriteria: oneAcCriterion()}, to: BacklogStatusReady, want: nil},
	}
}

// engineDiff separates edge-parity differences from gate differences so a
// caller can require the first to be empty while only reporting the second.
type engineDiff struct {
	Edges []string
	Gates []string
}

func allStatusPairs() [][2]BacklogStatus {
	pairs := make([][2]BacklogStatus, 0, len(builtInStageOrder)*len(builtInStageOrder))
	for _, from := range builtInStageOrder {
		for _, to := range builtInStageOrder {
			pairs = append(pairs, [2]BacklogStatus{from, to})
		}
	}
	return pairs
}

func unsatisfiedGateKeys(t *testing.T, e WorkflowEngine, in BacklogItemTransitionInput, to BacklogStatus) []string {
	t.Helper()
	statuses, err := e.PendingGates(in, to, nil)
	require.NoError(t, err)
	var keys []string
	for _, s := range statuses {
		if !s.Satisfied {
			keys = append(keys, fmt.Sprintf("%s/%s", s.GateID, s.Kind))
		}
	}
	sort.Strings(keys)
	return keys
}

// diffEngines compares underTest against reference over all 81 (from,to)
// pairs and the gate fixture corpus. Gate comparison uses GateID/Kind and
// errors.Is identity, never Description. It is the single harness Story 1.1.2
// reuses with the composed enforce engine as underTest.
func diffEngines(t *testing.T, reference, underTest WorkflowEngine, corpus []gateFixture) engineDiff {
	t.Helper()
	var d engineDiff
	for _, p := range allStatusPairs() {
		from, to := p[0], p[1]
		if r, u := reference.CanTransition(from, to, nil), underTest.CanTransition(from, to, nil); r != u {
			d.Edges = append(d.Edges, fmt.Sprintf("CanTransition(%s,%s): reference=%t underTest=%t", from, to, r, u))
		}
	}
	for _, from := range builtInStageOrder {
		if r, u := reference.AllowedTransitions(from, nil), underTest.AllowedTransitions(from, nil); fmt.Sprint(r) != fmt.Sprint(u) {
			d.Edges = append(d.Edges, fmt.Sprintf("AllowedTransitions(%s): reference=%v underTest=%v", from, r, u))
		}
	}
	for _, f := range corpus {
		rKeys := unsatisfiedGateKeys(t, reference, f.input, f.to)
		uKeys := unsatisfiedGateKeys(t, underTest, f.input, f.to)
		if fmt.Sprint(rKeys) != fmt.Sprint(uKeys) {
			d.Gates = append(d.Gates, fmt.Sprintf("%s PendingGates: reference=%v underTest=%v", f.name, rKeys, uKeys))
		}
		rErr, uErr := reference.ValidateGates(f.input, f.to, nil), underTest.ValidateGates(f.input, f.to, nil)
		if !errors.Is(uErr, rErr) || !errors.Is(rErr, uErr) {
			d.Gates = append(d.Gates, fmt.Sprintf("%s ValidateGates: reference=%v underTest=%v", f.name, rErr, uErr))
		}
	}
	return d
}

func TestEquivalenceHarness_PassesAndCorpusCoversEverySentinel_WhenBothSidesAreDefault(t *testing.T) {
	t.Parallel()
	def := NewDefaultWorkflowEngine()
	corpus := gateFixtureCorpus()

	require.Equal(t, engineDiff{}, diffEngines(t, def, NewDefaultWorkflowEngine(), corpus))

	for _, f := range corpus {
		got := def.ValidateGates(f.input, f.to, nil)
		if f.want == nil {
			require.NoError(t, got, f.name)
			continue
		}
		require.ErrorIs(t, got, f.want, f.name)
	}
}

func TestConfiguredWorkflowEngine_MatchesDefaultOn81Pairs_WhenSeeded(t *testing.T) {
	t.Parallel()
	configured, _ := newSeededConfiguredWorkflowEngine(t)

	diff := diffEngines(t, NewDefaultWorkflowEngine(), configured, nil)

	require.Empty(t, diff.Edges)
	require.Len(t, allStatusPairs(), 81)
}

// The raw ConfiguredWorkflowEngine carries no built-in guards (they live in
// DefaultWorkflowEngine); the composed enforce engine added by Story 1.1.2
// closes the gap. This test measures the gap and never fails on it.
func TestRawConfiguredGateGap_ReportedAsInformation_NotFailure(t *testing.T) {
	t.Parallel()
	configured, _ := newSeededConfiguredWorkflowEngine(t)
	def := NewDefaultWorkflowEngine()
	idea := BacklogItemTransitionInput{Status: BacklogStatusIdea}

	require.ErrorIs(t, def.ValidateGates(idea, BacklogStatusReady, nil), domain.ErrACRequired)
	pending, err := configured.PendingGates(idea, BacklogStatusReady, nil)
	require.NoError(t, err)
	require.Empty(t, pending, "raw Configured has no gate on idea->ready")

	diff := diffEngines(t, def, configured, gateFixtureCorpus())
	require.Empty(t, diff.Edges, "edge parity is a hard requirement; only gates may differ")
	require.NotEmpty(t, diff.Gates)
	for _, line := range diff.Gates {
		t.Logf("gate_builtin_absent_in_config: %s", line)
	}
}

// seededEdgeDiff returns "missing"/"extra" lines naming each edge by which
// side lacks it, comparing the live backlog_stages/stage_transitions rows to
// domain.ValidTransitions().
func seededEdgeDiff(ctx context.Context, t *testing.T, repo *EntRepository) []string {
	t.Helper()
	//nolint:entfullscan test assertion over a test-scoped in-memory DB.
	stages, err := repo.client.BacklogStage.Query().All(ctx)
	require.NoError(t, err)
	slugByID := make(map[string]string, len(stages))
	for _, s := range stages {
		slugByID[s.ID.String()] = s.Slug
	}
	//nolint:entfullscan test assertion over a test-scoped in-memory DB.
	rows, err := repo.client.StageTransition.Query().All(ctx)
	require.NoError(t, err)
	got := map[string]bool{}
	for _, r := range rows {
		got[slugByID[r.FromStageID.String()]+"->"+slugByID[r.ToStageID.String()]] = true
	}
	want := map[string]bool{}
	for from, targets := range domain.ValidTransitions() {
		for to := range targets {
			want[fmt.Sprintf("%s->%s", from, to)] = true
		}
	}
	var diffs []string
	for e := range want {
		if !got[e] {
			diffs = append(diffs, "missing "+e)
		}
	}
	for e := range got {
		if !want[e] {
			diffs = append(diffs, "extra "+e)
		}
	}
	sort.Strings(diffs)
	return diffs
}

func TestSeededStageConfig_ByteEquivalentToValidTransitions_WhenSeedApplied(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	ctx := context.Background()
	require.NoError(t, EnsureBuiltInWorkflowStages(ctx, repo.client))

	require.Empty(t, seededEdgeDiff(ctx, t, repo))
	// Ordering-insensitive hash equality: the fingerprint of the live rows is
	// the fingerprint of domain.ValidTransitions() and the seed tables.
	live, err := StageConfigFingerprint(ctx, repo.client)
	require.NoError(t, err)
	require.Equal(t, SeedFingerprint(), live)
}

func TestSeededStageConfig_DetectsDrift_WhenOneEdgeRemoved(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	ctx := context.Background()
	require.NoError(t, EnsureBuiltInWorkflowStages(ctx, repo.client))

	//nolint:entfullscan test assertion over a test-scoped in-memory DB.
	rows, err := repo.client.StageTransition.Query().All(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, rows)
	require.NoError(t, repo.client.StageTransition.DeleteOneID(rows[0].ID).Exec(ctx))

	diffs := seededEdgeDiff(ctx, t, repo)
	require.Len(t, diffs, 1)
	require.Contains(t, diffs[0], "missing ")
	require.Contains(t, diffs[0], "->", "the diff must name the removed edge")
}
