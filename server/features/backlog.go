package features

import "github.com/tstapler/stapler-squad/server/featureregistry"

// BacklogApprovePlan describes the approve-plan RPC for backlog items.
var BacklogApprovePlan = featureregistry.Feature{
	ID:          "backlog-approve-plan",
	Title:       "Approve Plan",
	Description: "Approves the plan artifact for a backlog item, marking it ready for execution.",
	RPCIDs:      []string{"backlog:approve-plan"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

// BacklogArchiveItem describes the archive-backlog-item RPC.
var BacklogArchiveItem = featureregistry.Feature{
	ID:          "backlog-archive-item",
	Title:       "Archive Backlog Item",
	Description: "Archives a backlog item, removing it from the active queue.",
	RPCIDs:      []string{"backlog:archive-item"},
	Status:      featureregistry.StatusExperimental,
	Since:       "1.0.0",
}

// BacklogAttachSession describes the attach-session-to-item RPC.
var BacklogAttachSession = featureregistry.Feature{
	ID:          "backlog-attach-session",
	Title:       "Attach Session To Item",
	Description: "Attaches an existing AI agent session to a backlog item.",
	RPCIDs:      []string{"backlog:attach-session"},
	Status:      featureregistry.StatusExperimental,
	Since:       "1.0.0",
}

// BacklogCreateItem describes the create-backlog-item RPC.
var BacklogCreateItem = featureregistry.Feature{
	ID:          "backlog-create-item",
	Title:       "Create Backlog Item",
	Description: "Creates a new backlog item with a title and optional metadata.",
	RPCIDs:      []string{"backlog:create-item"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

// BacklogCreateSource describes the create-item-source RPC.
var BacklogCreateSource = featureregistry.Feature{
	ID:          "backlog-create-source",
	Title:       "Create Item Source",
	Description: "Creates a new item source that feeds backlog items from an external integration.",
	RPCIDs:      []string{"backlog:create-source"},
	Status:      featureregistry.StatusExperimental,
	Since:       "1.0.0",
}

// BacklogDeleteSource describes the delete-item-source RPC.
var BacklogDeleteSource = featureregistry.Feature{
	ID:          "backlog-delete-source",
	Title:       "Delete Item Source",
	Description: "Deletes an item source and stops ingestion of backlog items from it.",
	RPCIDs:      []string{"backlog:delete-source"},
	Status:      featureregistry.StatusExperimental,
	Since:       "1.0.0",
}

// BacklogGetItem describes the get-backlog-item RPC.
var BacklogGetItem = featureregistry.Feature{
	ID:          "backlog-get-item",
	Title:       "Get Backlog Item",
	Description: "Retrieves a single backlog item by ID with its full details.",
	RPCIDs:      []string{"backlog:get-item"},
	Status:      featureregistry.StatusExperimental,
	Since:       "1.0.0",
}

// BacklogGetSyncHistory describes the get-sync-history RPC.
var BacklogGetSyncHistory = featureregistry.Feature{
	ID:          "backlog-get-sync-history",
	Title:       "Get Sync History",
	Description: "Retrieves the synchronization history for a backlog item source.",
	RPCIDs:      []string{"backlog:get-sync-history"},
	Status:      featureregistry.StatusExperimental,
	Since:       "1.0.0",
}

// BacklogListItems describes the list-backlog-items RPC.
var BacklogListItems = featureregistry.Feature{
	ID:          "backlog-list-items",
	Title:       "List Backlog Items",
	Description: "Lists backlog items with optional filtering by status and other criteria.",
	RPCIDs:      []string{"backlog:list-items"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

// BacklogListSources describes the list-item-sources RPC.
var BacklogListSources = featureregistry.Feature{
	ID:          "backlog-list-sources",
	Title:       "List Item Sources",
	Description: "Lists all configured item sources that feed backlog items.",
	RPCIDs:      []string{"backlog:list-sources"},
	Status:      featureregistry.StatusExperimental,
	Since:       "1.0.0",
}

// BacklogOverrideVerdict describes the override-verdict RPC.
var BacklogOverrideVerdict = featureregistry.Feature{
	ID:          "backlog-override-verdict",
	Title:       "Override Verdict",
	Description: "Overrides the triage verdict for a backlog item with a manual decision.",
	RPCIDs:      []string{"backlog:override-verdict"},
	Status:      featureregistry.StatusExperimental,
	Since:       "1.0.0",
}

// BacklogSpawnSessionAutonomous describes the spawn-session-from-item (autonomous) RPC.
var BacklogSpawnSessionAutonomous = featureregistry.Feature{
	ID:          "backlog-spawn-session-autonomous",
	Title:       "Spawn Session From Item (Autonomous)",
	Description: "Spawns an autonomous AI agent session from a backlog item without human-in-the-loop confirmation.",
	RPCIDs:      []string{"backlog:spawn-session-autonomous"},
	Status:      featureregistry.StatusExperimental,
	Since:       "1.0.0",
}

// BacklogSpawnSession describes the spawn-session-from-item RPC.
var BacklogSpawnSession = featureregistry.Feature{
	ID:          "backlog-spawn-session",
	Title:       "Spawn Session From Item",
	Description: "Spawns an AI agent session from a backlog item to begin working on it.",
	RPCIDs:      []string{"backlog:spawn-session"},
	Status:      featureregistry.StatusExperimental,
	Since:       "1.0.0",
}

// BacklogSuggestNext describes the suggest-next-item RPC.
var BacklogSuggestNext = featureregistry.Feature{
	ID:          "backlog-suggest-next",
	Title:       "Suggest Next Item",
	Description: "Suggests the next backlog item to work on based on priority and context.",
	RPCIDs:      []string{"backlog:suggest-next"},
	Status:      featureregistry.StatusExperimental,
	Since:       "1.0.0",
}

// BacklogTransitionStatus describes the transition-backlog-item-status RPC.
var BacklogTransitionStatus = featureregistry.Feature{
	ID:          "backlog-transition-status",
	Title:       "Transition Backlog Item Status",
	Description: "Transitions a backlog item through its lifecycle status states.",
	RPCIDs:      []string{"backlog:transition-status"},
	Status:      featureregistry.StatusExperimental,
	Since:       "1.0.0",
}

// BacklogTriggerReReview describes the trigger-re-review RPC.
var BacklogTriggerReReview = featureregistry.Feature{
	ID:          "backlog-trigger-re-review",
	Title:       "Trigger Re-Review",
	Description: "Triggers a re-review of a backlog item to re-evaluate its triage verdict.",
	RPCIDs:      []string{"backlog:trigger-re-review"},
	Status:      featureregistry.StatusExperimental,
	Since:       "1.0.0",
}

// BacklogTriggerSync describes the trigger-sync RPC.
var BacklogTriggerSync = featureregistry.Feature{
	ID:          "backlog-trigger-sync",
	Title:       "Trigger Sync",
	Description: "Triggers an immediate synchronization of backlog items from configured sources.",
	RPCIDs:      []string{"backlog:trigger-sync"},
	Status:      featureregistry.StatusExperimental,
	Since:       "1.0.0",
}

// BacklogTriggerTriage describes the trigger-triage RPC.
var BacklogTriggerTriage = featureregistry.Feature{
	ID:          "backlog-trigger-triage",
	Title:       "Trigger Triage",
	Description: "Triggers AI-powered triage of pending backlog items to assign priorities and verdicts.",
	RPCIDs:      []string{"backlog:trigger-triage"},
	Status:      featureregistry.StatusExperimental,
	Since:       "1.0.0",
}

// BacklogUpdateItem describes the update-backlog-item RPC.
var BacklogUpdateItem = featureregistry.Feature{
	ID:          "backlog-update-item",
	Title:       "Update Backlog Item",
	Description: "Updates the fields of an existing backlog item such as title or description.",
	RPCIDs:      []string{"backlog:update-item"},
	Status:      featureregistry.StatusExperimental,
	Since:       "1.0.0",
}

// BacklogUpdateSource describes the update-item-source RPC.
var BacklogUpdateSource = featureregistry.Feature{
	ID:          "backlog-update-source",
	Title:       "Update Item Source",
	Description: "Updates the configuration of an existing backlog item source.",
	RPCIDs:      []string{"backlog:update-source"},
	Status:      featureregistry.StatusExperimental,
	Since:       "1.0.0",
}

func init() {
	featureregistry.Register(BacklogApprovePlan)
	featureregistry.Register(BacklogArchiveItem)
	featureregistry.Register(BacklogAttachSession)
	featureregistry.Register(BacklogCreateItem)
	featureregistry.Register(BacklogCreateSource)
	featureregistry.Register(BacklogDeleteSource)
	featureregistry.Register(BacklogGetItem)
	featureregistry.Register(BacklogGetSyncHistory)
	featureregistry.Register(BacklogListItems)
	featureregistry.Register(BacklogListSources)
	featureregistry.Register(BacklogOverrideVerdict)
	featureregistry.Register(BacklogSpawnSessionAutonomous)
	featureregistry.Register(BacklogSpawnSession)
	featureregistry.Register(BacklogSuggestNext)
	featureregistry.Register(BacklogTransitionStatus)
	featureregistry.Register(BacklogTriggerReReview)
	featureregistry.Register(BacklogTriggerSync)
	featureregistry.Register(BacklogTriggerTriage)
	featureregistry.Register(BacklogUpdateItem)
	featureregistry.Register(BacklogUpdateSource)
}
