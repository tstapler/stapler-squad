package features

import "github.com/tstapler/stapler-squad/server/featureregistry"

// SessionCreate describes the create-session RPC and its UI surface.
var SessionCreate = featureregistry.Feature{
	ID:          "session-create",
	Title:       "Create Session",
	Description: "Creates a new AI agent session in a directory, worktree, or one-off mode.",
	RPCIDs:      []string{"session:create"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

// SessionList describes the list/watch session RPCs and their UI surface.
var SessionList = featureregistry.Feature{
	ID:          "session-list",
	Title:       "Session List",
	Description: "Lists and streams updates for all sessions.",
	RPCIDs:      []string{"session:list", "session:watch"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

// SessionDelete describes the delete-session RPC and its UI surface.
var SessionDelete = featureregistry.Feature{
	ID:          "session-delete",
	Title:       "Delete Session",
	Description: "Deletes a session and its associated worktree.",
	RPCIDs:      []string{"session:delete"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

func init() {
	featureregistry.Register(SessionCreate)
	featureregistry.Register(SessionList)
	featureregistry.Register(SessionDelete)
}
