package session

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/backlogstage"
	"github.com/tstapler/stapler-squad/session/ent/stagetransition"
)

// --- fakes ---------------------------------------------------------------

type fixedMode struct{ m EngineMode }

func (f fixedMode) Mode() EngineMode { return f.m }

type selectorClock struct{ now atomic.Int64 }

func newSelectorClock() *selectorClock {
	c := &selectorClock{}
	c.now.Store(time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC).UnixNano())
	return c
}
func (c *selectorClock) Now() time.Time          { return time.Unix(0, c.now.Load()).UTC() }
func (c *selectorClock) Advance(d time.Duration) { c.now.Add(int64(d)) }

type divergenceKey struct {
	Method EngineMethod
	Class  DivergenceClass
	Config string
	Path   CallPath
}

type callKey struct {
	Method EngineMethod
	Path   CallPath
}

type recordingShadow struct {
	mu          sync.Mutex
	calls       map[callKey]int
	divergences map[divergenceKey]int
	notCompared map[NotComparedReason]int
}

func newRecordingShadow() *recordingShadow {
	return &recordingShadow{calls: map[callKey]int{}, divergences: map[divergenceKey]int{}, notCompared: map[NotComparedReason]int{}}
}

func (r *recordingShadow) RecordShadowCall(m EngineMethod, p CallPath) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls[callKey{m, p}]++
}

func (r *recordingShadow) RecordDivergence(m EngineMethod, c DivergenceClass, config string, p CallPath) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.divergences[divergenceKey{m, c, config, p}]++
}

func (r *recordingShadow) RecordNotCompared(reason NotComparedReason) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.notCompared[reason]++
}

func (r *recordingShadow) totalDivergences() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, v := range r.divergences {
		n += v
	}
	return n
}

type panickingShadow struct{ discardRecorder }

func (panickingShadow) RecordShadowCall(EngineMethod, CallPath) { panic("recorder boom") }

type lineSink struct {
	mu    sync.Mutex
	lines []string
}

func (l *lineSink) logf(format string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lines = append(l.lines, fmt.Sprintf(format, args...))
}

func (l *lineSink) snapshot() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.lines...)
}

// failIfCalledGateRepo is a GateSatisfactionRepository that fails the test on
// any read; the embedded nil interface panics on every other method.
type failIfCalledGateRepo struct {
	GateSatisfactionRepository
	t     *testing.T
	calls atomic.Int64
}

func (r *failIfCalledGateRepo) GetByItemAndGate(context.Context, uuid.UUID, uuid.UUID) (*GateSatisfactionData, error) {
	r.calls.Add(1)
	r.t.Errorf("GateSatisfactionRepository.GetByItemAndGate must not be called")
	return nil, errors.New("unexpected call")
}

type failingStageRepo struct{ StageConfigRepository }

func (failingStageRepo) ListEnabledStages(context.Context) ([]*ent.BacklogStage, error) {
	return nil, errors.New("db down")
}

// --- builders ------------------------------------------------------------

type selectorHarness struct {
	sel      *SwitchableWorkflowEngine
	cfg      *ConfiguredWorkflowEngine
	client   *ent.Client
	recorder *recordingShadow
	warns    *lineSink
	infos    *lineSink
	clock    *selectorClock
}

func newSelectorHarness(t *testing.T, mode EngineMode, cfg *ConfiguredWorkflowEngine, opts ...SelectorOption) *selectorHarness {
	t.Helper()
	h := &selectorHarness{cfg: cfg, recorder: newRecordingShadow(), warns: &lineSink{}, infos: &lineSink{}, clock: newSelectorClock()}
	base := make([]SelectorOption, 0, 4+len(opts))
	base = append(base,
		WithShadowRecorder(h.recorder),
		WithSelectorClock(h.clock),
		WithSelectorLoggers(h.warns.logf, h.infos.logf),
		WithConfigLabeler(func() string { return "edited" }),
	)
	h.sel = NewSwitchableWorkflowEngine(NewDefaultWorkflowEngine(), cfg, fixedMode{mode}, append(base, opts...)...)
	return h
}

func newSeededSelectorHarness(t *testing.T, mode EngineMode, opts ...SelectorOption) *selectorHarness {
	t.Helper()
	cfg, client := newSeededConfiguredWorkflowEngine(t)
	h := newSelectorHarness(t, mode, cfg, opts...)
	h.client = client
	return h
}

func setEdgeEnabled(t *testing.T, client *ent.Client, cfg *ConfiguredWorkflowEngine, from, to BacklogStatus, enabled bool) {
	t.Helper()
	ctx := context.Background()
	n, err := client.StageTransition.Update().
		Where(
			stagetransition.HasFromStageWith(backlogstage.Slug(string(from))),
			stagetransition.HasToStageWith(backlogstage.Slug(string(to))),
		).
		SetEnabled(enabled).
		Save(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, n, "edge %s->%s must exist", from, to)
	require.NoError(t, cfg.InvalidateCache(ctx))
}

func addGateToSeededEdge(t *testing.T, client *ent.Client, cfg *ConfiguredWorkflowEngine, from, to BacklogStatus, kind GateKind, config map[string]interface{}) {
	t.Helper()
	ctx := context.Background()
	tr, err := client.StageTransition.Query().
		Where(
			stagetransition.HasFromStageWith(backlogstage.Slug(string(from))),
			stagetransition.HasToStageWith(backlogstage.Slug(string(to))),
		).Only(ctx)
	require.NoError(t, err)
	c := client.TransitionGate.Create().SetTransitionID(tr.ID).SetKind(string(kind)).SetStateful(isStatefulGateKind(kind)).SetEnabled(true)
	if config != nil {
		c = c.SetConfig(config)
	}
	_, err = c.Save(ctx)
	require.NoError(t, err)
	require.NoError(t, cfg.InvalidateCache(ctx))
}

func passingReviewItem() BacklogItemTransitionInput {
	return BacklogItemTransitionInput{Status: BacklogStatusReview, OverallOutcome: domain.ReviewOutcomePass}
}

// --- Off / Shadow / Enforce basics ---------------------------------------

func TestSwitchableWorkflowEngine_ReturnsDefaultAnswer_WhenModeOffOrShadow(t *testing.T) {
	t.Parallel()
	for _, mode := range []EngineMode{ModeOff, ModeShadow} {
		h := newSeededSelectorHarness(t, mode)
		setEdgeEnabled(t, h.client, h.cfg, BacklogStatusDone, BacklogStatusRefining, false)

		require.True(t, h.sel.CanTransition(BacklogStatusDone, BacklogStatusRefining, nil), "mode %d", mode)
		require.Contains(t, h.sel.AllowedTransitions(BacklogStatusDone, nil), BacklogStatusRefining)

		want := 0
		if mode == ModeShadow {
			want = 1
		}
		key := divergenceKey{MethodCanTransition, ClassGraph, "edited", PathTransition}
		require.Equal(t, want, h.recorder.divergences[key], "mode %d", mode)
	}
}

func TestSwitchableWorkflowEngine_EnforceHonorsOperatorEdit_WithoutRestart(t *testing.T) {
	t.Parallel()
	h := newSeededSelectorHarness(t, ModeEnforce)
	require.True(t, h.sel.CanTransition(BacklogStatusDone, BacklogStatusRefining, nil))

	setEdgeEnabled(t, h.client, h.cfg, BacklogStatusDone, BacklogStatusRefining, false)

	require.False(t, h.sel.CanTransition(BacklogStatusDone, BacklogStatusRefining, nil))
	require.True(t, h.sel.CanTransition(BacklogStatusReady, BacklogStatusInProgress, nil))
	require.NotContains(t, h.sel.AllowedTransitions(BacklogStatusDone, nil), BacklogStatusRefining)
}

func TestSwitchableWorkflowEngine_EnforceKeepsBuiltInGuards_WhenSeededConfigHasNoGateRows(t *testing.T) {
	t.Parallel()
	h := newSeededSelectorHarness(t, ModeEnforce)
	idea := BacklogItemTransitionInput{Status: BacklogStatusIdea}

	require.ErrorIs(t, h.sel.ValidateGates(idea, BacklogStatusReady, nil), domain.ErrACRequired)
	pending, err := h.sel.PendingGates(idea, BacklogStatusReady, nil)
	require.NoError(t, err)
	require.Len(t, pending, 1)
	require.False(t, pending[0].Satisfied)
}

func TestSwitchableWorkflowEngine_ShadowNeverDivergesOnOrdinaryGuardDenial_WhenSeeded(t *testing.T) {
	t.Parallel()
	h := newSeededSelectorHarness(t, ModeShadow)
	idea := BacklogItemTransitionInput{Status: BacklogStatusIdea}

	require.ErrorIs(t, h.sel.ValidateGates(idea, BacklogStatusReady, nil), domain.ErrACRequired)

	require.Zero(t, h.recorder.totalDivergences())
	require.Equal(t, 1, h.recorder.calls[callKey{MethodValidateGates, PathTransition}])
}

func TestSwitchableWorkflowEngine_ShadowCountsGateExtra_WhenOperatorAddsStructuralGate(t *testing.T) {
	t.Parallel()
	h := newSeededSelectorHarness(t, ModeShadow)
	addGateToSeededEdge(t, h.client, h.cfg, BacklogStatusReady, BacklogStatusInProgress, GateKindStructural,
		map[string]interface{}{"check_id": StructuralCheckACComplete})
	item := BacklogItemTransitionInput{Status: BacklogStatusReady, SkipPlanning: true, PlanArtifactsPath: "/plan"}
	require.NoError(t, NewDefaultWorkflowEngine().ValidateGates(item, BacklogStatusInProgress, nil), "fixture: Default must pass")

	require.NoError(t, h.sel.ValidateGates(item, BacklogStatusInProgress, nil), "shadow returns the Default answer")

	require.Equal(t, 1, h.recorder.divergences[divergenceKey{MethodValidateGates, ClassGateExtra, "edited", PathTransition}])
}

// --- fallback --------------------------------------------------------------

func TestSwitchableWorkflowEngine_FallsBackToDefaultGraph_WhenConfiguredCacheEmptyOrNil(t *testing.T) {
	t.Parallel()
	emptyRepo := NewTestEntRepository(t)
	emptyCfg, err := NewConfiguredWorkflowEngine(NewEntStageConfigRepository(emptyRepo.client), nil, nil)
	require.NoError(t, err)
	loadErrCfg, err := NewConfiguredWorkflowEngine(failingStageRepo{}, nil, nil)
	require.NoError(t, err)

	cases := []struct {
		name   string
		cfg    *ConfiguredWorkflowEngine
		reason FallbackReason
	}{
		{"cache_empty", emptyCfg, FallbackCacheEmpty},
		{"cfg_nil", nil, FallbackCfgNil},
		{"load_error", loadErrCfg, FallbackLoadError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newSelectorHarness(t, ModeEnforce, tc.cfg)
			calls := 0

			require.True(t, h.sel.CanTransition(BacklogStatusIdea, BacklogStatusReady, nil))
			calls++
			for _, from := range []BacklogStatus{BacklogStatusInProgress, BacklogStatusReview, BacklogStatusPRPending, BacklogStatusArchived} {
				require.Truef(t, h.sel.CanTransition(from, BacklogStatusArchived, nil), "%s->archived", from)
				calls++
			}
			require.NotContains(t, h.sel.AllowedTransitions(BacklogStatusInProgress, nil), BacklogStatusArchived)
			calls++
			require.False(t, h.sel.CanTransition(BacklogStatusIdea, BacklogStatusDone, nil), "fallback is not allow-all")
			calls++

			require.EqualValues(t, calls, h.sel.FallbackTotal(tc.reason))
			require.Len(t, h.warns.snapshot(), 1, "one WARN per minute")
			h.clock.Advance(61 * time.Second)
			h.sel.CanTransition(BacklogStatusIdea, BacklogStatusReady, nil)
			require.Len(t, h.warns.snapshot(), 2)
		})
	}
}

func TestSwitchableWorkflowEngine_UnionsSystemRequiredEdges_WhenLegacyDbDisablesInProgressReview(t *testing.T) {
	t.Parallel()
	h := newSeededSelectorHarness(t, ModeEnforce)
	setEdgeEnabled(t, h.client, h.cfg, BacklogStatusInProgress, BacklogStatusReview, false)

	require.True(t, h.sel.CanTransition(BacklogStatusInProgress, BacklogStatusReview, nil))
	require.Contains(t, h.sel.AllowedTransitions(BacklogStatusInProgress, nil), BacklogStatusReview)
	require.Zero(t, h.sel.FallbackTotal(FallbackCacheEmpty), "configured graph in use, not the fallback")
}

func TestSwitchableWorkflowEngine_TypedNilConfigured_AnswersFromDefault_WithoutPanic(t *testing.T) {
	t.Parallel()
	var cfg *ConfiguredWorkflowEngine
	h := newSelectorHarness(t, ModeEnforce, cfg)
	item := BacklogItemTransitionInput{Status: BacklogStatusIdea}

	require.NotPanics(t, func() {
		h.sel.CanTransition(BacklogStatusIdea, BacklogStatusReady, nil)
		h.sel.AllowedTransitions(BacklogStatusIdea, nil)
		_, _ = h.sel.PendingGates(item, BacklogStatusReady, nil)
		require.ErrorIs(t, h.sel.ValidateGates(item, BacklogStatusReady, nil), domain.ErrACRequired)
	})
	require.EqualValues(t, 4, h.sel.FallbackTotal(FallbackCfgNil))
}

// --- override_reason -------------------------------------------------------

func TestSwitchableWorkflowEngine_BypassesGuardsNotGraph_WhenOverrideReasonNonEmpty(t *testing.T) {
	t.Parallel()

	t.Run("operator gate is bypassed by override but blocks without it", func(t *testing.T) {
		t.Parallel()
		h := newSeededSelectorHarness(t, ModeEnforce)
		addGateToSeededEdge(t, h.client, h.cfg, BacklogStatusReview, BacklogStatusDone, GateKindHumanApproval, nil)

		item := passingReviewItem()
		require.ErrorIs(t, h.sel.ValidateGates(item, BacklogStatusDone, nil), ErrGateNotSatisfied)
		item.OverrideReason = "ship per owner"
		require.NoError(t, h.sel.ValidateGates(item, BacklogStatusDone, nil))
	})

	t.Run("synthetic deleted-stage gate is bypassed by override", func(t *testing.T) {
		t.Parallel()
		h := newSeededSelectorHarness(t, ModeEnforce)
		snap := &StageConfigSnapshot{StageName: "Gone", AllowedTransitions: []BacklogStatus{BacklogStatusReady}}
		item := BacklogItemTransitionInput{Status: "gone-stage"}

		require.True(t, h.sel.CanTransition("gone-stage", BacklogStatusReady, snap))
		require.ErrorIs(t, h.sel.ValidateGates(item, BacklogStatusReady, snap), ErrGateNotSatisfied)
		item.OverrideReason = "stage deleted, forcing"
		require.NoError(t, h.sel.ValidateGates(item, BacklogStatusReady, snap))
	})

	t.Run("Default guards that ignore override still deny", func(t *testing.T) {
		t.Parallel()
		h := newSeededSelectorHarness(t, ModeEnforce)
		const reason = "please"
		cases := []struct {
			name string
			item BacklogItemTransitionInput
			to   BacklogStatus
			want error
		}{
			{"ErrACRequired", BacklogItemTransitionInput{Status: BacklogStatusIdea}, BacklogStatusReady, domain.ErrACRequired},
			{"ErrPlanRequired", BacklogItemTransitionInput{Status: BacklogStatusReady}, BacklogStatusInProgress, domain.ErrPlanRequired},
			{"ErrPlanArtifactsRequired", BacklogItemTransitionInput{Status: BacklogStatusReady, PlanApproved: true}, BacklogStatusInProgress, domain.ErrPlanArtifactsRequired},
			{"ErrPlanArtifactsMissing", BacklogItemTransitionInput{Status: BacklogStatusReady, PlanApproved: true, PlanArtifactsPath: "/gone", PlanArtifactsPathMissing: true}, BacklogStatusInProgress, domain.ErrPlanArtifactsMissing},
			{"ErrUnresolvedBlockers", BacklogItemTransitionInput{Status: BacklogStatusReady, SkipPlanning: true, HasUnresolvedBlockers: true}, BacklogStatusInProgress, domain.ErrUnresolvedBlockers},
		}
		for _, tc := range cases {
			tc.item.OverrideReason = reason
			require.ErrorIsf(t, h.sel.ValidateGates(tc.item, tc.to, nil), tc.want, tc.name)
		}
	})

	t.Run("review to done without PASS succeeds under override exactly as under Default", func(t *testing.T) {
		t.Parallel()
		h := newSeededSelectorHarness(t, ModeEnforce)
		def := NewDefaultWorkflowEngine()
		item := BacklogItemTransitionInput{Status: BacklogStatusReview}

		require.ErrorIs(t, h.sel.ValidateGates(item, BacklogStatusDone, nil), domain.ErrVerdictRequired)
		item.OverrideReason = "owner approved"
		require.NoError(t, def.ValidateGates(item, BacklogStatusDone, nil))
		require.NoError(t, h.sel.ValidateGates(item, BacklogStatusDone, nil))
	})

	t.Run("override never bypasses the graph", func(t *testing.T) {
		t.Parallel()
		h := newSeededSelectorHarness(t, ModeEnforce)
		require.False(t, h.sel.CanTransition(BacklogStatusIdea, BacklogStatusDone, nil))
		setEdgeEnabled(t, h.client, h.cfg, BacklogStatusDone, BacklogStatusRefining, false)
		require.False(t, h.sel.CanTransition(BacklogStatusDone, BacklogStatusRefining, nil))
	})
}

// --- attribution -----------------------------------------------------------

func TestSwitchableWorkflowEngine_LogsFromToReason_WhenTransitionDeniedByGraphOperatorGateOrDefaultGuard(t *testing.T) {
	t.Parallel()
	h := newSeededSelectorHarness(t, ModeEnforce)
	addGateToSeededEdge(t, h.client, h.cfg, BacklogStatusReady, BacklogStatusQueued, GateKindHumanApproval, nil)

	require.False(t, h.sel.CanTransition(BacklogStatusIdea, BacklogStatusDone, nil))
	require.ErrorIs(t, h.sel.ValidateGates(BacklogItemTransitionInput{Status: BacklogStatusReady}, BacklogStatusQueued, nil), ErrGateNotSatisfied)
	require.ErrorIs(t, h.sel.ValidateGates(BacklogItemTransitionInput{Status: BacklogStatusReview}, BacklogStatusDone, nil), domain.ErrVerdictRequired)

	lines := strings.Join(h.infos.snapshot(), "\n")
	require.Contains(t, lines, "from=idea to=done reason=config_graph")
	require.Contains(t, lines, "from=ready to=queued reason=operator_gate")
	require.Contains(t, lines, "from=review to=done reason=default_guard")
	require.EqualValues(t, 1, h.sel.EnforceDeniedTotal(DenialConfigGraph))
	require.EqualValues(t, 1, h.sel.EnforceDeniedTotal(DenialOperatorGate))
	require.EqualValues(t, 1, h.sel.EnforceDeniedTotal(DenialDefaultGuard))
}

// --- shadow safety ---------------------------------------------------------

func TestSwitchableWorkflowEngine_ShadowDoesNoStatefulGateIo_WhenStatefulGateOnEdge(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	require.NoError(t, EnsureBuiltInWorkflowStages(context.Background(), repo.client))
	gateRepo := &failIfCalledGateRepo{t: t}
	cfg, err := NewConfiguredWorkflowEngine(NewEntStageConfigRepository(repo.client), gateRepo, nil)
	require.NoError(t, err)
	from, to := newCustomTransitionWithGates(t, repo.client, cfg,
		[]GateKind{GateKindHumanApproval, GateKindAutomatedReview, GateKindCustom}, []bool{true, true, true})
	h := newSelectorHarness(t, ModeShadow, cfg)
	item := BacklogItemTransitionInput{Status: from, ItemID: uuid.NewString()}

	require.NoError(t, h.sel.ValidateGates(item, to, nil), "Default has no gate on a custom edge")

	require.Equal(t, 1, h.recorder.notCompared[NotComparedStatefulGate])
	require.Zero(t, gateRepo.calls.Load())
	require.Zero(t, h.recorder.totalDivergences(), "skipped gates are not a divergence")
}

func TestShadow_ReturnsDefaultAnswerAndCountsError_WhenRecorderPanics(t *testing.T) {
	t.Parallel()
	h := newSeededSelectorHarness(t, ModeShadow, WithShadowRecorder(panickingShadow{}))

	require.True(t, h.sel.CanTransition(BacklogStatusIdea, BacklogStatusReady, nil))

	require.EqualValues(t, 1, h.sel.ShadowErrorsTotal())
	warns := h.warns.snapshot()
	require.Len(t, warns, 1)
	require.Contains(t, warns[0], "goroutine", "stack is logged")
}

func TestShadow_ReturnsDefaultAnswer_WhenConfiguredHasNilSnapshot(t *testing.T) {
	t.Parallel()
	zero := &ConfiguredWorkflowEngine{cache: &stageConfigCache{}}
	h := newSelectorHarness(t, ModeShadow, zero)

	require.NotPanics(t, func() {
		require.True(t, h.sel.CanTransition(BacklogStatusIdea, BacklogStatusReady, nil))
		require.NoError(t, h.sel.ValidateGates(BacklogItemTransitionInput{Status: BacklogStatusIdea, AcCriteria: oneAcCriterion()}, BacklogStatusReady, nil))
	})
	require.Zero(t, h.sel.ShadowErrorsTotal())
}

func TestShadow_SamplesReadPathOneInTen_AndEvaluatesEveryTransition(t *testing.T) {
	t.Parallel()
	h := newSeededSelectorHarness(t, ModeShadow)

	for i := 0; i < 1000; i++ {
		h.sel.AllowedTransitions(BacklogStatusIdea, nil)
		h.sel.CanTransition(BacklogStatusIdea, BacklogStatusReady, nil)
	}

	require.Equal(t, 100, h.recorder.calls[callKey{MethodAllowedTransitions, PathRead}])
	require.Equal(t, 1000, h.recorder.calls[callKey{MethodCanTransition, PathTransition}])
}

// --- mode caching ----------------------------------------------------------

type countingGateRepo struct {
	GateSatisfactionRepository
	calls atomic.Int64
}

func (r *countingGateRepo) GetByItemAndGate(context.Context, uuid.UUID, uuid.UUID) (*GateSatisfactionData, error) {
	r.calls.Add(1)
	return nil, errors.New("not found")
}

func TestSwitchableWorkflowEngine_ReadsModeFromTtlCache_NotConfigFileEveryCall(t *testing.T) {
	t.Parallel()
	var loads atomic.Int64
	clock := newSelectorClock()
	modes := NewCachedModeSource(func() EngineMode { loads.Add(1); return ModeShadow }, time.Second, clock)
	gateRepo := &countingGateRepo{}
	repo := NewTestEntRepository(t)
	require.NoError(t, EnsureBuiltInWorkflowStages(context.Background(), repo.client))
	cfg, err := NewConfiguredWorkflowEngine(NewEntStageConfigRepository(repo.client), gateRepo, nil)
	require.NoError(t, err)
	sel := NewSwitchableWorkflowEngine(NewDefaultWorkflowEngine(), cfg, modes, WithSelectorClock(clock))

	for i := 0; i < 50; i++ {
		sel.CanTransition(BacklogStatusIdea, BacklogStatusReady, nil)
	}
	require.EqualValues(t, 1, loads.Load(), "N calls inside the TTL load once")
	require.Zero(t, gateRepo.calls.Load(), "CanTransition does no per-call DB round trip")

	clock.Advance(1100 * time.Millisecond)
	sel.CanTransition(BacklogStatusIdea, BacklogStatusReady, nil)
	require.EqualValues(t, 2, loads.Load(), "reload after expiry")
}

// --- equivalence -----------------------------------------------------------

func TestSwitchableWorkflowEngine_ModeOffEqualsDefaultEngine_ForAllEightyOnePairsAndGateCorpus(t *testing.T) {
	t.Parallel()
	h := newSeededSelectorHarness(t, ModeOff)
	setEdgeEnabled(t, h.client, h.cfg, BacklogStatusDone, BacklogStatusRefining, false)

	require.Equal(t, engineDiff{}, diffEngines(t, NewDefaultWorkflowEngine(), h.sel, gateFixtureCorpus()))
}

// The only graph difference between Default and the composed engine on a
// seeded config is the archive group that validTransitions lacks, and it never
// reaches AllowedTransitions.
func TestEnforceAnswer_MatchesDefaultOn81PairsAndGateCorpus_ExceptSystemArchiveEdges(t *testing.T) {
	t.Parallel()
	h := newSeededSelectorHarness(t, ModeEnforce)

	diff := diffEngines(t, NewDefaultWorkflowEngine(), h.sel, gateFixtureCorpus())

	want := make([]string, 0, 4)
	for _, from := range []BacklogStatus{BacklogStatusInProgress, BacklogStatusReview, BacklogStatusPRPending, BacklogStatusArchived} {
		want = append(want, fmt.Sprintf("CanTransition(%s,archived): reference=false underTest=true", from))
	}
	require.ElementsMatch(t, want, diff.Edges)
	require.Empty(t, diff.Gates, "guard parity through the composition, including errors.Is identity")
}
