package backlogstore

import (
	"context"

	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/ent"
)

// Guidance is the pending-guidance surface the backlog store calls: it stamps
// HasPendingGuidanceRequests onto items it returns or publishes and drops the
// pending cache after an item delete. guidancestore.Repo implements it.
type Guidance interface {
	AttachPendingGuidance(ctx context.Context, item *backlogdto.BacklogItemData)
	ApplyPendingGuidanceToData(ctx context.Context, items []backlogdto.BacklogItemData)
	ApplyPendingGuidanceToSummaries(ctx context.Context, items []backlogdto.BacklogItemSummary)
	InvalidatePendingGuidance()
}

// OwnerPRRowsDeleter deletes an owner's PR link and tombstone rows inside tx
// (sessionpr.DeleteOwnerPRRows); the backlog store cannot import sessionpr
// because that would pull the github package into this leaf.
type OwnerPRRowsDeleter func(ctx context.Context, tx *ent.Tx, owner domain.PROwner) error

// Ports are the consumer-side dependencies New requires, all non-nil: a nil
// port panics on first use, as the nil embedded stores did before the ports existed.
type Ports struct {
	Guidance          Guidance
	DeleteOwnerPRRows OwnerPRRowsDeleter
}
