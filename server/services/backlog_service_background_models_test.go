package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/config"
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

func TestPinnedWorkStageModel_should_PinOnlyWhenDefaultProgramIsBareClaude(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	repo := t.TempDir()

	cfg := config.LoadConfig()
	cfg.DefaultProgram = "claude"
	cfg.SessionDefaults.Program = ""
	require.NoError(t, config.SaveConfig(cfg))
	assert.Equal(t, "family:sonnet", pinnedWorkStageModel(repo, "", ""))
	assert.Equal(t, "family:opus", pinnedWorkStageModel(repo, "", "family:opus"), "mode pin wins")

	for _, prog := range []string{"proxy-claude", "claude --dangerously-skip-permissions", "agy"} {
		cfg.DefaultProgram = prog
		require.NoError(t, config.SaveConfig(cfg))
		assert.Empty(t, pinnedWorkStageModel(repo, "", ""), "default program %q must not be pinned", prog)
	}
}
