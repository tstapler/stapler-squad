package features

import "github.com/tstapler/stapler-squad/server/featureregistry"

var DirectoryRuleDelete = featureregistry.Feature{
	ID:          "directory-rule-delete",
	Title:       "Delete Directory Rule",
	Description: "Deletes a directory rule by path, removing any associated session configuration for that directory.",
	RPCIDs:      []string{"directory-rule:delete"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

var DirectoryRuleUpsert = featureregistry.Feature{
	ID:          "directory-rule-upsert",
	Title:       "Upsert Directory Rule",
	Description: "Creates or updates a directory rule, associating session configuration with a given directory path.",
	RPCIDs:      []string{"directory-rule:upsert"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

func init() {
	featureregistry.Register(DirectoryRuleDelete)
	featureregistry.Register(DirectoryRuleUpsert)
}
