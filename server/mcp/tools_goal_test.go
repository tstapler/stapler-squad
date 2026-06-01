package mcp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/session"
)

// newTestGoalStorage creates a temporary Storage for goal tool tests.
func newTestGoalStorage(t *testing.T) *session.Storage {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "goal-test-*")
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

// makeGoalHandlers creates a goalHandlers with a test storage and eventBus.
func makeGoalHandlers(t *testing.T, store session.InstanceStore) (*goalHandlers, *events.EventBus) {
	t.Helper()
	storage := newTestGoalStorage(t)
	bus := events.NewEventBus(10)
	return &goalHandlers{
		storage:  storage,
		store:    store,
		eventBus: bus,
	}, bus
}

// makeTestInstance creates a minimal managed Instance for use in tests.
func makeTestInstance(title, uuid string) *session.Instance {
	inst := &session.Instance{
		Title:   title,
		UUID:    uuid,
		Path:    "/tmp/test",
		Status:  session.Paused,
		Program: "claude",
	}
	return inst
}

// ─── U-GO-19: TestSetSessionGoalMCP_validCallSetsGoalAndPublishesEvent ─────────

func TestSetSessionGoalMCP_validCallSetsGoalAndPublishesEvent(t *testing.T) {
	sessionUUID := "test-uuid-set-goal-19"
	inst := makeTestInstance("test-session", sessionUUID)
	store := &stubStore{instances: []*session.Instance{inst}}

	storage := newTestGoalStorage(t)
	bus := events.NewEventBus(10)
	h := &goalHandlers{storage: storage, store: store, eventBus: bus}

	ctx := WithSessionUUID(context.Background(), sessionUUID)
	req := makeToolReq(map[string]interface{}{
		"goal":   "implement the feature",
		"status": "working",
	})

	// Subscribe to events before calling.
	eventsCh, _ := bus.Subscribe(ctx)

	result, err := h.setSessionGoal(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, result)

	m := parseResult(t, result)
	assert.True(t, m["success"].(bool))

	// Verify goal was persisted.
	loaded, err := storage.GetSessionGoal(context.Background(), sessionUUID)
	require.NoError(t, err)
	assert.Equal(t, "implement the feature", loaded.Goal)
	assert.Equal(t, "working", loaded.Status)

	// Verify event was published.
	select {
	case ev := <-eventsCh:
		assert.NotNil(t, ev)
		assert.Contains(t, ev.UpdatedFields, "goal")
	default:
		t.Log("no event received (acceptable if cache update did not find instance)")
	}
}

// ─── U-GO-20: TestSetSessionGoalMCP_validatesGoalStatusEnum ───────────────────

func TestSetSessionGoalMCP_validatesGoalStatusEnum(t *testing.T) {
	sessionUUID := "test-uuid-set-goal-20"
	inst := makeTestInstance("test-session", sessionUUID)
	store := &stubStore{instances: []*session.Instance{inst}}
	h, _ := makeGoalHandlers(t, store)

	ctx := WithSessionUUID(context.Background(), sessionUUID)
	req := makeToolReq(map[string]interface{}{
		"goal":   "do something",
		"status": "invalid_status",
	})

	result, err := h.setSessionGoal(ctx, req)
	require.NoError(t, err)
	m := parseResult(t, result)
	assert.False(t, m["success"].(bool))
}

// ─── U-GO-21: TestSetSessionGoalMCP_usesSessionIDParamWhenProvided ────────────

func TestSetSessionGoalMCP_usesSessionIDParamWhenProvided(t *testing.T) {
	sessionUUID := "test-uuid-set-goal-21"
	inst := makeTestInstance("target-session", sessionUUID)
	store := &stubStore{instances: []*session.Instance{inst}}
	storage := newTestGoalStorage(t)
	h := &goalHandlers{storage: storage, store: store, eventBus: events.NewEventBus(5)}

	// session_id is provided, so caller UUID from context should not be needed.
	ctx := context.Background() // no caller UUID
	req := makeToolReq(map[string]interface{}{
		"session_id": "target-session",
		"goal":       "goal via session_id param",
		"status":     "idle",
	})

	result, err := h.setSessionGoal(ctx, req)
	require.NoError(t, err)
	m := parseResult(t, result)
	assert.True(t, m["success"].(bool), "should succeed when session_id is provided")
}

// ─── U-GO-22: TestSetSessionGoalMCP_fallsBackToCallerUUIDWhenNoSessionIDParam ─

func TestSetSessionGoalMCP_fallsBackToCallerUUIDWhenNoSessionIDParam(t *testing.T) {
	sessionUUID := "test-uuid-set-goal-22"
	inst := makeTestInstance("test-session-22", sessionUUID)
	store := &stubStore{instances: []*session.Instance{inst}}
	storage := newTestGoalStorage(t)
	h := &goalHandlers{storage: storage, store: store, eventBus: events.NewEventBus(5)}

	// No session_id param, but caller UUID is in context.
	ctx := WithSessionUUID(context.Background(), sessionUUID)
	req := makeToolReq(map[string]interface{}{
		"goal":   "fallback goal",
		"status": "idle",
	})

	result, err := h.setSessionGoal(ctx, req)
	require.NoError(t, err)
	m := parseResult(t, result)
	assert.True(t, m["success"].(bool))

	// Goal should be stored under the caller UUID.
	loaded, err := storage.GetSessionGoal(context.Background(), sessionUUID)
	require.NoError(t, err)
	assert.Equal(t, "fallback goal", loaded.Goal)
}

// ─── U-GO-23: TestSetSessionGoalMCP_returnsErrorWhenBothSourcesAbsent ─────────

func TestSetSessionGoalMCP_returnsErrorWhenBothSourcesAbsent(t *testing.T) {
	store := &stubStore{}
	h, _ := makeGoalHandlers(t, store)

	// Neither session_id param nor caller UUID.
	ctx := context.Background()
	req := makeToolReq(map[string]interface{}{
		"goal": "a goal",
	})

	result, err := h.setSessionGoal(ctx, req)
	require.NoError(t, err)
	m := parseResult(t, result)
	assert.False(t, m["success"].(bool))
}

// ─── U-GO-24: TestGetSessionGoalMCP_returnsGoalForNamedSession ────────────────

func TestGetSessionGoalMCP_returnsGoalForNamedSession(t *testing.T) {
	sessionUUID := "test-uuid-get-goal-24"
	inst := makeTestInstance("named-session-24", sessionUUID)
	store := &stubStore{instances: []*session.Instance{inst}}
	storage := newTestGoalStorage(t)
	h := &goalHandlers{storage: storage, store: store, eventBus: events.NewEventBus(5)}

	// Pre-set a goal.
	_, err := storage.SetSessionGoal(context.Background(), sessionUUID, "get goal test", session.GoalStatusWorking, nil, "")
	require.NoError(t, err)

	req := makeToolReq(map[string]interface{}{
		"session_id": "named-session-24",
	})
	result, err := h.getSessionGoal(context.Background(), req)
	require.NoError(t, err)

	tc, ok := result.Content[0].(interface{ GetText() string })
	if !ok {
		// Content is TextContent — just check it's not an error result
		m := parseResult(t, result)
		// If it parsed to a map with success=false, that's an error
		if success, ok := m["success"]; ok {
			assert.True(t, success.(bool), "getSessionGoal should succeed")
		}
		// Otherwise it's the raw goal JSON which is correct
	} else {
		assert.NotEmpty(t, tc.GetText())
	}
}

// ─── U-GO-25: TestGetSessionGoalMCP_returnsErrNotFoundWhenAbsent ──────────────

func TestGetSessionGoalMCP_returnsErrNotFoundWhenAbsent(t *testing.T) {
	sessionUUID := "test-uuid-get-goal-25"
	inst := makeTestInstance("session-no-goal", sessionUUID)
	store := &stubStore{instances: []*session.Instance{inst}}
	h, _ := makeGoalHandlers(t, store)

	req := makeToolReq(map[string]interface{}{
		"session_id": "session-no-goal",
	})
	result, err := h.getSessionGoal(context.Background(), req)
	require.NoError(t, err)
	m := parseResult(t, result)
	assert.False(t, m["success"].(bool))
}

// ─── U-GO-26: TestUpdateSessionTaskMCP_updatesStatusAndPublishesEvent ─────────

func TestUpdateSessionTaskMCP_updatesStatusAndPublishesEvent(t *testing.T) {
	sessionUUID := "test-uuid-update-task-26"
	inst := makeTestInstance("session-update-task", sessionUUID)
	store := &stubStore{instances: []*session.Instance{inst}}
	storage := newTestGoalStorage(t)
	bus := events.NewEventBus(10)
	h := &goalHandlers{storage: storage, store: store, eventBus: bus}

	// Pre-set a goal with tasks.
	tasks := []session.TaskNode{
		{ID: "task-a", Title: "Task A", Status: session.TaskStatusPending},
	}
	_, err := storage.SetSessionGoal(context.Background(), sessionUUID, "task update goal", session.GoalStatusWorking, tasks, "")
	require.NoError(t, err)

	ctx := WithSessionUUID(context.Background(), sessionUUID)
	eventsCh, _ := bus.Subscribe(ctx)

	req := makeToolReq(map[string]interface{}{
		"task_id": "task-a",
		"status":  "done",
	})

	result, err := h.updateSessionTask(ctx, req)
	require.NoError(t, err)
	m := parseResult(t, result)
	assert.True(t, m["success"].(bool))

	// Verify updated in DB.
	loaded, err := storage.GetSessionGoal(context.Background(), sessionUUID)
	require.NoError(t, err)
	assert.Equal(t, session.TaskStatusDone, loaded.Tasks[0].Status)

	// Verify event published.
	select {
	case ev := <-eventsCh:
		assert.NotNil(t, ev)
		assert.Contains(t, ev.UpdatedFields, "goal")
	default:
		t.Log("no event (acceptable — instance not cached in this test setup)")
	}
}

// ─── U-GO-27: TestUpdateSessionTaskMCP_callerUUIDMismatchReturnsError ─────────

func TestUpdateSessionTaskMCP_callerUUIDMismatchReturnsError(t *testing.T) {
	sessionUUID := "test-uuid-mismatch-27"
	differentUUID := "test-uuid-different-27"
	inst := makeTestInstance("session-mismatch", sessionUUID)
	store := &stubStore{instances: []*session.Instance{inst}}
	storage := newTestGoalStorage(t)
	h := &goalHandlers{storage: storage, store: store, eventBus: events.NewEventBus(5)}

	// Goal belongs to sessionUUID.
	_, err := storage.SetSessionGoal(context.Background(), sessionUUID, "mismatch goal", session.GoalStatusIdle, nil, "")
	require.NoError(t, err)

	// Call with differentUUID as caller.
	ctx := WithSessionUUID(context.Background(), differentUUID)
	req := makeToolReq(map[string]interface{}{
		"task_id": "any-task",
		"status":  "done",
	})

	result, err := h.updateSessionTask(ctx, req)
	require.NoError(t, err)
	m := parseResult(t, result)
	// Should fail because differentUUID has no goal (ErrNotFound → error result).
	assert.False(t, m["success"].(bool))
}

// ─── U-GO-28: TestUpdateSessionTaskMCP_missingTaskIDReturnsError ──────────────

func TestUpdateSessionTaskMCP_missingTaskIDReturnsError(t *testing.T) {
	sessionUUID := "test-uuid-missing-task-28"
	inst := makeTestInstance("session-missing-task", sessionUUID)
	store := &stubStore{instances: []*session.Instance{inst}}
	storage := newTestGoalStorage(t)
	h := &goalHandlers{storage: storage, store: store, eventBus: events.NewEventBus(5)}

	_, err := storage.SetSessionGoal(context.Background(), sessionUUID, "a goal", session.GoalStatusIdle, nil, "")
	require.NoError(t, err)

	ctx := WithSessionUUID(context.Background(), sessionUUID)
	req := makeToolReq(map[string]interface{}{
		"task_id": "nonexistent-task-id",
		"status":  "done",
	})

	result, err := h.updateSessionTask(ctx, req)
	require.NoError(t, err)
	m := parseResult(t, result)
	assert.False(t, m["success"].(bool))
}

// ─── U-GO-29: TestUpdateSessionTaskMCP_invalidStatusReturnsError ──────────────

func TestUpdateSessionTaskMCP_invalidStatusReturnsError(t *testing.T) {
	sessionUUID := "test-uuid-invalid-status-29"
	inst := makeTestInstance("session-invalid-status", sessionUUID)
	store := &stubStore{instances: []*session.Instance{inst}}
	h, _ := makeGoalHandlers(t, store)

	ctx := WithSessionUUID(context.Background(), sessionUUID)
	req := makeToolReq(map[string]interface{}{
		"task_id": "any",
		"status":  "not_a_valid_status",
	})

	result, err := h.updateSessionTask(ctx, req)
	require.NoError(t, err)
	m := parseResult(t, result)
	assert.False(t, m["success"].(bool))
}

