package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// forkResumeID returns the resume conversation UUID stored in the forked instance (if any).
func forkResumeID(inst *Instance) string {
	if inst.claudeSession == nil {
		return ""
	}
	return inst.claudeSession.ConversationUUID
}

// writeConvLines writes count JSON lines to path (simulates Claude history file).
func writeConvLines(t *testing.T, path string, count int) {
	t.Helper()
	f, err := os.Create(path)
	require.NoError(t, err)
	defer f.Close()
	for i := 1; i <= count; i++ {
		line, _ := json.Marshal(map[string]int{"seq": i})
		_, err = fmt.Fprintf(f, "%s\n", line)
		require.NoError(t, err)
	}
}

func TestForkFromCheckpoint_ConvUUID_SetAsResumeId(t *testing.T) {
	configDir := t.TempDir()

	// Create source instance.
	srcInst := &Instance{
		Title:   "src-session",
		Path:    t.TempDir(),
		started: true,
	}
	// Point HistoryFilePath at a real file with content.
	historyDir := t.TempDir()
	historyFile := filepath.Join(historyDir, "conv-aaa.jsonl")
	writeConvLines(t, historyFile, 10)
	srcInst.HistoryFilePath = historyFile
	srcInst.claudeSession = &ClaudeSessionData{ConversationUUID: "conv-aaa"}

	// Create checkpoint with conv data.
	cp, err := srcInst.CreateCheckpoint("snap", 5)
	require.NoError(t, err)
	// Manually set so we have a known UUID for assertions.
	cp.ClaudeConvUUID = "conv-aaa"
	cp.ConvLineCount = 5
	srcInst.Checkpoints[0] = *cp

	fork, err := srcInst.ForkFromCheckpoint(cp.ID, "fork-session", configDir)

	require.NoError(t, err)
	require.NotNil(t, fork)

	// ResumeId should be a new UUID (non-empty, not the original).
	assert.NotEmpty(t, forkResumeID(fork), "fork should have a resume ID")
	assert.NotEqual(t, "conv-aaa", forkResumeID(fork))
}

func TestForkFromCheckpoint_NoGitSHA_Succeeds(t *testing.T) {
	configDir := t.TempDir()
	srcInst := &Instance{
		Title:   "src-session",
		Path:    t.TempDir(),
		started: true,
	}

	cp, err := srcInst.CreateCheckpoint("snap", 0)
	require.NoError(t, err)
	// Leave GitCommitSHA empty to test graceful skip.
	assert.Empty(t, cp.GitCommitSHA)

	fork, err := srcInst.ForkFromCheckpoint(cp.ID, "fork-session", configDir)

	require.NoError(t, err)
	require.NotNil(t, fork)
	assert.False(t, fork.gitManager.HasWorktree(), "no git worktree should be created when SHA is empty")
}

func TestForkFromCheckpoint_NoConvUUID_EmptyResumeId(t *testing.T) {
	configDir := t.TempDir()
	srcInst := &Instance{
		Title:   "src-session",
		Path:    t.TempDir(),
		started: true,
	}
	// claudeSession is nil — no UUID available.

	cp, err := srcInst.CreateCheckpoint("snap", 0)
	require.NoError(t, err)

	fork, err := srcInst.ForkFromCheckpoint(cp.ID, "fork-session", configDir)

	require.NoError(t, err)
	assert.Empty(t, forkResumeID(fork), "no conv UUID → empty resume ID")
}

func TestForkFromCheckpoint_ForkedFromIDSet(t *testing.T) {
	configDir := t.TempDir()
	srcInst := &Instance{
		Title:   "src-session",
		Path:    t.TempDir(),
		started: true,
	}

	cp, err := srcInst.CreateCheckpoint("snap", 0)
	require.NoError(t, err)

	fork, err := srcInst.ForkFromCheckpoint(cp.ID, "fork-session", configDir)

	require.NoError(t, err)
	assert.Equal(t, "src-session", fork.ForkedFromID)
}

func TestForkFromCheckpoint_UnknownCheckpointID_ReturnsError(t *testing.T) {
	configDir := t.TempDir()
	srcInst := &Instance{Title: "src-session", Path: t.TempDir(), started: true}

	_, err := srcInst.ForkFromCheckpoint("nonexistent-id", "fork-session", configDir)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestForkFromCheckpoint_EmptyNewTitle_ReturnsError(t *testing.T) {
	configDir := t.TempDir()
	srcInst := &Instance{Title: "src-session", Path: t.TempDir(), started: true}
	cp, err := srcInst.CreateCheckpoint("snap", 0)
	require.NoError(t, err)

	_, err = srcInst.ForkFromCheckpoint(cp.ID, "", configDir)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty")
}

// TestForkFromCheckpoint_ForkedFileHasCorrectContent verifies that the JSONL file
// produced by ForkFromCheckpoint contains exactly the lines captured at checkpoint
// time and that every line is valid JSON with a non-nil message field.
// This is the critical end-to-end test that was previously missing.
func TestForkFromCheckpoint_ForkedFileHasCorrectContent(t *testing.T) {
	configDir := t.TempDir()
	historyDir := t.TempDir()

	// Write 10 realistic Claude turns (user/assistant pairs).
	historyFile := filepath.Join(historyDir, "conv-real.jsonl")
	writeParsableClaudeTurns(t, historyFile, 10)

	srcInst := &Instance{
		Title:           "src-session",
		Path:            t.TempDir(),
		started:         true,
		HistoryFilePath: historyFile,
		claudeSession:   &ClaudeSessionData{ConversationUUID: "conv-real"},
	}

	// Checkpoint after 6 turns.
	cp, err := srcInst.CreateCheckpoint("mid-point", 0)
	require.NoError(t, err)
	// Manually pin ConvLineCount since CreateCheckpoint counts real file lines.
	assert.Equal(t, uint64(10), cp.ConvLineCount, "all 10 lines should be counted")

	// Now create a checkpoint that only covers the first 6 lines.
	cp.ConvLineCount = 6
	srcInst.Checkpoints[0] = *cp

	fork, err := srcInst.ForkFromCheckpoint(cp.ID, "fork-session", configDir)
	require.NoError(t, err)
	require.NotNil(t, fork)

	// The fork's Claude session should have a new, different UUID.
	forkedUUID := forkResumeID(fork)
	require.NotEmpty(t, forkedUUID, "fork must have a conversation UUID")
	assert.NotEqual(t, "conv-real", forkedUUID)

	// Open the forked JSONL file and verify content.
	forkedFilePath := filepath.Join(historyDir, forkedUUID+".jsonl")
	data, err := os.ReadFile(forkedFilePath)
	require.NoError(t, err, "forked JSONL file must exist at %s", forkedFilePath)

	lines := splitNonEmpty(string(data))
	assert.Equal(t, 6, len(lines), "forked file should have exactly 6 lines (checkpoint ConvLineCount)")

	// Every line must be valid JSON with a non-nil message field (real user/assistant turn).
	for i, line := range lines {
		var raw rawClaudeTurn
		require.NoError(t, json.Unmarshal([]byte(line), &raw), "line %d must be valid JSON", i)
		assert.NotNil(t, raw.Message, "line %d should not be a skipped turn (it's a real user/assistant turn)", i)
	}
}

// TestForkFromCheckpoint_ConvLineCount_AccurateForLargeLines ensures that
// CreateCheckpoint correctly counts lines even when line content exceeds 64 KB
// (the old bufio.Scanner limit). This regression test guards the fix in
// instance_checkpoint.go that replaced Scanner with bufio.NewReader.
func TestForkFromCheckpoint_ConvLineCount_AccurateForLargeLines(t *testing.T) {
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
		started:         true,
		HistoryFilePath: historyFile,
	}

	cp, err := inst.CreateCheckpoint("snap", 0)
	require.NoError(t, err)

	// Must count all 3 lines, not stop at the 64 KB line.
	assert.Equal(t, uint64(3), cp.ConvLineCount,
		"ConvLineCount must be 3 even when a line exceeds 64 KB")
}

// writeParsableClaudeTurns writes count alternating user/assistant Claude JSONL turns.
func writeParsableClaudeTurns(t *testing.T, path string, count int) {
	t.Helper()
	f, err := os.Create(path)
	require.NoError(t, err)
	defer f.Close()
	enc := json.NewEncoder(f)
	for i := 0; i < count; i++ {
		role := "user"
		msgRole := "user"
		content := fmt.Sprintf("user message %d", i)
		if i%2 == 1 {
			role = "assistant"
			msgRole = "assistant"
			content = fmt.Sprintf("assistant reply %d", i)
		}
		turn := map[string]interface{}{
			"type":      role,
			"timestamp": fmt.Sprintf("2026-01-01T00:%02d:00Z", i),
			"message": map[string]interface{}{
				"role":    msgRole,
				"content": content,
			},
		}
		require.NoError(t, enc.Encode(turn))
	}
}

// splitNonEmpty splits s by newline and returns only non-empty lines.
func splitNonEmpty(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if strings.TrimSpace(l) != "" {
			out = append(out, l)
		}
	}
	return out
}
