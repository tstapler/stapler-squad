package features

import "github.com/tstapler/stapler-squad/server/featureregistry"

// ClaudeConfigGet describes the get-claude-config RPC.
var ClaudeConfigGet = featureregistry.Feature{
	ID:          "claude-config-get",
	Title:       "Get Claude Config",
	Description: "Retrieves the Claude configuration for a given context.",
	RPCIDs:      []string{"claude-config:get"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

// ClaudeConfigList describes the list-claude-configs RPC.
var ClaudeConfigList = featureregistry.Feature{
	ID:          "claude-config-list",
	Title:       "List Claude Configs",
	Description: "Lists all available Claude configurations.",
	RPCIDs:      []string{"claude-config:list"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

// ClaudeConfigUpdate describes the update-claude-config RPC.
var ClaudeConfigUpdate = featureregistry.Feature{
	ID:          "claude-config-update",
	Title:       "Update Claude Config",
	Description: "Updates the Claude configuration for a given context.",
	RPCIDs:      []string{"claude-config:update"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

func init() {
	featureregistry.Register(ClaudeConfigGet)
	featureregistry.Register(ClaudeConfigList)
	featureregistry.Register(ClaudeConfigUpdate)
}
