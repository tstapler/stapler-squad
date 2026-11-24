package session

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestResolveModel_should_ReturnConcreteID_When_KnownFamilyAlias covers the
// happy path this function exists for: "family:<alias>" resolves via the
// families map.
func TestResolveModel_should_ReturnConcreteID_When_KnownFamilyAlias(t *testing.T) {
	t.Parallel()
	families := map[string]string{"sonnet": "claude-sonnet-4-6"}
	resolved, err := ResolveModel(families, "family:sonnet")
	assert.NoError(t, err)
	assert.Equal(t, "claude-sonnet-4-6", resolved)
}

// TestResolveModel_should_PassThroughUnchanged_When_ValueHasNoFamilyPrefix
// covers both the empty-string case (no model configured) and an
// already-concrete model ID (pre-dating family aliases) — neither should be
// touched by the families lookup.
func TestResolveModel_should_PassThroughUnchanged_When_ValueHasNoFamilyPrefix(t *testing.T) {
	t.Parallel()
	families := map[string]string{"sonnet": "claude-sonnet-4-6"}
	for _, model := range []string{"", "claude-opus-4-8", "sonnet"} {
		resolved, err := ResolveModel(families, model)
		assert.NoError(t, err)
		assert.Equal(t, model, resolved, "model %q should pass through unchanged", model)
	}
}

// TestResolveModel_should_ReturnError_When_FamilyAliasUnknown is the error
// path: an unknown/retired alias must fail closed rather than passing the
// broken "family:xxx" string through to the CLI.
func TestResolveModel_should_ReturnError_When_FamilyAliasUnknown(t *testing.T) {
	t.Parallel()
	_, err := ResolveModel(map[string]string{"sonnet": "claude-sonnet-4-6"}, "family:retired-name")
	assert.Error(t, err)
}

// TestResolveExecutorProgram_should_AppendResolvedModel_When_BaseProgramEmptyOrClaude
// covers the two "isClaudeProgram" cases from FireNow's original inline
// logic: an empty base program (defaults to claude) and an explicit "claude".
func TestResolveExecutorProgram_should_AppendResolvedModel_When_BaseProgramEmptyOrClaude(t *testing.T) {
	t.Parallel()
	families := map[string]string{"sonnet": "claude-sonnet-4-6"}

	program, err := ResolveExecutorProgram("", "family:sonnet", families)
	assert.NoError(t, err)
	assert.Equal(t, "claude --model claude-sonnet-4-6", program)

	program, err = ResolveExecutorProgram("claude", "claude-opus-4-8", families)
	assert.NoError(t, err)
	assert.Equal(t, "claude --model claude-opus-4-8", program)
}

// TestResolveExecutorProgram_should_LeaveProgramUnchanged_When_BaseProgramNotClaude
// documents the v1 limitation: a resolved model is silently not appended for
// a non-Claude base program (e.g. "aider"), matching FireNow's original
// isClaudeProgram guard.
func TestResolveExecutorProgram_should_LeaveProgramUnchanged_When_BaseProgramNotClaude(t *testing.T) {
	t.Parallel()
	families := map[string]string{"sonnet": "claude-sonnet-4-6"}
	program, err := ResolveExecutorProgram("aider", "family:sonnet", families)
	assert.NoError(t, err)
	assert.Equal(t, "aider", program)
}

// TestResolveExecutorProgram_should_ReturnError_When_FamilyAliasUnknown
// verifies model-resolution errors propagate rather than being swallowed
// into an unresolved "family:xxx" program string.
func TestResolveExecutorProgram_should_ReturnError_When_FamilyAliasUnknown(t *testing.T) {
	t.Parallel()
	_, err := ResolveExecutorProgram("", "family:retired-name", map[string]string{"sonnet": "claude-sonnet-4-6"})
	assert.Error(t, err)
}
