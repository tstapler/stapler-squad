package features

import "github.com/tstapler/stapler-squad/server/featureregistry"

// RulesBulkUpsert describes the BulkUpsertRules RPC.
var RulesBulkUpsert = featureregistry.Feature{
	ID:          "bulk-upsert-rules",
	Title:       "Bulk Upsert Rules",
	Description: "Bulk creates or updates multiple session rules in a single request.",
	RPCIDs:      []string{"BulkUpsertRules"},
	Status:      featureregistry.StatusExperimental,
	Since:       "1.0.0",
}

// RulesExport describes the ExportRules RPC.
var RulesExport = featureregistry.Feature{
	ID:          "export-rules",
	Title:       "Export Rules",
	Description: "Exports the current set of session rules to a portable format.",
	RPCIDs:      []string{"ExportRules"},
	Status:      featureregistry.StatusExperimental,
	Since:       "1.0.0",
}

// RulesGenerateSuggested describes the GenerateSuggestedRule RPC.
var RulesGenerateSuggested = featureregistry.Feature{
	ID:          "generate-suggested-rule",
	Title:       "Generate Suggested Rule",
	Description: "Uses AI to generate suggested rules based on analytics gaps and session patterns.",
	RPCIDs:      []string{"GenerateSuggestedRule"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

// RulesValidate describes the ValidateRules RPC.
var RulesValidate = featureregistry.Feature{
	ID:          "validate-rules",
	Title:       "Validate Rules",
	Description: "Validates a set of session rules for correctness before applying them.",
	RPCIDs:      []string{"ValidateRules"},
	Status:      featureregistry.StatusExperimental,
	Since:       "1.0.0",
}

func init() {
	featureregistry.Register(RulesBulkUpsert)
	featureregistry.Register(RulesExport)
	featureregistry.Register(RulesGenerateSuggested)
	featureregistry.Register(RulesValidate)
}
