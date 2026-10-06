package services

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session"
)

const autoAllow = 0

func TestRulesToAntigravityPermissions_should_SkipRules_When_ConstraintCannotBeExpressed(t *testing.T) {
	cases := map[string]session.ApprovalRuleData{
		"python_mode_restriction": {Programs: []string{"python3"}, PythonModes: []string{"inline"}, SafePythonImportsOnly: true},
		"forbidden_flags":         {Programs: []string{"git"}, Subcommands: []string{"reset"}, ForbiddenFlags: []string{"--hard"}},
		"required_flags":          {Programs: []string{"git"}, Subcommands: []string{"reset"}, RequiredFlags: []string{"--hard"}},
		"blocked_subcommands":     {Programs: []string{"git"}, BlockedSubcommands: []string{"push"}},
		"ci_passing":              {Programs: []string{"git"}, Subcommands: []string{"push"}, RequireCIPassing: true},
		"session_idle":            {Programs: []string{"make"}, Subcommands: []string{"build"}, MinSessionIdleMinutes: 5},
		"file_pattern":            {ToolName: "Read", FilePattern: "^/home/.*"},
		"tool_category":           {ToolCategory: "mcp-read"},
		"programs_plus_pattern":   {Programs: []string{"git"}, CommandPattern: "^git status"},
		"unscoped_bash":           {ToolName: "Bash"},
		"no_scope_at_all":         {},
		"regex_command_pattern":   {ToolName: "Bash", CommandPattern: "^git .*"},
		"anchored_end_pattern":    {ToolName: "Bash", CommandPattern: "^git status$"},
		"unanchored_pattern":      {ToolName: "Bash", CommandPattern: "git status"},
		"match_all_pattern":       {ToolName: "Bash", CommandPattern: ".*"},
	}
	for name, rule := range cases {
		t.Run(name, func(t *testing.T) {
			rule.ID, rule.Enabled, rule.Decision = name, true, autoAllow
			assert.Empty(t, RulesToAntigravityPermissions([]session.ApprovalRuleData{rule}))
		})
	}
}

func TestRulesToAntigravityPermissions_should_ExportExpressibleRules(t *testing.T) {
	cases := map[string]struct {
		rule session.ApprovalRuleData
		want string
	}{
		"programs_and_subcommands": {session.ApprovalRuleData{Programs: []string{"git"}, Subcommands: []string{"status"}}, "command(git status)"},
		"bare_program":             {session.ApprovalRuleData{Programs: []string{"pwd"}}, "command(pwd)"},
		"anchored_literal_prefix":  {session.ApprovalRuleData{ToolName: "Bash", CommandPattern: "^git status"}, "command(git status)"},
		"non_bash_tool":            {session.ApprovalRuleData{ToolName: "Read"}, "command(Read)"},
		"anchored_tool_pattern":    {session.ApprovalRuleData{ToolPattern: "^mcp__brave"}, "command(mcp__brave)"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			rule := tc.rule
			rule.ID, rule.Enabled, rule.Decision = name, true, autoAllow
			assert.Equal(t, []string{tc.want}, RulesToAntigravityPermissions([]session.ApprovalRuleData{rule}))
		})
	}
}

func TestExportRulesToAntigravitySettings_should_RemoveStaleEntry_When_RuleDeleted(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")
	writeSettingsFixture(t, settingsPath, []string{"command(ls)"})

	git := autoAllowRule("git-status", session.ApprovalRuleData{Programs: []string{"git"}, Subcommands: []string{"status"}})
	require.NoError(t, ExportRulesToAntigravitySettings(settingsPath, []session.ApprovalRuleData{git}))
	assert.Contains(t, readAllow(t, settingsPath), "command(git status)")

	// Rule deleted: the entry this exporter added goes; the hand-added one stays.
	require.NoError(t, ExportRulesToAntigravitySettings(settingsPath, nil))
	allow := readAllow(t, settingsPath)
	assert.NotContains(t, allow, "command(git status)")
	assert.Contains(t, allow, "command(ls)")
}

func TestExportRulesToAntigravitySettings_should_NotClaimHandAddedEntry(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")
	writeSettingsFixture(t, settingsPath, []string{"command(pwd)"})

	pwd := autoAllowRule("pwd", session.ApprovalRuleData{Programs: []string{"pwd"}})
	require.NoError(t, ExportRulesToAntigravitySettings(settingsPath, []session.ApprovalRuleData{pwd}))
	require.NoError(t, ExportRulesToAntigravitySettings(settingsPath, nil))

	assert.Contains(t, readAllow(t, settingsPath), "command(pwd)", "a hand-added entry must survive rule deletion")
}

func TestExportRulesToAntigravitySettings_should_WriteOwnerOnlyFile(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")
	pwd := autoAllowRule("pwd", session.ApprovalRuleData{Programs: []string{"pwd"}})
	require.NoError(t, ExportRulesToAntigravitySettings(settingsPath, []session.ApprovalRuleData{pwd}))

	info, err := os.Stat(settingsPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())
}

// A deleted rule must not come back when exports overlap: each export reads the rules
// under the lock, so the last export to write reflects the last database change.
func TestExportAntigravityRulesFromDB_should_NotResurrectDeletedRule_When_ExportsOverlap(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	repo, err := session.NewEntRepository(session.WithDatabasePath(filepath.Join(t.TempDir(), "rules.db")))
	require.NoError(t, err)
	storage, err := session.NewStorageWithRepository(repo)
	require.NoError(t, err)
	defer storage.Close()

	ctx := context.Background()
	const n = 20
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		rule := autoAllowRule(fmt.Sprintf("r%d", i), session.ApprovalRuleData{Programs: []string{fmt.Sprintf("tool%d", i)}})
		require.NoError(t, storage.UpsertRule(ctx, rule))
		wg.Add(1)
		go func() {
			defer wg.Done()
			assert.NoError(t, ExportAntigravityRulesFromDB(ctx, storage))
		}()
	}
	for i := 0; i < n; i += 2 {
		require.NoError(t, storage.DeleteRule(ctx, fmt.Sprintf("r%d", i)))
		wg.Add(1)
		go func() {
			defer wg.Done()
			assert.NoError(t, ExportAntigravityRulesFromDB(ctx, storage))
		}()
	}
	wg.Wait()

	allow := readAllow(t, filepath.Join(home, ".gemini", "antigravity-cli", "settings.json"))
	for i := 0; i < n; i++ {
		entry := fmt.Sprintf("command(tool%d)", i)
		if i%2 == 0 {
			assert.NotContains(t, allow, entry, "deleted rule resurrected by an overlapping export")
		} else {
			assert.Contains(t, allow, entry)
		}
	}
}

func autoAllowRule(id string, r session.ApprovalRuleData) session.ApprovalRuleData {
	r.ID, r.Name, r.Enabled, r.Decision = id, id, true, autoAllow
	return r
}

func writeSettingsFixture(t *testing.T, path string, allow []string) {
	t.Helper()
	raw, err := json.Marshal(map[string]interface{}{
		"permissions": map[string]interface{}{"allow": allow},
	})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, raw, 0600))
}

func readAllow(t *testing.T, path string) []string {
	t.Helper()
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	var root struct {
		Permissions struct {
			Allow []string `json:"allow"`
		} `json:"permissions"`
	}
	require.NoError(t, json.Unmarshal(raw, &root))
	return root.Permissions.Allow
}
