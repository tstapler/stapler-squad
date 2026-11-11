package session

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// Not t.Parallel(): t.Setenv mutates process-global HOME.
func postExportInstance(t *testing.T) (*Instance, *int) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	calls := 0
	inst := &Instance{
		Title: "ported",
		Path:  "/home/test/proj",
		claudeExtension: claudeExtension{
			claudeSession: &ClaudeSessionData{ConversationUUID: "uuid-1"},
		},
	}
	inst.claudeSessionIDSavedCallback = func() { calls++ }
	return inst, &calls
}

func readJSONLines(t *testing.T, path string) []map[string]interface{} {
	t.Helper()
	f, err := os.Open(path)
	require.NoError(t, err)
	defer f.Close()
	var out []map[string]interface{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var m map[string]interface{}
		require.NoError(t, json.Unmarshal(sc.Bytes(), &m))
		out = append(out, m)
	}
	return out
}

func TestAgyAdapter_AfterExport_AppendsHistoryEntry(t *testing.T) {
	inst, _ := postExportInstance(t)
	require.NoError(t, NewAgyAdapter().AfterExport(context.Background(), inst))
	home, _ := os.UserHomeDir()
	lines := readJSONLines(t, filepath.Join(home, ".gemini", "antigravity-cli", "history.jsonl"))
	require.Len(t, lines, 1)
	require.Equal(t, "Ported from Claude: ported", lines[0]["display"])
	require.Equal(t, "uuid-1", lines[0]["conversationId"])
	require.Equal(t, inst.GetWorkingDirectory(), lines[0]["workspace"])
}

func TestClaudeAdapter_AfterExport_AppendsHistoryAndRecordsSession(t *testing.T) {
	inst, calls := postExportInstance(t)
	require.NoError(t, NewClaudeAdapter().AfterExport(context.Background(), inst))
	home, _ := os.UserHomeDir()
	lines := readJSONLines(t, filepath.Join(home, ".claude", "history.jsonl"))
	require.Len(t, lines, 1)
	require.Equal(t, "Ported from Antigravity: ported", lines[0]["display"])
	require.Equal(t, "uuid-1", lines[0]["sessionId"])
	require.Equal(t, inst.GetWorkingDirectory(), lines[0]["project"])
	require.Equal(t, 1, *calls)
	require.Equal(t, "ported", inst.claudeSession.ProjectName)
	require.Equal(t, inst.GetWorkingDirectory(), inst.claudeSession.Metadata["working_dir"])
}

func TestOpencodeAdapter_AfterExport_RecordsSessionOnly(t *testing.T) {
	inst, calls := postExportInstance(t)
	require.NoError(t, NewOpencodeAdapter().AfterExport(context.Background(), inst))
	require.Equal(t, 1, *calls)
	require.Equal(t, "ported", inst.claudeSession.ProjectName)
	home, _ := os.UserHomeDir()
	_, err := os.Stat(filepath.Join(home, ".claude", "history.jsonl"))
	require.True(t, os.IsNotExist(err))
}

func TestAppendHistoryJSONL_UnmarshalableEntryReturnsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "d", "history.jsonl")
	err := appendHistoryJSONL(path, "x", map[string]interface{}{"bad": make(chan int)})
	require.Error(t, err)
}
