package session

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/instance"
)

// ─── I-GO-01: TestSetGetSessionGoal_roundTrip ─────────────────────────────────

func TestSetGetSessionGoal_roundTrip(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	sessionUUID := "test-session-uuid-01"
	tasks := []instance.TaskNode{
		{ID: "t1", Title: "First task", Status: instance.TaskStatusPending},
	}

	got, err := storage.SetSessionGoal(context.Background(), sessionUUID, "implement feature X", instance.GoalStatusWorking, tasks, "user", "")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, sessionUUID, got.SessionUUID)
	assert.Equal(t, "implement feature X", got.Goal)
	assert.Equal(t, instance.GoalStatusWorking, got.Status)
	assert.Equal(t, 1, len(got.Tasks))

	loaded, err := storage.GetSessionGoal(context.Background(), sessionUUID)
	require.NoError(t, err)
	assert.Equal(t, got.Goal, loaded.Goal)
	assert.Equal(t, got.Status, loaded.Status)
	assert.Equal(t, got.SessionUUID, loaded.SessionUUID)
	assert.Equal(t, 1, len(loaded.Tasks))
	assert.Equal(t, "t1", loaded.Tasks[0].ID)
}

// ─── I-GO-02: TestSetSessionGoal_upsertReplacesPrevious ───────────────────────

func TestSetSessionGoal_upsertReplacesPrevious(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	sessionUUID := "test-session-uuid-02"

	_, err := storage.SetSessionGoal(context.Background(), sessionUUID, "first goal", instance.GoalStatusIdle, nil, "", "")
	require.NoError(t, err)

	_, err = storage.SetSessionGoal(context.Background(), sessionUUID, "second goal", instance.GoalStatusWorking, nil, "", "")
	require.NoError(t, err)

	loaded, err := storage.GetSessionGoal(context.Background(), sessionUUID)
	require.NoError(t, err)
	assert.Equal(t, "second goal", loaded.Goal)
	assert.Equal(t, instance.GoalStatusWorking, loaded.Status)
}

// ─── I-GO-03: TestGetSessionGoal_returnsErrNotFoundWhenAbsent ────────────────

func TestGetSessionGoal_returnsErrNotFoundWhenAbsent(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	_, err := storage.GetSessionGoal(context.Background(), "nonexistent-session-uuid")
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrNotFound), "expected ErrNotFound, got: %v", err)
}

// ─── I-GO-04: TestUpdateSessionTaskStatus_updatesCorrectTaskByID ─────────────

func TestUpdateSessionTaskStatus_updatesCorrectTaskByID(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	sessionUUID := "test-session-uuid-04"
	tasks := []instance.TaskNode{
		{ID: "t1", Title: "Task one", Status: instance.TaskStatusPending},
		{ID: "t2", Title: "Task two", Status: instance.TaskStatusPending},
	}
	_, err := storage.SetSessionGoal(context.Background(), sessionUUID, "my goal", instance.GoalStatusWorking, tasks, "", "")
	require.NoError(t, err)

	updated, err := storage.UpdateSessionTaskStatus(context.Background(), sessionUUID, "t1", instance.TaskStatusDone)
	require.NoError(t, err)

	assert.Equal(t, instance.TaskStatusDone, updated.Tasks[0].Status)
	assert.Equal(t, instance.TaskStatusPending, updated.Tasks[1].Status, "t2 should be unchanged")
}

// ─── I-GO-05: TestUpdateSessionTaskStatus_updatesNestedTaskByID ──────────────

func TestUpdateSessionTaskStatus_updatesNestedTaskByID(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	sessionUUID := "test-session-uuid-05"
	tasks := []instance.TaskNode{
		{ID: "parent", Title: "Parent task", Status: instance.TaskStatusInProgress, Children: []instance.TaskNode{
			{ID: "child1", Title: "Child task 1", Status: instance.TaskStatusPending},
		}},
	}
	_, err := storage.SetSessionGoal(context.Background(), sessionUUID, "nested goal", instance.GoalStatusWorking, tasks, "", "")
	require.NoError(t, err)

	updated, err := storage.UpdateSessionTaskStatus(context.Background(), sessionUUID, "child1", instance.TaskStatusDone)
	require.NoError(t, err)

	require.Equal(t, 1, len(updated.Tasks))
	require.Equal(t, 1, len(updated.Tasks[0].Children))
	assert.Equal(t, instance.TaskStatusDone, updated.Tasks[0].Children[0].Status)
}

// ─── I-GO-06: TestUpdateSessionTaskStatus_returnsErrorWhenTaskIDNotFound ─────

func TestUpdateSessionTaskStatus_returnsErrorWhenTaskIDNotFound(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	sessionUUID := "test-session-uuid-06"
	_, err := storage.SetSessionGoal(context.Background(), sessionUUID, "a goal", instance.GoalStatusIdle, nil, "", "")
	require.NoError(t, err)

	_, err = storage.UpdateSessionTaskStatus(context.Background(), sessionUUID, "nonexistent-task-id", instance.TaskStatusDone)
	require.Error(t, err)
}

// ─── I-GO-07/08: TestInitialPromptPersistenceRoundTrip ───────────────────────

func TestInitialPromptPersistenceRoundTrip(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	inst := &instance.Instance{
		Title:         "prompt-roundtrip",
		UUID:          "test-uuid-prompt-rt",
		Path:          "/tmp/test",
		Status:        domain.Paused,
		Program:       "claude",
		InitialPrompt: "please do the thing",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	inst.MarkStarted()

	require.NoError(t, storage.AddInstance(inst))

	loaded, err := storage.LoadInstances()
	require.NoError(t, err)

	var found *instance.Instance
	for _, s := range loaded {
		if s.UUID == inst.UUID {
			found = s
			break
		}
	}
	require.NotNil(t, found, "loaded instance not found")
	assert.Equal(t, "please do the thing", found.InitialPrompt)
}

// ─── I-GO-09: TestSessionGoalLoadedFromEntOnStartup ──────────────────────────

func TestSessionGoalLoadedFromEntOnStartup(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	// Create a session.
	inst := &instance.Instance{
		Title:     "goal-load-test",
		UUID:      "test-uuid-goal-load",
		Path:      "/tmp/test",
		Status:    domain.Paused,
		Program:   "claude",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	inst.MarkStarted()
	require.NoError(t, storage.AddInstance(inst))

	// Set a goal.
	_, err := storage.SetSessionGoal(context.Background(), inst.UUID, "the goal", instance.GoalStatusWorking, nil, "", "")
	require.NoError(t, err)

	// LoadInstances should populate SessionGoal.
	instances, err := storage.LoadInstances()
	require.NoError(t, err)

	var found *instance.Instance
	for _, s := range instances {
		if s.UUID == inst.UUID {
			found = s
			break
		}
	}
	require.NotNil(t, found)
	require.NotNil(t, found.GetSessionGoal(), "SessionGoal should be populated after LoadInstances")
	assert.Equal(t, "the goal", found.GetSessionGoal().Goal)
}

// ─── I-GO-10/11: Validation in storage ───────────────────────────────────────

func TestSetSessionGoal_validatesMaxTaskCount(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	tasks := make([]instance.TaskNode, 51)
	for i := range tasks {
		tasks[i] = instance.TaskNode{ID: "t", Title: "task", Status: instance.TaskStatusPending}
	}
	_, err := storage.SetSessionGoal(context.Background(), "test-uuid-count", "goal", instance.GoalStatusIdle, tasks, "", "")
	require.Error(t, err)
}

func TestSetSessionGoal_validatesMaxTaskDepth(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	// Build depth-4 tree (exceeds maxTaskDepth=3).
	tasks := []instance.TaskNode{
		{ID: "l1", Title: "L1", Status: instance.TaskStatusPending, Children: []instance.TaskNode{
			{ID: "l2", Title: "L2", Status: instance.TaskStatusPending, Children: []instance.TaskNode{
				{ID: "l3", Title: "L3", Status: instance.TaskStatusPending, Children: []instance.TaskNode{
					{ID: "l4", Title: "L4", Status: instance.TaskStatusPending},
				}},
			}},
		}},
	}
	_, err := storage.SetSessionGoal(context.Background(), "test-uuid-depth", "goal", instance.GoalStatusIdle, tasks, "", "")
	require.Error(t, err)
}
