package features

import "github.com/tstapler/stapler-squad/server/featureregistry"

// WorkflowCreate describes the CreateWorkflow RPC.
var WorkflowCreate = featureregistry.Feature{
	ID:          "create-workflow",
	Title:       "Create Workflow",
	Description: "Creates a new reusable workflow with a slug, command, and metadata.",
	RPCIDs:      []string{"CreateWorkflow"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

// WorkflowDelete describes the DeleteWorkflow RPC.
var WorkflowDelete = featureregistry.Feature{
	ID:          "delete-workflow",
	Title:       "Delete Workflow",
	Description: "Deletes a workflow by its ID, removing it from the available workflow catalog.",
	RPCIDs:      []string{"DeleteWorkflow"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

// WorkflowList describes the ListWorkflows RPC.
var WorkflowList = featureregistry.Feature{
	ID:          "list-workflows",
	Title:       "List Workflows",
	Description: "Lists all defined workflows available for execution in the current workspace.",
	RPCIDs:      []string{"ListWorkflows"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

// WorkflowRun describes the RunWorkflow RPC.
var WorkflowRun = featureregistry.Feature{
	ID:          "run-workflow",
	Title:       "Run Workflow",
	Description: "Executes a workflow by ID within a session, running its configured command.",
	RPCIDs:      []string{"RunWorkflow"},
	Status:      featureregistry.StatusExperimental,
	Since:       "1.0.0",
}

// WorkflowUpdate describes the UpdateWorkflow RPC.
var WorkflowUpdate = featureregistry.Feature{
	ID:          "update-workflow",
	Title:       "Update Workflow",
	Description: "Updates the configuration of an existing workflow such as its command or metadata.",
	RPCIDs:      []string{"UpdateWorkflow"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

func init() {
	featureregistry.Register(WorkflowCreate)
	featureregistry.Register(WorkflowDelete)
	featureregistry.Register(WorkflowList)
	featureregistry.Register(WorkflowRun)
	featureregistry.Register(WorkflowUpdate)
}
