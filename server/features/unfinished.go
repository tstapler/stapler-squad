package features

import "github.com/tstapler/stapler-squad/server/featureregistry"

var UnfinishedCommitPush = featureregistry.Feature{
	ID:          "unfinished-commit-push",
	Title:       "Quick Commit Push",
	Description: "Commits and pushes pending changes in a worktree with a single RPC call.",
	RPCIDs:      []string{"unfinished:commit-push"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

var UnfinishedDismiss = featureregistry.Feature{
	ID:          "unfinished-dismiss",
	Title:       "Dismiss Worktree",
	Description: "Dismisses a worktree from the unfinished work list so it no longer appears.",
	RPCIDs:      []string{"unfinished:dismiss"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

var UnfinishedGetAISummary = featureregistry.Feature{
	ID:          "unfinished-get-ai-summary",
	Title:       "Get Worktree AI Summary",
	Description: "Generates an AI-produced summary of uncommitted changes in a worktree.",
	RPCIDs:      []string{"unfinished:get-ai-summary"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

var UnfinishedGetConfig = featureregistry.Feature{
	ID:          "unfinished-get-config",
	Title:       "Get Unfinished Work Config",
	Description: "Retrieves the current configuration for the unfinished work scanner.",
	RPCIDs:      []string{"unfinished:get-config"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

var UnfinishedGetWorktreeDiff = featureregistry.Feature{
	ID:          "unfinished-get-worktree-diff",
	Title:       "Get Worktree Diff",
	Description: "Returns the full git diff for a worktree with uncommitted changes.",
	RPCIDs:      []string{"unfinished:get-worktree-diff"},
	Status:      featureregistry.StatusExperimental,
	Since:       "1.0.0",
}

var UnfinishedScan = featureregistry.Feature{
	ID:          "unfinished-scan",
	Title:       "Scan Unfinished Work",
	Description: "Triggers an immediate scan of all worktrees to refresh unfinished work state.",
	RPCIDs:      []string{"unfinished:scan"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

var UnfinishedSnooze = featureregistry.Feature{
	ID:          "unfinished-snooze",
	Title:       "Snooze Worktree",
	Description: "Temporarily snoozes a worktree so it is hidden from unfinished work for a set duration.",
	RPCIDs:      []string{"unfinished:snooze"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

var UnfinishedUndismiss = featureregistry.Feature{
	ID:          "unfinished-undismiss",
	Title:       "Undismiss Worktree",
	Description: "Restores a previously dismissed worktree back into the unfinished work list.",
	RPCIDs:      []string{"unfinished:undismiss"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

var UnfinishedUpdateConfig = featureregistry.Feature{
	ID:          "unfinished-update-config",
	Title:       "Update Unfinished Work Config",
	Description: "Updates the configuration for the unfinished work scanner, such as scan intervals and exclusions.",
	RPCIDs:      []string{"unfinished:update-config"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

func init() {
	featureregistry.Register(UnfinishedCommitPush)
	featureregistry.Register(UnfinishedDismiss)
	featureregistry.Register(UnfinishedGetAISummary)
	featureregistry.Register(UnfinishedGetConfig)
	featureregistry.Register(UnfinishedGetWorktreeDiff)
	featureregistry.Register(UnfinishedScan)
	featureregistry.Register(UnfinishedSnooze)
	featureregistry.Register(UnfinishedUndismiss)
	featureregistry.Register(UnfinishedUpdateConfig)
}
