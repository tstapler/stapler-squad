package workflowengine

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/backlogstage"
	"github.com/tstapler/stapler-squad/session/ent/stagetransition"
	"github.com/tstapler/stapler-squad/session/internal/entdbtest"
	"github.com/tstapler/stapler-squad/session/internal/gatefixturetest"
	"github.com/tstapler/stapler-squad/session/internal/sessiontest"
	"github.com/tstapler/stapler-squad/session/stagecfg"
)

// newSeededConfiguredWorkflowEngine returns an engine over a fresh in-memory
// database holding the built-in stages.
func newSeededConfiguredWorkflowEngine(t *testing.T) (*ConfiguredWorkflowEngine, *ent.Client) {
	t.Helper()
	client := entdbtest.NewClient(t)
	require.NoError(t, EnsureBuiltInWorkflowStages(context.Background(), client))
	engine, err := NewConfiguredWorkflowEngine(stagecfg.NewEntStageConfigRepository(client), stagecfg.NewEntGateSatisfactionRepository(client), nil)
	require.NoError(t, err)
	return engine, client
}

type fixedMode struct{ m EngineMode }

func (f fixedMode) Mode() EngineMode { return f.m }

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

type selectorHarness struct {
	sel      *SwitchableWorkflowEngine
	cfg      *ConfiguredWorkflowEngine
	client   *ent.Client
	recorder *recordingShadow
	warns    *lineSink
	infos    *lineSink
	clock    *sessiontest.SelectorClock
}

func setEdgeEnabled(t *testing.T, client *ent.Client, cfg *ConfiguredWorkflowEngine, from, to domain.BacklogStatus, enabled bool) {
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

func addGateToSeededEdge(t *testing.T, client *ent.Client, cfg *ConfiguredWorkflowEngine, from, to domain.BacklogStatus, kind domain.GateKind, config map[string]interface{}) {
	t.Helper()
	ctx := context.Background()
	tr, err := client.StageTransition.Query().
		Where(
			stagetransition.HasFromStageWith(backlogstage.Slug(string(from))),
			stagetransition.HasToStageWith(backlogstage.Slug(string(to))),
		).Only(ctx)
	require.NoError(t, err)
	c := client.TransitionGate.Create().SetTransitionID(tr.ID).SetKind(string(kind)).SetStateful(IsStatefulGateKind(kind)).SetEnabled(true)
	if config != nil {
		c = c.SetConfig(config)
	}
	_, err = c.Save(ctx)
	require.NoError(t, err)
	require.NoError(t, cfg.InvalidateCache(ctx))
}

func passingReviewItem() domain.BacklogItemTransitionInput {
	return domain.BacklogItemTransitionInput{Status: domain.BacklogStatusReview, OverallOutcome: domain.ReviewOutcomePass}
}

type classModes map[domain.CallerClass]EngineMode

func (c classModes) Mode() EngineMode { return c[domain.ClassOperator] }

func (c classModes) ModeFor(k domain.CallerClass) EngineMode { return c[k] }

func allClasses(m EngineMode) classModes {
	return classModes{domain.ClassOperator: m, domain.ClassAgent: m, domain.ClassSystem: m, "bogus": m}
}

func acCriteriaAllDone(t *testing.T) domain.AcCriteriaJSON {
	return gatefixturetest.ACCriteriaAllDone(t)
}

func acCriteriaOneUnchecked(t *testing.T) domain.AcCriteriaJSON {
	return gatefixturetest.ACCriteriaOneUnchecked(t)
}

func newCustomTransitionWithGates(t *testing.T, client *ent.Client, engine *ConfiguredWorkflowEngine, kinds []domain.GateKind, stateful []bool) (from, to domain.BacklogStatus) {
	return gatefixturetest.NewCustomTransitionWithGates(t, client, engine.InvalidateCache, kinds, stateful)
}

// newConfiguredWorkflowEngineEmptyCache returns an engine whose cache was never loaded.
func newConfiguredWorkflowEngineEmptyCache() *ConfiguredWorkflowEngine {
	return &ConfiguredWorkflowEngine{cache: &stageConfigCache{}}
}

// cachedGateIDs returns the IDs of the gates cached on the from->to edge.
func (e *ConfiguredWorkflowEngine) cachedGateIDs(from, to domain.BacklogStatus) ([]uuid.UUID, bool) {
	edge, ok := e.cache.Get(from, to)
	if !ok {
		return nil, false
	}
	ids := make([]uuid.UUID, len(edge.Gates))
	for i, g := range edge.Gates {
		ids[i] = g.ID
	}
	return ids, true
}
