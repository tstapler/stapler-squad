package historyadapters

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tstapler/stapler-squad/session/instance"
)

// TestSwitchProgram_WithinClaudeAntigravityFamily_PreservesUUID verifies that a switch
// that stays within the claude/antigravity family (recognized as history-portable) does
// NOT take the "leaving the family" clear-state branch. This only asserts the decision
// (no clear), not PortSessionHistory's own Import/Export behavior, which performs real
// file I/O against the adapters' backing stores and is out of scope here.
func TestSwitchProgram_WithinClaudeAntigravityFamily_PreservesUUID(t *testing.T) {
	t.Parallel()
	assert.True(t, instance.IsClaudeAntigravityCrossSwitch(DefaultHistoryAdapters(), "claude", "antigravity"))
	assert.True(t, instance.IsClaudeAntigravityCrossSwitch(DefaultHistoryAdapters(), "antigravity", "claude"))
	assert.True(t, instance.IsClaudeAntigravityCrossSwitch(DefaultHistoryAdapters(), "claude", "agy"))
	assert.True(t, instance.IsClaudeAntigravityCrossSwitch(DefaultHistoryAdapters(), "claude", "opencode"))
	assert.True(t, instance.IsClaudeAntigravityCrossSwitch(DefaultHistoryAdapters(), "opencode", "agy"))
	assert.False(t, instance.IsClaudeAntigravityCrossSwitch(DefaultHistoryAdapters(), "claude", "aider"))
	assert.False(t, instance.IsClaudeAntigravityCrossSwitch(DefaultHistoryAdapters(), "opencode", "opencode"))

	assert.True(t, instance.IsClaudeAntigravityFamily(DefaultHistoryAdapters(), "claude"))
	assert.True(t, instance.IsClaudeAntigravityFamily(DefaultHistoryAdapters(), "antigravity"))
	assert.True(t, instance.IsClaudeAntigravityFamily(DefaultHistoryAdapters(), "agy"))
	assert.True(t, instance.IsClaudeAntigravityFamily(DefaultHistoryAdapters(), "opencode"))
	assert.False(t, instance.IsClaudeAntigravityFamily(DefaultHistoryAdapters(), "aider"))
}

// TestGeminiFamilyGate_ConsistentWithAdapterResolution verifies that gemini (the standalone Gemini CLI)
// is handled by GeminiAdapter (and thus included in the family gate and cross-switch resolution),
// while remaining excluded from AgyAdapter.CanHandle (session/historyadapters/agy_adapter.go).
func TestGeminiFamilyGate_ConsistentWithAdapterResolution(t *testing.T) {
	t.Parallel()
	assert.True(t, instance.IsClaudeAntigravityFamily(DefaultHistoryAdapters(), "gemini"))
	assert.True(t, instance.IsClaudeAntigravityCrossSwitch(DefaultHistoryAdapters(), "claude", "gemini"))
	assert.True(t, instance.IsClaudeAntigravityCrossSwitch(DefaultHistoryAdapters(), "gemini", "claude"))
	assert.False(t, NewAgyAdapter().CanHandle("gemini"))
	assert.True(t, NewGeminiAdapter().CanHandle("gemini"))
}
