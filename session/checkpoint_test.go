package session

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckpoint_SerializationRoundtrip(t *testing.T) {
	cp := Checkpoint{
		ID:             "550e8400-e29b-41d4-a716-446655440000",
		SessionID:      "test-session-id",
		Label:          "before-refactor",
		ScrollbackSeq:  42,
		ScrollbackPath: "/tmp/scrollback.jsonl",
		ClaudeConvUUID: "660e8400-e29b-41d4-a716-446655440000",
		GitCommitSHA:   "abc123def456",
		Timestamp:      time.Now().UTC().Truncate(time.Millisecond),
	}

	data, err := json.Marshal(cp)
	require.NoError(t, err)

	var restored Checkpoint
	err = json.Unmarshal(data, &restored)
	require.NoError(t, err)

	assert.Equal(t, cp.ID, restored.ID)
	assert.Equal(t, cp.SessionID, restored.SessionID)
	assert.Equal(t, cp.Label, restored.Label)
	assert.Equal(t, cp.ScrollbackSeq, restored.ScrollbackSeq)
	assert.Equal(t, cp.ClaudeConvUUID, restored.ClaudeConvUUID)
	assert.Equal(t, cp.GitCommitSHA, restored.GitCommitSHA)
	assert.True(t, cp.Timestamp.Equal(restored.Timestamp))
}

func TestInstanceData_BackwardCompatNilCheckpoints(t *testing.T) {
	// Old format without checkpoints field
	oldJSON := `{"title":"test","path":"/tmp","status":0}`
	var data InstanceData
	err := json.Unmarshal([]byte(oldJSON), &data)
	require.NoError(t, err)
	assert.Nil(t, data.Checkpoints)
	assert.Empty(t, data.ActiveCheckpoint)
}

func TestCheckpointList_FindByID(t *testing.T) {
	cl := CheckpointList{
		{ID: "id-1", Label: "first"},
		{ID: "id-2", Label: "second"},
		{ID: "id-3", Label: "third"},
	}

	result := cl.FindByID("id-2")
	require.NotNil(t, result)
	assert.Equal(t, "second", result.Label)

	result = cl.FindByID("non-existent")
	assert.Nil(t, result)
}

func TestCheckpointList_FindByLabel(t *testing.T) {
	cl := CheckpointList{
		{ID: "id-1", Label: "first"},
		{ID: "id-2", Label: "second"},
		{ID: "id-3", Label: "first"}, // duplicate label
	}

	result := cl.FindByLabel("first")
	require.NotNil(t, result)
	assert.Equal(t, "id-1", result.ID) // first match

	result = cl.FindByLabel("non-existent")
	assert.Nil(t, result)
}

func TestCheckpointList_Latest(t *testing.T) {
	now := time.Now()
	cl := CheckpointList{
		{ID: "id-1", Timestamp: now.Add(-2 * time.Hour)},
		{ID: "id-2", Timestamp: now.Add(-1 * time.Hour)},
		{ID: "id-3", Timestamp: now.Add(-3 * time.Hour)},
	}

	latest := cl.Latest()
	require.NotNil(t, latest)
	assert.Equal(t, "id-2", latest.ID)
}

func TestCheckpointList_Latest_Empty(t *testing.T) {
	var cl CheckpointList
	latest := cl.Latest()
	assert.Nil(t, latest)
}

// --- Story 1.3.2d: Instance.CreateCheckpoint tests ---

func TestCreateCheckpoint_UnstartedInstance_ReturnsError(t *testing.T) {
	inst := &Instance{Title: "test-session"}
	// inst.started == false by default

	cp, err := inst.CreateCheckpoint("my-label", 0)

	require.Error(t, err)
	assert.Nil(t, cp)
	assert.Contains(t, err.Error(), "unstarted")
}

func TestCreateCheckpoint_StartedInstance_AllFieldsPopulated(t *testing.T) {
	inst := &Instance{
		Title: "test-session",
	}
	inst.started.Store(true)
	// Set conversation UUID so it gets captured.
	inst.claudeSession = &ClaudeSessionData{ConversationUUID: "conv-uuid-123"}

	before := time.Now()
	cp, err := inst.CreateCheckpoint("before-refactor", 42)
	after := time.Now()

	require.NoError(t, err)
	require.NotNil(t, cp)

	assert.NotEmpty(t, cp.ID, "checkpoint ID should be set")
	assert.Equal(t, "test-session", cp.SessionID)
	assert.Equal(t, "before-refactor", cp.Label)
	assert.Equal(t, uint64(42), cp.ScrollbackSeq)
	assert.Equal(t, "conv-uuid-123", cp.ClaudeConvUUID)
	// GitCommitSHA may be empty when no worktree is set — that's OK.
	assert.True(t, !cp.Timestamp.Before(before) && !cp.Timestamp.After(after),
		"timestamp should be within test execution window")
}

func TestCreateCheckpoint_IdIsValidUUID(t *testing.T) {
	inst := &Instance{Title: "test-session"}
	inst.started.Store(true)

	cp, err := inst.CreateCheckpoint("label", 0)

	require.NoError(t, err)
	_, parseErr := uuid.Parse(cp.ID)
	assert.NoError(t, parseErr, "checkpoint ID should be a valid UUID, got: %s", cp.ID)
}

func TestCreateCheckpoint_MultipleCheckpoints_AppendCorrectly(t *testing.T) {
	inst := &Instance{Title: "test-session"}
	inst.started.Store(true)

	cp1, err := inst.CreateCheckpoint("first", 10)
	require.NoError(t, err)

	cp2, err := inst.CreateCheckpoint("second", 20)
	require.NoError(t, err)

	assert.Equal(t, 2, len(inst.Checkpoints))
	assert.Equal(t, cp1.ID, inst.Checkpoints[0].ID)
	assert.Equal(t, cp2.ID, inst.Checkpoints[1].ID)
	assert.NotEqual(t, cp1.ID, cp2.ID, "checkpoint IDs should be unique")
}

func TestCreateCheckpoint_ActiveCheckpointUpdated(t *testing.T) {
	inst := &Instance{Title: "test-session"}
	inst.started.Store(true)

	cp1, _ := inst.CreateCheckpoint("first", 0)
	assert.Equal(t, cp1.ID, inst.ActiveCheckpoint)

	cp2, _ := inst.CreateCheckpoint("second", 0)
	assert.Equal(t, cp2.ID, inst.ActiveCheckpoint, "active checkpoint should be updated to latest")
}

func TestCreateCheckpoint_NoConversationUUID_EmptyField(t *testing.T) {
	inst := &Instance{Title: "test-session"}
	inst.started.Store(true)
	// claudeSession is nil — no UUID available yet.

	cp, err := inst.CreateCheckpoint("early-checkpoint", 5)

	require.NoError(t, err)
	assert.Empty(t, cp.ClaudeConvUUID, "conv UUID should be empty when session not linked")
}
