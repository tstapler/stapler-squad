package features

import "github.com/tstapler/stapler-squad/server/featureregistry"

var FileGetContent = featureregistry.Feature{
	ID:          "file-get-content",
	Title:       "Get File Content",
	Description: "Retrieves the text content of a file within a session worktree.",
	RPCIDs:      []string{"file:get-content"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

var FileList = featureregistry.Feature{
	ID:          "file-list",
	Title:       "List Files",
	Description: "Lists files and directories within a session worktree, respecting gitignore rules.",
	RPCIDs:      []string{"file:list"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

var FileSearch = featureregistry.Feature{
	ID:          "file-search",
	Title:       "Search Files",
	Description: "Searches for files by name within a session worktree with gitignore-aware filtering.",
	RPCIDs:      []string{"file:search"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

func init() {
	featureregistry.Register(FileGetContent)
	featureregistry.Register(FileList)
	featureregistry.Register(FileSearch)
}
