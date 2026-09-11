package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFileClaudeTrustStore_CreatesEntryPreservingOtherKeys(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	configPath := filepath.Join(home, ".claude.json")

	seed := map[string]any{
		"numStartups": 42,
		"projects": map[string]any{
			"/some/other/project": map[string]any{
				"hasTrustDialogAccepted": true,
				"lastCost":               1.23,
			},
		},
	}
	seedBytes, err := json.Marshal(seed)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(configPath, seedBytes, 0o600))

	targetDir := filepath.Join(t.TempDir(), "workdir")
	require.NoError(t, os.MkdirAll(targetDir, 0o755))

	require.NoError(t, fileClaudeTrustStore{}.MarkTrusted(targetDir))

	raw, err := os.ReadFile(configPath)
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(raw, &got))

	require.InDelta(t, 42, got["numStartups"], 0)

	projects, ok := got["projects"].(map[string]any)
	require.True(t, ok)

	other, ok := projects["/some/other/project"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, true, other["hasTrustDialogAccepted"])
	require.InDelta(t, 1.23, other["lastCost"], 0.0001)

	entry, ok := projects[targetDir].(map[string]any)
	require.True(t, ok, "expected an entry for %s", targetDir)
	require.Equal(t, true, entry["hasTrustDialogAccepted"])
}

func TestFileClaudeTrustStore_NoExistingConfigFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	targetDir := t.TempDir()
	require.NoError(t, fileClaudeTrustStore{}.MarkTrusted(targetDir))

	raw, err := os.ReadFile(filepath.Join(home, ".claude.json"))
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(raw, &got))

	projects, ok := got["projects"].(map[string]any)
	require.True(t, ok)
	entry, ok := projects[targetDir].(map[string]any)
	require.True(t, ok)
	require.Equal(t, true, entry["hasTrustDialogAccepted"])
}

// TestTrustStore_DefaultsToMemoryUnderTestIsolation verifies Instance's lazy
// trustStore() default: under config.IsIsolatedInstance() (true for every
// `go test` binary), it must resolve to a *memoryClaudeTrustStore, never
// fileClaudeTrustStore -- the guarantee that plain tests constructing a real
// claude-program Instance (bare &Instance{}, no explicit trustStore()
// wiring) can never write to the developer's real ~/.claude.json.
func TestTrustStore_DefaultsToMemoryUnderTestIsolation(t *testing.T) {
	inst := &Instance{Title: "test-session", Program: "claude"}
	_, ok := inst.trustStore().(*memoryClaudeTrustStore)
	require.True(t, ok, "expected trustStore() to default to *memoryClaudeTrustStore under test isolation")
}

// TestMarkWorkingDirTrusted_RecordsIntoInjectedStore exercises
// markWorkingDirTrusted end-to-end against an explicitly injected
// *memoryClaudeTrustStore, so the test asserts on the recorded trust
// decision itself rather than just the absence of a disk write.
func TestMarkWorkingDirTrusted_RecordsIntoInjectedStore(t *testing.T) {
	store := newMemoryClaudeTrustStore()
	workDir := t.TempDir()
	inst := &Instance{Title: "test-session", Program: "claude", Path: workDir, claudeTrustStoreImpl: store}

	inst.markWorkingDirTrusted()

	require.True(t, store.IsTrusted(workDir))
}

// TestMarkWorkingDirTrusted_NonClaudeAgentIsNoop verifies the agent-type
// gate: an aider/pi session must never touch the trust store at all (not
// even the in-memory default), since only claude reads ~/.claude.json.
func TestMarkWorkingDirTrusted_NonClaudeAgentIsNoop(t *testing.T) {
	store := newMemoryClaudeTrustStore()
	workDir := t.TempDir()
	inst := &Instance{Title: "test-session", Program: "aider", Path: workDir, claudeTrustStoreImpl: store}

	inst.markWorkingDirTrusted()

	require.False(t, store.IsTrusted(workDir))
}
