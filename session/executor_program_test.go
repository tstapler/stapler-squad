package session

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestValidateModel_should_Accept_When_EmptyOrWellFormed covers the happy
// paths: empty (means "use the program's default"), a plain model ID, and a
// "family:" alias.
func TestValidateModel_should_Accept_When_EmptyOrWellFormed(t *testing.T) {
	t.Parallel()
	for _, model := range []string{"", "claude-sonnet-4-6", "family:sonnet"} {
		assert.NoError(t, ValidateModel(model), "model %q", model)
	}
}

// TestValidateModel_should_Reject_When_ModelContainsShellMetacharactersOrWhitespace
// is the error path this function exists for: the resolved value is
// concatenated directly into a `claude --model <value>` program string, so
// whitespace/shell metacharacters must never pass.
func TestValidateModel_should_Reject_When_ModelContainsShellMetacharactersOrWhitespace(t *testing.T) {
	t.Parallel()
	for _, bad := range []string{"family: sonnet", "claude; rm -rf /", "claude sonnet", "$(whoami)"} {
		assert.Error(t, ValidateModel(bad), "expected %q to be rejected", bad)
	}
}
