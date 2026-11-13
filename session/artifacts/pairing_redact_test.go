package artifacts

import (
	"strings"
	"testing"

	"github.com/tstapler/stapler-squad/session/domain"

	"github.com/stretchr/testify/assert"
)

func TestCommandRecord_ShouldRedactSecretsInResult(t *testing.T) {
	t.Parallel()
	secret := "ghp_" + strings.Repeat("a", 36)
	rec := commandRecord(pendingCall{cmd: &domain.CommandArtifact{Type: "x", Command: "x"}}, "token="+secret)
	assert.NotContains(t, rec.Result, secret)
}
