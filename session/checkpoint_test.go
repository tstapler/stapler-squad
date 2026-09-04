package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/tmux"
)

func TestCheckpoint_SerializationRoundtrip(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	// Old format without checkpoints field
	oldJSON := `{"title":"test","path":"/tmp","status":0}`
	var data InstanceData
	err := json.Unmarshal([]byte(oldJSON), &data)
	require.NoError(t, err)
	assert.Nil(t, data.Checkpoints)
	assert.Empty(t, data.ActiveCheckpoint)
}

func TestCheckpointList_FindByID(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
	var cl CheckpointList
	latest := cl.Latest()
	assert.Nil(t, latest)
}

// --- Story 1.3.2d: Instance.CreateCheckpoint tests ---

func TestCreateCheckpoint_UnstartedInstance_ReturnsError(t *testing.T) {
	t.Parallel()
	inst := &Instance{Title: "test-session"}
	// inst.started == false by default

	cp, err := inst.CreateCheckpoint("my-label", 0)

	require.Error(t, err)
	assert.Nil(t, cp)
	assert.Contains(t, err.Error(), "unstarted")
}

func TestCreateCheckpoint_StartedInstance_AllFieldsPopulated(t *testing.T) {
	t.Parallel()
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

// TestForkFromCheckpoint_HonorsSessionNameOverrideMap verifies that
// ForkFromCheckpoint wires session.ResolveSessionBackend (tymux-bundled-
// integration Epic 4.4.3): with no per-request override concept for this
// restore-from-state path, a TymuxSessionOverrides entry keyed by the
// sanitized tmux session name of the *new* (forked) title still forces the
// forked instance's backend even though the process-wide default is
// registered as tymux.
func TestForkFromCheckpoint_HonorsSessionNameOverrideMap(t *testing.T) {
	RegisterBackendProvider(BackendTymux)
	t.Cleanup(func() { RegisterBackendProvider(BackendTmux) })

	testDir := t.TempDir()
	t.Setenv("STAPLER_SQUAD_TEST_DIR", testDir)

	inst := &Instance{Title: "checkpoint-fork-source"}
	inst.started.Store(true)
	cp, err := inst.CreateCheckpoint("before-fork", 0)
	require.NoError(t, err)
	require.NotNil(t, cp)

	const newTitle = "checkpoint-fork-override-test"
	sessionKey := tmux.NewSessionName(newTitle, tmux.TmuxPrefix).String()
	require.NoError(t, os.WriteFile(filepath.Join(testDir, "config.json"),
		[]byte(`{"tymux_session_overrides": {"`+sessionKey+`": false}}`), 0o644))

	newInst, err := inst.ForkFromCheckpoint(cp.ID, newTitle, t.TempDir())
	require.NoError(t, err)
	require.NotNil(t, newInst)

	assert.Equal(t, BackendTmux, newInst.Backend,
		"a TymuxSessionOverrides entry keyed by the sanitized tmux session name must force the backend even though the process-wide default is tymux")
}

func TestCreateCheckpoint_IdIsValidUUID(t *testing.T) {
	t.Parallel()
	inst := &Instance{Title: "test-session"}
	inst.started.Store(true)

	cp, err := inst.CreateCheckpoint("label", 0)

	require.NoError(t, err)
	_, parseErr := uuid.Parse(cp.ID)
	assert.NoError(t, parseErr, "checkpoint ID should be a valid UUID, got: %s", cp.ID)
}

func TestCreateCheckpoint_MultipleCheckpoints_AppendCorrectly(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	inst := &Instance{Title: "test-session"}
	inst.started.Store(true)

	cp1, _ := inst.CreateCheckpoint("first", 0)
	assert.Equal(t, cp1.ID, inst.ActiveCheckpoint)

	cp2, _ := inst.CreateCheckpoint("second", 0)
	assert.Equal(t, cp2.ID, inst.ActiveCheckpoint, "active checkpoint should be updated to latest")
}

func TestCreateCheckpoint_NoConversationUUID_EmptyField(t *testing.T) {
	t.Parallel()
	inst := &Instance{Title: "test-session"}
	inst.started.Store(true)
	// claudeSession is nil — no UUID available yet.

	cp, err := inst.CreateCheckpoint("early-checkpoint", 5)

	require.NoError(t, err)
	assert.Empty(t, cp.ClaudeConvUUID, "conv UUID should be empty when session not linked")
}

// TestCreateCheckpoint_TitleEscapingConfigDir_RejectsContainment verifies that
// createCheckpointLocked's title->checkpoint-dir containment check actually
// fires: it forces the Claude history adapter's Import to succeed (by seeding
// a real, minimal transcript file under a fake HOME) so the vulnerable
// cpDir-from-title code path is genuinely exercised, then uses a Title that
// escapes configDir and asserts no checkpoint file is written outside it.
func TestCreateCheckpoint_TitleEscapingConfigDir_RejectsContainment(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	testConfigDir := t.TempDir()
	t.Setenv("STAPLER_SQUAD_TEST_DIR", testConfigDir)

	workspace := t.TempDir()
	const convUUID = "conv-checkpoint-escape-test"
	projectDir := filepath.Join(fakeHome, ".claude", "projects", ClaudeProjectDirName(workspace))
	require.NoError(t, os.MkdirAll(projectDir, 0o755))
	// An empty (but present) transcript file makes ClaudeAdapter.Import succeed
	// with zero turns -- enough to drive execution into the cpDir/containment
	// branch without needing to parse real transcript content.
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, convUUID+".jsonl"), []byte{}, 0o644))

	const maliciousTitle = "../ssq-checkpoint-escape-poc"
	inst := &Instance{Title: maliciousTitle, Program: "claude", Path: workspace}
	inst.started.Store(true)
	inst.claudeSession = &ClaudeSessionData{ConversationUUID: convUUID}

	cp, err := inst.CreateCheckpoint("escape-attempt", 0)
	require.NoError(t, err, "containment rejection is swallowed, not propagated as an error")
	require.NotNil(t, cp)
	assert.Empty(t, cp.CanonicalPath, "no checkpoint file should be recorded when the title escapes configDir")

	escapedDir := filepath.Join(filepath.Dir(testConfigDir), "ssq-checkpoint-escape-poc")
	_, statErr := os.Stat(escapedDir)
	assert.True(t, os.IsNotExist(statErr), "no directory should have been created outside configDir at %s", escapedDir)
}

// TestCreateCheckpoint_LegitimateTitle_WritesUnderConfigDir is the control
// case for the test above: a normal title must still produce a real
// checkpoint file under configDir.
func TestCreateCheckpoint_LegitimateTitle_WritesUnderConfigDir(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	testConfigDir := t.TempDir()
	t.Setenv("STAPLER_SQUAD_TEST_DIR", testConfigDir)

	workspace := t.TempDir()
	const convUUID = "conv-checkpoint-legit-test"
	projectDir := filepath.Join(fakeHome, ".claude", "projects", ClaudeProjectDirName(workspace))
	require.NoError(t, os.MkdirAll(projectDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, convUUID+".jsonl"), []byte{}, 0o644))

	const legitTitle = "checkpoint-legit-title"
	inst := &Instance{Title: legitTitle, Program: "claude", Path: workspace}
	inst.started.Store(true)
	inst.claudeSession = &ClaudeSessionData{ConversationUUID: convUUID}

	cp, err := inst.CreateCheckpoint("legit", 0)
	require.NoError(t, err)
	require.NotNil(t, cp)
	require.NotEmpty(t, cp.CanonicalPath, "a legitimate title must still produce a checkpoint file")

	wantDir := filepath.Join(testConfigDir, legitTitle, "checkpoints")
	assert.True(t, strings.HasPrefix(cp.CanonicalPath, wantDir+string(filepath.Separator)),
		"checkpoint path %s should live under %s", cp.CanonicalPath, wantDir)
	_, statErr := os.Stat(cp.CanonicalPath)
	assert.NoError(t, statErr, "checkpoint file should actually exist on disk")
}

// TestForkFromCheckpoint_NewTitleEscapingConfigDir_SkipsScrollbackFork verifies
// ForkFromCheckpoint's isPathUnderDir guard: a newTitle that escapes the
// supplied configDir must not cause the scrollback fork to write outside it,
// even though ForkFromCheckpoint itself still succeeds (scrollback forking is
// best-effort).
func TestForkFromCheckpoint_NewTitleEscapingConfigDir_SkipsScrollbackFork(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	testConfigDir := t.TempDir()

	const srcTitle = "checkpoint-fork-escape-src"
	inst := &Instance{Title: srcTitle}
	inst.started.Store(true)
	cp, err := inst.CreateCheckpoint("before-fork", 7)
	require.NoError(t, err)
	require.NotNil(t, cp)

	srcScrollbackDir := filepath.Join(testConfigDir, srcTitle)
	require.NoError(t, os.MkdirAll(srcScrollbackDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(srcScrollbackDir, "scrollback.jsonl"), []byte(`{"seq":0}`+"\n"), 0o644))

	const maliciousNewTitle = "../ssq-fork-escape-poc"
	newInst, err := inst.ForkFromCheckpoint(cp.ID, maliciousNewTitle, testConfigDir)
	require.NoError(t, err)
	require.NotNil(t, newInst)

	escapedScrollback := filepath.Join(filepath.Dir(testConfigDir), "ssq-fork-escape-poc", "scrollback.jsonl")
	_, statErr := os.Stat(escapedScrollback)
	assert.True(t, os.IsNotExist(statErr), "scrollback must not be forked to a path escaping configDir at %s", escapedScrollback)
}

// TestIsPathUnderDir_RejectsPrefixWithoutSeparator covers the same
// prefix-check-without-separator bug class as isPathUnderAnyRoot: a sibling
// directory sharing a string prefix with dir must not be treated as
// contained within it.
func TestIsPathUnderDir_RejectsPrefixWithoutSeparator(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	assert.True(t, isPathUnderDir(dir, dir), "exact match should be contained")
	assert.True(t, isPathUnderDir(filepath.Join(dir, "sub", "file.jsonl"), dir), "descendant should be contained")
	assert.False(t, isPathUnderDir(filepath.Join(dir, "..", "escaped"), dir), "dot-dot escape should be rejected")
	assert.False(t, isPathUnderDir(filepath.Clean(dir)+"bar", dir), "sibling sharing a string prefix must be rejected")
}
