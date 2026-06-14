package features

import "github.com/tstapler/stapler-squad/server/featureregistry"

// AnalyticsGetEscapeSummary describes the GetEscapeAnalyticsSummary RPC.
var AnalyticsGetEscapeSummary = featureregistry.Feature{
	ID:          "get-escape-analytics-summary",
	Title:       "Get Escape Analytics Summary",
	Description: "Returns a summarized view of escape analytics events across sessions.",
	RPCIDs:      []string{"GetEscapeAnalyticsSummary"},
	Status:      featureregistry.StatusExperimental,
	Since:       "1.0.0",
}

// AnalyticsGetProgram describes the GetProgramAnalytics RPC.
var AnalyticsGetProgram = featureregistry.Feature{
	ID:          "get-program-analytics",
	Title:       "Get Program Analytics",
	Description: "Retrieves aggregated analytics data for a specific program or agent type.",
	RPCIDs:      []string{"GetProgramAnalytics"},
	Status:      featureregistry.StatusExperimental,
	Since:       "1.0.0",
}

// AnalyticsQueryEscape describes the QueryEscapeAnalytics RPC.
var AnalyticsQueryEscape = featureregistry.Feature{
	ID:          "query-escape-analytics",
	Title:       "Query Escape Analytics",
	Description: "Queries raw escape analytics events with filtering and pagination support.",
	RPCIDs:      []string{"QueryEscapeAnalytics"},
	Status:      featureregistry.StatusExperimental,
	Since:       "1.0.0",
}

// AnalyticsGetInsightsSummary describes the GetInsightsSummary RPC.
var AnalyticsGetInsightsSummary = featureregistry.Feature{
	ID:          "get-insights-summary",
	Title:       "Get Insights Summary",
	Description: "Returns a paginated summary of token usage and cost insights across sessions.",
	RPCIDs:      []string{"GetInsightsSummary"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

func init() {
	featureregistry.Register(AnalyticsGetEscapeSummary)
	featureregistry.Register(AnalyticsGetProgram)
	featureregistry.Register(AnalyticsQueryEscape)
	featureregistry.Register(AnalyticsGetInsightsSummary)
}
