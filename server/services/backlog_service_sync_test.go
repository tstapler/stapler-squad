package services

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
)

// TestAttachSessionToItem_should_RejectViaConnectError_When_ItemStatusGuardFails confirms
// the connect.CodeFailedPrecondition the MCP link_session_to_item handler depends on
// actually propagates from AttachSessionToItem's item-status guard, for an item in a
// terminal status ("done").
func TestAttachSessionToItem_should_RejectViaConnectError_When_ItemStatusGuardFails(t *testing.T) {
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)

	createResp, err := svc.CreateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
		Title: "terminal status item",
		AcceptanceCriteria: []*sessionv1.AcCriterion{
			{Index: 0, Text: "test", Status: "pending"},
		},
		SkipTriage:   true,
		SkipPlanning: true,
	}))
	require.NoError(t, err)
	itemID := createResp.Msg.Item.Id

	// Walk the state machine to a terminal status: idea -> ready -> in_progress -> review -> done.
	for _, target := range []session.BacklogStatus{
		session.BacklogStatusReady, session.BacklogStatusInProgress,
		session.BacklogStatusReview, session.BacklogStatusDone,
	} {
		_, err := storage.TransitionBacklogItemStatus(t.Context(), itemID, target, nil, session.TriggeredBySystem)
		require.NoError(t, err, "transition to %s", target)
	}

	_, err = svc.AttachSessionToItem(t.Context(), connect.NewRequest(&sessionv1.AttachSessionToItemRequest{
		ItemId:      itemID,
		SessionUuid: "some-session-uuid",
	}))
	require.Error(t, err)

	var connErr *connect.Error
	require.ErrorAs(t, err, &connErr)
	assert.Equal(t, connect.CodeFailedPrecondition, connErr.Code())
	assert.Contains(t, connErr.Error(), "done")
}

// TestAttachSessionToItem_should_NotCallTransitionBacklogItemStatus_When_ConfiguredWorkflowEngineHasDisabledTheEdge
// is the Story 2.1.2 regression test, mirroring
// TestOverrideVerdict_should_RefuseTransition_When_ConfiguredWorkflowEngineHasDisabledTheEdge
// (backlog_service_lifecycle_test.go) for the ready->in_progress call site:
// AttachSessionToItem must not transition the item when the injected engine
// refuses the edge, even though the static validTransitions map allows it.
func TestAttachSessionToItem_should_NotCallTransitionBacklogItemStatus_When_ConfiguredWorkflowEngineHasDisabledTheEdge(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	engine := disabledEdgeWorkflowEngine{
		WorkflowEngine: session.NewDefaultWorkflowEngine(),
		deniedFrom:     session.BacklogStatusReady,
		deniedTo:       session.BacklogStatusInProgress,
	}
	svc := NewBacklogService(storage, nil, nil, engine, nil, nil)

	createResp, err := svc.CreateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
		Title: "item with ready->in_progress disabled by the configured engine",
		AcceptanceCriteria: []*sessionv1.AcCriterion{
			{Index: 0, Text: "test", Status: "pending"},
		},
		SkipTriage:   true,
		SkipPlanning: true,
	}))
	require.NoError(t, err)
	itemID := createResp.Msg.Item.Id

	_, err = storage.TransitionBacklogItemStatus(t.Context(), itemID, session.BacklogStatusReady, nil, session.TriggeredBySystem)
	require.NoError(t, err)

	resp, err := svc.AttachSessionToItem(t.Context(), connect.NewRequest(&sessionv1.AttachSessionToItemRequest{
		ItemId:      itemID,
		SessionUuid: "attach-session-disabled-edge",
	}))
	require.NoError(t, err, "attach itself must still succeed — only the follow-on transition is skipped")
	require.NotNil(t, resp.Msg.ItemSession)

	final, err := storage.GetBacklogItem(t.Context(), itemID)
	require.NoError(t, err)
	assert.Equal(t, string(session.BacklogStatusReady), final.Status, "item must stay at ready when the engine refuses ready->in_progress")
}

// TestAttachSessionToItem_should_RegenerateSlashCommandsWithNewItemID_When_RelinkedToDifferentItem
// satisfies AC5's literal "verified by reading the file contents post-call in a test"
// requirement at the RPC layer: relinking the same live instance to a second item must
// leave done-0.md containing the SECOND item's id, not the first's, and the file set must
// match the second item's AC count exactly (no stale done-N.md beyond it).
func TestAttachSessionToItem_should_RegenerateSlashCommandsWithNewItemID_When_RelinkedToDifferentItem(t *testing.T) {
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
	repoPath := t.TempDir()
	// Distinct from repoPath (item.RepoPath): AttachSessionToItem rejects attaching a
	// session whose working directory IS the item's shared repo checkout — it requires
	// a dedicated worktree/directory (see the "shared repo checkout" guard in
	// backlog_service_sync.go), matching the real-world shape this test simulates.
	worktreePath := t.TempDir()

	makeItem := func(title string, acCount int) string {
		criteria := make([]*sessionv1.AcCriterion, acCount)
		for i := 0; i < acCount; i++ {
			criteria[i] = &sessionv1.AcCriterion{Index: int32(i), Text: "criterion", Status: "pending"}
		}
		resp, err := svc.CreateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
			Title:              title,
			RepoPath:           repoPath,
			AcceptanceCriteria: criteria,
			SkipTriage:         true,
		}))
		require.NoError(t, err)
		return resp.Msg.Item.Id
	}

	item1ID := makeItem("first item", 8)
	item2ID := makeItem("second item", 3)

	const attachUUID = "relink-session-uuid"
	require.NoError(t, storage.AddInstance(&session.Instance{
		Title: "relink-target",
		UUID:  attachUUID,
		Path:  worktreePath,
		// Paused (not Active) so LoadInstances doesn't attempt a real cold-restore
		// tmux/claude process start, matching the precedent in
		// TestAttachSessionToItem_WritesContextFileWithPlanArtifactsAndPriorSessions.
		Status:    session.Paused,
		Program:   "claude",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}))

	_, err := svc.AttachSessionToItem(t.Context(), connect.NewRequest(&sessionv1.AttachSessionToItemRequest{
		ItemId: item1ID, SessionUuid: attachUUID,
	}))
	require.NoError(t, err, "attach to first item (8 AC)")

	cmdDir := filepath.Join(worktreePath, ".claude", "commands", "backlog")
	entries, err := os.ReadDir(cmdDir)
	require.NoError(t, err)
	assert.Len(t, entries, 8*2+6, "8-AC item should have produced done-0..7 + fail-0..7 + status/review/ship/help/block/duplicate")

	_, err = svc.AttachSessionToItem(t.Context(), connect.NewRequest(&sessionv1.AttachSessionToItemRequest{
		ItemId: item2ID, SessionUuid: attachUUID,
	}))
	require.NoError(t, err, "relink to second item (3 AC)")

	entries, err = os.ReadDir(cmdDir)
	require.NoError(t, err)
	names := make(map[string]bool, len(entries))
	for _, e := range entries {
		names[e.Name()] = true
	}
	assert.Len(t, names, 3*2+6, "relinking to a 3-AC item must leave exactly the 3-AC file set, not a superset")
	for i := 3; i < 8; i++ {
		assert.False(t, names[fmt.Sprintf("done-%d.md", i)], "stale done file from the first item must not survive relink")
		assert.False(t, names[fmt.Sprintf("fail-%d.md", i)], "stale fail file from the first item must not survive relink")
	}

	data, err := os.ReadFile(filepath.Join(cmdDir, "done-0.md"))
	require.NoError(t, err)
	content := string(data)
	assert.Contains(t, content, item2ID, "done-0.md must carry the second item's id after relink")
	assert.NotContains(t, content, item1ID, "done-0.md must not carry the stale first item's id after relink")
}
