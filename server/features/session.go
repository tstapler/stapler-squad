package features

import "github.com/tstapler/stapler-squad/server/featureregistry"

// SessionCreate describes the create-session RPC and its UI surface.
var SessionCreate = featureregistry.Feature{
	ID:          "session-create",
	Title:       "Create Session",
	Description: "Creates a new AI agent session in a directory, worktree, or one-off mode.",
	RPCIDs:      []string{"session:create"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

// SessionList describes the list/watch session RPCs and their UI surface.
var SessionList = featureregistry.Feature{
	ID:          "session-list",
	Title:       "Session List",
	Description: "Lists and streams updates for all sessions.",
	RPCIDs:      []string{"session:list", "session:watch"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

// SessionDelete describes the delete-session RPC and its UI surface.
var SessionDelete = featureregistry.Feature{
	ID:          "session-delete",
	Title:       "Delete Session",
	Description: "Deletes a session and its associated worktree.",
	RPCIDs:      []string{"session:delete"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

// SessionAcknowledge describes the acknowledge-session RPC.
var SessionAcknowledge = featureregistry.Feature{
	ID:          "session-acknowledge",
	Title:       "Acknowledge Session",
	Description: "Acknowledges a session to mark it as seen and clear its notification state.",
	RPCIDs:      []string{"session:acknowledge"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

// SessionBatchCreate describes the batch-create-sessions RPC.
var SessionBatchCreate = featureregistry.Feature{
	ID:          "session-batch-create",
	Title:       "Batch Create Sessions",
	Description: "Creates multiple AI agent sessions concurrently with configurable parallelism limits.",
	RPCIDs:      []string{"session:batch-create"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

// SessionClearConversationState describes the clear-conversation-state RPC.
var SessionClearConversationState = featureregistry.Feature{
	ID:          "session-clear-conversation-state",
	Title:       "Clear Conversation State",
	Description: "Clears the stored conversation state for a session, resetting its history.",
	RPCIDs:      []string{"session:clear-conversation-state"},
	Status:      featureregistry.StatusExperimental,
	Since:       "1.0.0",
}

// SessionDeletePromptHistory describes the delete-prompt-history RPC.
var SessionDeletePromptHistory = featureregistry.Feature{
	ID:          "session-delete-prompt-history",
	Title:       "Delete Prompt History",
	Description: "Deletes a specific entry from the session prompt history.",
	RPCIDs:      []string{"session:delete-prompt-history"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

// SessionFork describes the fork-session RPC.
var SessionFork = featureregistry.Feature{
	ID:          "session-fork",
	Title:       "Fork Session",
	Description: "Forks an existing session from a checkpoint into a new independent session.",
	RPCIDs:      []string{"session:fork"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

// SessionGet describes the get-session RPC.
var SessionGet = featureregistry.Feature{
	ID:          "session-get",
	Title:       "Get Session",
	Description: "Retrieves a single session by its ID or title.",
	RPCIDs:      []string{"session:get"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

// SessionGetDiff describes the get-session-diff RPC.
var SessionGetDiff = featureregistry.Feature{
	ID:          "session-get-diff",
	Title:       "Get Session Diff",
	Description: "Returns the git diff for the files modified in a session.",
	RPCIDs:      []string{"session:get-diff"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

// SessionGetTerminalSnapshot describes the get-terminal-snapshot RPC.
var SessionGetTerminalSnapshot = featureregistry.Feature{
	ID:          "session-get-terminal-snapshot",
	Title:       "Get Terminal Snapshot",
	Description: "Retrieves a point-in-time snapshot of the session terminal output.",
	RPCIDs:      []string{"session:get-terminal-snapshot"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

// SessionGetVCSStatus describes the get-vcs-status RPC.
var SessionGetVCSStatus = featureregistry.Feature{
	ID:          "session-get-vcs-status",
	Title:       "Get VCS Status",
	Description: "Returns the version control system status for the files in a session workspace.",
	RPCIDs:      []string{"session:get-vcs-status"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

// SessionListBranches describes the list-branches RPC.
var SessionListBranches = featureregistry.Feature{
	ID:          "session-list-branches",
	Title:       "List Branches",
	Description: "Lists the git branches available in a given repository path.",
	RPCIDs:      []string{"session:list-branches"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

// SessionListPromptHistory describes the list-prompt-history RPC.
var SessionListPromptHistory = featureregistry.Feature{
	ID:          "session-list-prompt-history",
	Title:       "List Prompt History",
	Description: "Returns the prompt history entries recorded for a session.",
	RPCIDs:      []string{"session:list-prompt-history"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

// SessionRename describes the rename-session RPC.
var SessionRename = featureregistry.Feature{
	ID:          "session-rename",
	Title:       "Rename Session",
	Description: "Renames an existing session to a new unique title.",
	RPCIDs:      []string{"session:rename"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

// SessionRestart describes the restart-session RPC.
var SessionRestart = featureregistry.Feature{
	ID:          "session-restart",
	Title:       "Restart Session",
	Description: "Restarts a session, relaunching its AI agent process.",
	RPCIDs:      []string{"session:restart"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

// SessionRunOneShot describes the run-one-shot RPC.
var SessionRunOneShot = featureregistry.Feature{
	ID:          "session-run-one-shot",
	Title:       "Run One Shot",
	Description: "Runs a one-shot AI agent task in a session without persistent state.",
	RPCIDs:      []string{"session:run-one-shot"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

// SessionUpdate describes the update-session RPC.
var SessionUpdate = featureregistry.Feature{
	ID:          "session-update",
	Title:       "Update Session",
	Description: "Updates session metadata such as tags, title, and status.",
	RPCIDs:      []string{"session:update"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

func init() {
	featureregistry.Register(SessionCreate)
	featureregistry.Register(SessionList)
	featureregistry.Register(SessionDelete)
	featureregistry.Register(SessionAcknowledge)
	featureregistry.Register(SessionBatchCreate)
	featureregistry.Register(SessionClearConversationState)
	featureregistry.Register(SessionDeletePromptHistory)
	featureregistry.Register(SessionFork)
	featureregistry.Register(SessionGet)
	featureregistry.Register(SessionGetDiff)
	featureregistry.Register(SessionGetTerminalSnapshot)
	featureregistry.Register(SessionGetVCSStatus)
	featureregistry.Register(SessionListBranches)
	featureregistry.Register(SessionListPromptHistory)
	featureregistry.Register(SessionRename)
	featureregistry.Register(SessionRestart)
	featureregistry.Register(SessionRunOneShot)
	featureregistry.Register(SessionUpdate)
}
