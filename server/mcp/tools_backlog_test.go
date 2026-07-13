package mcp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/session"
)

// newTestBacklogStorage creates a temporary Storage for testing.
func newTestBacklogStorage(t *testing.T) *session.Storage {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "backlog-test-*")
	require.NoError(t, err)

	dbPath := filepath.Join(tmpDir, fmt.Sprintf("test-%d.db", time.Now().UnixNano()))
	repo, err := session.NewEntRepository(session.WithDatabasePath(dbPath))
	require.NoError(t, err)

	storage, err := session.NewStorageWithRepository(repo)
	require.NoError(t, err)

	t.Cleanup(func() {
		repo.Close()
		os.RemoveAll(tmpDir)
	})

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
