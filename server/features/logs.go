package features

import "github.com/tstapler/stapler-squad/server/featureregistry"

var LogsGet = featureregistry.Feature{
	ID:          "logs-get",
	Title:       "Get Logs",
	Description: "Retrieves log output for a session by ID or title.",
	RPCIDs:      []string{"logs:get"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

func init() {
	featureregistry.Register(LogsGet)
}
