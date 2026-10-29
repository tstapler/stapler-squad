package session

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeConversationFixture(t *testing.T, root, project, name, content string) string {
	t.Helper()
	dir := filepath.Join(root, project)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

func TestFindConversationFilePath_FoundAmongMultipleProjects(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	// WalkDir is lexical, so decoys sort before the real match to be visited first.
	writeConversationFixture(t, root, "proj-a", "a.jsonl", "{\"sessionId\":\"other-session\"}\n")
	writeConversationFixture(t, root, "proj-b", "agent-b.jsonl", "{\"sessionId\":\"target-session\"}\n") // agent files are skipped
	writeConversationFixture(t, root, "proj-c", "c.txt", "{\"sessionId\":\"target-session\"}\n")         // non-jsonl is skipped
	want := writeConversationFixture(t, root, "proj-d", "d.jsonl", "{\"x\":1}\n{\"sessionId\":\"target-session\"}\n")

	got, err := findConversationFilePath(context.Background(), root, "target-session")
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestFindConversationFilePath_NotFound(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeConversationFixture(t, root, "proj-a", "a.jsonl", "{\"sessionId\":\"other-session\"}\n")

	_, err := findConversationFilePath(context.Background(), root, "target-session")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestFindConversationFilePath_MissingRootIsNotFound(t *testing.T) {
	t.Parallel()
	_, err := findConversationFilePath(context.Background(), filepath.Join(t.TempDir(), "absent"), "target-session")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestFindConversationFilePath_CancelledContextAborts(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeConversationFixture(t, root, "proj-a", "a.jsonl", "{\"sessionId\":\"target-session\"}\n")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := findConversationFilePath(ctx, root, "target-session")
	require.ErrorIs(t, err, context.Canceled)
}
