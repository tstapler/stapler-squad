package features

import "github.com/tstapler/stapler-squad/server/featureregistry"

// CheckpointCreate describes the create-checkpoint RPC.
var CheckpointCreate = featureregistry.Feature{
	ID:          "checkpoint-create",
	Title:       "Create Checkpoint",
	Description: "Creates a named checkpoint snapshot for a running session.",
	RPCIDs:      []string{"checkpoint:create"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

// CheckpointList describes the list-checkpoints RPC.
var CheckpointList = featureregistry.Feature{
	ID:          "checkpoint-list",
	Title:       "List Checkpoints",
	Description: "Lists all checkpoints associated with a given session.",
	RPCIDs:      []string{"checkpoint:list"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

func init() {
	featureregistry.Register(CheckpointCreate)
	featureregistry.Register(CheckpointList)
}
