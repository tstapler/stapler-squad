package session

import (
	"context"
	"fmt"
	"sort"

	"github.com/google/uuid"

	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/backlogitem"
	"github.com/tstapler/stapler-squad/session/ent/itemsession"
	"github.com/tstapler/stapler-squad/session/ent/sessionpr"
)

// ItemGuardView is what the retention sweeper and the cleanup gate need to know
// about one backlog item's own PRs.
type ItemGuardView struct {
	// Direct are the item-direct links (agent-reported, Jules, manual) with the
	// item's tombstoned PRs removed.
	Direct []PRView
	// LegacyUnrepresented is true when the item's scalar pr_number/pr_url is
	// neither carried by a link of the item or its sessions nor covered by a
	// tombstone, or cannot be turned into a ref at all. It fails closed.
	LegacyUnrepresented bool
}

// ListItemGuardViews returns one ItemGuardView per requested item id. Statements
// per chunk of 500 ids: links, states, tombstones (skipped when no id has a
// direct link) and the item rows; the legacy coverage reads run only for items
// with pr_number > 0.
func (r *EntRepository) ListItemGuardViews(ctx context.Context, itemIDs []string) (map[string]ItemGuardView, error) {
	out := make(map[string]ItemGuardView, len(itemIDs))
	for _, chunk := range chunkSlice(itemIDs, itemSessionBatchChunk) {
		if err := r.fillItemGuardChunk(ctx, chunk, out); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (r *EntRepository) fillItemGuardChunk(ctx context.Context, ids []string, out map[string]ItemGuardView) error {
	direct, err := r.directItemViews(ctx, ids)
	if err != nil {
		return err
	}
	legacy, err := r.legacyUnrepresentedItems(ctx, ids)
	if err != nil {
		return err
	}
	for _, id := range ids {
		out[id] = ItemGuardView{Direct: direct[id], LegacyUnrepresented: legacy[id]}
	}
	return nil
}

func (r *EntRepository) directItemViews(ctx context.Context, ids []string) (map[string][]PRView, error) {
	byItem := make(map[string][]PRView)
	linkRows, err := r.Client.SessionPR.Query().Where(sessionpr.BacklogItemIDIn(ids...)).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list item guard links: %w", err)
	}
	if len(linkRows) == 0 {
		return byItem, nil
	}
	views, err := r.joinLinkStates(ctx, linkRows)
	if err != nil {
		return nil, err
	}
	tombs, err := r.ListItemTombstones(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, v := range views {
		if !containsRef(tombs[v.BacklogItemID], v.Ref) {
			byItem[v.BacklogItemID] = append(byItem[v.BacklogItemID], v)
		}
	}
	return byItem, nil
}

func (r *EntRepository) legacyUnrepresentedItems(ctx context.Context, ids []string) (map[string]bool, error) {
	uuids := make([]uuid.UUID, 0, len(ids))
	for _, id := range ids {
		if u, err := uuid.Parse(id); err == nil {
			uuids = append(uuids, u)
		}
	}
	out := make(map[string]bool)
	if len(uuids) == 0 {
		return out, nil
	}
	items, err := r.Client.BacklogItem.Query().
		Where(backlogitem.IDIn(uuids...), backlogitem.PrNumberGT(0)).
		Select(backlogitem.FieldID, backlogitem.FieldPrURL, backlogitem.FieldPrNumber).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list item guard rows: %w", err)
	}
	if len(items) == 0 {
		return out, nil
	}
	itemIDs := make([]string, len(items))
	itemUUIDs := make([]uuid.UUID, len(items))
	for i, it := range items {
		itemIDs[i], itemUUIDs[i] = it.ID.String(), it.ID
	}
	cov, err := r.itemCoverage(ctx, itemIDs, itemUUIDs)
	if err != nil {
		return nil, err
	}
	for _, it := range items {
		id := it.ID.String()
		_, unrepresented, perr := ItemHasUnrepresentedLegacyPR(it.PrURL, it.PrNumber, cov(id))
		out[id] = perr != nil || unrepresented
	}
	return out, nil
}

// ListItemIDsBySessionUUIDs returns every distinct item id of each session in
// one item_session query per chunk of 500. Unlike
// ListItemSessionsBySessionUUIDs it does not keep only the newest row per uuid.
func (r *EntRepository) ListItemIDsBySessionUUIDs(ctx context.Context, uuids []string) (map[string][]string, error) {
	out := make(map[string][]string)
	seen := make(map[[2]string]struct{})
	for _, chunk := range chunkSlice(uuids, itemSessionBatchChunk) {
		rows, err := r.Client.ItemSession.Query().
			Where(itemsession.SessionUUIDIn(chunk...)).
			WithBacklogItem(func(q *ent.BacklogItemQuery) { q.Select(backlogitem.FieldID) }).
			All(ctx)
		if err != nil {
			return nil, fmt.Errorf("list item ids by session uuids: %w", err)
		}
		for _, row := range rows {
			if row.Edges.BacklogItem == nil {
				continue
			}
			id := row.Edges.BacklogItem.ID.String()
			key := [2]string{row.SessionUUID, id}
			if _, dup := seen[key]; dup {
				continue
			}
			seen[key] = struct{}{}
			out[row.SessionUUID] = append(out[row.SessionUUID], id)
		}
	}
	for k := range out {
		sort.Strings(out[k])
	}
	return out, nil
}
