package services

import (
	"strings"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/session"
)

// pinnedStageModel returns the config-pinned model for a stage when the pipeline
// mode pinned none, so default-mode items stop inheriting the account default.
// Only claude-backed executors are pinned: a model alias means nothing to agy/gemini.
// Read via LoadConfig per call so edits apply live.
func pinnedStageModel(role session.StageRole, execProgram, execModel string) string {
	if execModel != "" || (execProgram != "" && execProgram != "claude") {
		return execModel
	}
	return config.LoadConfig().BackgroundStageModel(string(role))
}

// pinnedWorkStageModel is pinnedStageModel for the work stage, which spawns an
// interactive session: it also requires the resolved default program to be bare
// claude, because a pin replaces the whole program string and would drop the flags
// (or the program, e.g. proxy-claude) the user configured.
func pinnedWorkStageModel(repoPath, execProgram, execModel string) string {
	if execModel != "" || (execProgram != "" && execProgram != "claude") {
		return execModel
	}
	cfg := config.LoadConfig()
	if p := strings.TrimSpace(config.ResolveDefaults(cfg, repoPath, "").Program); p != "" && p != "claude" {
		return execModel
	}
	return cfg.BackgroundStageModel(string(session.StageRoleWork))
}
