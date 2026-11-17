package session

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/backlogitem"
	"github.com/tstapler/stapler-squad/session/ent/backlogitemdependency"
	"github.com/tstapler/stapler-squad/session/ent/itemsession"
	"github.com/tstapler/stapler-squad/session/instance"
)

// GetRepoPathAndLatestCompletedWorkSessionCommits returns itemID's RepoPath
// plus the Base/LastCommitSha (and session UUID) of its most recent completed (session_role ==
// work, ended_at set) ItemSession — the minimal data
// Storage.ComputeCurrentDiffHash needs. Unlike GetBacklogItem (which also
// eager-loads StatusEvents/ProgressNotes) and ListItemSessions (unbounded,
// eager-loads ReviewVerdict for every session), this pushes the "most recent
// completed work session" filter/order/limit into SQL — two bounded, no-edge
// queries regardless of how many sessions/events/notes the item has
// accumulated, rather than O(sessions_for_item) work on every review-verdict
// save. baseSHA/headSHA are both "" (no error) when the item has no
// completed work session yet.
func (r *EntRepository) GetRepoPathAndLatestCompletedWorkSessionCommits(ctx context.Context, itemID string) (repoPath, baseSHA, headSHA, workSessionUUID string, err error) {
	parsedID, err := r.resolveBacklogItemLookup(ctx, itemID)
	if err != nil {
		return "", "", "", "", fmt.Errorf("%w: invalid id %q: %v", domain.ErrNotFound, itemID, err)
	}

	item, err := r.Client.BacklogItem.Query().
		Where(backlogitem.ID(parsedID)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return "", "", "", "", fmt.Errorf("%w: backlog item %s", domain.ErrNotFound, itemID)
		}
		return "", "", "", "", fmt.Errorf("failed to get backlog item %s: %w", itemID, err)
	}

	ws, err := r.Client.ItemSession.Query().
		Where(
			itemsession.HasBacklogItemWith(backlogitem.ID(parsedID)),
			itemsession.SessionRoleEQ(instance.SessionRoleWork),
			itemsession.EndedAtNotNil(),
		).
		Order(ent.Desc(itemsession.FieldCreatedAt)).
		First(ctx)
	if ent.IsNotFound(err) {
		return item.RepoPath, "", "", "", nil
	}
	if err != nil {
		return "", "", "", "", fmt.Errorf("failed to query latest completed work session for item %s: %w", itemID, err)
	}
	return item.RepoPath, ws.BaseCommitSha, ws.LastCommitSha, ws.SessionUUID, nil
}

// ListBacklogItemDependencies returns both directions of itemID's dependency
// edges: blockedBy (items itemID waits on) and blocking (items waiting on
// itemID). Resolved edges are included — callers decide from Status whether a
// blocker is still unresolved. Results are ordered by edge creation time for a
// stable display. Two queries total, each eager-loading the far end.
func (r *EntRepository) ListBacklogItemDependencies(ctx context.Context, itemID string) (blockedBy, blocking []backlogdto.BacklogItemDependencyRef, err error) {
	parsed, err := r.resolveBacklogItemLookup(ctx, itemID)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: invalid id %q: %v", domain.ErrNotFound, itemID, err)
	}

	inbound, err := r.Client.BacklogItemDependency.Query().
		Where(backlogitemdependency.BlockedIDEQ(parsed)).
		WithBlocker().
		Order(ent.Asc(backlogitemdependency.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list blockers of %s: %w", itemID, err)
	}
	outbound, err := r.Client.BacklogItemDependency.Query().
		Where(backlogitemdependency.BlockerIDEQ(parsed)).
		WithBlocked().
		Order(ent.Asc(backlogitemdependency.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list items blocked by %s: %w", itemID, err)
	}

	for _, row := range inbound {
		if row.Edges.Blocker != nil {
			blockedBy = append(blockedBy, dependencyRefFromItem(row.Edges.Blocker))
		}
	}
	for _, row := range outbound {
		if row.Edges.Blocked != nil {
			blocking = append(blocking, dependencyRefFromItem(row.Edges.Blocked))
		}
	}
	return blockedBy, blocking, nil
}

// UnresolvedBlockersForItems returns, per blocked item ID, the blockers that
// have not yet reached done/archived (same resolved definition as
// UnresolvedBlockerItemIDs), eager-loading each blocker's title/status in a
// single query so list views avoid an N+1. Items with no unresolved blocker
// are absent from the map.
func (r *EntRepository) UnresolvedBlockersForItems(ctx context.Context, itemIDs []string) (map[string][]backlogdto.BacklogItemDependencyRef, error) {
	result := make(map[string][]backlogdto.BacklogItemDependencyRef)
	if len(itemIDs) == 0 {
		return result, nil
	}
	byUUID := make(map[uuid.UUID]string, len(itemIDs))
	parsed := make([]uuid.UUID, 0, len(itemIDs))
	for _, id := range itemIDs {
		u, err := r.resolveBacklogItemLookup(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid id %q: %v", domain.ErrNotFound, id, err)
		}
		byUUID[u] = id
		parsed = append(parsed, u)
	}
	// Chunk the IN list to stay under SQLite's bound-variable limit.
	const chunkSize = 500
	var rows []*ent.BacklogItemDependency
	for start := 0; start < len(parsed); start += chunkSize {
		end := min(start+chunkSize, len(parsed))
		chunk, err := r.Client.BacklogItemDependency.Query().
			Where(
				backlogitemdependency.BlockedIDIn(parsed[start:end]...),
				backlogitemdependency.HasBlockerWith(backlogitem.StatusNotIn(domain.TerminalStatusStrings()...)),
			).
			WithBlocker().
			Order(ent.Asc(backlogitemdependency.FieldCreatedAt)).
			All(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to load unresolved blockers: %w", err)
		}
		rows = append(rows, chunk...)
	}
	for _, row := range rows {
		if row.Edges.Blocker == nil {
			continue
		}
		key := byUUID[row.BlockedID]
		result[key] = append(result[key], dependencyRefFromItem(row.Edges.Blocker))
	}
	return result, nil
}

func dependencyRefFromItem(item *ent.BacklogItem) backlogdto.BacklogItemDependencyRef {
	return backlogdto.BacklogItemDependencyRef{
		ItemID:   item.ID.String(),
		PublicID: item.PublicID,
		Title:    item.Title,
		Status:   item.Status,
	}
}

// UnresolvedBlockerItemIDs returns, for the given candidate itemIDs, the
// subset that have at least one dependency edge whose blocker has not yet
// reached a resolved status. A blocker counts as resolved once it reaches
// BacklogStatusDone (shipped) or BacklogStatusArchived (won't ship) — an
// archived blocker is never coming back to "done", so treating it as still
// blocking would permanently strand its dependent. Batches the check into a
// single query (rather than one per candidate) to avoid N+1 queries in
// dequeue paths.
func (r *EntRepository) UnresolvedBlockerItemIDs(ctx context.Context, itemIDs []string) (map[string]bool, error) {
	result := make(map[string]bool)
	if len(itemIDs) == 0 {
		return result, nil
	}

	parsedByString := make(map[uuid.UUID]string, len(itemIDs))
	parsedIDs := make([]uuid.UUID, 0, len(itemIDs))
	for _, id := range itemIDs {
		parsed, err := r.resolveBacklogItemLookup(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid id %q: %v", domain.ErrNotFound, id, err)
		}
		parsedByString[parsed] = id
		parsedIDs = append(parsedIDs, parsed)
	}

	rows, err := r.Client.BacklogItemDependency.Query().
		Where(
			backlogitemdependency.BlockedIDIn(parsedIDs...),
			backlogitemdependency.HasBlockerWith(
				backlogitem.StatusNotIn(domain.TerminalStatusStrings()...),
			),
		).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query unresolved backlog item dependencies: %w", err)
	}
	for _, row := range rows {
		result[parsedByString[row.BlockedID]] = true
	}
	return result, nil
}

// UnresolvedBlockerIDs returns the blocker item IDs still unresolved for a
// single blocked item, for building a human-readable stuck-reason message.
// Unlike UnresolvedBlockerItemIDs (batched presence check across many
// candidates), this returns which specific items are doing the blocking.
func (r *EntRepository) UnresolvedBlockerIDs(ctx context.Context, itemID string) ([]string, error) {
	parsed, err := r.resolveBacklogItemLookup(ctx, itemID)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid id %q: %v", domain.ErrNotFound, itemID, err)
	}

	rows, err := r.Client.BacklogItemDependency.Query().
		Where(
			backlogitemdependency.BlockedIDEQ(parsed),
			backlogitemdependency.HasBlockerWith(
				backlogitem.StatusNotIn(domain.TerminalStatusStrings()...),
			),
		).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query unresolved backlog item dependencies: %w", err)
	}

	blockerIDs := make([]string, 0, len(rows))
	for _, row := range rows {
		blockerIDs = append(blockerIDs, row.BlockerID.String())
	}
	return blockerIDs, nil
}
