package shadowledger

import (
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/internal/sessiontest"
)

type shadowRig struct {
	sel   *session.SwitchableWorkflowEngine
	rec   *ShadowLedgerRecorder
	clock *sessiontest.SelectorClock
	dir   string
	h     *selectorHarness
}

// newShadowRig builds a Shadow-mode selector over a seeded engine whose
// recorder is the real ledger recorder (the recording fake is bypassed).
func newShadowRig(t *testing.T, label string, opts ...session.SelectorOption) *shadowRig {
	t.Helper()
	cfg, client := newSeededConfiguredWorkflowEngine(t)
	clock := sessiontest.NewSelectorClock()
	dir := t.TempDir()
	rec := NewShadowLedgerRecorder(dir, WithShadowClock(clock))
	base := make([]session.SelectorOption, 0, 4+len(opts))
	base = append(base,
		session.WithShadowRecorder(rec), session.WithSelectorClock(clock), session.WithConfigLabeler(func() string { return label }),
		session.WithSelectorLoggers(func(string, ...any) {}, func(string, ...any) {}),
	)
	sel := session.NewSwitchableWorkflowEngine(session.NewDefaultWorkflowEngine(), cfg, fixedMode{session.ModeShadow}, append(base, opts...)...)
	return &shadowRig{sel: sel, rec: rec, clock: clock, dir: dir, h: &selectorHarness{sel: sel, cfg: cfg, client: client}}
}

func mustEmptyConfigured(t *testing.T) *session.ConfiguredWorkflowEngine {
	t.Helper()
	cfg, err := session.NewConfiguredWorkflowEngine(session.NewEntStageConfigRepository(session.NewTestEntRepository(t).Client), nil, nil)
	require.NoError(t, err)
	return cfg
}

func (r *shadowRig) operator() *session.ClassLedger {
	return r.rec.Snapshot().Classes[string(domain.ClassOperator)]
}

func totalDivergenceCount(cl *session.ClassLedger) int64 {
	var n int64
	for _, v := range cl.Divergence {
		n += v
	}
	return n
}

func TestShadowRecorder_DoesNotCountGuardDenial_WhenSeededConfigAndFortyDenialsIn500Calls(t *testing.T) {
	t.Parallel()
	rig := newShadowRig(t, session.ConfigSeeded)
	idea := domain.BacklogItemTransitionInput{Status: domain.BacklogStatusIdea}
	pass := passingReviewItem()

	for i := 0; i < 500; i++ {
		if i < 40 {
			require.ErrorIs(t, rig.sel.ValidateGates(idea, domain.BacklogStatusReady, nil), domain.ErrACRequired)
			continue
		}
		require.NoError(t, rig.sel.ValidateGates(pass, domain.BacklogStatusPRPending, nil))
	}

	cl := rig.operator()
	require.Equal(t, int64(500), cl.Calls["ValidateGates|transition"])
	require.Zero(t, totalDivergenceCount(cl), "an ordinary guard denial is not a divergence")
	require.Equal(t, int64(500), cl.ConfigSeen[session.ConfigSeeded])
}

func TestShadowRecorder_ClassifiesDivergence_WhenGraphGateExtraErrorKindCacheEmpty(t *testing.T) {
	t.Parallel()
	rig := newShadowRig(t, session.ConfigEdited)
	setEdgeEnabled(t, rig.h.client, rig.h.cfg, domain.BacklogStatusDone, domain.BacklogStatusRefining, false)
	addGateToSeededEdge(t, rig.h.client, rig.h.cfg, domain.BacklogStatusReady, domain.BacklogStatusInProgress, domain.GateKindStructural,
		map[string]interface{}{"check_id": domain.StructuralCheckACComplete})

	require.True(t, rig.sel.CanTransition(domain.BacklogStatusDone, domain.BacklogStatusRefining, nil), "shadow returns the Default answer")
	item := domain.BacklogItemTransitionInput{Status: domain.BacklogStatusReady, SkipPlanning: true, PlanArtifactsPath: "/plan"}
	require.NoError(t, rig.sel.ValidateGates(item, domain.BacklogStatusInProgress, nil))

	cl := rig.operator()
	require.Equal(t, int64(1), cl.Divergence["CanTransition|graph|edited|transition"])
	require.Equal(t, int64(1), cl.Divergence["ValidateGates|gate_extra|edited|transition"])
	require.Equal(t, int64(1), cl.Edges["done->refining"].Divergences)

	// error_kind: Default and the candidate deny with different errors.
	require.Equal(t, session.ClassErrorKind, session.CompareGateErrors(domain.ErrACRequired, errors.New("other")))
	rig.rec.RecordDivergence(session.MethodValidateGates, session.ClassErrorKind, session.ConfigEdited, session.PathTransition)
	require.Equal(t, int64(1), rig.operator().Divergence["ValidateGates|error_kind|edited|transition"])

	// cache_empty: the candidate falls back to Default, so it never diverges.
	empty := session.NewSwitchableWorkflowEngine(session.NewDefaultWorkflowEngine(), mustEmptyConfigured(t), fixedMode{session.ModeShadow},
		session.WithShadowRecorder(rig.rec), session.WithConfigLabeler(func() string { return session.ConfigSeeded }))
	before := totalDivergenceCount(rig.operator())
	require.True(t, empty.CanTransition(domain.BacklogStatusIdea, domain.BacklogStatusReady, nil))
	require.Equal(t, before, totalDivergenceCount(rig.operator()))
}

func TestShadowRecorder_CountsGateMissing_WhenCompositionDefectInjected(t *testing.T) {
	t.Parallel()
	rig := newShadowRig(t, session.ConfigSeeded)
	rig.sel.Enf = session.NewEnforceAnswer(guardDroppingEngine{session.NewDefaultWorkflowEngine()}, rig.h.cfg)

	require.ErrorIs(t, rig.sel.ValidateGates(domain.BacklogItemTransitionInput{Status: domain.BacklogStatusIdea}, domain.BacklogStatusReady, nil), domain.ErrACRequired)

	cl := rig.operator()
	require.Equal(t, int64(1), cl.Divergence["ValidateGates|gate_missing|seeded|transition"])
	v := session.EvaluateSoak(domain.ClassOperator, rig.rec.Snapshot(), session.DefaultSoakParams(domain.ClassOperator, rig.clock.Now()))
	require.Equal(t, 1, v.ExitCode)
	require.Equal(t, int64(1), v.Alarms[string(session.ClassGateMissing)])
}

// guardDroppingEngine is a composition defect: the Default guard half drops every guard.
type guardDroppingEngine struct{ *session.DefaultWorkflowEngine }

func (guardDroppingEngine) ValidateGates(domain.BacklogItemTransitionInput, domain.BacklogStatus, *domain.StageConfigSnapshot) error {
	return nil
}

func TestShadowRecorder_SeparatesReadAndTransitionPaths_WhenUiCallsAllowedTransitions(t *testing.T) {
	t.Parallel()
	rig := newShadowRig(t, session.ConfigSeeded, session.WithReadSampler(func() bool { return true }))

	for i := 0; i < 7; i++ {
		rig.sel.AllowedTransitions(domain.BacklogStatusIdea, nil)
	}
	for i := 0; i < 3; i++ {
		rig.sel.CanTransition(domain.BacklogStatusIdea, domain.BacklogStatusReady, nil)
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
	clock := sessiontest.NewSelectorClock()
	first := NewShadowLedgerRecorder(dir, WithShadowClock(clock))
	first.RecordShadowCall(session.MethodCanTransition, session.PathTransition)
	first.RecordObservation(session.ShadowObservation{Method: session.MethodCanTransition, Path: session.PathTransition, From: domain.BacklogStatusReady, To: domain.BacklogStatusQueued, Config: session.ConfigSeeded})
	require.NoError(t, first.FlushIfDue())
	firstStart := first.Snapshot().BinaryStarts[0]

	clock.Advance(30 * time.Second)
	require.NoError(t, first.FlushIfDue(), "not due yet")
	info1, err := os.Stat(ShadowLedgerPath(dir))
	require.NoError(t, err)

	clock.Advance(2 * time.Hour)
	second := NewShadowLedgerRecorder(dir, WithShadowClock(clock))
	second.RecordShadowCall(session.MethodCanTransition, session.PathTransition)
	second.RecordObservation(session.ShadowObservation{Method: session.MethodCanTransition, Path: session.PathTransition, From: domain.BacklogStatusReady, To: domain.BacklogStatusQueued, Config: session.ConfigSeeded})

	snap := second.Snapshot()
	require.Equal(t, int64(2), snap.Classes[string(domain.ClassOperator)].Calls["CanTransition|transition"], "counters resume from the ledger")
	require.Equal(t, int64(2), snap.Classes[string(domain.ClassOperator)].Edges["ready->queued"].TransitionSamples)
	require.Len(t, snap.BinaryStarts, 2)
	require.Equal(t, firstStart, snap.BinaryStarts[0], "the earliest start is kept")
	require.True(t, snap.BinaryStarts[1].After(firstStart))
	require.NotNil(t, info1)
}

func TestShadowRecorderLedger_KeepsRingOf200ReproRecords_UsesConfigDirAndStartsFreshWhenCorrupt(t *testing.T) {
	t.Parallel()
	rec := NewShadowLedgerRecorder(t.TempDir(), WithShadowClock(sessiontest.NewSelectorClock()))
	for i := 0; i < 250; i++ {
		rec.RecordObservation(session.ShadowObservation{
			Method: session.MethodValidateGates, Path: session.PathTransition, From: domain.BacklogStatusReady, To: domain.BacklogStatusQueued,
			ItemHash: session.HashItemID("item-secret-id"), Class: session.ClassGateExtra, Config: session.ConfigEdited, Default: "ok", Enforce: "gate",
		})
	}
	repro := rec.Snapshot().Repro
	require.Len(t, repro, reproRingSize)
	for _, r := range repro {
		require.NotContains(t, r.ItemHash, "item-secret-id", "item ids are hashed")
		require.Equal(t, session.HashItemID("item-secret-id"), r.ItemHash)
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
	require.Empty(t, fresh.Snapshot().Classes[string(domain.ClassOperator)].Calls)
	require.Len(t, warns, 1)

	oversizedDir := t.TempDir()
	require.NoError(t, os.WriteFile(ShadowLedgerPath(oversizedDir), []byte(strings.Repeat(" ", maxLedgerBytes+1)), 0o600))
	NewShadowLedgerRecorder(oversizedDir, warn)
	require.Len(t, warns, 2, "an oversized ledger also starts fresh with a WARN")
}

func TestShadowLedgerRecorder_BoundsDistinctEdges_WhenCustomStagesExplode(t *testing.T) {
	t.Parallel()
	rec := NewShadowLedgerRecorder("", WithShadowClock(sessiontest.NewSelectorClock()))
	for i := 0; i < maxLedgerEdges+50; i++ {
		rec.RecordObservation(session.ShadowObservation{Path: session.PathTransition, From: domain.BacklogStatus(string(rune('a'+i%26)) + strings.Repeat("x", i)), To: domain.BacklogStatusDone})
	}
	require.LessOrEqual(t, len(rec.Snapshot().Classes[string(domain.ClassOperator)].Edges), maxLedgerEdges+1)
	require.Positive(t, rec.Snapshot().Classes[string(domain.ClassOperator)].Edges[otherEdgeKey].TransitionSamples)
}

func TestShadowLedgerRecorder_IsRaceFree_WhenRecordedAndSnapshottedConcurrently(t *testing.T) {
	t.Parallel()
	rec := NewShadowLedgerRecorder(t.TempDir(), WithShadowClock(sessiontest.NewSelectorClock()))
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				rec.RecordShadowCall(session.MethodCanTransition, session.PathTransition)
				rec.RecordDivergence(session.MethodCanTransition, session.ClassGraph, session.ConfigEdited, session.PathTransition)
				rec.RecordObservation(session.ShadowObservation{Path: session.PathTransition, From: domain.BacklogStatusReady, To: domain.BacklogStatusQueued, Class: session.ClassGraph, Config: session.ConfigEdited})
				if i%50 == 0 {
					_ = rec.Snapshot()
					require.NoError(t, rec.Flush())
				}
			}
		}()
	}
	wg.Wait()
	require.Equal(t, int64(1600), rec.Snapshot().Classes[string(domain.ClassOperator)].Calls["CanTransition|transition"])
}

func TestNewConfigLabeler_LabelsSeededEditedAndUnlabeled_AndCachesFingerprint(t *testing.T) {
	t.Parallel()
	clock := sessiontest.NewSelectorClock()
	calls := 0
	fp := session.SeedFingerprint()
	var err error
	label := NewConfigLabeler(func() (string, error) { calls++; return fp, err }, clock)

	require.Equal(t, session.ConfigSeeded, label())
	require.Equal(t, session.ConfigSeeded, label())
	require.Equal(t, 1, calls, "cached within the refresh window")

	fp = "different"
	clock.Advance(labelerRefreshWindow + time.Second)
	require.Equal(t, session.ConfigEdited, label())

	err = errors.New("db down")
	clock.Advance(labelerRefreshWindow + time.Second)
	require.Equal(t, session.ConfigUnlabeled, label())
}

func TestShadowLedger_KeepsSeparateSectionPerClass_AndCountsDivergencePerCaller(t *testing.T) {
	t.Parallel()
	rig := newShadowRig(t, session.ConfigSeeded)
	rig.sel.Modes = classModes{domain.ClassOperator: session.ModeShadow, domain.ClassAgent: session.ModeShadow, domain.ClassSystem: session.ModeShadow}
	spec, ok := domain.LookupTransitionCaller(domain.CallerMCPResumeWork) // agent, unchecked, built-in guards
	require.True(t, ok)
	agent := rig.sel.ForCaller(spec)

	for i := 0; i < 3; i++ {
		require.NoError(t, agent.ValidateGates(domain.BacklogItemTransitionInput{Status: domain.BacklogStatusReady}, domain.BacklogStatusInProgress, nil))
	}
	_ = rig.sel.CanTransition(domain.BacklogStatusReady, domain.BacklogStatusQueued, nil) // operator policy

	snap := rig.rec.Snapshot()
	ag, op := snap.Classes[string(domain.ClassAgent)], snap.Classes[string(domain.ClassOperator)]
	require.NotNil(t, ag)
	require.Equal(t, int64(3), ag.Calls["ValidateGates|transition"])
	require.Equal(t, int64(3), ag.Divergence["ValidateGates|guard_new|seeded|transition"])
	require.Equal(t, int64(3), ag.CallerDivergence[string(domain.CallerMCPResumeWork)+"|guard_new"])
	require.Equal(t, int64(3), ag.Edges["ready->in_progress"].Divergences)
	require.Equal(t, int64(1), op.Calls["CanTransition|transition"], "operator section holds only the operator call")
	require.Zero(t, totalDivergenceCount(op))
	require.Nil(t, snap.Classes[string(domain.ClassSystem)], "an untouched class gets no section")

	require.NotEmpty(t, snap.Repro)
	last := snap.Repro[len(snap.Repro)-1]
	require.Equal(t, string(domain.ClassAgent), last.Class)
	require.Equal(t, string(domain.CallerMCPResumeWork), last.Caller)
	require.Equal(t, string(session.ClassGuardNew), last.Divergence)
}

func TestShadowLedger_PerClassSectionsSurviveFlushAndRestart(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	clock := sessiontest.NewSelectorClock()
	first := NewShadowLedgerRecorder(dir, WithShadowClock(clock))
	first.RecordShadowCallFor(domain.ClassAgent, session.MethodValidateGates, session.PathTransition)
	first.RecordDivergenceFor(domain.ClassAgent, session.MethodValidateGates, session.ClassGuardNew, session.ConfigSeeded, session.PathTransition)
	first.RecordObservation(session.ShadowObservation{
		Method: session.MethodValidateGates, Path: session.PathTransition, From: domain.BacklogStatusReady, To: domain.BacklogStatusInProgress,
		Class: session.ClassGuardNew, Config: session.ConfigSeeded, CallerClass: domain.ClassAgent, Caller: domain.CallerMCPResumeWork,
	})
	first.RecordShadowCallFor(domain.ClassSystem, session.MethodCanTransition, session.PathTransition)
	require.NoError(t, first.Flush())

	second := NewShadowLedgerRecorder(dir, WithShadowClock(clock))
	second.RecordShadowCallFor(domain.ClassAgent, session.MethodValidateGates, session.PathTransition)
	snap := second.Snapshot()
	require.Equal(t, int64(2), snap.Classes[string(domain.ClassAgent)].Calls["ValidateGates|transition"])
	require.Equal(t, int64(1), snap.Classes[string(domain.ClassAgent)].CallerDivergence[string(domain.CallerMCPResumeWork)+"|guard_new"])
	require.Equal(t, int64(1), snap.Classes[string(domain.ClassSystem)].Calls["CanTransition|transition"])
}

func TestShadowLedgerRecorder_UnclassedMethodsRecordIntoTheRecordersOwnClass(t *testing.T) {
	t.Parallel()
	rec := NewShadowLedgerRecorder("", WithShadowClass(domain.ClassAgent))
	rec.RecordShadowCall(session.MethodCanTransition, session.PathTransition)
	rec.RecordShadowCallFor(domain.CallerClass("bogus"), session.MethodCanTransition, session.PathTransition)
	require.Equal(t, int64(2), rec.Snapshot().Classes[string(domain.ClassAgent)].Calls["CanTransition|transition"], "an unknown class falls back to the recorder's own")
}
