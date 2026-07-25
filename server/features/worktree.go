package features

import "github.com/tstapler/stapler-squad/server/featureregistry"

// WorktreeList describes the list-worktrees RPC.
var WorktreeList = featureregistry.Feature{
	ID:          "worktree-list",
	Title:       "List Worktrees",
	Description: "Lists all git worktrees available for a given repository path.",
	RPCIDs:      []string{"worktree:list"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

func init() {
	featureregistry.Register(WorktreeList)
}
