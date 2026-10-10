package services

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session"
)

func TestRulesToAntigravityPermissions_FormatsCorrectly(t *testing.T) {
	rules := []session.ApprovalRuleData{
		{
			ID:          "rule-git",
			Name:        "Git commands",
			Enabled:     true,
			Decision:    0, // AutoAllow
			Programs:    []string{"git"},
			Subcommands: []string{"status", "log", "diff"},
		},
		{
			ID:       "rule-rtk",
			Name:     "RTK",
			Enabled:  true,
			Decision: 0,
			Programs: []string{"rtk"},
		},
		{
			ID:          "rule-disabled",
			Name:        "Disabled rule",
			Enabled:     false,
			Decision:    0,
			Programs:    []string{"rm"},
			Subcommands: []string{"-rf"},
		},
		{
			ID:       "rule-deny",
			Name:     "Deny rule",
			Enabled:  true,
			Decision: 1, // AutoDeny
			Programs: []string{"sudo"},
		},
	}

	perms := RulesToAntigravityPermissions(rules)
	assert.Contains(t, perms, "command(git status)")
	assert.Contains(t, perms, "command(git log)")
	assert.Contains(t, perms, "command(git diff)")
	assert.Contains(t, perms, "command(rtk)")
	assert.NotContains(t, perms, "command(rm -rf)")
	assert.NotContains(t, perms, "command(sudo)")
}

func TestExportRulesToAntigravitySettings_MergesWithoutDuplicates(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")

	initial := map[string]interface{}{
		"allowNonWorkspaceAccess": true,
		"permissions": map[string]interface{}{
			"allow": []interface{}{
				"command(pwd)",
				"command(ls)",
			},
		},
	}
	raw, err := json.MarshalIndent(initial, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(settingsPath, raw, 0600))

	rules := []session.ApprovalRuleData{
		{
			ID:          "rule-git",
			Name:        "Git commands",
			Enabled:     true,
			Decision:    0,
			Programs:    []string{"git"},
			Subcommands: []string{"status"},
		},
		{
			ID:       "rule-pwd",
			Name:     "PWD command",
			Enabled:  true,
			Decision: 0,
			Programs: []string{"pwd"},
		},
	}

	require.NoError(t, ExportRulesToAntigravitySettings(settingsPath, rules))

	data, err := os.ReadFile(settingsPath)
	require.NoError(t, err)

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &result))

	assert.Equal(t, true, result["allowNonWorkspaceAccess"])

	perms, ok := result["permissions"].(map[string]interface{})
	require.True(t, ok)

	allowSlice, ok := perms["allow"].([]interface{})
	require.True(t, ok)

	allows := make([]string, 0, len(allowSlice))
	for _, item := range allowSlice {
		allows = append(allows, item.(string))
	}

	assert.Contains(t, allows, "command(pwd)")
	assert.Contains(t, allows, "command(ls)")
	assert.Contains(t, allows, "command(git status)")

	// Count occurrences of command(pwd) to ensure no duplicate
	pwdCount := 0
	for _, a := range allows {
		if a == "command(pwd)" {
			pwdCount++
		}
	}
	assert.Equal(t, 1, pwdCount, "should deduplicate existing permission entries")
}

func TestExportAntigravityRulesFromDB_ExportsToHomeGemini(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	repo, err := session.NewEntRepository(session.WithDatabasePath(":memory:"))
	require.NoError(t, err)
	storage, err := session.NewStorageWithRepository(repo)
	require.NoError(t, err)
	defer storage.Close()

	// Seed a rule
	require.NoError(t, storage.UpsertRule(context.Background(), session.ApprovalRuleData{
		ID:          "rule-make",
		Name:        "Allow make",
		Enabled:     true,
		Decision:    0,
		Programs:    []string{"make"},
		Subcommands: []string{"build"},
	}))

	require.NoError(t, ExportAntigravityRulesFromDB(context.Background(), storage))

	agyPath := filepath.Join(homeDir, ".gemini", "antigravity-cli", "settings.json")
	geminiPath := filepath.Join(homeDir, ".gemini", "settings.json")

	assert.FileExists(t, agyPath)
	assert.FileExists(t, geminiPath)

	data, err := os.ReadFile(agyPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "command(make build)")
}
