package services

// backlog_service_transitions_test.go — tests for the StageTransition +
// TransitionGate CRUD RPCs (Story 2.7.2 of backlog-custom-workflow-stages).
// Two test names are taken verbatim from validation.md's Epic 2.7 rows.
//
// The concurrency test at the bottom of this file
// (TestCreateStageTransition_should_SerializeConcurrentCallers_When_BothWouldJointlyProduceInvalidGraph)
// implements the barrier-synchronized two-concurrent-caller fixture deferred
// from Epic 2.6's Task 2.6.1g1/g2 to this epic's Task 2.7.2h3 — see that
// test's own doc comment for why it could only exist once Task 2.7.2h's
// WithTx wrapper did.

import (
	"sync"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
)

// newTransitionCRUDTestService mirrors newStageCRUDTestService
// (backlog_service_stages_test.go) — same wiring, reused here under its own
// name for this file's readability.
func newTransitionCRUDTestService(t *testing.T) (*BacklogService, session.StageCRUDRepository, *session.ConfiguredWorkflowEngine, *session.Storage) {
	t.Helper()
	return newStageCRUDTestService(t)
}

// mustCreateFreshCustomTransition creates two brand-new custom stages
// (fromSlug as an entry stage, toSlug as a terminal stage — graph-valid with
// just the one edge between them) and the transition connecting them, for
// gate-CRUD tests that don't want to collide with a built-in edge already
// seeded by EnsureBuiltInWorkflowStages (e.g. review -> done).
func mustCreateFreshCustomTransition(t *testing.T, svc *BacklogService, fromSlug, toSlug string) *connect.Response[sessionv1.CreateStageTransitionResponse] {
	t.Helper()
	ctx := t.Context()

	_, err := svc.CreateStage(ctx, connect.NewRequest(&sessionv1.CreateStageRequest{
		Slug: fromSlug, Name: fromSlug, IsEntry: true, Enabled: true,
	}))
	require.NoError(t, err)
	_, err = svc.CreateStage(ctx, connect.NewRequest(&sessionv1.CreateStageRequest{
		Slug: toSlug, Name: toSlug, IsTerminal: true, Enabled: true,
	}))
	require.NoError(t, err)

	edgeResp, err := svc.CreateStageTransition(ctx, connect.NewRequest(&sessionv1.CreateStageTransitionRequest{
		FromStageSlug: fromSlug,
		ToStageSlug:   toSlug,
		Enabled:       true,
	}))
	require.NoError(t, err)
	return edgeResp
}

// ─── TestCreateStageTransition ──────────────────────────────────────────────

// TestCreateStageTransition_should_PersistAndReturnTransition_When_GraphRemainsValid
// (validation.md, Story 2.7.2) is the happy path: a new edge between two new,
// well-connected custom stages persists and is returned with no warnings.
func TestCreateStageTransition_should_PersistAndReturnTransition_When_GraphRemainsValid(t *testing.T) {
	t.Parallel()
	svc, _, _, _ := newTransitionCRUDTestService(t)
	ctx := t.Context()

	_, err := svc.CreateStage(ctx, connect.NewRequest(&sessionv1.CreateStageRequest{
		Slug: "custom-a", Name: "Custom A", IsEntry: true, Enabled: true,
	}))
	require.NoError(t, err)
	_, err = svc.CreateStage(ctx, connect.NewRequest(&sessionv1.CreateStageRequest{
		Slug: "custom-b", Name: "Custom B", IsTerminal: true, Enabled: true,
	}))
	require.NoError(t, err)

	resp, err := svc.CreateStageTransition(ctx, connect.NewRequest(&sessionv1.CreateStageTransitionRequest{
		FromStageSlug: "custom-a",
		ToStageSlug:   "custom-b",
		Enabled:       true,
	}))
	require.NoError(t, err)
	assert.Equal(t, "custom-a", resp.Msg.Item.FromStageSlug)
	assert.Equal(t, "custom-b", resp.Msg.Item.ToStageSlug)
	assert.True(t, resp.Msg.Item.Enabled)
	// Not asserting Warnings is empty: the built-in seeded graph already has
	// several gate-free cycles (e.g. review -> pr_pending -> review) that
	// ValidateGraph reports on every call regardless of this new edge — only
	// assert this specific edge didn't introduce a NEW one.
	for _, w := range resp.Msg.Warnings {
		assert.NotContains(t, w, "custom-a")
		assert.NotContains(t, w, "custom-b")
	}

	getResp, err := svc.GetStageTransition(ctx, connect.NewRequest(&sessionv1.GetStageTransitionRequest{Id: resp.Msg.Item.Id}))
	require.NoError(t, err)
	assert.Equal(t, resp.Msg.Item.Id, getResp.Msg.Item.Id)
}

// TestCreateStageTransition_should_InvokeGraphValidatorBeforeCommitting_When_RequestWouldCreateUnreachableStage
// (validation.md, Story 2.7.2) proves Epic 2.6's ValidateGraph runs before
// the transition commits: "orphan" gets an outgoing edge (so it is not a
// dead end) but nothing points to it, so it remains unreachable from any
// entry stage — the create must fail and leave nothing persisted, verified
// via a follow-up ListStageTransitions.
func TestCreateStageTransition_should_InvokeGraphValidatorBeforeCommitting_When_RequestWouldCreateUnreachableStage(t *testing.T) {
	t.Parallel()
	svc, _, _, _ := newTransitionCRUDTestService(t)
	ctx := t.Context()

	_, err := svc.CreateStage(ctx, connect.NewRequest(&sessionv1.CreateStageRequest{
		Slug: "orphan", Name: "Orphan", Enabled: true,
	}))
	require.NoError(t, err)

	_, err = svc.CreateStageTransition(ctx, connect.NewRequest(&sessionv1.CreateStageTransitionRequest{
		FromStageSlug: "orphan",
		ToStageSlug:   string(session.BacklogStatusDone),
		Enabled:       true,
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	assert.Contains(t, err.Error(), "orphan")

	listResp, err := svc.ListStageTransitions(ctx, connect.NewRequest(&sessionv1.ListStageTransitionsRequest{
		FromStageSlug: strPtr("orphan"),
	}))
	require.NoError(t, err)
	assert.Empty(t, listResp.Msg.Items, "no partial write: the rejected transition must not be persisted")
}

// ─── TestUpdateStageTransition ──────────────────────────────────────────────

// TestUpdateStageTransition_should_ReturnFailedPreconditionAndPersistNothing_When_DisablingLastEnabledEdgeForStageWithLiveItems
// (Task 2.7.2f) proves disabling the last enabled outgoing edge for a stage
// with a live item is rejected, and a follow-up read confirms the edge is
// still enabled.
func TestUpdateStageTransition_should_ReturnFailedPreconditionAndPersistNothing_When_DisablingLastEnabledEdgeForStageWithLiveItems(t *testing.T) {
	t.Parallel()
	svc, _, _, storage := newTransitionCRUDTestService(t)
	ctx := t.Context()

	// design-review-with-one-enabled-edge scenario from Story 2.6.1's AC:
	// a custom stage whose only enabled outgoing edge is about to be
	// disabled. IsEntry is set so the stage is graph-valid on its own with
	// just this one outgoing edge (no incoming edge needed for this test).
	_, err := svc.CreateStage(ctx, connect.NewRequest(&sessionv1.CreateStageRequest{
		Slug: "design-review", Name: "Design Review", IsEntry: true, Enabled: true,
	}))
	require.NoError(t, err)
	edgeResp, err := svc.CreateStageTransition(ctx, connect.NewRequest(&sessionv1.CreateStageTransitionRequest{
		FromStageSlug: "design-review",
		ToStageSlug:   string(session.BacklogStatusReady),
		Enabled:       true,
	}))
	require.NoError(t, err)

	_, err = storage.CreateBacklogItem(ctx, session.BacklogItemData{Title: "live item", Status: "design-review"})
	require.NoError(t, err)

	_, err = svc.UpdateStageTransition(ctx, connect.NewRequest(&sessionv1.UpdateStageTransitionRequest{
		Id:      edgeResp.Msg.Item.Id,
		Enabled: boolPtr(false),
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
	assert.Contains(t, err.Error(), "design-review")
	assert.Contains(t, err.Error(), "1")

	getResp, err := svc.GetStageTransition(ctx, connect.NewRequest(&sessionv1.GetStageTransitionRequest{Id: edgeResp.Msg.Item.Id}))
	require.NoError(t, err)
	assert.True(t, getResp.Msg.Item.Enabled, "no partial write: the edge must still be enabled")
}

// TestUpdateStageTransition_should_Succeed_When_DisablingEdgeForStageWithZeroLiveItems
// proves the disable guard never fires when nothing is currently on the
// stage, even for the last enabled edge — Task 2.6.1f's "always allowed"
// acceptance criterion.
func TestUpdateStageTransition_should_Succeed_When_DisablingEdgeForStageWithZeroLiveItems(t *testing.T) {
	t.Parallel()
	svc, _, _, _ := newTransitionCRUDTestService(t)
	ctx := t.Context()

	_, err := svc.CreateStage(ctx, connect.NewRequest(&sessionv1.CreateStageRequest{
		Slug: "design-review-2", Name: "Design Review 2", IsEntry: true, Enabled: true,
	}))
	require.NoError(t, err)
	edgeResp, err := svc.CreateStageTransition(ctx, connect.NewRequest(&sessionv1.CreateStageTransitionRequest{
		FromStageSlug: "design-review-2",
		ToStageSlug:   string(session.BacklogStatusReady),
		Enabled:       true,
	}))
	require.NoError(t, err)

	updateResp, err := svc.UpdateStageTransition(ctx, connect.NewRequest(&sessionv1.UpdateStageTransitionRequest{
		Id:      edgeResp.Msg.Item.Id,
		Enabled: boolPtr(false),
	}))
	require.NoError(t, err)
	assert.False(t, updateResp.Msg.Item.Enabled)
}

// TestDeleteStageTransition_should_ReturnFailedPreconditionAndPersistNothing_When_DeletingLastEnabledEdgeForStageWithLiveItems
// is DeleteStageTransition's sibling to the Update test above: deleting the
// last enabled outgoing edge is exactly as capable of stranding live items as
// disabling it, and must get the identical live-item safety check.
func TestDeleteStageTransition_should_ReturnFailedPreconditionAndPersistNothing_When_DeletingLastEnabledEdgeForStageWithLiveItems(t *testing.T) {
	t.Parallel()
	svc, _, _, storage := newTransitionCRUDTestService(t)
	ctx := t.Context()

	_, err := svc.CreateStage(ctx, connect.NewRequest(&sessionv1.CreateStageRequest{
		Slug: "design-review-delete", Name: "Design Review Delete", IsEntry: true, Enabled: true,
	}))
	require.NoError(t, err)
	edgeResp, err := svc.CreateStageTransition(ctx, connect.NewRequest(&sessionv1.CreateStageTransitionRequest{
		FromStageSlug: "design-review-delete",
		ToStageSlug:   string(session.BacklogStatusReady),
		Enabled:       true,
	}))
	require.NoError(t, err)

	_, err = storage.CreateBacklogItem(ctx, session.BacklogItemData{Title: "live item", Status: "design-review-delete"})
	require.NoError(t, err)

	_, err = svc.DeleteStageTransition(ctx, connect.NewRequest(&sessionv1.DeleteStageTransitionRequest{
		Id: edgeResp.Msg.Item.Id,
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
	assert.Contains(t, err.Error(), "design-review-delete")
	assert.Contains(t, err.Error(), "1")

	getResp, err := svc.GetStageTransition(ctx, connect.NewRequest(&sessionv1.GetStageTransitionRequest{Id: edgeResp.Msg.Item.Id}))
	require.NoError(t, err, "no partial write: the edge must still exist")
	assert.True(t, getResp.Msg.Item.Enabled)
}

// TestDeleteStageTransition_should_Succeed_When_DeletingEdgeForStageWithZeroLiveItems
// proves the delete guard never fires when nothing is currently on the
// stage, even for the last enabled edge.
func TestDeleteStageTransition_should_Succeed_When_DeletingEdgeForStageWithZeroLiveItems(t *testing.T) {
	t.Parallel()
	svc, _, _, _ := newTransitionCRUDTestService(t)
	ctx := t.Context()

	_, err := svc.CreateStage(ctx, connect.NewRequest(&sessionv1.CreateStageRequest{
		Slug: "design-review-delete-2", Name: "Design Review Delete 2", IsEntry: true, Enabled: true,
	}))
	require.NoError(t, err)
	edgeResp, err := svc.CreateStageTransition(ctx, connect.NewRequest(&sessionv1.CreateStageTransitionRequest{
		FromStageSlug: "design-review-delete-2",
		ToStageSlug:   string(session.BacklogStatusReady),
		Enabled:       true,
	}))
	require.NoError(t, err)

	_, err = svc.DeleteStageTransition(ctx, connect.NewRequest(&sessionv1.DeleteStageTransitionRequest{
		Id: edgeResp.Msg.Item.Id,
	}))
	require.NoError(t, err)

	_, err = svc.GetStageTransition(ctx, connect.NewRequest(&sessionv1.GetStageTransitionRequest{Id: edgeResp.Msg.Item.Id}))
	require.Error(t, err, "the transition must actually be gone")
}

// ─── TransitionGate config validation (Task 2.7.2g3) ────────────────────────

// TestCreateTransitionGate_should_ReturnInvalidArgumentAndPersistNothing_When_ConfigNamesKeyOutsideAllowlist
// proves the RPC handler itself calls session.ParseGateConfig before
// persisting (not only at eventual invocation time) — an unrecognized key
// is rejected with CodeInvalidArgument and nothing is written.
func TestCreateTransitionGate_should_ReturnInvalidArgumentAndPersistNothing_When_ConfigNamesKeyOutsideAllowlist(t *testing.T) {
	t.Parallel()
	svc, _, _, _ := newTransitionCRUDTestService(t)
	ctx := t.Context()

	edgeResp := mustCreateFreshCustomTransition(t, svc, "gate-a", "gate-b")

	_, err := svc.CreateTransitionGate(ctx, connect.NewRequest(&sessionv1.CreateTransitionGateRequest{
		TransitionId: edgeResp.Msg.Item.Id,
		Kind:         "custom",
		Config:       map[string]string{"skill": "review-feasibility", "extra_flag": "danger"},
		Enabled:      true,
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	assert.Contains(t, err.Error(), "extra_flag")

	listResp, err := svc.ListTransitionGates(ctx, connect.NewRequest(&sessionv1.ListTransitionGatesRequest{
		TransitionId: strPtr(edgeResp.Msg.Item.Id),
	}))
	require.NoError(t, err)
	assert.Empty(t, listResp.Msg.Items)
}

// TestCreateTransitionGate_should_PersistAndReturnGate_When_ConfigIsValid is
// the gate-CRUD happy path, round-tripping a valid "custom" gate config.
func TestCreateTransitionGate_should_PersistAndReturnGate_When_ConfigIsValid(t *testing.T) {
	t.Parallel()
	svc, _, _, _ := newTransitionCRUDTestService(t)
	ctx := t.Context()

	edgeResp := mustCreateFreshCustomTransition(t, svc, "gate-c", "gate-d")

	gateResp, err := svc.CreateTransitionGate(ctx, connect.NewRequest(&sessionv1.CreateTransitionGateRequest{
		TransitionId: edgeResp.Msg.Item.Id,
		Kind:         "custom",
		Config:       map[string]string{"skill": "review-feasibility"},
		Enabled:      true,
	}))
	require.NoError(t, err)
	assert.Equal(t, "review-feasibility", gateResp.Msg.Item.Config["skill"])

	getResp, err := svc.GetStageTransition(ctx, connect.NewRequest(&sessionv1.GetStageTransitionRequest{Id: edgeResp.Msg.Item.Id}))
	require.NoError(t, err)
	require.Len(t, getResp.Msg.Item.Gates, 1)
	assert.Equal(t, gateResp.Msg.Item.Id, getResp.Msg.Item.Gates[0].Id)
}

// ─── Task 2.7.2h3: TOCTOU concurrency test (deferred from 2.6.1g1/g2) ───────

// TestCreateStageTransition_should_SerializeConcurrentCallers_When_BothWouldJointlyProduceInvalidGraph
// implements the barrier-synchronized two-concurrent-caller fixture Epic
// 2.6's Task 2.6.1g1/g2 deferred to this task, per plan.md: that test could
// only exist once Task 2.7.2h's WithTx wrapper existed to serialize against.
//
// Concretely, "would each individually pass validation but jointly produce
// an invalid graph" means: two concurrent CreateStageTransition calls racing
// to create the IDENTICAL (from,to) edge. Each individually would pass
// ValidateGraph and persist cleanly if run alone. Run unserialized against a
// pre-transaction snapshot, both writers could observe the same "edge does
// not exist yet" read and jointly attempt to commit two rows for what must
// be a single edge (StageTransition's (from_stage_id,to_stage_id) unique
// index — session/ent/schema/stage_transition.go) — an invalid graph state
// (a duplicated edge) that could otherwise surface as a silently-lost write
// or a corrupted read. Task 2.7.2h's WithTx wrapper (backed by this
// project's single-ent-connection SQLite pool, session/ent_repository.go's
// db.SetMaxOpenConns(1), which serializes the two transactions at the
// connection-pool level) is what turns this into "exactly one caller
// commits; the other's transaction observes the first caller's
// already-committed row and fails cleanly with CodeAlreadyExists" instead.
func TestCreateStageTransition_should_SerializeConcurrentCallers_When_BothWouldJointlyProduceInvalidGraph(t *testing.T) {
	svc, _, _, _ := newTransitionCRUDTestService(t)
	ctx := t.Context()

	_, err := svc.CreateStage(ctx, connect.NewRequest(&sessionv1.CreateStageRequest{
		Slug: "race-a", Name: "Race A", IsEntry: true, Enabled: true,
	}))
	require.NoError(t, err)
	_, err = svc.CreateStage(ctx, connect.NewRequest(&sessionv1.CreateStageRequest{
		Slug: "race-b", Name: "Race B", IsTerminal: true, Enabled: true,
	}))
	require.NoError(t, err)

	const callers = 2
	var wg sync.WaitGroup
	start := make(chan struct{})
	results := make([]error, callers)
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			<-start
			_, callErr := svc.CreateStageTransition(ctx, connect.NewRequest(&sessionv1.CreateStageTransitionRequest{
				FromStageSlug: "race-a",
				ToStageSlug:   "race-b",
				Enabled:       true,
			}))
			results[idx] = callErr
		}(i)
	}
	close(start)
	wg.Wait()

	successCount, conflictCount := 0, 0
	for _, err := range results {
		switch {
		case err == nil:
			successCount++
		case connect.CodeOf(err) == connect.CodeAlreadyExists:
			conflictCount++
		default:
			t.Fatalf("unexpected error from concurrent CreateStageTransition: %v", err)
		}
	}
	assert.Equal(t, 1, successCount, "exactly one concurrent caller should have committed the edge")
	assert.Equal(t, 1, conflictCount, "the other caller must observe a clean conflict, not silent data loss or duplication")

	listResp, err := svc.ListStageTransitions(ctx, connect.NewRequest(&sessionv1.ListStageTransitionsRequest{
		FromStageSlug: strPtr("race-a"),
	}))
	require.NoError(t, err)
	require.Len(t, listResp.Msg.Items, 1, "no partial/duplicate write: exactly one persisted transition row after both calls settle")
}
