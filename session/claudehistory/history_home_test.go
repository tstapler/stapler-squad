package claudehistory

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests resolve ~/.claude through $HOME, so they mutate process-global
// env and cannot run in parallel.
func TestHomeResolution_FindsConversationUnderClaudeProjects(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	projects := filepath.Join(home, ".claude", "projects")
	want := writeConversationFixture(t, projects, "proj-a", "a.jsonl",
		"{\"type\":\"user\",\"sessionId\":\"home-session\",\"message\":{\"role\":\"user\",\"content\":\"first\"}}\n"+
			"{\"type\":\"assistant\",\"sessionId\":\"home-session\",\"message\":{\"role\":\"assistant\",\"content\":\"second\"}}\n")

	got, err := FindConversationFilePath(t.Context(), "home-session")
	require.NoError(t, err)
	assert.Equal(t, want, got)

	require.NoError(t, os.WriteFile(filepath.Join(home, ".claude", "history.jsonl"),
		[]byte("{\"display\":\"hi\",\"timestamp\":1000,\"project\":\"/p\",\"sessionId\":\"home-session\"}\n"), 0o600))

	sh, err := NewClaudeSessionHistoryFromClaudeDir()
	require.NoError(t, err)
	assert.Equal(t, 1, sh.Count())

	all, err := sh.GetMessagesFromConversationFile("home-session", 0)
	require.NoError(t, err)
	assert.Len(t, all, 2)

	last, err := sh.GetMessagesFromConversationFile("home-session", 1)
	require.NoError(t, err)
	require.Len(t, last, 1)
	assert.Equal(t, "second", last[0].Content)
}

func TestHomeResolution_FindConversationFilePath_NoHomeIsAnError(t *testing.T) {
	t.Setenv("HOME", "")
	if _, err := os.UserHomeDir(); err == nil {
		t.Skip("platform resolves a home directory without $HOME")
	}
	_, err := FindConversationFilePath(t.Context(), "x")
	require.Error(t, err)
	_, err = NewClaudeSessionHistoryFromClaudeDir()
	require.Error(t, err)
}
