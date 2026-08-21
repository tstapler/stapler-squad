package features

import "github.com/tstapler/stapler-squad/server/featureregistry"

// WorkspaceGetInfo describes the get-workspace-info RPC.
var WorkspaceGetInfo = featureregistry.Feature{
	ID:          "workspace-get-info",
	Title:       "Get Workspace Info",
	Description: "Retrieves workspace metadata and configuration for a given session.",
	RPCIDs:      []string{"workspace:get-info"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

// WorkspaceListTargets describes the list-workspace-targets RPC.
var WorkspaceListTargets = featureregistry.Feature{
	ID:          "workspace-list-targets",
	Title:       "List Workspace Targets",
	Description: "Lists available workspace targets that a session can switch to.",
	RPCIDs:      []string{"workspace:list-targets"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

// WorkspaceSwitch describes the switch-workspace RPC.
var WorkspaceSwitch = featureregistry.Feature{
	ID:          "workspace-switch",
	Title:       "Switch Workspace",
	Description: "Switches a session to a different workspace target.",
	RPCIDs:      []string{"workspace:switch"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

func init() {
	featureregistry.Register(WorkspaceGetInfo)
	featureregistry.Register(WorkspaceListTargets)
	featureregistry.Register(WorkspaceSwitch)
}
