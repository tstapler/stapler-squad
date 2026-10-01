package services

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/pkg/classifier"
)

const (
	yamlDeny  = "deny"
	yamlAllow = "allow"
)

func writeSharedRules(t *testing.T, path, decision string) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(`rules:
  - name: block make
    tool: Bash
    programs: [make]
    priority: 1000 # above every seed rule, so the test rule is not shadowed
    decision: `+decision+`
`), 0o600))
	// Guarantee a distinct mtime even on coarse-resolution filesystems.
	future := time.Now().Add(time.Duration(len(decision)) * time.Hour)
	require.NoError(t, os.Chtimes(path, future, future))
}

func classifyMake(c *classifier.RuleBasedClassifier) classifier.ClassificationDecision {
	payload := classifier.PermissionRequestPayload{ToolName: "Bash", ToolInput: map[string]interface{}{"command": "make build"}}
	return c.Classify(payload, classifier.ClassificationContext{}).Decision
}

func newReloaderUnderTest(t *testing.T) (*configFileRulesReloader, *RulesService, string) {
	t.Helper()
	rs := newRulesServiceWithAI(t, nil)
	path := filepath.Join(t.TempDir(), "shared_rules.yaml")
	return &configFileRulesReloader{path: path, apply: rs.rebuildConfigFileRules}, rs, path
}

// TestConfigFileRulesReloader_PicksUpEdits proves the server follows shared_rules.yaml:
// initial load, an edit that flips the decision, and deletion.
func TestConfigFileRulesReloader_PicksUpEdits(t *testing.T) {
	r, rs, path := newReloaderUnderTest(t)

	baseline := classifyMake(rs.classifier)
	assert.True(t, r.reloadIfChanged(), "first call primes even when the file is missing")
	assert.Equal(t, baseline, classifyMake(rs.classifier))

	writeSharedRules(t, path, yamlDeny)
	assert.True(t, r.reloadIfChanged())
	assert.Equal(t, classifier.AutoDeny, classifyMake(rs.classifier))

	assert.False(t, r.reloadIfChanged(), "unchanged file is not reloaded")

	writeSharedRules(t, path, yamlAllow)
	assert.True(t, r.reloadIfChanged())
	assert.Equal(t, classifier.AutoAllow, classifyMake(rs.classifier))

	require.NoError(t, os.Remove(path))
	assert.True(t, r.reloadIfChanged())
	assert.Equal(t, baseline, classifyMake(rs.classifier), "deleted file clears its rules")
}

// TestConfigFileRulesReloader_KeepsLastGood_OnParseError proves a half-saved or broken edit
// cannot silently drop a deny rule.
func TestConfigFileRulesReloader_KeepsLastGood_OnParseError(t *testing.T) {
	r, rs, path := newReloaderUnderTest(t)
	writeSharedRules(t, path, yamlDeny)
	require.True(t, r.reloadIfChanged())

	require.NoError(t, os.WriteFile(path, []byte("rules: [unclosed"), 0o600))
	future := time.Now().Add(48 * time.Hour)
	require.NoError(t, os.Chtimes(path, future, future))

	assert.False(t, r.reloadIfChanged())
	assert.Equal(t, classifier.AutoDeny, classifyMake(rs.classifier))
}

// TestRebuilds_PreserveConfigFileRules guards the allow-lists in the other two rebuild paths:
// editing a user rule or reloading claude-settings must not wipe shared_rules.yaml rules.
func TestRebuilds_PreserveConfigFileRules(t *testing.T) {
	r, rs, path := newReloaderUnderTest(t)
	writeSharedRules(t, path, yamlDeny)
	require.True(t, r.reloadIfChanged())

	rs.rebuildClassifier()
	assert.Equal(t, classifier.AutoDeny, classifyMake(rs.classifier), "survives a user-rule rebuild")

	rs.rebuildClaudeSettingsRules(nil)
	assert.Equal(t, classifier.AutoDeny, classifyMake(rs.classifier), "survives a claude-settings rebuild")
}
