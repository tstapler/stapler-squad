package mcp

import (
	"context"
	"testing"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/session"
)

const missingItemUUID = "00000000-0000-0000-0000-000000000000"

func newDepHandlers(t *testing.T) (*backlogHandlers, *session.Storage) {
	t.Helper()
	storage := newTestBacklogStorage(t)
	svc := services.NewBacklogService(storage, nil, nil, nil, nil, nil)
	return &backlogHandlers{storage: storage, backlogSvc: svc}, storage
}

func toolText(t *testing.T, res *mcpgo.CallToolResult) string {
	t.Helper()
	require.NotNil(t, res)
	require.NotEmpty(t, res.Content)
	tc, ok := res.Content[0].(mcpgo.TextContent)
	require.True(t, ok)
	return tc.Text
}

func newDepItems(t *testing.T, storage *session.Storage, titles ...string) []session.BacklogItemData {
	t.Helper()
	out := make([]session.BacklogItemData, 0, len(titles))
	for _, title := range titles {
		ac, err := acCriteriaFromStrings([]string{"Given X, when Y, then Z"})
		require.NoError(t, err)
		it, err := storage.CreateBacklogItem(context.Background(), session.BacklogItemData{
			Title:              title,
			Status:             string(session.BacklogStatusIdea),
			AcceptanceCriteria: ac, // required before idea -> ready
		})
		require.NoError(t, err)
		out = append(out, *it)
	}
	return out
}

func edgeArgs(blocker, blocked string) map[string]any {
	return map[string]any{"blocker_item_id": blocker, "blocked_item_id": blocked}
}

func TestMCPAddBacklogItemDependency_should_PersistEdge_When_Valid(t *testing.T) {
	h, storage := newDepHandlers(t)
	items := newDepItems(t, storage, "A", "B")

	res, err := h.addBacklogItemDependency(context.Background(), makeToolReq(edgeArgs(items[0].ID, items[1].ID)))
	require.NoError(t, err)
	assert.Contains(t, toolText(t, res), "Added dependency")

	blockedBy, _, err := storage.ListBacklogItemDependencies(context.Background(), items[1].ID)
	require.NoError(t, err)
	require.Len(t, blockedBy, 1)
	assert.Equal(t, items[0].ID, blockedBy[0].ItemID)
}

func TestMCPAddBacklogItemDependency_should_Reject_When_SelfEdgeOrCycle(t *testing.T) {
	h, storage := newDepHandlers(t)
	items := newDepItems(t, storage, "A", "B")
	ctx := context.Background()

	res, err := h.addBacklogItemDependency(ctx, makeToolReq(edgeArgs(items[0].ID, items[0].ID)))
	require.NoError(t, err)
	assert.Contains(t, toolText(t, res), ErrInvalidArgument)

	_, err = h.addBacklogItemDependency(ctx, makeToolReq(edgeArgs(items[0].ID, items[1].ID)))
	require.NoError(t, err)
	res, err = h.addBacklogItemDependency(ctx, makeToolReq(edgeArgs(items[1].ID, items[0].ID)))
	require.NoError(t, err)
	assert.Contains(t, toolText(t, res), ErrInvalidArgument, "reverse edge closes a cycle")
}

func TestMCPAddBacklogItemDependency_should_ReportInvalidArgument_When_IDMalformedOrMissing(t *testing.T) {
	h, storage := newDepHandlers(t)
	items := newDepItems(t, storage, "A")
	ctx := context.Background()

	res, err := h.addBacklogItemDependency(ctx, makeToolReq(edgeArgs("not-a-uuid", items[0].ID)))
	require.NoError(t, err)
	assert.Contains(t, toolText(t, res), ErrInvalidArgument)

	res, err = h.addBacklogItemDependency(ctx, makeToolReq(edgeArgs(missingItemUUID, items[0].ID)))
	require.NoError(t, err)
	assert.Contains(t, toolText(t, res), ErrItemNotFound)
}

func TestMCPRemoveBacklogItemDependency_should_RemoveEdgeAndBeIdempotent(t *testing.T) {
	h, storage := newDepHandlers(t)
	items := newDepItems(t, storage, "A", "B")
	ctx := context.Background()
	require.NoError(t, storage.AddBacklogItemDependency(ctx, session.BacklogItemDependencyEdge{BlockerID: items[0].ID, BlockedID: items[1].ID}))

	for i := 0; i < 2; i++ { // second pass: edge already gone
		res, err := h.removeBacklogItemDependency(ctx, makeToolReq(edgeArgs(items[0].ID, items[1].ID)))
		require.NoError(t, err)
		assert.Contains(t, toolText(t, res), "Removed")
	}
	blockedBy, _, err := storage.ListBacklogItemDependencies(ctx, items[1].ID)
	require.NoError(t, err)
	assert.Empty(t, blockedBy)
}

func TestMCPTriggerTriage_should_ReportNotFound_When_ItemMissing(t *testing.T) {
	h, _ := newDepHandlers(t)

	res, err := h.triggerTriage(context.Background(), makeToolReq(map[string]any{"item_id": missingItemUUID}))
	require.NoError(t, err)
	assert.Contains(t, toolText(t, res), ErrItemNotFound)
}

func TestMCPTriggerTriage_should_Reject_When_ItemHasNoRepoPath(t *testing.T) {
	h, storage := newDepHandlers(t)
	items := newDepItems(t, storage, "no repo")

	res, err := h.triggerTriage(context.Background(), makeToolReq(map[string]any{"item_id": items[0].ID}))
	require.NoError(t, err)
	assert.Contains(t, toolText(t, res), `"success":false`, "triage needs an absolute repo_path")
}

func TestMCPTransitionBacklogItemStatus_should_RequireExpectedStatus(t *testing.T) {
	h, storage := newDepHandlers(t)
	items := newDepItems(t, storage, "A")

	res, err := h.transitionBacklogItemStatus(context.Background(), makeToolReq(map[string]any{
		"item_id": items[0].ID, "target_status": "ready",
	}))
	require.NoError(t, err)
	text := toolText(t, res)
	assert.Contains(t, text, ErrInvalidArgument)
	assert.Contains(t, text, "expected_status")
}

func TestMCPTransitionBacklogItemStatus_should_Reject_When_ExpectedStatusStale(t *testing.T) {
	h, storage := newDepHandlers(t)
	items := newDepItems(t, storage, "A") // status idea

	res, err := h.transitionBacklogItemStatus(context.Background(), makeToolReq(map[string]any{
		"item_id": items[0].ID, "target_status": "ready", "expected_status": "queued",
	}))
	require.NoError(t, err)
	assert.Contains(t, toolText(t, res), `"success":false`)

	got, err := storage.GetBacklogItem(context.Background(), items[0].ID)
	require.NoError(t, err)
	assert.Equal(t, string(session.BacklogStatusIdea), got.Status, "stale guard must not move the item")
}

func TestMCPTransitionBacklogItemStatus_should_Succeed_When_GuardMatches(t *testing.T) {
	h, storage := newDepHandlers(t)
	items := newDepItems(t, storage, "A")

	res, err := h.transitionBacklogItemStatus(context.Background(), makeToolReq(map[string]any{
		"item_id": items[0].ID, "target_status": "ready", "expected_status": "idea",
	}))
	require.NoError(t, err)
	assert.Contains(t, toolText(t, res), "Transitioned")

	got, err := storage.GetBacklogItem(context.Background(), items[0].ID)
	require.NoError(t, err)
	assert.Equal(t, string(session.BacklogStatusReady), got.Status)
}

func TestMCPTransitionBacklogItemStatus_should_RefuseStart_When_BlockerUnresolved(t *testing.T) {
	h, storage := newDepHandlers(t)
	ctx := context.Background()
	items := newDepItems(t, storage, "blocker", "dependent")
	require.NoError(t, storage.AddBacklogItemDependency(ctx, session.BacklogItemDependencyEdge{BlockerID: items[0].ID, BlockedID: items[1].ID}))
	for _, to := range []string{"ready", "queued"} {
		res, err := h.transitionBacklogItemStatus(ctx, makeToolReq(map[string]any{
			"item_id": items[1].ID, "target_status": to, "expected_status": statusBefore(to),
		}))
		require.NoError(t, err)
		require.Contains(t, toolText(t, res), "Transitioned")
	}

	res, err := h.transitionBacklogItemStatus(ctx, makeToolReq(map[string]any{
		"item_id": items[1].ID, "target_status": "in_progress", "expected_status": "queued",
	}))
	require.NoError(t, err)
	text := toolText(t, res)
	assert.Contains(t, text, ErrFailedPrecondition)

	got, err := storage.GetBacklogItem(ctx, items[1].ID)
	require.NoError(t, err)
	assert.Equal(t, string(session.BacklogStatusQueued), got.Status)
}

func statusBefore(to string) string {
	if to == "ready" {
		return "idea"
	}
	return "ready"
}

func TestMCPCreateBacklogItem_should_LinkBlockers_When_DependsOnGiven(t *testing.T) {
	h, storage := newDepHandlers(t)
	h.backlogSvc = nil // skip auto-triage
	items := newDepItems(t, storage, "blocker")
	ctx := context.Background()

	res, err := h.createBacklogItem(ctx, makeToolReq(map[string]any{
		"title":       "dependent",
		"depends_on":  []any{items[0].ID, items[0].ID}, // duplicate is collapsed
		"skip_triage": true,
	}))
	require.NoError(t, err)
	text := toolText(t, res)
	require.Contains(t, text, "Created backlog item")
	assert.Contains(t, text, "Blocked by "+items[0].ID)

	_, blocking, err := storage.ListBacklogItemDependencies(ctx, items[0].ID)
	require.NoError(t, err)
	assert.Len(t, blocking, 1)
}

func TestMCPCreateBacklogItem_should_CreateNothing_When_DependsOnBlockerMissing(t *testing.T) {
	h, storage := newDepHandlers(t)
	h.backlogSvc = nil
	ctx := context.Background()

	res, err := h.createBacklogItem(ctx, makeToolReq(map[string]any{
		"title": "dependent", "depends_on": []any{missingItemUUID}, "skip_triage": true,
	}))
	require.NoError(t, err)
	assert.Contains(t, toolText(t, res), ErrItemNotFound)

	list, err := storage.ListBacklogItems(context.Background(), session.BacklogItemFilter{})
	require.NoError(t, err)
	assert.Empty(t, list, "a bad depends_on must fail before the item is created")
}
