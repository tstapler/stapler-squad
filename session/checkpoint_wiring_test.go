package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/instance"
)

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
	projectDir := filepath.Join(fakeHome, ".claude", "projects", instance.ClaudeProjectDirName(workspace))
	require.NoError(t, os.MkdirAll(projectDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, convUUID+".jsonl"), []byte{}, 0o644))

	const legitTitle = "checkpoint-legit-title"
	inst := &instance.Instance{Title: legitTitle, Program: "claude", Path: workspace}
	inst.MarkStarted()
	inst.SetClaudeSession(&domain.ClaudeSessionData{ConversationUUID: convUUID})

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
