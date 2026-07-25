package features

import "github.com/tstapler/stapler-squad/server/featureregistry"

// DefaultsGet describes the get-session-defaults RPC.
var DefaultsGet = featureregistry.Feature{
	ID:          "defaults-get",
	Title:       "Get Session Defaults",
	Description: "Retrieves the current session defaults configuration.",
	RPCIDs:      []string{"defaults:get"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

// DefaultsResolve describes the resolve-defaults RPC.
var DefaultsResolve = featureregistry.Feature{
	ID:          "defaults-resolve",
	Title:       "Resolve Defaults",
	Description: "Resolves effective session defaults for a given path or context.",
	RPCIDs:      []string{"defaults:resolve"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

// DefaultsUpdateGlobal describes the update-global-defaults RPC.
var DefaultsUpdateGlobal = featureregistry.Feature{
	ID:          "defaults-update-global",
	Title:       "Update Global Defaults",
	Description: "Updates the global session defaults such as the default program.",
	RPCIDs:      []string{"defaults:update-global"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

func init() {
	featureregistry.Register(DefaultsGet)
	featureregistry.Register(DefaultsResolve)
	featureregistry.Register(DefaultsUpdateGlobal)
}
