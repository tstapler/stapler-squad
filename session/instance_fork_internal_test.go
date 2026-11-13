package session

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestForkFromCheckpoint_NoGitSHA_Succeeds(t *testing.T) {
	t.Parallel()
	configDir := t.TempDir()
	srcInst := &Instance{
		Title: "src-session",
		Path:  t.TempDir(),
	}
	srcInst.started.Store(true)

	cp, err := srcInst.CreateCheckpoint("snap", 0)
	require.NoError(t, err)
	// Leave GitCommitSHA empty to test graceful skip.
	assert.Empty(t, cp.GitCommitSHA)

	fork, err := srcInst.ForkFromCheckpoint(cp.ID, "fork-session", configDir)

	require.NoError(t, err)
	require.NotNil(t, fork)
	assert.False(t, fork.gitManager.HasWorktree(), "no git worktree should be created when SHA is empty")
}

func TestForkFromCheckpoint_ForkedFromIDSet(t *testing.T) {
	t.Parallel()
	configDir := t.TempDir()
	srcInst := &Instance{
		Title: "src-session",
		Path:  t.TempDir(),
	}
	srcInst.started.Store(true)

	cp, err := srcInst.CreateCheckpoint("snap", 0)
	require.NoError(t, err)

	fork, err := srcInst.ForkFromCheckpoint(cp.ID, "fork-session", configDir)

	require.NoError(t, err)
	assert.Equal(t, "src-session", fork.ForkedFromID)
}

func TestForkFromCheckpoint_UnknownCheckpointID_ReturnsError(t *testing.T) {
	t.Parallel()
	configDir := t.TempDir()
	srcInst := &Instance{Title: "src-session", Path: t.TempDir()}
	srcInst.started.Store(true)

	_, err := srcInst.ForkFromCheckpoint("nonexistent-id", "fork-session", configDir)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestForkFromCheckpoint_EmptyNewTitle_ReturnsError(t *testing.T) {
	t.Parallel()
	configDir := t.TempDir()
	srcInst := &Instance{Title: "src-session", Path: t.TempDir()}
	srcInst.started.Store(true)
	cp, err := srcInst.CreateCheckpoint("snap", 0)
	require.NoError(t, err)

	_, err = srcInst.ForkFromCheckpoint(cp.ID, "", configDir)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty")
}

// TestForkFromCheckpoint_ConvLineCount_AccurateForLargeLines ensures that
// CreateCheckpoint correctly counts lines even when line content exceeds 64 KB
// (the old bufio.Scanner limit). This regression test guards the fix in
// instance_checkpoint.go that replaced Scanner with bufio.NewReader.
func TestForkFromCheckpoint_ConvLineCount_AccurateForLargeLines(t *testing.T) {
	t.Parallel()
	historyDir := t.TempDir()
	historyFile := filepath.Join(historyDir, "conv-large.jsonl")

	// Write 3 lines where one line is > 64 KB (simulating a tool result with a large payload).
	f, err := os.Create(historyFile)
	require.NoError(t, err)
	bigPayload := make([]byte, 128*1024) // 128 KB
	for i := range bigPayload {
		bigPayload[i] = 'x'
	}
	lines := []string{
		`{"type":"user","timestamp":"2026-01-01T00:00:00Z","message":{"role":"user","content":"q1"}}`,
		fmt.Sprintf(`{"type":"assistant","timestamp":"2026-01-01T00:01:00Z","message":{"role":"assistant","content":[{"type":"text","text":"%s"}]}}`, bigPayload),
		`{"type":"user","timestamp":"2026-01-01T00:02:00Z","message":{"role":"user","content":"q2"}}`,
	}
	for _, l := range lines {
		fmt.Fprintln(f, l)
	}
	f.Close()

	inst := &Instance{
		Title:           "large-line-session",
		Path:            t.TempDir(),
		HistoryFilePath: historyFile,
	}
	inst.started.Store(true)

	cp, err := inst.CreateCheckpoint("snap", 0)
	require.NoError(t, err)

	// Must count all 3 lines, not stop at the 64 KB line.
	assert.Equal(t, uint64(3), cp.ConvLineCount,
		"ConvLineCount must be 3 even when a line exceeds 64 KB")
}
