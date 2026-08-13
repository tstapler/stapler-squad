package features

import "github.com/tstapler/stapler-squad/server/featureregistry"

// InsightsWatch describes the WatchInsights RPC.
var InsightsWatch = featureregistry.Feature{
	ID:          "watch-insights",
	Title:       "Watch Insights",
	Description: "Streams real-time token usage and cost insight events as they are recorded.",
	RPCIDs:      []string{"WatchInsights"},
	Status:      featureregistry.StatusExperimental,
	Since:       "1.0.0",
}

func init() {
	featureregistry.Register(InsightsWatch)
}
