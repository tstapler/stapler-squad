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
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/instance"
)

// forkResumeID returns the resume conversation UUID stored in the forked instance (if any).
func forkResumeID(inst *instance.Instance) string {
	cs := inst.GetClaudeSession()
	if cs == nil {
		return ""
	}
	return cs.ConversationUUID
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
	t.Parallel()
	configDir := t.TempDir()

	// Create source instance.
	srcInst := &instance.Instance{
		Title: "src-session",
		Path:  t.TempDir(),
	}
	srcInst.MarkStarted()
	// Point HistoryFilePath at a real file with content.
	historyDir := t.TempDir()
	historyFile := filepath.Join(historyDir, "conv-aaa.jsonl")
	writeConvLines(t, historyFile, 10)
	srcInst.HistoryFilePath = historyFile
	srcInst.SetClaudeSession(&domain.ClaudeSessionData{ConversationUUID: "conv-aaa"})

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

func TestForkFromCheckpoint_NoConvUUID_EmptyResumeId(t *testing.T) {
	t.Parallel()
	configDir := t.TempDir()
	srcInst := &instance.Instance{
		Title: "src-session",
		Path:  t.TempDir(),
	}
	srcInst.MarkStarted()
	// claudeSession is nil — no UUID available.

	cp, err := srcInst.CreateCheckpoint("snap", 0)
	require.NoError(t, err)

	fork, err := srcInst.ForkFromCheckpoint(cp.ID, "fork-session", configDir)

	require.NoError(t, err)
	assert.Empty(t, forkResumeID(fork), "no conv UUID → empty resume ID")
}

// TestForkFromCheckpoint_ForkedFileHasCorrectContent verifies that the JSONL file
// produced by ForkFromCheckpoint contains exactly the lines captured at checkpoint
// time and that every line is valid JSON with a non-nil message field.
// This is the critical end-to-end test that was previously missing.
func TestForkFromCheckpoint_ForkedFileHasCorrectContent(t *testing.T) {
	t.Parallel()
	configDir := t.TempDir()
	historyDir := t.TempDir()

	// Write 10 realistic Claude turns (user/assistant pairs).
	historyFile := filepath.Join(historyDir, "conv-real.jsonl")
	writeParsableClaudeTurns(t, historyFile, 10)

	srcInst := (&instance.Instance{
		Title:           "src-session",
		Path:            t.TempDir(),
		HistoryFilePath: historyFile,
	}).WithClaudeSessionForTest(&domain.ClaudeSessionData{ConversationUUID: "conv-real"})
	srcInst.MarkStarted()

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
