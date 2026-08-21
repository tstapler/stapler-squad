package features

import "github.com/tstapler/stapler-squad/server/featureregistry"

var DebugCreateSnapshot = featureregistry.Feature{
	ID:          "debug-create-snapshot",
	Title:       "Create Debug Snapshot",
	Description: "Creates a debug snapshot of the current session state for diagnostics.",
	RPCIDs:      []string{"debug:create-snapshot"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

func init() {
	featureregistry.Register(DebugCreateSnapshot)
}
