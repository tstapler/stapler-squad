package session

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpencodeAdapter_NameAndCanHandle(t *testing.T) {
	adapter := NewOpencodeAdapter()
	assert.Equal(t, "opencode", adapter.Name())

	assert.True(t, adapter.CanHandle("opencode"))
	assert.True(t, adapter.CanHandle("OPENCODE"))
	assert.True(t, adapter.CanHandle("/usr/local/bin/opencode"))
	assert.False(t, adapter.CanHandle("claude"))
	assert.False(t, adapter.CanHandle("antigravity"))
}

func TestOpencodeAdapter_ExportAndImport_RoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	homeBackup := os.Getenv("HOME")
	t.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", homeBackup)

	adapter := NewOpencodeAdapter()
	inst, err := NewInstance(InstanceOptions{
		Title:      "test-opencode-session",
		WorkingDir: tmpDir,
		Program:    "opencode",
	})
	require.NoError(t, err)

	now := time.Now().Truncate(time.Millisecond)
	turns := []CanonicalTurn{
		{
			Role: RoleUser,
			Blocks: []CanonicalBlock{
				NewTextBlock("Hello OpenCode"),
			},
			Timestamp: now,
			TurnIndex: 0,
		},
		{
			Role: RoleAssistant,
			Blocks: []CanonicalBlock{
				NewTextBlock("I will read the file now."),
				NewToolUseBlock("call_123", "read", []byte(`{"path":"main.go"}`)),
			},
			Timestamp: now.Add(time.Second),
			TurnIndex: 1,
		},
		{
			Role: RoleUser,
			Blocks: []CanonicalBlock{
				NewToolResultBlock("call_123", "read", "package main\n\nfunc main() {}\n", false),
			},
			Timestamp: now.Add(2 * time.Second),
			TurnIndex: 2,
		},
	}

	ctx := context.Background()
	err = adapter.Export(ctx, turns, inst)
	require.NoError(t, err)

	convUUID := inst.GetClaudeConversationUUID()
	assert.NotEmpty(t, convUUID, "Export must generate and set a conversation UUID")

	dbPath := filepath.Join(tmpDir, ".local", "share", "opencode", "opencode.db")
	assert.FileExists(t, dbPath)

	importedTurns, err := adapter.Import(ctx, inst)
	require.NoError(t, err)
	require.Len(t, importedTurns, 3)

	assert.Equal(t, RoleUser, importedTurns[0].Role)
	assert.Equal(t, "Hello OpenCode", importedTurns[0].Blocks[0].Text)

	assert.Equal(t, RoleAssistant, importedTurns[1].Role)
	assert.Equal(t, "I will read the file now.", importedTurns[1].Blocks[0].Text)
	assert.Equal(t, BlockKindToolUse, importedTurns[1].Blocks[1].Kind)
	assert.Equal(t, "read", importedTurns[1].Blocks[1].ToolName)
	assert.Equal(t, "call_123", importedTurns[1].Blocks[1].ToolID)

	assert.Equal(t, RoleUser, importedTurns[2].Role)
	assert.Equal(t, BlockKindToolResult, importedTurns[2].Blocks[0].Kind)
	assert.Equal(t, "call_123", importedTurns[2].Blocks[0].ToolResultID)
	assert.Equal(t, "package main\n\nfunc main() {}\n", importedTurns[2].Blocks[0].ToolResultContent)
}

func TestOpencodeAdapter_Import_NoSession_ReturnsError(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	adapter := NewOpencodeAdapter()
	inst, err := NewInstance(InstanceOptions{
		Title:      "nonexistent-session",
		WorkingDir: tmpDir,
		Program:    "opencode",
	})
	require.NoError(t, err)

	_, err = adapter.Import(context.Background(), inst)
	assert.Error(t, err)
}
