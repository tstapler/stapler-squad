package features

import "github.com/tstapler/stapler-squad/server/featureregistry"

// ShellDelete describes the DeleteShell RPC.
var ShellDelete = featureregistry.Feature{
	ID:          "delete-shell",
	Title:       "Delete Shell",
	Description: "Deletes a shell pane from a session, terminating its process.",
	RPCIDs:      []string{"DeleteShell"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

// ShellList describes the ListShells RPC.
var ShellList = featureregistry.Feature{
	ID:          "list-shells",
	Title:       "List Shells",
	Description: "Lists all active shell panes associated with a given session.",
	RPCIDs:      []string{"ListShells"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

// ShellRestart describes the RestartShell RPC.
var ShellRestart = featureregistry.Feature{
	ID:          "restart-shell",
	Title:       "Restart Shell",
	Description: "Restarts a specific shell pane within a session.",
	RPCIDs:      []string{"RestartShell"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

// ShellSpawn describes the SpawnShell RPC.
var ShellSpawn = featureregistry.Feature{
	ID:          "spawn-shell",
	Title:       "Spawn Shell",
	Description: "Spawns a new shell pane inside a running session.",
	RPCIDs:      []string{"SpawnShell"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

// ShellStop describes the StopShell RPC.
var ShellStop = featureregistry.Feature{
	ID:          "stop-shell",
	Title:       "Stop Shell",
	Description: "Stops a running shell pane within a session without deleting it.",
	RPCIDs:      []string{"StopShell"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

func init() {
	featureregistry.Register(ShellDelete)
	featureregistry.Register(ShellList)
	featureregistry.Register(ShellRestart)
	featureregistry.Register(ShellSpawn)
	featureregistry.Register(ShellStop)
}
