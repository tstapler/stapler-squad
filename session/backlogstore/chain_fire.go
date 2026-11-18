package backlogstore

import (
	"context"
	"fmt"
	"time"

	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/ent/backlogitem"
)

// ClaimChainFire atomically claims item id's pipeline chain-fire attempt by
// flipping chain_fired from false to true, conditioned on both chain_fired
// still being false and updated_at still matching expectedUpdatedAt — a
// genuine SQL-level compare-and-swap folded into the UPDATE's own WHERE
// clause, the same pattern (and for the identical reason) as
// TransitionBacklogItemStatus's precondition handling above (see that
// method's doc comment, BUG-026): a Get-then-check-in-Go-then-write sequence
// (which UpdateBacklogItem's precondition parameter still is, deliberately
// not reused here) leaves a race window wide enough for two concurrent
// callers to both pass the check and both issue their own write. Used by
// ChainFirer.Fire to guarantee at most one goroutine ever reaches
// TriggerFirer.FireTriggerChained for a given item (webhook-triggers Task
// 6.2.1d — the happy-path async dispatch and TriggerChainReconciler's
// periodic sweep can otherwise race on the same item).
//
// Returns claimed=false (not an error) when another caller already won the
// claim or the row's updated_at moved for any other reason — callers must
// treat that as "someone else is handling this," not a failure.
func (b *Backlog) ClaimChainFire(ctx context.Context, id string, expectedUpdatedAt time.Time) (claimed bool, err error) {
	parsedID, err := b.ResolveBacklogItemLookup(ctx, id)
	if err != nil {
		return false, fmt.Errorf("%w: invalid id %q: %v", domain.ErrNotFound, id, err)
	}

	affected, err := b.client.BacklogItem.Update().
		Where(backlogitem.ID(parsedID), backlogitem.ChainFired(false), backlogitem.UpdatedAtEQ(expectedUpdatedAt)).
		SetChainFired(true).
		Save(ctx)
	if err != nil {
		return false, fmt.Errorf("claim chain fire for item %s: %w", id, err)
	}
	if affected == 0 {
		return false, nil
	}

	// Best-effort publish: never blocks or fails the claim itself.
	if item, getErr := b.client.BacklogItem.Get(ctx, parsedID); getErr == nil {
		result := BacklogItemToData(item)
		b.PublishItemChanged(ctx, &result, backlogdto.BacklogItemChange{
			Kind:          backlogdto.ChangeItemUpdated,
			UpdatedFields: []string{"chainFired"},
		})
	}
	return true, nil
}

// RevertChainFireClaim unconditionally resets chain_fired back to false —
// used by ChainFirer.Fire to release its claim after a subsequent
// FireTriggerChained attempt fails, so TriggerChainReconciler retries on its
// next tick. Safe to call unconditionally (no precondition): only the
// goroutine that just won ClaimChainFire's claim for this item can ever reach
// this call, so nothing else can be racing this specific revert.
func (b *Backlog) RevertChainFireClaim(ctx context.Context, id string) error {
	parsedID, err := b.ResolveBacklogItemLookup(ctx, id)
	if err != nil {
		return fmt.Errorf("%w: invalid id %q: %v", domain.ErrNotFound, id, err)
	}
	if _, err := b.client.BacklogItem.UpdateOneID(parsedID).SetChainFired(false).Save(ctx); err != nil {
		return fmt.Errorf("revert chain fire claim for item %s: %w", id, err)
	}
	return nil
}
