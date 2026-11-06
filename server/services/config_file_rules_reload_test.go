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
	assert.False(t, r.reloadIfChanged(), "first poll of a missing file is treated as transient")
	assert.Equal(t, classifier.AutoAllow, classifyMake(rs.classifier))
	assert.True(t, r.reloadIfChanged())
	assert.Equal(t, baseline, classifyMake(rs.classifier), "file missing for two polls clears its rules")
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

// TestConfigFileRulesReloader_KeepsLastGood_OnEmptyFile proves a truncated, not-yet-rewritten
// save cannot clear deny rules, and that a valid rule beside an invalid one still applies.
func TestConfigFileRulesReloader_KeepsLastGood_OnEmptyFile(t *testing.T) {
	r, rs, path := newReloaderUnderTest(t)
	writeSharedRules(t, path, yamlDeny)
	require.True(t, r.reloadIfChanged())

	require.NoError(t, os.WriteFile(path, nil, 0o600))
	future := time.Now().Add(72 * time.Hour)
	require.NoError(t, os.Chtimes(path, future, future))

	assert.False(t, r.reloadIfChanged())
	assert.Equal(t, classifier.AutoDeny, classifyMake(rs.classifier))
}

func TestConfigFileRulesReloader_AppliesValidRules_WhenOneIsInvalid(t *testing.T) {
	r, rs, path := newReloaderUnderTest(t)
	require.NoError(t, os.WriteFile(path, []byte(`rules:
  - {name: broken, tool: Bash, command_pattern: '(', decision: allow}
  - {name: block make, tool: Bash, programs: [make], priority: 1000, decision: deny}
`), 0o600))

	assert.True(t, r.reloadIfChanged())
	assert.Equal(t, classifier.AutoDeny, classifyMake(rs.classifier))
}

func classifyEcho(c *classifier.RuleBasedClassifier) classifier.ClassificationDecision {
	payload := classifier.PermissionRequestPayload{ToolName: "Bash", ToolInput: map[string]interface{}{"command": "zzz-unknown-tool arg"}}
	return c.Classify(payload, classifier.ClassificationContext{}).Decision
}

// TestConfigFileRulesReloader_InvalidRegex_NeverWidens classifies a command only the broken
// rule could match: if the bad pattern degraded to match-all it would auto-allow.
func TestConfigFileRulesReloader_InvalidRegex_NeverWidens(t *testing.T) {
	r, rs, path := newReloaderUnderTest(t)
	baseline := classifyEcho(rs.classifier)
	require.NoError(t, os.WriteFile(path, []byte("rules:\n  - {name: broken, tool: Bash, command_pattern: '(', priority: 1000, decision: allow}\n"), 0o600))

	r.reloadIfChanged()
	assert.Equal(t, baseline, classifyEcho(rs.classifier))
}

// TestConfigFileRulesReloader_BadEdit_KeepsPreviousDeny proves an edit that breaks one rule
// cannot drop a deny that was working before it.
func TestConfigFileRulesReloader_BadEdit_KeepsPreviousDeny(t *testing.T) {
	r, rs, path := newReloaderUnderTest(t)
	writeSharedRules(t, path, yamlDeny)
	require.True(t, r.reloadIfChanged())

	require.NoError(t, os.WriteFile(path, []byte(`rules:
  - {name: block make, tool: Bash, programs: [make], priority: 1000, command_pattern: '(', decision: deny}
`), 0o600))
	future := time.Now().Add(96 * time.Hour)
	require.NoError(t, os.Chtimes(path, future, future))

	assert.False(t, r.reloadIfChanged())
	assert.Equal(t, classifier.AutoDeny, classifyMake(rs.classifier))
}

// TestConfigFileRulesReloader_TransientMissingFile_KeepsRules covers delete-then-write saves.
func TestConfigFileRulesReloader_TransientMissingFile_KeepsRules(t *testing.T) {
	r, rs, path := newReloaderUnderTest(t)
	writeSharedRules(t, path, yamlDeny)
	require.True(t, r.reloadIfChanged())

	require.NoError(t, os.Remove(path))
	assert.False(t, r.reloadIfChanged())
	writeSharedRules(t, path, yamlDeny)
	r.reloadIfChanged()
	assert.Equal(t, classifier.AutoDeny, classifyMake(rs.classifier))
}
