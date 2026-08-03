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
		storage:               storage,
		resolveSessionBranch:  func(context.Context, string) (string, error) { return "backlog/ship-it", nil },
		verifyPRMatchesBranch: func(context.Context, string, string, int, string) (bool, error) { return true, nil },
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
	tmpDir, err := os.MkdirTemp("", "backlog-persist-fail-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, fmt.Sprintf("test-%d.db", time.Now().UnixNano()))
	repo, err := session.NewEntRepository(session.WithDatabasePath(dbPath))
	require.NoError(t, err)
	storage, err := session.NewStorageWithRepository(repo)
	require.NoError(t, err)

	item, sessionUUID := setupReportPRCreatedFixture(t, storage, session.BacklogStatusReview)

	// Close the DB connection now that fixture setup succeeded — every
	// subsequent storage call inside reportPRCreated (including the primary
	// PR-field/transition write) will fail.
	require.NoError(t, repo.Close())

	handler := &backlogHandlers{
		storage:               storage,
		resolveSessionBranch:  func(context.Context, string) (string, error) { return "backlog/ship-it", nil },
		verifyPRMatchesBranch: func(context.Context, string, string, int, string) (bool, error) { return true, nil },
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
		verifyPRMatchesBranch: func(context.Context, string, string, int, string) (bool, error) {
			verifyCalled = true
			return true, nil
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
		verifyPRMatchesBranch: func(_ context.Context, _, _ string, _ int, expectedBranch string) (bool, error) {
			assert.Equal(t, "backlog/ship-it", expectedBranch)
			return false, nil // definitive mismatch — a real PR exists, but for a different branch/number
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
		verifyPRMatchesBranch: func(context.Context, string, string, int, string) (bool, error) {
			return false, fmt.Errorf("GitHub API: rate limited (403)")
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
