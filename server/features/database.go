package features

import "github.com/tstapler/stapler-squad/server/featureregistry"

// DatabaseGetCurrent describes the get-current-database RPC.
var DatabaseGetCurrent = featureregistry.Feature{
	ID:          "database-get-current",
	Title:       "Get Current Database",
	Description: "Returns the currently active database for the session service.",
	RPCIDs:      []string{"database:get-current"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

// DatabaseList describes the list-databases RPC.
var DatabaseList = featureregistry.Feature{
	ID:          "database-list",
	Title:       "List Databases",
	Description: "Lists all available databases known to the session service.",
	RPCIDs:      []string{"database:list"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

// DatabaseMerge describes the merge-database RPC.
var DatabaseMerge = featureregistry.Feature{
	ID:          "database-merge",
	Title:       "Merge Database",
	Description: "Merges a source database into the current database.",
	RPCIDs:      []string{"database:merge"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

// DatabaseSwitch describes the switch-database RPC.
var DatabaseSwitch = featureregistry.Feature{
	ID:          "database-switch",
	Title:       "Switch Database",
	Description: "Switches the session service to use a different database at the given path.",
	RPCIDs:      []string{"database:switch"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

func init() {
	featureregistry.Register(DatabaseGetCurrent)
	featureregistry.Register(DatabaseList)
	featureregistry.Register(DatabaseMerge)
	featureregistry.Register(DatabaseSwitch)
}
