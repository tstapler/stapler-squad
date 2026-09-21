package services

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

// TestListSlashCommands_RejectsTraversingTargetDirectory verifies that
// ListSlashCommands' validateTargetDirectory gate actually prevents walking a
// target_directory containing ".." components: a marker command file placed
// at the path the traversal resolves to must NOT show up in the response,
// proving the containment check runs before filepath.Join/WalkDir ever touch
// that directory (a broken check would still find it, since
// filepath.Join cleans the same way).
func TestListSlashCommands_RejectsTraversingTargetDirectory(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // isolate ~/.claude/commands (user-source) from the real home dir

	base := t.TempDir()
	// escapeDir is what filepath.Clean(maliciousTargetDir) resolves to below.
	// The point isn't where it lands -- it's that validateTargetDirectory must
	// reject any target_directory containing literal ".." components, purely
	// from the string, before that resolved directory is ever walked.
	escapeDir := filepath.Join(base, "ssq-slash-cmd-escape-poc")
	commandsDir := filepath.Join(escapeDir, ".claude", "commands")
	require.NoError(t, os.MkdirAll(commandsDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(commandsDir, "only-if-vulnerable.md"), []byte("# should never be listed"), 0o644))

	maliciousTargetDir := filepath.Join(base, "sub") + string(filepath.Separator) + ".." + string(filepath.Separator) + "ssq-slash-cmd-escape-poc"

	svc := NewSlashCommandService()
	resp, err := svc.ListSlashCommands(context.Background(), connect.NewRequest(&sessionv1.ListSlashCommandsRequest{
		TargetDirectory: maliciousTargetDir,
	}))
	require.NoError(t, err, "a rejected target_directory is logged and skipped, not returned as an RPC error")
	require.NotNil(t, resp)

	for _, cmd := range resp.Msg.GetCommands() {
		assert.NotEqual(t, "only-if-vulnerable", cmd.GetName(),
			"a command from a target_directory that escapes via '..' must never be listed")
	}
}

// TestListSlashCommands_LegitimateTargetDirectory_ListsProjectCommands is the
// control case: a clean absolute target_directory must still surface its
// project commands.
func TestListSlashCommands_LegitimateTargetDirectory_ListsProjectCommands(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	targetDir := t.TempDir()
	commandsDir := filepath.Join(targetDir, ".claude", "commands")
	require.NoError(t, os.MkdirAll(commandsDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(commandsDir, "legit-command.md"), []byte("# a real project command"), 0o644))

	svc := NewSlashCommandService()
	resp, err := svc.ListSlashCommands(context.Background(), connect.NewRequest(&sessionv1.ListSlashCommandsRequest{
		TargetDirectory: targetDir,
	}))
	require.NoError(t, err)
	require.NotNil(t, resp)

	found := false
	for _, cmd := range resp.Msg.GetCommands() {
		if cmd.GetName() == "legit-command" {
			found = true
			assert.Equal(t, "project", cmd.GetSource())
		}
	}
	assert.True(t, found, "a legitimate target_directory's project command should be listed")
}
