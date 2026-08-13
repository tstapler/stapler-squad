package features

import "github.com/tstapler/stapler-squad/server/featureregistry"

var ProfileDelete = featureregistry.Feature{
	ID:          "profile-delete",
	Title:       "Delete Profile",
	Description: "Deletes an existing profile by name from the session service.",
	RPCIDs:      []string{"profile:delete"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

var ProfileUpsert = featureregistry.Feature{
	ID:          "profile-upsert",
	Title:       "Upsert Profile",
	Description: "Creates or updates a profile in the session service.",
	RPCIDs:      []string{"profile:upsert"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

func init() {
	featureregistry.Register(ProfileDelete)
	featureregistry.Register(ProfileUpsert)
}
