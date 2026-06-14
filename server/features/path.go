package features

import "github.com/tstapler/stapler-squad/server/featureregistry"

// PathListCompletions describes the list-path-completions RPC.
var PathListCompletions = featureregistry.Feature{
	ID:          "path-list-completions",
	Title:       "List Path Completions",
	Description: "Returns filesystem path completions for a given prefix, supporting tilde expansion, hidden files, symlinks, and truncation.",
	RPCIDs:      []string{"path:list-completions"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

func init() {
	featureregistry.Register(PathListCompletions)
}
