package classifier

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfigFileRules_MissingFile_IsNotAnError(t *testing.T) {
	rules, err := LoadConfigFileRules(filepath.Join(t.TempDir(), "absent.yaml"))
	require.NoError(t, err)
	assert.Empty(t, rules)
}

func TestLoadConfigFileRules_ParsesDecisionPriorityAndSource(t *testing.T) {
	path := filepath.Join(t.TempDir(), "shared_rules.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`rules:
  - name: no force push
    tool: Bash
    command_pattern: 'git push.*--force'
    decision: deny
  - tool: Bash
  - name: disabled one
    decision: allow
    enabled: false
    priority: 50
`), 0o600))

	rules, err := LoadConfigFileRules(path)
	require.NoError(t, err)
	require.Len(t, rules, 2)

	assert.Equal(t, AutoDeny, rules[0].Decision)
	assert.Equal(t, 10, rules[0].Priority, "zero priority defaults to 10")
	assert.Equal(t, string(SourceConfig), rules[0].Source)
	assert.Equal(t, "config-no-force-push", rules[0].ID)
	assert.NotNil(t, rules[0].CommandPattern)

	assert.Equal(t, AutoAllow, rules[1].Decision)
	assert.False(t, rules[1].Enabled)
	assert.Equal(t, 50, rules[1].Priority)
}

func TestLoadConfigFileRules_MalformedYAML_ReturnsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "shared_rules.yaml")
	require.NoError(t, os.WriteFile(path, []byte("rules: [unclosed"), 0o600))
	_, err := LoadConfigFileRules(path)
	require.Error(t, err)
}

func writeRules(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "shared_rules.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

// An invalid pattern must drop the rule, never widen it into "match every Bash command".
func TestLoadConfigFileRules_InvalidRegex_SkipsRuleKeepsOthers(t *testing.T) {
	path := writeRules(t, `rules:
  - {name: bad allow, tool: Bash, command_pattern: '^git (status', decision: allow}
  - {name: good deny, tool: Bash, programs: [mkfs], decision: deny}
`)
	rules, err := LoadConfigFileRules(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bad allow")
	require.Len(t, rules, 1)
	assert.Equal(t, "good deny", rules[0].Name)
}

func TestLoadConfigFileRules_UnknownDecision_SkipsRule(t *testing.T) {
	path := writeRules(t, "rules:\n  - {name: typo, tool: Bash, decision: Deny}\n")
	rules, err := LoadConfigFileRules(path)
	require.Error(t, err)
	assert.Empty(t, rules)
}

func TestLoadConfigFileRules_EmptyFile_IsAnError(t *testing.T) {
	for _, content := range []string{"", "  \n\t\n"} {
		rules, err := LoadConfigFileRules(writeRules(t, content))
		require.Error(t, err)
		assert.Nil(t, rules)
	}
}
