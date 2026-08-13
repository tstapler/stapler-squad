package features

import "github.com/tstapler/stapler-squad/server/featureregistry"

// UnfinishedWork describes the unfinished-work RPCs and their UI surface.
var UnfinishedWork = featureregistry.Feature{
	ID:          "unfinished-work",
	Title:       "Unfinished Work",
	Description: "Surfaces pending changes across git worktrees.",
	RPCIDs:      []string{"unfinished:list", "unfinished:watch"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

func init() {
	featureregistry.Register(UnfinishedWork)
}
