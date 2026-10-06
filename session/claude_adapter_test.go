package session

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestReadCanonicalTurnsFromFile_TolersTrailingPartialLine_When_FileCaughtMidWrite
// exercises the pitfalls-research-mandated "tolerate trailing partial lines"
// requirement (Task 1.1.4b): a JSONL file with 3 well-formed lines followed
// by a 4th truncated line (simulating a read that raced a live writer mid
// write) must yield the 3 well-formed turns and must not error.
func TestReadCanonicalTurnsFromFile_TolersTrailingPartialLine_When_FileCaughtMidWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "conversation.jsonl")

	wellFormed := `{"type":"user","message":{"role":"user","content":"hello"},"uuid":"u1","timestamp":"2026-07-16T10:00:00Z","sessionId":"s1","cwd":"/tmp"}` + "\n" +
		`{"type":"assistant","message":{"role":"assistant","content":"hi there"},"uuid":"u2","timestamp":"2026-07-16T10:00:01Z","sessionId":"s1","cwd":"/tmp"}` + "\n" +
		`{"type":"user","message":{"role":"user","content":"how are you"},"uuid":"u3","timestamp":"2026-07-16T10:00:02Z","sessionId":"s1","cwd":"/tmp"}` + "\n"
	// Truncated 4th line: no trailing newline and cut off mid-JSON, exactly
	// as a reader would observe a file caught mid-write.
	truncated := `{"type":"user","message":{"role":"user","content":"this line got cut of`

	require.NoError(t, os.WriteFile(path, []byte(wellFormed+truncated), 0600))

	turns, err := ReadCanonicalTurnsFromFile(path)
	require.NoError(t, err)
	require.Len(t, turns, 3)
	assert.Equal(t, RoleUser, turns[0].Role)
	assert.Equal(t, RoleAssistant, turns[1].Role)
	assert.Equal(t, RoleUser, turns[2].Role)
}

func TestReadCanonicalTurnsFromFile_ReturnsError_When_FileDoesNotExist(t *testing.T) {
	t.Parallel()
	_, err := ReadCanonicalTurnsFromFile("/nonexistent/path/does-not-exist.jsonl")
	require.Error(t, err)
}
