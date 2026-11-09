package session

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/backlogitem"
)

// RecomputeItemPRMirror makes backlog_item.pr_number/pr_url follow ActivePR of the
// derived item PR set. It writes only when the active PR exists and its number
// differs; an empty set never zeroes the stored value (a round whose PR is not
// linked yet must not blank a pr_pending item). The UPDATE is conditional on the
// number read here, so a concurrent writer wins and a pr_pending item is never
// observed with pr_number=0. The mirror is an output only: the set derivation
// never reads it as input.
func (r *EntRepository) RecomputeItemPRMirror(ctx context.Context, itemID string) (bool, error) {
	id, err := uuid.Parse(itemID)
	if err != nil {
		return false, fmt.Errorf("recompute item pr mirror: invalid item id %q: %w", itemID, err)
	}
	item, err := r.Client.BacklogItem.Get(ctx, id)
	if err != nil {
		return false, fmt.Errorf("recompute item pr mirror: load item %s: %w", itemID, err)
	}
	return r.recomputeItemPRMirror(ctx, item)
}

func (r *EntRepository) recomputeItemPRMirror(ctx context.Context, item *ent.BacklogItem) (bool, error) {
	prs, err := r.ItemPRs(ctx, item)
	if err != nil {
		return false, err
	}
	active := ActivePR(prs, "")
	if active == nil || active.Ref.Number() == item.PrNumber {
		return false, nil
	}
	url := active.URL
	if url == "" {
		url = synthesizePRURL(active.Ref)
	}
	n, err := r.Client.BacklogItem.Update().
		Where(backlogitem.ID(item.ID), backlogitem.PrNumber(item.PrNumber)).
		SetPrNumber(active.Ref.Number()).SetPrURL(url).
		Save(ctx)
	if err != nil {
		return false, fmt.Errorf("recompute item pr mirror: update item %s: %w", item.ID, err)
	}
	return n > 0, nil
}
