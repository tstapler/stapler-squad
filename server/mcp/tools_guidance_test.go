package mcp

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session"
)

// newTestGuidanceStorage creates a temporary Storage for guidance tool tests,
// mirroring newTestGoalStorage/newTestBacklogStorage's established pattern.
func newTestGuidanceStorage(t *testing.T) *session.Storage {
	t.Helper()
	repo := session.NewTestEntRepository(t)
	storage, err := session.NewStorageWithRepository(repo)
	require.NoError(t, err)
	return storage
}

// --- create_guidance_request: one happy path per question kind ---

func TestCreateGuidanceRequest_should_CreatePendingRequest_When_QuestionTypeYesNo(t *testing.T) {
	t.Parallel()
	storage := newTestGuidanceStorage(t)
	h := &backlogHandlers{storage: storage}
	callerUUID := uuid.New().String()
	ctx := WithSessionUUID(context.Background(), callerUUID)

	result, err := h.createGuidanceRequest(ctx, makeToolReq(map[string]interface{}{
		"scope":         "standalone",
		"question_text": "Deploy to prod now?",
		"question_type": "yes-no",
	}))
	require.NoError(t, err)

	m := parseResult(t, result)
	require.True(t, m["success"].(bool), "expected success, got: %v", m)
	assert.Equal(t, "standalone", m["scope"])
	assert.Equal(t, "yes-no", m["question_type"])
	assert.Equal(t, "pending", m["status"])
	assert.NotEmpty(t, m["id"])
}

func TestCreateGuidanceRequest_should_CreatePendingRequest_When_QuestionTypeMultipleChoice(t *testing.T) {
	t.Parallel()
	storage := newTestGuidanceStorage(t)
	h := &backlogHandlers{storage: storage}
	callerUUID := uuid.New().String()
	ctx := WithSessionUUID(context.Background(), callerUUID)

	result, err := h.createGuidanceRequest(ctx, makeToolReq(map[string]interface{}{
		"scope":         "standalone",
		"question_text": "Which approach?",
		"question_type": "multiple-choice",
		"options":       []interface{}{"approach A", "approach B"},
	}))
	require.NoError(t, err)

	m := parseResult(t, result)
	require.True(t, m["success"].(bool), "expected success, got: %v", m)
	assert.Equal(t, "multiple-choice", m["question_type"])
	opts, ok := m["options"].([]interface{})
	require.True(t, ok, "expected options to be present in the result")
	assert.Equal(t, []interface{}{"approach A", "approach B"}, opts)
}

func TestCreateGuidanceRequest_should_CreatePendingRequest_When_QuestionTypeShortAnswer(t *testing.T) {
	t.Parallel()
	storage := newTestGuidanceStorage(t)
	h := &backlogHandlers{storage: storage}
	callerUUID := uuid.New().String()
	ctx := WithSessionUUID(context.Background(), callerUUID)

	result, err := h.createGuidanceRequest(ctx, makeToolReq(map[string]interface{}{
		"scope":         "session",
		"question_text": "What should the migration name be?",
		"question_type": "short-answer",
	}))
	require.NoError(t, err)

	m := parseResult(t, result)
	require.True(t, m["success"].(bool), "expected success, got: %v", m)
	assert.Equal(t, "session", m["scope"])
	assert.Equal(t, "short-answer", m["question_type"])
	// scope=session fills in.SessionUUID from the caller, per
	// applyGuidanceRequestScopeFields.
	assert.Equal(t, callerUUID, m["session_uuid"])
}

// --- create_guidance_request: malformed/unparseable options ---

// TestParseGuidanceOptionsArg_should_ReturnEmpty_When_OptionsArgIsNotAnArray
// covers parseGuidanceOptionsArg directly for the shapes the MCP framework
// can hand it that aren't the expected []any of strings: a bare string, an
// array of non-string elements, and an empty array.
func TestParseGuidanceOptionsArg_should_ReturnEmpty_When_OptionsArgIsNotAnArray(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		args map[string]any
	}{
		{"options is a bare string, not an array", map[string]any{"options": "not-an-array"}},
		{"options array contains only non-string elements", map[string]any{"options": []any{1, 2, 3}}},
		{"options is an empty array", map[string]any{"options": []any{}}},
		{"options key absent", map[string]any{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, "", parseGuidanceOptionsArg(tt.args))
		})
	}
}

// TestCreateGuidanceRequest_should_SucceedWithNoOptions_When_OptionsArgIsUnparseable
// confirms create_guidance_request degrades gracefully (creates the request
// with empty options) rather than erroring when "options" is present but not
// the expected array-of-strings shape.
func TestCreateGuidanceRequest_should_SucceedWithNoOptions_When_OptionsArgIsUnparseable(t *testing.T) {
	t.Parallel()
	storage := newTestGuidanceStorage(t)
	h := &backlogHandlers{storage: storage}
	callerUUID := uuid.New().String()
	ctx := WithSessionUUID(context.Background(), callerUUID)

	result, err := h.createGuidanceRequest(ctx, makeToolReq(map[string]interface{}{
		"scope":         "standalone",
		"question_text": "Which approach?",
		"question_type": "multiple-choice",
		"options":       "not-an-array",
	}))
	require.NoError(t, err)

	m := parseResult(t, result)
	require.True(t, m["success"].(bool), "expected success even with an unparseable options arg, got: %v", m)
	_, hasOptions := m["options"]
	assert.False(t, hasOptions, "options must be omitted (empty), not error, for an unparseable options arg")
}

// --- create_guidance_request: validation errors ---

func TestCreateGuidanceRequest_should_ReturnError_When_ScopeInvalid(t *testing.T) {
	t.Parallel()
	storage := newTestGuidanceStorage(t)
	h := &backlogHandlers{storage: storage}
	ctx := WithSessionUUID(context.Background(), uuid.New().String())

	result, err := h.createGuidanceRequest(ctx, makeToolReq(map[string]interface{}{
		"scope":         "not-a-real-scope",
		"question_text": "Deploy?",
		"question_type": "yes-no",
	}))
	require.NoError(t, err)
	m := parseResult(t, result)
	assert.False(t, m["success"].(bool))
}

func TestCreateGuidanceRequest_should_ReturnError_When_NoCallerSessionUUID(t *testing.T) {
	t.Parallel()
	storage := newTestGuidanceStorage(t)
	h := &backlogHandlers{storage: storage}

	result, err := h.createGuidanceRequest(context.Background(), makeToolReq(map[string]interface{}{
		"scope":         "standalone",
		"question_text": "Deploy?",
		"question_type": "yes-no",
	}))
	require.NoError(t, err)
	m := parseResult(t, result)
	assert.False(t, m["success"].(bool))
}

// --- create_guidance_request: scope=backlog-item ownership (AC6) ---

func TestCreateGuidanceRequest_should_Succeed_When_BacklogItemScopeCallerIsLinked(t *testing.T) {
	t.Parallel()
	storage := newTestGuidanceStorage(t)
	h := &backlogHandlers{storage: storage}
	ctx := context.Background()

	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{Title: "guidance mcp test item"})
	require.NoError(t, err)
	callerUUID := uuid.New().String()
	_, err = storage.CreateItemSession(ctx, session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: callerUUID,
		SessionRole: "work",
	})
	require.NoError(t, err)

	result, err := h.createGuidanceRequest(WithSessionUUID(ctx, callerUUID), makeToolReq(map[string]interface{}{
		"scope":         "backlog-item",
		"item_id":       item.ID,
		"question_text": "Merge now?",
		"question_type": "yes-no",
	}))
	require.NoError(t, err)
	m := parseResult(t, result)
	require.True(t, m["success"].(bool), "expected success, got: %v", m)
	assert.Equal(t, item.ID, m["item_id"])
}

func TestCreateGuidanceRequest_should_ReturnPermissionDenied_When_BacklogItemScopeCallerNotLinked(t *testing.T) {
	t.Parallel()
	storage := newTestGuidanceStorage(t)
	h := &backlogHandlers{storage: storage}
	ctx := context.Background()

	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{Title: "guidance mcp unlinked item"})
	require.NoError(t, err)
	callerUUID := uuid.New().String()

	result, err := h.createGuidanceRequest(WithSessionUUID(ctx, callerUUID), makeToolReq(map[string]interface{}{
		"scope":         "backlog-item",
		"item_id":       item.ID,
		"question_text": "Merge now?",
		"question_type": "yes-no",
	}))
	require.NoError(t, err)
	m := parseResult(t, result)
	require.False(t, m["success"].(bool))
	errMap, ok := m["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, ErrPermissionDenied, errMap["code"])
}

// --- get_guidance_request: round trip through create/get ---

func TestGetGuidanceRequest_should_RoundTrip_When_RequestWasJustCreated(t *testing.T) {
	t.Parallel()
	storage := newTestGuidanceStorage(t)
	h := &backlogHandlers{storage: storage}
	ctx := WithSessionUUID(context.Background(), uuid.New().String())

	created, err := h.createGuidanceRequest(ctx, makeToolReq(map[string]interface{}{
		"scope":         "standalone",
		"question_text": "Round trip question?",
		"question_type": "yes-no",
	}))
	require.NoError(t, err)
	createdM := parseResult(t, created)
	require.True(t, createdM["success"].(bool))
	id, ok := createdM["id"].(string)
	require.True(t, ok)
	require.NotEmpty(t, id)

	fetched, err := h.getGuidanceRequest(context.Background(), makeToolReq(map[string]interface{}{
		"question_id": id,
	}))
	require.NoError(t, err)
	fetchedM := parseResult(t, fetched)
	require.True(t, fetchedM["success"].(bool), "expected success, got: %v", fetchedM)
	assert.Equal(t, id, fetchedM["id"])
	assert.Equal(t, "Round trip question?", fetchedM["question_text"])
	assert.Equal(t, "pending", fetchedM["status"])
}

func TestGetGuidanceRequest_should_ReturnError_When_IdNotFound(t *testing.T) {
	t.Parallel()
	storage := newTestGuidanceStorage(t)
	h := &backlogHandlers{storage: storage}

	result, err := h.getGuidanceRequest(context.Background(), makeToolReq(map[string]interface{}{
		"question_id": uuid.New().String(),
	}))
	require.NoError(t, err)
	m := parseResult(t, result)
	assert.False(t, m["success"].(bool))
}

func TestGetGuidanceRequest_should_ReturnError_When_QuestionIdMissing(t *testing.T) {
	t.Parallel()
	storage := newTestGuidanceStorage(t)
	h := &backlogHandlers{storage: storage}

	result, err := h.getGuidanceRequest(context.Background(), makeToolReq(map[string]interface{}{}))
	require.NoError(t, err)
	m := parseResult(t, result)
	assert.False(t, m["success"].(bool))
}
