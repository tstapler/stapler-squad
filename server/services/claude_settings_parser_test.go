package services

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeSettingsFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))
}

func TestLoadClaudeSettingsRulesDetailed_AllPathsValid_ReturnsPerPathRules(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeSettingsFile(t, filepath.Join(home, ".claude", "settings.json"),
		`{"permissions":{"allow":["Bash(git log*)"]}}`)

	results := LoadClaudeSettingsRulesDetailed("")

	var global *ClaudeSettingsPathResult
	for i := range results {
		if results[i].Label == "global" {
			global = &results[i]
		}
	}
	require.NotNil(t, global)
	assert.NoError(t, global.Err)
	require.Len(t, global.Rules, 1)
	assert.Equal(t, "Bash", global.Rules[0].ToolName)
}

func TestLoadClaudeSettingsRulesDetailed_AllPathsMissingOrUnreadable_ReturnsEmptyResultsNoError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	results := LoadClaudeSettingsRulesDetailed("")

	require.NotEmpty(t, results)
	for _, r := range results {
		assert.Empty(t, r.Rules)
		// Missing files are reported via ErrSettingsNotFound, not treated as a crash/abort.
		if r.Err != nil {
			assert.ErrorIs(t, r.Err, ErrSettingsNotFound)
		}
	}
}

func TestLoadClaudeSettingsRulesDetailed_OneMalformedPath_ReturnsErrForThatPathOnly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	projectDir := t.TempDir()

	writeSettingsFile(t, filepath.Join(home, ".claude", "settings.json"),
		`{"permissions":{"allow":["Bash(git *)"]}}`)
	writeSettingsFile(t, filepath.Join(projectDir, ".claude", "settings.json"),
		`{"permissions": {"allow": [`) // truncated/invalid JSON

	results := LoadClaudeSettingsRulesDetailed(projectDir)

	var global, project *ClaudeSettingsPathResult
	for i := range results {
		switch results[i].Label {
		case "global":
			global = &results[i]
		case "project":
			project = &results[i]
		}
	}
	require.NotNil(t, global)
	require.NotNil(t, project)

	assert.NoError(t, global.Err)
	require.Len(t, global.Rules, 1)

	require.Error(t, project.Err)
	assert.Empty(t, project.Rules)
}

func TestLoadClaudeSettingsRules_FlattensDetailedResults_IgnoringErrors(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	projectDir := t.TempDir()

	writeSettingsFile(t, filepath.Join(home, ".claude", "settings.json"),
		`{"permissions":{"allow":["Bash(git *)"]}}`)
	writeSettingsFile(t, filepath.Join(projectDir, ".claude", "settings.json"),
		`not json at all`)

	rules := LoadClaudeSettingsRules(projectDir)

	require.Len(t, rules, 1)
	assert.Equal(t, "Bash", rules[0].ToolName)
}

func TestResolveSettingsPathOrOriginal_Symlink_ReturnsRealPath(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real", "settings.json")
	writeSettingsFile(t, real, `{}`)

	link := filepath.Join(dir, "settings.json")
	require.NoError(t, os.Symlink(real, link))

	resolved := resolveSettingsPathOrOriginal(link)

	realResolved, err := filepath.EvalSymlinks(real)
	require.NoError(t, err)
	assert.Equal(t, realResolved, resolved)
}

func TestResolveSettingsPathOrOriginal_NoSymlink_ReturnsUnchanged(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	writeSettingsFile(t, path, `{}`)

	resolved := resolveSettingsPathOrOriginal(path)

	realResolved, err := filepath.EvalSymlinks(path)
	require.NoError(t, err)
	assert.Equal(t, realResolved, resolved)
}

func TestResolveSettingsPathOrOriginal_NonexistentPath_FallsBackToOriginal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "does-not-exist.json")

	resolved := resolveSettingsPathOrOriginal(path)

	assert.Equal(t, path, resolved)
}

// TestLoadClaudeSettingsRulesDetailed_ProjectDirEqualsHomeDir_DedupesToSinglePathEntry
// regression-guards adversarial-review Blocker 2 / pre-mortem P1 #2: the live deployed
// systemd unit runs with WorkingDirectory=$HOME, so projectDir == home and the "global"
// and "project" settingsPaths entries resolve to the identical file. Without dedup, that
// file's rules would be loaded (and counted) twice.
func TestLoadClaudeSettingsRulesDetailed_ProjectDirEqualsHomeDir_DedupesToSinglePathEntry(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeSettingsFile(t, filepath.Join(home, ".claude", "settings.json"),
		`{"permissions":{"allow":["Bash(git *)"]}}`)

	results := LoadClaudeSettingsRulesDetailed(home)

	resolvedHomeSettings, err := filepath.EvalSymlinks(filepath.Join(home, ".claude", "settings.json"))
	require.NoError(t, err)

	count := 0
	for _, r := range results {
		if r.Path == resolvedHomeSettings {
			count++
			assert.Equal(t, "global", r.Label, "the deduped entry should keep the global label/priority, not project")
		}
	}
	assert.Equal(t, 1, count, "settings.json should appear exactly once when projectDir == home")
}
