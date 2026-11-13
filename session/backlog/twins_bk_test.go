package backlog

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/backlogstage"
	"github.com/tstapler/stapler-squad/session/ent/stagetransition"
)

func denOpenDenial(t *testing.T, h *gateHarness, itemID string, caller session.TransitionCaller, from, to session.BacklogStatus, reason string) {
	t.Helper()
	applied, err := h.storage.MarkStuck(context.Background(), itemID, domain.StuckReasonTransitionDenied, from,
		domain.FormatTransitionDeniedContext(domain.TransitionDenial{Caller: string(caller), From: string(from), To: string(to), Reason: reason}, "test"))
	require.NoError(t, err)
	require.True(t, applied, "the item must be at %s for the denial to anchor", from)
}

func denBlock(t *testing.T, h *gateHarness, from, to session.BacklogStatus) {
	t.Helper()
	addGateToSeededEdge(t, h.repo.Client, h.cfg, from, to, session.GateKindHumanApproval, nil)
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
func newStuckReviewTestItem(t *testing.T, storage *session.Storage, outcome session.ReviewOutcome, endReviewSession, withActiveWorkSession bool) *backlogdto.BacklogItemData {
	t.Helper()
	ctx := context.Background()

	item, err := storage.CreateBacklogItem(ctx, backlogdto.BacklogItemData{
		Title:              "Stuck review test item",
		AcceptanceCriteria: `[]`,
		Priority:           1,
		Status:             string(session.BacklogStatusReview),
	})
	require.NoError(t, err)

	reviewIS, err := storage.CreateItemSessionWithVerdict(ctx, backlogdto.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: "headless-review-" + uuid.New().String(),
		SessionRole: session.SessionRoleReview,
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
			SessionRole: session.SessionRoleWork,
		})
		require.NoError(t, err)
	}

	return item
}

// backdateNextRemediationAt directly manipulates a row's next_remediation_at
// via the ent client, bypassing the normal write path — test-only helper for
// simulating "time has passed" without an actual sleep.
func backdateNextRemediationAt(t *testing.T, repo *session.EntRepository, itemID string, reason domain.StuckReason, at time.Time) {
	t.Helper()
	applied, err := repo.RecordRemediationAttempt(context.Background(), itemID, reason, currentAttempts(t, repo, itemID, reason), &at)
	require.NoError(t, err)
	require.True(t, applied)
}

// currentAttempts reads back the current remediation_attempts for (itemID,
// reason) — backdateNextRemediationAt needs it to avoid disturbing the
// attempt count while only backdating the timer.
func currentAttempts(t *testing.T, repo *session.EntRepository, itemID string, reason domain.StuckReason) int32 {
	t.Helper()
	rows, err := repo.FindOpenStuckStates(context.Background())
	require.NoError(t, err)
	row, ok := session.FindOpenStuckStateFor(rows, itemID, reason)
	require.True(t, ok)
	return row.RemediationAttempts
}

// createTestEntRepository is a thin wrapper around NewTestEntRepository
// (testing.go) kept so the ~90 existing `repo, cleanup := createTestEntRepository(t)`
// call sites in this package don't need touching. NewTestEntRepository
// already registers its own t.Cleanup, so the returned cleanup func is a
// no-op retained only for call-site compatibility.
func createTestEntRepository(t *testing.T) (*session.EntRepository, func()) {
	repo := session.NewTestEntRepository(t)

	ctx := context.Background()
	sessions, err := repo.List(ctx)
	require.NoError(t, err, "Failed to list sessions")
	require.Empty(t, sessions, "Database should be empty but has %d sessions", len(sessions))

	return repo, func() {}
}

type reportedDenial struct {
	itemID       string
	caller       session.TransitionCaller
	from, to     session.BacklogStatus
	reason       string
	err          error
	statusAtCall string
}

// recordingReporter records every Report and, to prove "before the write path
// returns", the item's stored status at the instant of the call.
type recordingReporter struct {
	mu      sync.Mutex
	calls   []reportedDenial
	storage *session.Storage
	panics  bool
}

func (r *recordingReporter) Report(ctx context.Context, item *backlogdto.BacklogItemData, caller session.TransitionCaller, from, to session.BacklogStatus, reason string, denyErr error) {
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
	storage *session.Storage
	repo    *session.EntRepository
	cfg     *session.ConfiguredWorkflowEngine
	sel     *session.SwitchableWorkflowEngine
	rep     *recordingReporter
}

func newGateHarness(t *testing.T, mode session.EngineMode) *gateHarness {
	t.Helper()
	repo := session.NewTestEntRepository(t)
	ctx := context.Background()
	require.NoError(t, session.EnsureBuiltInWorkflowStages(ctx, repo.Client))
	cfg, err := session.NewConfiguredWorkflowEngine(session.NewEntStageConfigRepository(repo.Client), session.NewEntGateSatisfactionRepository(repo.Client), nil)
	require.NoError(t, err)
	storage, err := session.NewStorageWithRepository(repo)
	require.NoError(t, err)

	sel := session.NewSwitchableWorkflowEngine(session.NewDefaultWorkflowEngine(), cfg, allClasses(mode),
		session.WithSelectorLoggers(func(string, ...any) {}, func(string, ...any) {}))
	rep := &recordingReporter{storage: storage}
	require.NoError(t, storage.SetTransitionGate(sel, rep))
	return &gateHarness{storage: storage, repo: repo, cfg: cfg, sel: sel, rep: rep}
}

func (h *gateHarness) newItem(t *testing.T, status session.BacklogStatus) *backlogdto.BacklogItemData {
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
func (h *gateHarness) moveRaw(t *testing.T, id string, to session.BacklogStatus) {
	t.Helper()
	_, err := h.repo.RawTransitionBacklogItemStatus(context.Background(), id, to, nil, session.TriggeredBySystem)
	require.NoError(t, err)
}

func addGateToSeededEdge(t *testing.T, client *ent.Client, cfg *session.ConfiguredWorkflowEngine, from, to session.BacklogStatus, kind session.GateKind, config map[string]interface{}) {
	t.Helper()
	ctx := context.Background()
	tr, err := client.StageTransition.Query().
		Where(
			stagetransition.HasFromStageWith(backlogstage.Slug(string(from))),
			stagetransition.HasToStageWith(backlogstage.Slug(string(to))),
		).Only(ctx)
	require.NoError(t, err)
	c := client.TransitionGate.Create().SetTransitionID(tr.ID).SetKind(string(kind)).SetStateful(session.IsStatefulGateKind(kind)).SetEnabled(true)
	if config != nil {
		c = c.SetConfig(config)
	}
	_, err = c.Save(ctx)
	require.NoError(t, err)
	require.NoError(t, cfg.InvalidateCache(ctx))
}

type classModes map[session.CallerClass]session.EngineMode

func (c classModes) Mode() session.EngineMode { return c[session.ClassOperator] }

func (c classModes) ModeFor(k session.CallerClass) session.EngineMode { return c[k] }

func allClasses(m session.EngineMode) classModes {
	return classModes{session.ClassOperator: m, session.ClassAgent: m, session.ClassSystem: m, "bogus": m}
}
