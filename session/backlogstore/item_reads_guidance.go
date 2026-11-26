package backlogstore

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/backlogactivitynote"
	"github.com/tstapler/stapler-squad/session/ent/backlogitem"
	"github.com/tstapler/stapler-squad/session/ent/backlogprogressnote"
	"github.com/tstapler/stapler-squad/session/ent/backlogstatusevent"
	"github.com/tstapler/stapler-squad/session/ent/itemsession"
)

// GetBacklogItem retrieves a backlog item by either its legacy UUID string
// or its bl_-prefixed public id (see ResolveBacklogItemLookup).
func (b *Backlog) GetBacklogItem(ctx context.Context, id string) (*backlogdto.BacklogItemData, error) {
	parsedID, err := b.ResolveBacklogItemLookup(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid id %q: %v", domain.ErrNotFound, id, err)
	}

	item, err := b.client.BacklogItem.Query().
		Where(backlogitem.ID(parsedID)).
		WithSource().
		WithStatusEvents(func(q *ent.BacklogStatusEventQuery) {
			q.Order(ent.Asc(backlogstatusevent.FieldCreatedAt))
		}).
		WithProgressNotes(func(q *ent.BacklogProgressNoteQuery) {
			q.Order(ent.Asc(backlogprogressnote.FieldCreatedAt))
		}).
		WithActivityNotes(func(q *ent.BacklogActivityNoteQuery) {
			q.Order(ent.Asc(backlogactivitynote.FieldCreatedAt))
		}).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf("%w: backlog item %s", domain.ErrNotFound, id)
		}
		return nil, fmt.Errorf("failed to get backlog item %s: %w", id, err)
	}
	result := BacklogItemToData(item)
	b.guidance.AttachPendingGuidance(ctx, &result)
	return &result, nil
}

// excludedTerminalStatuses returns the statuses filter.ExcludeDone/ExcludeArchived
// ask to exclude from a default (no explicit Statuses) query, as a slice for
// StatusNotIn. The two flags are independent — either, both, or neither may be
// set — see BacklogItemFilter's doc comments. Reviewed for Epic 2.1, Story
// 2.1.3: deliberately built-in-only, unlike IsTerminalStatus — these flags
// name the two built-in statuses specifically (a caller opting out of "done"
// items, say), not "whatever is terminal", so a custom terminal stage is not
// silently swept in here.
func excludedTerminalStatuses(filter backlogdto.BacklogItemFilter) []string {
	var excluded []string
	if filter.ExcludeDone {
		excluded = append(excluded, string(domain.BacklogStatusDone))
	}
	if filter.ExcludeArchived {
		excluded = append(excluded, string(domain.BacklogStatusArchived))
	}
	return excluded
}

// ListBacklogItems returns backlog items with optional filtering.
func (b *Backlog) ListBacklogItems(ctx context.Context, filter backlogdto.BacklogItemFilter) ([]backlogdto.BacklogItemData, error) {
	q := b.client.BacklogItem.Query()

	if len(filter.Statuses) > 0 {
		q = q.Where(backlogitem.StatusIn(filter.Statuses...))
	} else if excluded := excludedTerminalStatuses(filter); len(excluded) > 0 {
		q = q.Where(backlogitem.StatusNotIn(excluded...))
	}

	q = applySourcePluginFilter(q, filter.SourcePluginIDs)
	if len(filter.Priorities) > 0 {
		q = q.Where(backlogitem.PriorityIn(filter.Priorities...))
	}

	if filter.ChainFired != nil {
		q = q.Where(backlogitem.ChainFiredEQ(*filter.ChainFired))
	}
	if filter.NextWorkflowIDSet != nil {
		if *filter.NextWorkflowIDSet {
			q = q.Where(backlogitem.NextWorkflowIDNotNil())
		} else {
			q = q.Where(backlogitem.NextWorkflowIDIsNil())
		}
	}

	switch filter.SortBy {
	case "priority":
		q = q.Order(ent.Asc(backlogitem.FieldPriority), ent.Desc(backlogitem.FieldUpdatedAt))
	case "updated_at":
		q = q.Order(ent.Desc(backlogitem.FieldUpdatedAt))
	default:
		q = q.Order(ent.Asc(backlogitem.FieldPriority), ent.Desc(backlogitem.FieldUpdatedAt))
	}

	// Apply safety cap: use caller-supplied limit when set, otherwise default to 1000.
	const defaultSafetyLimit = 1000
	limit := filter.Limit
	if limit <= 0 {
		limit = defaultSafetyLimit
	}
	q = q.Limit(limit)
	if filter.Offset > 0 {
		q = q.Offset(filter.Offset)
	}

	items, err := q.WithSource().All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list backlog items: %w", err)
	}

	result := make([]backlogdto.BacklogItemData, len(items))
	idIndex := make(map[string]int, len(items))
	parsedUUIDs := make([]uuid.UUID, 0, len(items))
	for i, item := range items {
		result[i] = BacklogItemToData(item)
		idIndex[item.ID.String()] = i
		parsedUUIDs = append(parsedUUIDs, item.ID)
	}
	if len(parsedUUIDs) == 0 {
		return result, nil
	}
	b.guidance.ApplyPendingGuidanceToData(ctx, result)

	// Eager-load item sessions via a single batched query, matching
	// ListBacklogItemSummaries's pattern, so WatchBacklogItems' fresh-connection
	// snapshot branch never publishes items with dropped ItemSessions.
	sessions, err := b.client.ItemSession.Query().
		Where(itemsession.HasBacklogItemWith(backlogitem.IDIn(parsedUUIDs...))).
		WithBacklogItem().
		WithReviewVerdict().
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list item sessions for backlog items: %w", err)
	}
	for _, s := range sessions {
		if s.Edges.BacklogItem == nil {
			continue
		}
		itemID := s.Edges.BacklogItem.ID.String()
		idx, ok := idIndex[itemID]
		if !ok {
			continue
		}
		result[idx].ItemSessions = append(result[idx].ItemSessions, ItemSessionToSummary(s))
	}
	return result, nil
}

// ListBacklogItemSummaries returns lightweight BacklogItemSummary values for list views.
// Three-phase: (1) scalar fields via .All(), (2) item sessions + review verdicts via edge loading.
func (b *Backlog) ListBacklogItemSummaries(ctx context.Context, filter backlogdto.BacklogItemFilter) ([]backlogdto.BacklogItemSummary, error) {
	q := b.client.BacklogItem.Query()

	if len(filter.Statuses) > 0 {
		q = q.Where(backlogitem.StatusIn(filter.Statuses...))
	} else if excluded := excludedTerminalStatuses(filter); len(excluded) > 0 {
		q = q.Where(backlogitem.StatusNotIn(excluded...))
	}
	q = applySourcePluginFilter(q, filter.SourcePluginIDs)
	if len(filter.Priorities) > 0 {
		q = q.Where(backlogitem.PriorityIn(filter.Priorities...))
	}
	if filter.ChainFired != nil {
		q = q.Where(backlogitem.ChainFiredEQ(*filter.ChainFired))
	}
	if filter.NextWorkflowIDSet != nil {
		if *filter.NextWorkflowIDSet {
			q = q.Where(backlogitem.NextWorkflowIDNotNil())
		} else {
			q = q.Where(backlogitem.NextWorkflowIDIsNil())
		}
	}
	switch filter.SortBy {
	case "priority":
		q = q.Order(ent.Asc(backlogitem.FieldPriority), ent.Desc(backlogitem.FieldUpdatedAt))
	case "updated_at":
		q = q.Order(ent.Desc(backlogitem.FieldUpdatedAt))
	default:
		q = q.Order(ent.Asc(backlogitem.FieldPriority), ent.Desc(backlogitem.FieldUpdatedAt))
	}

	const defaultSafetyLimit = 1000
	limit := filter.Limit
	if limit <= 0 {
		limit = defaultSafetyLimit
	}
	q = q.Limit(limit)
	if filter.Offset > 0 {
		q = q.Offset(filter.Offset)
	}

	// Phase 1: load scalar fields; avoid Select/Scan to sidestep nullable-time pitfalls.
	items, err := q.WithSource().All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list backlog item summaries: %w", err)
	}
	summaries := make([]backlogdto.BacklogItemSummary, len(items))
	idIndex := make(map[string]int, len(items))
	parsedUUIDs := make([]uuid.UUID, 0, len(items))
	for i, item := range items {
		summaries[i] = backlogdto.BacklogItemSummary{
			ID:                  item.ID.String(),
			PublicIDRaw:         item.PublicID,
			ExternalID:          item.ExternalID,
			ExternalURL:         item.ExternalURL,
			SourcePluginID:      sourcePluginIDOf(item),
			Labels:              item.Labels,
			Title:               item.Title,
			Status:              domain.BacklogStatus(item.Status),
			Priority:            item.Priority,
			RepoPath:            item.RepoPath,
			AcceptanceCriteria:  domain.AcCriteriaJSON(item.AcceptanceCriteria),
			Notes:               item.Notes,
			PrURL:               item.PrURL,
			PrNumber:            item.PrNumber,
			CreatedAt:           item.CreatedAt,
			UpdatedAt:           item.UpdatedAt,
			ArchivedAt:          item.ArchivedAt,
			SkipPlanning:        item.SkipPlanning,
			PlanApproved:        item.PlanApproved,
			PlanArtifactsPath:   item.PlanArtifactsPath,
			PlanRejectionReason: item.PlanRejectionReason,
			IsIntegrationChild:  item.IsIntegrationChild,
			IntegrationChildID:  uuidPtrString(item.IntegrationChildID),
		}
		idIndex[item.ID.String()] = i
		parsedUUIDs = append(parsedUUIDs, item.ID)
	}
	if len(parsedUUIDs) == 0 {
		return summaries, nil
	}
	b.guidance.ApplyPendingGuidanceToSummaries(ctx, summaries)

	// Phase 2+3: load item sessions with review verdicts via edge loading.
	sessions, err := b.client.ItemSession.Query().
		Where(itemsession.HasBacklogItemWith(backlogitem.IDIn(parsedUUIDs...))).
		WithBacklogItem().
		WithReviewVerdict().
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list item sessions for summaries: %w", err)
	}
	for _, s := range sessions {
		if s.Edges.BacklogItem == nil {
			continue
		}
		itemID := s.Edges.BacklogItem.ID.String()
		idx, ok := idIndex[itemID]
		if !ok {
			continue
		}
		summaries[idx].ItemSessions = append(summaries[idx].ItemSessions, ItemSessionToSummary(s))
	}
	return summaries, nil
}
