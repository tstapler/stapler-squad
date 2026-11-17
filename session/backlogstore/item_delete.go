package backlogstore

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/backlogitem"
	"github.com/tstapler/stapler-squad/session/ent/itemsession"
	"github.com/tstapler/stapler-squad/session/ent/reviewverdict"
)

// DeleteBacklogItem permanently removes an item and all its child records.
func (b *Backlog) DeleteBacklogItem(ctx context.Context, id string) error {
	// The item_id FK nulls out via ON DELETE SET NULL, which bypasses ent hooks.
	defer b.guidance.InvalidatePendingGuidance()
	parsedID, err := b.ResolveBacklogItemLookup(ctx, id)
	if err != nil {
		return fmt.Errorf("%w: invalid id %q: %v", domain.ErrNotFound, id, err)
	}

	// Fetch the item before deleting so a ChangeItemRemoved event can carry a
	// snapshot of it — once DeleteOneID succeeds below, the row is gone and
	// can no longer be read back.
	existing, err := b.client.BacklogItem.Get(ctx, parsedID)
	if err != nil {
		if ent.IsNotFound(err) {
			return fmt.Errorf("%w: backlog item %s", domain.ErrNotFound, id)
		}
		return fmt.Errorf("failed to get backlog item %s: %w", id, err)
	}

	// Build the publish snapshot (including ItemSessions) now, before this
	// item's sessions are deleted below — AttachItemSessionsForPublish's
	// ListItemSessions query would return nothing once they're gone.
	result := BacklogItemToData(existing)
	b.AttachItemSessionsForPublish(ctx, &result)

	// Resolve full item_session rows (not just IDs) so their cost fields can be
	// preserved in the deleted-cost ledger below before they're hard-deleted.
	itemSessions, err := b.client.ItemSession.Query().
		Where(itemsession.HasBacklogItemWith(backlogitem.ID(parsedID))).
		All(ctx)
	if err != nil {
		return fmt.Errorf("failed to query item sessions for backlog item %s: %w", id, err)
	}

	// The ledger write and the three deletes below must commit atomically: a
	// partial failure between the ledger CreateBulk and BacklogItem.DeleteOneID
	// would otherwise leave a retry racing a live ItemSession that a fresh
	// DeleteBacklogItem call would ledger AGAIN — a permanent duplicate cost
	// row (the ledger has no other retry-idempotency guard, only the
	// session_uuid unique index added alongside this transaction to make a
	// same-key retry fail loudly instead of double-counting). Follows the
	// same b.client.Tx(ctx) pattern as MarkStuck/AddBacklogItemDependency
	// above.
	tx, err := b.client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("delete backlog item: begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if len(itemSessions) > 0 {
		// Story 3: preserve each ItemSession's cost attribution in a durable
		// ledger before it's hard-deleted below — see
		// session/ent/schema/deleted_item_session_cost.go's doc comment for why
		// this is additive-ledger rather than soft-delete.
		ledgerBuilders := make([]*ent.DeletedItemSessionCostCreate, 0, len(itemSessions))
		itemSessionIDs := make([]uuid.UUID, 0, len(itemSessions))
		for _, is := range itemSessions {
			itemSessionIDs = append(itemSessionIDs, is.ID)
			ledgerBuilders = append(ledgerBuilders, tx.DeletedItemSessionCost.Create().
				SetConversationUUID(is.ConversationUUID).
				SetSessionUUID(is.SessionUUID).
				SetSessionRole(is.SessionRole).
				SetItemID(parsedID.String()).
				SetItemTitle(existing.Title).
				SetEstimatedCostUsd(is.EstimatedCostUsd).
				SetCostPriced(is.CostPriced).
				SetCreatedAt(is.CreatedAt))
		}
		if _, err := tx.DeletedItemSessionCost.CreateBulk(ledgerBuilders...).Save(ctx); err != nil {
			return fmt.Errorf("delete backlog item: write deleted-cost ledger for %s: %w", id, err)
		}

		_, err = tx.ReviewVerdict.Delete().
			Where(reviewverdict.HasItemSessionWith(itemsession.IDIn(itemSessionIDs...))).
			Exec(ctx)
		if err != nil {
			return fmt.Errorf("delete backlog item: delete review verdicts for %s: %w", id, err)
		}

		_, err = tx.ItemSession.Delete().
			Where(itemsession.IDIn(itemSessionIDs...)).
			Exec(ctx)
		if err != nil {
			return fmt.Errorf("delete backlog item: delete item sessions for %s: %w", id, err)
		}
	}

	if err := b.deleteOwnerPRRows(ctx, tx, domain.ItemPROwner(parsedID.String())); err != nil {
		return fmt.Errorf("delete backlog item: %w", err)
	}

	err = tx.BacklogItem.DeleteOneID(parsedID).Exec(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return fmt.Errorf("%w: backlog item %s", domain.ErrNotFound, id)
		}
		return fmt.Errorf("delete backlog item: delete %s: %w", id, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("delete backlog item: commit: %w", err)
	}

	// Best-effort publish: never blocks or fails the delete itself. result was
	// built above, before the item's sessions were deleted — use
	// hooks.PublishItemChanged directly (not PublishItemChanged) so this
	// pre-captured snapshot isn't clobbered by a re-query that would now find
	// no sessions.
	b.hooks.PublishItemChanged(&result, backlogdto.BacklogItemChange{
		Kind: backlogdto.ChangeItemRemoved,
	})

	return nil
}
