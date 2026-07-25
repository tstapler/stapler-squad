package features

import "github.com/tstapler/stapler-squad/server/featureregistry"

// ProjectCreate describes the create project RPC.
var ProjectCreate = featureregistry.Feature{
	ID:          "project-create",
	Title:       "Create Project",
	Description: "Creates a new project for grouping and organizing sessions.",
	RPCIDs:      []string{"project:create"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

// ProjectList describes the list projects RPC.
var ProjectList = featureregistry.Feature{
	ID:          "project-list",
	Title:       "List Projects",
	Description: "Lists all projects available in the current workspace.",
	RPCIDs:      []string{"project:list"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

// ProjectUpdate describes the update project RPC.
var ProjectUpdate = featureregistry.Feature{
	ID:          "project-update",
	Title:       "Update Project",
	Description: "Updates the name or metadata of an existing project.",
	RPCIDs:      []string{"project:update"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

// ProjectDelete describes the delete project RPC.
var ProjectDelete = featureregistry.Feature{
	ID:          "project-delete",
	Title:       "Delete Project",
	Description: "Deletes a project and disassociates its sessions.",
	RPCIDs:      []string{"project:delete"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

// ProjectAssignSessions describes the assign sessions to project RPC.
var ProjectAssignSessions = featureregistry.Feature{
	ID:          "project-assign-sessions",
	Title:       "Assign Sessions To Project",
	Description: "Assigns one or more sessions to a project for organizational grouping.",
	RPCIDs:      []string{"project:assign-sessions"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

func init() {
	featureregistry.Register(ProjectCreate)
	featureregistry.Register(ProjectList)
	featureregistry.Register(ProjectUpdate)
	featureregistry.Register(ProjectDelete)
	featureregistry.Register(ProjectAssignSessions)
}
