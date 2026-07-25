package features

import "github.com/tstapler/stapler-squad/server/featureregistry"

// ApprovalDeleteRule describes the delete-approval-rule RPC.
var ApprovalDeleteRule = featureregistry.Feature{
	ID:          "approval-delete-rule",
	Title:       "Delete Approval Rule",
	Description: "Deletes an existing approval rule by ID.",
	RPCIDs:      []string{"approval:delete-rule"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

// ApprovalGetAnalytics describes the get-approval-analytics RPC.
var ApprovalGetAnalytics = featureregistry.Feature{
	ID:          "approval-get-analytics",
	Title:       "Get Approval Analytics",
	Description: "Returns analytics and approval rate summaries over a configurable time window.",
	RPCIDs:      []string{"approval:get-analytics"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

// ApprovalListPending describes the list-pending-approvals RPC.
var ApprovalListPending = featureregistry.Feature{
	ID:          "approval-list-pending",
	Title:       "List Pending Approvals",
	Description: "Lists all pending approval requests, optionally filtered by session ID.",
	RPCIDs:      []string{"approval:list-pending"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

// ApprovalListRules describes the list-approval-rules RPC.
var ApprovalListRules = featureregistry.Feature{
	ID:          "approval-list-rules",
	Title:       "List Approval Rules",
	Description: "Lists all approval rules, with optional filtering by source.",
	RPCIDs:      []string{"approval:list-rules"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

// ApprovalResolve describes the resolve-approval RPC.
var ApprovalResolve = featureregistry.Feature{
	ID:          "approval-resolve",
	Title:       "Resolve Approval",
	Description: "Resolves a pending approval request by allowing or denying the requested action.",
	RPCIDs:      []string{"approval:resolve"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

// ApprovalUpsertRule describes the upsert-approval-rule RPC.
var ApprovalUpsertRule = featureregistry.Feature{
	ID:          "approval-upsert-rule",
	Title:       "Upsert Approval Rule",
	Description: "Creates or updates an approval rule defining auto-allow or auto-deny patterns.",
	RPCIDs:      []string{"approval:upsert-rule"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

func init() {
	featureregistry.Register(ApprovalDeleteRule)
	featureregistry.Register(ApprovalGetAnalytics)
	featureregistry.Register(ApprovalListPending)
	featureregistry.Register(ApprovalListRules)
	featureregistry.Register(ApprovalResolve)
	featureregistry.Register(ApprovalUpsertRule)
}
