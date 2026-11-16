package session

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/internal/sessiontest"
	"github.com/tstapler/stapler-squad/session/workflowengine"
)

// --- fakes ---------------------------------------------------------------

func newRecordingShadow() *recordingShadow {
	return &recordingShadow{calls: map[callKey]int{}, divergences: map[divergenceKey]int{}, notCompared: map[workflowengine.NotComparedReason]int{}}
}

type panickingShadow struct{ discardRecorder }

func (panickingShadow) RecordShadowCall(workflowengine.EngineMethod, workflowengine.CallPath) {
	panic("recorder boom")
}

// failIfCalledGateRepo is a GateSatisfactionRepository that fails the test on
// any read; the embedded nil interface panics on every other method.
type failIfCalledGateRepo struct {
	workflowengine.GateSatisfactionRepository
	t     *testing.T
	calls atomic.Int64
}

func (r *failIfCalledGateRepo) GetByItemAndGate(context.Context, uuid.UUID, uuid.UUID) (*workflowengine.GateSatisfactionData, error) {
	r.calls.Add(1)
	r.t.Errorf("GateSatisfactionRepository.GetByItemAndGate must not be called")
	return nil, errors.New("unexpected call")
}

type failingStageRepo struct {
	workflowengine.StageConfigRepository
}

func (failingStageRepo) ListEnabledStages(context.Context) ([]*ent.BacklogStage, error) {
	return nil, errors.New("db down")
}

// --- builders ------------------------------------------------------------

func newSelectorHarness(t *testing.T, mode workflowengine.EngineMode, cfg *workflowengine.ConfiguredWorkflowEngine, opts ...workflowengine.SelectorOption) *selectorHarness {
	t.Helper()
	h := &selectorHarness{cfg: cfg, recorder: newRecordingShadow(), warns: &lineSink{}, infos: &lineSink{}, clock: sessiontest.NewSelectorClock()}
	base := make([]workflowengine.SelectorOption, 0, 4+len(opts))
	base = append(base,
		workflowengine.WithShadowRecorder(h.recorder),
		workflowengine.WithSelectorClock(h.clock),
		workflowengine.WithSelectorLoggers(h.warns.logf, h.infos.logf),
		workflowengine.WithConfigLabeler(func() string { return "edited" }),
	)
	h.sel = workflowengine.NewSwitchableWorkflowEngine(workflowengine.NewDefaultWorkflowEngine(), cfg, fixedMode{mode}, append(base, opts...)...)
	return h
}

func newSeededSelectorHarness(t *testing.T, mode workflowengine.EngineMode, opts ...workflowengine.SelectorOption) *selectorHarness {
	t.Helper()
	cfg, client := newSeededConfiguredWorkflowEngine(t)
	h := newSelectorHarness(t, mode, cfg, opts...)
	h.client = client
	return h
}

// --- Off / Shadow / Enforce basics ---------------------------------------

func TestSwitchableWorkflowEngine_ReturnsDefaultAnswer_WhenModeOffOrShadow(t *testing.T) {
	t.Parallel()
	for _, mode := range []workflowengine.EngineMode{workflowengine.ModeOff, workflowengine.ModeShadow} {
		h := newSeededSelectorHarness(t, mode)
		setEdgeEnabled(t, h.client, h.cfg, domain.BacklogStatusDone, domain.BacklogStatusRefining, false)

		require.True(t, h.sel.CanTransition(domain.BacklogStatusDone, domain.BacklogStatusRefining, nil), "mode %d", mode)
		require.Contains(t, h.sel.AllowedTransitions(domain.BacklogStatusDone, nil), domain.BacklogStatusRefining)

		want := 0
		if mode == workflowengine.ModeShadow {
			want = 1
		}
		key := divergenceKey{workflowengine.MethodCanTransition, workflowengine.ClassGraph, "edited", workflowengine.PathTransition}
		require.Equal(t, want, h.recorder.divergences[key], "mode %d", mode)
	}
}

func TestSwitchableWorkflowEngine_EnforceHonorsOperatorEdit_WithoutRestart(t *testing.T) {
	t.Parallel()
	h := newSeededSelectorHarness(t, workflowengine.ModeEnforce)
	require.True(t, h.sel.CanTransition(domain.BacklogStatusDone, domain.BacklogStatusRefining, nil))

	setEdgeEnabled(t, h.client, h.cfg, domain.BacklogStatusDone, domain.BacklogStatusRefining, false)

	require.False(t, h.sel.CanTransition(domain.BacklogStatusDone, domain.BacklogStatusRefining, nil))
	require.True(t, h.sel.CanTransition(domain.BacklogStatusReady, domain.BacklogStatusInProgress, nil))
	require.NotContains(t, h.sel.AllowedTransitions(domain.BacklogStatusDone, nil), domain.BacklogStatusRefining)
}

func TestSwitchableWorkflowEngine_EnforceKeepsBuiltInGuards_WhenSeededConfigHasNoGateRows(t *testing.T) {
	t.Parallel()
	h := newSeededSelectorHarness(t, workflowengine.ModeEnforce)
	idea := domain.BacklogItemTransitionInput{Status: domain.BacklogStatusIdea}

	require.ErrorIs(t, h.sel.ValidateGates(idea, domain.BacklogStatusReady, nil), domain.ErrACRequired)
	pending, err := h.sel.PendingGates(idea, domain.BacklogStatusReady, nil)
	require.NoError(t, err)
	require.Len(t, pending, 1)
	require.False(t, pending[0].Satisfied)
}

func TestSwitchableWorkflowEngine_ShadowNeverDivergesOnOrdinaryGuardDenial_WhenSeeded(t *testing.T) {
	t.Parallel()
	h := newSeededSelectorHarness(t, workflowengine.ModeShadow)
	idea := domain.BacklogItemTransitionInput{Status: domain.BacklogStatusIdea}

	require.ErrorIs(t, h.sel.ValidateGates(idea, domain.BacklogStatusReady, nil), domain.ErrACRequired)

	require.Zero(t, h.recorder.totalDivergences())
	require.Equal(t, 1, h.recorder.calls[callKey{workflowengine.MethodValidateGates, workflowengine.PathTransition}])
}

func TestSwitchableWorkflowEngine_ShadowCountsGateExtra_WhenOperatorAddsStructuralGate(t *testing.T) {
	t.Parallel()
	h := newSeededSelectorHarness(t, workflowengine.ModeShadow)
	addGateToSeededEdge(t, h.client, h.cfg, domain.BacklogStatusReady, domain.BacklogStatusInProgress, domain.GateKindStructural,
		map[string]interface{}{"check_id": domain.StructuralCheckACComplete})
	item := domain.BacklogItemTransitionInput{Status: domain.BacklogStatusReady, SkipPlanning: true, PlanArtifactsPath: "/plan"}
	require.NoError(t, workflowengine.NewDefaultWorkflowEngine().ValidateGates(item, domain.BacklogStatusInProgress, nil), "fixture: Default must pass")

	require.NoError(t, h.sel.ValidateGates(item, domain.BacklogStatusInProgress, nil), "shadow returns the Default answer")

	require.Equal(t, 1, h.recorder.divergences[divergenceKey{workflowengine.MethodValidateGates, workflowengine.ClassGateExtra, "edited", workflowengine.PathTransition}])
}

// --- fallback --------------------------------------------------------------

func TestSwitchableWorkflowEngine_FallsBackToDefaultGraph_WhenConfiguredCacheEmptyOrNil(t *testing.T) {
	t.Parallel()
	emptyRepo := NewTestEntRepository(t)
	emptyCfg, err := workflowengine.NewConfiguredWorkflowEngine(NewEntStageConfigRepository(emptyRepo.Client), nil, nil)
	require.NoError(t, err)
	loadErrCfg, err := workflowengine.NewConfiguredWorkflowEngine(failingStageRepo{}, nil, nil)
	require.NoError(t, err)

	cases := []struct {
		name   string
		cfg    *workflowengine.ConfiguredWorkflowEngine
		reason workflowengine.FallbackReason
	}{
		{"cache_empty", emptyCfg, workflowengine.FallbackCacheEmpty},
		{"cfg_nil", nil, workflowengine.FallbackCfgNil},
		{"load_error", loadErrCfg, workflowengine.FallbackLoadError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newSelectorHarness(t, workflowengine.ModeEnforce, tc.cfg)
			calls := 0

			require.True(t, h.sel.CanTransition(domain.BacklogStatusIdea, domain.BacklogStatusReady, nil))
			calls++
			for _, from := range []domain.BacklogStatus{domain.BacklogStatusInProgress, domain.BacklogStatusReview, domain.BacklogStatusPRPending, domain.BacklogStatusArchived} {
				require.Truef(t, h.sel.CanTransition(from, domain.BacklogStatusArchived, nil), "%s->archived", from)
				calls++
			}
			require.NotContains(t, h.sel.AllowedTransitions(domain.BacklogStatusInProgress, nil), domain.BacklogStatusArchived)
			calls++
			require.False(t, h.sel.CanTransition(domain.BacklogStatusIdea, domain.BacklogStatusDone, nil), "fallback is not allow-all")
			calls++

			require.EqualValues(t, calls, h.sel.FallbackTotal(tc.reason))
			require.Len(t, h.warns.snapshot(), 1, "one WARN per minute")
			h.clock.Advance(61 * time.Second)
			h.sel.CanTransition(domain.BacklogStatusIdea, domain.BacklogStatusReady, nil)
			require.Len(t, h.warns.snapshot(), 2)
		})
	}
}

func TestSwitchableWorkflowEngine_UnionsSystemRequiredEdges_WhenLegacyDbDisablesInProgressReview(t *testing.T) {
	t.Parallel()
	h := newSeededSelectorHarness(t, workflowengine.ModeEnforce)
	setEdgeEnabled(t, h.client, h.cfg, domain.BacklogStatusInProgress, domain.BacklogStatusReview, false)

	require.True(t, h.sel.CanTransition(domain.BacklogStatusInProgress, domain.BacklogStatusReview, nil))
	require.Contains(t, h.sel.AllowedTransitions(domain.BacklogStatusInProgress, nil), domain.BacklogStatusReview)
	require.Zero(t, h.sel.FallbackTotal(workflowengine.FallbackCacheEmpty), "configured graph in use, not the fallback")
}

func TestSwitchableWorkflowEngine_TypedNilConfigured_AnswersFromDefault_WithoutPanic(t *testing.T) {
	t.Parallel()
	var cfg *workflowengine.ConfiguredWorkflowEngine
	h := newSelectorHarness(t, workflowengine.ModeEnforce, cfg)
	item := domain.BacklogItemTransitionInput{Status: domain.BacklogStatusIdea}

	require.NotPanics(t, func() {
		h.sel.CanTransition(domain.BacklogStatusIdea, domain.BacklogStatusReady, nil)
		h.sel.AllowedTransitions(domain.BacklogStatusIdea, nil)
		_, _ = h.sel.PendingGates(item, domain.BacklogStatusReady, nil)
		require.ErrorIs(t, h.sel.ValidateGates(item, domain.BacklogStatusReady, nil), domain.ErrACRequired)
	})
	require.EqualValues(t, 4, h.sel.FallbackTotal(workflowengine.FallbackCfgNil))
}

// --- override_reason -------------------------------------------------------

func TestSwitchableWorkflowEngine_BypassesGuardsNotGraph_WhenOverrideReasonNonEmpty(t *testing.T) {
	t.Parallel()

	t.Run("operator gate is bypassed by override but blocks without it", func(t *testing.T) {
		t.Parallel()
		h := newSeededSelectorHarness(t, workflowengine.ModeEnforce)
		addGateToSeededEdge(t, h.client, h.cfg, domain.BacklogStatusReview, domain.BacklogStatusDone, domain.GateKindHumanApproval, nil)

		item := passingReviewItem()
		require.ErrorIs(t, h.sel.ValidateGates(item, domain.BacklogStatusDone, nil), domain.ErrGateNotSatisfied)
		item.OverrideReason = "ship per owner"
		require.NoError(t, h.sel.ValidateGates(item, domain.BacklogStatusDone, nil))
	})

	t.Run("synthetic deleted-stage gate is bypassed by override", func(t *testing.T) {
		t.Parallel()
		h := newSeededSelectorHarness(t, workflowengine.ModeEnforce)
		snap := &domain.StageConfigSnapshot{StageName: "Gone", AllowedTransitions: []domain.BacklogStatus{domain.BacklogStatusReady}}
		item := domain.BacklogItemTransitionInput{Status: "gone-stage"}

		require.True(t, h.sel.CanTransition("gone-stage", domain.BacklogStatusReady, snap))
		require.ErrorIs(t, h.sel.ValidateGates(item, domain.BacklogStatusReady, snap), domain.ErrGateNotSatisfied)
		item.OverrideReason = "stage deleted, forcing"
		require.NoError(t, h.sel.ValidateGates(item, domain.BacklogStatusReady, snap))
	})

	t.Run("Default guards that ignore override still deny", func(t *testing.T) {
		t.Parallel()
		h := newSeededSelectorHarness(t, workflowengine.ModeEnforce)
		const reason = "please"
		cases := []struct {
			name string
			item domain.BacklogItemTransitionInput
			to   domain.BacklogStatus
			want error
		}{
			{"ErrACRequired", domain.BacklogItemTransitionInput{Status: domain.BacklogStatusIdea}, domain.BacklogStatusReady, domain.ErrACRequired},
			{"ErrPlanRequired", domain.BacklogItemTransitionInput{Status: domain.BacklogStatusReady}, domain.BacklogStatusInProgress, domain.ErrPlanRequired},
			{"ErrPlanArtifactsRequired", domain.BacklogItemTransitionInput{Status: domain.BacklogStatusReady, PlanApproved: true}, domain.BacklogStatusInProgress, domain.ErrPlanArtifactsRequired},
			{"ErrPlanArtifactsMissing", domain.BacklogItemTransitionInput{Status: domain.BacklogStatusReady, PlanApproved: true, PlanArtifactsPath: "/gone", PlanArtifactsPathMissing: true}, domain.BacklogStatusInProgress, domain.ErrPlanArtifactsMissing},
			{"ErrUnresolvedBlockers", domain.BacklogItemTransitionInput{Status: domain.BacklogStatusReady, SkipPlanning: true, HasUnresolvedBlockers: true}, domain.BacklogStatusInProgress, domain.ErrUnresolvedBlockers},
		}
		for _, tc := range cases {
			tc.item.OverrideReason = reason
			require.ErrorIsf(t, h.sel.ValidateGates(tc.item, tc.to, nil), tc.want, tc.name)
		}
	})

	t.Run("review to done without PASS succeeds under override exactly as under Default", func(t *testing.T) {
		t.Parallel()
		h := newSeededSelectorHarness(t, workflowengine.ModeEnforce)
		def := workflowengine.NewDefaultWorkflowEngine()
		item := domain.BacklogItemTransitionInput{Status: domain.BacklogStatusReview}

		require.ErrorIs(t, h.sel.ValidateGates(item, domain.BacklogStatusDone, nil), domain.ErrVerdictRequired)
		item.OverrideReason = "owner approved"
		require.NoError(t, def.ValidateGates(item, domain.BacklogStatusDone, nil))
		require.NoError(t, h.sel.ValidateGates(item, domain.BacklogStatusDone, nil))
	})

	t.Run("override never bypasses the graph", func(t *testing.T) {
		t.Parallel()
		h := newSeededSelectorHarness(t, workflowengine.ModeEnforce)
		require.False(t, h.sel.CanTransition(domain.BacklogStatusIdea, domain.BacklogStatusDone, nil))
		setEdgeEnabled(t, h.client, h.cfg, domain.BacklogStatusDone, domain.BacklogStatusRefining, false)
		require.False(t, h.sel.CanTransition(domain.BacklogStatusDone, domain.BacklogStatusRefining, nil))
	})
}

// --- attribution -----------------------------------------------------------

func TestSwitchableWorkflowEngine_LogsFromToReason_WhenTransitionDeniedByGraphOperatorGateOrDefaultGuard(t *testing.T) {
	t.Parallel()
	h := newSeededSelectorHarness(t, workflowengine.ModeEnforce)
	addGateToSeededEdge(t, h.client, h.cfg, domain.BacklogStatusReady, domain.BacklogStatusQueued, domain.GateKindHumanApproval, nil)

	require.False(t, h.sel.CanTransition(domain.BacklogStatusIdea, domain.BacklogStatusDone, nil))
	require.ErrorIs(t, h.sel.ValidateGates(domain.BacklogItemTransitionInput{Status: domain.BacklogStatusReady}, domain.BacklogStatusQueued, nil), domain.ErrGateNotSatisfied)
	require.ErrorIs(t, h.sel.ValidateGates(domain.BacklogItemTransitionInput{Status: domain.BacklogStatusReview}, domain.BacklogStatusDone, nil), domain.ErrVerdictRequired)

	lines := strings.Join(h.infos.snapshot(), "\n")
	require.Contains(t, lines, "from=idea to=done reason=config_graph")
	require.Contains(t, lines, "from=ready to=queued reason=operator_gate")
	require.Contains(t, lines, "from=review to=done reason=default_guard")
	require.EqualValues(t, 1, h.sel.EnforceDeniedTotal(workflowengine.DenialConfigGraph))
	require.EqualValues(t, 1, h.sel.EnforceDeniedTotal(workflowengine.DenialOperatorGate))
	require.EqualValues(t, 1, h.sel.EnforceDeniedTotal(workflowengine.DenialDefaultGuard))
}

// --- shadow safety ---------------------------------------------------------

func TestSwitchableWorkflowEngine_ShadowDoesNoStatefulGateIo_WhenStatefulGateOnEdge(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	require.NoError(t, workflowengine.EnsureBuiltInWorkflowStages(context.Background(), repo.Client))
	gateRepo := &failIfCalledGateRepo{t: t}
	cfg, err := workflowengine.NewConfiguredWorkflowEngine(NewEntStageConfigRepository(repo.Client), gateRepo, nil)
	require.NoError(t, err)
	from, to := newCustomTransitionWithGates(t, repo.Client, cfg,
		[]domain.GateKind{domain.GateKindHumanApproval, domain.GateKindAutomatedReview, domain.GateKindCustom}, []bool{true, true, true})
	h := newSelectorHarness(t, workflowengine.ModeShadow, cfg)
	item := domain.BacklogItemTransitionInput{Status: from, ItemID: uuid.NewString()}

	require.NoError(t, h.sel.ValidateGates(item, to, nil), "Default has no gate on a custom edge")

	require.Equal(t, 1, h.recorder.notCompared[workflowengine.NotComparedStatefulGate])
	require.Zero(t, gateRepo.calls.Load())
	require.Zero(t, h.recorder.totalDivergences(), "skipped gates are not a divergence")
}

func TestShadow_ReturnsDefaultAnswerAndCountsError_WhenRecorderPanics(t *testing.T) {
	t.Parallel()
	h := newSeededSelectorHarness(t, workflowengine.ModeShadow, workflowengine.WithShadowRecorder(panickingShadow{}))

	require.True(t, h.sel.CanTransition(domain.BacklogStatusIdea, domain.BacklogStatusReady, nil))

	require.EqualValues(t, 1, h.sel.ShadowErrorsTotal())
	warns := h.warns.snapshot()
	require.Len(t, warns, 1)
	require.Contains(t, warns[0], "goroutine", "stack is logged")
}

func TestShadow_ReturnsDefaultAnswer_WhenConfiguredHasNilSnapshot(t *testing.T) {
	t.Parallel()
	zero := workflowengine.NewConfiguredWorkflowEngineEmptyCache()
	h := newSelectorHarness(t, workflowengine.ModeShadow, zero)

	require.NotPanics(t, func() {
		require.True(t, h.sel.CanTransition(domain.BacklogStatusIdea, domain.BacklogStatusReady, nil))
		require.NoError(t, h.sel.ValidateGates(domain.BacklogItemTransitionInput{Status: domain.BacklogStatusIdea, AcCriteria: oneAcCriterion()}, domain.BacklogStatusReady, nil))
	})
	require.Zero(t, h.sel.ShadowErrorsTotal())
}

func TestShadow_SamplesReadPathOneInTen_AndEvaluatesEveryTransition(t *testing.T) {
	t.Parallel()
	h := newSeededSelectorHarness(t, workflowengine.ModeShadow)

	for i := 0; i < 1000; i++ {
		h.sel.AllowedTransitions(domain.BacklogStatusIdea, nil)
		h.sel.CanTransition(domain.BacklogStatusIdea, domain.BacklogStatusReady, nil)
	}

	require.Equal(t, 100, h.recorder.calls[callKey{workflowengine.MethodAllowedTransitions, workflowengine.PathRead}])
	require.Equal(t, 1000, h.recorder.calls[callKey{workflowengine.MethodCanTransition, workflowengine.PathTransition}])
}

// --- mode caching ----------------------------------------------------------

type countingGateRepo struct {
	workflowengine.GateSatisfactionRepository
	calls atomic.Int64
}

func (r *countingGateRepo) GetByItemAndGate(context.Context, uuid.UUID, uuid.UUID) (*workflowengine.GateSatisfactionData, error) {
	r.calls.Add(1)
	return nil, errors.New("not found")
}

func TestSwitchableWorkflowEngine_ReadsModeFromTtlCache_NotConfigFileEveryCall(t *testing.T) {
	t.Parallel()
	var loads atomic.Int64
	clock := sessiontest.NewSelectorClock()
	modes := workflowengine.NewCachedModeSource(func() workflowengine.EngineMode { loads.Add(1); return workflowengine.ModeShadow }, time.Second, clock)
	gateRepo := &countingGateRepo{}
	repo := NewTestEntRepository(t)
	require.NoError(t, workflowengine.EnsureBuiltInWorkflowStages(context.Background(), repo.Client))
	cfg, err := workflowengine.NewConfiguredWorkflowEngine(NewEntStageConfigRepository(repo.Client), gateRepo, nil)
	require.NoError(t, err)
	sel := workflowengine.NewSwitchableWorkflowEngine(workflowengine.NewDefaultWorkflowEngine(), cfg, modes, workflowengine.WithSelectorClock(clock))

	for i := 0; i < 50; i++ {
		sel.CanTransition(domain.BacklogStatusIdea, domain.BacklogStatusReady, nil)
	}
	require.EqualValues(t, 1, loads.Load(), "N calls inside the TTL load once")
	require.Zero(t, gateRepo.calls.Load(), "CanTransition does no per-call DB round trip")

	clock.Advance(1100 * time.Millisecond)
	sel.CanTransition(domain.BacklogStatusIdea, domain.BacklogStatusReady, nil)
	require.EqualValues(t, 2, loads.Load(), "reload after expiry")
}

// --- equivalence -----------------------------------------------------------

func TestSwitchableWorkflowEngine_ModeOffEqualsDefaultEngine_ForAllEightyOnePairsAndGateCorpus(t *testing.T) {
	t.Parallel()
	h := newSeededSelectorHarness(t, workflowengine.ModeOff)
	setEdgeEnabled(t, h.client, h.cfg, domain.BacklogStatusDone, domain.BacklogStatusRefining, false)

	require.Equal(t, engineDiff{}, diffEngines(t, workflowengine.NewDefaultWorkflowEngine(), h.sel, gateFixtureCorpus()))
}

// The only graph difference between Default and the composed engine on a
// seeded config is the archive group that validTransitions lacks, and it never
// reaches AllowedTransitions.
func TestEnforceAnswer_MatchesDefaultOn81PairsAndGateCorpus_ExceptSystemArchiveEdges(t *testing.T) {
	t.Parallel()
	h := newSeededSelectorHarness(t, workflowengine.ModeEnforce)

	diff := diffEngines(t, workflowengine.NewDefaultWorkflowEngine(), h.sel, gateFixtureCorpus())

	want := make([]string, 0, 4)
	for _, from := range []domain.BacklogStatus{domain.BacklogStatusInProgress, domain.BacklogStatusReview, domain.BacklogStatusPRPending, domain.BacklogStatusArchived} {
		want = append(want, fmt.Sprintf("CanTransition(%s,archived): reference=false underTest=true", from))
	}
	for _, e := range workflowengine.SystemRequiredOffGraph { // added by the Story 1.5.1 edge audit
		want = append(want, fmt.Sprintf("CanTransition(%s,%s): reference=false underTest=true", e.From, e.To))
	}
	require.ElementsMatch(t, want, diff.Edges)
	require.Empty(t, diff.Gates, "guard parity through the composition, including errors.Is identity")
}

// --- Story 1.5.2: class-aware selector -----------------------------------------

func enforceAll(h *selectorHarness) *selectorHarness {
	h.sel.Modes = allClasses(workflowengine.ModeEnforce)
	return h
}
func shadowAll(h *selectorHarness) *selectorHarness {
	h.sel.Modes = allClasses(workflowengine.ModeShadow)
	return h
}
func viewFor(h *selectorHarness, class domain.CallerClass, g domain.GuardPolicy) domain.WorkflowEngine {
	return h.sel.ForClass(class, g)
}

type classedShadow struct {
	recordingShadow
	obs []workflowengine.ShadowObservation
}

func newClassedShadow() *classedShadow {
	return &classedShadow{recordingShadow: *newRecordingShadow()}
}
func (c *classedShadow) RecordShadowCallFor(_ domain.CallerClass, m workflowengine.EngineMethod, p workflowengine.CallPath) {
	c.RecordShadowCall(m, p)
}
func (c *classedShadow) RecordDivergenceFor(_ domain.CallerClass, m workflowengine.EngineMethod, d workflowengine.DivergenceClass, cfg string, p workflowengine.CallPath) {
	c.RecordDivergence(m, d, cfg, p)
}
func (c *classedShadow) RecordNotComparedFor(_ domain.CallerClass, r workflowengine.NotComparedReason) {
	c.RecordNotCompared(r)
}
func (c *classedShadow) RecordObservation(o workflowengine.ShadowObservation) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.obs = append(c.obs, o)
}

func TestEnforceAnswerPolicy_OverrideTruthTable_PerClassAndGuardPolicy(t *testing.T) {
	t.Parallel()
	const reason = "owner approved"
	noPass := domain.BacklogItemTransitionInput{Status: domain.BacklogStatusReview, OverrideReason: reason}

	type row struct {
		name   string
		class  domain.CallerClass
		guards domain.GuardPolicy
		gated  bool // a human_approval gate sits on review->done
		want   error
	}
	rows := []row{
		// No Configured gate: the Default guard's own exemption is all that override can free.
		{"operator frees the Default exemption", domain.ClassOperator, domain.GuardsBuiltIn, false, nil},
		{"agent frees the Default exemption", domain.ClassAgent, domain.GuardsBuiltIn, false, nil},
		{"system has no override, built-in guard denies", domain.ClassSystem, domain.GuardsBuiltIn, false, domain.ErrVerdictRequired},
		{"system with no guards passes", domain.ClassSystem, domain.GuardsNone, false, nil},
		// A Configured operator gate: only the operator's override bypasses it.
		{"operator override bypasses the Configured gate", domain.ClassOperator, domain.GuardsBuiltIn, true, nil},
		{"agent override never bypasses the Configured gate", domain.ClassAgent, domain.GuardsBuiltIn, true, domain.ErrGateNotSatisfied},
		{"system hits the Configured gate", domain.ClassSystem, domain.GuardsNone, true, domain.ErrGateNotSatisfied},
		{"unknown class fails closed to agent", domain.CallerClass("bogus"), domain.GuardsBuiltIn, true, domain.ErrGateNotSatisfied},
	}
	for _, tc := range rows {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := enforceAll(newSeededSelectorHarness(t, workflowengine.ModeEnforce))
			if tc.gated {
				addGateToSeededEdge(t, h.client, h.cfg, domain.BacklogStatusReview, domain.BacklogStatusDone, domain.GateKindHumanApproval, nil)
			}
			err := viewFor(h, tc.class, tc.guards).ValidateGates(noPass, domain.BacklogStatusDone, nil)
			if tc.want == nil {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, tc.want)
			}
		})
	}
}

func TestEnforceAnswerPolicy_OverrideNeverFreesACPlanOrBlockerDenials_ForOperatorAndAgent(t *testing.T) {
	t.Parallel()
	h := enforceAll(newSeededSelectorHarness(t, workflowengine.ModeEnforce))
	cases := []struct {
		name string
		item domain.BacklogItemTransitionInput
		to   domain.BacklogStatus
		want error
	}{
		{"ErrACRequired", domain.BacklogItemTransitionInput{Status: domain.BacklogStatusIdea}, domain.BacklogStatusReady, domain.ErrACRequired},
		{"ErrPlanRequired", domain.BacklogItemTransitionInput{Status: domain.BacklogStatusReady}, domain.BacklogStatusInProgress, domain.ErrPlanRequired},
		{"ErrPlanArtifactsRequired", domain.BacklogItemTransitionInput{Status: domain.BacklogStatusReady, PlanApproved: true}, domain.BacklogStatusInProgress, domain.ErrPlanArtifactsRequired},
		{"ErrPlanArtifactsMissing", domain.BacklogItemTransitionInput{Status: domain.BacklogStatusReady, PlanApproved: true, PlanArtifactsPath: "/gone", PlanArtifactsPathMissing: true}, domain.BacklogStatusInProgress, domain.ErrPlanArtifactsMissing},
		{"ErrUnresolvedBlockers", domain.BacklogItemTransitionInput{Status: domain.BacklogStatusReady, SkipPlanning: true, HasUnresolvedBlockers: true}, domain.BacklogStatusInProgress, domain.ErrUnresolvedBlockers},
	}
	for _, class := range []domain.CallerClass{domain.ClassOperator, domain.ClassAgent} {
		for _, tc := range cases {
			tc.item.OverrideReason = "please"
			require.ErrorIsf(t, viewFor(h, class, domain.GuardsBuiltIn).ValidateGates(tc.item, tc.to, nil), tc.want, "%s %s", class, tc.name)
		}
	}
}

func TestEnforceAnswerPolicy_AgentOverrideFreesOnlyReviewAndPRPendingToReadyAndDone(t *testing.T) {
	t.Parallel()
	h := enforceAll(newSeededSelectorHarness(t, workflowengine.ModeEnforce))
	agent := viewFor(h, domain.ClassAgent, domain.GuardsBuiltIn)
	for _, from := range []domain.BacklogStatus{domain.BacklogStatusReview, domain.BacklogStatusPRPending} {
		for _, to := range []domain.BacklogStatus{domain.BacklogStatusReady, domain.BacklogStatusDone} {
			item := domain.BacklogItemTransitionInput{Status: from, OverallOutcome: domain.ReviewOutcomePass}
			if to == domain.BacklogStatusDone {
				item.OverallOutcome = ""
			}
			require.Errorf(t, agent.ValidateGates(item, to, nil), "%s->%s without override is denied", from, to)
			item.OverrideReason = "owner approved"
			require.NoErrorf(t, agent.ValidateGates(item, to, nil), "%s->%s with override is freed", from, to)
		}
	}
}

func TestEnforceAnswerPolicy_PendingGatesOmitConfiguredGatesOnlyForOperatorOverride(t *testing.T) {
	t.Parallel()
	h := enforceAll(newSeededSelectorHarness(t, workflowengine.ModeEnforce))
	addGateToSeededEdge(t, h.client, h.cfg, domain.BacklogStatusReview, domain.BacklogStatusDone, domain.GateKindHumanApproval, nil)
	item := passingReviewItem()
	item.OverrideReason = "owner approved"

	unsatisfied := func(class domain.CallerClass, guards domain.GuardPolicy) int {
		statuses, err := viewFor(h, class, guards).PendingGates(item, domain.BacklogStatusDone, nil)
		require.NoError(t, err)
		n := 0
		for _, s := range statuses {
			if !s.Satisfied {
				n++
			}
		}
		return n
	}
	require.Zero(t, unsatisfied(domain.ClassOperator, domain.GuardsBuiltIn), "operator override omits the Configured gate")
	require.Equal(t, 1, unsatisfied(domain.ClassAgent, domain.GuardsBuiltIn), "agent override keeps it")
	require.Equal(t, 1, unsatisfied(domain.ClassSystem, domain.GuardsNone), "system has no override")

	statuses, err := viewFor(h, domain.ClassSystem, domain.GuardsNone).PendingGates(item, domain.BacklogStatusDone, nil)
	require.NoError(t, err)
	for _, s := range statuses {
		require.False(t, strings.HasPrefix(s.GateID, workflowengine.BuiltInGateIDPrefix), "GuardsNone lists no Default guard status")
	}
}

func TestForCaller_ForwardingUsesGraphWithoutSystemRequiredUnion(t *testing.T) {
	t.Parallel()
	h := enforceAll(newSeededSelectorHarness(t, workflowengine.ModeEnforce))
	setEdgeEnabled(t, h.client, h.cfg, domain.BacklogStatusInProgress, domain.BacklogStatusReview, false)

	delegate, ok := domain.LookupTransitionCaller(domain.CallerMCPTransitionDelegate)
	require.True(t, ok)
	require.True(t, delegate.Forwarding)
	fwd := h.sel.ForCaller(delegate)
	require.False(t, fwd.CanTransition(domain.BacklogStatusInProgress, domain.BacklogStatusReview, nil), "a disabled edge denies a forwarder")
	require.NotContains(t, fwd.AllowedTransitions(domain.BacklogStatusInProgress, nil), domain.BacklogStatusReview)
	require.True(t, viewFor(h, domain.ClassAgent, domain.GuardsBuiltIn).CanTransition(domain.BacklogStatusInProgress, domain.BacklogStatusReview, nil), "a fixed-edge agent keeps the union")
}

func TestForCaller_UncheckedCallerIsAllowedInOffAndShadow_AndEnforcesInEnforce(t *testing.T) {
	t.Parallel()
	spec, ok := domain.LookupTransitionCaller(domain.CallerMCPResumeWork) // agent, unchecked, ready->in_progress, built-in guards
	require.True(t, ok)
	require.Equal(t, domain.LegacyUnchecked, spec.Legacy)
	noPlan := domain.BacklogItemTransitionInput{Status: domain.BacklogStatusReady}

	for _, mode := range []workflowengine.EngineMode{workflowengine.ModeOff, workflowengine.ModeShadow} {
		h := newSeededSelectorHarness(t, mode)
		h.sel.Modes = allClasses(mode)
		v := h.sel.ForCaller(spec)
		require.True(t, v.CanTransition(domain.BacklogStatusIdea, domain.BacklogStatusDone, nil), "legacy answer is allow, mode %d", mode)
		require.NoError(t, v.ValidateGates(noPlan, domain.BacklogStatusInProgress, nil), "mode %d", mode)
	}
	h := enforceAll(newSeededSelectorHarness(t, workflowengine.ModeEnforce))
	v := h.sel.ForCaller(spec)
	require.False(t, v.CanTransition(domain.BacklogStatusIdea, domain.BacklogStatusDone, nil))
	require.ErrorIs(t, v.ValidateGates(noPlan, domain.BacklogStatusInProgress, nil), domain.ErrPlanRequired)
}

func TestShadow_UncheckedAgentGuardDenial_IsGuardNewLabelledWithClassAndCaller(t *testing.T) {
	t.Parallel()
	rec := newClassedShadow()
	h := shadowAll(newSeededSelectorHarness(t, workflowengine.ModeShadow, workflowengine.WithShadowRecorder(rec)))
	spec, _ := domain.LookupTransitionCaller(domain.CallerMCPResumeWork)
	v := h.sel.ForCaller(spec)

	require.NoError(t, v.ValidateGates(domain.BacklogItemTransitionInput{Status: domain.BacklogStatusReady}, domain.BacklogStatusInProgress, nil))
	require.Equal(t, 1, rec.divergences[divergenceKey{workflowengine.MethodValidateGates, workflowengine.ClassGuardNew, "edited", workflowengine.PathTransition}])
	require.Len(t, rec.obs, 1)
	require.Equal(t, workflowengine.ClassGuardNew, rec.obs[0].Class)
	require.Equal(t, domain.ClassAgent, rec.obs[0].CallerClass)
	require.Equal(t, domain.CallerMCPResumeWork, rec.obs[0].Caller)
}

func TestShadow_UncheckedCallerOperatorGate_IsGateExtraNotGuardNew(t *testing.T) {
	t.Parallel()
	rec := newClassedShadow()
	h := shadowAll(newSeededSelectorHarness(t, workflowengine.ModeShadow, workflowengine.WithShadowRecorder(rec)))
	addGateToSeededEdge(t, h.client, h.cfg, domain.BacklogStatusReady, domain.BacklogStatusInProgress, domain.GateKindStructural,
		map[string]interface{}{"check_id": domain.StructuralCheckACComplete})
	spec, _ := domain.LookupTransitionCaller(domain.CallerMCPResumeWork) // agent, unchecked, ready->in_progress, built-in guards
	v := h.sel.ForCaller(spec)

	item := domain.BacklogItemTransitionInput{Status: domain.BacklogStatusReady, SkipPlanning: true, PlanArtifactsPath: "/plan"} // Default guard passes
	require.NoError(t, v.ValidateGates(item, domain.BacklogStatusInProgress, nil))
	require.Equal(t, 1, rec.divergences[divergenceKey{workflowengine.MethodValidateGates, workflowengine.ClassGateExtra, "edited", workflowengine.PathTransition}])
	require.Zero(t, rec.divergences[divergenceKey{workflowengine.MethodValidateGates, workflowengine.ClassGuardNew, "edited", workflowengine.PathTransition}])
}

func TestShadow_UncheckedCallers_SeededConfig_ZeroDivergenceExceptGuardNewForBuiltInGuardCallers(t *testing.T) {
	t.Parallel()
	rec := newClassedShadow()
	h := shadowAll(newSeededSelectorHarness(t, workflowengine.ModeShadow, workflowengine.WithShadowRecorder(rec)))
	statuses := append([]domain.BacklogStatus(nil), workflowengine.BuiltInStageOrder...)

	known := map[string]bool{}
	for _, k := range domain.KnownOffGraphCallerEdges() {
		known[string(k.Caller)+"|"+workflowengine.EdgeKey(k.Edge.From, k.Edge.To)] = true
	}
	calls := 0
	for _, spec := range domain.TransitionCallers() {
		if spec.Legacy != domain.LegacyUnchecked || spec.Forwarding {
			continue
		}
		v := h.sel.ForCaller(spec)
		for _, e := range spec.Edges {
			froms := []domain.BacklogStatus{e.From}
			if e.From == domain.AnyStatus {
				froms = statuses
			}
			for _, from := range froms {
				if known[string(spec.Name)+"|"+workflowengine.EdgeKey(from, e.To)] {
					continue // a tolerated caller bug (Task 1.5.5c), reported as a graph divergence by design
				}
				calls++
				v.CanTransition(from, e.To, nil)
				_ = v.ValidateGates(domain.BacklogItemTransitionInput{Status: from}, e.To, nil)
			}
		}
	}
	require.Positive(t, calls)
	for _, o := range rec.obs {
		if o.Class == "" {
			continue
		}
		require.Equalf(t, workflowengine.ClassGuardNew, o.Class, "%s %s->%s: only guard_new is expected", o.Caller, o.From, o.To)
		require.NotEqualf(t, domain.ClassSystem, o.CallerClass, "%s: system callers carry GuardsNone, so no guard_new", o.Caller)
		t.Logf("guard_new for owner acknowledgement: class=%s caller=%s %s->%s", o.CallerClass, o.Caller, o.From, o.To)
	}
	for k, n := range rec.divergences {
		require.Equalf(t, workflowengine.ClassGuardNew, k.Class, "%+v x%d", k, n)
	}
}

func TestModeFor_FallsBackToOperatorModeAndCapsOtherClassesAtShadow_WhenSourceIsNotClassAware(t *testing.T) {
	t.Parallel()
	mk := func(m workflowengine.EngineMode) *workflowengine.SwitchableWorkflowEngine {
		return workflowengine.NewSwitchableWorkflowEngine(workflowengine.NewDefaultWorkflowEngine(), nil, fixedMode{m})
	}
	require.Equal(t, workflowengine.ModeEnforce, mk(workflowengine.ModeEnforce).ModeFor(domain.ClassOperator))
	require.Equal(t, workflowengine.ModeShadow, mk(workflowengine.ModeEnforce).ModeFor(domain.ClassAgent), "no enforce flag exists for agent")
	require.Equal(t, workflowengine.ModeShadow, mk(workflowengine.ModeShadow).ModeFor(domain.ClassSystem))
	require.Equal(t, workflowengine.ModeOff, mk(workflowengine.ModeOff).ModeFor(domain.ClassAgent))
	require.Equal(t, workflowengine.ModeOff, workflowengine.NewSwitchableWorkflowEngine(workflowengine.NewDefaultWorkflowEngine(), nil, nil).ModeFor(domain.ClassOperator))
	require.Equal(t, workflowengine.ModeEnforce, workflowengine.NewSwitchableWorkflowEngine(workflowengine.NewDefaultWorkflowEngine(), nil, classModes{domain.ClassAgent: workflowengine.ModeEnforce}).ModeFor(domain.ClassAgent))
}

func TestOperatorMethods_AreTheOperatorPolicyView(t *testing.T) {
	t.Parallel()
	h := enforceAll(newSeededSelectorHarness(t, workflowengine.ModeEnforce))
	addGateToSeededEdge(t, h.client, h.cfg, domain.BacklogStatusReview, domain.BacklogStatusDone, domain.GateKindHumanApproval, nil)
	item := passingReviewItem()
	item.OverrideReason = "owner approved"
	require.NoError(t, h.sel.ValidateGates(item, domain.BacklogStatusDone, nil))
	require.NoError(t, viewFor(h, domain.ClassOperator, domain.GuardsBuiltIn).ValidateGates(item, domain.BacklogStatusDone, nil))
	require.ErrorIs(t, viewFor(h, domain.ClassAgent, domain.GuardsBuiltIn).ValidateGates(item, domain.BacklogStatusDone, nil), domain.ErrGateNotSatisfied)
}

// discardRecorder is a no-op ShadowRecorder for tests that embed it and override one method.
type discardRecorder struct{}

func (discardRecorder) RecordShadowCall(workflowengine.EngineMethod, workflowengine.CallPath) {}
func (discardRecorder) RecordDivergence(workflowengine.EngineMethod, workflowengine.DivergenceClass, string, workflowengine.CallPath) {
}
func (discardRecorder) RecordNotCompared(workflowengine.NotComparedReason) {}
