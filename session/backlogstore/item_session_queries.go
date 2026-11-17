package backlogstore

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/backlogitem"
	"github.com/tstapler/stapler-squad/session/ent/itemsession"
)

// GetItemSession retrieves an ItemSession by entity UUID string. Loads the BacklogItem edge.
func (b *Backlog) GetItemSession(ctx context.Context, id string) (backlogdto.ItemSessionSummary, error) {
	parsedID, err := uuid.Parse(id)
	if err != nil {
		return backlogdto.ItemSessionSummary{}, fmt.Errorf("%w: invalid id %q: %v", domain.ErrNotFound, id, err)
	}

	is, err := b.client.ItemSession.Query().
		Where(itemsession.ID(parsedID)).
		WithBacklogItem().
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return backlogdto.ItemSessionSummary{}, fmt.Errorf("%w: item session %s", domain.ErrNotFound, id)
		}
		return backlogdto.ItemSessionSummary{}, fmt.Errorf("failed to get item session %s: %w", id, err)
	}
	return ItemSessionToSummary(is), nil
}

// ListItemSessions returns all ItemSessions for a given BacklogItem UUID string.
func (b *Backlog) ListItemSessions(ctx context.Context, itemID string) ([]backlogdto.ItemSessionSummary, error) {
	parsedItemID, err := b.ResolveBacklogItemLookup(ctx, itemID)
	if err != nil {
		return nil, fmt.Errorf("invalid item id %q: %w", itemID, err)
	}

	sessions, err := b.client.ItemSession.Query().
		Where(itemsession.HasBacklogItemWith(backlogitem.ID(parsedItemID))).
		WithReviewVerdict().
		Order(ent.Asc(itemsession.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list item sessions for item %s: %w", itemID, err)
	}
	result := make([]backlogdto.ItemSessionSummary, len(sessions))
	for i, is := range sessions {
		s := ItemSessionToSummary(is)
		s.BacklogItemID = itemID
		result[i] = s
	}
	return result, nil
}

// GetBaseCommitSHAsForSessions returns a map of sessionUUID → base_commit_sha for
// the given session UUIDs, including only rows with a non-empty value. Used at
// startup to restore dirBaseSHA for directory-mode backlog sessions — the
// counterpart to the SetDirBaseSHA call at spawn.
//
// Reads base_commit_sha, falling back to last_commit_sha only for rows written
// before the two were split. That fallback is safe precisely because the bug
// being fixed meant the two fields held the same value on every legacy row; it
// must NOT be extended to rows that have a base_commit_sha, since last_commit_sha
// is now live-refreshed to the session's tip and would give a moving diff base.
func (b *Backlog) GetBaseCommitSHAsForSessions(ctx context.Context, sessionUUIDs []string) (map[string]string, error) {
	rows, err := b.client.ItemSession.Query().
		Where(itemsession.SessionUUIDIn(sessionUUIDs...)).
		Select(itemsession.FieldSessionUUID, itemsession.FieldBaseCommitSha, itemsession.FieldLastCommitSha).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("GetBaseCommitSHAsForSessions: %w", err)
	}
	result := make(map[string]string, len(rows))
	for _, row := range rows {
		switch {
		case row.BaseCommitSha != "":
			result[row.SessionUUID] = row.BaseCommitSha
		case row.LastCommitSha != "":
			result[row.SessionUUID] = row.LastCommitSha
		}
	}
	return result, nil
}

// GetItemSessionBySessionUUID looks up the most recent active ItemSession by session UUID alone.
// session_uuid is not unique across records (a session may be reused), so we order by
// created_at descending and take the first match. Returns ErrNotFound if no record exists.
// Loads the BacklogItem edge so BacklogItemID is populated in the returned summary.
func (b *Backlog) GetItemSessionBySessionUUID(ctx context.Context, sessionUUID string) (backlogdto.ItemSessionSummary, error) {
	is, err := b.client.ItemSession.Query().
		Where(itemsession.SessionUUID(sessionUUID)).
		WithBacklogItem().
		Order(ent.Desc(itemsession.FieldCreatedAt)).
		First(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return backlogdto.ItemSessionSummary{}, fmt.Errorf("%w: item session for session=%s", domain.ErrNotFound, sessionUUID)
		}
		return backlogdto.ItemSessionSummary{}, fmt.Errorf("failed to get item session by session uuid: %w", err)
	}
	return ItemSessionToSummary(is), nil
}

// ListItemSessionsBySessionUUIDs returns the most recent ItemSession for each
// of the given session UUIDs in a single query (UUIDs with no row are absent
// from the map). The ReviewVerdict edge is loaded; the BacklogItem edge is not.
func (b *Backlog) ListItemSessionsBySessionUUIDs(ctx context.Context, sessionUUIDs []string) (map[string]backlogdto.ItemSessionSummary, error) {
	if len(sessionUUIDs) == 0 {
		return map[string]backlogdto.ItemSessionSummary{}, nil
	}
	rows, err := b.client.ItemSession.Query().
		Where(itemsession.SessionUUIDIn(sessionUUIDs...)).
		WithReviewVerdict().
		Order(ent.Desc(itemsession.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list item sessions by session uuids: %w", err)
	}
	result := make(map[string]backlogdto.ItemSessionSummary, len(rows))
	for _, row := range rows {
		if _, seen := result[row.SessionUUID]; !seen { // newest first
			result[row.SessionUUID] = ItemSessionToSummary(row)
		}
	}
	return result, nil
}

// GetItemSessionBySessionAndItem looks up an ItemSession by both sessionUUID and backlog item ID.
func (b *Backlog) GetItemSessionBySessionAndItem(ctx context.Context, sessionUUID string, itemID string) (backlogdto.ItemSessionSummary, error) {
	parsedItemID, err := b.ResolveBacklogItemLookup(ctx, itemID)
	if err != nil {
		return backlogdto.ItemSessionSummary{}, fmt.Errorf("invalid item id %q: %w", itemID, err)
	}

	is, err := b.client.ItemSession.Query().
		Where(
			itemsession.SessionUUID(sessionUUID),
			itemsession.HasBacklogItemWith(backlogitem.ID(parsedItemID)),
		).
		Order(ent.Desc(itemsession.FieldCreatedAt)).
		First(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return backlogdto.ItemSessionSummary{}, fmt.Errorf("%w: item session for session=%s item=%s", domain.ErrNotFound, sessionUUID, itemID)
		}
		return backlogdto.ItemSessionSummary{}, fmt.Errorf("failed to get item session: %w", err)
	}
	summary := ItemSessionToSummary(is)
	summary.BacklogItemID = itemID
	return summary, nil
}

// GetMostRecentReviewVerdictForItem returns the OverallOutcome from the
// most recently created ReviewVerdict associated with any ItemSession for the
// given BacklogItem UUID. Returns "" (not an error) when no verdict exists yet.
func (b *Backlog) GetMostRecentReviewVerdictForItem(ctx context.Context, itemID string) (domain.ReviewOutcome, error) {
	parsedItemID, err := b.ResolveBacklogItemLookup(ctx, itemID)
	if err != nil {
		return "", fmt.Errorf("invalid item id %q: %w", itemID, err)
	}

	// Find the most recent ItemSession for this item that has a review verdict.
	is, err := b.client.ItemSession.Query().
		Where(
			itemsession.HasBacklogItemWith(backlogitem.ID(parsedItemID)),
			itemsession.HasReviewVerdict(),
		).
		WithReviewVerdict().
		Order(ent.Desc(itemsession.FieldCreatedAt)).
		First(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return "", nil
		}
		return "", fmt.Errorf("failed to query review verdict for item %s: %w", itemID, err)
	}

	if is.Edges.ReviewVerdict == nil {
		return "", nil
	}
	return domain.ReviewOutcome(is.Edges.ReviewVerdict.OverallOutcome), nil
}

// GetRecentReviewVerdictSummaries returns up to limit ReviewVerdicts for the
// given BacklogItem UUID, most recent first. Reuses the existing
// ReviewVerdictSummary DTO (see repository.go) — only OverallOutcome,
// Summary, and DiffHash are populated, since that's all callers
// (IsRepeatedFailure and IsFlakyVerdictFlipFlop in stuck/decisions.go) need.
func (b *Backlog) GetRecentReviewVerdictSummaries(ctx context.Context, itemID string, limit int) ([]backlogdto.ReviewVerdictSummary, error) {
	parsedItemID, err := b.ResolveBacklogItemLookup(ctx, itemID)
	if err != nil {
		return nil, fmt.Errorf("invalid item id %q: %w", itemID, err)
	}

	sessions, err := b.client.ItemSession.Query().
		Where(
			itemsession.HasBacklogItemWith(backlogitem.ID(parsedItemID)),
			itemsession.HasReviewVerdict(),
		).
		WithReviewVerdict().
		Order(ent.Desc(itemsession.FieldCreatedAt)).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query review verdicts for item %s: %w", itemID, err)
	}

	result := make([]backlogdto.ReviewVerdictSummary, 0, len(sessions))
	for _, is := range sessions {
		if is.Edges.ReviewVerdict == nil {
			continue
		}
		result = append(result, backlogdto.ReviewVerdictSummary{
			ID:                is.Edges.ReviewVerdict.ID.String(),
			OverallOutcome:    is.Edges.ReviewVerdict.OverallOutcome,
			PerCriterion:      is.Edges.ReviewVerdict.PerCriterion,
			Summary:           is.Edges.ReviewVerdict.Summary,
			DiffHash:          is.Edges.ReviewVerdict.DiffHash,
			DiffTruncated:     is.Edges.ReviewVerdict.DiffTruncated,
			ReviewedCommitSha: is.Edges.ReviewVerdict.ReviewedCommitSha,
			OverrideBy:        is.Edges.ReviewVerdict.OverrideBy,
		})
	}
	return result, nil
}

// ListOpenJulesItemSessions returns every not-yet-ended jules_work ItemSession
// across all backlog items, joined with enough of its parent item's metadata
// (via the ItemSessionBacklogEntry shape already returned by
// GetAllItemSessionsWithBacklogInfo) for the dispatcher/poller to act on it
// without a second query.
func (b *Backlog) ListOpenJulesItemSessions(ctx context.Context) ([]backlogdto.ItemSessionBacklogEntry, error) {
	sessions, err := b.client.ItemSession.Query().
		Where(
			itemsession.SessionRole(domain.SessionRoleJulesWork),
			itemsession.EndedAtIsNil(),
		).
		WithBacklogItem().
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list open jules item sessions: %w", err)
	}
	results := make([]backlogdto.ItemSessionBacklogEntry, 0, len(sessions))
	for _, is := range sessions {
		if is.Edges.BacklogItem == nil {
			continue
		}
		results = append(results, backlogdto.ItemSessionBacklogEntry{
			SessionUUID: is.SessionUUID,
			SessionRole: is.SessionRole,
			ItemID:      is.Edges.BacklogItem.ID.String(),
			ItemTitle:   is.Edges.BacklogItem.Title,
			ItemStatus:  is.Edges.BacklogItem.Status,
		})
	}
	return results, nil
}

// CountJulesItemSessionsSince counts jules_work ItemSessions created since since
// that reached a confirmed, billed Jules session — feeds MaxJulesSessionsPerDay
// (Story 2.2.2), the daily cloud-spend cap. Excludes rows still carrying the
// domain.JulesPendingUUIDPrefix reservation (never confirmed) and rows that ended with
// end_reason "dispatch_failed" (failed at CreateSession), so a bad key or a
// Jules-side outage doesn't consume the day's cap on attempts nothing was
// actually billed for (pre-mortem P2 #5).
func (b *Backlog) CountJulesItemSessionsSince(ctx context.Context, since time.Time) (int, error) {
	n, err := b.client.ItemSession.Query().
		Where(
			itemsession.SessionRole(domain.SessionRoleJulesWork),
			itemsession.CreatedAtGTE(since),
			itemsession.Not(itemsession.SessionUUIDHasPrefix(domain.JulesPendingUUIDPrefix)),
			itemsession.EndReasonNEQ(domain.JulesDispatchFailedEndReason),
		).
		Count(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to count jules item sessions since %s: %w", since, err)
	}
	return n, nil
}
