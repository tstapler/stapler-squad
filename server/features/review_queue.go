package features

import "github.com/tstapler/stapler-squad/server/featureregistry"

// ReviewQueueList describes the list/watch review-queue RPCs and their UI surface.
var ReviewQueueList = featureregistry.Feature{
	ID:          "review-queue-list",
	Title:       "Review Queue",
	Description: "Lists and streams the review queue of sessions needing attention.",
	RPCIDs:      []string{"review-queue:list", "review-queue:watch"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

// ReviewQueueAcknowledge describes the acknowledge review-queue RPC.
var ReviewQueueAcknowledge = featureregistry.Feature{
	ID:          "review-queue-acknowledge",
	Title:       "Acknowledge Review Item",
	Description: "Acknowledges (skips) an item in the review queue.",
	RPCIDs:      []string{"review-queue:acknowledge"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

// ReviewQueueGet describes the get review-queue RPC.
var ReviewQueueGet = featureregistry.Feature{
	ID:          "review-queue-get",
	Title:       "Get Review Queue",
	Description: "Fetches the current review queue of sessions requiring user attention.",
	RPCIDs:      []string{"review-queue:get"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

func init() {
	featureregistry.Register(ReviewQueueList)
	featureregistry.Register(ReviewQueueAcknowledge)
	featureregistry.Register(ReviewQueueGet)
}
