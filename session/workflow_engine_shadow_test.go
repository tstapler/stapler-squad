package session

import (
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/session/domain"
)

type shadowRig struct {
	sel   *SwitchableWorkflowEngine
	rec   *ShadowLedgerRecorder
	clock *selectorClock
	dir   string
	h     *selectorHarness
}

// newShadowRig builds a Shadow-mode selector over a seeded engine whose
// recorder is the real ledger recorder (the recording fake is bypassed).
func newShadowRig(t *testing.T, label string, opts ...SelectorOption) *shadowRig {
	t.Helper()
	cfg, client := newSeededConfiguredWorkflowEngine(t)
	clock := newSelectorClock()
	dir := t.TempDir()
	rec := NewShadowLedgerRecorder(dir, WithShadowClock(clock))
	base := make([]SelectorOption, 0, 4+len(opts))
	base = append(base,
		WithShadowRecorder(rec), WithSelectorClock(clock), WithConfigLabeler(func() string { return label }),
		WithSelectorLoggers(func(string, ...any) {}, func(string, ...any) {}),
	)
	sel := NewSwitchableWorkflowEngine(NewDefaultWorkflowEngine(), cfg, fixedMode{ModeShadow}, append(base, opts...)...)
	return &shadowRig{sel: sel, rec: rec, clock: clock, dir: dir, h: &selectorHarness{sel: sel, cfg: cfg, client: client}}
}

func mustEmptyConfigured(t *testing.T) *ConfiguredWorkflowEngine {
	t.Helper()
	cfg, err := NewConfiguredWorkflowEngine(NewEntStageConfigRepository(NewTestEntRepository(t).client), nil, nil)
	require.NoError(t, err)
	return cfg
}

func (r *shadowRig) operator() *ClassLedger { return r.rec.Snapshot().Classes[string(ClassOperator)] }

func totalDivergenceCount(cl *ClassLedger) int64 {
	var n int64
	for _, v := range cl.Divergence {
		n += v
	}
	return n
}

func TestShadowRecorder_DoesNotCountGuardDenial_WhenSeededConfigAndFortyDenialsIn500Calls(t *testing.T) {
	t.Parallel()
	rig := newShadowRig(t, ConfigSeeded)
	idea := BacklogItemTransitionInput{Status: BacklogStatusIdea}
	pass := passingReviewItem()

	for i := 0; i < 500; i++ {
		if i < 40 {
			require.ErrorIs(t, rig.sel.ValidateGates(idea, BacklogStatusReady, nil), domain.ErrACRequired)
			continue
		}
		require.NoError(t, rig.sel.ValidateGates(pass, BacklogStatusPRPending, nil))
	}

	cl := rig.operator()
	require.Equal(t, int64(500), cl.Calls["ValidateGates|transition"])
	require.Zero(t, totalDivergenceCount(cl), "an ordinary guard denial is not a divergence")
	require.Equal(t, int64(500), cl.ConfigSeen[ConfigSeeded])
}

func TestShadowRecorder_ClassifiesDivergence_WhenGraphGateExtraErrorKindCacheEmpty(t *testing.T) {
	t.Parallel()
	rig := newShadowRig(t, ConfigEdited)
	setEdgeEnabled(t, rig.h.client, rig.h.cfg, BacklogStatusDone, BacklogStatusRefining, false)
	addGateToSeededEdge(t, rig.h.client, rig.h.cfg, BacklogStatusReady, BacklogStatusInProgress, GateKindStructural,
		map[string]interface{}{"check_id": StructuralCheckACComplete})

	require.True(t, rig.sel.CanTransition(BacklogStatusDone, BacklogStatusRefining, nil), "shadow returns the Default answer")
	item := BacklogItemTransitionInput{Status: BacklogStatusReady, SkipPlanning: true, PlanArtifactsPath: "/plan"}
	require.NoError(t, rig.sel.ValidateGates(item, BacklogStatusInProgress, nil))

	cl := rig.operator()
	require.Equal(t, int64(1), cl.Divergence["CanTransition|graph|edited|transition"])
	require.Equal(t, int64(1), cl.Divergence["ValidateGates|gate_extra|edited|transition"])
	require.Equal(t, int64(1), cl.Edges["done->refining"].Divergences)

	// error_kind: Default and the candidate deny with different errors.
	require.Equal(t, ClassErrorKind, compareGateErrors(domain.ErrACRequired, errors.New("other")))
	rig.rec.RecordDivergence(MethodValidateGates, ClassErrorKind, ConfigEdited, PathTransition)
	require.Equal(t, int64(1), rig.operator().Divergence["ValidateGates|error_kind|edited|transition"])

	// cache_empty: the candidate falls back to Default, so it never diverges.
	empty := NewSwitchableWorkflowEngine(NewDefaultWorkflowEngine(), mustEmptyConfigured(t), fixedMode{ModeShadow},
		WithShadowRecorder(rig.rec), WithConfigLabeler(func() string { return ConfigSeeded }))
	before := totalDivergenceCount(rig.operator())
	require.True(t, empty.CanTransition(BacklogStatusIdea, BacklogStatusReady, nil))
	require.Equal(t, before, totalDivergenceCount(rig.operator()))
}

func TestShadowRecorder_CountsGateMissing_WhenCompositionDefectInjected(t *testing.T) {
	t.Parallel()
	rig := newShadowRig(t, ConfigSeeded)
	rig.sel.enf = enforceAnswer{def: guardDroppingEngine{NewDefaultWorkflowEngine()}, cfg: rig.h.cfg}

	require.ErrorIs(t, rig.sel.ValidateGates(BacklogItemTransitionInput{Status: BacklogStatusIdea}, BacklogStatusReady, nil), domain.ErrACRequired)

	cl := rig.operator()
	require.Equal(t, int64(1), cl.Divergence["ValidateGates|gate_missing|seeded|transition"])
	v := EvaluateSoak(ClassOperator, rig.rec.Snapshot(), DefaultSoakParams(ClassOperator, rig.clock.Now()))
	require.Equal(t, 1, v.ExitCode)
	require.Equal(t, int64(1), v.Alarms[string(ClassGateMissing)])
}

// guardDroppingEngine is a composition defect: the Default guard half drops every guard.
type guardDroppingEngine struct{ *DefaultWorkflowEngine }

func (guardDroppingEngine) ValidateGates(BacklogItemTransitionInput, BacklogStatus, *StageConfigSnapshot) error {
	return nil
}

func TestShadowRecorder_SeparatesReadAndTransitionPaths_WhenUiCallsAllowedTransitions(t *testing.T) {
	t.Parallel()
	rig := newShadowRig(t, ConfigSeeded, WithReadSampler(func() bool { return true }))

	for i := 0; i < 7; i++ {
		rig.sel.AllowedTransitions(BacklogStatusIdea, nil)
	}
	for i := 0; i < 3; i++ {
		rig.sel.CanTransition(BacklogStatusIdea, BacklogStatusReady, nil)
	}

	cl := rig.operator()
	require.Equal(t, int64(7), cl.Calls["AllowedTransitions|read"])
	require.Equal(t, int64(3), cl.Calls["CanTransition|transition"])
	require.Zero(t, cl.Calls["AllowedTransitions|transition"])
	require.Equal(t, int64(3), cl.Edges["idea->ready"].TransitionSamples)
	require.Equal(t, int64(7), cl.Edges["idea->*"].ReadSamples)
}

func TestShadowRecorderLedger_ResumesCountersAndBinaryStartedAt_WhenRestarted(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	clock := newSelectorClock()
	first := NewShadowLedgerRecorder(dir, WithShadowClock(clock))
	first.RecordShadowCall(MethodCanTransition, PathTransition)
	first.RecordObservation(ShadowObservation{Method: MethodCanTransition, Path: PathTransition, From: BacklogStatusReady, To: BacklogStatusQueued, Config: ConfigSeeded})
	require.NoError(t, first.FlushIfDue())
	firstStart := first.Snapshot().BinaryStarts[0]

	clock.Advance(30 * time.Second)
	require.NoError(t, first.FlushIfDue(), "not due yet")
	info1, err := os.Stat(ShadowLedgerPath(dir))
	require.NoError(t, err)

	clock.Advance(2 * time.Hour)
	second := NewShadowLedgerRecorder(dir, WithShadowClock(clock))
	second.RecordShadowCall(MethodCanTransition, PathTransition)
	second.RecordObservation(ShadowObservation{Method: MethodCanTransition, Path: PathTransition, From: BacklogStatusReady, To: BacklogStatusQueued, Config: ConfigSeeded})

	snap := second.Snapshot()
	require.Equal(t, int64(2), snap.Classes[string(ClassOperator)].Calls["CanTransition|transition"], "counters resume from the ledger")
	require.Equal(t, int64(2), snap.Classes[string(ClassOperator)].Edges["ready->queued"].TransitionSamples)
	require.Len(t, snap.BinaryStarts, 2)
	require.Equal(t, firstStart, snap.BinaryStarts[0], "the earliest start is kept")
	require.True(t, snap.BinaryStarts[1].After(firstStart))
	require.NotNil(t, info1)
}

func TestShadowRecorderLedger_KeepsRingOf200ReproRecords_UsesConfigDirAndStartsFreshWhenCorrupt(t *testing.T) {
	t.Parallel()
	rec := NewShadowLedgerRecorder(t.TempDir(), WithShadowClock(newSelectorClock()))
	for i := 0; i < 250; i++ {
		rec.RecordObservation(ShadowObservation{
			Method: MethodValidateGates, Path: PathTransition, From: BacklogStatusReady, To: BacklogStatusQueued,
			ItemHash: HashItemID("item-secret-id"), Class: ClassGateExtra, Config: ConfigEdited, Default: "ok", Enforce: "gate",
		})
	}
	repro := rec.Snapshot().Repro
	require.Len(t, repro, reproRingSize)
	for _, r := range repro {
		require.NotContains(t, r.ItemHash, "item-secret-id", "item ids are hashed")
		require.Equal(t, HashItemID("item-secret-id"), r.ItemHash)
	}

	dir, err := ShadowLedgerDir()
	require.NoError(t, err)
	want, err := config.GetConfigDir()
	require.NoError(t, err)
	require.Equal(t, want, dir, "the ledger lives in the instance-aware config dir")

	var warns []string
	warn := WithShadowWarn(func(f string, a ...any) { warns = append(warns, f) })
	corruptDir := t.TempDir()
	require.NoError(t, os.WriteFile(ShadowLedgerPath(corruptDir), []byte("{not json"), 0o600))
	fresh := NewShadowLedgerRecorder(corruptDir, warn)
	require.Empty(t, fresh.Snapshot().Classes[string(ClassOperator)].Calls)
	require.Len(t, warns, 1)

	oversizedDir := t.TempDir()
	require.NoError(t, os.WriteFile(ShadowLedgerPath(oversizedDir), []byte(strings.Repeat(" ", maxLedgerBytes+1)), 0o600))
	NewShadowLedgerRecorder(oversizedDir, warn)
	require.Len(t, warns, 2, "an oversized ledger also starts fresh with a WARN")
}

func TestShadowLedgerRecorder_BoundsDistinctEdges_WhenCustomStagesExplode(t *testing.T) {
	t.Parallel()
	rec := NewShadowLedgerRecorder("", WithShadowClock(newSelectorClock()))
	for i := 0; i < maxLedgerEdges+50; i++ {
		rec.RecordObservation(ShadowObservation{Path: PathTransition, From: BacklogStatus(string(rune('a'+i%26)) + strings.Repeat("x", i)), To: BacklogStatusDone})
	}
	require.LessOrEqual(t, len(rec.Snapshot().Classes[string(ClassOperator)].Edges), maxLedgerEdges+1)
	require.Positive(t, rec.Snapshot().Classes[string(ClassOperator)].Edges[otherEdgeKey].TransitionSamples)
}

func TestShadowLedgerRecorder_IsRaceFree_WhenRecordedAndSnapshottedConcurrently(t *testing.T) {
	t.Parallel()
	rec := NewShadowLedgerRecorder(t.TempDir(), WithShadowClock(newSelectorClock()))
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				rec.RecordShadowCall(MethodCanTransition, PathTransition)
				rec.RecordDivergence(MethodCanTransition, ClassGraph, ConfigEdited, PathTransition)
				rec.RecordObservation(ShadowObservation{Path: PathTransition, From: BacklogStatusReady, To: BacklogStatusQueued, Class: ClassGraph, Config: ConfigEdited})
				if i%50 == 0 {
					_ = rec.Snapshot()
					require.NoError(t, rec.Flush())
				}
			}
		}()
	}
	wg.Wait()
	require.Equal(t, int64(1600), rec.Snapshot().Classes[string(ClassOperator)].Calls["CanTransition|transition"])
}

func TestNewConfigLabeler_LabelsSeededEditedAndUnlabeled_AndCachesFingerprint(t *testing.T) {
	t.Parallel()
	clock := newSelectorClock()
	calls := 0
	fp := SeedFingerprint()
	var err error
	label := NewConfigLabeler(func() (string, error) { calls++; return fp, err }, clock)

	require.Equal(t, ConfigSeeded, label())
	require.Equal(t, ConfigSeeded, label())
	require.Equal(t, 1, calls, "cached within the refresh window")

	fp = "different"
	clock.Advance(labelerRefreshWindow + time.Second)
	require.Equal(t, ConfigEdited, label())

	err = errors.New("db down")
	clock.Advance(labelerRefreshWindow + time.Second)
	require.Equal(t, ConfigUnlabeled, label())
}

// --- Story 1.5.2: per-class ledger sections and the guard_new class ----------

func TestShadowLedger_KeepsSeparateSectionPerClass_AndCountsDivergencePerCaller(t *testing.T) {
	t.Parallel()
	rig := newShadowRig(t, ConfigSeeded)
	rig.sel.modes = classModes{ClassOperator: ModeShadow, ClassAgent: ModeShadow, ClassSystem: ModeShadow}
	spec, ok := LookupTransitionCaller(CallerMCPResumeWork) // agent, unchecked, built-in guards
	require.True(t, ok)
	agent := rig.sel.ForCaller(spec)

	for i := 0; i < 3; i++ {
		require.NoError(t, agent.ValidateGates(BacklogItemTransitionInput{Status: BacklogStatusReady}, BacklogStatusInProgress, nil))
	}
	_ = rig.sel.CanTransition(BacklogStatusReady, BacklogStatusQueued, nil) // operator policy

	snap := rig.rec.Snapshot()
	ag, op := snap.Classes[string(ClassAgent)], snap.Classes[string(ClassOperator)]
	require.NotNil(t, ag)
	require.Equal(t, int64(3), ag.Calls["ValidateGates|transition"])
	require.Equal(t, int64(3), ag.Divergence["ValidateGates|guard_new|seeded|transition"])
	require.Equal(t, int64(3), ag.CallerDivergence[string(CallerMCPResumeWork)+"|guard_new"])
	require.Equal(t, int64(3), ag.Edges["ready->in_progress"].Divergences)
	require.Equal(t, int64(1), op.Calls["CanTransition|transition"], "operator section holds only the operator call")
	require.Zero(t, totalDivergenceCount(op))
	require.Nil(t, snap.Classes[string(ClassSystem)], "an untouched class gets no section")

	require.NotEmpty(t, snap.Repro)
	last := snap.Repro[len(snap.Repro)-1]
	require.Equal(t, string(ClassAgent), last.Class)
	require.Equal(t, string(CallerMCPResumeWork), last.Caller)
	require.Equal(t, string(ClassGuardNew), last.Divergence)
}

func TestShadowLedger_PerClassSectionsSurviveFlushAndRestart(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	clock := newSelectorClock()
	first := NewShadowLedgerRecorder(dir, WithShadowClock(clock))
	first.RecordShadowCallFor(ClassAgent, MethodValidateGates, PathTransition)
	first.RecordDivergenceFor(ClassAgent, MethodValidateGates, ClassGuardNew, ConfigSeeded, PathTransition)
	first.RecordObservation(ShadowObservation{
		Method: MethodValidateGates, Path: PathTransition, From: BacklogStatusReady, To: BacklogStatusInProgress,
		Class: ClassGuardNew, Config: ConfigSeeded, CallerClass: ClassAgent, Caller: CallerMCPResumeWork,
	})
	first.RecordShadowCallFor(ClassSystem, MethodCanTransition, PathTransition)
	require.NoError(t, first.Flush())

	second := NewShadowLedgerRecorder(dir, WithShadowClock(clock))
	second.RecordShadowCallFor(ClassAgent, MethodValidateGates, PathTransition)
	snap := second.Snapshot()
	require.Equal(t, int64(2), snap.Classes[string(ClassAgent)].Calls["ValidateGates|transition"])
	require.Equal(t, int64(1), snap.Classes[string(ClassAgent)].CallerDivergence[string(CallerMCPResumeWork)+"|guard_new"])
	require.Equal(t, int64(1), snap.Classes[string(ClassSystem)].Calls["CanTransition|transition"])
}

func TestShadowLedgerRecorder_UnclassedMethodsRecordIntoTheRecordersOwnClass(t *testing.T) {
	t.Parallel()
	rec := NewShadowLedgerRecorder("", WithShadowClass(ClassAgent))
	rec.RecordShadowCall(MethodCanTransition, PathTransition)
	rec.RecordShadowCallFor(CallerClass("bogus"), MethodCanTransition, PathTransition)
	require.Equal(t, int64(2), rec.Snapshot().Classes[string(ClassAgent)].Calls["CanTransition|transition"], "an unknown class falls back to the recorder's own")
}
