package services

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

// claudeConfigTestDir creates a temporary directory that impersonates ~/.claude for
// the duration of the test by overriding HOME.  It returns a cleanup function that
// restores the original HOME value.
func claudeConfigTestDir(t *testing.T) string {
	t.Helper()

	tmpHome := t.TempDir()

	// Point HOME at the temp directory so GetClaudeDir() resolves to tmpHome/.claude
	origHome, hadHome := os.LookupEnv("HOME")
	t.Setenv("HOME", tmpHome)
	t.Cleanup(func() {
		if hadHome {
			os.Setenv("HOME", origHome) //nolint:errcheck
		} else {
			os.Unsetenv("HOME") //nolint:errcheck
		}
	})

	claudeDir := filepath.Join(tmpHome, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatalf("failed to create temp .claude dir: %v", err)
	}
	return claudeDir
}

// TestGetClaudeConfig_ReturnsConfig verifies that GetClaudeConfig returns a config
// file that was written to the test directory.
func TestGetClaudeConfig_ReturnsConfig(t *testing.T) {
	claudeDir := claudeConfigTestDir(t)

	// Write a known file into the fake ~/.claude directory.
	content := "# Hello from CLAUDE.md\n"
	filePath := filepath.Join(claudeDir, "CLAUDE.md")
	require.NoError(t, os.WriteFile(filePath, []byte(content), 0644))

	svc := NewConfigService()
	req := connect.NewRequest(&sessionv1.GetClaudeConfigRequest{
		Filename: "CLAUDE.md",
	})
	resp, err := svc.GetClaudeConfig(context.Background(), req)

	require.NoError(t, err)
	require.NotNil(t, resp.Msg)
	require.NotNil(t, resp.Msg.Config)
	assert.Equal(t, "CLAUDE.md", resp.Msg.Config.Name)
	assert.Equal(t, content, resp.Msg.Config.Content)
}

// TestGetClaudeConfig_NotFound verifies that requesting a file that does not exist
// returns CodeNotFound.
func TestGetClaudeConfig_NotFound(t *testing.T) {
	claudeConfigTestDir(t)

	svc := NewConfigService()
	req := connect.NewRequest(&sessionv1.GetClaudeConfigRequest{
		Filename: "does-not-exist.md",
	})
	_, err := svc.GetClaudeConfig(context.Background(), req)

	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeNotFound, connectErr.Code())
}

// TestListClaudeConfigs_ReturnsList verifies that ListClaudeConfigs returns a list
// that includes files written to the test ~/.claude directory.
func TestListClaudeConfigs_ReturnsList(t *testing.T) {
	claudeDir := claudeConfigTestDir(t)

	// Write a couple of files.
	for _, name := range []string{"CLAUDE.md", "agents.md"} {
		require.NoError(t, os.WriteFile(filepath.Join(claudeDir, name), []byte("test"), 0644))
	}
	// Also write a hidden file; it should be excluded by the implementation.
	require.NoError(t, os.WriteFile(filepath.Join(claudeDir, ".hidden"), []byte("ignored"), 0644))

	svc := NewConfigService()
	req := connect.NewRequest(&sessionv1.ListClaudeConfigsRequest{})
	resp, err := svc.ListClaudeConfigs(context.Background(), req)

	require.NoError(t, err)
	require.NotNil(t, resp.Msg)
	// We expect at least the two files we created (hidden file excluded).
	assert.GreaterOrEqual(t, len(resp.Msg.Configs), 2)
	names := make(map[string]bool, len(resp.Msg.Configs))
	for _, c := range resp.Msg.Configs {
		names[c.Name] = true
	}
	assert.True(t, names["CLAUDE.md"], "expected CLAUDE.md in list")
	assert.True(t, names["agents.md"], "expected agents.md in list")
	assert.False(t, names[".hidden"], ".hidden should be excluded")
}

// TestListClaudeConfigs_EmptyDir verifies that ListClaudeConfigs returns an empty
// (non-nil) list when the ~/.claude directory exists but is empty.
func TestListClaudeConfigs_EmptyDir(t *testing.T) {
	claudeConfigTestDir(t) // creates the empty directory

	svc := NewConfigService()
	req := connect.NewRequest(&sessionv1.ListClaudeConfigsRequest{})
	resp, err := svc.ListClaudeConfigs(context.Background(), req)

	require.NoError(t, err)
	require.NotNil(t, resp.Msg)
	assert.NotNil(t, resp.Msg.Configs)
}

// TestUpdateClaudeConfig_NoOp verifies that updating a markdown file with new
// content succeeds and the updated content is returned.
func TestUpdateClaudeConfig_NoOp(t *testing.T) {
	claudeDir := claudeConfigTestDir(t)

	// Pre-create the file.
	original := "# original\n"
	require.NoError(t, os.WriteFile(filepath.Join(claudeDir, "CLAUDE.md"), []byte(original), 0644))

	svc := NewConfigService()
	req := connect.NewRequest(&sessionv1.UpdateClaudeConfigRequest{
		Filename: "CLAUDE.md",
		Content:  "# updated\n",
		Validate: false,
	})
	resp, err := svc.UpdateClaudeConfig(context.Background(), req)

	require.NoError(t, err)
	require.NotNil(t, resp.Msg)
	require.NotNil(t, resp.Msg.Config)
	assert.Equal(t, "# updated\n", resp.Msg.Config.Content)
}
