package features

import "github.com/tstapler/stapler-squad/server/featureregistry"

// PRClose describes the close-PR RPC.
var PRClose = featureregistry.Feature{
	ID:          "pr-close",
	Title:       "Close PR",
	Description: "Closes a pull request associated with a session.",
	RPCIDs:      []string{"pr:close"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

// PRGetComments describes the get-PR-comments RPC.
var PRGetComments = featureregistry.Feature{
	ID:          "pr-get-comments",
	Title:       "Get PR Comments",
	Description: "Retrieves all review comments for a pull request associated with a session.",
	RPCIDs:      []string{"pr:get-comments"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

// PRGetInfo describes the get-PR-info RPC.
var PRGetInfo = featureregistry.Feature{
	ID:          "pr-get-info",
	Title:       "Get PR Info",
	Description: "Retrieves metadata and status information for a pull request associated with a session.",
	RPCIDs:      []string{"pr:get-info"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

// PRMerge describes the merge-PR RPC.
var PRMerge = featureregistry.Feature{
	ID:          "pr-merge",
	Title:       "Merge PR",
	Description: "Merges a pull request associated with a session.",
	RPCIDs:      []string{"pr:merge"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

// PRPostComment describes the post-PR-comment RPC.
var PRPostComment = featureregistry.Feature{
	ID:          "pr-post-comment",
	Title:       "Post PR Comment",
	Description: "Posts a review comment on a pull request associated with a session.",
	RPCIDs:      []string{"pr:post-comment"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

func init() {
	featureregistry.Register(PRClose)
	featureregistry.Register(PRGetComments)
	featureregistry.Register(PRGetInfo)
	featureregistry.Register(PRMerge)
	featureregistry.Register(PRPostComment)
}
