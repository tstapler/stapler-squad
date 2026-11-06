package session

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/backlogitem"
	"github.com/tstapler/stapler-squad/session/ent/sessionpr"
)

// itemPRLinkWrite is the item-direct link an item PR writer records in the same
// transaction as its pr_number write, whether or not any multi-PR scope is on, so the
// refresher polls the PR and the cleanup gate and sweeper see it. Drop, when set, is
// the replaced PR of a correction: its item-direct link is deleted without a
// tombstone. A tombstoned Ref writes no link (the operator detached it).
type itemPRLinkWrite struct {
	Ref    PRRef
	Source AttachSource
	URL    string
	Drop   *PRRef
}

// newItemPRLinkWrite builds the write for a PR association, or nil when its URL and
// number do not form a ref (the gate then fails closed on the unrepresented number).
func newItemPRLinkWrite(pr PRAssociation, replaced *PRRef) *itemPRLinkWrite {
	ref, err := ParseItemPRRef(pr.URL, pr.Number)
	if err != nil {
		log.WarningLog().Printf("[Storage] item PR link not written: %v", err)
		return nil
	}
	src := pr.Source
	if src == "" {
		src = AttachSourceManual
	}
	return &itemPRLinkWrite{Ref: ref, Source: src, URL: pr.URL, Drop: replaced}
}

// writeTx upserts the item-direct link and drops the replaced one inside tx. A nil
// receiver is a no-op.
func (w *itemPRLinkWrite) writeTx(ctx context.Context, tx *ent.Tx, itemID string, afterLink func() error) error {
	if w == nil {
		return nil
	}
	owner := ItemPROwner(itemID)
	if w.Drop != nil && *w.Drop != w.Ref {
		if err := deleteItemLinkTx(ctx, tx, owner, *w.Drop); err != nil {
			return err
		}
	}
	res, err := upsertSessionPRTx(ctx, tx, owner, w.Ref, PRLink{
		Role: PRRolePrimary, Source: w.Source, Initial: PRInitial{State: PRStateOpen, URL: w.URL},
	})
	if err != nil {
		return fmt.Errorf("write item-direct PR link: %w", err)
	}
	if res.Tombstoned {
		log.InfoLog().Printf("[Storage] item %s detached %s earlier: no item-direct link written", itemID, w.Ref)
	}
	if afterLink != nil {
		return afterLink()
	}
	return nil
}

// deleteItemLinkTx removes the owner's link to ref (no tombstone) inside tx.
func deleteItemLinkTx(ctx context.Context, tx *ent.Tx, owner PROwner, ref PRRef) error {
	ids, err := tx.PullRequestState.Query().Where(refPredicate(ref)).IDs(ctx)
	if err != nil {
		return fmt.Errorf("look up replaced PR state: %w", err)
	}
	if len(ids) == 0 {
		return nil
	}
	if _, err := tx.SessionPR.Delete().
		Where(append(linkOwnerPredicates(owner), sessionpr.PullRequestStateIDIn(ids...))...).Exec(ctx); err != nil {
		return fmt.Errorf("delete replaced item-direct link: %w", err)
	}
	return nil
}

// SetItemPRFieldsWithLink writes the item's pr_url/pr_number and its item-direct link
// in one transaction (pushAndCreatePR and shipViaAgentOrFallback, BUG-058 narrowed):
// a failure of either leaves neither. The transition to pr_pending stays a separate
// call, so BUG-058 itself is not closed.
func (r *EntRepository) SetItemPRFieldsWithLink(ctx context.Context, itemID, prURL string, prNumber int, source AttachSource) error {
	id, err := uuid.Parse(itemID)
	if err != nil {
		return fmt.Errorf("set item PR fields: invalid item id %q: %w", itemID, err)
	}
	link := newItemPRLinkWrite(PRAssociation{URL: prURL, Number: prNumber, Source: source}, nil)
	return r.withPRTx(ctx, "set item pr fields", func(tx *ent.Tx) error {
		if n, uerr := tx.BacklogItem.Update().Where(backlogitem.ID(id)).SetPrURL(prURL).SetPrNumber(prNumber).Save(ctx); uerr != nil {
			return fmt.Errorf("update item: %w", uerr)
		} else if n == 0 {
			return fmt.Errorf("%w: backlog item %s", ErrNotFound, itemID)
		}
		return link.writeTx(ctx, tx, itemID, r.afterPRLinkHook)
	})
}
