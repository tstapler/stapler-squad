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
	for _, e := range systemRequiredOffGraph { // added by the Story 1.5.1 edge audit
		want = append(want, fmt.Sprintf("CanTransition(%s,%s): reference=false underTest=true", e.From, e.To))
	}
	require.ElementsMatch(t, want, diff.Edges)
	require.Empty(t, diff.Gates, "guard parity through the composition, including errors.Is identity")
}

// --- Story 1.5.2: class-aware selector -----------------------------------------

type classModes map[CallerClass]EngineMode

func (c classModes) Mode() EngineMode                 { return c[ClassOperator] }
func (c classModes) ModeFor(k CallerClass) EngineMode { return c[k] }
func allClasses(m EngineMode) classModes {
	return classModes{ClassOperator: m, ClassAgent: m, ClassSystem: m, "bogus": m}
}
func enforceAll(h *selectorHarness) *selectorHarness {
	h.sel.modes = allClasses(ModeEnforce)
	return h
}
func shadowAll(h *selectorHarness) *selectorHarness {
	h.sel.modes = allClasses(ModeShadow)
	return h
}
func viewFor(h *selectorHarness, class CallerClass, g GuardPolicy) WorkflowEngine {
	return h.sel.ForClass(class, g)
}

type classedShadow struct {
	recordingShadow
	obs []ShadowObservation
}

func newClassedShadow() *classedShadow {
	return &classedShadow{recordingShadow: *newRecordingShadow()}
}
func (c *classedShadow) RecordShadowCallFor(_ CallerClass, m EngineMethod, p CallPath) {
	c.RecordShadowCall(m, p)
}
func (c *classedShadow) RecordDivergenceFor(_ CallerClass, m EngineMethod, d DivergenceClass, cfg string, p CallPath) {
	c.RecordDivergence(m, d, cfg, p)
}
func (c *classedShadow) RecordNotComparedFor(_ CallerClass, r NotComparedReason) {
	c.RecordNotCompared(r)
}
func (c *classedShadow) RecordObservation(o ShadowObservation) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.obs = append(c.obs, o)
}

func TestEnforceAnswerPolicy_OverrideTruthTable_PerClassAndGuardPolicy(t *testing.T) {
	t.Parallel()
	const reason = "owner approved"
	noPass := BacklogItemTransitionInput{Status: BacklogStatusReview, OverrideReason: reason}

	type row struct {
		name   string
		class  CallerClass
		guards GuardPolicy
		gated  bool // a human_approval gate sits on review->done
		want   error
	}
	rows := []row{
		// No Configured gate: the Default guard's own exemption is all that override can free.
		{"operator frees the Default exemption", ClassOperator, GuardsBuiltIn, false, nil},
		{"agent frees the Default exemption", ClassAgent, GuardsBuiltIn, false, nil},
		{"system has no override, built-in guard denies", ClassSystem, GuardsBuiltIn, false, domain.ErrVerdictRequired},
		{"system with no guards passes", ClassSystem, GuardsNone, false, nil},
		// A Configured operator gate: only the operator's override bypasses it.
		{"operator override bypasses the Configured gate", ClassOperator, GuardsBuiltIn, true, nil},
		{"agent override never bypasses the Configured gate", ClassAgent, GuardsBuiltIn, true, ErrGateNotSatisfied},
		{"system hits the Configured gate", ClassSystem, GuardsNone, true, ErrGateNotSatisfied},
		{"unknown class fails closed to agent", CallerClass("bogus"), GuardsBuiltIn, true, ErrGateNotSatisfied},
	}
	for _, tc := range rows {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := enforceAll(newSeededSelectorHarness(t, ModeEnforce))
			if tc.gated {
				addGateToSeededEdge(t, h.client, h.cfg, BacklogStatusReview, BacklogStatusDone, GateKindHumanApproval, nil)
			}
			err := viewFor(h, tc.class, tc.guards).ValidateGates(noPass, BacklogStatusDone, nil)
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
	h := enforceAll(newSeededSelectorHarness(t, ModeEnforce))
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
	for _, class := range []CallerClass{ClassOperator, ClassAgent} {
		for _, tc := range cases {
			tc.item.OverrideReason = "please"
			require.ErrorIsf(t, viewFor(h, class, GuardsBuiltIn).ValidateGates(tc.item, tc.to, nil), tc.want, "%s %s", class, tc.name)
		}
	}
}

func TestEnforceAnswerPolicy_AgentOverrideFreesOnlyReviewAndPRPendingToReadyAndDone(t *testing.T) {
	t.Parallel()
	h := enforceAll(newSeededSelectorHarness(t, ModeEnforce))
	agent := viewFor(h, ClassAgent, GuardsBuiltIn)
	for _, from := range []BacklogStatus{BacklogStatusReview, BacklogStatusPRPending} {
		for _, to := range []BacklogStatus{BacklogStatusReady, BacklogStatusDone} {
			item := BacklogItemTransitionInput{Status: from, OverallOutcome: domain.ReviewOutcomePass}
			if to == BacklogStatusDone {
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
	h := enforceAll(newSeededSelectorHarness(t, ModeEnforce))
	addGateToSeededEdge(t, h.client, h.cfg, BacklogStatusReview, BacklogStatusDone, GateKindHumanApproval, nil)
	item := passingReviewItem()
	item.OverrideReason = "owner approved"

	unsatisfied := func(class CallerClass, guards GuardPolicy) int {
		statuses, err := viewFor(h, class, guards).PendingGates(item, BacklogStatusDone, nil)
		require.NoError(t, err)
		n := 0
		for _, s := range statuses {
			if !s.Satisfied {
				n++
			}
		}
		return n
	}
	require.Zero(t, unsatisfied(ClassOperator, GuardsBuiltIn), "operator override omits the Configured gate")
	require.Equal(t, 1, unsatisfied(ClassAgent, GuardsBuiltIn), "agent override keeps it")
	require.Equal(t, 1, unsatisfied(ClassSystem, GuardsNone), "system has no override")

	statuses, err := viewFor(h, ClassSystem, GuardsNone).PendingGates(item, BacklogStatusDone, nil)
	require.NoError(t, err)
	for _, s := range statuses {
		require.False(t, strings.HasPrefix(s.GateID, builtInGateIDPrefix), "GuardsNone lists no Default guard status")
	}
}

func TestForCaller_ForwardingUsesGraphWithoutSystemRequiredUnion(t *testing.T) {
	t.Parallel()
	h := enforceAll(newSeededSelectorHarness(t, ModeEnforce))
	setEdgeEnabled(t, h.client, h.cfg, BacklogStatusInProgress, BacklogStatusReview, false)

	delegate, ok := LookupTransitionCaller(CallerMCPTransitionDelegate)
	require.True(t, ok)
	require.True(t, delegate.Forwarding)
	fwd := h.sel.ForCaller(delegate)
	require.False(t, fwd.CanTransition(BacklogStatusInProgress, BacklogStatusReview, nil), "a disabled edge denies a forwarder")
	require.NotContains(t, fwd.AllowedTransitions(BacklogStatusInProgress, nil), BacklogStatusReview)
	require.True(t, viewFor(h, ClassAgent, GuardsBuiltIn).CanTransition(BacklogStatusInProgress, BacklogStatusReview, nil), "a fixed-edge agent keeps the union")
}

func TestForCaller_UncheckedCallerIsAllowedInOffAndShadow_AndEnforcesInEnforce(t *testing.T) {
	t.Parallel()
	spec, ok := LookupTransitionCaller(CallerMCPResumeWork) // agent, unchecked, ready->in_progress, built-in guards
	require.True(t, ok)
	require.Equal(t, LegacyUnchecked, spec.Legacy)
	noPlan := BacklogItemTransitionInput{Status: BacklogStatusReady}

	for _, mode := range []EngineMode{ModeOff, ModeShadow} {
		h := newSeededSelectorHarness(t, mode)
		h.sel.modes = allClasses(mode)
		v := h.sel.ForCaller(spec)
		require.True(t, v.CanTransition(BacklogStatusIdea, BacklogStatusDone, nil), "legacy answer is allow, mode %d", mode)
		require.NoError(t, v.ValidateGates(noPlan, BacklogStatusInProgress, nil), "mode %d", mode)
	}
	h := enforceAll(newSeededSelectorHarness(t, ModeEnforce))
	v := h.sel.ForCaller(spec)
	require.False(t, v.CanTransition(BacklogStatusIdea, BacklogStatusDone, nil))
	require.ErrorIs(t, v.ValidateGates(noPlan, BacklogStatusInProgress, nil), domain.ErrPlanRequired)
}

func TestShadow_UncheckedAgentGuardDenial_IsGuardNewLabelledWithClassAndCaller(t *testing.T) {
	t.Parallel()
	rec := newClassedShadow()
	h := shadowAll(newSeededSelectorHarness(t, ModeShadow, WithShadowRecorder(rec)))
	spec, _ := LookupTransitionCaller(CallerMCPResumeWork)
	v := h.sel.ForCaller(spec)

	require.NoError(t, v.ValidateGates(BacklogItemTransitionInput{Status: BacklogStatusReady}, BacklogStatusInProgress, nil))
	require.Equal(t, 1, rec.divergences[divergenceKey{MethodValidateGates, ClassGuardNew, "edited", PathTransition}])
	require.Len(t, rec.obs, 1)
	require.Equal(t, ClassGuardNew, rec.obs[0].Class)
	require.Equal(t, ClassAgent, rec.obs[0].CallerClass)
	require.Equal(t, CallerMCPResumeWork, rec.obs[0].Caller)
}

func TestShadow_UncheckedCallerOperatorGate_IsGateExtraNotGuardNew(t *testing.T) {
	t.Parallel()
	rec := newClassedShadow()
	h := shadowAll(newSeededSelectorHarness(t, ModeShadow, WithShadowRecorder(rec)))
	addGateToSeededEdge(t, h.client, h.cfg, BacklogStatusReady, BacklogStatusInProgress, GateKindStructural,
		map[string]interface{}{"check_id": StructuralCheckACComplete})
	spec, _ := LookupTransitionCaller(CallerMCPResumeWork) // agent, unchecked, ready->in_progress, built-in guards
	v := h.sel.ForCaller(spec)

	item := BacklogItemTransitionInput{Status: BacklogStatusReady, SkipPlanning: true, PlanArtifactsPath: "/plan"} // Default guard passes
	require.NoError(t, v.ValidateGates(item, BacklogStatusInProgress, nil))
	require.Equal(t, 1, rec.divergences[divergenceKey{MethodValidateGates, ClassGateExtra, "edited", PathTransition}])
	require.Zero(t, rec.divergences[divergenceKey{MethodValidateGates, ClassGuardNew, "edited", PathTransition}])
}

func TestShadow_UncheckedCallers_SeededConfig_ZeroDivergenceExceptGuardNewForBuiltInGuardCallers(t *testing.T) {
	t.Parallel()
	rec := newClassedShadow()
	h := shadowAll(newSeededSelectorHarness(t, ModeShadow, WithShadowRecorder(rec)))
	statuses := append([]BacklogStatus(nil), builtInStageOrder...)

	known := map[string]bool{}
	for _, k := range KnownOffGraphCallerEdges() {
		known[string(k.Caller)+"|"+EdgeKey(k.Edge.From, k.Edge.To)] = true
	}
	calls := 0
	for _, spec := range TransitionCallers() {
		if spec.Legacy != LegacyUnchecked || spec.Forwarding {
			continue
		}
		v := h.sel.ForCaller(spec)
		for _, e := range spec.Edges {
			froms := []BacklogStatus{e.From}
			if e.From == AnyStatus {
				froms = statuses
			}
			for _, from := range froms {
				if known[string(spec.Name)+"|"+EdgeKey(from, e.To)] {
					continue // a tolerated caller bug (Task 1.5.5c), reported as a graph divergence by design
				}
				calls++
				v.CanTransition(from, e.To, nil)
				_ = v.ValidateGates(BacklogItemTransitionInput{Status: from}, e.To, nil)
			}
		}
	}
	require.Positive(t, calls)
	for _, o := range rec.obs {
		if o.Class == "" {
			continue
		}
		require.Equalf(t, ClassGuardNew, o.Class, "%s %s->%s: only guard_new is expected", o.Caller, o.From, o.To)
		require.NotEqualf(t, ClassSystem, o.CallerClass, "%s: system callers carry GuardsNone, so no guard_new", o.Caller)
		t.Logf("guard_new for owner acknowledgement: class=%s caller=%s %s->%s", o.CallerClass, o.Caller, o.From, o.To)
	}
	for k, n := range rec.divergences {
		require.Equalf(t, ClassGuardNew, k.Class, "%+v x%d", k, n)
	}
}

func TestModeFor_FallsBackToOperatorModeAndCapsOtherClassesAtShadow_WhenSourceIsNotClassAware(t *testing.T) {
	t.Parallel()
	mk := func(m EngineMode) *SwitchableWorkflowEngine {
		return NewSwitchableWorkflowEngine(NewDefaultWorkflowEngine(), nil, fixedMode{m})
	}
	require.Equal(t, ModeEnforce, mk(ModeEnforce).ModeFor(ClassOperator))
	require.Equal(t, ModeShadow, mk(ModeEnforce).ModeFor(ClassAgent), "no enforce flag exists for agent")
	require.Equal(t, ModeShadow, mk(ModeShadow).ModeFor(ClassSystem))
	require.Equal(t, ModeOff, mk(ModeOff).ModeFor(ClassAgent))
	require.Equal(t, ModeOff, NewSwitchableWorkflowEngine(NewDefaultWorkflowEngine(), nil, nil).ModeFor(ClassOperator))
	require.Equal(t, ModeEnforce, NewSwitchableWorkflowEngine(NewDefaultWorkflowEngine(), nil, classModes{ClassAgent: ModeEnforce}).ModeFor(ClassAgent))
}

func TestOperatorMethods_AreTheOperatorPolicyView(t *testing.T) {
	t.Parallel()
	h := enforceAll(newSeededSelectorHarness(t, ModeEnforce))
	addGateToSeededEdge(t, h.client, h.cfg, BacklogStatusReview, BacklogStatusDone, GateKindHumanApproval, nil)
	item := passingReviewItem()
	item.OverrideReason = "owner approved"
	require.NoError(t, h.sel.ValidateGates(item, BacklogStatusDone, nil))
	require.NoError(t, viewFor(h, ClassOperator, GuardsBuiltIn).ValidateGates(item, BacklogStatusDone, nil))
	require.ErrorIs(t, viewFor(h, ClassAgent, GuardsBuiltIn).ValidateGates(item, BacklogStatusDone, nil), ErrGateNotSatisfied)
}
