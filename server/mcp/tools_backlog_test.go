package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	githubpkg "github.com/tstapler/stapler-squad/github"
	"github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/headless"
	"go.uber.org/goleak"
)

// newTestLinkBacklogSvc constructs a real *services.BacklogService for
// link_session_to_item tests — matching this package's established pattern
// (see e.g. TestImportGitHubIssue_should_TriggerTriage_When_BacklogSvcWiredAndRepoPathSet)
// for exercising AttachSessionToItem's actual validation (ITEM_NOT_FOUND,
// FAILED_PRECONDITION) rather than scripting a fake — backlogHandlers.backlogSvc
// is a concrete *services.BacklogService field, not an interface, so a fake
// double can no longer be injected there.
func newTestLinkBacklogSvc(t *testing.T, storage *session.Storage) *services.BacklogService {
	t.Helper()
	svc := services.NewBacklogService(storage, nil, nil, nil, nil, nil)
	t.Cleanup(svc.Shutdown)
	return svc
}

// createTestItem creates a backlog item with the given status for link_session_to_item tests.
func createTestItem(t *testing.T, storage *session.Storage, status string) *session.BacklogItemData {
	t.Helper()
	item, err := storage.CreateBacklogItem(context.Background(), session.BacklogItemData{
		Title:              "Link test item",
		Description:        "Item used to test link_session_to_item",
		AcceptanceCriteria: `[{"index":0,"text":"Criterion","status":"pending"}]`,
		Priority:           1,
		Status:             status,
	})
	require.NoError(t, err)
	return item
}

func TestLinkSessionToItem_should_CreateItemSession_When_ValidUnlinkedItem(t *testing.T) {
	storage := newTestBacklogStorage(t)
	item := createTestItem(t, storage, string(session.BacklogStatusInProgress))
	backlogSvc := newTestLinkBacklogSvc(t, storage)
	handler := &backlogHandlers{storage: storage, backlogSvc: backlogSvc}
	callerUUID := uuid.New().String()
	ctx := WithSessionUUID(context.Background(), callerUUID)

	result, err := handler.linkSessionToItem(ctx, makeToolReq(map[string]interface{}{"item_id": item.ID}))
	require.NoError(t, err)

	m := parseResult(t, result)
	require.True(t, m["success"].(bool))
	assert.False(t, m["already_linked"].(bool))
	assert.Equal(t, item.ID, m["item_id"].(string))

	_, lookupErr := storage.GetItemSessionBySessionAndItem(context.Background(), callerUUID, item.ID)
	assert.NoError(t, lookupErr, "AttachSessionToItem must have created a real ItemSession row")
}

func TestLinkSessionToItem_should_UnblockReportProgress_When_CalledAgainstRealStorage(t *testing.T) {
	storage := newTestBacklogStorage(t)
	item := createTestItem(t, storage, string(session.BacklogStatusInProgress))
	callerUUID := uuid.New().String()
	ctx := WithSessionUUID(context.Background(), callerUUID)

	backlogSvc := newTestLinkBacklogSvc(t, storage)
	handler := &backlogHandlers{storage: storage, backlogSvc: backlogSvc}
	linkResult, err := handler.linkSessionToItem(ctx, makeToolReq(map[string]interface{}{"item_id": item.ID}))
	require.NoError(t, err)
	require.True(t, parseResult(t, linkResult)["success"].(bool))

	progressResult, err := handler.reportProgress(ctx, makeToolReq(map[string]interface{}{
		"item_id": item.ID, "criteria_index": float64(0), "status": "pass",
	}))
	require.NoError(t, err)
	// reportProgress returns plain text (not a JSON MCPResult) on success — confirm
	// it did not return the PERMISSION_DENIED JSON error shape.
	require.Len(t, progressResult.Content, 1)
	tc, ok := progressResult.Content[0].(mcpgo.TextContent)
	require.True(t, ok)
	assert.NotContains(t, tc.Text, string(ErrPermissionDenied))
}

func TestLinkSessionToItem_should_ShortCircuitToNoOp_When_AlreadyLinkedToSameItem(t *testing.T) {
	storage := newTestBacklogStorage(t)
	item := createTestItem(t, storage, string(session.BacklogStatusInProgress))
	callerUUID := uuid.New().String()
	ctx := WithSessionUUID(context.Background(), callerUUID)
	_, err := storage.CreateItemSession(context.Background(), session.ItemSessionData{
		ItemID: item.ID, SessionUUID: callerUUID, SessionRole: session.SessionRoleWork,
	})
	require.NoError(t, err)

	backlogSvc := newTestLinkBacklogSvc(t, storage)
	handler := &backlogHandlers{storage: storage, backlogSvc: backlogSvc}

	result, err := handler.linkSessionToItem(ctx, makeToolReq(map[string]interface{}{"item_id": item.ID}))
	require.NoError(t, err)
	m := parseResult(t, result)
	require.True(t, m["success"].(bool))
	assert.True(t, m["already_linked"].(bool))

	sessions, listErr := storage.ListItemSessions(context.Background(), item.ID)
	require.NoError(t, listErr)
	assert.Len(t, sessions, 1, "attach must not be re-triggered on an idempotent relink — no duplicate row")
}

func TestLinkSessionToItem_should_ReturnConflict_When_ItemClaimedByOtherLiveSession(t *testing.T) {
	storage := newTestBacklogStorage(t)
	item := createTestItem(t, storage, string(session.BacklogStatusInProgress))
	otherUUID := uuid.New().String()
	_, err := storage.CreateItemSession(context.Background(), session.ItemSessionData{
		ItemID: item.ID, SessionUUID: otherUUID, SessionRole: session.SessionRoleWork,
	})
	require.NoError(t, err)

	callerUUID := uuid.New().String()
	ctx := WithSessionUUID(context.Background(), callerUUID)
	backlogSvc := newTestLinkBacklogSvc(t, storage)
	handler := &backlogHandlers{storage: storage, backlogSvc: backlogSvc}

	result, err := handler.linkSessionToItem(ctx, makeToolReq(map[string]interface{}{"item_id": item.ID}))
	require.NoError(t, err)
	m := parseResult(t, result)
	require.False(t, m["success"].(bool))
	errObj := m["error"].(map[string]interface{})
	assert.Equal(t, ErrConflict, errObj["code"].(string))
	assert.NotContains(t, errObj["message"].(string), otherUUID,
		"the owning session's UUID must never be disclosed to a different caller — it doubles as this MCP layer's bearer credential")

	// No new row was created for the caller.
	_, lookupErr := storage.GetItemSessionBySessionAndItem(context.Background(), callerUUID, item.ID)
	assert.Error(t, lookupErr)
}

func TestLinkSessionToItem_should_CreateItemSession_When_ConflictingRowOwnerIsLivenessDead(t *testing.T) {
	storage := newTestBacklogStorage(t)
	item := createTestItem(t, storage, string(session.BacklogStatusInProgress))
	deadUUID := uuid.New().String()
	_, err := storage.CreateItemSession(context.Background(), session.ItemSessionData{
		ItemID: item.ID, SessionUUID: deadUUID, SessionRole: session.SessionRoleWork,
	})
	require.NoError(t, err)

	callerUUID := uuid.New().String()
	ctx := WithSessionUUID(context.Background(), callerUUID)
	backlogSvc := newTestLinkBacklogSvc(t, storage)
	handler := &backlogHandlers{
		storage: storage, backlogSvc: backlogSvc,
		liveCheck: func(sessionUUID string) bool { return sessionUUID != deadUUID },
	}

	result, err := handler.linkSessionToItem(ctx, makeToolReq(map[string]interface{}{"item_id": item.ID}))
	require.NoError(t, err)
	m := parseResult(t, result)
	require.True(t, m["success"].(bool), "a liveness-dead owner's row must not block a relink")
	assert.False(t, m["already_linked"].(bool))

	// Confirm the nil-liveCheck fallback still returns CONFLICT (preserves pre-feature behavior).
	handlerNoLiveCheck := &backlogHandlers{storage: storage, backlogSvc: backlogSvc}
	ctx2 := WithSessionUUID(context.Background(), uuid.New().String())
	result2, err := handlerNoLiveCheck.linkSessionToItem(ctx2, makeToolReq(map[string]interface{}{"item_id": item.ID}))
	require.NoError(t, err)
	m2 := parseResult(t, result2)
	require.False(t, m2["success"].(bool))
	assert.Equal(t, ErrConflict, m2["error"].(map[string]interface{})["code"].(string))
}

func TestLinkSessionToItem_should_ReturnItemNotFound_When_ItemIdDoesNotExist(t *testing.T) {
	storage := newTestBacklogStorage(t)
	backlogSvc := newTestLinkBacklogSvc(t, storage)
	handler := &backlogHandlers{storage: storage, backlogSvc: backlogSvc}
	ctx := WithSessionUUID(context.Background(), uuid.New().String())

	result, err := handler.linkSessionToItem(ctx, makeToolReq(map[string]interface{}{"item_id": "00000000-0000-0000-0000-000000000099"}))
	require.NoError(t, err)
	m := parseResult(t, result)
	require.False(t, m["success"].(bool))
	assert.Equal(t, ErrItemNotFound, m["error"].(map[string]interface{})["code"].(string))
}

func TestLinkSessionToItem_should_ReturnFailedPrecondition_When_ItemInTerminalStatus(t *testing.T) {
	storage := newTestBacklogStorage(t)
	item := createTestItem(t, storage, string(session.BacklogStatusDone))
	backlogSvc := newTestLinkBacklogSvc(t, storage)
	handler := &backlogHandlers{storage: storage, backlogSvc: backlogSvc}
	ctx := WithSessionUUID(context.Background(), uuid.New().String())

	result, err := handler.linkSessionToItem(ctx, makeToolReq(map[string]interface{}{"item_id": item.ID}))
	require.NoError(t, err)
	m := parseResult(t, result)
	require.False(t, m["success"].(bool))
	assert.Equal(t, ErrFailedPrecondition, m["error"].(map[string]interface{})["code"].(string))
}

func TestLinkSessionToItem_should_ReturnUnavailable_When_AttacherNil(t *testing.T) {
	storage := newTestBacklogStorage(t)
	handler := &backlogHandlers{storage: storage, backlogSvc: nil}
	ctx := WithSessionUUID(context.Background(), uuid.New().String())

	result, err := handler.linkSessionToItem(ctx, makeToolReq(map[string]interface{}{"item_id": uuid.New().String()}))
	require.NoError(t, err)
	m := parseResult(t, result)
	require.False(t, m["success"].(bool))
	assert.Equal(t, ErrUnavailable, m["error"].(map[string]interface{})["code"].(string))
}

func TestGetLinkedItem_should_ReturnMostRecentLink_When_NoItemIdProvided(t *testing.T) {
	storage := newTestBacklogStorage(t)
	olderItem := createTestItem(t, storage, string(session.BacklogStatusInProgress))
	newerItem := createTestItem(t, storage, string(session.BacklogStatusInProgress))
	callerUUID := uuid.New().String()
	ctx := WithSessionUUID(context.Background(), callerUUID)

	_, err := storage.CreateItemSession(context.Background(), session.ItemSessionData{
		ItemID: olderItem.ID, SessionUUID: callerUUID, SessionRole: session.SessionRoleWork,
	})
	require.NoError(t, err)
	time.Sleep(2 * time.Millisecond) // ensure distinct created_at ordering
	_, err = storage.CreateItemSession(context.Background(), session.ItemSessionData{
		ItemID: newerItem.ID, SessionUUID: callerUUID, SessionRole: session.SessionRoleWork,
	})
	require.NoError(t, err)

	handler := &backlogHandlers{storage: storage}
	result, err := handler.getLinkedItem(ctx, makeToolReq(nil))
	require.NoError(t, err)
	m := parseResult(t, result)
	require.True(t, m["success"].(bool))
	assert.True(t, m["linked"].(bool))
	assert.Equal(t, newerItem.ID, m["item_id"].(string))
}

func TestGetLinkedItem_should_ReturnNotLinked_When_SessionHasNoItemSessionRows(t *testing.T) {
	storage := newTestBacklogStorage(t)
	handler := &backlogHandlers{storage: storage}
	ctx := WithSessionUUID(context.Background(), uuid.New().String())

	result, err := handler.getLinkedItem(ctx, makeToolReq(nil))
	require.NoError(t, err)
	m := parseResult(t, result)
	require.True(t, m["success"].(bool), "not-linked is a valid read result, not an error")
	assert.False(t, m["linked"].(bool))
}

func TestGetLinkedItem_should_ReturnLinkedFalseForSpecificItem_When_SessionLinkedToDifferentItem(t *testing.T) {
	storage := newTestBacklogStorage(t)
	linkedItem := createTestItem(t, storage, string(session.BacklogStatusInProgress))
	otherItem := createTestItem(t, storage, string(session.BacklogStatusInProgress))
	callerUUID := uuid.New().String()
	ctx := WithSessionUUID(context.Background(), callerUUID)
	_, err := storage.CreateItemSession(context.Background(), session.ItemSessionData{
		ItemID: linkedItem.ID, SessionUUID: callerUUID, SessionRole: session.SessionRoleWork,
	})
	require.NoError(t, err)

	handler := &backlogHandlers{storage: storage}
	result, err := handler.getLinkedItem(ctx, makeToolReq(map[string]interface{}{"item_id": otherItem.ID}))
	require.NoError(t, err)
	m := parseResult(t, result)
	require.True(t, m["success"].(bool))
	assert.False(t, m["linked"].(bool))
	assert.Equal(t, otherItem.ID, m["item_id"].(string))
}

func TestResolveItemLink_should_NameBothItemsAndLinkTool_When_LinkedToDifferentItem(t *testing.T) {
	storage := newTestBacklogStorage(t)
	linkedItem := createTestItem(t, storage, string(session.BacklogStatusInProgress))
	otherItem := createTestItem(t, storage, string(session.BacklogStatusInProgress))
	callerUUID := uuid.New().String()
	_, err := storage.CreateItemSession(context.Background(), session.ItemSessionData{
		ItemID: linkedItem.ID, SessionUUID: callerUUID, SessionRole: session.SessionRoleWork,
	})
	require.NoError(t, err)

	handler := &backlogHandlers{storage: storage}
	_, result := handler.resolveItemLink(context.Background(), callerUUID, otherItem.ID)
	require.NotNil(t, result)
	m := parseResult(t, result)
	require.False(t, m["success"].(bool))
	errObj := m["error"].(map[string]interface{})
	assert.Equal(t, ErrPermissionDenied, errObj["code"].(string))
	assert.Contains(t, errObj["remediation"].(string), linkedItem.ID)
	assert.Contains(t, errObj["remediation"].(string), otherItem.ID)
	assert.Contains(t, errObj["remediation"].(string), "link_session_to_item")
}

func TestResolveItemLink_should_NameLinkToolWithNoPriorLink_When_SessionHasNoLink(t *testing.T) {
	storage := newTestBacklogStorage(t)
	item := createTestItem(t, storage, string(session.BacklogStatusInProgress))
	handler := &backlogHandlers{storage: storage}

	_, result := handler.resolveItemLink(context.Background(), uuid.New().String(), item.ID)
	require.NotNil(t, result)
	m := parseResult(t, result)
	require.False(t, m["success"].(bool))
	errObj := m["error"].(map[string]interface{})
	assert.Equal(t, ErrPermissionDenied, errObj["code"].(string))
	assert.Contains(t, errObj["remediation"].(string), "link_session_to_item")
	assert.Contains(t, errObj["remediation"].(string), item.ID)
}

func TestReportProgress_should_ReturnActionableHint_When_SessionNotLinked(t *testing.T) {
	storage := newTestBacklogStorage(t)
	item := createTestItem(t, storage, string(session.BacklogStatusInProgress))
	handler := &backlogHandlers{storage: storage}
	ctx := WithSessionUUID(context.Background(), uuid.New().String())

	result, err := handler.reportProgress(ctx, makeToolReq(map[string]interface{}{
		"item_id": item.ID, "criteria_index": float64(0), "status": "pass",
	}))
	require.NoError(t, err)
	m := parseResult(t, result)
	require.False(t, m["success"].(bool))
	errObj := m["error"].(map[string]interface{})
	assert.Contains(t, errObj["remediation"].(string), "link_session_to_item")
}

func TestRegisterBacklogTools_should_RegisterLinkSessionToItemTool_When_ToolsRegistered(t *testing.T) {
	data, err := os.ReadFile("tools_backlog.go")
	require.NoError(t, err)
	content := string(data)
	assert.Contains(t, content, `mcpgo.NewTool("link_session_to_item"`)
	assert.Contains(t, content, `mcpgo.NewTool("get_linked_item"`)
}

// newTestBacklogStorage creates a temporary Storage for testing.
func newTestBacklogStorage(t *testing.T) *session.Storage {
	t.Helper()
	repo := session.NewTestEntRepository(t)

	storage, err := session.NewStorageWithRepository(repo)
	require.NoError(t, err)

	return storage
}

// TestReportProgress_RejectsWhenNoSessionUUID verifies that reportProgress
// returns PERMISSION_DENIED when STAPLER_SESSION_UUID is not in context.
func TestReportProgress_RejectsWhenNoSessionUUID(t *testing.T) {
	storage := newTestBacklogStorage(t)
	handler := &backlogHandlers{storage: storage}

	ctx := context.Background() // No session UUID injected

	req := makeToolReq(map[string]interface{}{
		"item_id":        "00000000-0000-0000-0000-000000000001",
		"criteria_index": float64(0),
		"status":         "pass",
	})

	result, err := handler.reportProgress(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, result)

	m := parseResult(t, result)
	require.False(t, m["success"].(bool))

	errObj, ok := m["error"].(map[string]interface{})
	require.True(t, ok)

	errCode, ok := errObj["code"].(string)
	require.True(t, ok)
	require.Equal(t, ErrPermissionDenied, errCode)
}

// TestReportProgress_RejectsWhenSessionNotLinkedToItem verifies that reportProgress
// returns PERMISSION_DENIED when the session is not linked to the specified item.
func TestReportProgress_RejectsWhenSessionNotLinkedToItem(t *testing.T) {
	storage := newTestBacklogStorage(t)
	ctx := context.Background()

	// Create two items
	item1Data := session.BacklogItemData{
		Title:              "Item 1",
		Description:        "First item",
		AcceptanceCriteria: `[{"index":0,"text":"Criterion","status":"pending"}]`,
		Priority:           1,
		Status:             string(session.BacklogStatusInProgress),
	}
	item1, err := storage.CreateBacklogItem(ctx, item1Data)
	require.NoError(t, err)

	item2Data := session.BacklogItemData{
		Title:              "Item 2",
		Description:        "Second item",
		AcceptanceCriteria: `[{"index":0,"text":"Criterion","status":"pending"}]`,
		Priority:           2,
		Status:             string(session.BacklogStatusInProgress),
	}
	item2, err := storage.CreateBacklogItem(ctx, item2Data)
	require.NoError(t, err)

	// Create session linked to item1
	sessionUUID := uuid.New().String()
	isData := session.ItemSessionData{
		ItemID:      item1.ID,
		SessionUUID: sessionUUID,
		SessionRole: "work",
	}
	_, err = storage.CreateItemSession(ctx, isData)
	require.NoError(t, err)

	// Try to report progress on item2 (not linked to this session)
	handler := &backlogHandlers{storage: storage}
	ctxWithUUID := WithSessionUUID(context.Background(), sessionUUID)

	req := makeToolReq(map[string]interface{}{
		"item_id":        item2.ID,
		"criteria_index": float64(0),
		"status":         "pass",
	})

	result, err := handler.reportProgress(ctxWithUUID, req)
	require.NoError(t, err)

	m := parseResult(t, result)
	require.False(t, m["success"].(bool))

	errObj, ok := m["error"].(map[string]interface{})
	require.True(t, ok)

	errCode, ok := errObj["code"].(string)
	require.True(t, ok)
	require.Equal(t, ErrPermissionDenied, errCode)
}

// TestReportProgress_SuccessfullyUpdatesAcStatus verifies that reportProgress
// successfully updates AC criterion status when session is properly linked.
func TestReportProgress_SuccessfullyUpdatesAcStatus(t *testing.T) {
	storage := newTestBacklogStorage(t)
	ctx := context.Background()

	// Create item
	itemData := session.BacklogItemData{
		Title:              "Test item",
		Description:        "Item for testing",
		AcceptanceCriteria: `[{"index":0,"text":"Must work","status":"pending"},{"index":1,"text":"Tests pass","status":"pending"}]`,
		Priority:           1,
		Status:             string(session.BacklogStatusInProgress),
	}
	item, err := storage.CreateBacklogItem(ctx, itemData)
	require.NoError(t, err)

	// Create session linked to item
	sessionUUID := uuid.New().String()
	isData := session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: sessionUUID,
		SessionRole: "work",
	}
	_, err = storage.CreateItemSession(ctx, isData)
	require.NoError(t, err)

	// Report progress on criterion 0
	handler := &backlogHandlers{storage: storage}
	ctxWithUUID := WithSessionUUID(context.Background(), sessionUUID)

	req := makeToolReq(map[string]interface{}{
		"item_id":        item.ID,
		"criteria_index": float64(0),
		"status":         "pass",
		"note":           "implemented successfully",
	})

	result, err := handler.reportProgress(ctxWithUUID, req)
	require.NoError(t, err)

	// Success returns plain text, not JSON
	require.Len(t, result.Content, 1)
	tc, ok := result.Content[0].(mcpgo.TextContent)
	require.True(t, ok)
	require.Contains(t, tc.Text, "Criterion")
	require.Contains(t, tc.Text, "updated")

	// Verify the criterion was updated
	fetchedItem, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)

	criteria, err := session.ParseAcCriteria(fetchedItem.AcceptanceCriteria)
	require.NoError(t, err)
	require.Len(t, criteria, 2)
	require.Equal(t, session.AcStatusDone, criteria[0].Status, "criterion 0 should be marked done")
	require.Equal(t, session.AcStatusPending, criteria[1].Status, "criterion 1 should remain pending")
}

// TestReportProgress_AppendsProgressNoteHistory verifies that reportProgress creates
// a progress-note history row (AppendProgressNote) alongside the existing
// overwrite-in-place AC criterion update — the history call is additive, not a
// replacement for the existing behavior.
func TestReportProgress_AppendsProgressNoteHistory(t *testing.T) {
	storage := newTestBacklogStorage(t)
	ctx := context.Background()

	itemData := session.BacklogItemData{
		Title:              "Test item",
		Description:        "Item for testing",
		AcceptanceCriteria: `[{"index":0,"text":"Must work","status":"pending"}]`,
		Priority:           1,
		Status:             string(session.BacklogStatusInProgress),
	}
	item, err := storage.CreateBacklogItem(ctx, itemData)
	require.NoError(t, err)

	sessionUUID := uuid.New().String()
	isData := session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: sessionUUID,
		SessionRole: "work",
	}
	_, err = storage.CreateItemSession(ctx, isData)
	require.NoError(t, err)

	handler := &backlogHandlers{storage: storage}
	ctxWithUUID := WithSessionUUID(context.Background(), sessionUUID)

	// First call: in_progress with a note.
	req1 := makeToolReq(map[string]interface{}{
		"item_id":        item.ID,
		"criteria_index": float64(0),
		"status":         "in_progress",
		"note":           "started investigating",
	})
	result1, err := handler.reportProgress(ctxWithUUID, req1)
	require.NoError(t, err)
	require.Len(t, result1.Content, 1)

	// Second call: pass with a different note, same criterion — the existing
	// overwrite behavior replaces the criterion's current note, but the history
	// must retain BOTH notes.
	req2 := makeToolReq(map[string]interface{}{
		"item_id":        item.ID,
		"criteria_index": float64(0),
		"status":         "pass",
		"note":           "implemented successfully",
	})
	result2, err := handler.reportProgress(ctxWithUUID, req2)
	require.NoError(t, err)
	require.Len(t, result2.Content, 1)

	// The current-note-per-criterion behavior is unchanged: only the latest note
	// is visible on the AC criterion itself.
	fetchedItem, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	criteria, err := session.ParseAcCriteria(fetchedItem.AcceptanceCriteria)
	require.NoError(t, err)
	require.Len(t, criteria, 1)
	require.Equal(t, session.AcStatusDone, criteria[0].Status)
	require.Equal(t, "implemented successfully", criteria[0].Note, "current note is overwritten by the latest call, as before")

	// The new history log must contain BOTH notes, in order.
	notes, err := storage.ListProgressNotesForItem(ctx, item.ID)
	require.NoError(t, err)
	require.Len(t, notes, 2, "both report_progress calls must be preserved in the history")

	require.Equal(t, "started investigating", notes[0].Note)
	require.Equal(t, "in_progress", notes[0].Status)

	require.Equal(t, "implemented successfully", notes[1].Note)
	require.Equal(t, "done", notes[1].Status)
}

// TestGetBacklogItem_ReturnsItemWithEnvelope verifies that getBacklogItem
// returns a properly formatted envelope with item data.
func TestGetBacklogItem_ReturnsItemWithEnvelope(t *testing.T) {
	storage := newTestBacklogStorage(t)
	ctx := context.Background()

	// Create item
	itemData := session.BacklogItemData{
		Title:              "Feature: User login",
		Description:        "Implement user authentication flow",
		AcceptanceCriteria: `[{"index":0,"text":"User can login with email","status":"pending"},{"index":1,"text":"Password is hashed","status":"pending"}]`,
		Priority:           1,
		Status:             string(session.BacklogStatusInProgress),
	}
	item, err := storage.CreateBacklogItem(ctx, itemData)
	require.NoError(t, err)

	// Call getBacklogItem
	handler := &backlogHandlers{storage: storage}
	req := makeToolReq(map[string]interface{}{
		"item_id": item.ID,
	})

	result, err := handler.getBacklogItem(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Extract text content
	require.Len(t, result.Content, 1)
	tc, ok := result.Content[0].(mcpgo.TextContent)
	require.True(t, ok)

	text := tc.Text

	// Verify envelope markers
	require.Contains(t, text, "--- BACKLOG ITEM DATA", "should contain envelope header")
	require.Contains(t, text, "--- END BACKLOG ITEM DATA", "should contain envelope footer")

	// Verify item content within envelope
	require.Contains(t, text, "Feature: User login", "should contain item title")
	require.Contains(t, text, "Implement user authentication flow", "should contain description")
	require.Contains(t, text, "User can login with email", "should contain AC criterion")
	require.Contains(t, text, "Password is hashed", "should contain second AC criterion")
	require.Contains(t, text, "report_progress", "should list available tools")
}

// TestGetBacklogItem_WorkRoleGuidance_InstructsShipOnPassAndAfterAttemptCap
// verifies the role:work guidance a live work session reads on every
// get_backlog_item/backlog status poll tells it to run /backlog/ship both
// immediately on a PASS verdict and as a bounded escape hatch after
// session.MaxSameSessionReviewAttempts cycles without one — closing the gap
// where this text previously said "PASS → status becomes done, you're
// finished" and "Keep looping until PASS" with no mention of /backlog/ship at
// all (see de6d7878-9d6e-4081-acfa-02ff545c87b4, 2026-07-20). This is the
// dynamic counterpart to session.taskProtocolBlock and review.md — the text a
// running session actually re-reads on every poll, not just at session start.
func TestGetBacklogItem_WorkRoleGuidance_InstructsShipOnPassAndAfterAttemptCap(t *testing.T) {
	storage := newTestBacklogStorage(t)
	ctx := context.Background()

	itemData := session.BacklogItemData{
		Title:  "Ship me",
		Status: string(session.BacklogStatusInProgress),
	}
	item, err := storage.CreateBacklogItem(ctx, itemData)
	require.NoError(t, err)

	sessionUUID := uuid.New().String()
	_, err = storage.CreateItemSession(ctx, session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: sessionUUID,
		SessionRole: session.SessionRoleWork,
	})
	require.NoError(t, err)

	handler := &backlogHandlers{storage: storage}
	ctxWithUUID := WithSessionUUID(ctx, sessionUUID)
	req := makeToolReq(map[string]interface{}{
		"item_id": item.ID,
	})

	result, err := handler.getBacklogItem(ctxWithUUID, req)
	require.NoError(t, err)
	require.Len(t, result.Content, 1)
	tc, ok := result.Content[0].(mcpgo.TextContent)
	require.True(t, ok)
	text := tc.Text

	require.Contains(t, text, "Your Role: Work")
	require.Contains(t, text, "/backlog/ship")
	require.Contains(t, text, fmt.Sprintf("%d cycles", session.MaxSameSessionReviewAttempts))
	require.NotContains(t, text, "you're finished", "a PASS verdict must no longer read as the end of the task")
}

// TestGetBacklogItem_ReviewRoleGuidance_InstructsEndingSessionAfterVerdict is
// the regression guard for BUG-047 (a reviewer that submits a verdict and
// then never exits leaves the item wedged in "review" forever). Symmetric to
// TestGetBacklogItem_WorkRoleGuidance_InstructsShipOnPassAndAfterAttemptCap's
// coverage of the work role's "Do NOT end your session" instruction.
func TestGetBacklogItem_ReviewRoleGuidance_InstructsEndingSessionAfterVerdict(t *testing.T) {
	storage := newTestBacklogStorage(t)
	ctx := context.Background()

	itemData := session.BacklogItemData{
		Title:  "Review me",
		Status: string(session.BacklogStatusReview),
	}
	item, err := storage.CreateBacklogItem(ctx, itemData)
	require.NoError(t, err)

	sessionUUID := uuid.New().String()
	_, err = storage.CreateItemSession(ctx, session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: sessionUUID,
		SessionRole: session.SessionRoleReview,
	})
	require.NoError(t, err)

	handler := &backlogHandlers{storage: storage}
	ctxWithUUID := WithSessionUUID(ctx, sessionUUID)
	req := makeToolReq(map[string]interface{}{
		"item_id": item.ID,
	})

	result, err := handler.getBacklogItem(ctxWithUUID, req)
	require.NoError(t, err)
	require.Len(t, result.Content, 1)
	tc, ok := result.Content[0].(mcpgo.TextContent)
	require.True(t, ok)
	text := tc.Text

	require.Contains(t, text, "Your Role: Review")
	require.Contains(t, text, "End your session immediately after calling submit_review_verdict")
}

// TestGetBacklogItem_ReturnsNotFoundError verifies that getBacklogItem
// returns an error when item doesn't exist.
func TestGetBacklogItem_ReturnsNotFoundError(t *testing.T) {
	storage := newTestBacklogStorage(t)
	handler := &backlogHandlers{storage: storage}

	nonExistentID := "00000000-0000-0000-0000-000000000999"
	req := makeToolReq(map[string]interface{}{
		"item_id": nonExistentID,
	})

	result, err := handler.getBacklogItem(context.Background(), req)
	require.NoError(t, err)

	m := parseResult(t, result)
	require.False(t, m["success"].(bool))

	errObj, ok := m["error"].(map[string]interface{})
	require.True(t, ok)

	errCode, ok := errObj["code"].(string)
	require.True(t, ok)
	require.Equal(t, ErrItemNotFound, errCode)
}

// TestReportProgress_ValidatesStatusValues verifies that reportProgress
// rejects invalid status values.
func TestReportProgress_ValidatesStatusValues(t *testing.T) {
	storage := newTestBacklogStorage(t)
	handler := &backlogHandlers{storage: storage}

	sessionUUID := uuid.New().String()
	ctxWithUUID := WithSessionUUID(context.Background(), sessionUUID)

	req := makeToolReq(map[string]interface{}{
		"item_id":        "00000000-0000-0000-0000-000000000001",
		"criteria_index": float64(0),
		"status":         "invalid_status",
	})

	result, err := handler.reportProgress(ctxWithUUID, req)
	require.NoError(t, err)

	m := parseResult(t, result)
	require.False(t, m["success"].(bool))

	errObj, ok := m["error"].(map[string]interface{})
	require.True(t, ok)

	errCode, ok := errObj["code"].(string)
	require.True(t, ok)
	require.Equal(t, ErrInvalidArgument, errCode)
}

// TestReportProgress_MapsStatusValues verifies that "pass" is mapped to "done"
// and other values are passed through correctly.
func TestReportProgress_MapsStatusValues(t *testing.T) {
	storage := newTestBacklogStorage(t)
	ctx := context.Background()

	// Create item
	itemData := session.BacklogItemData{
		Title:              "Status mapping test",
		Description:        "Test status mapping",
		AcceptanceCriteria: `[{"index":0,"text":"Criterion","status":"pending"}]`,
		Priority:           1,
		Status:             string(session.BacklogStatusInProgress),
	}
	item, err := storage.CreateBacklogItem(ctx, itemData)
	require.NoError(t, err)

	// Create session
	sessionUUID := uuid.New().String()
	isData := session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: sessionUUID,
		SessionRole: "work",
	}
	_, err = storage.CreateItemSession(ctx, isData)
	require.NoError(t, err)

	handler := &backlogHandlers{storage: storage}
	ctxWithUUID := WithSessionUUID(context.Background(), sessionUUID)

	// Test "pass" → "done" mapping
	req := makeToolReq(map[string]interface{}{
		"item_id":        item.ID,
		"criteria_index": float64(0),
		"status":         "pass",
	})

	result, err := handler.reportProgress(ctxWithUUID, req)
	require.NoError(t, err)

	// Success returns plain text, not JSON
	require.Len(t, result.Content, 1)
	tc, ok := result.Content[0].(mcpgo.TextContent)
	require.True(t, ok)
	require.Contains(t, tc.Text, "Criterion")
	require.Contains(t, tc.Text, "updated")

	// Verify criterion is marked "done"
	fetchedItem, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)

	criteria, err := session.ParseAcCriteria(fetchedItem.AcceptanceCriteria)
	require.NoError(t, err)
	require.Equal(t, session.AcStatusDone, criteria[0].Status, "pass should be mapped to done")
}

// ─── T-11 tests 7 & 8: submitTriageResult notification publishing ─────────────

// setupTriageSession creates a backlog item, links a triage session with role=triage,
// and returns the item ID and session UUID for use in notification tests.
func setupTriageSession(t *testing.T, storage *session.Storage) (itemID, sessionUUID string) {
	t.Helper()
	ctx := context.Background()

	itemData := session.BacklogItemData{
		Title:    "Triage notification test item",
		Status:   string(session.BacklogStatusIdea),
		Priority: 2,
	}
	item, err := storage.CreateBacklogItem(ctx, itemData)
	require.NoError(t, err)

	sessUUID := uuid.New().String()
	_, isErr := storage.CreateItemSession(ctx, session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: sessUUID,
		SessionRole: session.SessionRoleTriage,
	})
	require.NoError(t, isErr)

	return item.ID, sessUUID
}

// TestSubmitTriageResult_PublishesNotificationOnSuccess verifies that a triage-complete
// notification is published to the EventBus with item_id in metadata.
func TestSubmitTriageResult_PublishesNotificationOnSuccess(t *testing.T) {
	storage := newTestBacklogStorage(t)
	bus := events.NewEventBus(32)

	itemID, sessUUID := setupTriageSession(t, storage)

	handler := &backlogHandlers{storage: storage, eventBus: bus}
	ctxWithUUID := WithSessionUUID(context.Background(), sessUUID)

	// Subscribe before submitting so we capture the event.
	subCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	eventCh, _ := bus.Subscribe(subCtx)

	req := makeToolReq(map[string]interface{}{
		"item_id": itemID,
		"summary": "Item looks reasonable",
		"suggestions": []interface{}{
			map[string]interface{}{"text": "Add tests", "rationale": "coverage"},
		},
	})

	result, err := handler.submitTriageResult(ctxWithUUID, req)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify success response.
	require.Len(t, result.Content, 1)
	tc, ok := result.Content[0].(mcpgo.TextContent)
	require.True(t, ok)
	require.Contains(t, tc.Text, "Triage result submitted")

	// Expect one event on the bus.
	select {
	case event := <-eventCh:
		require.NotNil(t, event)
		assert.Equal(t, itemID, event.NotificationMetadata["item_id"], "event should carry item_id in metadata")
		assert.Equal(t, "Triage complete", event.NotificationTitle)
	case <-time.After(2 * time.Second):
		t.Fatal("expected notification event was not published within 2s")
	}
}

// TestSubmitTriageResult_AppliesPriorityAndItemCategory_When_Provided guards the
// interactive (non-headless) triage path's own priority/item_category assignment —
// the same fix applied to the headless TriggerTriage path
// (applyTriageResultToUpdate, server/services/backlog_service_triage.go), so an
// agent-driven "sdd" pipeline-mode triage session assigns labels/priority too, not
// just the headless one.
func TestSubmitTriageResult_AppliesPriorityAndItemCategory_When_Provided(t *testing.T) {
	storage := newTestBacklogStorage(t)
	itemID, sessUUID := setupTriageSession(t, storage) // created with Priority: 2

	handler := &backlogHandlers{storage: storage}
	ctxWithUUID := WithSessionUUID(context.Background(), sessUUID)

	req := makeToolReq(map[string]interface{}{
		"item_id":       itemID,
		"summary":       "Critical bug found",
		"priority":      float64(1),
		"item_category": "bugfix",
		"suggestions": []interface{}{
			map[string]interface{}{"text": "Add tests", "rationale": "coverage"},
		},
	})

	_, err := handler.submitTriageResult(ctxWithUUID, req)
	require.NoError(t, err)

	updated, loadErr := storage.GetBacklogItem(context.Background(), itemID)
	require.NoError(t, loadErr)
	assert.Equal(t, 1, updated.Priority, "the assessed priority must be applied")
	assert.Equal(t, "bugfix", updated.Category, "the assessed item_category must be applied")
}

// TestSubmitTriageResult_IgnoresInvalidPriorityAndCategory_When_OutOfRange verifies
// an out-of-range priority or an invalid item_category is silently ignored (item's
// existing priority/category untouched) rather than corrupting the item — same
// convention as applyTriageResultToUpdate's headless-path equivalent.
func TestSubmitTriageResult_IgnoresInvalidPriorityAndCategory_When_OutOfRange(t *testing.T) {
	storage := newTestBacklogStorage(t)
	itemID, sessUUID := setupTriageSession(t, storage) // created with Priority: 2

	handler := &backlogHandlers{storage: storage}
	ctxWithUUID := WithSessionUUID(context.Background(), sessUUID)

	req := makeToolReq(map[string]interface{}{
		"item_id":       itemID,
		"summary":       "Out of range values",
		"priority":      float64(9),
		"item_category": "not-a-real-category",
	})

	_, err := handler.submitTriageResult(ctxWithUUID, req)
	require.NoError(t, err)

	updated, loadErr := storage.GetBacklogItem(context.Background(), itemID)
	require.NoError(t, loadErr)
	assert.Equal(t, 2, updated.Priority, "an out-of-range priority must not be applied")
	assert.Empty(t, updated.Category, "an invalid item_category must not be applied")
}

// TestRequestReview_TransitionsItemToReview verifies that requestReview
// transitions the item from in_progress to review.
func TestRequestReview_TransitionsItemToReview(t *testing.T) {
	storage := newTestBacklogStorage(t)
	ctx := context.Background()

	itemData := session.BacklogItemData{
		Title:  "Review me",
		Status: string(session.BacklogStatusInProgress),
	}
	item, err := storage.CreateBacklogItem(ctx, itemData)
	require.NoError(t, err)

	sessionUUID := uuid.New().String()
	_, err = storage.CreateItemSession(ctx, session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: sessionUUID,
		SessionRole: session.SessionRoleWork,
	})
	require.NoError(t, err)

	handler := &backlogHandlers{storage: storage}
	ctxWithUUID := WithSessionUUID(ctx, sessionUUID)

	req := makeToolReq(map[string]interface{}{
		"item_id": item.ID,
		"message": "Implemented the feature, all criteria done.",
	})

	result, err := handler.requestReview(ctxWithUUID, req)
	require.NoError(t, err)
	require.Len(t, result.Content, 1)
	tc, ok := result.Content[0].(mcpgo.TextContent)
	require.True(t, ok)
	require.Contains(t, tc.Text, "review")

	// Verify item is now in review status.
	fetched, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	require.Equal(t, string(session.BacklogStatusReview), fetched.Status)
}

// TestRequestReview_SucceedsAgain_AfterAutoReopenFollowingZombieReviewerFailure
// is the regression test for backlog item 4c71d3a3-1dd5-4d82-86ec-694a98835d2f:
// request_review used to fail permanently and deterministically, forever, once
// an item reached "review" and its reviewer session died before
// handleReviewSessionExited ever processed its FAIL verdict (a zombie
// reviewer — no clean exit event; live repro on backlog item 40a243b0 via
// list_workspace_peers showing status:Active, lifecycle:gone).
//
// The crash-recovery sweep (session/backlog_lifecycle.go's
// reconcileUnprocessedReviewVerdicts) correctly detects this shape and calls
// into BacklogService.AutoReopenAfterFailedReview — simulated directly here,
// the same way the sweep does after it confirms the reviewer is dead. But
// AutoReopenAfterFailedReview's hasActiveWorkSession guard used to skip the
// review->in_progress transition ENTIRELY whenever the original work session
// was still alive (exactly this case: that live work session is the one
// about to call request_review again after fixing the FAIL findings) —
// leaving the item permanently wedged in "review", since request_review's
// own precondition hardcodes ExpectedStatus: in_progress. Left unfixed, the
// second requestReview call below fails with "concurrent modification
// detected: expected status \"in_progress\", got \"review\"" every time.
func TestRequestReview_SucceedsAgain_AfterAutoReopenFollowingZombieReviewerFailure(t *testing.T) {
	storage := newTestBacklogStorage(t)
	ctx := context.Background()

	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "Escalation reasoning",
		Status: string(session.BacklogStatusReview),
	})
	require.NoError(t, err)

	// The work session that originally called request_review and is still
	// alive — it stays running, fixes the FAIL findings, and is about to call
	// request_review again. This is exactly AutoReopenAfterFailedReview's
	// hasActiveWorkSession case.
	workUUID := uuid.New().String()
	_, err = storage.CreateItemSession(ctx, session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: workUUID,
		SessionRole: session.SessionRoleWork,
	})
	require.NoError(t, err)

	// The reviewer session that recorded a FAIL verdict and then died as a
	// zombie. By the time AutoReopenAfterFailedReview runs (dispatched by the
	// crash-recovery sweep once it independently confirms the reviewer
	// process is gone), the verdict is already on record — that confirmation
	// step is exercised separately by reconcileUnprocessedReviewVerdicts'
	// own tests, not re-tested here.
	_, err = storage.CreateItemSessionWithVerdict(ctx, session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: uuid.New().String(),
		SessionRole: session.SessionRoleReview,
	}, session.ReviewVerdictData{
		OverallOutcome: session.ReviewVerdictFail,
		Summary:        "found gaps in criterion 2",
	})
	require.NoError(t, err)

	// Simulates the crash-recovery sweep handing the confirmed-zombie item to
	// AutoReopenAfterFailedReview.
	svc := services.NewBacklogService(storage, nil, nil, nil, nil, nil)
	require.NoError(t, svc.AutoReopenAfterFailedReview(ctx, item.ID))

	fetched, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	require.Equal(t, string(session.BacklogStatusInProgress), fetched.Status,
		"AutoReopenAfterFailedReview must transition back to in_progress even when the live work session blocks a respawn — otherwise request_review can never succeed again")

	// The live work session's request_review call must now succeed.
	handler := &backlogHandlers{storage: storage}
	ctxWithUUID := WithSessionUUID(ctx, workUUID)
	req := makeToolReq(map[string]interface{}{
		"item_id": item.ID,
		"message": "Fixed the noted gaps in criterion 2.",
	})

	result, err := handler.requestReview(ctxWithUUID, req)
	require.NoError(t, err)
	require.Len(t, result.Content, 1)
	tc, ok := result.Content[0].(mcpgo.TextContent)
	require.True(t, ok)
	require.Contains(t, tc.Text, "review", "request_review must succeed and move the item back to review, not fail the concurrent-modification precondition")

	fetched, err = storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	require.Equal(t, string(session.BacklogStatusReview), fetched.Status)
}

// TestRequestReview_TransitionsDirectlyToDone_When_SkipReviewGateEnabled verifies
// that an item with SkipReviewGate=true never enters "review" via request_review —
// it must go straight from in_progress to done, matching every other
// SkipReviewGate-aware code path (session/backlog_lifecycle.go's onSessionExited,
// TriggerReviewForSession, ReviewGateRunner.Run). Before this fix, request_review
// always transitioned to review regardless of the flag, and because
// TriggerReviewForSession also honors the flag (no-op), the item would then sit in
// review forever with no gate ever spawned to move it forward.
func TestRequestReview_TransitionsDirectlyToDone_When_SkipReviewGateEnabled(t *testing.T) {
	storage := newTestBacklogStorage(t)
	ctx := context.Background()

	itemData := session.BacklogItemData{
		Title:          "Skip the gate",
		Status:         string(session.BacklogStatusInProgress),
		SkipReviewGate: true,
	}
	item, err := storage.CreateBacklogItem(ctx, itemData)
	require.NoError(t, err)

	sessionUUID := uuid.New().String()
	_, err = storage.CreateItemSession(ctx, session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: sessionUUID,
		SessionRole: session.SessionRoleWork,
	})
	require.NoError(t, err)

	handler := &backlogHandlers{storage: storage}
	ctxWithUUID := WithSessionUUID(ctx, sessionUUID)

	req := makeToolReq(map[string]interface{}{
		"item_id": item.ID,
		"message": "Implemented the feature, all criteria done.",
	})

	result, err := handler.requestReview(ctxWithUUID, req)
	require.NoError(t, err)
	require.Len(t, result.Content, 1)
	tc, ok := result.Content[0].(mcpgo.TextContent)
	require.True(t, ok)
	require.Contains(t, tc.Text, "SkipReviewGate")

	// Verify item went straight to done, never review.
	fetched, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	require.Equal(t, string(session.BacklogStatusDone), fetched.Status)
}

// TestRequestReview_PersistsVerificationNotesOnWorkSession verifies that a
// non-empty verification_notes argument is stored on the caller's ItemSession
// so the review gate can later surface it in the reviewer's prompt.
func TestRequestReview_PersistsVerificationNotesOnWorkSession(t *testing.T) {
	storage := newTestBacklogStorage(t)
	ctx := context.Background()

	itemData := session.BacklogItemData{
		Title:  "Review me",
		Status: string(session.BacklogStatusInProgress),
	}
	item, err := storage.CreateBacklogItem(ctx, itemData)
	require.NoError(t, err)

	sessionUUID := uuid.New().String()
	workIS, err := storage.CreateItemSession(ctx, session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: sessionUUID,
		SessionRole: session.SessionRoleWork,
	})
	require.NoError(t, err)

	handler := &backlogHandlers{storage: storage}
	ctxWithUUID := WithSessionUUID(ctx, sessionUUID)

	notes := "ran `go test ./session/...` -> ok (41 tests); ran make install-service, confirmed session groups under Category=Backlog"
	req := makeToolReq(map[string]interface{}{
		"item_id":            item.ID,
		"message":            "Implemented the feature, all criteria done.",
		"verification_notes": notes,
	})

	result, err := handler.requestReview(ctxWithUUID, req)
	require.NoError(t, err)
	require.Len(t, result.Content, 1)

	fetched, err := storage.GetItemSession(ctx, workIS.ID)
	require.NoError(t, err)
	assert.Equal(t, notes, fetched.VerificationNotes)
}

// TestRequestReview_AppendsToExistingVerificationNotes_RatherThanOverwriting
// verifies requestReview's notes-persistence now mirrors reportDuplicate's
// own append-not-overwrite fix (see the notesMarker/UpdateItemSessionVerificationNotes
// comment in reportDuplicate, tools_backlog.go): UpdateItemSessionVerificationNotes
// is a plain overwrite, not an append, so a request_review call that follows
// an earlier report_duplicate call (or an earlier request_review call, e.g.
// before a rework cycle) on the same ItemSession must not silently erase that
// prior evidence. Added as a new test rather than extending
// TestRequestReview_PersistsVerificationNotesOnWorkSession per AC9/FR9: that
// test's existing single-call, empty-starting-state assertion
// (assert.Equal(t, notes, fetched.VerificationNotes)) must keep holding
// unmodified.
func TestRequestReview_AppendsToExistingVerificationNotes_RatherThanOverwriting(t *testing.T) {
	storage := newTestBacklogStorage(t)
	ctx := context.Background()

	itemData := session.BacklogItemData{
		Title:  "Review me again",
		Status: string(session.BacklogStatusInProgress),
	}
	item, err := storage.CreateBacklogItem(ctx, itemData)
	require.NoError(t, err)

	sessionUUID := uuid.New().String()
	workIS, err := storage.CreateItemSession(ctx, session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: sessionUUID,
		SessionRole: session.SessionRoleWork,
	})
	require.NoError(t, err)

	// Seed prior evidence on the ItemSession, as if left by an earlier
	// report_duplicate call before a rework cycle.
	priorNotes := "duplicate_ref=https://github.com/tstapler/stapler-squad/pull/272 reason=turned out not to be a duplicate after all"
	require.NoError(t, storage.UpdateItemSessionVerificationNotes(ctx, workIS.ID, priorNotes))

	handler := &backlogHandlers{storage: storage}
	ctxWithUUID := WithSessionUUID(ctx, sessionUUID)

	newNotes := "ran `go test ./session/...` -> ok (41 tests)"
	req := makeToolReq(map[string]interface{}{
		"item_id":            item.ID,
		"message":            "Implemented the feature after all, all criteria done.",
		"verification_notes": newNotes,
	})

	result, err := handler.requestReview(ctxWithUUID, req)
	require.NoError(t, err)
	require.Len(t, result.Content, 1)

	fetched, err := storage.GetItemSession(ctx, workIS.ID)
	require.NoError(t, err)
	assert.Contains(t, fetched.VerificationNotes, priorNotes, "prior VerificationNotes must be preserved, not overwritten")
	assert.Contains(t, fetched.VerificationNotes, newNotes, "the new verification_notes must also be present")
}

// TestRequestReview_RejectsVerificationNotesOver4000Chars verifies the length
// guard on verification_notes mirrors the existing guard on message.
func TestRequestReview_RejectsVerificationNotesOver4000Chars(t *testing.T) {
	storage := newTestBacklogStorage(t)
	ctx := context.Background()

	itemData := session.BacklogItemData{
		Title:  "Review me",
		Status: string(session.BacklogStatusInProgress),
	}
	item, err := storage.CreateBacklogItem(ctx, itemData)
	require.NoError(t, err)

	sessionUUID := uuid.New().String()
	_, err = storage.CreateItemSession(ctx, session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: sessionUUID,
		SessionRole: session.SessionRoleWork,
	})
	require.NoError(t, err)

	handler := &backlogHandlers{storage: storage}
	ctxWithUUID := WithSessionUUID(ctx, sessionUUID)

	req := makeToolReq(map[string]interface{}{
		"item_id":            item.ID,
		"message":            "Done.",
		"verification_notes": strings.Repeat("a", 4001),
	})

	result, err := handler.requestReview(ctxWithUUID, req)
	require.NoError(t, err)

	m := parseResult(t, result)
	require.False(t, m["success"].(bool))
	errObj, ok := m["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, ErrInvalidArgument, errObj["code"].(string))

	// Item must remain in_progress — the transition should not have happened.
	fetched, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(session.BacklogStatusInProgress), fetched.Status)
}

// TestRequestReview_RejectsWhenSessionNotLinked verifies PERMISSION_DENIED
// when session is not linked to the item.
func TestRequestReview_RejectsWhenSessionNotLinked(t *testing.T) {
	storage := newTestBacklogStorage(t)
	ctx := context.Background()

	itemData := session.BacklogItemData{
		Title:  "Unlinked item",
		Status: string(session.BacklogStatusInProgress),
	}
	item, err := storage.CreateBacklogItem(ctx, itemData)
	require.NoError(t, err)

	sessionUUID := uuid.New().String()
	ctxWithUUID := WithSessionUUID(ctx, sessionUUID)
	handler := &backlogHandlers{storage: storage}

	req := makeToolReq(map[string]interface{}{
		"item_id": item.ID,
		"message": "Done.",
	})

	result, err := handler.requestReview(ctxWithUUID, req)
	require.NoError(t, err)

	m := parseResult(t, result)
	require.False(t, m["success"].(bool))
	errObj := m["error"].(map[string]interface{})
	require.Equal(t, ErrPermissionDenied, errObj["code"])
}

// TestFormatDirtyPathsRejectionMessage_ListsEachPath verifies the message names the
// specific dirty paths instead of a blanket "git add -A" instruction.
func TestFormatDirtyPathsRejectionMessage_ListsEachPath(t *testing.T) {
	msg := formatDirtyPathsRejectionMessage([]string{"server/handler.go", "scratch.txt"})
	assert.Contains(t, msg, "server/handler.go")
	assert.Contains(t, msg, "scratch.txt")
	assert.NotContains(t, msg, "git add -A")
}

// TestFormatDirtyPathsRejectionMessage_CapsAtMaxPaths verifies a large path list is
// capped with an "...and N more" suffix rather than listed in full.
func TestFormatDirtyPathsRejectionMessage_CapsAtMaxPaths(t *testing.T) {
	paths := make([]string, 15)
	for i := range paths {
		paths[i] = fmt.Sprintf("file-%d.txt", i)
	}

	msg := formatDirtyPathsRejectionMessage(paths)

	for i := 0; i < maxRejectionMessagePaths; i++ {
		assert.Contains(t, msg, paths[i])
	}
	for i := maxRejectionMessagePaths; i < len(paths); i++ {
		assert.NotContains(t, msg, paths[i])
	}
	assert.Contains(t, msg, "...and 5 more")
}

// TestFormatDirtyPathsRejectionMessage_EmptyPaths_UsesGenericWording verifies the
// fallback wording when no specific paths are available (e.g. GetWorktreeDirtyPaths
// itself errored).
func TestFormatDirtyPathsRejectionMessage_EmptyPaths_UsesGenericWording(t *testing.T) {
	msg := formatDirtyPathsRejectionMessage(nil)
	assert.Contains(t, msg, "uncommitted changes")
	assert.NotContains(t, msg, "git add -A")
}

// --- request_review: Phase 2 CAS generalization (Epic 4.1) ---

// TestRequestReview_TransitionsPRPendingItemToReview verifies FR1's happy
// path: a request_review call sourced from pr_pending (not just the
// pre-existing in_progress path) succeeds, and the resulting
// BacklogStatusEvent is attributed TriggeredBy "agent" (ADR-003), not
// "system".
func TestRequestReview_TransitionsPRPendingItemToReview(t *testing.T) {
	storage := newTestBacklogStorage(t)
	ctx := context.Background()

	itemData := session.BacklogItemData{
		Title:  "Re-request after PR feedback",
		Status: string(session.BacklogStatusPRPending),
	}
	item, err := storage.CreateBacklogItem(ctx, itemData)
	require.NoError(t, err)

	sessionUUID := uuid.New().String()
	_, err = storage.CreateItemSession(ctx, session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: sessionUUID,
		SessionRole: session.SessionRoleWork,
	})
	require.NoError(t, err)

	handler := &backlogHandlers{storage: storage}
	ctxWithUUID := WithSessionUUID(ctx, sessionUUID)

	req := makeToolReq(map[string]interface{}{
		"item_id": item.ID,
		"message": "Re-requesting review after addressing PR feedback",
	})

	result, err := handler.requestReview(ctxWithUUID, req)
	require.NoError(t, err)
	require.Len(t, result.Content, 1)
	tc, ok := result.Content[0].(mcpgo.TextContent)
	require.True(t, ok)
	require.Contains(t, tc.Text, "review")

	fetched, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	require.Equal(t, string(session.BacklogStatusReview), fetched.Status)

	require.NotEmpty(t, fetched.StatusEvents)
	last := fetched.StatusEvents[len(fetched.StatusEvents)-1]
	require.Equal(t, session.TriggeredByAgent, last.TriggeredBy)
}

// TestRequestReview_RejectsWhenSourceStatusNotAllowed verifies the
// validateSelfResolveSource whitelist (pitfalls.md §0): request_review must
// refuse any source status outside {in_progress, pr_pending}, before any
// mutation.
func TestRequestReview_RejectsWhenSourceStatusNotAllowed(t *testing.T) {
	statuses := []string{
		string(session.BacklogStatusDone),
		string(session.BacklogStatusIdea),
		string(session.BacklogStatusReview),
		string(session.BacklogStatusArchived),
	}

	for _, status := range statuses {
		status := status
		t.Run(status, func(t *testing.T) {
			storage := newTestBacklogStorage(t)
			ctx := context.Background()

			itemData := session.BacklogItemData{
				Title:  "Disallowed source status",
				Status: status,
			}
			item, err := storage.CreateBacklogItem(ctx, itemData)
			require.NoError(t, err)

			sessionUUID := uuid.New().String()
			_, err = storage.CreateItemSession(ctx, session.ItemSessionData{
				ItemID:      item.ID,
				SessionUUID: sessionUUID,
				SessionRole: session.SessionRoleWork,
			})
			require.NoError(t, err)

			handler := &backlogHandlers{storage: storage}
			ctxWithUUID := WithSessionUUID(ctx, sessionUUID)

			req := makeToolReq(map[string]interface{}{
				"item_id": item.ID,
				"message": "Attempting review from a disallowed status.",
			})

			result, err := handler.requestReview(ctxWithUUID, req)
			require.NoError(t, err)

			m := parseResult(t, result)
			require.False(t, m["success"].(bool))
			errObj := m["error"].(map[string]interface{})
			require.Equal(t, ErrInvalidArgument, errObj["code"])
			require.Contains(t, errObj["message"], status)

			fetched, err := storage.GetBacklogItem(ctx, item.ID)
			require.NoError(t, err)
			require.Equal(t, status, fetched.Status)
		})
	}
}

// TestRequestReview_RejectsWhenActiveReviewSessionExists_AndSourceIsPRPending
// verifies FR2: a pr_pending-sourced request_review call is refused while an
// active (unended) review-role session already exists for the item, with
// zero mutation.
func TestRequestReview_RejectsWhenActiveReviewSessionExists_AndSourceIsPRPending(t *testing.T) {
	storage := newTestBacklogStorage(t)
	ctx := context.Background()

	itemData := session.BacklogItemData{
		Title:  "Zombie reviewer, pr_pending",
		Status: string(session.BacklogStatusPRPending),
	}
	item, err := storage.CreateBacklogItem(ctx, itemData)
	require.NoError(t, err)

	workUUID := uuid.New().String()
	_, err = storage.CreateItemSession(ctx, session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: workUUID,
		SessionRole: session.SessionRoleWork,
	})
	require.NoError(t, err)

	reviewUUID := uuid.New().String()
	_, err = storage.CreateItemSession(ctx, session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: reviewUUID,
		SessionRole: session.SessionRoleReview,
	})
	require.NoError(t, err)

	handler := &backlogHandlers{storage: storage}
	ctxWithUUID := WithSessionUUID(ctx, workUUID)

	req := makeToolReq(map[string]interface{}{
		"item_id": item.ID,
		"message": "Re-requesting review while one is already active.",
	})

	result, err := handler.requestReview(ctxWithUUID, req)
	require.NoError(t, err)

	m := parseResult(t, result)
	require.False(t, m["success"].(bool))
	errObj := m["error"].(map[string]interface{})
	require.Equal(t, ErrInvalidArgument, errObj["code"])
	require.Contains(t, errObj["message"], "active review session")

	fetched, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	require.Equal(t, string(session.BacklogStatusPRPending), fetched.Status)
}

// TestRequestReview_AllowsActiveReviewSession_WhenSourceIsInProgress verifies
// FR2's guard is scoped to the pr_pending source path only: the same
// active-review-session setup must not affect the pre-existing in_progress
// path's behavior.
func TestRequestReview_AllowsActiveReviewSession_WhenSourceIsInProgress(t *testing.T) {
	storage := newTestBacklogStorage(t)
	ctx := context.Background()

	itemData := session.BacklogItemData{
		Title:  "Zombie reviewer, in_progress",
		Status: string(session.BacklogStatusInProgress),
	}
	item, err := storage.CreateBacklogItem(ctx, itemData)
	require.NoError(t, err)

	workUUID := uuid.New().String()
	_, err = storage.CreateItemSession(ctx, session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: workUUID,
		SessionRole: session.SessionRoleWork,
	})
	require.NoError(t, err)

	reviewUUID := uuid.New().String()
	_, err = storage.CreateItemSession(ctx, session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: reviewUUID,
		SessionRole: session.SessionRoleReview,
	})
	require.NoError(t, err)

	handler := &backlogHandlers{storage: storage}
	ctxWithUUID := WithSessionUUID(ctx, workUUID)

	req := makeToolReq(map[string]interface{}{
		"item_id": item.ID,
		"message": "Requesting review; guard must not apply here.",
	})

	result, err := handler.requestReview(ctxWithUUID, req)
	require.NoError(t, err)
	require.Len(t, result.Content, 1)
	tc, ok := result.Content[0].(mcpgo.TextContent)
	require.True(t, ok)
	require.Contains(t, tc.Text, "review")

	fetched, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	require.Equal(t, string(session.BacklogStatusReview), fetched.Status)
}

// TestRequestReview_FailsClosed_WhenListItemSessionsErrors verifies Task
// 2.2.1b's fail-closed correction: a ListItemSessions storage error on the
// pr_pending path must refuse the call (INTERNAL_ERROR), never silently fall
// through as though no reviewer were active.
func TestRequestReview_FailsClosed_WhenListItemSessionsErrors(t *testing.T) {
	storage := newTestBacklogStorage(t)
	ctx := context.Background()

	itemData := session.BacklogItemData{
		Title:  "Storage flaky on active-reviewer check",
		Status: string(session.BacklogStatusPRPending),
	}
	item, err := storage.CreateBacklogItem(ctx, itemData)
	require.NoError(t, err)

	sessionUUID := uuid.New().String()
	_, err = storage.CreateItemSession(ctx, session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: sessionUUID,
		SessionRole: session.SessionRoleWork,
	})
	require.NoError(t, err)

	injectedErr := errors.New("boom: storage unavailable")
	handler := &backlogHandlers{
		storage: storage,
		listItemSessionsFn: func(ctx context.Context, itemID string) ([]session.ItemSessionSummary, error) {
			return nil, injectedErr
		},
	}
	ctxWithUUID := WithSessionUUID(ctx, sessionUUID)

	req := makeToolReq(map[string]interface{}{
		"item_id": item.ID,
		"message": "Re-requesting review during a storage blip.",
	})

	result, err := handler.requestReview(ctxWithUUID, req)
	require.NoError(t, err)

	m := parseResult(t, result)
	require.False(t, m["success"].(bool))
	errObj := m["error"].(map[string]interface{})
	require.Equal(t, ErrInternalError, errObj["code"])

	fetched, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	require.Equal(t, string(session.BacklogStatusPRPending), fetched.Status)
}

// callToolRaceResult pairs a CallToolResult with its error for use with
// channels fed from spawned goroutines racing an MCP tool call. require/assert
// calls must never run inside the goroutine itself — t.FailNow() (which
// require calls on failure) only unwinds the goroutine that's running, not
// the main test goroutine — so both the result AND error are collected here
// and all assertions happen in the main test goroutine after collection.
type callToolRaceResult struct {
	result *mcpgo.CallToolResult
	err    error
}

// requestReviewRaceResult is a request_review-specific alias for readability
// at call sites in TestRequestReview_ReportsDistinctMessage_WhenCASPreconditionFails.
type requestReviewRaceResult = callToolRaceResult

// TestRequestReview_ReportsDistinctMessage_WhenCASPreconditionFails verifies
// pitfalls.md §1/§5c: when two request_review calls genuinely race on the
// same in_progress item, the DB-level atomic UPDATE...WHERE guarantees
// exactly one winner; the loser must see a distinct, non-retry message (not
// the generic "transition to %s failed" text), still under ErrInternalError.
func TestRequestReview_ReportsDistinctMessage_WhenCASPreconditionFails(t *testing.T) {
	storage := newTestBacklogStorage(t)
	ctx := context.Background()

	itemData := session.BacklogItemData{
		Title:  "Racing review requests",
		Status: string(session.BacklogStatusInProgress),
	}
	item, err := storage.CreateBacklogItem(ctx, itemData)
	require.NoError(t, err)

	sessionUUID := uuid.New().String()
	_, err = storage.CreateItemSession(ctx, session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: sessionUUID,
		SessionRole: session.SessionRoleWork,
	})
	require.NoError(t, err)

	// readBarrier forces both goroutines' pre-transition GetBacklogItem reads
	// to complete — both observing Status: "in_progress" — before either is
	// allowed to proceed to its TransitionBacklogItemStatus write. Without
	// this, startBarrier alone only synchronizes goroutine *start*: on a
	// CI runner with more cores/different scheduling, one goroutine can run
	// its full read->whitelist-check->write sequence before the other's
	// first read even executes, so the "loser" reads the post-write
	// Status: "review" and fails the whitelist check (ErrInvalidArgument)
	// instead of racing the CAS write (ErrPreconditionFailed / ErrInternalError)
	// — the actual behavior this test exists to exercise.
	var readBarrier sync.WaitGroup
	readBarrier.Add(2)
	getBacklogItemFn := func(fnCtx context.Context, itemID string) (*session.BacklogItemData, error) {
		item, err := storage.GetBacklogItem(fnCtx, itemID)
		readBarrier.Done()
		readBarrier.Wait()
		return item, err
	}

	handler := &backlogHandlers{storage: storage, getBacklogItemFn: getBacklogItemFn}
	ctxWithUUID := WithSessionUUID(ctx, sessionUUID)

	var wg sync.WaitGroup
	var startBarrier sync.WaitGroup
	startBarrier.Add(1)
	results := make(chan requestReviewRaceResult, 2)

	race := func() {
		defer wg.Done()
		startBarrier.Wait()
		req := makeToolReq(map[string]interface{}{
			"item_id": item.ID,
			"message": "Racing request_review call.",
		})
		result, callErr := handler.requestReview(ctxWithUUID, req)
		// Do not require/assert here — this closure runs in a spawned
		// goroutine, and t.FailNow() (which require calls internally) only
		// unwinds the goroutine that calls it, not the test goroutine. Send
		// both the result and error through the channel and let the main
		// test goroutine make all require/assert calls after collecting.
		results <- requestReviewRaceResult{result: result, err: callErr}
	}

	wg.Add(2)
	go race()
	go race()
	startBarrier.Done()
	wg.Wait()
	close(results)

	var successes, failures int
	var failureMsg string
	for rr := range results {
		require.NoError(t, rr.err)
		result := rr.result
		require.Len(t, result.Content, 1)
		tc, ok := result.Content[0].(mcpgo.TextContent)
		require.True(t, ok)

		// The success path returns plain (non-JSON) text; errResult always
		// emits a JSON-encoded MCPResult. Distinguish on that, since both
		// possible outcomes here arrive as mcpgo.TextContent.
		var parsed map[string]interface{}
		if jsonErr := json.Unmarshal([]byte(tc.Text), &parsed); jsonErr != nil {
			successes++
			continue
		}
		failures++
		errObj := parsed["error"].(map[string]interface{})
		require.Equal(t, ErrInternalError, errObj["code"])
		failureMsg, _ = errObj["message"].(string)
	}

	require.Equal(t, 1, successes, "exactly one racer should win")
	require.Equal(t, 1, failures, "the loser must get a distinct message, not silently succeed")
	require.Contains(t, failureMsg, "state changed")

	fetched, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	require.Equal(t, string(session.BacklogStatusReview), fetched.Status)
}

// TestSubmitTriageResult_NoNotificationWhenEventBusNil verifies that submitTriageResult
// does not panic when eventBus is nil and still returns a success result.
func TestSubmitTriageResult_NoNotificationWhenEventBusNil(t *testing.T) {
	storage := newTestBacklogStorage(t)

	itemID, sessUUID := setupTriageSession(t, storage)

	handler := &backlogHandlers{storage: storage, eventBus: nil}
	ctxWithUUID := WithSessionUUID(context.Background(), sessUUID)

	req := makeToolReq(map[string]interface{}{
		"item_id": itemID,
		"summary": "All looks good, no suggestions needed",
	})

	require.NotPanics(t, func() {
		result, err := handler.submitTriageResult(ctxWithUUID, req)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.Len(t, result.Content, 1)
		tc, ok := result.Content[0].(mcpgo.TextContent)
		require.True(t, ok)
		assert.Contains(t, tc.Text, "Triage result submitted")
	})
}

// --- report_pr_created ---

// setupReportPRCreatedFixture creates a review-status item with a linked
// work session, returning the item and the session UUID. verifyResult/
// verifyErr control the injected verifyPRMatchesBranch stub's return value.
func setupReportPRCreatedFixture(t *testing.T, storage *session.Storage, itemStatus session.BacklogStatus) (*session.BacklogItemData, string) {
	t.Helper()
	ctx := context.Background()

	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "Ship it",
		Status: string(itemStatus),
	})
	require.NoError(t, err)

	sessionUUID := uuid.New().String()
	_, err = storage.CreateItemSession(ctx, session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: sessionUUID,
		SessionRole: session.SessionRoleWork,
	})
	require.NoError(t, err)

	return item, sessionUUID
}

// TestReportPRCreated_should_TransitionToPRPending_When_ValidPR verifies the
// happy path: a work session reports a PR that verifies against GitHub, and
// the item transitions review -> pr_pending with pr_url/pr_number persisted.
func TestReportPRCreated_should_TransitionToPRPending_When_ValidPR(t *testing.T) {
	storage := newTestBacklogStorage(t)
	item, sessionUUID := setupReportPRCreatedFixture(t, storage, session.BacklogStatusReview)

	handler := &backlogHandlers{
		storage:              storage,
		resolveSessionBranch: func(context.Context, string) (string, error) { return "backlog/ship-it", nil },
		verifyPRMatchesBranch: func(context.Context, string, string, int, string) (PRVerification, error) {
			return NewPRVerification(true, true, "backlog/ship-it", githubpkg.PRStateOpen, "tstapler"), nil
		},
	}
	ctxWithUUID := WithSessionUUID(context.Background(), sessionUUID)

	req := makeToolReq(map[string]interface{}{
		"item_id":   item.ID,
		"pr_url":    "https://github.com/tstapler/stapler-squad/pull/42",
		"pr_number": float64(42),
		"summary":   "Implemented the feature and shipped it via /backlog:ship.",
	})

	result, err := handler.reportPRCreated(ctxWithUUID, req)
	require.NoError(t, err)
	require.Len(t, result.Content, 1)
	tc, ok := result.Content[0].(mcpgo.TextContent)
	require.True(t, ok)
	assert.Contains(t, tc.Text, "pr_pending")

	fetched, err := storage.GetBacklogItem(context.Background(), item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(session.BacklogStatusPRPending), fetched.Status)
	assert.Equal(t, 42, fetched.PrNumber)
	assert.Equal(t, "https://github.com/tstapler/stapler-squad/pull/42", fetched.PrURL)
}

// TestReportPRCreated_should_ReturnError_When_PersistFails mirrors BUG-040's
// own TestPushAndCreatePR_PRFieldsPersistFails_StaysInReview_AndNotifies test
// shape: force the underlying storage write to fail (by closing the real
// on-disk SQLite connection right before the call, the same technique
// BUG-040's regression test uses), and assert the tool call itself surfaces
// an error rather than silently reporting success.
func TestReportPRCreated_should_ReturnError_When_PersistFails(t *testing.T) {
	repo := session.NewTestEntRepository(t)
	storage, err := session.NewStorageWithRepository(repo)
	require.NoError(t, err)

	item, sessionUUID := setupReportPRCreatedFixture(t, storage, session.BacklogStatusReview)

	// Close the DB connection now that fixture setup succeeded — every
	// subsequent storage call inside reportPRCreated (including the primary
	// PR-field/transition write) will fail.
	require.NoError(t, repo.Close())

	handler := &backlogHandlers{
		storage:              storage,
		resolveSessionBranch: func(context.Context, string) (string, error) { return "backlog/ship-it", nil },
		verifyPRMatchesBranch: func(context.Context, string, string, int, string) (PRVerification, error) {
			return NewPRVerification(true, true, "backlog/ship-it", githubpkg.PRStateOpen, "tstapler"), nil
		},
	}
	ctxWithUUID := WithSessionUUID(context.Background(), sessionUUID)

	req := makeToolReq(map[string]interface{}{
		"item_id":   item.ID,
		"pr_url":    "https://github.com/tstapler/stapler-squad/pull/42",
		"pr_number": float64(42),
		"summary":   "Implemented the feature.",
	})

	result, err := handler.reportPRCreated(ctxWithUUID, req)
	require.NoError(t, err)

	m := parseResult(t, result)
	require.False(t, m["success"].(bool), "report_pr_created must not silently succeed when the storage write fails")
}

// TestReportPRCreated_should_RejectCall_When_ItemStatusIneligible (AC6): an
// item whose status is not "review" or "pr_pending" must be rejected with a
// message naming its actual status, before any storage write or PR
// verification is attempted — distinct from AC7's genuine-CAS-race message
// (TestReportPRCreated_should_ReturnFriendlyError_When_CASFailsOutOfBand),
// which only fires for a race that happens after this status check passes.
func TestReportPRCreated_should_RejectCall_When_ItemStatusIneligible(t *testing.T) {
	for _, status := range []session.BacklogStatus{
		session.BacklogStatusIdea,
		session.BacklogStatusReady,
		session.BacklogStatusDone,
	} {
		t.Run(string(status), func(t *testing.T) {
			storage := newTestBacklogStorage(t)
			item, sessionUUID := setupReportPRCreatedFixture(t, storage, status)

			verifyCalled := false
			handler := &backlogHandlers{
				storage:              storage,
				resolveSessionBranch: func(context.Context, string) (string, error) { return "backlog/ship-it", nil },
				verifyPRMatchesBranch: func(context.Context, string, string, int, string) (PRVerification, error) {
					verifyCalled = true
					return NewPRVerification(true, true, "backlog/ship-it", githubpkg.PRStateOpen, "tstapler"), nil
				},
			}
			ctxWithUUID := WithSessionUUID(context.Background(), sessionUUID)

			req := makeToolReq(map[string]interface{}{
				"item_id":   item.ID,
				"pr_url":    "https://github.com/tstapler/stapler-squad/pull/42",
				"pr_number": float64(42),
				"summary":   "Implemented the feature.",
			})

			result, err := handler.reportPRCreated(ctxWithUUID, req)
			require.NoError(t, err)

			m := parseResult(t, result)
			require.False(t, m["success"].(bool))
			errObj, ok := m["error"].(map[string]interface{})
			require.True(t, ok)
			assert.Equal(t, ErrInvalidArgument, errObj["code"].(string))
			msg := errObj["message"].(string)
			assert.Contains(t, msg, "is at status")
			assert.Contains(t, msg, string(status), "the message must name the item's actual status")
			assert.NotContains(t, msg, "item state changed since your last read",
				"a structurally ineligible status must not surface the genuine-CAS-race message")
			assert.False(t, verifyCalled, "the status guard must short-circuit before any GitHub verification call")

			fetched, err := storage.GetBacklogItem(context.Background(), item.ID)
			require.NoError(t, err)
			assert.Equal(t, string(status), fetched.Status, "a rejected call must not change the item's status")
		})
	}
}

// TestReportPRCreated_should_NoOp_When_AlreadyPRPendingSamePR verifies the
// idempotency contract: calling report_pr_created again for a PR already
// recorded on a pr_pending item is a no-op success, not an error.
func TestReportPRCreated_should_NoOp_When_AlreadyPRPendingSamePR(t *testing.T) {
	storage := newTestBacklogStorage(t)
	ctx := context.Background()

	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "Already shipped",
		Status: string(session.BacklogStatusPRPending),
	})
	require.NoError(t, err)
	prURL := "https://github.com/tstapler/stapler-squad/pull/42"
	prNum := 42
	_, err = storage.UpdateBacklogItem(ctx, item.ID, session.BacklogItemUpdate{PrURL: &prURL, PrNumber: &prNum}, nil)
	require.NoError(t, err)

	sessionUUID := uuid.New().String()
	_, err = storage.CreateItemSession(ctx, session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: sessionUUID,
		SessionRole: session.SessionRoleWork,
	})
	require.NoError(t, err)

	verifyCalled := false
	handler := &backlogHandlers{
		storage:              storage,
		resolveSessionBranch: func(context.Context, string) (string, error) { return "backlog/ship-it", nil },
		verifyPRMatchesBranch: func(context.Context, string, string, int, string) (PRVerification, error) {
			verifyCalled = true
			return NewPRVerification(true, true, "backlog/ship-it", githubpkg.PRStateOpen, "tstapler"), nil
		},
	}
	ctxWithUUID := WithSessionUUID(ctx, sessionUUID)

	req := makeToolReq(map[string]interface{}{
		"item_id":   item.ID,
		"pr_url":    prURL,
		"pr_number": float64(prNum),
		"summary":   "Implemented the feature.",
	})

	result, err := handler.reportPRCreated(ctxWithUUID, req)
	require.NoError(t, err)
	require.Len(t, result.Content, 1)
	tc, ok := result.Content[0].(mcpgo.TextContent)
	require.True(t, ok)
	assert.Contains(t, tc.Text, "already recorded")
	assert.False(t, verifyCalled, "idempotent no-op must short-circuit before any GitHub verification call")
}

// TestReportPRCreated_should_RejectCall_When_CallerRoleNotWork verifies the
// role guard: a review-role session may not call report_pr_created.
func TestReportPRCreated_should_RejectCall_When_CallerRoleNotWork(t *testing.T) {
	storage := newTestBacklogStorage(t)
	ctx := context.Background()

	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "Review-role caller",
		Status: string(session.BacklogStatusReview),
	})
	require.NoError(t, err)

	sessionUUID := uuid.New().String()
	_, err = storage.CreateItemSession(ctx, session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: sessionUUID,
		SessionRole: session.SessionRoleReview,
	})
	require.NoError(t, err)

	handler := &backlogHandlers{storage: storage}
	ctxWithUUID := WithSessionUUID(ctx, sessionUUID)

	req := makeToolReq(map[string]interface{}{
		"item_id":   item.ID,
		"pr_url":    "https://github.com/tstapler/stapler-squad/pull/42",
		"pr_number": float64(42),
		"summary":   "Implemented the feature.",
	})

	result, err := handler.reportPRCreated(ctxWithUUID, req)
	require.NoError(t, err)

	m := parseResult(t, result)
	require.False(t, m["success"].(bool))
	errObj, ok := m["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, ErrPermissionDenied, errObj["code"].(string))
}

// TestReportPRCreated_should_RejectCall_When_BranchMismatch is the
// GitHub-verification regression test: a self-reported PR whose head branch
// (per GitHub) does not match this session's own branch must be rejected,
// not persisted.
func TestReportPRCreated_should_RejectCall_When_BranchMismatch(t *testing.T) {
	storage := newTestBacklogStorage(t)
	item, sessionUUID := setupReportPRCreatedFixture(t, storage, session.BacklogStatusReview)

	handler := &backlogHandlers{
		storage:              storage,
		resolveSessionBranch: func(context.Context, string) (string, error) { return "backlog/ship-it", nil },
		verifyPRMatchesBranch: func(_ context.Context, _, _ string, _ int, expectedBranch string) (PRVerification, error) {
			assert.Equal(t, "backlog/ship-it", expectedBranch)
			// definitive mismatch — a real PR exists, but for a different branch/number
			return NewPRVerification(true, false, "totally-unrelated-branch", githubpkg.PRStateOpen, "tstapler"), nil
		},
	}
	ctxWithUUID := WithSessionUUID(context.Background(), sessionUUID)

	req := makeToolReq(map[string]interface{}{
		"item_id":   item.ID,
		"pr_url":    "https://github.com/tstapler/stapler-squad/pull/999",
		"pr_number": float64(999),
		"summary":   "Implemented the feature.",
	})

	result, err := handler.reportPRCreated(ctxWithUUID, req)
	require.NoError(t, err)

	m := parseResult(t, result)
	require.False(t, m["success"].(bool))
	errObj, ok := m["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, ErrInvalidArgument, errObj["code"].(string))

	// Item must remain untouched — the mismatch must not be persisted.
	fetched, err := storage.GetBacklogItem(context.Background(), item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(session.BacklogStatusReview), fetched.Status)
	assert.Equal(t, 0, fetched.PrNumber)
}

// TestReportPRCreated_should_ReturnRetryableError_When_GitHubLookupTransientlyFails
// verifies that a transient GitHub lookup failure (rate limit, network) is
// surfaced as a retryable INTERNAL_ERROR, distinct from a definitive
// mismatch (INVALID_ARGUMENT) — the agent should retry, not correct itself.
func TestReportPRCreated_should_ReturnRetryableError_When_GitHubLookupTransientlyFails(t *testing.T) {
	storage := newTestBacklogStorage(t)
	item, sessionUUID := setupReportPRCreatedFixture(t, storage, session.BacklogStatusReview)

	handler := &backlogHandlers{
		storage:              storage,
		resolveSessionBranch: func(context.Context, string) (string, error) { return "backlog/ship-it", nil },
		verifyPRMatchesBranch: func(context.Context, string, string, int, string) (PRVerification, error) {
			return PRVerification{}, fmt.Errorf("GitHub API: rate limited (403)")
		},
	}
	ctxWithUUID := WithSessionUUID(context.Background(), sessionUUID)

	req := makeToolReq(map[string]interface{}{
		"item_id":   item.ID,
		"pr_url":    "https://github.com/tstapler/stapler-squad/pull/42",
		"pr_number": float64(42),
		"summary":   "Implemented the feature.",
	})

	result, err := handler.reportPRCreated(ctxWithUUID, req)
	require.NoError(t, err)

	m := parseResult(t, result)
	require.False(t, m["success"].(bool))
	errObj, ok := m["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, ErrInternalError, errObj["code"].(string), "a transient lookup failure must be retryable (INTERNAL_ERROR), not a definitive mismatch (INVALID_ARGUMENT)")

	// Item must remain untouched.
	fetched, err := storage.GetBacklogItem(context.Background(), item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(session.BacklogStatusReview), fetched.Status)
	assert.Equal(t, 0, fetched.PrNumber)
}

// TestReportPRCreated_LoserPRNeverPersists_WhenCASPreconditionFails is the
// regression test for the lost-update bug in SetBacklogItemPRAndTransition
// (session/storage.go): that function used to write PrURL/PrNumber
// unconditionally (precondition: nil) and only afterward apply its CAS
// precondition to the status transition. Two racing report_pr_created calls
// with DIFFERENT PR numbers could both pass that unconditional field write;
// only the loser's transition failed afterward — by which point its PR
// number had already clobbered the winner's persisted value. The fix
// persists the status transition and PrURL/PrNumber together in a single
// atomic UPDATE ... WHERE (TransitionBacklogItemStatusWithPRFields,
// session/ent_repository_backlog.go), so this test's assertion — whichever
// call's transition actually wins the CAS is also the call whose PR number
// is durably persisted, the loser's PR number must never land, even
// transiently — holds by construction, not just by timing luck.
//
// Mirrors TestReportDuplicate_ReportsDistinctMessage_WhenCASPreconditionFails's
// real-concurrency structure, plus TestRequestReview_ReportsDistinctMessage_
// WhenCASPreconditionFails's readBarrier (via the getBacklogItemFor seam
// reportPRCreated is now wired through). The readBarrier's job is only to
// line up both racers' pre-write reads so they actually attempt to race
// instead of one running start-to-finish before the other's first read even
// executes — it does not itself decide a winner; that's the storage layer's
// atomic UPDATE ... WHERE (the same SQL-level CAS TransitionBacklogItemStatus
// already uses elsewhere).
func TestReportPRCreated_LoserPRNeverPersists_WhenCASPreconditionFails(t *testing.T) {
	storage := newTestBacklogStorage(t)
	ctx := context.Background()

	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "Racing PR reports",
		Status: string(session.BacklogStatusReview),
	})
	require.NoError(t, err)

	sessionUUID := uuid.New().String()
	_, err = storage.CreateItemSession(ctx, session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: sessionUUID,
		SessionRole: session.SessionRoleWork,
	})
	require.NoError(t, err)

	// readBarrier forces both goroutines' pre-write GetBacklogItem reads
	// (routed through the getBacklogItemFor seam) to complete before either
	// is allowed to proceed into SetBacklogItemPRAndTransition's write —
	// this only ensures both racers actually race (see
	// TestRequestReview_ReportsDistinctMessage_WhenCASPreconditionFails's
	// doc comment for why startBarrier alone is not sufficient); which one
	// wins is decided by the storage layer's atomic UPDATE ... WHERE, not by
	// this barrier.
	var readBarrier sync.WaitGroup
	readBarrier.Add(2)
	getBacklogItemFn := func(fnCtx context.Context, itemID string) (*session.BacklogItemData, error) {
		it, getErr := storage.GetBacklogItem(fnCtx, itemID)
		readBarrier.Done()
		readBarrier.Wait()
		return it, getErr
	}

	handler := &backlogHandlers{
		storage:              storage,
		getBacklogItemFn:     getBacklogItemFn,
		resolveSessionBranch: func(context.Context, string) (string, error) { return "backlog/ship-it", nil },
		verifyPRMatchesBranch: func(context.Context, string, string, int, string) (PRVerification, error) {
			return NewPRVerification(true, true, "backlog/ship-it", githubpkg.PRStateOpen, "tstapler"), nil
		},
	}
	ctxWithUUID := WithSessionUUID(ctx, sessionUUID)

	type prRaceResult struct {
		prNumber int
		result   *mcpgo.CallToolResult
		err      error
	}

	var wg sync.WaitGroup
	var startBarrier sync.WaitGroup
	startBarrier.Add(1)
	results := make(chan prRaceResult, 2)

	race := func(prNumber int) {
		defer wg.Done()
		startBarrier.Wait()
		req := makeToolReq(map[string]interface{}{
			"item_id":   item.ID,
			"pr_url":    fmt.Sprintf("https://github.com/tstapler/stapler-squad/pull/%d", prNumber),
			"pr_number": float64(prNumber),
			"summary":   fmt.Sprintf("Racing report_pr_created call for PR #%d.", prNumber),
		})
		result, callErr := handler.reportPRCreated(ctxWithUUID, req)
		// Do not require/assert here — this closure runs in a spawned
		// goroutine; see callToolRaceResult's doc comment.
		results <- prRaceResult{prNumber: prNumber, result: result, err: callErr}
	}

	wg.Add(2)
	go race(100)
	go race(200)
	startBarrier.Done()
	wg.Wait()
	close(results)

	var successes, failures int
	var winnerPRNumber int
	for rr := range results {
		require.NoError(t, rr.err)
		require.Len(t, rr.result.Content, 1)
		tc, ok := rr.result.Content[0].(mcpgo.TextContent)
		require.True(t, ok)

		// The success path returns plain (non-JSON) text; errResult always
		// emits a JSON-encoded MCPResult.
		var parsed map[string]interface{}
		if jsonErr := json.Unmarshal([]byte(tc.Text), &parsed); jsonErr != nil {
			successes++
			winnerPRNumber = rr.prNumber
			continue
		}
		failures++
	}

	require.Equal(t, 1, successes, "exactly one racer should win the CAS transition")
	require.Equal(t, 1, failures, "the loser must see an error, not silently succeed")

	fetched, err := storage.GetBacklogItem(context.Background(), item.ID)
	require.NoError(t, err)
	require.Equal(t, string(session.BacklogStatusPRPending), fetched.Status)
	assert.Equal(t, winnerPRNumber, fetched.PrNumber,
		"the persisted PrNumber must match whichever racer's transition actually won the CAS — the loser's PR number must never land, even transiently")
	assert.Contains(t, fetched.PrURL, fmt.Sprintf("/pull/%d", winnerPRNumber))
}

// TestCreateBacklogItem_should_PersistItem_When_TitleProvided verifies the
// happy path: a plain title-only call creates an idea-status item with
// default priority, discoverable afterward via GetBacklogItem.
func TestCreateBacklogItem_should_PersistItem_When_TitleProvided(t *testing.T) {
	storage := newTestBacklogStorage(t)
	handler := &backlogHandlers{storage: storage}
	ctx := WithSessionUUID(context.Background(), uuid.New().String())

	req := makeToolReq(map[string]interface{}{
		"title":               "Fix the thing",
		"description":         "It's broken because of X.",
		"acceptance_criteria": []interface{}{"Given X, when Y, then Z"},
		"priority":            float64(1),
		"category":            "bugfix",
	})

	result, err := handler.createBacklogItem(ctx, req)
	require.NoError(t, err)
	require.Len(t, result.Content, 1)
	tc, ok := result.Content[0].(mcpgo.TextContent)
	require.True(t, ok)
	assert.Contains(t, tc.Text, "Fix the thing")

	var itemID string
	_, scanErr := fmt.Sscanf(tc.Text, "Created backlog item %s", &itemID)
	require.NoError(t, scanErr)
	itemID = strings.TrimSuffix(itemID, ":")

	fetched, err := storage.GetBacklogItem(context.Background(), itemID)
	require.NoError(t, err)
	assert.Equal(t, "Fix the thing", fetched.Title)
	assert.Equal(t, string(session.BacklogStatusIdea), fetched.Status)
	assert.Equal(t, 1, fetched.Priority)
	assert.Equal(t, "bugfix", fetched.Category)
	ac, parseErr := session.ParseAcCriteria(fetched.AcceptanceCriteria)
	require.NoError(t, parseErr)
	require.Len(t, ac, 1)
	assert.Equal(t, "Given X, when Y, then Z", ac[0].Text)
	assert.Equal(t, session.AcStatusPending, ac[0].Status)
}

// TestCreateBacklogItem_should_ReturnError_When_TitleMissing verifies the
// tool refuses an empty title rather than persisting an untitled item.
func TestCreateBacklogItem_should_ReturnError_When_TitleMissing(t *testing.T) {
	storage := newTestBacklogStorage(t)
	handler := &backlogHandlers{storage: storage}
	ctx := WithSessionUUID(context.Background(), uuid.New().String())

	result, err := handler.createBacklogItem(ctx, makeToolReq(map[string]interface{}{
		"description": "No title here.",
	}))
	require.NoError(t, err)

	m := parseResult(t, result)
	require.False(t, m["success"].(bool), "create_backlog_item must reject a missing title")
}

// TestCreateBacklogItem_should_ReturnError_When_PriorityOutOfRange verifies
// the 1-5 priority bound (matching the web UI's BacklogItemForm P1-P5 select)
// is enforced here too, not just at the proto/UI layer.
func TestCreateBacklogItem_should_ReturnError_When_PriorityOutOfRange(t *testing.T) {
	storage := newTestBacklogStorage(t)
	handler := &backlogHandlers{storage: storage}
	ctx := WithSessionUUID(context.Background(), uuid.New().String())

	result, err := handler.createBacklogItem(ctx, makeToolReq(map[string]interface{}{
		"title":    "Bad priority",
		"priority": float64(9),
	}))
	require.NoError(t, err)

	m := parseResult(t, result)
	require.False(t, m["success"].(bool), "create_backlog_item must reject an out-of-range priority")
}

// TestCreateBacklogItem_should_Succeed_When_NoSessionUUID is the regression
// test for the bug where a manually-registered MCP client (e.g. `claude mcp
// add` from a plain terminal with no STAPLER_SESSION_UUID set) got a hard
// PERMISSION_DENIED from create_backlog_item even though the tool's write
// path (storage.CreateBacklogItem) takes no session parameter and creates no
// session/item link — the session UUID was only ever used for the audit-trail
// log line, so there was no real invariant to protect by rejecting the call.
func TestCreateBacklogItem_should_Succeed_When_NoSessionUUID(t *testing.T) {
	storage := newTestBacklogStorage(t)
	handler := &backlogHandlers{storage: storage}
	ctx := context.Background() // No session UUID injected — simulates a manual MCP client.

	result, err := handler.createBacklogItem(ctx, makeToolReq(map[string]interface{}{
		"title": "Filed from a manual MCP client",
	}))
	require.NoError(t, err)
	require.Len(t, result.Content, 1)
	tc, ok := result.Content[0].(mcpgo.TextContent)
	require.True(t, ok)
	assert.Contains(t, tc.Text, "Filed from a manual MCP client")
}

// TestImportGitHubIssue_should_PersistItem_When_IssueFetchSucceeds verifies
// the happy path against a stubbed GitHub API — title/body land on the new
// item and Notes records the source issue URL, mirroring
// BacklogService.ImportGitHubIssue's own behavior (backlog_service_sync.go).
func TestImportGitHubIssue_should_PersistItem_When_IssueFetchSucceeds(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "test-token")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"number": 316, "title": "bug: something is broken", "body": "Steps to reproduce...",
			"html_url": "https://github.com/tstapler/stapler-squad/issues/316",
			"user": {"login": "tstapler"}, "state": "open"
		}`))
	}))
	defer srv.Close()
	prevBaseURL := githubpkg.GhBaseURL
	githubpkg.GhBaseURL = srv.URL + "/"
	defer func() { githubpkg.GhBaseURL = prevBaseURL }()

	storage := newTestBacklogStorage(t)
	handler := &backlogHandlers{storage: storage}
	ctx := WithSessionUUID(context.Background(), uuid.New().String())

	result, err := handler.importGitHubIssue(ctx, makeToolReq(map[string]interface{}{
		"issue_url": "https://github.com/tstapler/stapler-squad/issues/316",
	}))
	require.NoError(t, err)
	require.Len(t, result.Content, 1)
	tc, ok := result.Content[0].(mcpgo.TextContent)
	require.True(t, ok)
	assert.Contains(t, tc.Text, "bug: something is broken")

	var itemID string
	_, scanErr := fmt.Sscanf(tc.Text, "Imported backlog item %s", &itemID)
	require.NoError(t, scanErr)
	itemID = strings.TrimSuffix(itemID, ":")

	fetched, err := storage.GetBacklogItem(context.Background(), itemID)
	require.NoError(t, err)
	assert.Equal(t, "bug: something is broken", fetched.Title)
	assert.Equal(t, "Steps to reproduce...", fetched.Description)
	assert.Contains(t, fetched.Notes, "https://github.com/tstapler/stapler-squad/issues/316")
}

// TestImportGitHubIssue_should_Succeed_When_NoSessionUUID mirrors
// TestCreateBacklogItem_should_Succeed_When_NoSessionUUID's regression
// coverage for import_github_issue: it has the same "session UUID used only
// for the audit log" shape, so a manual MCP client must not be rejected.
func TestImportGitHubIssue_should_Succeed_When_NoSessionUUID(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "test-token")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"number": 318, "title": "filed via manual mcp client", "body": "...",
			"html_url": "https://github.com/tstapler/stapler-squad/issues/318",
			"user": {"login": "tstapler"}, "state": "open"
		}`))
	}))
	defer srv.Close()
	prevBaseURL := githubpkg.GhBaseURL
	githubpkg.GhBaseURL = srv.URL + "/"
	defer func() { githubpkg.GhBaseURL = prevBaseURL }()

	storage := newTestBacklogStorage(t)
	handler := &backlogHandlers{storage: storage}
	ctx := context.Background() // No session UUID injected — simulates a manual MCP client.

	result, err := handler.importGitHubIssue(ctx, makeToolReq(map[string]interface{}{
		"issue_url": "https://github.com/tstapler/stapler-squad/issues/318",
	}))
	require.NoError(t, err)
	require.Len(t, result.Content, 1)
	tc, ok := result.Content[0].(mcpgo.TextContent)
	require.True(t, ok)
	assert.Contains(t, tc.Text, "filed via manual mcp client")
}

// TestImportGitHubIssue_should_TriggerTriage_When_BacklogSvcWiredAndRepoPathSet
// is import_github_issue's half of the BUG-061 regression coverage — see
// TestCreateBacklogItem_should_TriggerTriage_When_BacklogSvcWiredAndRepoPathSet's
// doc comment for the full root-cause explanation. ImportGitHubIssue (RPC)
// already triggered triage before this fix; only the MCP tool bypassed it.
func TestImportGitHubIssue_should_TriggerTriage_When_BacklogSvcWiredAndRepoPathSet(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "test-token")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"number": 317, "title": "needs triage via import", "body": "...",
			"html_url": "https://github.com/tstapler/stapler-squad/issues/317",
			"user": {"login": "tstapler"}, "state": "open"
		}`))
	}))
	defer srv.Close()
	prevBaseURL := githubpkg.GhBaseURL
	githubpkg.GhBaseURL = srv.URL + "/"
	defer func() { githubpkg.GhBaseURL = prevBaseURL }()

	storage := newTestBacklogStorage(t)
	backlogSvc := services.NewBacklogService(storage, nil, nil, nil, nil, nil)
	backlogSvc.SetHeadlessPool(&fakeTriageHeadlessPool{})
	t.Cleanup(backlogSvc.Shutdown)

	handler := &backlogHandlers{storage: storage, backlogSvc: backlogSvc}
	ctx := WithSessionUUID(context.Background(), uuid.New().String())

	result, err := handler.importGitHubIssue(ctx, makeToolReq(map[string]interface{}{
		"issue_url": "https://github.com/tstapler/stapler-squad/issues/317",
		"repo_path": t.TempDir(),
	}))
	require.NoError(t, err)
	tc, ok := result.Content[0].(mcpgo.TextContent)
	require.True(t, ok)
	assert.Contains(t, tc.Text, "Auto-triage started.")

	var itemID string
	_, scanErr := fmt.Sscanf(tc.Text, "Imported backlog item %s", &itemID)
	require.NoError(t, scanErr)
	itemID = strings.TrimSuffix(itemID, ":")

	sessions, err := storage.ListItemSessions(context.Background(), itemID)
	require.NoError(t, err)
	require.Len(t, sessions, 1, "expected exactly one ItemSession created by TriggerTriage")
	assert.Equal(t, session.SessionRoleTriage, sessions[0].Role)
}

// TestImportGitHubIssue_should_ReturnError_When_URLNotAnIssue verifies a
// non-issue GitHub URL (e.g. a PR) is rejected before any network call.
func TestImportGitHubIssue_should_ReturnError_When_URLNotAnIssue(t *testing.T) {
	storage := newTestBacklogStorage(t)
	handler := &backlogHandlers{storage: storage}
	ctx := WithSessionUUID(context.Background(), uuid.New().String())

	result, err := handler.importGitHubIssue(ctx, makeToolReq(map[string]interface{}{
		"issue_url": "https://github.com/tstapler/stapler-squad/pull/320",
	}))
	require.NoError(t, err)

	m := parseResult(t, result)
	require.False(t, m["success"].(bool), "import_github_issue must reject a non-issue URL")
}

// --- decideOverridePolicy (Task 4.0) ---

// TestDecideOverridePolicy is a table-driven unit test directly against
// decideOverridePolicy — no mock storage, no item/session fixtures. This is
// the authoritative place the override-path decision logic is pinned; the
// TestReportPRCreated_* tests below only confirm reportPRCreated wires this
// function's outcome into the right handler behavior.
func TestDecideOverridePolicy(t *testing.T) {
	tests := []struct {
		name           string
		v              PRVerification
		overrideReason string
		callerLogin    string
		forceOverride  bool
		wantAccept     bool
		wantCode       connect.Code
	}{
		{
			name:           "not-exists rejects regardless of override_reason (empty)",
			v:              NewPRVerification(false, false, "", "", ""),
			overrideReason: "",
			callerLogin:    "tstapler",
			wantAccept:     false,
			wantCode:       connect.CodeNotFound,
		},
		{
			name:           "not-exists rejects regardless of override_reason (non-empty)",
			v:              NewPRVerification(false, false, "", "", ""),
			overrideReason: "here's why",
			callerLogin:    "tstapler",
			wantAccept:     false,
			wantCode:       connect.CodeNotFound,
		},
		{
			name:           "matched accepts regardless of override_reason/callerLogin (fast path)",
			v:              NewPRVerification(true, true, "backlog/x", githubpkg.PRStateOpen, "tstapler"),
			overrideReason: "",
			callerLogin:    "",
			wantAccept:     true,
		},
		{
			name:           "mismatch, empty override_reason rejects even when author would match",
			v:              NewPRVerification(true, false, "feature/y", githubpkg.PRStateOpen, "tstapler"),
			overrideReason: "",
			callerLogin:    "tstapler",
			wantAccept:     false,
			wantCode:       connect.CodeInvalidArgument,
		},
		{
			name:           "mismatch, reason given, open state, author mismatch rejects",
			v:              NewPRVerification(true, false, "feature/y", githubpkg.PRStateOpen, "someone-else"),
			overrideReason: "here's why",
			callerLogin:    "tstapler",
			wantAccept:     false,
			wantCode:       connect.CodePermissionDenied,
		},
		{
			name:           "mismatch, reason given, closed state AND author mismatch rejects on author, not state",
			v:              NewPRVerification(true, false, "feature/y", githubpkg.PRStateClosed, "someone-else"),
			overrideReason: "here's why",
			callerLogin:    "tstapler",
			wantAccept:     false,
			wantCode:       connect.CodePermissionDenied,
		},
		{
			name:           "mismatch, reason given, author matches, closed state rejects on state",
			v:              NewPRVerification(true, false, "feature/y", githubpkg.PRStateClosed, "tstapler"),
			overrideReason: "here's why",
			callerLogin:    "tstapler",
			wantAccept:     false,
			wantCode:       connect.CodeFailedPrecondition,
		},
		{
			name:           "mismatch, reason given, author matches, open state accepts",
			v:              NewPRVerification(true, false, "feature/y", githubpkg.PRStateOpen, "tstapler"),
			overrideReason: "here's why",
			callerLogin:    "tstapler",
			wantAccept:     true,
		},
		{
			name:           "mismatch, reason given, author matches, merged state accepts (confirmed AC1 repro shape)",
			v:              NewPRVerification(true, false, "feature/y", githubpkg.PRStateMerged, "tstapler"),
			overrideReason: "here's why",
			callerLogin:    "tstapler",
			wantAccept:     true,
		},
		{
			name:           "forceOverride: matched branch still rejects without override_reason (reassignment AC1)",
			v:              NewPRVerification(true, true, "backlog/x", githubpkg.PRStateOpen, "tstapler"),
			overrideReason: "",
			callerLogin:    "tstapler",
			forceOverride:  true,
			wantAccept:     false,
			wantCode:       connect.CodeInvalidArgument,
		},
		{
			name:           "forceOverride: matched branch still rejects on author mismatch (reassignment AC9)",
			v:              NewPRVerification(true, true, "backlog/x", githubpkg.PRStateOpen, "someone-else"),
			overrideReason: "correcting a bad PR",
			callerLogin:    "tstapler",
			forceOverride:  true,
			wantAccept:     false,
			wantCode:       connect.CodePermissionDenied,
		},
		{
			name:           "forceOverride: matched branch, reason given, author matches, open state accepts",
			v:              NewPRVerification(true, true, "backlog/x", githubpkg.PRStateOpen, "tstapler"),
			overrideReason: "correcting a bad PR",
			callerLogin:    "tstapler",
			forceOverride:  true,
			wantAccept:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			accept, code, _ := decideOverridePolicy(tt.v, tt.overrideReason, tt.callerLogin, tt.forceOverride)
			assert.Equal(t, tt.wantAccept, accept)
			if !tt.wantAccept {
				assert.Equal(t, tt.wantCode, code)
			}
		})
	}
}

// --- report_pr_created fallback/override path (Story 3/4) ---

// TestReportPRCreated_should_TransitionToPRPending_When_FallbackBranchWithOverrideReason
// is the confirmed real repro shape (AC1/AC2): a work session's PR was opened
// from a clean fallback branch because the tracked branch was polluted by
// another session sharing the worktree. With a matching self-authored PR and
// a non-empty override_reason, the item must still transition to pr_pending,
// and the override use must be audited via a structured log.Warn line.
func TestReportPRCreated_should_TransitionToPRPending_When_FallbackBranchWithOverrideReason(t *testing.T) {
	storage := newTestBacklogStorage(t)
	item, sessionUUID := setupReportPRCreatedFixture(t, storage, session.BacklogStatusReview)

	var buf strings.Builder
	origLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(origLogger) })

	handler := &backlogHandlers{
		storage: storage,
		resolveSessionBranch: func(context.Context, string) (string, error) {
			return "backlog/stapler-squad-ci-status-diff-viewer", nil
		},
		verifyPRMatchesBranch: func(context.Context, string, string, int, string) (PRVerification, error) {
			return NewPRVerification(true, false, "feature/ci-status-diff-viewer", githubpkg.PRStateMerged, "tstapler"), nil
		},
		resolveCallerGitHubLogin: func(context.Context) (string, error) { return "tstapler", nil },
	}
	ctxWithUUID := WithSessionUUID(context.Background(), sessionUUID)

	req := makeToolReq(map[string]interface{}{
		"item_id":         item.ID,
		"pr_url":          "https://github.com/tstapler/stapler-squad/pull/326",
		"pr_number":       float64(326),
		"summary":         "Shipped the fix from a clean fallback branch.",
		"override_reason": "tracked branch had unrelated commits from a shared worktree; opened PR from a clean branch instead",
	})

	result, err := handler.reportPRCreated(ctxWithUUID, req)
	require.NoError(t, err)
	require.Len(t, result.Content, 1)
	tc, ok := result.Content[0].(mcpgo.TextContent)
	require.True(t, ok)
	assert.Contains(t, tc.Text, "pr_pending")

	fetched, err := storage.GetBacklogItem(context.Background(), item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(session.BacklogStatusPRPending), fetched.Status)
	assert.Equal(t, 326, fetched.PrNumber)

	logOutput := buf.String()
	assert.Contains(t, logOutput, "recording PR via override")
	assert.Contains(t, logOutput, sessionUUID)
	assert.Contains(t, logOutput, item.ID)
	assert.Contains(t, logOutput, "pr_number=326")
	assert.Contains(t, logOutput, "actual_head_branch=feature/ci-status-diff-viewer")
	assert.Contains(t, logOutput, "tracked_branch=backlog/stapler-squad-ci-status-diff-viewer")
	assert.Contains(t, logOutput, "pr_author=tstapler")
	assert.Contains(t, logOutput, "override_reason=")
}

// TestReportPRCreated_should_RejectCall_When_FallbackBranchMissingOverrideReason
// confirms reportPRCreated actually wires decideOverridePolicy's reject into
// an untouched item (not a decision-logic re-check — that's TestDecideOverridePolicy's
// job) and that resolveCallerGitHubLogin is never called when override_reason
// is missing — a call already doomed to reject for a missing reason must not
// pay for a GitHub identity lookup it doesn't need.
func TestReportPRCreated_should_RejectCall_When_FallbackBranchMissingOverrideReason(t *testing.T) {
	storage := newTestBacklogStorage(t)
	item, sessionUUID := setupReportPRCreatedFixture(t, storage, session.BacklogStatusReview)

	loginCalled := false
	handler := &backlogHandlers{
		storage: storage,
		resolveSessionBranch: func(context.Context, string) (string, error) {
			return "backlog/stapler-squad-ci-status-diff-viewer", nil
		},
		verifyPRMatchesBranch: func(context.Context, string, string, int, string) (PRVerification, error) {
			return NewPRVerification(true, false, "feature/ci-status-diff-viewer", githubpkg.PRStateMerged, "tstapler"), nil
		},
		resolveCallerGitHubLogin: func(context.Context) (string, error) {
			loginCalled = true
			return "tstapler", nil
		},
	}
	ctxWithUUID := WithSessionUUID(context.Background(), sessionUUID)

	req := makeToolReq(map[string]interface{}{
		"item_id":   item.ID,
		"pr_url":    "https://github.com/tstapler/stapler-squad/pull/326",
		"pr_number": float64(326),
		"summary":   "Shipped the fix from a clean fallback branch.",
	})

	result, err := handler.reportPRCreated(ctxWithUUID, req)
	require.NoError(t, err)

	m := parseResult(t, result)
	require.False(t, m["success"].(bool))
	errObj, ok := m["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, ErrInvalidArgument, errObj["code"].(string))
	assert.False(t, loginCalled, "resolveCallerGitHubLogin must not be called when override_reason is missing")

	fetched, err := storage.GetBacklogItem(context.Background(), item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(session.BacklogStatusReview), fetched.Status)
	assert.Equal(t, 0, fetched.PrNumber)
}

// TestReportPRCreated_should_DocumentOverrideWorkaround_When_BranchMismatchRejected
// (AC3): the rejection message when override_reason is missing must document
// the workaround concretely — naming the actual head branch, the tracked
// branch, the override_reason argument, and the authorship requirement — not
// just say "it failed."
func TestReportPRCreated_should_DocumentOverrideWorkaround_When_BranchMismatchRejected(t *testing.T) {
	storage := newTestBacklogStorage(t)
	item, sessionUUID := setupReportPRCreatedFixture(t, storage, session.BacklogStatusReview)

	handler := &backlogHandlers{
		storage: storage,
		resolveSessionBranch: func(context.Context, string) (string, error) {
			return "backlog/stapler-squad-ci-status-diff-viewer", nil
		},
		verifyPRMatchesBranch: func(context.Context, string, string, int, string) (PRVerification, error) {
			return NewPRVerification(true, false, "feature/ci-status-diff-viewer", githubpkg.PRStateMerged, "tstapler"), nil
		},
	}
	ctxWithUUID := WithSessionUUID(context.Background(), sessionUUID)

	req := makeToolReq(map[string]interface{}{
		"item_id":   item.ID,
		"pr_url":    "https://github.com/tstapler/stapler-squad/pull/326",
		"pr_number": float64(326),
		"summary":   "Shipped the fix from a clean fallback branch.",
	})

	result, err := handler.reportPRCreated(ctxWithUUID, req)
	require.NoError(t, err)

	m := parseResult(t, result)
	require.False(t, m["success"].(bool))
	errObj, ok := m["error"].(map[string]interface{})
	require.True(t, ok)
	msg := errObj["message"].(string)
	assert.Contains(t, msg, "override_reason")
	assert.Contains(t, msg, "feature/ci-status-diff-viewer")
	assert.Contains(t, msg, "backlog/stapler-squad-ci-status-diff-viewer")
	assert.Contains(t, msg, "authored by your own GitHub identity")
}

// TestReportPRCreated_should_RejectCall_When_UnrelatedClosedPRWithOverrideReason
// confirms wiring only (decision logic is pinned by TestDecideOverridePolicy):
// a real, closed, self-authored PR is still rejected by the state gate even
// with a matching author and an override_reason.
func TestReportPRCreated_should_RejectCall_When_UnrelatedClosedPRWithOverrideReason(t *testing.T) {
	storage := newTestBacklogStorage(t)
	item, sessionUUID := setupReportPRCreatedFixture(t, storage, session.BacklogStatusReview)

	handler := &backlogHandlers{
		storage: storage,
		resolveSessionBranch: func(context.Context, string) (string, error) {
			return "backlog/stapler-squad-ci-status-diff-viewer", nil
		},
		verifyPRMatchesBranch: func(context.Context, string, string, int, string) (PRVerification, error) {
			return NewPRVerification(true, false, "totally-unrelated-branch", githubpkg.PRStateClosed, "tstapler"), nil
		},
		resolveCallerGitHubLogin: func(context.Context) (string, error) { return "tstapler", nil },
	}
	ctxWithUUID := WithSessionUUID(context.Background(), sessionUUID)

	req := makeToolReq(map[string]interface{}{
		"item_id":         item.ID,
		"pr_url":          "https://github.com/tstapler/stapler-squad/pull/999",
		"pr_number":       float64(999),
		"summary":         "Unrelated PR.",
		"override_reason": "trying anyway",
	})

	result, err := handler.reportPRCreated(ctxWithUUID, req)
	require.NoError(t, err)

	m := parseResult(t, result)
	require.False(t, m["success"].(bool))
	errObj, ok := m["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, ErrInvalidArgument, errObj["code"].(string))

	fetched, err := storage.GetBacklogItem(context.Background(), item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(session.BacklogStatusReview), fetched.Status)
	assert.Equal(t, 0, fetched.PrNumber)
}

// TestReportPRCreated_should_RejectCall_When_UnrelatedPRAuthorMismatch
// confirms wiring only (decision logic is pinned by TestDecideOverridePolicy):
// a real, open, correct-repo PR — everything the pre-repair design required
// for acceptance — but authored by someone else must still be rejected, and
// the message must name both the PR's actual author and the caller's own
// resolved identity.
func TestReportPRCreated_should_RejectCall_When_UnrelatedPRAuthorMismatch(t *testing.T) {
	storage := newTestBacklogStorage(t)
	item, sessionUUID := setupReportPRCreatedFixture(t, storage, session.BacklogStatusReview)

	handler := &backlogHandlers{
		storage: storage,
		resolveSessionBranch: func(context.Context, string) (string, error) {
			return "backlog/stapler-squad-ci-status-diff-viewer", nil
		},
		verifyPRMatchesBranch: func(context.Context, string, string, int, string) (PRVerification, error) {
			return NewPRVerification(true, false, "totally-unrelated-branch", githubpkg.PRStateOpen, "a-different-github-user"), nil
		},
		resolveCallerGitHubLogin: func(context.Context) (string, error) { return "tstapler", nil },
	}
	ctxWithUUID := WithSessionUUID(context.Background(), sessionUUID)

	req := makeToolReq(map[string]interface{}{
		"item_id":         item.ID,
		"pr_url":          "https://github.com/tstapler/stapler-squad/pull/999",
		"pr_number":       float64(999),
		"summary":         "Unrelated PR.",
		"override_reason": "trying anyway",
	})

	result, err := handler.reportPRCreated(ctxWithUUID, req)
	require.NoError(t, err)

	m := parseResult(t, result)
	require.False(t, m["success"].(bool))
	errObj, ok := m["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, ErrInvalidArgument, errObj["code"].(string))
	msg := errObj["message"].(string)
	assert.Contains(t, msg, "a-different-github-user")
	assert.Contains(t, msg, "tstapler")

	fetched, err := storage.GetBacklogItem(context.Background(), item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(session.BacklogStatusReview), fetched.Status)
	assert.Equal(t, 0, fetched.PrNumber)
}

// TestReportPRCreated_should_RejectCall_When_PRNumberDoesNotExist confirms
// wiring only (decision logic pinned by TestDecideOverridePolicy's not-exists
// case): existence can never be overridden, and resolveCallerGitHubLogin must
// never be called since decideOverridePolicy's guard requires Exists, which
// is false here.
func TestReportPRCreated_should_RejectCall_When_PRNumberDoesNotExist(t *testing.T) {
	storage := newTestBacklogStorage(t)
	item, sessionUUID := setupReportPRCreatedFixture(t, storage, session.BacklogStatusReview)

	loginCalled := false
	handler := &backlogHandlers{
		storage: storage,
		resolveSessionBranch: func(context.Context, string) (string, error) {
			return "backlog/stapler-squad-ci-status-diff-viewer", nil
		},
		verifyPRMatchesBranch: func(context.Context, string, string, int, string) (PRVerification, error) {
			return NewPRVerification(false, false, "", "", ""), nil
		},
		resolveCallerGitHubLogin: func(context.Context) (string, error) {
			loginCalled = true
			return "tstapler", nil
		},
	}
	ctxWithUUID := WithSessionUUID(context.Background(), sessionUUID)

	req := makeToolReq(map[string]interface{}{
		"item_id":         item.ID,
		"pr_url":          "https://github.com/tstapler/stapler-squad/pull/99999",
		"pr_number":       float64(99999),
		"summary":         "Nonexistent PR.",
		"override_reason": "trying anyway",
	})

	result, err := handler.reportPRCreated(ctxWithUUID, req)
	require.NoError(t, err)

	m := parseResult(t, result)
	require.False(t, m["success"].(bool))
	errObj, ok := m["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, ErrInvalidArgument, errObj["code"].(string))
	assert.Contains(t, errObj["message"].(string), "does not exist")
	assert.False(t, loginCalled, "resolveCallerGitHubLogin must not be called when the PR does not exist")

	fetched, err := storage.GetBacklogItem(context.Background(), item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(session.BacklogStatusReview), fetched.Status)
	assert.Equal(t, 0, fetched.PrNumber)
}

// --- report_pr_created: reassignment from pr_pending ---

// setupReportPRCreatedReassignmentFixture creates a pr_pending-status item
// with a linked work session and an already-tracked PR (trackedPRNumber),
// mirroring setupReportPRCreatedFixture but for the reassignment path (item
// already pr_pending rather than review). Optionally seeds
// PrFeedbackAddressedAt so AC7 (clearing it on reassignment) can be asserted.
func setupReportPRCreatedReassignmentFixture(t *testing.T, storage *session.Storage, trackedPRNumber int, seedFeedbackAddressedAt bool) (*session.BacklogItemData, string) {
	t.Helper()
	ctx := context.Background()

	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "Ship it",
		Status: string(session.BacklogStatusPRPending),
	})
	require.NoError(t, err)

	sessionUUID := uuid.New().String()
	_, err = storage.CreateItemSession(ctx, session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: sessionUUID,
		SessionRole: session.SessionRoleWork,
	})
	require.NoError(t, err)

	prURL := fmt.Sprintf("https://github.com/tstapler/stapler-squad/pull/%d", trackedPRNumber)
	update := session.BacklogItemUpdate{PrURL: &prURL, PrNumber: &trackedPRNumber}
	if seedFeedbackAddressedAt {
		seeded := time.Now().Add(-time.Hour)
		update.PrFeedbackAddressedAt = &seeded
	}
	updated, err := storage.UpdateBacklogItem(ctx, item.ID, update, nil)
	require.NoError(t, err)

	return updated, sessionUUID
}

// TestReportPRCreated_should_ReassignPR_When_AlreadyPRPendingWithOverrideReason
// (AC0): a work session may correct an already-tracked PR to a different one
// when the item is pr_pending, given a valid override_reason and a new PR
// that verifies on GitHub as open and self-authored.
func TestReportPRCreated_should_ReassignPR_When_AlreadyPRPendingWithOverrideReason(t *testing.T) {
	storage := newTestBacklogStorage(t)
	item, sessionUUID := setupReportPRCreatedReassignmentFixture(t, storage, 100, false)

	handler := &backlogHandlers{
		storage:              storage,
		resolveSessionBranch: func(context.Context, string) (string, error) { return "backlog/ship-it", nil },
		verifyPRMatchesBranch: func(_ context.Context, _, _ string, prNumber int, _ string) (PRVerification, error) {
			if prNumber == 100 {
				return NewPRVerification(true, false, "backlog/ship-it", githubpkg.PRStateOpen, "tstapler"), nil
			}
			return NewPRVerification(true, true, "backlog/ship-it", githubpkg.PRStateOpen, "tstapler"), nil
		},
		resolveCallerGitHubLogin: func(context.Context) (string, error) { return "tstapler", nil },
	}
	ctxWithUUID := WithSessionUUID(context.Background(), sessionUUID)

	req := makeToolReq(map[string]interface{}{
		"item_id":         item.ID,
		"pr_url":          "https://github.com/tstapler/stapler-squad/pull/200",
		"pr_number":       float64(200),
		"summary":         "Corrected the tracked PR after closing the polluted one.",
		"override_reason": "tracked branch was polluted by another session; opened a clean PR instead and closed the original",
	})

	result, err := handler.reportPRCreated(ctxWithUUID, req)
	require.NoError(t, err)
	require.Len(t, result.Content, 1)
	tc, ok := result.Content[0].(mcpgo.TextContent)
	require.True(t, ok)
	assert.Contains(t, tc.Text, "pr_pending")

	fetched, err := storage.GetBacklogItem(context.Background(), item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(session.BacklogStatusPRPending), fetched.Status)
	assert.Equal(t, 200, fetched.PrNumber)
	assert.Equal(t, "https://github.com/tstapler/stapler-squad/pull/200", fetched.PrURL)
}

// TestReportPRCreated_should_RejectReassignment_When_AlreadyPRPendingMissingOverrideReason
// (AC1): reassignment from pr_pending without override_reason is rejected
// before any GitHub call, even though nothing about the new PR itself has
// been checked yet — a matching branch does not excuse the missing reason.
func TestReportPRCreated_should_RejectReassignment_When_AlreadyPRPendingMissingOverrideReason(t *testing.T) {
	storage := newTestBacklogStorage(t)
	item, sessionUUID := setupReportPRCreatedReassignmentFixture(t, storage, 100, false)

	verifyCalled := false
	loginCalled := false
	handler := &backlogHandlers{
		storage:              storage,
		resolveSessionBranch: func(context.Context, string) (string, error) { return "backlog/ship-it", nil },
		verifyPRMatchesBranch: func(context.Context, string, string, int, string) (PRVerification, error) {
			verifyCalled = true
			return NewPRVerification(true, true, "backlog/ship-it", githubpkg.PRStateOpen, "tstapler"), nil
		},
		resolveCallerGitHubLogin: func(context.Context) (string, error) {
			loginCalled = true
			return "tstapler", nil
		},
	}
	ctxWithUUID := WithSessionUUID(context.Background(), sessionUUID)

	req := makeToolReq(map[string]interface{}{
		"item_id":   item.ID,
		"pr_url":    "https://github.com/tstapler/stapler-squad/pull/200",
		"pr_number": float64(200),
		"summary":   "Trying to correct the PR without a reason.",
	})

	result, err := handler.reportPRCreated(ctxWithUUID, req)
	require.NoError(t, err)

	m := parseResult(t, result)
	require.False(t, m["success"].(bool))
	errObj, ok := m["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, ErrInvalidArgument, errObj["code"].(string))
	assert.Contains(t, errObj["message"].(string), "override_reason")
	assert.False(t, verifyCalled, "must reject before any GitHub verification when reassigning without override_reason")
	assert.False(t, loginCalled, "resolveCallerGitHubLogin must not be called when override_reason is missing")

	fetched, err := storage.GetBacklogItem(context.Background(), item.ID)
	require.NoError(t, err)
	assert.Equal(t, 100, fetched.PrNumber, "tracked PR must remain unchanged")
	assert.Equal(t, string(session.BacklogStatusPRPending), fetched.Status)
}

// TestReportPRCreated_should_RejectReassignment_When_CurrentPRAlreadyMerged
// (AC2): once the currently tracked PR is merged, reassignment is
// hard-rejected with no override escape hatch — a merged PR's association
// with the item must never be silently swapped, even with override_reason
// set. The new PR's own verification must never even run.
func TestReportPRCreated_should_RejectReassignment_When_CurrentPRAlreadyMerged(t *testing.T) {
	storage := newTestBacklogStorage(t)
	item, sessionUUID := setupReportPRCreatedReassignmentFixture(t, storage, 100, false)

	handler := &backlogHandlers{
		storage:              storage,
		resolveSessionBranch: func(context.Context, string) (string, error) { return "backlog/ship-it", nil },
		verifyPRMatchesBranch: func(_ context.Context, _, _ string, prNumber int, _ string) (PRVerification, error) {
			if prNumber == 100 {
				return NewPRVerification(true, false, "backlog/ship-it", githubpkg.PRStateMerged, "tstapler"), nil
			}
			t.Fatalf("verifyPRMatchesBranch called for new PR #%d — must hard-reject before verifying the new PR once the currently tracked PR is already merged", prNumber)
			return PRVerification{}, nil
		},
		resolveCallerGitHubLogin: func(context.Context) (string, error) { return "tstapler", nil },
	}
	ctxWithUUID := WithSessionUUID(context.Background(), sessionUUID)

	req := makeToolReq(map[string]interface{}{
		"item_id":         item.ID,
		"pr_url":          "https://github.com/tstapler/stapler-squad/pull/200",
		"pr_number":       float64(200),
		"summary":         "Trying to reassign a merged PR.",
		"override_reason": "trying anyway",
	})

	result, err := handler.reportPRCreated(ctxWithUUID, req)
	require.NoError(t, err)

	m := parseResult(t, result)
	require.False(t, m["success"].(bool))
	errObj, ok := m["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, ErrInvalidArgument, errObj["code"].(string))
	assert.Contains(t, errObj["message"].(string), "already merged")

	fetched, err := storage.GetBacklogItem(context.Background(), item.ID)
	require.NoError(t, err)
	assert.Equal(t, 100, fetched.PrNumber, "tracked PR must remain unchanged")
	assert.Equal(t, string(session.BacklogStatusPRPending), fetched.Status)
}

// TestReportPRCreated_should_RejectSecondReassignment_When_ConcurrentCASRace
// (AC3): two concurrent reassignment calls racing on the same pr_pending
// item, each correcting to a different new PR number, must result in exactly
// one winner via the same atomic CAS write TestReportPRCreated_
// LoserPRNeverPersists_WhenCASPreconditionFails exercises for the review ->
// pr_pending path — the loser gets a clear error and its PR number never
// lands, even transiently.
func TestReportPRCreated_should_RejectSecondReassignment_When_ConcurrentCASRace(t *testing.T) {
	storage := newTestBacklogStorage(t)
	item, sessionUUID := setupReportPRCreatedReassignmentFixture(t, storage, 100, false)

	// readBarrier forces both goroutines' pre-write GetBacklogItem reads
	// (routed through the getBacklogItemFor seam) to complete before either
	// is allowed to proceed into SetBacklogItemPRAndTransition's write — see
	// TestReportPRCreated_LoserPRNeverPersists_WhenCASPreconditionFails's doc
	// comment for why this matters.
	var readBarrier sync.WaitGroup
	readBarrier.Add(2)
	getBacklogItemFn := func(fnCtx context.Context, itemID string) (*session.BacklogItemData, error) {
		it, getErr := storage.GetBacklogItem(fnCtx, itemID)
		readBarrier.Done()
		readBarrier.Wait()
		return it, getErr
	}

	handler := &backlogHandlers{
		storage:              storage,
		getBacklogItemFn:     getBacklogItemFn,
		resolveSessionBranch: func(context.Context, string) (string, error) { return "backlog/ship-it", nil },
		verifyPRMatchesBranch: func(_ context.Context, _, _ string, prNumber int, _ string) (PRVerification, error) {
			if prNumber == 100 {
				return NewPRVerification(true, false, "backlog/ship-it", githubpkg.PRStateOpen, "tstapler"), nil
			}
			return NewPRVerification(true, true, "backlog/ship-it", githubpkg.PRStateOpen, "tstapler"), nil
		},
		resolveCallerGitHubLogin: func(context.Context) (string, error) { return "tstapler", nil },
	}
	ctxWithUUID := WithSessionUUID(context.Background(), sessionUUID)

	type prRaceResult struct {
		prNumber int
		result   *mcpgo.CallToolResult
		err      error
	}

	var wg sync.WaitGroup
	var startBarrier sync.WaitGroup
	startBarrier.Add(1)
	results := make(chan prRaceResult, 2)

	race := func(prNumber int) {
		defer wg.Done()
		startBarrier.Wait()
		req := makeToolReq(map[string]interface{}{
			"item_id":         item.ID,
			"pr_url":          fmt.Sprintf("https://github.com/tstapler/stapler-squad/pull/%d", prNumber),
			"pr_number":       float64(prNumber),
			"summary":         fmt.Sprintf("Racing reassignment call for PR #%d.", prNumber),
			"override_reason": "racing correction",
		})
		result, callErr := handler.reportPRCreated(ctxWithUUID, req)
		results <- prRaceResult{prNumber: prNumber, result: result, err: callErr}
	}

	wg.Add(2)
	go race(300)
	go race(400)
	startBarrier.Done()
	wg.Wait()
	close(results)

	var successes, failures int
	var winnerPRNumber int
	for rr := range results {
		require.NoError(t, rr.err)
		require.Len(t, rr.result.Content, 1)
		tc, ok := rr.result.Content[0].(mcpgo.TextContent)
		require.True(t, ok)

		// The success path returns plain (non-JSON) text; errResult always
		// emits a JSON-encoded MCPResult.
		var parsed map[string]interface{}
		if jsonErr := json.Unmarshal([]byte(tc.Text), &parsed); jsonErr != nil {
			successes++
			winnerPRNumber = rr.prNumber
			continue
		}
		failures++
	}

	require.Equal(t, 1, successes, "exactly one racer should win the CAS reassignment")
	require.Equal(t, 1, failures, "the loser must see an error, not silently succeed")

	fetched, err := storage.GetBacklogItem(context.Background(), item.ID)
	require.NoError(t, err)
	require.Equal(t, string(session.BacklogStatusPRPending), fetched.Status)
	assert.Equal(t, winnerPRNumber, fetched.PrNumber,
		"the persisted PrNumber must match whichever racer's reassignment actually won the CAS — the loser's PR number must never land, even transiently")
}

// TestReportPRCreated_should_RecordDistinctAuditNote_When_Reassigned (AC6):
// the audit trail must distinguish a reassignment from a first-time PR
// recording — SetBacklogItemPRAndTransition's progress note Status is
// "pr_corrected" on reassignment, never the first-time "pr_created".
func TestReportPRCreated_should_RecordDistinctAuditNote_When_Reassigned(t *testing.T) {
	storage := newTestBacklogStorage(t)
	item, sessionUUID := setupReportPRCreatedReassignmentFixture(t, storage, 100, false)

	handler := &backlogHandlers{
		storage:              storage,
		resolveSessionBranch: func(context.Context, string) (string, error) { return "backlog/ship-it", nil },
		verifyPRMatchesBranch: func(_ context.Context, _, _ string, prNumber int, _ string) (PRVerification, error) {
			if prNumber == 100 {
				return NewPRVerification(true, false, "backlog/ship-it", githubpkg.PRStateOpen, "tstapler"), nil
			}
			return NewPRVerification(true, true, "backlog/ship-it", githubpkg.PRStateOpen, "tstapler"), nil
		},
		resolveCallerGitHubLogin: func(context.Context) (string, error) { return "tstapler", nil },
	}
	ctxWithUUID := WithSessionUUID(context.Background(), sessionUUID)

	req := makeToolReq(map[string]interface{}{
		"item_id":         item.ID,
		"pr_url":          "https://github.com/tstapler/stapler-squad/pull/200",
		"pr_number":       float64(200),
		"summary":         "Corrected the tracked PR.",
		"override_reason": "tracked branch was polluted; opened a clean PR instead",
	})

	_, err := handler.reportPRCreated(ctxWithUUID, req)
	require.NoError(t, err)

	notes, err := storage.ListProgressNotesForItem(context.Background(), item.ID)
	require.NoError(t, err)
	require.NotEmpty(t, notes)
	found := false
	for _, n := range notes {
		assert.NotEqual(t, "pr_created", n.Status, "a reassignment must not be recorded with the same note status as a first-time recording")
		if n.Status == "pr_corrected" {
			found = true
		}
	}
	assert.True(t, found, "expected a progress note with status=pr_corrected distinguishing this reassignment from a first-time PR recording")
}

// TestReportPRCreated_should_ClearPrFeedbackAddressedAt_When_Reassigned
// (AC7): reassigning to a new PR must not leave the old PR's
// feedback-dedup watermark in place.
func TestReportPRCreated_should_ClearPrFeedbackAddressedAt_When_Reassigned(t *testing.T) {
	storage := newTestBacklogStorage(t)
	item, sessionUUID := setupReportPRCreatedReassignmentFixture(t, storage, 100, true)
	require.NotNil(t, item.PrFeedbackAddressedAt, "fixture must seed pr_feedback_addressed_at")

	handler := &backlogHandlers{
		storage:              storage,
		resolveSessionBranch: func(context.Context, string) (string, error) { return "backlog/ship-it", nil },
		verifyPRMatchesBranch: func(_ context.Context, _, _ string, prNumber int, _ string) (PRVerification, error) {
			if prNumber == 100 {
				return NewPRVerification(true, false, "backlog/ship-it", githubpkg.PRStateOpen, "tstapler"), nil
			}
			return NewPRVerification(true, true, "backlog/ship-it", githubpkg.PRStateOpen, "tstapler"), nil
		},
		resolveCallerGitHubLogin: func(context.Context) (string, error) { return "tstapler", nil },
	}
	ctxWithUUID := WithSessionUUID(context.Background(), sessionUUID)

	req := makeToolReq(map[string]interface{}{
		"item_id":         item.ID,
		"pr_url":          "https://github.com/tstapler/stapler-squad/pull/200",
		"pr_number":       float64(200),
		"summary":         "Corrected the tracked PR.",
		"override_reason": "tracked branch was polluted; opened a clean PR instead",
	})

	_, err := handler.reportPRCreated(ctxWithUUID, req)
	require.NoError(t, err)

	fetched, err := storage.GetBacklogItem(context.Background(), item.ID)
	require.NoError(t, err)
	assert.Nil(t, fetched.PrFeedbackAddressedAt, "pr_feedback_addressed_at must be cleared when the tracked PR is reassigned")
}

// TestReportPRCreated_should_ReturnFriendlyError_When_CASFailsOutOfBand
// (AC8): a CAS failure from an out-of-band status change between this call's
// read and its write must surface as a friendly, actionable message — not
// the raw internal precondition-failed error. Uses the getBacklogItemFn seam
// to inject a write (bumping updated_at) between the handler's read and its
// own primary write, simulating another action landing in that gap.
func TestReportPRCreated_should_ReturnFriendlyError_When_CASFailsOutOfBand(t *testing.T) {
	storage := newTestBacklogStorage(t)
	item, sessionUUID := setupReportPRCreatedFixture(t, storage, session.BacklogStatusReview)

	getBacklogItemFn := func(fnCtx context.Context, itemID string) (*session.BacklogItemData, error) {
		it, getErr := storage.GetBacklogItem(fnCtx, itemID)
		if getErr != nil {
			return it, getErr
		}
		title := "Retitled out from under this call"
		if _, updErr := storage.UpdateBacklogItem(fnCtx, itemID, session.BacklogItemUpdate{Title: &title}, nil); updErr != nil {
			return it, updErr
		}
		return it, nil
	}

	handler := &backlogHandlers{
		storage:              storage,
		getBacklogItemFn:     getBacklogItemFn,
		resolveSessionBranch: func(context.Context, string) (string, error) { return "backlog/ship-it", nil },
		verifyPRMatchesBranch: func(context.Context, string, string, int, string) (PRVerification, error) {
			return NewPRVerification(true, true, "backlog/ship-it", githubpkg.PRStateOpen, "tstapler"), nil
		},
	}
	ctxWithUUID := WithSessionUUID(context.Background(), sessionUUID)

	req := makeToolReq(map[string]interface{}{
		"item_id":   item.ID,
		"pr_url":    "https://github.com/tstapler/stapler-squad/pull/42",
		"pr_number": float64(42),
		"summary":   "Implemented the feature.",
	})

	result, err := handler.reportPRCreated(ctxWithUUID, req)
	require.NoError(t, err)

	m := parseResult(t, result)
	require.False(t, m["success"].(bool))
	errObj, ok := m["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, ErrInternalError, errObj["code"].(string))
	msg := errObj["message"].(string)
	assert.Contains(t, msg, "item state changed since your last read")
	assert.NotContains(t, msg, "precondition failed", "must surface the friendly wrapped message, not the raw internal error text")
}

// TestReportPRCreated_should_RejectReassignment_When_AuthorMismatch (AC9):
// the reassignment path requires the new PR's author to match the caller's
// verified GitHub identity, even when the new PR's branch matches this
// item's tracked branch — the same gate the review-path override already
// enforces (TestReportPRCreated_should_RejectCall_When_UnrelatedPRAuthorMismatch).
func TestReportPRCreated_should_RejectReassignment_When_AuthorMismatch(t *testing.T) {
	storage := newTestBacklogStorage(t)
	item, sessionUUID := setupReportPRCreatedReassignmentFixture(t, storage, 100, false)

	handler := &backlogHandlers{
		storage:              storage,
		resolveSessionBranch: func(context.Context, string) (string, error) { return "backlog/ship-it", nil },
		verifyPRMatchesBranch: func(_ context.Context, _, _ string, prNumber int, _ string) (PRVerification, error) {
			if prNumber == 100 {
				return NewPRVerification(true, false, "backlog/ship-it", githubpkg.PRStateOpen, "tstapler"), nil
			}
			return NewPRVerification(true, true, "backlog/ship-it", githubpkg.PRStateOpen, "someone-else"), nil
		},
		resolveCallerGitHubLogin: func(context.Context) (string, error) { return "tstapler", nil },
	}
	ctxWithUUID := WithSessionUUID(context.Background(), sessionUUID)

	req := makeToolReq(map[string]interface{}{
		"item_id":         item.ID,
		"pr_url":          "https://github.com/tstapler/stapler-squad/pull/200",
		"pr_number":       float64(200),
		"summary":         "Attempting to reassign to someone else's PR.",
		"override_reason": "trying anyway",
	})

	result, err := handler.reportPRCreated(ctxWithUUID, req)
	require.NoError(t, err)

	m := parseResult(t, result)
	require.False(t, m["success"].(bool))
	errObj, ok := m["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, ErrInvalidArgument, errObj["code"].(string))
	assert.Contains(t, errObj["message"].(string), "someone-else")

	fetched, err := storage.GetBacklogItem(context.Background(), item.ID)
	require.NoError(t, err)
	assert.Equal(t, 100, fetched.PrNumber, "tracked PR must remain unchanged on author mismatch")
	assert.Equal(t, string(session.BacklogStatusPRPending), fetched.Status)
}

// fakeTriageHeadlessPool is a minimal headless.PoolClient stub used only to
// prove MaybeTriggerTriage actually reaches TriggerTriage's headless-pool gate
// — it does not need to exercise the full triage parse/persist pipeline in
// any detail, just return a syntactically valid HeadlessTriageResult so the
// background goroutine TriggerTriage launches completes cleanly instead of
// racing this test's t.Cleanup (which closes the ent repo and removes its
// temp dir).
type fakeTriageHeadlessPool struct{}

func (f *fakeTriageHeadlessPool) CallBlocking(ctx context.Context, key headless.FeatureKey, systemPrompt, userPrompt string, opts headless.CallOptions) (string, float64, error) {
	return `{"title":"t","summary":"s","suggestions":[]}`, 0, nil
}

// TestCreateBacklogItem_should_TriggerTriage_When_BacklogSvcWiredAndRepoPathSet
// is the regression test for BUG-061: create_backlog_item used to call
// h.storage.CreateBacklogItem directly with no post-create auto-triage step,
// unlike BacklogService.CreateBacklogItem (the RPC handler backing the web
// UI's "New Idea" form), which does trigger triage. An item self-filed by an
// agent session via this MCP tool sat in "idea" with zero triage attempts
// forever — reconcileOrphanedTriageItems (session/backlog_lifecycle.go) only
// ever detects items that already have a prior triage-role ItemSession, so it
// can never originate the first attempt for an item created this way.
func TestCreateBacklogItem_should_TriggerTriage_When_BacklogSvcWiredAndRepoPathSet(t *testing.T) {
	storage := newTestBacklogStorage(t)
	backlogSvc := services.NewBacklogService(storage, nil, nil, nil, nil, nil)
	backlogSvc.SetHeadlessPool(&fakeTriageHeadlessPool{})
	t.Cleanup(backlogSvc.Shutdown)

	handler := &backlogHandlers{storage: storage, backlogSvc: backlogSvc}
	ctx := WithSessionUUID(context.Background(), uuid.New().String())

	result, err := handler.createBacklogItem(ctx, makeToolReq(map[string]interface{}{
		"title":     "Needs triage",
		"repo_path": t.TempDir(),
	}))
	require.NoError(t, err)
	require.Len(t, result.Content, 1)
	tc, ok := result.Content[0].(mcpgo.TextContent)
	require.True(t, ok)
	assert.Contains(t, tc.Text, "Auto-triage started.",
		"create_backlog_item must trigger auto-triage when a BacklogService and repo_path are available")

	var itemID string
	_, scanErr := fmt.Sscanf(tc.Text, "Created backlog item %s", &itemID)
	require.NoError(t, scanErr)
	itemID = strings.TrimSuffix(itemID, ":")

	sessions, err := storage.ListItemSessions(context.Background(), itemID)
	require.NoError(t, err)
	require.Len(t, sessions, 1, "expected exactly one ItemSession created by TriggerTriage")
	assert.Equal(t, session.SessionRoleTriage, sessions[0].Role)
}

// TestCreateBacklogItem_should_NotTriggerTriage_When_BacklogSvcNil verifies
// the pre-fix behavior is preserved when no BacklogService is wired (e.g. the
// stdio MCP fallback path, which has no *services.BacklogService available —
// see RunServer's doc comment): item creation still succeeds, just without
// auto-triage, rather than panicking on a nil backlogSvc.
func TestCreateBacklogItem_should_NotTriggerTriage_When_BacklogSvcNil(t *testing.T) {
	storage := newTestBacklogStorage(t)
	handler := &backlogHandlers{storage: storage} // backlogSvc left nil
	ctx := WithSessionUUID(context.Background(), uuid.New().String())

	result, err := handler.createBacklogItem(ctx, makeToolReq(map[string]interface{}{
		"title":     "No backlog service wired",
		"repo_path": t.TempDir(),
	}))
	require.NoError(t, err)
	tc, ok := result.Content[0].(mcpgo.TextContent)
	require.True(t, ok)
	assert.Contains(t, tc.Text, "Auto-triage not triggered")

	var itemID string
	_, scanErr := fmt.Sscanf(tc.Text, "Created backlog item %s", &itemID)
	require.NoError(t, scanErr)
	itemID = strings.TrimSuffix(itemID, ":")

	sessions, err := storage.ListItemSessions(context.Background(), itemID)
	require.NoError(t, err)
	assert.Empty(t, sessions, "no ItemSession should be created when backlogSvc is nil")
}

// TestRegisterBacklogTools_RequestReview_DescribesAlreadyImplementedCitationRequirement
// verifies that the request_review tool description (and its verification_notes
// field description) instruct the agent to cite an exact file path and
// function/symbol when claiming an acceptance criterion is already satisfied by
// existing code, rather than making an unsupported "already implemented" claim.
func TestRegisterBacklogTools_RequestReview_DescribesAlreadyImplementedCitationRequirement(t *testing.T) {
	data, err := os.ReadFile("tools_backlog.go")
	require.NoError(t, err, "read tools_backlog.go")

	content := string(data)
	assert.Contains(t, content, "already satisfied by existing code",
		"request_review tool description must instruct agents to flag already-implemented acceptance criteria")
	assert.Contains(t, content, "cite the exact file path and function/symbol",
		"request_review tool description must require a file/function citation for already-implemented claims")
	assert.Contains(t, content, "already implemented",
		"request_review tool description must call out unsupported \"already implemented\" claims as weak evidence")
}

// --- report_duplicate ---

// setupReportDuplicateFixture creates an item at itemStatus with a linked
// work-role session, returning the item and the session UUID.
func setupReportDuplicateFixture(t *testing.T, storage *session.Storage, itemStatus session.BacklogStatus) (*session.BacklogItemData, string) {
	t.Helper()
	ctx := context.Background()

	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "Maybe a duplicate",
		Status: string(itemStatus),
	})
	require.NoError(t, err)

	sessionUUID := uuid.New().String()
	_, err = storage.CreateItemSession(ctx, session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: sessionUUID,
		SessionRole: session.SessionRoleWork,
	})
	require.NoError(t, err)

	return item, sessionUUID
}

// failOnCallVerifyGitHubRef returns a verifyGitHubRef stub that fails the
// test if invoked. Epic 3.1's Goal requires every FR6 refusal test to use
// this stub, turning "refused before any GitHub network call" into a
// property the suite enforces on every run.
func failOnCallVerifyGitHubRef(t *testing.T) func(ctx context.Context, ref *githubpkg.ParsedGitHubRef) error {
	t.Helper()
	return func(ctx context.Context, ref *githubpkg.ParsedGitHubRef) error {
		t.Fatal("verifyGitHubRef must not be called on a refusal path")
		return nil
	}
}

// fakeReviewTrigger records TriggerReviewForSession calls for assertions —
// the regression test for Task 3.3.3b's fixed second-spawn bug needs to
// assert the trigger is (or is not) called, not just check message text.
type fakeReviewTrigger struct {
	calls []string
}

func (f *fakeReviewTrigger) TriggerReviewForSession(sessionUUID string) {
	f.calls = append(f.calls, sessionUUID)
}

// resetGhBaseURL overrides githubpkg.GhBaseURL for a test and returns a
// restore func. Mirrors github/commits_test.go's and
// server/services/backlog_github_rpc_test.go's identically-named helpers —
// GhBaseURL is exported specifically so each consuming package can point it
// at an httptest.Server without needing a shared cross-package test helper.
func resetGhBaseURL(ts *httptest.Server) func() {
	githubpkg.GhBaseURL = ts.URL + "/"
	return func() { githubpkg.GhBaseURL = "https://api.github.com/" }
}

// TestReportDuplicate_VerifyGitHubRefExists_DispatchesPRTypeToRealGetPR
// exercises the real verifyGitHubRefExists dispatch switch (tools_backlog.go)
// end to end. Every other TestReportDuplicate_* test constructs backlogHandlers
// with a stubbed verifyGitHubRef field, so the real `switch ref.Type { case
// RefTypePR: ... case RefTypeIssue: ... case RefTypeCommit: ... }` dispatcher
// is never reached by any other test in this package — this leaves
// verifyGitHubRef nil so reportDuplicate falls through to verifyRef's real
// fallback (h.verifyGitHubRefExists), and points githubpkg.GhBaseURL at an
// httptest.Server (same pattern as github/repos_pr_test.go's TestGetPR and
// github/commits_test.go) to prove the PR branch of the switch actually
// invokes githubpkg.GetPR against the expected REST path.
func TestReportDuplicate_VerifyGitHubRefExists_DispatchesPRTypeToRealGetPR(t *testing.T) {
	storage := newTestBacklogStorage(t)
	item, sessionUUID := setupReportDuplicateFixture(t, storage, session.BacklogStatusInProgress)

	var gotPath string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"number":272,"title":"fix: something","state":"open",` +
			`"html_url":"https://github.com/tstapler/stapler-squad/pull/272"}`))
	}))
	defer ts.Close()
	defer resetGhBaseURL(ts)()
	t.Setenv("GITHUB_TOKEN", "fake-token")

	// verifyGitHubRef deliberately left nil — the real verifyRef/
	// verifyGitHubRefExists dispatch chain must run.
	handler := &backlogHandlers{storage: storage}
	ctxWithUUID := WithSessionUUID(context.Background(), sessionUUID)

	req := makeToolReq(map[string]interface{}{
		"item_id":       item.ID,
		"duplicate_ref": "https://github.com/tstapler/stapler-squad/pull/272",
		"reason":        "fc63d55b superseded by PR #272, same fix already merged",
	})

	result, err := handler.reportDuplicate(ctxWithUUID, req)
	require.NoError(t, err)
	require.Len(t, result.Content, 1)
	tc, ok := result.Content[0].(mcpgo.TextContent)
	require.True(t, ok)
	assert.Contains(t, tc.Text, "review")

	assert.Equal(t, "/repos/tstapler/stapler-squad/pulls/272", gotPath,
		"the real dispatcher must route RefTypePR to GetPR's REST path, not shell out or skip verification")

	fetched, err := storage.GetBacklogItem(context.Background(), item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(session.BacklogStatusReview), fetched.Status)
}

// --- Story 4.2.1: success paths ---

func TestReportDuplicate_TransitionsInProgressItemToReview_WithVerifiedPR(t *testing.T) {
	storage := newTestBacklogStorage(t)
	item, sessionUUID := setupReportDuplicateFixture(t, storage, session.BacklogStatusInProgress)

	trigger := &fakeReviewTrigger{}
	handler := &backlogHandlers{
		storage:       storage,
		reviewTrigger: trigger,
		verifyGitHubRef: func(ctx context.Context, ref *githubpkg.ParsedGitHubRef) error {
			assert.Equal(t, githubpkg.RefTypePR, ref.Type)
			assert.Equal(t, "tstapler", ref.Owner)
			assert.Equal(t, "stapler-squad", ref.Repo)
			assert.Equal(t, 272, ref.PRNumber)
			return nil
		},
	}
	ctxWithUUID := WithSessionUUID(context.Background(), sessionUUID)

	req := makeToolReq(map[string]interface{}{
		"item_id":       item.ID,
		"duplicate_ref": "https://github.com/tstapler/stapler-squad/pull/272",
		"reason":        "fc63d55b superseded by PR #272, same fix already merged",
	})

	result, err := handler.reportDuplicate(ctxWithUUID, req)
	require.NoError(t, err)
	require.Len(t, result.Content, 1)
	tc, ok := result.Content[0].(mcpgo.TextContent)
	require.True(t, ok)
	assert.Contains(t, tc.Text, "review")
	assert.Contains(t, tc.Text, "Reviewer notified")

	fetched, err := storage.GetBacklogItem(context.Background(), item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(session.BacklogStatusReview), fetched.Status)

	require.NotEmpty(t, fetched.StatusEvents)
	last := fetched.StatusEvents[len(fetched.StatusEvents)-1]
	assert.Equal(t, session.TriggeredByAgent, last.TriggeredBy)
	require.NotNil(t, last.Note)
	assert.Contains(t, *last.Note, "duplicate of https://github.com/tstapler/stapler-squad/pull/272")

	itemSess, err := storage.GetItemSessionBySessionAndItem(context.Background(), sessionUUID, item.ID)
	require.NoError(t, err)
	assert.Contains(t, itemSess.VerificationNotes, "duplicate_ref=https://github.com/tstapler/stapler-squad/pull/272")
	assert.Contains(t, itemSess.VerificationNotes, "reason=fc63d55b superseded by PR #272, same fix already merged")

	require.Len(t, trigger.calls, 1, "reviewTrigger must be called when no reviewer is active")
	assert.Equal(t, sessionUUID, trigger.calls[0])
}

func TestReportDuplicate_TransitionsPRPendingItemToReview_WithVerifiedIssue(t *testing.T) {
	storage := newTestBacklogStorage(t)
	item, sessionUUID := setupReportDuplicateFixture(t, storage, session.BacklogStatusPRPending)

	handler := &backlogHandlers{
		storage: storage,
		verifyGitHubRef: func(ctx context.Context, ref *githubpkg.ParsedGitHubRef) error {
			assert.Equal(t, githubpkg.RefTypeIssue, ref.Type)
			assert.Equal(t, 99, ref.IssueNumber)
			return nil
		},
	}
	ctxWithUUID := WithSessionUUID(context.Background(), sessionUUID)

	req := makeToolReq(map[string]interface{}{
		"item_id":       item.ID,
		"duplicate_ref": "https://github.com/tstapler/stapler-squad/issues/99",
		"reason":        "already tracked by issue #99",
	})

	result, err := handler.reportDuplicate(ctxWithUUID, req)
	require.NoError(t, err)
	tc, ok := result.Content[0].(mcpgo.TextContent)
	require.True(t, ok)
	assert.Contains(t, tc.Text, "review")

	fetched, err := storage.GetBacklogItem(context.Background(), item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(session.BacklogStatusReview), fetched.Status)
	require.NotEmpty(t, fetched.StatusEvents)
	assert.Equal(t, session.TriggeredByAgent, fetched.StatusEvents[len(fetched.StatusEvents)-1].TriggeredBy)
}

func TestReportDuplicate_TransitionsInProgressItemToReview_WithVerifiedCommit(t *testing.T) {
	storage := newTestBacklogStorage(t)
	item, sessionUUID := setupReportDuplicateFixture(t, storage, session.BacklogStatusInProgress)

	handler := &backlogHandlers{
		storage: storage,
		verifyGitHubRef: func(ctx context.Context, ref *githubpkg.ParsedGitHubRef) error {
			assert.Equal(t, githubpkg.RefTypeCommit, ref.Type)
			assert.Equal(t, "fc63d55bd1234567890abcdef1234567890abcd", ref.CommitSHA)
			return nil
		},
	}
	ctxWithUUID := WithSessionUUID(context.Background(), sessionUUID)

	req := makeToolReq(map[string]interface{}{
		"item_id":       item.ID,
		"duplicate_ref": "https://github.com/tstapler/stapler-squad/commit/fc63d55bd1234567890abcdef1234567890abcd",
		"reason":        "same fix already committed",
	})

	result, err := handler.reportDuplicate(ctxWithUUID, req)
	require.NoError(t, err)
	tc, ok := result.Content[0].(mcpgo.TextContent)
	require.True(t, ok)
	assert.Contains(t, tc.Text, "review")

	fetched, err := storage.GetBacklogItem(context.Background(), item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(session.BacklogStatusReview), fetched.Status)
}

// TestReportDuplicate_MessageSaysReviewerNotified_WhenNoActiveReviewSession
// is the dedicated assertion for Story 3.3.3's affirmative message text (FR5
// positive branch) — validation.md's gap-fill list, distinct from the
// transition/audit-field assertions in Task 4.2.1b.
func TestReportDuplicate_MessageSaysReviewerNotified_WhenNoActiveReviewSession(t *testing.T) {
	storage := newTestBacklogStorage(t)
	item, sessionUUID := setupReportDuplicateFixture(t, storage, session.BacklogStatusInProgress)

	handler := &backlogHandlers{
		storage:         storage,
		verifyGitHubRef: func(ctx context.Context, ref *githubpkg.ParsedGitHubRef) error { return nil },
	}
	ctxWithUUID := WithSessionUUID(context.Background(), sessionUUID)

	req := makeToolReq(map[string]interface{}{
		"item_id":       item.ID,
		"duplicate_ref": "https://github.com/tstapler/stapler-squad/pull/272",
		"reason":        "duplicate",
	})

	result, err := handler.reportDuplicate(ctxWithUUID, req)
	require.NoError(t, err)
	tc, ok := result.Content[0].(mcpgo.TextContent)
	require.True(t, ok)
	assert.Contains(t, tc.Text, "Reviewer notified")
	assert.NotContains(t, tc.Text, "next review pass")
}

// --- Story 4.2.2: FR6 refusal paths (zero mutation, no GitHub call) ---

func TestReportDuplicate_RejectsWhenSkipReviewGateEnabled(t *testing.T) {
	storage := newTestBacklogStorage(t)
	ctx := context.Background()

	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:          "Skip gate item",
		Status:         string(session.BacklogStatusInProgress),
		SkipReviewGate: true,
	})
	require.NoError(t, err)

	sessionUUID := uuid.New().String()
	_, err = storage.CreateItemSession(ctx, session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: sessionUUID,
		SessionRole: session.SessionRoleWork,
	})
	require.NoError(t, err)

	handler := &backlogHandlers{storage: storage, verifyGitHubRef: failOnCallVerifyGitHubRef(t)}
	ctxWithUUID := WithSessionUUID(ctx, sessionUUID)

	req := makeToolReq(map[string]interface{}{
		"item_id":       item.ID,
		"duplicate_ref": "https://github.com/tstapler/stapler-squad/pull/272",
		"reason":        "duplicate",
	})

	result, err := handler.reportDuplicate(ctxWithUUID, req)
	require.NoError(t, err)

	m := parseResult(t, result)
	require.False(t, m["success"].(bool))
	errObj := m["error"].(map[string]interface{})
	assert.Equal(t, ErrInvalidArgument, errObj["code"])
	assert.Contains(t, errObj["message"], "SkipReviewGate")

	fetched, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(session.BacklogStatusInProgress), fetched.Status)
}

func TestReportDuplicate_RejectsWhenSessionRoleNotWork(t *testing.T) {
	storage := newTestBacklogStorage(t)
	ctx := context.Background()

	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "Review-role caller",
		Status: string(session.BacklogStatusInProgress),
	})
	require.NoError(t, err)

	sessionUUID := uuid.New().String()
	_, err = storage.CreateItemSession(ctx, session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: sessionUUID,
		SessionRole: session.SessionRoleReview,
	})
	require.NoError(t, err)

	handler := &backlogHandlers{storage: storage, verifyGitHubRef: failOnCallVerifyGitHubRef(t)}
	ctxWithUUID := WithSessionUUID(ctx, sessionUUID)

	req := makeToolReq(map[string]interface{}{
		"item_id":       item.ID,
		"duplicate_ref": "https://github.com/tstapler/stapler-squad/pull/272",
		"reason":        "duplicate",
	})

	result, err := handler.reportDuplicate(ctxWithUUID, req)
	require.NoError(t, err)

	m := parseResult(t, result)
	require.False(t, m["success"].(bool))
	errObj := m["error"].(map[string]interface{})
	assert.Equal(t, ErrPermissionDenied, errObj["code"])
}

func TestReportDuplicate_RejectsWhenSessionNotLinked(t *testing.T) {
	storage := newTestBacklogStorage(t)
	ctx := context.Background()

	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "Unlinked item",
		Status: string(session.BacklogStatusInProgress),
	})
	require.NoError(t, err)

	sessionUUID := uuid.New().String()
	handler := &backlogHandlers{storage: storage, verifyGitHubRef: failOnCallVerifyGitHubRef(t)}
	ctxWithUUID := WithSessionUUID(ctx, sessionUUID)

	req := makeToolReq(map[string]interface{}{
		"item_id":       item.ID,
		"duplicate_ref": "https://github.com/tstapler/stapler-squad/pull/272",
		"reason":        "duplicate",
	})

	result, err := handler.reportDuplicate(ctxWithUUID, req)
	require.NoError(t, err)

	m := parseResult(t, result)
	require.False(t, m["success"].(bool))
	errObj := m["error"].(map[string]interface{})
	assert.Equal(t, ErrPermissionDenied, errObj["code"])
}

func TestReportDuplicate_RejectsWhenSourceStatusNotAllowed(t *testing.T) {
	statuses := []string{
		string(session.BacklogStatusDone),
		string(session.BacklogStatusIdea),
		string(session.BacklogStatusReview),
		string(session.BacklogStatusArchived),
	}

	for _, status := range statuses {
		status := status
		t.Run(status, func(t *testing.T) {
			storage := newTestBacklogStorage(t)
			ctx := context.Background()

			item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
				Title:  "Disallowed source status",
				Status: status,
			})
			require.NoError(t, err)

			sessionUUID := uuid.New().String()
			_, err = storage.CreateItemSession(ctx, session.ItemSessionData{
				ItemID:      item.ID,
				SessionUUID: sessionUUID,
				SessionRole: session.SessionRoleWork,
			})
			require.NoError(t, err)

			handler := &backlogHandlers{storage: storage, verifyGitHubRef: failOnCallVerifyGitHubRef(t)}
			ctxWithUUID := WithSessionUUID(ctx, sessionUUID)

			req := makeToolReq(map[string]interface{}{
				"item_id":       item.ID,
				"duplicate_ref": "https://github.com/tstapler/stapler-squad/pull/272",
				"reason":        "duplicate",
			})

			result, err := handler.reportDuplicate(ctxWithUUID, req)
			require.NoError(t, err)

			m := parseResult(t, result)
			require.False(t, m["success"].(bool))
			errObj := m["error"].(map[string]interface{})
			assert.Equal(t, ErrInvalidArgument, errObj["code"])
			assert.Contains(t, errObj["message"], status)

			fetched, err := storage.GetBacklogItem(ctx, item.ID)
			require.NoError(t, err)
			assert.Equal(t, status, fetched.Status)
		})
	}
}

func TestReportDuplicate_RejectsWhenDuplicateRefOrReasonTooLong(t *testing.T) {
	cases := []struct {
		name         string
		duplicateRef string
		reason       string
		wantContains string
	}{
		{
			name:         "duplicate_ref too long",
			duplicateRef: "https://github.com/tstapler/stapler-squad/pull/" + strings.Repeat("1", 501),
			reason:       "valid reason",
			wantContains: "duplicate_ref",
		},
		{
			name:         "reason too long",
			duplicateRef: "https://github.com/tstapler/stapler-squad/pull/272",
			reason:       strings.Repeat("a", 1001),
			wantContains: "reason",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			storage := newTestBacklogStorage(t)
			item, sessionUUID := setupReportDuplicateFixture(t, storage, session.BacklogStatusInProgress)

			handler := &backlogHandlers{storage: storage, verifyGitHubRef: failOnCallVerifyGitHubRef(t)}
			ctxWithUUID := WithSessionUUID(context.Background(), sessionUUID)

			req := makeToolReq(map[string]interface{}{
				"item_id":       item.ID,
				"duplicate_ref": tc.duplicateRef,
				"reason":        tc.reason,
			})

			result, err := handler.reportDuplicate(ctxWithUUID, req)
			require.NoError(t, err)

			m := parseResult(t, result)
			require.False(t, m["success"].(bool))
			errObj := m["error"].(map[string]interface{})
			assert.Equal(t, ErrInvalidArgument, errObj["code"])
			assert.Contains(t, errObj["message"], tc.wantContains)

			fetched, err := storage.GetBacklogItem(context.Background(), item.ID)
			require.NoError(t, err)
			assert.Equal(t, string(session.BacklogStatusInProgress), fetched.Status)
		})
	}
}

// TestReportDuplicate_AllowsCrossRepoDuplicateRef verifies a duplicate_ref
// pointing at a different repo than the item's own is allowed (adversarial-
// review-added — nothing in FR3 or the dispatcher restricts duplicate_ref to
// the item's own repo). This is a success path, not a refusal — it must NOT
// use failOnCallVerifyGitHubRef.
func TestReportDuplicate_AllowsCrossRepoDuplicateRef(t *testing.T) {
	storage := newTestBacklogStorage(t)
	item, sessionUUID := setupReportDuplicateFixture(t, storage, session.BacklogStatusInProgress)

	verifyCalledWith := (*githubpkg.ParsedGitHubRef)(nil)
	handler := &backlogHandlers{
		storage: storage,
		verifyGitHubRef: func(ctx context.Context, ref *githubpkg.ParsedGitHubRef) error {
			verifyCalledWith = ref
			return nil
		},
	}
	ctxWithUUID := WithSessionUUID(context.Background(), sessionUUID)

	req := makeToolReq(map[string]interface{}{
		"item_id":       item.ID,
		"duplicate_ref": "https://github.com/some-other-org/some-other-repo/pull/5",
		"reason":        "same fix already shipped in a shared-library repo",
	})

	result, err := handler.reportDuplicate(ctxWithUUID, req)
	require.NoError(t, err)
	tc, ok := result.Content[0].(mcpgo.TextContent)
	require.True(t, ok)
	assert.Contains(t, tc.Text, "review")

	require.NotNil(t, verifyCalledWith)
	assert.Equal(t, "some-other-org", verifyCalledWith.Owner)
	assert.Equal(t, "some-other-repo", verifyCalledWith.Repo)

	fetched, err := storage.GetBacklogItem(context.Background(), item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(session.BacklogStatusReview), fetched.Status)
}

// --- Story 3.2.1 gap-fill: pre-network parse/type validation (FR3) ---

func TestReportDuplicate_RejectsWhenDuplicateRefNotParseable(t *testing.T) {
	storage := newTestBacklogStorage(t)
	item, sessionUUID := setupReportDuplicateFixture(t, storage, session.BacklogStatusInProgress)

	handler := &backlogHandlers{storage: storage, verifyGitHubRef: failOnCallVerifyGitHubRef(t)}
	ctxWithUUID := WithSessionUUID(context.Background(), sessionUUID)

	req := makeToolReq(map[string]interface{}{
		"item_id":       item.ID,
		"duplicate_ref": "not a url at all",
		"reason":        "duplicate",
	})

	result, err := handler.reportDuplicate(ctxWithUUID, req)
	require.NoError(t, err)

	m := parseResult(t, result)
	require.False(t, m["success"].(bool))
	errObj := m["error"].(map[string]interface{})
	assert.Equal(t, ErrInvalidArgument, errObj["code"])

	fetched, err := storage.GetBacklogItem(context.Background(), item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(session.BacklogStatusInProgress), fetched.Status)
}

func TestReportDuplicate_RejectsWhenDuplicateRefIsUnsupportedRefType(t *testing.T) {
	storage := newTestBacklogStorage(t)
	item, sessionUUID := setupReportDuplicateFixture(t, storage, session.BacklogStatusInProgress)

	handler := &backlogHandlers{storage: storage, verifyGitHubRef: failOnCallVerifyGitHubRef(t)}
	ctxWithUUID := WithSessionUUID(context.Background(), sessionUUID)

	req := makeToolReq(map[string]interface{}{
		"item_id":       item.ID,
		"duplicate_ref": "https://github.com/tstapler/stapler-squad/tree/main",
		"reason":        "duplicate",
	})

	result, err := handler.reportDuplicate(ctxWithUUID, req)
	require.NoError(t, err)

	m := parseResult(t, result)
	require.False(t, m["success"].(bool))
	errObj := m["error"].(map[string]interface{})
	assert.Equal(t, ErrInvalidArgument, errObj["code"])
	assert.Contains(t, errObj["message"], "Branch")

	fetched, err := storage.GetBacklogItem(context.Background(), item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(session.BacklogStatusInProgress), fetched.Status)
}

// --- Story 4.2.3: FR4 two/three-channel error tests ---

func TestReportDuplicate_RejectsWhenGitHubRefNotFound(t *testing.T) {
	storage := newTestBacklogStorage(t)
	item, sessionUUID := setupReportDuplicateFixture(t, storage, session.BacklogStatusInProgress)

	handler := &backlogHandlers{
		storage: storage,
		verifyGitHubRef: func(ctx context.Context, ref *githubpkg.ParsedGitHubRef) error {
			return fmt.Errorf("%w: PR not found (404)", githubpkg.ErrGitHubRefNotFound)
		},
	}
	ctxWithUUID := WithSessionUUID(context.Background(), sessionUUID)

	req := makeToolReq(map[string]interface{}{
		"item_id":       item.ID,
		"duplicate_ref": "https://github.com/tstapler/stapler-squad/pull/999999",
		"reason":        "duplicate",
	})

	result, err := handler.reportDuplicate(ctxWithUUID, req)
	require.NoError(t, err)

	m := parseResult(t, result)
	require.False(t, m["success"].(bool))
	errObj := m["error"].(map[string]interface{})
	assert.Equal(t, ErrInvalidArgument, errObj["code"])
	assert.Contains(t, errObj["message"], "does not exist")

	fetched, err := storage.GetBacklogItem(context.Background(), item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(session.BacklogStatusInProgress), fetched.Status)
}

func TestReportDuplicate_ReturnsRetryableError_WhenGitHubVerificationTimesOut(t *testing.T) {
	storage := newTestBacklogStorage(t)
	item, sessionUUID := setupReportDuplicateFixture(t, storage, session.BacklogStatusInProgress)

	handler := &backlogHandlers{
		storage: storage,
		verifyGitHubRef: func(ctx context.Context, ref *githubpkg.ParsedGitHubRef) error {
			return fmt.Errorf("context deadline exceeded")
		},
	}
	ctxWithUUID := WithSessionUUID(context.Background(), sessionUUID)

	req := makeToolReq(map[string]interface{}{
		"item_id":       item.ID,
		"duplicate_ref": "https://github.com/tstapler/stapler-squad/pull/272",
		"reason":        "duplicate",
	})

	result, err := handler.reportDuplicate(ctxWithUUID, req)
	require.NoError(t, err)

	m := parseResult(t, result)
	require.False(t, m["success"].(bool))
	errObj := m["error"].(map[string]interface{})
	assert.Equal(t, ErrInternalError, errObj["code"])
	assert.Contains(t, errObj["message"], "retry")

	fetched, err := storage.GetBacklogItem(context.Background(), item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(session.BacklogStatusInProgress), fetched.Status)
}

func TestReportDuplicate_ReturnsRetryableError_WhenGitHubRateLimited(t *testing.T) {
	storage := newTestBacklogStorage(t)
	item, sessionUUID := setupReportDuplicateFixture(t, storage, session.BacklogStatusInProgress)

	handler := &backlogHandlers{
		storage: storage,
		verifyGitHubRef: func(ctx context.Context, ref *githubpkg.ParsedGitHubRef) error {
			return fmt.Errorf("GitHub API: rate limited (429)")
		},
	}
	ctxWithUUID := WithSessionUUID(context.Background(), sessionUUID)

	req := makeToolReq(map[string]interface{}{
		"item_id":       item.ID,
		"duplicate_ref": "https://github.com/tstapler/stapler-squad/pull/272",
		"reason":        "duplicate",
	})

	result, err := handler.reportDuplicate(ctxWithUUID, req)
	require.NoError(t, err)

	m := parseResult(t, result)
	require.False(t, m["success"].(bool))
	errObj := m["error"].(map[string]interface{})
	assert.Equal(t, ErrInternalError, errObj["code"])
	assert.Contains(t, errObj["message"], "retry")

	fetched, err := storage.GetBacklogItem(context.Background(), item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(session.BacklogStatusInProgress), fetched.Status)
}

func TestReportDuplicate_RejectsWhenGitHubAccessDenied(t *testing.T) {
	storage := newTestBacklogStorage(t)
	item, sessionUUID := setupReportDuplicateFixture(t, storage, session.BacklogStatusInProgress)

	handler := &backlogHandlers{
		storage: storage,
		verifyGitHubRef: func(ctx context.Context, ref *githubpkg.ParsedGitHubRef) error {
			return fmt.Errorf("%w: forbidden (403)", githubpkg.ErrGitHubAccessDenied)
		},
	}
	ctxWithUUID := WithSessionUUID(context.Background(), sessionUUID)

	req := makeToolReq(map[string]interface{}{
		"item_id":       item.ID,
		"duplicate_ref": "https://github.com/tstapler/stapler-squad/pull/272",
		"reason":        "duplicate",
	})

	result, err := handler.reportDuplicate(ctxWithUUID, req)
	require.NoError(t, err)

	m := parseResult(t, result)
	require.False(t, m["success"].(bool))
	errObj := m["error"].(map[string]interface{})
	assert.Equal(t, ErrInvalidArgument, errObj["code"])
	assert.Contains(t, errObj["message"], "denied access")

	fetched, err := storage.GetBacklogItem(context.Background(), item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(session.BacklogStatusInProgress), fetched.Status)
}

// TestReportDuplicate_ReturnsNonRetryableError_WhenNotAuthenticated formalizes
// the test Task 3.2.2b's own text promised ("Add
// TestReportDuplicate_ReturnsNonRetryableError_WhenNotAuthenticated to Story
// 4.2.3") but Story 4.2.3's task list never actually tracked — closes the FR4
// gap validation.md found. A missing-credentials failure must be classified
// distinctly from a generic transient failure (pre-mortem F1, P1): this is
// not retryable, and the message must say so explicitly rather than folding
// it into ordinary "retry" wording.
func TestReportDuplicate_ReturnsNonRetryableError_WhenNotAuthenticated(t *testing.T) {
	storage := newTestBacklogStorage(t)
	item, sessionUUID := setupReportDuplicateFixture(t, storage, session.BacklogStatusInProgress)

	handler := &backlogHandlers{
		storage: storage,
		verifyGitHubRef: func(ctx context.Context, ref *githubpkg.ParsedGitHubRef) error {
			return githubpkg.ErrNotAuthenticated
		},
	}
	ctxWithUUID := WithSessionUUID(context.Background(), sessionUUID)

	req := makeToolReq(map[string]interface{}{
		"item_id":       item.ID,
		"duplicate_ref": "https://github.com/tstapler/stapler-squad/pull/272",
		"reason":        "duplicate",
	})

	result, err := handler.reportDuplicate(ctxWithUUID, req)
	require.NoError(t, err)

	m := parseResult(t, result)
	require.False(t, m["success"].(bool))
	errObj := m["error"].(map[string]interface{})
	assert.Equal(t, ErrInternalError, errObj["code"])
	msg, _ := errObj["message"].(string)
	assert.Contains(t, msg, "GitHub credentials")
	assert.Contains(t, msg, "not a transient failure")

	fetched, err := storage.GetBacklogItem(context.Background(), item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(session.BacklogStatusInProgress), fetched.Status)
}

// --- Story 4.2.4: idempotency tests (ADR-004) ---

func TestReportDuplicate_NoOpOnExactRetry(t *testing.T) {
	storage := newTestBacklogStorage(t)
	item, sessionUUID := setupReportDuplicateFixture(t, storage, session.BacklogStatusInProgress)

	verifyCallCount := 0
	handler := &backlogHandlers{
		storage: storage,
		verifyGitHubRef: func(ctx context.Context, ref *githubpkg.ParsedGitHubRef) error {
			verifyCallCount++
			return nil
		},
	}
	ctxWithUUID := WithSessionUUID(context.Background(), sessionUUID)

	req := makeToolReq(map[string]interface{}{
		"item_id":       item.ID,
		"duplicate_ref": "https://github.com/tstapler/stapler-squad/pull/272",
		"reason":        "duplicate",
	})

	// First call succeeds and transitions the item.
	result, err := handler.reportDuplicate(ctxWithUUID, req)
	require.NoError(t, err)
	tc, ok := result.Content[0].(mcpgo.TextContent)
	require.True(t, ok)
	assert.Contains(t, tc.Text, "review")
	assert.Equal(t, 1, verifyCallCount)

	// Second, identical call is a no-op success — no second GitHub call, no
	// second TransitionBacklogItemStatus attempt.
	result2, err := handler.reportDuplicate(ctxWithUUID, req)
	require.NoError(t, err)
	tc2, ok := result2.Content[0].(mcpgo.TextContent)
	require.True(t, ok)
	assert.Contains(t, tc2.Text, "already recorded")
	assert.Equal(t, 1, verifyCallCount, "the no-op retry must not call GitHub verification again")

	fetched, err := storage.GetBacklogItem(context.Background(), item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(session.BacklogStatusReview), fetched.Status)
	require.Len(t, fetched.StatusEvents, 1, "the no-op retry must not add a second status event")
}

func TestReportDuplicate_RejectsDifferentRefAfterAlreadyResolved(t *testing.T) {
	storage := newTestBacklogStorage(t)
	item, sessionUUID := setupReportDuplicateFixture(t, storage, session.BacklogStatusInProgress)

	handler := &backlogHandlers{
		storage: storage,
		verifyGitHubRef: func(ctx context.Context, ref *githubpkg.ParsedGitHubRef) error {
			return nil
		},
	}
	ctxWithUUID := WithSessionUUID(context.Background(), sessionUUID)

	firstReq := makeToolReq(map[string]interface{}{
		"item_id":       item.ID,
		"duplicate_ref": "https://github.com/tstapler/stapler-squad/pull/272",
		"reason":        "duplicate",
	})
	_, err := handler.reportDuplicate(ctxWithUUID, firstReq)
	require.NoError(t, err)

	// Second call with a DIFFERENT ref, after the item already left the
	// whitelist — must be rejected, not merged into VerificationNotes
	// (ADR-004).
	handler.verifyGitHubRef = failOnCallVerifyGitHubRef(t)
	secondReq := makeToolReq(map[string]interface{}{
		"item_id":       item.ID,
		"duplicate_ref": "https://github.com/tstapler/stapler-squad/pull/300",
		"reason":        "actually this one",
	})
	result, err := handler.reportDuplicate(ctxWithUUID, secondReq)
	require.NoError(t, err)

	m := parseResult(t, result)
	require.False(t, m["success"].(bool))
	errObj := m["error"].(map[string]interface{})
	assert.Equal(t, ErrInvalidArgument, errObj["code"])
	assert.Contains(t, errObj["message"], "review")

	itemSess, err := storage.GetItemSessionBySessionAndItem(context.Background(), sessionUUID, item.ID)
	require.NoError(t, err)
	assert.NotContains(t, itemSess.VerificationNotes, "pull/300", "the second, differing ref must not be persisted")

	fetched, err := storage.GetBacklogItem(context.Background(), item.ID)
	require.NoError(t, err)
	require.Len(t, fetched.StatusEvents, 1, "the rejected second call must not add a status event")
}

// TestReportDuplicate_DoesNotTreatPrefixRefAsIdempotentMatch is the regression
// test for the idempotency check's deliberate use of an exact, delimited-line
// match (strings.HasPrefix(line, notesMarker+" ")) instead of
// strings.Contains — see the notesMarker comment in reportDuplicate
// (tools_backlog.go) and project_plans/backlog-self-resolve/research/pitfalls.md
// §0 / implementation/architecture-review.md for the real bug this fixed: a
// shorter ref (e.g. .../pull/27) is a literal string-prefix of a longer,
// unrelated ref (.../pull/272) sharing the same URL prefix. This test proves
// the collision is not silently misclassified as the exact-retry no-op —
// after the item leaves the {in_progress, pr_pending} whitelist, a second
// call with a differing (even if prefix-colliding) ref must hit the ordinary
// disallowed-source-status refusal path, same as any other second call
// (ADR-004's "reject, don't merge").
func TestReportDuplicate_DoesNotTreatPrefixRefAsIdempotentMatch(t *testing.T) {
	storage := newTestBacklogStorage(t)
	item, sessionUUID := setupReportDuplicateFixture(t, storage, session.BacklogStatusInProgress)

	handler := &backlogHandlers{
		storage:         storage,
		verifyGitHubRef: func(ctx context.Context, ref *githubpkg.ParsedGitHubRef) error { return nil },
	}
	ctxWithUUID := WithSessionUUID(context.Background(), sessionUUID)

	// First call: the longer ref, .../pull/272. Item transitions to review.
	firstReq := makeToolReq(map[string]interface{}{
		"item_id":       item.ID,
		"duplicate_ref": "https://github.com/tstapler/stapler-squad/pull/272",
		"reason":        "duplicate of the longer PR",
	})
	firstResult, err := handler.reportDuplicate(ctxWithUUID, firstReq)
	require.NoError(t, err)
	firstTC, ok := firstResult.Content[0].(mcpgo.TextContent)
	require.True(t, ok)
	assert.Contains(t, firstTC.Text, "review")

	fetchedAfterFirst, err := storage.GetBacklogItem(context.Background(), item.ID)
	require.NoError(t, err)
	require.Equal(t, string(session.BacklogStatusReview), fetchedAfterFirst.Status)

	// Second call on the SAME item: a shorter ref, .../pull/27, which is a
	// literal string-prefix of the first call's .../pull/272. Must NOT be
	// verified against GitHub — must not reach that far — because the item
	// is already outside the {in_progress, pr_pending} whitelist regardless
	// of idempotency classification.
	handler.verifyGitHubRef = failOnCallVerifyGitHubRef(t)
	secondReq := makeToolReq(map[string]interface{}{
		"item_id":       item.ID,
		"duplicate_ref": "https://github.com/tstapler/stapler-squad/pull/27",
		"reason":        "a different, shorter PR that happens to prefix-collide",
	})
	secondResult, err := handler.reportDuplicate(ctxWithUUID, secondReq)
	require.NoError(t, err)

	m := parseResult(t, secondResult)
	require.False(t, m["success"].(bool),
		"a prefix-colliding ref must not be silently treated as the idempotent no-op success")
	errObj := m["error"].(map[string]interface{})
	assert.Equal(t, ErrInvalidArgument, errObj["code"],
		"must hit the ordinary disallowed-source-status refusal, not report a false idempotent success")
	assert.Contains(t, errObj["message"], "review")

	fetchedFinal, err := storage.GetBacklogItem(context.Background(), item.ID)
	require.NoError(t, err)
	require.Len(t, fetchedFinal.StatusEvents, 1, "the refused prefix-colliding second call must not add a status event")
}

// TestReportDuplicate_ReportsDistinctMessage_WhenCASPreconditionFails mirrors
// TestRequestReview_ReportsDistinctMessage_WhenCASPreconditionFails's
// real-concurrency structure for report_duplicate's identical
// errors.Is(transErr, session.ErrPreconditionFailed) branch (tools_backlog.go,
// near where TransitionBacklogItemStatus is called): when two report_duplicate
// calls genuinely race on the same in_progress item, the DB-level atomic
// UPDATE...WHERE guarantees exactly one winner; the loser must see the same
// distinct, non-retry "state changed" message, still under ErrInternalError.
func TestReportDuplicate_ReportsDistinctMessage_WhenCASPreconditionFails(t *testing.T) {
	storage := newTestBacklogStorage(t)
	ctx := context.Background()

	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "Racing duplicate reports",
		Status: string(session.BacklogStatusInProgress),
	})
	require.NoError(t, err)

	sessionUUID := uuid.New().String()
	_, err = storage.CreateItemSession(ctx, session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: sessionUUID,
		SessionRole: session.SessionRoleWork,
	})
	require.NoError(t, err)

	// readBarrier forces both goroutines' pre-transition GetBacklogItem reads
	// to complete — both observing Status: "in_progress" — before either is
	// allowed to proceed to its TransitionBacklogItemStatus write. Without
	// this, startBarrier alone only synchronizes goroutine *start*: under
	// scheduling variance (e.g. a busy machine running the full test suite in
	// parallel), one goroutine can run its full read->write sequence before
	// the other's first read even executes. The loser then observes the
	// winner's already-committed Status: "review" plus matching
	// VerificationNotes and takes reportDuplicate's idempotency short-circuit
	// (same duplicate_ref/reason, same session) instead of racing the CAS
	// write — producing two successes instead of one success + one CAS
	// failure. Mirrors TestRequestReview_ReportsDistinctMessage_WhenCASPreconditionFails's
	// identical fix for the identical race shape.
	var readBarrier sync.WaitGroup
	readBarrier.Add(2)
	getBacklogItemFn := func(fnCtx context.Context, itemID string) (*session.BacklogItemData, error) {
		item, err := storage.GetBacklogItem(fnCtx, itemID)
		readBarrier.Done()
		readBarrier.Wait()
		return item, err
	}

	handler := &backlogHandlers{
		storage:          storage,
		verifyGitHubRef:  func(ctx context.Context, ref *githubpkg.ParsedGitHubRef) error { return nil },
		getBacklogItemFn: getBacklogItemFn,
	}
	ctxWithUUID := WithSessionUUID(ctx, sessionUUID)

	var wg sync.WaitGroup
	var startBarrier sync.WaitGroup
	startBarrier.Add(1)
	results := make(chan callToolRaceResult, 2)

	race := func() {
		defer wg.Done()
		startBarrier.Wait()
		req := makeToolReq(map[string]interface{}{
			"item_id":       item.ID,
			"duplicate_ref": "https://github.com/tstapler/stapler-squad/pull/272",
			"reason":        "Racing report_duplicate call.",
		})
		result, callErr := handler.reportDuplicate(ctxWithUUID, req)
		// See callToolRaceResult's doc comment: no require/assert calls
		// inside this goroutine — send both result and error and assert in
		// the main test goroutine after collecting from the channel.
		results <- callToolRaceResult{result: result, err: callErr}
	}

	wg.Add(2)
	go race()
	go race()
	startBarrier.Done()
	wg.Wait()
	close(results)

	var successes, failures int
	var failureMsg string
	for rr := range results {
		require.NoError(t, rr.err)
		result := rr.result
		require.Len(t, result.Content, 1)
		tc, ok := result.Content[0].(mcpgo.TextContent)
		require.True(t, ok)

		// The success path returns plain (non-JSON) text; errResult always
		// emits a JSON-encoded MCPResult. Distinguish on that, since both
		// possible outcomes here arrive as mcpgo.TextContent.
		var parsed map[string]interface{}
		if jsonErr := json.Unmarshal([]byte(tc.Text), &parsed); jsonErr != nil {
			successes++
			continue
		}
		failures++
		errObj := parsed["error"].(map[string]interface{})
		require.Equal(t, ErrInternalError, errObj["code"])
		failureMsg, _ = errObj["message"].(string)
	}

	require.Equal(t, 1, successes, "exactly one racer should win")
	require.Equal(t, 1, failures, "the loser must get a distinct message, not silently succeed")
	require.Contains(t, failureMsg, "state changed")

	fetched, err := storage.GetBacklogItem(context.Background(), item.ID)
	require.NoError(t, err)
	require.Equal(t, string(session.BacklogStatusReview), fetched.Status)
}

// --- Story 4.2.5: FR5 messaging tests ---

// TestReportDuplicate_MessageSaysNextReviewPass_WhenReviewSessionActive is
// the regression test for Task 3.3.3b's fixed second-spawn bug: with an
// active review session present, the trigger must NOT be called (the message
// wording alone was insufficient to catch a reintroduced unconditional
// trigger call).
func TestReportDuplicate_MessageSaysNextReviewPass_WhenReviewSessionActive(t *testing.T) {
	storage := newTestBacklogStorage(t)
	ctx := context.Background()

	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "Reviewer already running",
		Status: string(session.BacklogStatusInProgress),
	})
	require.NoError(t, err)

	workUUID := uuid.New().String()
	_, err = storage.CreateItemSession(ctx, session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: workUUID,
		SessionRole: session.SessionRoleWork,
	})
	require.NoError(t, err)

	reviewUUID := uuid.New().String()
	_, err = storage.CreateItemSession(ctx, session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: reviewUUID,
		SessionRole: session.SessionRoleReview,
	})
	require.NoError(t, err)

	trigger := &fakeReviewTrigger{}
	handler := &backlogHandlers{
		storage:         storage,
		reviewTrigger:   trigger,
		verifyGitHubRef: func(ctx context.Context, ref *githubpkg.ParsedGitHubRef) error { return nil },
	}
	ctxWithUUID := WithSessionUUID(ctx, workUUID)

	req := makeToolReq(map[string]interface{}{
		"item_id":       item.ID,
		"duplicate_ref": "https://github.com/tstapler/stapler-squad/pull/272",
		"reason":        "duplicate",
	})

	result, err := handler.reportDuplicate(ctxWithUUID, req)
	require.NoError(t, err)
	tc, ok := result.Content[0].(mcpgo.TextContent)
	require.True(t, ok)
	assert.Contains(t, tc.Text, "next review pass")
	assert.NotContains(t, tc.Text, "Reviewer notified")

	assert.Empty(t, trigger.calls, "reviewTrigger must NOT be called when a reviewer is already active — would spawn a second, concurrent review session")

	fetched, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(session.BacklogStatusReview), fetched.Status, "the transition itself must still succeed")
}

func TestReportDuplicate_MessageSaysNextReviewPass_WhenListItemSessionsErrors(t *testing.T) {
	storage := newTestBacklogStorage(t)
	item, sessionUUID := setupReportDuplicateFixture(t, storage, session.BacklogStatusInProgress)

	injectedErr := errors.New("boom: storage unavailable")
	trigger := &fakeReviewTrigger{}
	handler := &backlogHandlers{
		storage:         storage,
		reviewTrigger:   trigger,
		verifyGitHubRef: func(ctx context.Context, ref *githubpkg.ParsedGitHubRef) error { return nil },
		listItemSessionsFn: func(ctx context.Context, itemID string) ([]session.ItemSessionSummary, error) {
			return nil, injectedErr
		},
	}
	ctxWithUUID := WithSessionUUID(context.Background(), sessionUUID)

	req := makeToolReq(map[string]interface{}{
		"item_id":       item.ID,
		"duplicate_ref": "https://github.com/tstapler/stapler-squad/pull/272",
		"reason":        "duplicate",
	})

	result, err := handler.reportDuplicate(ctxWithUUID, req)
	require.NoError(t, err)
	tc, ok := result.Content[0].(mcpgo.TextContent)
	require.True(t, ok)
	assert.Contains(t, tc.Text, "next review pass", "a ListItemSessions failure must default to the conservative message, never claim live-reviewer visibility it can't confirm")
	assert.NotContains(t, tc.Text, "Reviewer notified")
	assert.Empty(t, trigger.calls, "a ListItemSessions failure must also skip the trigger call, not just the message")

	fetched, err := storage.GetBacklogItem(context.Background(), item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(session.BacklogStatusReview), fetched.Status, "the transition itself must still succeed despite the messaging-branch failure")
}

// --- FR7 gap-fill: append-not-overwrite VerificationNotes ---

// TestReportDuplicate_PreservesExistingVerificationNotes_WhenAppendingNewEntry
// verifies Task 3.3.2a's fix: a work ItemSession that already has
// VerificationNotes from an earlier request_review call (e.g. before a
// rework cycle) must not have that prior evidence silently discarded when
// report_duplicate persists its own entry.
func TestReportDuplicate_PreservesExistingVerificationNotes_WhenAppendingNewEntry(t *testing.T) {
	storage := newTestBacklogStorage(t)
	ctx := context.Background()

	item, sessionUUID := setupReportDuplicateFixture(t, storage, session.BacklogStatusInProgress)

	itemSess, err := storage.GetItemSessionBySessionAndItem(ctx, sessionUUID, item.ID)
	require.NoError(t, err)
	priorNotes := "ran `go test ./session/...` -> ok (41 tests)"
	require.NoError(t, storage.UpdateItemSessionVerificationNotes(ctx, itemSess.ID, priorNotes))

	handler := &backlogHandlers{
		storage:         storage,
		verifyGitHubRef: func(ctx context.Context, ref *githubpkg.ParsedGitHubRef) error { return nil },
	}
	ctxWithUUID := WithSessionUUID(ctx, sessionUUID)

	req := makeToolReq(map[string]interface{}{
		"item_id":       item.ID,
		"duplicate_ref": "https://github.com/tstapler/stapler-squad/pull/272",
		"reason":        "duplicate",
	})

	_, err = handler.reportDuplicate(ctxWithUUID, req)
	require.NoError(t, err)

	fetched, err := storage.GetItemSession(ctx, itemSess.ID)
	require.NoError(t, err)
	assert.Contains(t, fetched.VerificationNotes, priorNotes, "prior VerificationNotes must be preserved, not overwritten")
	assert.Contains(t, fetched.VerificationNotes, "duplicate_ref=https://github.com/tstapler/stapler-squad/pull/272")
}

// --- Story 4.2.6: sequential interaction with report_pr_created ---

// TestReportDuplicate_RejectsThirdCall_AfterSequentialReportPRCreatedThenReportDuplicate
// verifies report_pr_created and report_duplicate compose correctly across a
// real status change (review -> pr_pending -> review, matching
// SetBacklogItemPRAndTransition's actual precondition), and that a
// third, now-disallowed call is cleanly refused rather than silently
// misbehaving. This is sequential state-machine composition on a single
// goroutine, not a concurrency test — see
// TestRequestReview_ReportsDistinctMessage_WhenCASPreconditionFails for the
// genuinely-concurrent counterpart in this file.
func TestReportDuplicate_RejectsThirdCall_AfterSequentialReportPRCreatedThenReportDuplicate(t *testing.T) {
	storage := newTestBacklogStorage(t)
	// report_pr_created's precondition (SetBacklogItemPRAndTransition)
	// requires the source status to be "review" — seed there, matching
	// setupReportPRCreatedFixture's own convention.
	item, sessionUUID := setupReportDuplicateFixture(t, storage, session.BacklogStatusReview)

	handler := &backlogHandlers{
		storage:              storage,
		resolveSessionBranch: func(context.Context, string) (string, error) { return "backlog/ship-it", nil },
		verifyPRMatchesBranch: func(context.Context, string, string, int, string) (PRVerification, error) {
			return NewPRVerification(true, true, "backlog/ship-it", githubpkg.PRStateOpen, "tstapler"), nil
		},
		verifyGitHubRef: func(ctx context.Context, ref *githubpkg.ParsedGitHubRef) error { return nil },
	}
	ctxWithUUID := WithSessionUUID(context.Background(), sessionUUID)

	// Step 1: report_pr_created succeeds, review -> pr_pending.
	prReq := makeToolReq(map[string]interface{}{
		"item_id":   item.ID,
		"pr_url":    "https://github.com/tstapler/stapler-squad/pull/42",
		"pr_number": float64(42),
		"summary":   "Implemented the feature.",
	})
	prResult, err := handler.reportPRCreated(ctxWithUUID, prReq)
	require.NoError(t, err)
	prTC, ok := prResult.Content[0].(mcpgo.TextContent)
	require.True(t, ok)
	assert.Contains(t, prTC.Text, "pr_pending")

	fetchedAfterPR, err := storage.GetBacklogItem(context.Background(), item.ID)
	require.NoError(t, err)
	require.Equal(t, string(session.BacklogStatusPRPending), fetchedAfterPR.Status)
	require.Len(t, fetchedAfterPR.StatusEvents, 1)

	// Step 2: report_duplicate on the now-pr_pending item is a legitimate
	// re-request (pr_pending is in the whitelist), not a race — expected to
	// succeed, producing a second status event.
	dupReq := makeToolReq(map[string]interface{}{
		"item_id":       item.ID,
		"duplicate_ref": "https://github.com/tstapler/stapler-squad/pull/272",
		"reason":        "same fix already merged in PR #272",
	})
	dupResult, err := handler.reportDuplicate(ctxWithUUID, dupReq)
	require.NoError(t, err)
	dupTC, ok := dupResult.Content[0].(mcpgo.TextContent)
	require.True(t, ok)
	assert.Contains(t, dupTC.Text, "review")

	fetchedAfterDup, err := storage.GetBacklogItem(context.Background(), item.ID)
	require.NoError(t, err)
	require.Equal(t, string(session.BacklogStatusReview), fetchedAfterDup.Status)
	require.Len(t, fetchedAfterDup.StatusEvents, 2, "two legitimate, sequential transitions produce two status-event rows, not one")

	// Step 3: a second report_duplicate call now (item at review, not in the
	// whitelist) must be cleanly refused, not silently misbehave.
	handler.verifyGitHubRef = failOnCallVerifyGitHubRef(t)
	secondDupReq := makeToolReq(map[string]interface{}{
		"item_id":       item.ID,
		"duplicate_ref": "https://github.com/tstapler/stapler-squad/pull/300",
		"reason":        "a different duplicate candidate",
	})
	loserResult, err := handler.reportDuplicate(ctxWithUUID, secondDupReq)
	require.NoError(t, err)

	m := parseResult(t, loserResult)
	require.False(t, m["success"].(bool))
	errObj := m["error"].(map[string]interface{})
	assert.Equal(t, ErrInvalidArgument, errObj["code"])
	assert.Contains(t, errObj["message"], "review")

	fetchedFinal, err := storage.GetBacklogItem(context.Background(), item.ID)
	require.NoError(t, err)
	require.Len(t, fetchedFinal.StatusEvents, 2, "the refused third call must not add a status event")
}

// --- Epic 3.4/4.3b: MCP registration description content (FR10) ---

// TestRegisterBacklogTools_ReportDuplicate_DescribesRetryGuidance mirrors the
// existing in-repo precedent
// TestRegisterBacklogTools_RequestReview_DescribesAlreadyImplementedCitationRequirement:
// reads the source file directly and asserts substring presence, rather than
// instantiating the MCP server and introspecting a live tool registration.
func TestRegisterBacklogTools_ReportDuplicate_DescribesRetryGuidance(t *testing.T) {
	data, err := os.ReadFile("tools_backlog.go")
	require.NoError(t, err, "read tools_backlog.go")

	content := string(data)
	assert.Contains(t, content, "report_duplicate",
		"report_duplicate must be registered")
	assert.Contains(t, content, "INTERNAL_ERROR, this is transient — retry the call",
		"report_duplicate's description must give explicit retry guidance for INTERNAL_ERROR results (FR10)")
	assert.Contains(t, content, "no configured GitHub credentials",
		"report_duplicate's description must escalate the no-credentials case as non-retryable, distinct from ordinary retry guidance")
}

// --- BUG-047: submit_review_verdict eager review->in_progress transition ---

// fakeAutoReopenSpawner is a minimal session.AutoReopenSpawner test double —
// records every itemID it's called with, so tests can assert the eager
// transition path was (or wasn't) invoked without needing a real
// *services.BacklogService (heavy setup: SessionService, worktrees, etc.).
type fakeAutoReopenSpawner struct {
	mu    sync.Mutex
	calls []string
	err   error
}

func (f *fakeAutoReopenSpawner) AutoReopenAfterFailedReview(ctx context.Context, itemID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, itemID)
	return f.err
}

func (f *fakeAutoReopenSpawner) calledWith() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.calls))
	copy(out, f.calls)
	return out
}

// setupReviewSession creates a review-status BacklogItem with a linked
// review-role ItemSession for sessionUUID, mirroring the setup shape used by
// TestRequestReview_TransitionsItemToReview for the work role.
func setupReviewSession(t *testing.T, storage *session.Storage) (itemID, sessionUUID string) {
	t.Helper()
	ctx := context.Background()

	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "Review me",
		Status: string(session.BacklogStatusReview),
	})
	require.NoError(t, err)

	sessUUID := uuid.New().String()
	_, err = storage.CreateItemSession(ctx, session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: sessUUID,
		SessionRole: session.SessionRoleReview,
	})
	require.NoError(t, err)

	return item.ID, sessUUID
}

func verdictsArg(outcome string) []interface{} {
	return []interface{}{
		map[string]interface{}{
			"criterion_index": float64(0),
			"outcome":         outcome,
			"evidence":        "clear evidence from the diff",
		},
	}
}

// setupReviewSessionWithCompletedWorkSession is setupReviewSession plus a
// real on-disk git repo (item.RepoPath) and a completed work-role
// ItemSession with a valid Base/LastCommitSha range — the fixture shape
// Storage.ComputeCurrentDiffHash needs to actually resolve a hash, rather
// than best-effort-returning "".
func setupReviewSessionWithCompletedWorkSession(t *testing.T, storage *session.Storage) (itemID, sessionUUID, repoPath string) {
	t.Helper()
	ctx := context.Background()

	repoPath = initGitRepo(t)
	baseSHA := strings.TrimSpace(runGit(t, repoPath, "rev-parse", "HEAD"))
	writeFile(t, repoPath, "foo.go", "package foo\n")
	runGit(t, repoPath, "add", "foo.go")
	runGit(t, repoPath, "commit", "-m", "work session change")
	headSHA := strings.TrimSpace(runGit(t, repoPath, "rev-parse", "HEAD"))

	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:    "Review me",
		RepoPath: repoPath,
		Status:   string(session.BacklogStatusReview),
	})
	require.NoError(t, err)

	ws, err := storage.CreateItemSession(ctx, session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: uuid.New().String(),
		SessionRole: session.SessionRoleWork,
	})
	require.NoError(t, err)
	require.NoError(t, storage.SetItemSessionBaseCommit(ctx, ws.ID, baseSHA))
	require.NoError(t, storage.UpdateItemSessionGitActivity(ctx, ws.ID, headSHA, "work session change", time.Now(), 1))
	require.NoError(t, storage.UpdateItemSessionEnded(ctx, ws.ID, time.Now()))

	sessUUID := uuid.New().String()
	_, err = storage.CreateItemSession(ctx, session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: sessUUID,
		SessionRole: session.SessionRoleReview,
	})
	require.NoError(t, err)

	return item.ID, sessUUID, repoPath
}

// TestSubmitReviewVerdict_should_persistDiffHash_When_CompletedWorkSessionExists
// is the write-site regression test for Story 3.2.1: submit_review_verdict
// (the MCP tool the interactive review agent actually calls) must stamp
// DiffHash on the saved verdict via Storage.ComputeCurrentDiffHash, not
// leave it empty. Without this test, a typo'd itemID/ctx or an accidentally
// dropped field at this specific call site would compile and pass every
// other test while silently making IsFlakyVerdictFlipFlop permanently see
// an empty hash (never a match) for every verdict submitted through this
// path — the review agent's own submission path, the one most commonly
// exercised in production.
func TestSubmitReviewVerdict_should_persistDiffHash_When_CompletedWorkSessionExists(t *testing.T) {
	storage := newTestBacklogStorage(t)
	itemID, sessUUID, _ := setupReviewSessionWithCompletedWorkSession(t, storage)

	handler := &backlogHandlers{storage: storage}
	ctx := WithSessionUUID(context.Background(), sessUUID)

	req := makeToolReq(map[string]interface{}{
		"item_id":  itemID,
		"summary":  "Looks good.",
		"verdicts": verdictsArg("PASS"),
	})

	_, err := handler.submitReviewVerdict(ctx, req)
	require.NoError(t, err)

	recent, err := storage.GetRecentReviewVerdictSummaries(ctx, itemID, 1)
	require.NoError(t, err)
	require.Len(t, recent, 1)
	assert.NotEmpty(t, recent[0].DiffHash, "submit_review_verdict must stamp DiffHash via ComputeCurrentDiffHash, not leave it empty")
}

// TestSubmitReviewVerdict_should_InvokeAutoReopener_When_OutcomeIsFail is
// acceptance criterion 0/2's core regression guard: a FAIL verdict must drive
// the review->in_progress transition (via AutoReopenSpawner, which owns
// spawning a new work session when none is active) immediately, in the same
// call, rather than waiting for the review session to exit or the sweep to fire.
func TestSubmitReviewVerdict_should_InvokeAutoReopener_When_OutcomeIsFail(t *testing.T) {
	storage := newTestBacklogStorage(t)
	itemID, sessUUID := setupReviewSession(t, storage)

	reopener := &fakeAutoReopenSpawner{}
	handler := &backlogHandlers{storage: storage, autoReopener: reopener}
	ctx := WithSessionUUID(context.Background(), sessUUID)

	req := makeToolReq(map[string]interface{}{
		"item_id":  itemID,
		"summary":  "Blocked by security check.",
		"verdicts": verdictsArg("FAIL"),
	})

	_, err := handler.submitReviewVerdict(ctx, req)
	require.NoError(t, err)

	assert.Equal(t, []string{itemID}, reopener.calledWith(),
		"a FAIL verdict must eagerly call AutoReopenAfterFailedReview exactly once")
}

// TestSubmitReviewVerdict_should_InvokeAutoReopener_When_OutcomeIsPartialOrUnverifiable
// extends the FAIL coverage above to the other two reject outcomes.
func TestSubmitReviewVerdict_should_InvokeAutoReopener_When_OutcomeIsPartialOrUnverifiable(t *testing.T) {
	for _, outcome := range []string{"PARTIAL", "UNVERIFIABLE"} {
		t.Run(outcome, func(t *testing.T) {
			storage := newTestBacklogStorage(t)
			itemID, sessUUID := setupReviewSession(t, storage)

			reopener := &fakeAutoReopenSpawner{}
			handler := &backlogHandlers{storage: storage, autoReopener: reopener}
			ctx := WithSessionUUID(context.Background(), sessUUID)

			req := makeToolReq(map[string]interface{}{
				"item_id":  itemID,
				"summary":  "Needs more work.",
				"verdicts": verdictsArg(outcome),
			})

			_, err := handler.submitReviewVerdict(ctx, req)
			require.NoError(t, err)

			assert.Equal(t, []string{itemID}, reopener.calledWith())
		})
	}
}

// TestSubmitReviewVerdict_should_NotInvokeAutoReopener_When_OutcomeIsPass is
// acceptance criterion 5: PASS stays deferred to handleReviewSessionExited
// (which pushes the branch and creates the PR) — the eager path must not
// fire for it.
func TestSubmitReviewVerdict_should_NotInvokeAutoReopener_When_OutcomeIsPass(t *testing.T) {
	storage := newTestBacklogStorage(t)
	itemID, sessUUID := setupReviewSession(t, storage)

	reopener := &fakeAutoReopenSpawner{}
	handler := &backlogHandlers{storage: storage, autoReopener: reopener}
	ctx := WithSessionUUID(context.Background(), sessUUID)

	req := makeToolReq(map[string]interface{}{
		"item_id":  itemID,
		"summary":  "All good.",
		"verdicts": verdictsArg("PASS"),
	})

	_, err := handler.submitReviewVerdict(ctx, req)
	require.NoError(t, err)

	assert.Empty(t, reopener.calledWith(), "a PASS verdict must not trigger the eager auto-reopen path")

	fetched, err := storage.GetBacklogItem(context.Background(), itemID)
	require.NoError(t, err)
	assert.Equal(t, string(session.BacklogStatusReview), fetched.Status,
		"PASS must leave the item in review — only handleReviewSessionExited transitions it, on session exit")
}

// TestSubmitReviewVerdict_should_NotError_When_AutoReopenerNil verifies the
// eager transition is optional/nil-safe (the stdio MCP fallback path has no
// BacklogService to wire — see RunServer's doc comment) rather than a hard
// dependency that would break submit_review_verdict wherever it isn't wired.
func TestSubmitReviewVerdict_should_NotError_When_AutoReopenerNil(t *testing.T) {
	storage := newTestBacklogStorage(t)
	itemID, sessUUID := setupReviewSession(t, storage)

	handler := &backlogHandlers{storage: storage} // autoReopener left nil
	ctx := WithSessionUUID(context.Background(), sessUUID)

	req := makeToolReq(map[string]interface{}{
		"item_id":  itemID,
		"summary":  "Blocked.",
		"verdicts": verdictsArg("FAIL"),
	})

	result, err := handler.submitReviewVerdict(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, result)

	fetched, err := storage.GetBacklogItem(context.Background(), itemID)
	require.NoError(t, err)
	assert.Equal(t, string(session.BacklogStatusReview), fetched.Status,
		"with autoReopener nil, the eager transition must be skipped entirely — the item stays in review for the session-exit/sweep paths to handle, not silently left in some other state")
}

// TestSubmitReviewVerdict_should_NotSurfaceError_When_AutoReopenerFails
// verifies acceptance criterion 1's CAS-harmless-failure contract from the
// caller's side: if AutoReopenAfterFailedReview's own CAS precondition fails
// (e.g. handleReviewSessionExited already raced ahead of it), the error is
// swallowed (logged only) — the review session's submit_review_verdict call
// must not itself fail or surface that as an error to the caller.
func TestSubmitReviewVerdict_should_NotSurfaceError_When_AutoReopenerFails(t *testing.T) {
	storage := newTestBacklogStorage(t)
	itemID, sessUUID := setupReviewSession(t, storage)

	reopener := &fakeAutoReopenSpawner{err: errors.New("concurrent modification detected: expected status review, got in_progress")}
	handler := &backlogHandlers{storage: storage, autoReopener: reopener}
	ctx := WithSessionUUID(context.Background(), sessUUID)

	req := makeToolReq(map[string]interface{}{
		"item_id":  itemID,
		"summary":  "Blocked.",
		"verdicts": verdictsArg("FAIL"),
	})

	result, err := handler.submitReviewVerdict(ctx, req)
	require.NoError(t, err)
	require.Len(t, result.Content, 1)
	tc, ok := result.Content[0].(mcpgo.TextContent)
	require.True(t, ok)
	assert.Contains(t, tc.Text, "Review verdict submitted",
		"AutoReopenAfterFailedReview's CAS failure must not surface as a tool error — the normal success text must still be returned")
	assert.Equal(t, []string{itemID}, reopener.calledWith())
}

// --- wait_for_backlog_event ---

// waitCallOutcome carries both the result and error of a
// waitForBacklogEvent call made on a spawned goroutine back to the main
// test goroutine. require.NoError (and other testify t.FailNow()-triggering
// assertions) must only be called from the goroutine running the test, per
// the testing.T contract — calling it from a spawned goroutine just halts
// that goroutine via runtime.Goexit() without failing the test or unblocking
// a channel receive, which would make a real failure hang until the test's
// own outer timeout instead of reporting. So the goroutine sends both values
// through this struct and the main goroutine asserts after receiving.
type waitCallOutcome struct {
	res *mcpgo.CallToolResult
	err error
}

func decodeWaitResult(t *testing.T, res *mcpgo.CallToolResult) WaitForBacklogEventResult {
	t.Helper()
	require.Len(t, res.Content, 1)
	tc, ok := res.Content[0].(mcpgo.TextContent)
	require.True(t, ok)
	var out WaitForBacklogEventResult
	require.NoError(t, json.Unmarshal([]byte(tc.Text), &out))
	return out
}

func TestWaitForBacklogEvent_ReturnsErrorWhenEventBusNil(t *testing.T) {
	storage := newTestBacklogStorage(t)
	ctx := context.Background()

	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "No event bus",
		Status: string(session.BacklogStatusInProgress),
	})
	require.NoError(t, err)

	handler := &backlogHandlers{storage: storage, eventBus: nil}
	req := makeToolReq(map[string]interface{}{"item_id": item.ID})

	result, err := handler.waitForBacklogEvent(ctx, req)
	require.NoError(t, err)

	m := parseResult(t, result)
	require.False(t, m["success"].(bool))
	errObj, ok := m["error"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, ErrEventStreamUnavailable, errObj["code"])
}

func TestWaitForBacklogEvent_ReturnsErrorForInvalidItemID(t *testing.T) {
	storage := newTestBacklogStorage(t)
	handler := &backlogHandlers{storage: storage, eventBus: events.NewEventBus(32)}

	req := makeToolReq(map[string]interface{}{"item_id": "not-a-uuid"})
	result, err := handler.waitForBacklogEvent(context.Background(), req)
	require.NoError(t, err)

	m := parseResult(t, result)
	require.False(t, m["success"].(bool))
	errObj, ok := m["error"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, ErrInvalidArgument, errObj["code"])
}

func TestWaitForBacklogEvent_ReturnsItemNotFoundForUnknownItem(t *testing.T) {
	storage := newTestBacklogStorage(t)
	handler := &backlogHandlers{storage: storage, eventBus: events.NewEventBus(32)}

	req := makeToolReq(map[string]interface{}{"item_id": "00000000-0000-0000-0000-000000000099"})
	result, err := handler.waitForBacklogEvent(context.Background(), req)
	require.NoError(t, err)

	m := parseResult(t, result)
	require.False(t, m["success"].(bool))
	errObj, ok := m["error"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, ErrItemNotFound, errObj["code"])
}

func TestWaitForBacklogEvent_ReturnsImmediatelyWhenVerdictAlreadyRecorded(t *testing.T) {
	storage := newTestBacklogStorage(t)
	ctx := context.Background()

	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "Already reviewed",
		Status: string(session.BacklogStatusReview),
	})
	require.NoError(t, err)

	_, err = storage.CreateItemSessionWithVerdict(ctx, session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: uuid.New().String(),
		SessionRole: session.SessionRoleReview,
	}, session.ReviewVerdictData{
		OverallOutcome: session.ReviewOutcomePass,
		Summary:        "lgtm",
	})
	require.NoError(t, err)

	handler := &backlogHandlers{storage: storage, eventBus: events.NewEventBus(32)}
	req := makeToolReq(map[string]interface{}{"item_id": item.ID, "event_type": "verdict_recorded", "timeout_seconds": float64(30)})

	start := time.Now()
	result, err := handler.waitForBacklogEvent(ctx, req)
	elapsed := time.Since(start)
	require.NoError(t, err)

	require.Less(t, elapsed, 5*time.Second, "must return immediately from the precheck, not wait out timeout_seconds")
	out := decodeWaitResult(t, result)
	require.True(t, out.Success)
	require.True(t, out.EventReceived)
	require.True(t, out.FromCurrentState)
	require.Equal(t, "verdict_recorded", out.EventKind)
	require.Equal(t, string(session.ReviewOutcomePass), out.VerdictOutcome)
}

func TestWaitForBacklogEvent_ReturnsImmediatelyWhenItemAlreadyArchived(t *testing.T) {
	storage := newTestBacklogStorage(t)
	ctx := context.Background()

	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "Already archived",
		Status: string(session.BacklogStatusDone),
	})
	require.NoError(t, err)
	_, err = storage.ArchiveBacklogItem(ctx, item.ID)
	require.NoError(t, err)

	handler := &backlogHandlers{storage: storage, eventBus: events.NewEventBus(32)}
	req := makeToolReq(map[string]interface{}{"item_id": item.ID, "event_type": "item_archived", "timeout_seconds": float64(30)})

	start := time.Now()
	result, err := handler.waitForBacklogEvent(ctx, req)
	elapsed := time.Since(start)
	require.NoError(t, err)

	require.Less(t, elapsed, 5*time.Second)
	out := decodeWaitResult(t, result)
	require.True(t, out.EventReceived)
	require.True(t, out.FromCurrentState)
	require.Equal(t, "item_archived", out.EventKind)
	require.True(t, out.IsTerminal)
}

// waitSubscriberCount polls until the eventBus reports the expected
// subscriber count, avoiding a sleep-based race — mirrors the fix-flaky-
// tests rule's "prefer a synchronization primitive" guidance.
func waitSubscriberCount(t *testing.T, bus *events.EventBus, want int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if bus.SubscriberCount() == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for SubscriberCount() == %d (got %d)", want, bus.SubscriberCount())
}

func TestWaitForBacklogEvent_ReturnsMatchedEventOnLiveVerdict(t *testing.T) {
	storage := newTestBacklogStorage(t)
	ctx := context.Background()
	bus := events.NewEventBus(32)

	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "Awaiting verdict",
		Status: string(session.BacklogStatusReview),
	})
	require.NoError(t, err)

	handler := &backlogHandlers{storage: storage, eventBus: bus}
	req := makeToolReq(map[string]interface{}{"item_id": item.ID, "timeout_seconds": float64(5)})

	resultCh := make(chan waitCallOutcome, 1)
	go func() {
		res, callErr := handler.waitForBacklogEvent(ctx, req)
		resultCh <- waitCallOutcome{res: res, err: callErr}
	}()

	waitSubscriberCount(t, bus, 1)
	bus.Publish(&events.Event{
		Type: events.EventBacklogItemChanged,
		BacklogItemPayload: &events.BacklogItemEventPayload{
			Kind: events.BacklogChangeVerdictRecorded,
			Item: &session.BacklogItemData{ID: item.ID, Status: string(session.BacklogStatusReview)},
			Verdict: &session.ReviewVerdictData{
				OverallOutcome: session.ReviewOutcomePass,
				Summary:        "ship it",
			},
		},
	})

	select {
	case outcome := <-resultCh:
		require.NoError(t, outcome.err)
		out := decodeWaitResult(t, outcome.res)
		require.True(t, out.EventReceived)
		require.False(t, out.FromCurrentState)
		require.Equal(t, "verdict_recorded", out.EventKind)
		require.Equal(t, string(session.ReviewOutcomePass), out.VerdictOutcome)
		require.Equal(t, "ship it", out.VerdictSummary)
	case <-time.After(3 * time.Second):
		t.Fatal("waitForBacklogEvent did not return after matching event was published")
	}
}

func TestWaitForBacklogEvent_FiltersByEventType(t *testing.T) {
	storage := newTestBacklogStorage(t)
	ctx := context.Background()
	bus := events.NewEventBus(32)

	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "Filtered wait",
		Status: string(session.BacklogStatusReview),
	})
	require.NoError(t, err)
	otherItem, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "Different item",
		Status: string(session.BacklogStatusReview),
	})
	require.NoError(t, err)

	handler := &backlogHandlers{storage: storage, eventBus: bus}
	req := makeToolReq(map[string]interface{}{
		"item_id":         item.ID,
		"event_type":      "verdict_recorded",
		"timeout_seconds": float64(5),
	})

	resultCh := make(chan waitCallOutcome, 1)
	go func() {
		res, callErr := handler.waitForBacklogEvent(ctx, req)
		resultCh <- waitCallOutcome{res: res, err: callErr}
	}()

	waitSubscriberCount(t, bus, 1)

	// Wrong item, status_transition kind — must be ignored.
	bus.Publish(&events.Event{
		Type: events.EventBacklogItemChanged,
		BacklogItemPayload: &events.BacklogItemEventPayload{
			Kind: events.BacklogChangeStatusTransition,
			Item: &session.BacklogItemData{ID: otherItem.ID, Status: string(session.BacklogStatusInProgress)},
		},
	})
	// Right item, wrong kind (status_transition, filter wants verdict_recorded) — must be ignored.
	bus.Publish(&events.Event{
		Type: events.EventBacklogItemChanged,
		BacklogItemPayload: &events.BacklogItemEventPayload{
			Kind:      events.BacklogChangeStatusTransition,
			Item:      &session.BacklogItemData{ID: item.ID, Status: string(session.BacklogStatusInProgress)},
			OldStatus: string(session.BacklogStatusReview),
			NewStatus: string(session.BacklogStatusInProgress),
		},
	})
	// Right item, right kind — must match.
	bus.Publish(&events.Event{
		Type: events.EventBacklogItemChanged,
		BacklogItemPayload: &events.BacklogItemEventPayload{
			Kind: events.BacklogChangeVerdictRecorded,
			Item: &session.BacklogItemData{ID: item.ID, Status: string(session.BacklogStatusReview)},
			Verdict: &session.ReviewVerdictData{
				OverallOutcome: session.ReviewOutcomePass,
				Summary:        "matched",
			},
		},
	})

	select {
	case outcome := <-resultCh:
		require.NoError(t, outcome.err)
		out := decodeWaitResult(t, outcome.res)
		require.True(t, out.EventReceived)
		require.Equal(t, "verdict_recorded", out.EventKind)
		require.Equal(t, "matched", out.VerdictSummary)
	case <-time.After(3 * time.Second):
		t.Fatal("waitForBacklogEvent did not return the correctly-filtered event")
	}
}

func TestWaitForBacklogEvent_TimesOutWithNoEvent(t *testing.T) {
	storage := newTestBacklogStorage(t)
	ctx := context.Background()

	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "Never verdicted",
		Status: string(session.BacklogStatusReview),
	})
	require.NoError(t, err)

	handler := &backlogHandlers{storage: storage, eventBus: events.NewEventBus(32)}
	req := makeToolReq(map[string]interface{}{"item_id": item.ID, "timeout_seconds": float64(1)})

	start := time.Now()
	result, err := handler.waitForBacklogEvent(ctx, req)
	elapsed := time.Since(start)
	require.NoError(t, err)

	require.GreaterOrEqual(t, elapsed, 1*time.Second, "must actually wait, not return an instant false-positive timeout")
	out := decodeWaitResult(t, result)
	require.True(t, out.Success)
	require.False(t, out.EventReceived)
	require.NotNil(t, out.Error)
	require.Equal(t, "WAIT_TIMEOUT", out.Error.Code)
}

func TestWaitForBacklogEvent_ConcurrentWaitersBothReceiveEvent(t *testing.T) {
	storage := newTestBacklogStorage(t)
	ctx := context.Background()
	bus := events.NewEventBus(32)

	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "Two waiters",
		Status: string(session.BacklogStatusReview),
	})
	require.NoError(t, err)

	handler := &backlogHandlers{storage: storage, eventBus: bus}
	req := makeToolReq(map[string]interface{}{"item_id": item.ID, "timeout_seconds": float64(5)})

	results := make(chan waitCallOutcome, 2)
	for i := 0; i < 2; i++ {
		go func() {
			res, callErr := handler.waitForBacklogEvent(ctx, req)
			results <- waitCallOutcome{res: res, err: callErr}
		}()
	}

	waitSubscriberCount(t, bus, 2)
	bus.Publish(&events.Event{
		Type: events.EventBacklogItemChanged,
		BacklogItemPayload: &events.BacklogItemEventPayload{
			Kind: events.BacklogChangeVerdictRecorded,
			Item: &session.BacklogItemData{ID: item.ID, Status: string(session.BacklogStatusReview)},
			Verdict: &session.ReviewVerdictData{
				OverallOutcome: session.ReviewOutcomePass,
				Summary:        "both should see this",
			},
		},
	})

	var got []WaitForBacklogEventResult
	for i := 0; i < 2; i++ {
		select {
		case outcome := <-results:
			require.NoError(t, outcome.err)
			got = append(got, decodeWaitResult(t, outcome.res))
		case <-time.After(3 * time.Second):
			t.Fatal("timed out waiting for both concurrent waiters to return")
		}
	}

	require.Len(t, got, 2)
	for _, r := range got {
		require.True(t, r.EventReceived)
		require.Equal(t, string(session.ReviewOutcomePass), r.VerdictOutcome)
		require.Equal(t, "both should see this", r.VerdictSummary)
	}
}

func TestWaitForBacklogEvent_NoGoroutineLeak(t *testing.T) {
	storage := newTestBacklogStorage(t)
	ctx := context.Background()
	bus := events.NewEventBus(32)
	handler := &backlogHandlers{storage: storage, eventBus: bus}

	// Baseline is captured after storage setup (not before) — opening the
	// sqlite-backed Storage starts its own long-lived database/sql connection
	// pool goroutines that are test-infra noise, not something
	// waitForBacklogEvent itself could leak.
	baseline := goleak.IgnoreCurrent()

	// Timeout-exit path.
	timeoutItem, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "Leak check: timeout",
		Status: string(session.BacklogStatusReview),
	})
	require.NoError(t, err)
	_, err = handler.waitForBacklogEvent(ctx, makeToolReq(map[string]interface{}{
		"item_id":         timeoutItem.ID,
		"timeout_seconds": float64(1),
	}))
	require.NoError(t, err)

	// Matched-event-exit path.
	matchedItem, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "Leak check: matched",
		Status: string(session.BacklogStatusReview),
	})
	require.NoError(t, err)
	resultCh := make(chan waitCallOutcome, 1)
	go func() {
		res, callErr := handler.waitForBacklogEvent(ctx, makeToolReq(map[string]interface{}{
			"item_id":         matchedItem.ID,
			"timeout_seconds": float64(5),
		}))
		resultCh <- waitCallOutcome{res: res, err: callErr}
	}()
	waitSubscriberCount(t, bus, 1)
	bus.Publish(&events.Event{
		Type: events.EventBacklogItemChanged,
		BacklogItemPayload: &events.BacklogItemEventPayload{
			Kind: events.BacklogChangeVerdictRecorded,
			Item: &session.BacklogItemData{ID: matchedItem.ID, Status: string(session.BacklogStatusReview)},
			Verdict: &session.ReviewVerdictData{
				OverallOutcome: session.ReviewOutcomePass,
			},
		},
	})
	select {
	case outcome := <-resultCh:
		require.NoError(t, outcome.err)
	case <-time.After(3 * time.Second):
		t.Fatal("matched-path call did not return")
	}

	require.Eventually(t, func() bool { return bus.SubscriberCount() == 0 }, 3*time.Second, 10*time.Millisecond,
		"both calls must Unsubscribe on exit")
	goleak.VerifyNone(t, baseline)
}

// TestWaitForBacklogEvent_SubscribeBeforeReadClosesRace proves the
// subscribe-before-read ordering: an event published inside
// withTestAfterWaitSubscribeHook's hook (after Subscribe returns, before the
// precheck read) is not missed. The hook is scoped to this test's own ctx via
// context.WithValue, so it can never collide with a t.Parallel() sibling's
// call into the same code path.
func TestWaitForBacklogEvent_SubscribeBeforeReadClosesRace(t *testing.T) {
	storage := newTestBacklogStorage(t)
	bus := events.NewEventBus(32)

	item, err := storage.CreateBacklogItem(context.Background(), session.BacklogItemData{
		Title:  "Race window",
		Status: string(session.BacklogStatusReview),
	})
	require.NoError(t, err)

	ctx := withTestAfterWaitSubscribeHook(context.Background(), func() {
		bus.Publish(&events.Event{
			Type: events.EventBacklogItemChanged,
			BacklogItemPayload: &events.BacklogItemEventPayload{
				Kind: events.BacklogChangeVerdictRecorded,
				Item: &session.BacklogItemData{ID: item.ID, Status: string(session.BacklogStatusReview)},
				Verdict: &session.ReviewVerdictData{
					OverallOutcome: session.ReviewOutcomePass,
					Summary:        "landed in the race window",
				},
			},
		})
	})

	handler := &backlogHandlers{storage: storage, eventBus: bus}
	req := makeToolReq(map[string]interface{}{"item_id": item.ID, "timeout_seconds": float64(2)})

	result, err := handler.waitForBacklogEvent(ctx, req)
	require.NoError(t, err)

	out := decodeWaitResult(t, result)
	require.True(t, out.EventReceived, "an event published inside the subscribe hook must not be missed")
	require.Equal(t, "landed in the race window", out.VerdictSummary)
}

// TestWaitForBacklogEvent_ReturnsWhenEventBusClosedWhileWaiting covers the
// select loop's `case evt, ok := <-eventCh: if !ok { ... }` branch — the
// EventBus being closed (e.g. a shutdown path) while a wait is in flight.
// Previously untested (CRITICAL finding from code review): a bug here would
// only surface as every in-flight wait hanging until its own
// timeout_seconds elapses instead of returning promptly.
func TestWaitForBacklogEvent_ReturnsWhenEventBusClosedWhileWaiting(t *testing.T) {
	storage := newTestBacklogStorage(t)
	ctx := context.Background()
	bus := events.NewEventBus(32)

	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "Bus closes mid-wait",
		Status: string(session.BacklogStatusReview),
	})
	require.NoError(t, err)

	handler := &backlogHandlers{storage: storage, eventBus: bus}
	req := makeToolReq(map[string]interface{}{"item_id": item.ID, "timeout_seconds": float64(5)})

	resultCh := make(chan waitCallOutcome, 1)
	start := time.Now()
	go func() {
		res, callErr := handler.waitForBacklogEvent(ctx, req)
		resultCh <- waitCallOutcome{res: res, err: callErr}
	}()

	waitSubscriberCount(t, bus, 1)
	bus.Close()

	select {
	case outcome := <-resultCh:
		elapsed := time.Since(start)
		require.NoError(t, outcome.err)
		require.Less(t, elapsed, 5*time.Second, "must return promptly on bus closure, not wait out the full timeout")
		out := decodeWaitResult(t, outcome.res)
		require.False(t, out.EventReceived)
		require.NotNil(t, out.Error)
	case <-time.After(3 * time.Second):
		t.Fatal("waitForBacklogEvent did not return after the EventBus was closed")
	}
}

// TestBuildMatchedWaitResult_MapsAllEventKinds is a direct table-driven unit
// test of buildMatchedWaitResult (a pure function) covering the
// item_updated/triage_progress_updated -> UpdatedFields mapping, the
// item_removed -> RemovedReason/IsTerminal mapping, and session_attached —
// none of which had any prior coverage.
func TestBuildMatchedWaitResult_MapsAllEventKinds(t *testing.T) {
	const itemID = "item-under-test"

	tests := []struct {
		name    string
		payload *events.BacklogItemEventPayload
		check   func(t *testing.T, res WaitForBacklogEventResult)
	}{
		{
			name: "item_updated populates UpdatedFields",
			payload: &events.BacklogItemEventPayload{
				Kind:          events.BacklogChangeItemUpdated,
				Item:          &session.BacklogItemData{ID: itemID, Status: string(session.BacklogStatusInProgress)},
				UpdatedFields: []string{"title", "description"},
			},
			check: func(t *testing.T, res WaitForBacklogEventResult) {
				require.Equal(t, "item_updated", res.EventKind)
				require.Equal(t, []string{"title", "description"}, res.UpdatedFields)
				require.False(t, res.IsTerminal)
			},
		},
		{
			name: "triage_progress_updated also populates UpdatedFields",
			payload: &events.BacklogItemEventPayload{
				Kind:          events.BacklogChangeTriageProgressUpdated,
				Item:          &session.BacklogItemData{ID: itemID, Status: string(session.BacklogStatusInProgress)},
				UpdatedFields: []string{"triage_notes"},
			},
			check: func(t *testing.T, res WaitForBacklogEventResult) {
				require.Equal(t, "item_updated", res.EventKind)
				require.Equal(t, []string{"triage_notes"}, res.UpdatedFields)
			},
		},
		{
			name: "item_removed populates RemovedReason and sets IsTerminal",
			payload: &events.BacklogItemEventPayload{
				Kind:          events.BacklogChangeItemRemoved,
				Item:          &session.BacklogItemData{ID: itemID, Status: string(session.BacklogStatusInProgress)},
				RemovedReason: "duplicate of #123",
			},
			check: func(t *testing.T, res WaitForBacklogEventResult) {
				require.Equal(t, "item_removed", res.EventKind)
				require.Equal(t, "duplicate of #123", res.RemovedReason)
				require.True(t, res.IsTerminal)
			},
		},
		{
			name: "session_attached",
			payload: &events.BacklogItemEventPayload{
				Kind:      events.BacklogChangeSessionAttached,
				Item:      &session.BacklogItemData{ID: itemID, Status: string(session.BacklogStatusInProgress)},
				SessionID: "session-abc",
			},
			check: func(t *testing.T, res WaitForBacklogEventResult) {
				require.Equal(t, "session_attached", res.EventKind)
				require.Equal(t, itemID, res.ItemID)
				require.Equal(t, string(session.BacklogStatusInProgress), res.Status)
				require.False(t, res.IsTerminal)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res := buildMatchedWaitResult(itemID, tc.payload)
			require.True(t, res.Success)
			require.True(t, res.EventReceived)
			require.Equal(t, itemID, res.ItemID)
			tc.check(t, res)
		})
	}
}

// --- list_backlog_items ---

// newTestListBacklogHandlers wires a *backlogHandlers with a real
// *services.BacklogService, mirroring newTestWorkflowHandlers' shape for the
// workflow tools — see server/mcp/tools_workflow_test.go.
func newTestListBacklogHandlers(t *testing.T) *backlogHandlers {
	t.Helper()
	storage := newTestBacklogStorage(t)
	svc := services.NewBacklogService(storage, nil, nil, nil, nil, nil)
	return &backlogHandlers{storage: storage, backlogSvc: svc}
}

func createBacklogItemWithStatusAndPriority(t *testing.T, storage *session.Storage, title, status string, priority int) *session.BacklogItemData {
	t.Helper()
	item, err := storage.CreateBacklogItem(context.Background(), session.BacklogItemData{
		Title:    title,
		Status:   status,
		Priority: priority,
	})
	require.NoError(t, err)
	return item
}

func TestListBacklogItems_ReturnsFilteredItems_When_StatusFilterApplied(t *testing.T) {
	h := newTestListBacklogHandlers(t)
	createBacklogItemWithStatusAndPriority(t, h.storage, "Ready 1", string(session.BacklogStatusReady), 3)
	createBacklogItemWithStatusAndPriority(t, h.storage, "In progress 1", string(session.BacklogStatusInProgress), 3)
	createBacklogItemWithStatusAndPriority(t, h.storage, "Done 1", string(session.BacklogStatusDone), 3)

	res, err := h.listBacklogItems(context.Background(), makeToolReq(map[string]interface{}{
		"status": []interface{}{"ready", "in_progress"},
	}))
	require.NoError(t, err)
	out := parseResult(t, res)
	require.True(t, out["success"].(bool))
	items := out["items"].([]interface{})
	require.Len(t, items, 2)
	for _, raw := range items {
		item := raw.(map[string]interface{})
		require.NotEqual(t, "done", item["status"])
	}
}

func TestListBacklogItems_ReturnsInvalidArgument_When_StatusValueUnknown(t *testing.T) {
	h := newTestListBacklogHandlers(t)

	res, err := h.listBacklogItems(context.Background(), makeToolReq(map[string]interface{}{
		"status": []interface{}{"readdy"},
	}))
	require.NoError(t, err)
	out := parseResult(t, res)
	require.False(t, out["success"].(bool))
	require.Equal(t, ErrInvalidArgument, out["error"].(map[string]interface{})["code"])
}

func TestListBacklogItems_ReturnsInvalidArgument_When_PriorityOutOfRange(t *testing.T) {
	h := newTestListBacklogHandlers(t)

	for _, bad := range []float64{0, 99} {
		t.Run(fmt.Sprintf("priority=%v", bad), func(t *testing.T) {
			res, err := h.listBacklogItems(context.Background(), makeToolReq(map[string]interface{}{
				"priority": []interface{}{bad},
			}))
			require.NoError(t, err)
			out := parseResult(t, res)
			require.False(t, out["success"].(bool), "priority=%v should be rejected", bad)
			require.Equal(t, ErrInvalidArgument, out["error"].(map[string]interface{})["code"])
		})
	}
}

func TestListBacklogItems_ReturnsInvalidArgument_When_StatusEntryWrongType(t *testing.T) {
	h := newTestListBacklogHandlers(t)

	res, err := h.listBacklogItems(context.Background(), makeToolReq(map[string]interface{}{
		"status": []interface{}{"ready", float64(1)},
	}))
	require.NoError(t, err)
	out := parseResult(t, res)
	require.False(t, out["success"].(bool), "a non-string status entry must be rejected, not silently dropped")
	require.Equal(t, ErrInvalidArgument, out["error"].(map[string]interface{})["code"])
}

func TestListBacklogItems_ReturnsInvalidArgument_When_PriorityEntryWrongType(t *testing.T) {
	h := newTestListBacklogHandlers(t)

	res, err := h.listBacklogItems(context.Background(), makeToolReq(map[string]interface{}{
		"priority": []interface{}{"3"},
	}))
	require.NoError(t, err)
	out := parseResult(t, res)
	require.False(t, out["success"].(bool), "a non-numeric priority entry must be rejected, not silently dropped")
	require.Equal(t, ErrInvalidArgument, out["error"].(map[string]interface{})["code"])
}

// TestListBacklogItems_FiltersByValidPriority verifies priority values 1-5
// actually reach the RPC and filter results — previously only the
// out-of-range rejection path was tested, leaving the priorities/priorities32
// wiring itself unverified.
func TestListBacklogItems_FiltersByValidPriority(t *testing.T) {
	h := newTestListBacklogHandlers(t)
	createBacklogItemWithStatusAndPriority(t, h.storage, "P1 item", string(session.BacklogStatusReady), 1)
	createBacklogItemWithStatusAndPriority(t, h.storage, "P3 item", string(session.BacklogStatusReady), 3)
	createBacklogItemWithStatusAndPriority(t, h.storage, "P5 item", string(session.BacklogStatusReady), 5)

	res, err := h.listBacklogItems(context.Background(), makeToolReq(map[string]interface{}{
		"priority": []interface{}{float64(1), float64(5)},
	}))
	require.NoError(t, err)
	out := parseResult(t, res)
	require.True(t, out["success"].(bool))
	items := out["items"].([]interface{})
	require.Len(t, items, 2)
	for _, raw := range items {
		p := raw.(map[string]interface{})["priority"].(float64)
		require.Contains(t, []float64{1, 5}, p)
	}
}

// TestListBacklogItems_PassesThroughIncludeTerminalIncludeArchivedSortBy
// verifies include_terminal/include_archived/sort_by are actually threaded
// into the RPC request rather than silently dropped or transposed.
func TestListBacklogItems_PassesThroughIncludeTerminalIncludeArchivedSortBy(t *testing.T) {
	h := newTestListBacklogHandlers(t)
	createBacklogItemWithStatusAndPriority(t, h.storage, "Done item", string(session.BacklogStatusDone), 3)
	createBacklogItemWithStatusAndPriority(t, h.storage, "Archived item", string(session.BacklogStatusArchived), 3)

	// Default (no include_terminal/include_archived): both terminal statuses hidden.
	res, err := h.listBacklogItems(context.Background(), makeToolReq(map[string]interface{}{}))
	require.NoError(t, err)
	out := parseResult(t, res)
	require.Empty(t, out["items"])

	// include_terminal=true surfaces the done item but not the archived one.
	res, err = h.listBacklogItems(context.Background(), makeToolReq(map[string]interface{}{
		"include_terminal": true,
	}))
	require.NoError(t, err)
	out = parseResult(t, res)
	items := out["items"].([]interface{})
	require.Len(t, items, 1)
	require.Equal(t, "Done item", items[0].(map[string]interface{})["title"])

	// include_terminal=true + include_archived=true surfaces both, and sort_by
	// is accepted without error (RPC-level sort correctness is exercised by
	// BacklogService's own tests — this only verifies the field reaches the RPC).
	res, err = h.listBacklogItems(context.Background(), makeToolReq(map[string]interface{}{
		"include_terminal": true,
		"include_archived": true,
		"sort_by":          "priority",
	}))
	require.NoError(t, err)
	out = parseResult(t, res)
	require.True(t, out["success"].(bool))
	items = out["items"].([]interface{})
	require.Len(t, items, 2)
}

// TestListBacklogItems_ClampsLimitAboveMax verifies a limit above the MCP
// tool's own cap (50) is clamped rather than passed straight through.
func TestListBacklogItems_ClampsLimitAboveMax(t *testing.T) {
	h := newTestListBacklogHandlers(t)
	for i := 0; i < 60; i++ {
		createBacklogItemWithStatusAndPriority(t, h.storage, fmt.Sprintf("Ready %d", i), string(session.BacklogStatusReady), 3)
	}

	res, err := h.listBacklogItems(context.Background(), makeToolReq(map[string]interface{}{
		"status": []interface{}{"ready"},
		"limit":  float64(1000),
	}))
	require.NoError(t, err)
	out := parseResult(t, res)
	items := out["items"].([]interface{})
	require.Len(t, items, 50, "limit must be clamped to the tool's max of 50")
}

func TestListBacklogItems_ReturnsDefaultLimitOf10_When_NoLimitArgGiven(t *testing.T) {
	h := newTestListBacklogHandlers(t)
	for i := 0; i < 15; i++ {
		createBacklogItemWithStatusAndPriority(t, h.storage, fmt.Sprintf("Ready %d", i), string(session.BacklogStatusReady), 3)
	}

	res, err := h.listBacklogItems(context.Background(), makeToolReq(map[string]interface{}{
		"status": []interface{}{"ready"},
	}))
	require.NoError(t, err)
	out := parseResult(t, res)
	require.True(t, out["success"].(bool))
	items := out["items"].([]interface{})
	require.Len(t, items, 10)
	require.True(t, out["has_more"].(bool))
	require.Equal(t, float64(15), out["total_count"])
}

func TestListBacklogItems_ReturnsUnavailable_When_BacklogSvcNil(t *testing.T) {
	storage := newTestBacklogStorage(t)
	h := &backlogHandlers{storage: storage} // backlogSvc left nil

	res, err := h.listBacklogItems(context.Background(), makeToolReq(map[string]interface{}{}))
	require.NoError(t, err)
	out := parseResult(t, res)
	require.False(t, out["success"].(bool))
	require.Equal(t, ErrInternalError, out["error"].(map[string]interface{})["code"])
}

// TestListBacklogItems_OffsetPagesPastFirstLimit protects the new
// offset/limit slicing logic — unlike get_notification_history/
// search_claude_history, which pass offset straight through to an
// already-tested RPC, list_backlog_items' pagination is new MCP-layer code
// with no wire-level test coverage of its own.
func TestListBacklogItems_OffsetPagesPastFirstLimit(t *testing.T) {
	h := newTestListBacklogHandlers(t)
	for i := 0; i < 15; i++ {
		createBacklogItemWithStatusAndPriority(t, h.storage, fmt.Sprintf("Ready %d", i), string(session.BacklogStatusReady), 3)
	}

	first, err := h.listBacklogItems(context.Background(), makeToolReq(map[string]interface{}{
		"status": []interface{}{"ready"},
		"limit":  float64(10),
		"offset": float64(0),
	}))
	require.NoError(t, err)
	firstOut := parseResult(t, first)
	firstItems := firstOut["items"].([]interface{})
	require.Len(t, firstItems, 10)
	require.True(t, firstOut["has_more"].(bool))

	second, err := h.listBacklogItems(context.Background(), makeToolReq(map[string]interface{}{
		"status": []interface{}{"ready"},
		"limit":  float64(10),
		"offset": float64(10),
	}))
	require.NoError(t, err)
	secondOut := parseResult(t, second)
	secondItems := secondOut["items"].([]interface{})
	require.Len(t, secondItems, 5)
	require.False(t, secondOut["has_more"].(bool))

	firstIDs := make(map[string]bool, len(firstItems))
	for _, raw := range firstItems {
		firstIDs[raw.(map[string]interface{})["id"].(string)] = true
	}
	for _, raw := range secondItems {
		id := raw.(map[string]interface{})["id"].(string)
		require.False(t, firstIDs[id], "second page must not repeat an item already returned on the first page")
	}
}

// TestPaginateBacklogItems_SlicesAndReportsHasMore exercises the extracted
// pagination helper directly, independent of the RPC/handler plumbing.
func TestPaginateBacklogItems_SlicesAndReportsHasMore(t *testing.T) {
	items := make([]*sessionv1.BacklogItem, 15)
	for i := range items {
		items[i] = &sessionv1.BacklogItem{Id: fmt.Sprintf("item-%d", i)}
	}

	page, hasMore := paginateBacklogItems(items, 10, 0)
	require.Len(t, page, 10)
	require.True(t, hasMore)
	require.Equal(t, "item-0", page[0].Id)

	page, hasMore = paginateBacklogItems(items, 10, 10)
	require.Len(t, page, 5)
	require.False(t, hasMore)
	require.Equal(t, "item-10", page[0].Id)

	page, hasMore = paginateBacklogItems(items, 10, 100)
	require.Empty(t, page)
	require.False(t, hasMore)
}

// --- post_backlog_update (Epic 8.3) ---

// TestPostBacklogUpdate_should_Succeed_When_SessionLinkedToItem mirrors a
// gated tool's happy path, but via the new ungated tool.
func TestPostBacklogUpdate_should_Succeed_When_SessionLinkedToItem(t *testing.T) {
	t.Parallel()
	storage := newTestBacklogStorage(t)
	ctx := context.Background()

	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "item for linked post_backlog_update test",
		Status: string(session.BacklogStatusInProgress),
	})
	require.NoError(t, err)

	sessionUUID := uuid.New().String()
	_, err = storage.CreateItemSession(ctx, session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: sessionUUID,
		SessionRole: "work",
	})
	require.NoError(t, err)

	handler := &backlogHandlers{storage: storage, store: &stubStore{}}
	req := makeToolReq(map[string]interface{}{
		"item_id": item.ID,
		"message": "made progress",
	})

	result, err := handler.postBacklogUpdate(WithSessionUUID(ctx, sessionUUID), req)
	require.NoError(t, err)
	require.Len(t, result.Content, 1)
	tc, ok := result.Content[0].(mcpgo.TextContent)
	require.True(t, ok)
	assert.Contains(t, tc.Text, "Posted activity update")

	notes, err := storage.ListActivityNotesForItem(ctx, item.ID)
	require.NoError(t, err)
	require.Len(t, notes, 1)
	assert.Equal(t, sessionUUID, notes[0].AuthorSessionUUID)
	assert.Equal(t, "made progress", notes[0].Message)
}

// TestPostBacklogUpdate_should_Succeed_When_SessionNotLinkedToItem is the
// core behavior difference from every gated tool: unlike report_progress/
// request_review (which require GetItemSessionBySessionAndItem to succeed),
// post_backlog_update has no item-linkage check at all.
func TestPostBacklogUpdate_should_Succeed_When_SessionNotLinkedToItem(t *testing.T) {
	t.Parallel()
	storage := newTestBacklogStorage(t)
	ctx := context.Background()

	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "item for unlinked post_backlog_update test",
		Status: string(session.BacklogStatusInProgress),
	})
	require.NoError(t, err)

	// Deliberately no CreateItemSession call.
	sessionUUID := uuid.New().String()
	handler := &backlogHandlers{storage: storage, store: &stubStore{}}
	req := makeToolReq(map[string]interface{}{
		"item_id": item.ID,
		"message": "an unlinked note",
	})

	result, err := handler.postBacklogUpdate(WithSessionUUID(ctx, sessionUUID), req)
	require.NoError(t, err)
	tc, ok := result.Content[0].(mcpgo.TextContent)
	require.True(t, ok)
	assert.Contains(t, tc.Text, "Posted activity update")

	notes, err := storage.ListActivityNotesForItem(ctx, item.ID)
	require.NoError(t, err)
	require.Len(t, notes, 1)
	assert.Equal(t, sessionUUID, notes[0].AuthorSessionUUID, "the note must be attributed to the caller even though it is not linked to the item")
}

// TestPostBacklogUpdate_should_Succeed_When_NoSessionUUIDPresent verifies a
// manual/external MCP client (no STAPLER_SESSION_UUID at all) is a
// legitimate caller — the note's author falls back to the "manual" sentinel,
// not an error. Note: this deliberately does not assert an empty
// AuthorSessionUUID — callerSessionUUIDForAudit (tools_backlog.go) returns
// the literal string "manual" (manualCallerSentinel), not "", when no
// session UUID is present in context.
func TestPostBacklogUpdate_should_Succeed_When_NoSessionUUIDPresent(t *testing.T) {
	t.Parallel()
	storage := newTestBacklogStorage(t)
	ctx := context.Background()

	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title: "item for no-session-uuid post_backlog_update test",
	})
	require.NoError(t, err)

	// No store needed: the manualCallerSentinel path never calls h.store.
	handler := &backlogHandlers{storage: storage}
	req := makeToolReq(map[string]interface{}{
		"item_id": item.ID,
		"message": "a manual note",
	})

	result, err := handler.postBacklogUpdate(context.Background(), req)
	require.NoError(t, err)
	tc, ok := result.Content[0].(mcpgo.TextContent)
	require.True(t, ok)
	assert.Contains(t, tc.Text, "Posted activity update")

	notes, err := storage.ListActivityNotesForItem(ctx, item.ID)
	require.NoError(t, err)
	require.Len(t, notes, 1)
	assert.Equal(t, manualCallerSentinel, notes[0].AuthorSessionUUID, "with no session context, provenance falls back to the \"manual\" sentinel, not an error")
}

// TestPostBacklogUpdate_should_UseSessionIDParamForProvenance_When_Provided
// verifies an explicit session_id param overrides the context's caller UUID
// for attribution (mirrors set_session_goal's pattern).
func TestPostBacklogUpdate_should_UseSessionIDParamForProvenance_When_Provided(t *testing.T) {
	t.Parallel()
	storage := newTestBacklogStorage(t)
	ctx := context.Background()

	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{Title: "item for session_id override test"})
	require.NoError(t, err)

	namedUUID := uuid.New().String()
	store := &stubStore{instances: []*session.Instance{
		{Title: "named-session", UUID: namedUUID},
	}}
	handler := &backlogHandlers{storage: storage, store: store}

	callerUUID := uuid.New().String() // different from named-session's UUID
	req := makeToolReq(map[string]interface{}{
		"item_id":    item.ID,
		"message":    "note attributed to a named session",
		"session_id": "named-session",
	})

	result, err := handler.postBacklogUpdate(WithSessionUUID(ctx, callerUUID), req)
	require.NoError(t, err)
	tc, ok := result.Content[0].(mcpgo.TextContent)
	require.True(t, ok)
	assert.Contains(t, tc.Text, "Posted activity update")

	notes, err := storage.ListActivityNotesForItem(ctx, item.ID)
	require.NoError(t, err)
	require.Len(t, notes, 1)
	assert.Equal(t, namedUUID, notes[0].AuthorSessionUUID, "session_id param must override the context's caller UUID for provenance")
	assert.Equal(t, "named-session", notes[0].AuthorSessionTitle)
}

// TestPostBacklogUpdate_should_RejectMessage_When_EmptyOrWhitespaceOnly
// covers the message-required validation, including the trim-then-check
// order (a whitespace-only message must not slip through as "non-empty").
func TestPostBacklogUpdate_should_RejectMessage_When_EmptyOrWhitespaceOnly(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		message interface{}
	}{
		{"empty string", ""},
		{"whitespace only", "   \n\t  "},
		{"missing entirely", nil},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			storage := newTestBacklogStorage(t)
			ctx := context.Background()
			item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{Title: "item"})
			require.NoError(t, err)

			handler := &backlogHandlers{storage: storage}
			args := map[string]interface{}{"item_id": item.ID}
			if tc.message != nil {
				args["message"] = tc.message
			}

			result, err := handler.postBacklogUpdate(ctx, makeToolReq(args))
			require.NoError(t, err)
			m := parseResult(t, result)
			require.False(t, m["success"].(bool))
			errObj := m["error"].(map[string]interface{})
			assert.Equal(t, ErrInvalidArgument, errObj["code"])
		})
	}
}

// TestPostBacklogUpdate_should_RejectMessage_When_OverMaxLength mirrors
// request_review's 2000-char cap and confirms the over-length message is
// never persisted.
func TestPostBacklogUpdate_should_RejectMessage_When_OverMaxLength(t *testing.T) {
	t.Parallel()
	storage := newTestBacklogStorage(t)
	ctx := context.Background()
	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{Title: "item"})
	require.NoError(t, err)

	handler := &backlogHandlers{storage: storage}
	req := makeToolReq(map[string]interface{}{
		"item_id": item.ID,
		"message": strings.Repeat("a", maxPostBacklogUpdateMessageLen+1),
	})

	result, err := handler.postBacklogUpdate(ctx, req)
	require.NoError(t, err)
	m := parseResult(t, result)
	require.False(t, m["success"].(bool))
	errObj := m["error"].(map[string]interface{})
	assert.Equal(t, ErrInvalidArgument, errObj["code"])

	notes, err := storage.ListActivityNotesForItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Empty(t, notes, "an over-length message must never be persisted")
}

// TestPostBacklogUpdate_should_ReturnErrItemNotFound_When_ItemIDDoesNotExist
// [Concern 2 fix] proves ent.IsConstraintError detection in AppendActivityNote
// and the handler's errors.Is mapping work end to end: a nonexistent item_id
// must map to ITEM_NOT_FOUND, not a raw INTERNAL_ERROR.
func TestPostBacklogUpdate_should_ReturnErrItemNotFound_When_ItemIDDoesNotExist(t *testing.T) {
	t.Parallel()
	storage := newTestBacklogStorage(t)
	handler := &backlogHandlers{storage: storage}

	req := makeToolReq(map[string]interface{}{
		"item_id": uuid.New().String(),
		"message": "a note for an item that doesn't exist",
	})

	result, err := handler.postBacklogUpdate(context.Background(), req)
	require.NoError(t, err)
	m := parseResult(t, result)
	require.False(t, m["success"].(bool))
	errObj := m["error"].(map[string]interface{})
	assert.Equal(t, ErrItemNotFound, errObj["code"], "a nonexistent item_id must map to ITEM_NOT_FOUND, not INTERNAL_ERROR")
}

// TestPostBacklogUpdate_should_StripHTMLTagsBeforePersisting_When_MessageContainsMarkup
// [Concern 1 fix] proves sanitization happens before Create(), not only at
// get_backlog_item render time — reads the note back via
// ListActivityNotesForItem and asserts the persisted message has the tag
// stripped.
func TestPostBacklogUpdate_should_StripHTMLTagsBeforePersisting_When_MessageContainsMarkup(t *testing.T) {
	t.Parallel()
	storage := newTestBacklogStorage(t)
	ctx := context.Background()
	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{Title: "item"})
	require.NoError(t, err)

	handler := &backlogHandlers{storage: storage}
	req := makeToolReq(map[string]interface{}{
		"item_id": item.ID,
		"message": "<script>alert(1)</script>hello",
	})

	result, err := handler.postBacklogUpdate(context.Background(), req)
	require.NoError(t, err)
	tc, ok := result.Content[0].(mcpgo.TextContent)
	require.True(t, ok)
	assert.Contains(t, tc.Text, "Posted activity update")

	notes, err := storage.ListActivityNotesForItem(ctx, item.ID)
	require.NoError(t, err)
	require.Len(t, notes, 1)
	assert.NotContains(t, notes[0].Message, "<script>", "HTML tags must be stripped before Create(), not only at get_backlog_item render time")
	assert.NotContains(t, notes[0].Message, "</script>")
	assert.Contains(t, notes[0].Message, "alert(1)hello")
}

// TestPostBacklogUpdateFeature_should_NotLoosenGatedToolsPermissions_When_SessionUnlinkedOrAbsent
// is the regression guard requirements.md calls for: proves this feature did
// not loosen report_progress/request_review's existing gates by exercising
// both tools under the same unlinked/no-session-UUID conditions
// post_backlog_update is deliberately exempt from, on a handler that also
// has post_backlog_update wired.
func TestPostBacklogUpdateFeature_should_NotLoosenGatedToolsPermissions_When_SessionUnlinkedOrAbsent(t *testing.T) {
	t.Parallel()
	storage := newTestBacklogStorage(t)
	ctx := context.Background()

	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "item for gated-tools regression guard",
		Status: string(session.BacklogStatusInProgress),
	})
	require.NoError(t, err)

	handler := &backlogHandlers{storage: storage}

	assertPermissionDenied := func(t *testing.T, result *mcpgo.CallToolResult, callErr error) {
		t.Helper()
		require.NoError(t, callErr)
		m := parseResult(t, result)
		require.False(t, m["success"].(bool))
		errObj := m["error"].(map[string]interface{})
		assert.Equal(t, ErrPermissionDenied, errObj["code"])
	}

	t.Run("report_progress: no session UUID", func(t *testing.T) {
		req := makeToolReq(map[string]interface{}{
			"item_id":        item.ID,
			"criteria_index": float64(0),
			"status":         "pass",
		})
		result, callErr := handler.reportProgress(context.Background(), req)
		assertPermissionDenied(t, result, callErr)
	})

	t.Run("report_progress: unlinked session UUID", func(t *testing.T) {
		req := makeToolReq(map[string]interface{}{
			"item_id":        item.ID,
			"criteria_index": float64(0),
			"status":         "pass",
		})
		result, callErr := handler.reportProgress(WithSessionUUID(ctx, uuid.New().String()), req)
		assertPermissionDenied(t, result, callErr)
	})

	t.Run("request_review: no session UUID", func(t *testing.T) {
		req := makeToolReq(map[string]interface{}{
			"item_id": item.ID,
			"message": "done",
		})
		result, callErr := handler.requestReview(context.Background(), req)
		assertPermissionDenied(t, result, callErr)
	})

	t.Run("request_review: unlinked session UUID", func(t *testing.T) {
		req := makeToolReq(map[string]interface{}{
			"item_id": item.ID,
			"message": "done",
		})
		result, callErr := handler.requestReview(WithSessionUUID(ctx, uuid.New().String()), req)
		assertPermissionDenied(t, result, callErr)
	})
}

// --- wait_for_backlog_event exclusion of activity notes (Epic 8.4 / Blocker 3) ---

// TestWaitForBacklogEvent_should_NotWake_When_ActivityNoteAddedFires is the
// regression test for Epic 4.4's early-skip: an activity-note-posted event
// must never satisfy any event_type filter, including the default "any".
// Mirrors TestWaitForBacklogEvent_FiltersByEventType's harness (subscribe,
// wait for SubscriberCount, publish).
func TestWaitForBacklogEvent_should_NotWake_When_ActivityNoteAddedFires(t *testing.T) {
	storage := newTestBacklogStorage(t)
	ctx := context.Background()
	bus := events.NewEventBus(32)

	// Baseline captured after storage setup, mirroring
	// TestWaitForBacklogEvent_NoGoroutineLeak's rationale: opening the
	// sqlite-backed Storage starts its own long-lived pool goroutines that
	// are test-infra noise, not something waitForBacklogEvent could leak.
	baseline := goleak.IgnoreCurrent()

	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "Activity note should not wake wait_for_backlog_event",
		Status: string(session.BacklogStatusInProgress),
	})
	require.NoError(t, err)

	handler := &backlogHandlers{storage: storage, eventBus: bus}
	req := makeToolReq(map[string]interface{}{"item_id": item.ID, "timeout_seconds": float64(1)})

	resultCh := make(chan waitCallOutcome, 1)
	go func() {
		res, callErr := handler.waitForBacklogEvent(ctx, req)
		resultCh <- waitCallOutcome{res: res, err: callErr}
	}()

	waitSubscriberCount(t, bus, 1)
	bus.Publish(&events.Event{
		Type: events.EventBacklogItemChanged,
		BacklogItemPayload: &events.BacklogItemEventPayload{
			Kind:         events.BacklogChangeActivityNoteAdded,
			Item:         &session.BacklogItemData{ID: item.ID, Status: string(session.BacklogStatusInProgress)},
			ActivityNote: &session.ActivityNoteData{Message: "an informal note"},
		},
	})

	select {
	case outcome := <-resultCh:
		require.NoError(t, outcome.err)
		out := decodeWaitResult(t, outcome.res)
		require.True(t, out.Success)
		assert.False(t, out.EventReceived, "an activity-note event must never satisfy wait_for_backlog_event, even with event_type=\"any\"")
		require.NotNil(t, out.Error)
		assert.Equal(t, "WAIT_TIMEOUT", out.Error.Code)
	case <-time.After(3 * time.Second):
		t.Fatal("waitForBacklogEvent did not return within the timeout window")
	}

	require.Eventually(t, func() bool { return bus.SubscriberCount() == 0 }, 3*time.Second, 10*time.Millisecond,
		"the timed-out call must Unsubscribe on exit")
	goleak.VerifyNone(t, baseline)
}

// TestWaitForBacklogEvent_should_StillWake_When_StatusTransitionFires proves
// Epic 4.4's exclusion is targeted, not a rewrite: a real status-transition
// event on the same item still wakes the waiter exactly as before.
func TestWaitForBacklogEvent_should_StillWake_When_StatusTransitionFires(t *testing.T) {
	storage := newTestBacklogStorage(t)
	ctx := context.Background()
	bus := events.NewEventBus(32)

	// See TestWaitForBacklogEvent_should_NotWake_When_ActivityNoteAddedFires's
	// comment on why baseline is captured after storage setup.
	baseline := goleak.IgnoreCurrent()

	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "Status transition should still wake wait_for_backlog_event",
		Status: string(session.BacklogStatusInProgress),
	})
	require.NoError(t, err)

	handler := &backlogHandlers{storage: storage, eventBus: bus}
	req := makeToolReq(map[string]interface{}{"item_id": item.ID, "timeout_seconds": float64(5)})

	resultCh := make(chan waitCallOutcome, 1)
	go func() {
		res, callErr := handler.waitForBacklogEvent(ctx, req)
		resultCh <- waitCallOutcome{res: res, err: callErr}
	}()

	waitSubscriberCount(t, bus, 1)
	bus.Publish(&events.Event{
		Type: events.EventBacklogItemChanged,
		BacklogItemPayload: &events.BacklogItemEventPayload{
			Kind:      events.BacklogChangeStatusTransition,
			Item:      &session.BacklogItemData{ID: item.ID, Status: string(session.BacklogStatusReview)},
			OldStatus: string(session.BacklogStatusInProgress),
			NewStatus: string(session.BacklogStatusReview),
		},
	})

	select {
	case outcome := <-resultCh:
		require.NoError(t, outcome.err)
		out := decodeWaitResult(t, outcome.res)
		require.True(t, out.EventReceived)
		assert.Equal(t, "status_changed", out.EventKind)
	case <-time.After(3 * time.Second):
		t.Fatal("waitForBacklogEvent did not return after a genuine status-transition event")
	}

	require.Eventually(t, func() bool { return bus.SubscriberCount() == 0 }, 3*time.Second, 10*time.Millisecond,
		"the matched call must Unsubscribe on exit")
	goleak.VerifyNone(t, baseline)
}

// --- get_backlog_item's "## Activity Log" rendering (Epic 8.6) ---

// TestGetBacklogItem_should_RenderActivityLogAfterVerdict_When_BothPresent
// verifies heading placement: "## Activity Log" appears after "## Latest
// Review Verdict" and before the role-aware guidance block.
func TestGetBacklogItem_should_RenderActivityLogAfterVerdict_When_BothPresent(t *testing.T) {
	t.Parallel()
	storage := newTestBacklogStorage(t)
	ctx := context.Background()

	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "item with verdict and activity log",
		Status: string(session.BacklogStatusReview),
	})
	require.NoError(t, err)

	_, err = storage.CreateItemSessionWithVerdict(ctx, session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: uuid.New().String(),
		SessionRole: session.SessionRoleReview,
	}, session.ReviewVerdictData{
		OverallOutcome: session.ReviewOutcomePass,
		Summary:        "lgtm",
	})
	require.NoError(t, err)

	require.NoError(t, storage.AppendActivityNote(ctx, item.ID, "", "worker-1", "a note about progress"))

	handler := &backlogHandlers{storage: storage}
	result, err := handler.getBacklogItem(ctx, makeToolReq(map[string]interface{}{"item_id": item.ID}))
	require.NoError(t, err)
	tc, ok := result.Content[0].(mcpgo.TextContent)
	require.True(t, ok)
	text := tc.Text

	verdictIdx := strings.Index(text, "## Latest Review Verdict")
	activityIdx := strings.Index(text, "## Activity Log")
	toolsIdx := strings.Index(text, "## Available MCP Tools")
	require.NotEqual(t, -1, verdictIdx, "expected the verdict section to be present")
	require.NotEqual(t, -1, activityIdx, "expected the activity log section to be present")
	require.NotEqual(t, -1, toolsIdx, "expected the role-aware guidance block to be present")
	assert.Less(t, verdictIdx, activityIdx, "Activity Log must come after Latest Review Verdict")
	assert.Less(t, activityIdx, toolsIdx, "Activity Log must come before the role-aware guidance block")
}

// TestGetBacklogItem_should_OmitActivityLogSection_When_NoNotesExist matches
// the "## Latest Review Verdict" section's own only-if-present convention.
func TestGetBacklogItem_should_OmitActivityLogSection_When_NoNotesExist(t *testing.T) {
	t.Parallel()
	storage := newTestBacklogStorage(t)
	ctx := context.Background()
	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{Title: "item with no notes"})
	require.NoError(t, err)

	handler := &backlogHandlers{storage: storage}
	result, err := handler.getBacklogItem(ctx, makeToolReq(map[string]interface{}{"item_id": item.ID}))
	require.NoError(t, err)
	text := result.Content[0].(mcpgo.TextContent).Text

	assert.NotContains(t, text, "## Activity Log")
}

// TestGetBacklogItem_should_RenderEachActivityNoteEntry_When_AuthorFallbackVaries
// verifies the "- note from %s at %s: %s" format and the author-fallback
// chain (title -> raw UUID -> "manual").
func TestGetBacklogItem_should_RenderEachActivityNoteEntry_When_AuthorFallbackVaries(t *testing.T) {
	t.Parallel()
	storage := newTestBacklogStorage(t)
	ctx := context.Background()
	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{Title: "item"})
	require.NoError(t, err)

	require.NoError(t, storage.AppendActivityNote(ctx, item.ID, "uuid-with-title", "Worker One", "note with a title"))
	require.NoError(t, storage.AppendActivityNote(ctx, item.ID, "uuid-only-1234", "", "note with only a uuid"))
	require.NoError(t, storage.AppendActivityNote(ctx, item.ID, "", "", "a fully manual note"))

	notes, err := storage.ListActivityNotesForItem(ctx, item.ID)
	require.NoError(t, err)
	require.Len(t, notes, 3)

	handler := &backlogHandlers{storage: storage}
	result, err := handler.getBacklogItem(ctx, makeToolReq(map[string]interface{}{"item_id": item.ID}))
	require.NoError(t, err)
	text := result.Content[0].(mcpgo.TextContent).Text

	assert.Contains(t, text, fmt.Sprintf("- note from Worker One at %s: note with a title", notes[0].CreatedAt.Format(time.RFC3339)))
	assert.Contains(t, text, fmt.Sprintf("- note from uuid-only-1234 at %s: note with only a uuid", notes[1].CreatedAt.Format(time.RFC3339)))
	assert.Contains(t, text, fmt.Sprintf("- note from manual at %s: a fully manual note", notes[2].CreatedAt.Format(time.RFC3339)))
}

// TestGetBacklogItem_should_CapActivityLogAtTwentyEntries_When_MoreThanTwentyPosted
// verifies the maxActivityLogEntriesRendered cap and its
// "(N older entries not shown)" line.
func TestGetBacklogItem_should_CapActivityLogAtTwentyEntries_When_MoreThanTwentyPosted(t *testing.T) {
	t.Parallel()
	storage := newTestBacklogStorage(t)
	ctx := context.Background()
	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{Title: "item with many notes"})
	require.NoError(t, err)

	const total = maxActivityLogEntriesRendered + 3
	for i := 0; i < total; i++ {
		require.NoError(t, storage.AppendActivityNote(ctx, item.ID, "", "", fmt.Sprintf("note number %d", i)))
	}

	handler := &backlogHandlers{storage: storage}
	result, err := handler.getBacklogItem(ctx, makeToolReq(map[string]interface{}{"item_id": item.ID}))
	require.NoError(t, err)
	text := result.Content[0].(mcpgo.TextContent).Text

	assert.Contains(t, text, "(3 older entries not shown)")
	// Trailing "\n" (every rendered entry ends the line right after the message)
	// makes each check exact — without it, e.g. "note number 1" would be a false
	// substring match against the still-present "note number 10".
	for i := 0; i < 3; i++ {
		assert.NotContains(t, text, fmt.Sprintf("note number %d\n", i))
	}
	for i := 3; i < total; i++ {
		assert.Contains(t, text, fmt.Sprintf("note number %d\n", i))
	}
}

// TestGetBacklogItem_should_SanitizeActivityLogEntry_When_MessageContainsHTML
// proves session.SanitizeForAgentContext is applied at render time,
// independent of post_backlog_update's own persist-time stripping — the note
// is written directly via storage.AppendActivityNote, bypassing the handler
// entirely, so any stripping observed here can only be render-time.
func TestGetBacklogItem_should_SanitizeActivityLogEntry_When_MessageContainsHTML(t *testing.T) {
	t.Parallel()
	storage := newTestBacklogStorage(t)
	ctx := context.Background()
	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{Title: "item"})
	require.NoError(t, err)

	require.NoError(t, storage.AppendActivityNote(ctx, item.ID, "", "", "<b>bold</b> note"))

	handler := &backlogHandlers{storage: storage}
	result, err := handler.getBacklogItem(ctx, makeToolReq(map[string]interface{}{"item_id": item.ID}))
	require.NoError(t, err)
	text := result.Content[0].(mcpgo.TextContent).Text

	assert.NotContains(t, text, "<b>")
	assert.NotContains(t, text, "</b>")
	assert.Contains(t, text, "bold note")
}

// TestGetBacklogItem_should_NotUseVerdictFormatStrings_When_NoVerdictExists
// proves no accidental format collision with the verdict section: the
// activity log section's own output never contains "Outcome:" or
// "Criterion " when no review verdict exists.
func TestGetBacklogItem_should_NotUseVerdictFormatStrings_When_NoVerdictExists(t *testing.T) {
	t.Parallel()
	storage := newTestBacklogStorage(t)
	ctx := context.Background()
	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{Title: "item"})
	require.NoError(t, err)
	require.NoError(t, storage.AppendActivityNote(ctx, item.ID, "", "", "just a note"))

	handler := &backlogHandlers{storage: storage}
	result, err := handler.getBacklogItem(ctx, makeToolReq(map[string]interface{}{"item_id": item.ID}))
	require.NoError(t, err)
	text := result.Content[0].(mcpgo.TextContent).Text

	assert.NotContains(t, text, "Outcome:")
	assert.NotContains(t, text, "Criterion ")
}

// --- Link error consistency (ITEM_NOT_FOUND vs PERMISSION_DENIED) ---
//
// See project_plans/backlog-link-error-consistency/. Before this fix, every
// mutating tool collapsed "item doesn't exist" and "item exists but this
// session has no link to it" into the same PERMISSION_DENIED, while
// get_backlog_item correctly distinguished the two via ITEM_NOT_FOUND.

type linkConsistencyCase struct {
	name   string
	invoke func(h *backlogHandlers, ctx context.Context, itemID string) (*mcpgo.CallToolResult, error)
}

// linkConsistencyMutatingTools covers link-check disambiguation only (item-missing
// vs. item-exists-no-link) — it deliberately does not encode a "correct role" per
// tool, since 2 of the 7 (report_progress, request_review) have no role check at
// all. Role-mismatch coverage lives in the standalone Test*_should_ReturnPermissionDenied_When_CallerRoleNot*
// tests below, one per handler that actually has a role branch.
var linkConsistencyMutatingTools = []linkConsistencyCase{
	{"report_progress", func(h *backlogHandlers, ctx context.Context, itemID string) (*mcpgo.CallToolResult, error) {
		return h.reportProgress(ctx, makeToolReq(map[string]interface{}{"item_id": itemID, "criteria_index": float64(0), "status": "pass"}))
	}},
	{"request_review", func(h *backlogHandlers, ctx context.Context, itemID string) (*mcpgo.CallToolResult, error) {
		return h.requestReview(ctx, makeToolReq(map[string]interface{}{"item_id": itemID, "message": "done"}))
	}},
	{"submit_review_verdict", func(h *backlogHandlers, ctx context.Context, itemID string) (*mcpgo.CallToolResult, error) {
		return h.submitReviewVerdict(ctx, makeToolReq(map[string]interface{}{
			"item_id": itemID, "summary": "s",
			"verdicts": []interface{}{map[string]interface{}{"criterion_index": float64(0), "outcome": "PASS", "evidence": "e"}},
		}))
	}},
	{"report_pr_created", func(h *backlogHandlers, ctx context.Context, itemID string) (*mcpgo.CallToolResult, error) {
		return h.reportPRCreated(ctx, makeToolReq(map[string]interface{}{
			"item_id": itemID, "pr_url": "https://github.com/tstapler/stapler-squad/pull/1", "pr_number": float64(1), "summary": "s",
		}))
	}},
	{"submit_triage_result", func(h *backlogHandlers, ctx context.Context, itemID string) (*mcpgo.CallToolResult, error) {
		return h.submitTriageResult(ctx, makeToolReq(map[string]interface{}{"item_id": itemID, "summary": "s"}))
	}},
	{"report_blocked", func(h *backlogHandlers, ctx context.Context, itemID string) (*mcpgo.CallToolResult, error) {
		return h.reportBlocked(ctx, makeToolReq(map[string]interface{}{"item_id": itemID, "rationale": "blocked on external dependency"}))
	}},
	{"report_duplicate", func(h *backlogHandlers, ctx context.Context, itemID string) (*mcpgo.CallToolResult, error) {
		return h.reportDuplicate(ctx, makeToolReq(map[string]interface{}{
			"item_id": itemID, "duplicate_ref": "https://github.com/tstapler/stapler-squad/pull/1", "reason": "dup",
		}))
	}},
}

// TestBacklogTools_LinkErrorConsistency_should_ReturnPermissionDenied_When_ItemExistsButNoLink
// verifies AC3(a): when the backlog item exists but the calling session has no
// ItemSession link row, every mutating tool returns PERMISSION_DENIED with a
// self-diagnosable message (AC2) naming both the session UUID/item ID and all
// mutating tools, not just the one that was called.
func TestBacklogTools_LinkErrorConsistency_should_ReturnPermissionDenied_When_ItemExistsButNoLink(t *testing.T) {
	for _, tc := range linkConsistencyMutatingTools {
		t.Run(tc.name, func(t *testing.T) {
			storage := newTestBacklogStorage(t)
			item, err := storage.CreateBacklogItem(context.Background(), session.BacklogItemData{
				Title:              "Link consistency test item",
				AcceptanceCriteria: `[{"index":0,"text":"Criterion","status":"pending"}]`,
				Priority:           1,
				Status:             string(session.BacklogStatusInProgress),
			})
			require.NoError(t, err)

			sessionUUID := uuid.New().String()
			handler := &backlogHandlers{storage: storage}
			ctx := WithSessionUUID(context.Background(), sessionUUID)

			result, err := tc.invoke(handler, ctx, item.ID)
			require.NoError(t, err)
			m := parseResult(t, result)
			require.False(t, m["success"].(bool))

			errObj, ok := m["error"].(map[string]interface{})
			require.True(t, ok)
			assert.Equal(t, ErrPermissionDenied, errObj["code"].(string))

			message, _ := errObj["message"].(string)
			assert.Contains(t, message, sessionUUID)
			assert.Contains(t, message, item.ID)

			remediation, _ := errObj["remediation"].(string)
			for _, toolName := range []string{
				"report_progress", "request_review", "submit_review_verdict", "report_pr_created",
				"submit_triage_result", "report_blocked", "report_duplicate",
			} {
				assert.Contains(t, remediation, toolName)
			}
		})
	}
}

// TestBacklogTools_LinkErrorConsistency_should_ReturnItemNotFound_When_ItemDoesNotExist
// verifies AC3(b): when the backlog item itself does not exist, get_backlog_item
// and every mutating tool return ITEM_NOT_FOUND — never PERMISSION_DENIED —
// regardless of the calling session's link state.
func TestBacklogTools_LinkErrorConsistency_should_ReturnItemNotFound_When_ItemDoesNotExist(t *testing.T) {
	storage := newTestBacklogStorage(t)
	itemID := uuid.New().String() // never created
	sessionUUID := uuid.New().String()
	handler := &backlogHandlers{storage: storage}
	ctx := WithSessionUUID(context.Background(), sessionUUID)

	assertItemNotFound := func(t *testing.T, result *mcpgo.CallToolResult, err error) {
		t.Helper()
		require.NoError(t, err)
		m := parseResult(t, result)
		require.False(t, m["success"].(bool))
		errObj, ok := m["error"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, ErrItemNotFound, errObj["code"].(string))
		remediation, _ := errObj["remediation"].(string)
		assert.NotEmpty(t, remediation)
		assert.Contains(t, strings.ToLower(remediation), "not exist")
		assert.Contains(t, strings.ToLower(remediation), "do not retry")
	}

	t.Run("get_backlog_item", func(t *testing.T) {
		result, err := handler.getBacklogItem(ctx, makeToolReq(map[string]interface{}{"item_id": itemID}))
		assertItemNotFound(t, result, err)
	})

	for _, tc := range linkConsistencyMutatingTools {
		t.Run(tc.name, func(t *testing.T) {
			result, err := tc.invoke(handler, ctx, itemID)
			assertItemNotFound(t, result, err)
		})
	}
}

// TestSubmitReviewVerdict_should_ReturnPermissionDenied_When_CallerRoleNotReview
// verifies the pre-existing role-mismatch PERMISSION_DENIED branch is intact
// after routing submitReviewVerdict's link check through resolveItemLink — no
// prior test in this file covered this branch (adversarial-review.md Concern 1).
func TestSubmitReviewVerdict_should_ReturnPermissionDenied_When_CallerRoleNotReview(t *testing.T) {
	storage := newTestBacklogStorage(t)
	item, err := storage.CreateBacklogItem(context.Background(), session.BacklogItemData{
		Title:              "Wrong role item",
		AcceptanceCriteria: `[{"index":0,"text":"Criterion","status":"pending"}]`,
		Priority:           1,
		Status:             string(session.BacklogStatusReview),
	})
	require.NoError(t, err)

	sessionUUID := uuid.New().String()
	_, err = storage.CreateItemSession(context.Background(), session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: sessionUUID,
		SessionRole: session.SessionRoleWork, // wrong role
	})
	require.NoError(t, err)

	handler := &backlogHandlers{storage: storage}
	ctx := WithSessionUUID(context.Background(), sessionUUID)

	result, err := handler.submitReviewVerdict(ctx, makeToolReq(map[string]interface{}{
		"item_id": item.ID, "summary": "s",
		"verdicts": []interface{}{map[string]interface{}{"criterion_index": float64(0), "outcome": "PASS", "evidence": "e"}},
	}))
	require.NoError(t, err)
	m := parseResult(t, result)
	require.False(t, m["success"].(bool))
	errObj, ok := m["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, ErrPermissionDenied, errObj["code"].(string))
	assert.Contains(t, errObj["message"].(string), "only 'review' role may submit verdicts")
}

// TestSubmitTriageResult_should_ReturnPermissionDenied_When_CallerRoleNotTriage
// mirrors TestSubmitReviewVerdict_should_ReturnPermissionDenied_When_CallerRoleNotReview
// for submitTriageResult's role check.
func TestSubmitTriageResult_should_ReturnPermissionDenied_When_CallerRoleNotTriage(t *testing.T) {
	storage := newTestBacklogStorage(t)
	item, err := storage.CreateBacklogItem(context.Background(), session.BacklogItemData{
		Title:              "Wrong role item",
		AcceptanceCriteria: `[{"index":0,"text":"Criterion","status":"pending"}]`,
		Priority:           1,
		Status:             string(session.BacklogStatusInProgress),
	})
	require.NoError(t, err)

	sessionUUID := uuid.New().String()
	_, err = storage.CreateItemSession(context.Background(), session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: sessionUUID,
		SessionRole: session.SessionRoleWork, // wrong role
	})
	require.NoError(t, err)

	handler := &backlogHandlers{storage: storage}
	ctx := WithSessionUUID(context.Background(), sessionUUID)

	result, err := handler.submitTriageResult(ctx, makeToolReq(map[string]interface{}{"item_id": item.ID, "summary": "s"}))
	require.NoError(t, err)
	m := parseResult(t, result)
	require.False(t, m["success"].(bool))
	errObj, ok := m["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, ErrPermissionDenied, errObj["code"].(string))
	assert.Contains(t, errObj["message"].(string), "only 'triage' role may submit triage results")
}

// TestReportBlocked_should_ReturnPermissionDenied_When_CallerRoleNotWork mirrors
// TestSubmitTriageResult_should_ReturnPermissionDenied_When_CallerRoleNotTriage for
// reportBlocked's role check — prior to this test, reportBlocked had zero role-mismatch
// coverage anywhere in this file (confirmed by testing-quality review of this PR).
func TestReportBlocked_should_ReturnPermissionDenied_When_CallerRoleNotWork(t *testing.T) {
	storage := newTestBacklogStorage(t)
	item, err := storage.CreateBacklogItem(context.Background(), session.BacklogItemData{
		Title:              "Wrong role item",
		AcceptanceCriteria: `[{"index":0,"text":"Criterion","status":"pending"}]`,
		Priority:           1,
		Status:             string(session.BacklogStatusInProgress),
	})
	require.NoError(t, err)

	sessionUUID := uuid.New().String()
	_, err = storage.CreateItemSession(context.Background(), session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: sessionUUID,
		SessionRole: session.SessionRoleReview, // wrong role
	})
	require.NoError(t, err)

	handler := &backlogHandlers{storage: storage}
	ctx := WithSessionUUID(context.Background(), sessionUUID)

	result, err := handler.reportBlocked(ctx, makeToolReq(map[string]interface{}{"item_id": item.ID, "rationale": "blocked on external dependency"}))
	require.NoError(t, err)
	m := parseResult(t, result)
	require.False(t, m["success"].(bool))
	errObj, ok := m["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, ErrPermissionDenied, errObj["code"].(string))
	assert.Contains(t, errObj["message"].(string), "only 'work' role may report a blocked item")
}

// TestBacklogHandlers_should_HaveNoRemainingRawLinkCheck_When_SourceIsScanned is a
// structural guard (pre-mortem.md Failure #1, adversarial-review.md Concern 2):
// asserts the pre-fix raw "errors.Is(linkErr, session.ErrNotFound) -> unconditional
// PERMISSION_DENIED" pattern is gone from every call site, so a future mutating
// tool (or a silently-skipped story) that reintroduces it is caught even if nobody
// remembers to add it to linkConsistencyMutatingTools above. The expected count
// below must be bumped whenever a new mutating backlog tool is added — see the
// first review cycle of project_plans/backlog-link-error-consistency, which
// caught report_blocked and report_duplicate missing this exact guard after a
// stale-base merge with origin/main.
func TestBacklogHandlers_should_HaveNoRemainingRawLinkCheck_When_SourceIsScanned(t *testing.T) {
	data, err := os.ReadFile("tools_backlog.go")
	require.NoError(t, err, "read tools_backlog.go")
	content := string(data)

	const staleMessage = "this session is not linked to the specified backlog item"
	assert.Equal(t, 0, strings.Count(content, staleMessage),
		"the old undisambiguated PERMISSION_DENIED message must not appear anywhere — every call site must route through resolveItemLink")

	assert.Equal(t, 1, strings.Count(content, "func (h *backlogHandlers) resolveItemLink("),
		"resolveItemLink must be defined exactly once")

	callSiteCount := strings.Count(content, "h.resolveItemLink(ctx, callerUUID, itemID)")
	assert.Equal(t, 7, callSiteCount,
		"expected exactly 7 mutating handlers (report_progress, request_review, submit_review_verdict, report_pr_created, submit_triage_result, report_blocked, report_duplicate) to call resolveItemLink")
}
