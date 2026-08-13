package features

import "github.com/tstapler/stapler-squad/server/featureregistry"

// InteractionLog describes the log-user-interaction RPC.
var InteractionLog = featureregistry.Feature{
	ID:          "interaction-log",
	Title:       "Log User Interaction",
	Description: "Records a user interaction event associated with a session for analytics and auditing.",
	RPCIDs:      []string{"interaction:log"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

func init() {
	featureregistry.Register(InteractionLog)
}
