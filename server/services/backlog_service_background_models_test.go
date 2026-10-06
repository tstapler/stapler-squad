package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tstapler/stapler-squad/session"
)

func TestPinnedStageModel_should_PinOnlyClaudeWithoutModeModel(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	assert.Equal(t, "family:sonnet", pinnedStageModel(session.StageRoleReview, "", ""))
	assert.Equal(t, "family:sonnet", pinnedStageModel(session.StageRoleReview, "claude", ""))
	assert.Equal(t, "family:opus", pinnedStageModel(session.StageRoleReview, "", "family:opus"), "mode pin wins")
	for _, prog := range []string{"agy", "gemini", "proxy-claude"} {
		assert.Empty(t, pinnedStageModel(session.StageRoleReview, prog, ""), prog)
	}
}

func TestPinnedWorkStageModel_should_SkipWhenExecutorProgramNotClaude(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	assert.Empty(t, pinnedWorkStageModel(t.TempDir(), "agy", ""))
}
