package stagecfg

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateModel_should_Accept_When_EmptyOrWellFormed(t *testing.T) {
	t.Parallel()
	for _, model := range []string{"", "claude-sonnet-4-6", "family:sonnet"} {
		assert.NoError(t, ValidateModel(model), "model %q", model)
	}
}

// The resolved value is concatenated into a `claude --model <value>` program
// string, so whitespace and shell metacharacters must never pass.
func TestValidateModel_should_Reject_When_ModelContainsShellMetacharactersOrWhitespace(t *testing.T) {
	t.Parallel()
	for _, bad := range []string{"family: sonnet", "claude; rm -rf /", "claude sonnet", "$(whoami)"} {
		assert.Error(t, ValidateModel(bad), "expected %q to be rejected", bad)
	}
}
