package mcp

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestProgramSchemaOptions_should_IncludeAllBuiltinsAndCustom_When_SvcHasCustomProgram
// pins programSchemaOptions' happy path: the shared helper both
// registerLifecycleTools and registerGitHubTools depend on for their program
// property (see tools_program_common.go) must surface every built-in plus
// any custom program registered via UpsertProgramConfig -- the exact
// discoverability gap the hardcoded Enum("claude", "aider") this replaces
// left unfilled (research/features.md).
func TestProgramSchemaOptions_should_IncludeAllBuiltinsAndCustom_When_SvcHasCustomProgram(t *testing.T) {
	lh := newWorktreeGuardHandlers(t)
	upsertTestCustomProgram(t, lh.svc)

	opts := programSchemaOptions(lh.svc)
	schema := map[string]any{}
	for _, opt := range opts {
		opt(schema)
	}

	enum, ok := schema["enum"].([]string)
	require.True(t, ok, "expected schema[\"enum\"] to be a []string, got %#v", schema["enum"])
	assert.Contains(t, enum, "bash")
	assert.Contains(t, enum, "opencode")
	assert.Contains(t, enum, testCustomProgramID)
}

// TestProgramSchemaOptions_should_OmitEnum_When_SvcIsNil covers the
// documented githubHandlers.svc-may-be-nil case (research/stack.md):
// programSchemaOptions must degrade to a description-only property, never
// panic, and never set an empty/misleading enum.
func TestProgramSchemaOptions_should_OmitEnum_When_SvcIsNil(t *testing.T) {
	opts := programSchemaOptions(nil)
	schema := map[string]any{}
	for _, opt := range opts {
		opt(schema)
	}

	_, ok := schema["enum"]
	assert.False(t, ok, "schema[\"enum\"] must be absent when svc is nil, got %#v", schema["enum"])
	desc, _ := schema["description"].(string)
	assert.NotEmpty(t, desc, "schema[\"description\"] must still be set when svc is nil")
}
