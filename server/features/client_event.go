package features

import "github.com/tstapler/stapler-squad/server/featureregistry"

// ClientEventLog describes the log-client-events RPC.
var ClientEventLog = featureregistry.Feature{
	ID:          "client-event-log",
	Title:       "Log Client Events",
	Description: "Receives and records client-side events from the frontend for server-side processing.",
	RPCIDs:      []string{"client-event:log"},
	Status:      featureregistry.StatusExperimental,
	Since:       "1.0.0",
}

func init() {
	featureregistry.Register(ClientEventLog)
}
