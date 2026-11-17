package backlogdto

import "time"

// ItemSourceData is the domain model for an external item source.
type ItemSourceData struct {
	ID                    string
	PluginID              string
	DisplayName           string
	Config                string // JSON, may contain encrypted token
	Enabled               bool
	ForwardSyncEnabled    bool
	BackwardSyncEnabled   bool
	ForwardSyncCloseLabel string
	TokenConfigured       bool
	LastSyncedAt          *time.Time
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

// ItemSourceUpdate carries the mutable fields for UpdateItemSource.
type ItemSourceUpdate struct {
	DisplayName           *string
	Enabled               *bool
	ForwardSyncEnabled    *bool
	BackwardSyncEnabled   *bool
	ForwardSyncCloseLabel *string
	Config                *string
}

// BacklogItemDependencyEdge names a blocker/blocked pair explicitly so the
// two bare ID strings can't be silently swapped at a call site — see
// the `primitive-obsession-checklist` skill.
type BacklogItemDependencyEdge struct {
	// BlockerID is the item that must reach a resolved status (done or
	// archived) before BlockedID is eligible for dequeue/start.
	BlockerID string
	// BlockedID is the dependent item, gated until BlockerID resolves.
	BlockedID string
}

// BacklogItemDependencyRef identifies the other end of a dependency edge with
// just enough context (title, status) to render "blocked by <title>".
type BacklogItemDependencyRef struct {
	ItemID   string
	PublicID string // "" when the item has no public ID yet
	Title    string
	Status   string
}

// DeletedItemSessionCostEntry is a ledger row preserving one deleted
// ItemSession's cost attribution — written by DeleteBacklogItem just before
// the item's ItemSession rows are hard-deleted. Returned by
// GetDeletedItemSessionCostLedger. Field names mirror ItemSessionBacklogEntry
// so callers can fold both shapes through the same SessionMeta path.
type DeletedItemSessionCostEntry struct {
	ConversationUUID string
	SessionUUID      string
	SessionRole      string
	ItemID           string
	ItemTitle        string
	EstimatedCostUsd float64
	CostPriced       bool
	// CreatedAt is the original ItemSession's created_at (not this ledger
	// row's deleted_at) — Insights' time-range filter needs the work's
	// original timestamp.
	CreatedAt time.Time
}
