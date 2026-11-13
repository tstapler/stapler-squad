package domain

// ApprovalMetadata holds metadata about a pending approval for enriching review queue items.
type ApprovalMetadata struct {
	ApprovalID string
	ToolName   string
	ToolInput  map[string]interface{}
	Cwd        string
	Orphaned   bool

	// EscalationReason and EscalationCategory explain why this request was
	// escalated for manual review (no-match/explicit-rule/domain-age/
	// unclassifiable/unexpected). Copied from PendingApproval via
	// ApprovalStore.GetApprovalMetadataBySession.
	EscalationReason   string
	EscalationCategory string

	// RiskLevel is the classifier-assigned risk level ("low"/"medium"/"high"/"critical"),
	// copied from PendingApproval.RiskLevel. "" means not recorded — never treated as "low".
	RiskLevel string
}

// ApprovalMetadataProvider provides approval metadata for enriching review queue items.
// This interface decouples the poller (session package) from the ApprovalStore (services package).
type ApprovalMetadataProvider interface {
	// GetApprovalMetadataBySession returns approval metadata for the given session ID.
	// Returns nil if no approvals exist for the session.
	GetApprovalMetadataBySession(sessionID string) []ApprovalMetadata
}
