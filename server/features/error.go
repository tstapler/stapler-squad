package features

import "github.com/tstapler/stapler-squad/server/featureregistry"

// ErrorAcknowledge describes the acknowledge-error RPC.
var ErrorAcknowledge = featureregistry.Feature{
	ID:          "error-acknowledge",
	Title:       "Acknowledge Error",
	Description: "Acknowledges an error, marking it as seen and dismissing it from the active error list.",
	RPCIDs:      []string{"error:acknowledge"},
	Status:      featureregistry.StatusExperimental,
	Since:       "1.0.0",
}

// ErrorList describes the list-errors RPC.
var ErrorList = featureregistry.Feature{
	ID:          "error-list",
	Title:       "List Errors",
	Description: "Lists all active errors reported by sessions or the server.",
	RPCIDs:      []string{"error:list"},
	Status:      featureregistry.StatusExperimental,
	Since:       "1.0.0",
}

func init() {
	featureregistry.Register(ErrorAcknowledge)
	featureregistry.Register(ErrorList)
}
