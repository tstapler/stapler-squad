package features

import "github.com/tstapler/stapler-squad/server/featureregistry"

// FlagsGet describes the GetFeatureFlags RPC.
var FlagsGet = featureregistry.Feature{
	ID:          "get-feature-flags",
	Title:       "Get Feature Flags",
	Description: "Retrieves the current state of all feature flags for the running instance.",
	RPCIDs:      []string{"GetFeatureFlags"},
	Status:      featureregistry.StatusExperimental,
	Since:       "1.0.0",
}

// FlagsUpdate describes the UpdateFeatureFlag RPC.
var FlagsUpdate = featureregistry.Feature{
	ID:          "update-feature-flag",
	Title:       "Update Feature Flag",
	Description: "Updates the enabled state of a specific feature flag at runtime.",
	RPCIDs:      []string{"UpdateFeatureFlag"},
	Status:      featureregistry.StatusExperimental,
	Since:       "1.0.0",
}

func init() {
	featureregistry.Register(FlagsGet)
	featureregistry.Register(FlagsUpdate)
}
