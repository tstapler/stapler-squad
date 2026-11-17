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

// ItemLinkWriter writes an item PR's item-direct link inside the status
// writer's transaction (session's itemPRLinkWrite: it needs the github-aware
// sessionpr package this leaf cannot import). A nil interface is a no-op.
type ItemLinkWriter interface {
	WriteTx(ctx context.Context, tx *ent.Tx, itemID string, afterLink func() error) error
}

// Ports are the consumer-side dependencies New requires, all non-nil: a nil
// Guidance, DeleteOwnerPRRows or AfterLinkHook panics on first use, as the nil
// embedded stores did before the ports existed. New has one caller,
// session.EntRepository.initStores.
type Ports struct {
	Guidance          Guidance
	DeleteOwnerPRRows OwnerPRRowsDeleter
	// AfterLinkHook returns the test hook (sessionpr.PRStore.AfterLinkHook), or nil,
	// read at call time because tests install it after construction.
	AfterLinkHook func() func() error
}
