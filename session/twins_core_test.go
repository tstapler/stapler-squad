package session

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/backlogstage"
	"github.com/tstapler/stapler-squad/session/ent/stagetransition"
	"github.com/tstapler/stapler-squad/session/internal/sessiontest"
)

func denOpenDenial(t *testing.T, h *gateHarness, itemID string, caller domain.TransitionCaller, from, to BacklogStatus, reason string) {
	t.Helper()
	applied, err := h.storage.MarkStuck(context.Background(), itemID, domain.StuckReasonTransitionDenied, from,
		domain.FormatTransitionDeniedContext(domain.TransitionDenial{Caller: string(caller), From: string(from), To: string(to), Reason: reason}, "test"))
	require.NoError(t, err)
	require.True(t, applied, "the item must be at %s for the denial to anchor", from)
}

func denBlock(t *testing.T, h *gateHarness, from, to BacklogStatus) {
	t.Helper()
	addGateToSeededEdge(t, h.repo.Client, h.cfg, from, to, domain.GateKindHumanApproval, nil)
}

func denOpenRows(t *testing.T, h *gateHarness, itemID string, reason domain.StuckReason) []backlogdto.OpenStuckStateData {
	t.Helper()
	rows, err := h.storage.FindOpenStuckStates(context.Background())
	require.NoError(t, err)
	var out []backlogdto.OpenStuckStateData
	for _, r := range rows {
		if r.ItemID == itemID && r.Reason == reason {
			out = append(out, r)
		}
	}
	return out
}

// sessDisableGates stands in for an operator satisfying (or removing) every
// configured gate: the next decision finds none.
func sessDisableGates(t *testing.T, h *gateHarness) {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, h.repo.Client.TransitionGate.Update().SetEnabled(false).Exec(ctx))
	require.NoError(t, h.cfg.InvalidateCache(ctx))
}

// newStuckReviewTestItem creates a review-status BacklogItem with a review
// ItemSession carrying the given verdict outcome. If endReviewSession is true,
// the review session's EndedAt is set (simulating the review session having
// exited) — the item then qualifies as "stuck" per FindStuckReviewItems as
// long as no other active session exists. If withActiveWorkSession is true, a
// second ItemSession (role=work, EndedAt nil) is attached, simulating rework
// still in progress — this should exclude the item from FindStuckReviewItems.
func newStuckReviewTestItem(t *testing.T, storage *Storage, outcome ReviewOutcome, endReviewSession, withActiveWorkSession bool) *backlogdto.BacklogItemData {
	t.Helper()
	ctx := context.Background()

	item, err := storage.CreateBacklogItem(ctx, backlogdto.BacklogItemData{
		Title:              "Stuck review test item",
		AcceptanceCriteria: `[]`,
		Priority:           1,
		Status:             string(BacklogStatusReview),
	})
	require.NoError(t, err)

	reviewIS, err := storage.CreateItemSessionWithVerdict(ctx, backlogdto.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: "headless-review-" + uuid.New().String(),
		SessionRole: SessionRoleReview,
	}, backlogdto.ReviewVerdictData{
		OverallOutcome: outcome,
		Summary:        "no diff available",
	})
	require.NoError(t, err)

	if endReviewSession {
		require.NoError(t, storage.UpdateItemSessionEnded(ctx, reviewIS.ID, time.Now()))
	}

	if withActiveWorkSession {
		_, err := storage.CreateItemSession(ctx, backlogdto.ItemSessionData{
			ItemID:      item.ID,
			SessionUUID: uuid.New().String(),
			SessionRole: SessionRoleWork,
		})
		require.NoError(t, err)
	}

	return item
}

// backdateNextRemediationAt directly manipulates a row's next_remediation_at
// via the ent client, bypassing the normal write path — test-only helper for
// simulating "time has passed" without an actual sleep.
func backdateNextRemediationAt(t *testing.T, repo *EntRepository, itemID string, reason domain.StuckReason, at time.Time) {
	t.Helper()
	applied, err := repo.RecordRemediationAttempt(context.Background(), itemID, reason, currentAttempts(t, repo, itemID, reason), &at)
	require.NoError(t, err)
	require.True(t, applied)
}

// currentAttempts reads back the current remediation_attempts for (itemID,
// reason) — backdateNextRemediationAt needs it to avoid disturbing the
// attempt count while only backdating the timer.
func currentAttempts(t *testing.T, repo *EntRepository, itemID string, reason domain.StuckReason) int32 {
	t.Helper()
	rows, err := repo.FindOpenStuckStates(context.Background())
	require.NoError(t, err)
	row, ok := FindOpenStuckStateFor(rows, itemID, reason)
	require.True(t, ok)
	return row.RemediationAttempts
}

// newSeededConfiguredWorkflowEngine seeds the built-in stage graph and
// returns a ConfiguredWorkflowEngine loaded from it, plus the underlying
// ent.Client (for tests that need to write additional rows directly, e.g. a
// simulated operator-added custom transition — standing in for the
// not-yet-implemented Epic 2.7 CRUD RPCs).
func newSeededConfiguredWorkflowEngine(t *testing.T) (*ConfiguredWorkflowEngine, *ent.Client) {
	t.Helper()
	repo := NewTestEntRepository(t)
	ctx := context.Background()
	client := repo.Client

	require.NoError(t, EnsureBuiltInWorkflowStages(ctx, client))

	stageRepo := NewEntStageConfigRepository(client)
	gateSatisfactionRepo := NewEntGateSatisfactionRepository(client)
	engine, err := NewConfiguredWorkflowEngine(stageRepo, gateSatisfactionRepo, nil)
	require.NoError(t, err)
	return engine, client
}

// createTestEntRepository is a thin wrapper around NewTestEntRepository
// (testing.go) kept so the ~90 existing `repo, cleanup := createTestEntRepository(t)`
// call sites in this package don't need touching. NewTestEntRepository
// already registers its own t.Cleanup, so the returned cleanup func is a
// no-op retained only for call-site compatibility.
func createTestEntRepository(t *testing.T) (*EntRepository, func()) {
	repo := NewTestEntRepository(t)

	ctx := context.Background()
	sessions, err := repo.List(ctx)
	require.NoError(t, err, "Failed to list sessions")
	require.Empty(t, sessions, "Database should be empty but has %d sessions", len(sessions))

	return repo, func() {}
}

// fakePipelineModeRepository is a test double for PipelineModeRepository.
// Only ListEnabled is exercised by pipeline_engine.go — the remaining methods
// return an error if called, since no test in this file should reach them.
type fakePipelineModeRepository struct {
	listEnabledFn func(ctx context.Context) ([]*ent.PipelineMode, error)
}

func (f *fakePipelineModeRepository) Create(context.Context, PipelineModeCreateInput) (*ent.PipelineMode, error) {
	return nil, errors.New("fakePipelineModeRepository.Create: not implemented")
}

func (f *fakePipelineModeRepository) Update(context.Context, uuid.UUID, PipelineModeUpdateInput) (*ent.PipelineMode, error) {
	return nil, errors.New("fakePipelineModeRepository.Update: not implemented")
}

func (f *fakePipelineModeRepository) Delete(context.Context, uuid.UUID) error {
	return errors.New("fakePipelineModeRepository.Delete: not implemented")
}

func (f *fakePipelineModeRepository) GetByID(context.Context, uuid.UUID) (*ent.PipelineMode, error) {
	return nil, errors.New("fakePipelineModeRepository.GetByID: not implemented")
}

func (f *fakePipelineModeRepository) GetBySlug(context.Context, string) (*ent.PipelineMode, error) {
	return nil, errors.New("fakePipelineModeRepository.GetBySlug: not implemented")
}

func (f *fakePipelineModeRepository) ListAll(context.Context) ([]*ent.PipelineMode, error) {
	return nil, errors.New("fakePipelineModeRepository.ListAll: not implemented")
}

func (f *fakePipelineModeRepository) ListEnabled(ctx context.Context) ([]*ent.PipelineMode, error) {
	return f.listEnabledFn(ctx)
}

type reportedDenial struct {
	itemID       string
	caller       domain.TransitionCaller
	from, to     BacklogStatus
	reason       string
	err          error
	statusAtCall string
}

// recordingReporter records every Report and, to prove "before the write path
// returns", the item's stored status at the instant of the call.
type recordingReporter struct {
	mu      sync.Mutex
	calls   []reportedDenial
	storage *Storage
	panics  bool
}

func (r *recordingReporter) Report(ctx context.Context, item *backlogdto.BacklogItemData, caller domain.TransitionCaller, from, to BacklogStatus, reason string, denyErr error) {
	cur, _ := r.storage.GetBacklogItem(ctx, item.ID)
	r.mu.Lock()
	r.calls = append(r.calls, reportedDenial{item.ID, caller, from, to, reason, denyErr, cur.Status})
	r.mu.Unlock()
	if r.panics {
		panic("reporter boom")
	}
}

func (r *recordingReporter) snapshot() []reportedDenial {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]reportedDenial(nil), r.calls...)
}

type gateHarness struct {
	storage *Storage
	repo    *EntRepository
	cfg     *ConfiguredWorkflowEngine
	sel     *SwitchableWorkflowEngine
	rep     *recordingReporter
}

func newGateHarness(t *testing.T, mode EngineMode) *gateHarness {
	t.Helper()
	repo := NewTestEntRepository(t)
	ctx := context.Background()
	require.NoError(t, EnsureBuiltInWorkflowStages(ctx, repo.Client))
	cfg, err := NewConfiguredWorkflowEngine(NewEntStageConfigRepository(repo.Client), NewEntGateSatisfactionRepository(repo.Client), nil)
	require.NoError(t, err)
	storage, err := NewStorageWithRepository(repo)
	require.NoError(t, err)

	sel := NewSwitchableWorkflowEngine(NewDefaultWorkflowEngine(), cfg, allClasses(mode),
		WithSelectorLoggers(func(string, ...any) {}, func(string, ...any) {}))
	rep := &recordingReporter{storage: storage}
	require.NoError(t, storage.SetTransitionGate(sel, rep))
	return &gateHarness{storage: storage, repo: repo, cfg: cfg, sel: sel, rep: rep}
}

func (h *gateHarness) newItem(t *testing.T, status BacklogStatus) *backlogdto.BacklogItemData {
	t.Helper()
	item, err := h.storage.CreateBacklogItem(context.Background(), backlogdto.BacklogItemData{Title: "gate item", Status: string(status)})
	require.NoError(t, err)
	return item
}

func (h *gateHarness) status(t *testing.T, id string) string {
	t.Helper()
	item, err := h.storage.GetBacklogItem(context.Background(), id)
	require.NoError(t, err)
	return item.Status
}

// moveRaw changes the stored status behind the gate's back (a concurrent mover).
func (h *gateHarness) moveRaw(t *testing.T, id string, to BacklogStatus) {
	t.Helper()
	_, err := h.repo.RawTransitionBacklogItemStatus(context.Background(), id, to, nil, TriggeredBySystem)
	require.NoError(t, err)
}

// newStuckInProgressItem is an in_progress item whose only work session ended.
func (h *gateHarness) newStuckInProgressItem(t *testing.T) *backlogdto.BacklogItemData {
	t.Helper()
	ctx := context.Background()
	item, err := h.storage.CreateBacklogItem(ctx, backlogdto.BacklogItemData{Title: "stuck item", Status: string(BacklogStatusInProgress), Notes: "existing notes"})
	require.NoError(t, err)
	is, err := h.storage.CreateItemSession(ctx, backlogdto.ItemSessionData{ItemID: item.ID, SessionUUID: uuid.NewString(), SessionRole: SessionRoleWork})
	require.NoError(t, err)
	require.NoError(t, h.storage.UpdateItemSessionEnded(ctx, is.ID, time.Now().Add(-time.Minute)))
	return item
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

func addGateToSeededEdge(t *testing.T, client *ent.Client, cfg *ConfiguredWorkflowEngine, from, to BacklogStatus, kind domain.GateKind, config map[string]interface{}) {
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

func passingReviewItem() BacklogItemTransitionInput {
	return BacklogItemTransitionInput{Status: BacklogStatusReview, OverallOutcome: domain.ReviewOutcomePass}
}

type classModes map[domain.CallerClass]EngineMode

func (c classModes) Mode() EngineMode { return c[domain.ClassOperator] }

func (c classModes) ModeFor(k domain.CallerClass) EngineMode { return c[k] }

func allClasses(m EngineMode) classModes {
	return classModes{domain.ClassOperator: m, domain.ClassAgent: m, domain.ClassSystem: m, "bogus": m}
}
