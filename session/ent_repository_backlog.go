package session

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/backlogitem"
	"github.com/tstapler/stapler-squad/session/ent/itemsource"
)

// --- converters ---

func backlogItemToData(item *ent.BacklogItem) BacklogItemData {
	data := BacklogItemData{
		ID:                 item.ID.String(),
		Title:              item.Title,
		Description:        item.Description,
		AcceptanceCriteria: item.AcceptanceCriteria,
		Priority:           item.Priority,
		Status:             item.Status,
		RepoPath:           item.RepoPath,
		SkipReviewGate:     item.SkipReviewGate,
		SkipPlanning:       item.SkipPlanning,
		PlanApproved:       item.PlanApproved,
		PlanApprovedAt:     item.PlanApprovedAt,
		PlanArtifactsPath:  item.PlanArtifactsPath,
		Notes:              item.Notes,
		ExternalID:         item.ExternalID,
		ArchivedAt:         item.ArchivedAt,
		CreatedAt:          item.CreatedAt,
		UpdatedAt:          item.UpdatedAt,
	}
	// Resolve source ID from the eager-loaded edge when available.
	if item.Edges.Source != nil {
		data.SourceID = item.Edges.Source.ID.String()
	}
	return data
}

func itemSourceToData(src *ent.ItemSource) ItemSourceData {
	data := ItemSourceData{
		ID:           src.ID.String(),
		PluginID:     src.PluginID,
		DisplayName:  src.DisplayName,
		Config:       src.Config,
		Enabled:      src.Enabled,
		LastSyncedAt: src.LastSyncedAt,
		CreatedAt:    src.CreatedAt,
		UpdatedAt:    src.UpdatedAt,
	}
	// TokenConfigured: true when the config JSON contains a non-empty "token" key.
	data.TokenConfigured = src.Config != "" && strings.Contains(src.Config, `"token"`)
	return data
}

// --- BacklogItem CRUD ---

// CreateBacklogItem inserts a new backlog item.
func (r *EntRepository) CreateBacklogItem(ctx context.Context, data BacklogItemData) (*BacklogItemData, error) {
	priority := data.Priority
	if priority == 0 {
		priority = 3
	}
	status := data.Status
	if status == "" {
		status = string(BacklogStatusIdea)
	}

	c := r.client.BacklogItem.Create().
		SetTitle(data.Title).
		SetNillableDescription(&data.Description).
		SetNillableAcceptanceCriteria(&data.AcceptanceCriteria).
		SetPriority(priority).
		SetStatus(status).
		SetNillableRepoPath(&data.RepoPath).
		SetSkipReviewGate(data.SkipReviewGate).
		SetSkipPlanning(data.SkipPlanning).
		SetPlanApproved(data.PlanApproved).
		SetNillablePlanApprovedAt(data.PlanApprovedAt).
		SetNillablePlanArtifactsPath(&data.PlanArtifactsPath).
		SetNillableNotes(&data.Notes).
		SetNillableExternalID(&data.ExternalID).
		SetNillableArchivedAt(data.ArchivedAt)

	if data.SourceID != "" {
		sourceUUID, parseErr := uuid.Parse(data.SourceID)
		if parseErr == nil {
			c.SetSourceID(sourceUUID)
		}
	}

	item, err := c.Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create backlog item: %w", err)
	}
	result := backlogItemToData(item)
	return &result, nil
}

// GetBacklogItem retrieves a backlog item by UUID string.
func (r *EntRepository) GetBacklogItem(ctx context.Context, id string) (*BacklogItemData, error) {
	parsedID, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid id %q: %v", ErrNotFound, id, err)
	}

	item, err := r.client.BacklogItem.Query().
		Where(backlogitem.ID(parsedID)).
		WithSource().
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf("%w: backlog item %s", ErrNotFound, id)
		}
		return nil, fmt.Errorf("failed to get backlog item %s: %w", id, err)
	}
	result := backlogItemToData(item)
	return &result, nil
}

// ListBacklogItems returns backlog items with optional filtering.
func (r *EntRepository) ListBacklogItems(ctx context.Context, filter BacklogItemFilter) ([]BacklogItemData, error) {
	q := r.client.BacklogItem.Query()

	if len(filter.Statuses) > 0 {
		q = q.Where(backlogitem.StatusIn(filter.Statuses...))
	} else if filter.ExcludeTerminal {
		q = q.Where(backlogitem.StatusNotIn(
			string(BacklogStatusDone),
			string(BacklogStatusArchived),
		))
	}

	if len(filter.Priorities) > 0 {
		q = q.Where(backlogitem.PriorityIn(filter.Priorities...))
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

	result := make([]BacklogItemData, len(items))
	for i, item := range items {
		result[i] = backlogItemToData(item)
	}
	return result, nil
}

// UpdateBacklogItem modifies an existing backlog item with optional precondition check.
func (r *EntRepository) UpdateBacklogItem(ctx context.Context, id string, update BacklogItemUpdate, precondition *BacklogItemPrecondition) (*BacklogItemData, error) {
	parsedID, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid id %q: %v", ErrNotFound, id, err)
	}

	// Fetch current item for precondition check.
	current, err := r.client.BacklogItem.Get(ctx, parsedID)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf("%w: backlog item %s", ErrNotFound, id)
		}
		return nil, fmt.Errorf("failed to get backlog item %s: %w", id, err)
	}

	if precondition != nil {
		if precondition.ExpectedStatus != "" && current.Status != precondition.ExpectedStatus {
			return nil, fmt.Errorf("%w: expected status %q, got %q", ErrPreconditionFailed, precondition.ExpectedStatus, current.Status)
		}
		if precondition.ExpectedUpdatedAt != nil && !current.UpdatedAt.Equal(*precondition.ExpectedUpdatedAt) {
			return nil, fmt.Errorf("%w: updated_at mismatch", ErrPreconditionFailed)
		}
	}

	u := r.client.BacklogItem.UpdateOneID(parsedID)
	if update.Title != nil {
		u.SetTitle(*update.Title)
	}
	if update.Description != nil {
		u.SetDescription(*update.Description)
	}
	if update.AcceptanceCriteria != nil {
		u.SetAcceptanceCriteria(*update.AcceptanceCriteria)
	}
	if update.Priority != nil {
		u.SetPriority(*update.Priority)
	}
	if update.RepoPath != nil {
		u.SetRepoPath(*update.RepoPath)
	}
	if update.SkipReviewGate != nil {
		u.SetSkipReviewGate(*update.SkipReviewGate)
	}
	if update.SkipPlanning != nil {
		u.SetSkipPlanning(*update.SkipPlanning)
	}
	if update.Notes != nil {
		u.SetNotes(*update.Notes)
	}
	if update.PlanApproved != nil {
		u.SetPlanApproved(*update.PlanApproved)
	}
	if update.PlanApprovedAt != nil {
		u.SetPlanApprovedAt(*update.PlanApprovedAt)
	}
	if update.PlanArtifactsPath != nil {
		u.SetPlanArtifactsPath(*update.PlanArtifactsPath)
	}

	item, err := u.Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to update backlog item %s: %w", id, err)
	}
	result := backlogItemToData(item)
	return &result, nil
}

// ArchiveBacklogItem sets the archived_at timestamp on a backlog item.
func (r *EntRepository) ArchiveBacklogItem(ctx context.Context, id string) (*BacklogItemData, error) {
	parsedID, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid id %q: %v", ErrNotFound, id, err)
	}

	now := time.Now()
	item, err := r.client.BacklogItem.UpdateOneID(parsedID).
		SetArchivedAt(now).
		SetStatus(string(BacklogStatusArchived)).
		SetUserModifiedStatusAt(now).
		Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf("%w: backlog item %s", ErrNotFound, id)
		}
		return nil, fmt.Errorf("failed to archive backlog item %s: %w", id, err)
	}
	result := backlogItemToData(item)
	return &result, nil
}

// TransitionBacklogItemStatus changes the status of a backlog item with optional precondition.
func (r *EntRepository) TransitionBacklogItemStatus(ctx context.Context, id string, toStatus BacklogStatus, precondition *BacklogItemPrecondition) (*BacklogItemData, error) {
	parsedID, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid id %q: %v", ErrNotFound, id, err)
	}

	current, err := r.client.BacklogItem.Get(ctx, parsedID)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf("%w: backlog item %s", ErrNotFound, id)
		}
		return nil, fmt.Errorf("failed to get backlog item %s: %w", id, err)
	}

	if precondition != nil {
		if precondition.ExpectedStatus != "" && current.Status != precondition.ExpectedStatus {
			return nil, fmt.Errorf("%w: expected status %q, got %q", ErrPreconditionFailed, precondition.ExpectedStatus, current.Status)
		}
		if precondition.ExpectedUpdatedAt != nil && !current.UpdatedAt.Equal(*precondition.ExpectedUpdatedAt) {
			return nil, fmt.Errorf("%w: updated_at mismatch", ErrPreconditionFailed)
		}
	}

	now := time.Now()
	item, err := r.client.BacklogItem.UpdateOneID(parsedID).
		SetStatus(string(toStatus)).
		SetUserModifiedStatusAt(now).
		Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf("%w: backlog item %s", ErrNotFound, id)
		}
		return nil, fmt.Errorf("failed to transition backlog item %s status: %w", id, err)
	}
	result := backlogItemToData(item)
	return &result, nil
}

// --- ItemSource CRUD ---

// CreateItemSource registers a new external item source.
func (r *EntRepository) CreateItemSource(ctx context.Context, data ItemSourceData) (*ItemSourceData, error) {
	src, err := r.client.ItemSource.Create().
		SetPluginID(data.PluginID).
		SetDisplayName(data.DisplayName).
		SetNillableConfig(&data.Config).
		SetEnabled(data.Enabled).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create item source: %w", err)
	}
	result := itemSourceToData(src)
	return &result, nil
}

// ListItemSources returns all registered item sources.
func (r *EntRepository) ListItemSources(ctx context.Context) ([]ItemSourceData, error) {
	sources, err := r.client.ItemSource.Query().All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list item sources: %w", err)
	}
	result := make([]ItemSourceData, len(sources))
	for i, src := range sources {
		result[i] = itemSourceToData(src)
	}
	return result, nil
}

// UpdateItemSource modifies an existing item source.
func (r *EntRepository) UpdateItemSource(ctx context.Context, id string, update ItemSourceUpdate) (*ItemSourceData, error) {
	parsedID, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid id %q: %v", ErrNotFound, id, err)
	}

	u := r.client.ItemSource.UpdateOneID(parsedID)
	if update.DisplayName != nil {
		u.SetDisplayName(*update.DisplayName)
	}
	if update.Enabled != nil {
		u.SetEnabled(*update.Enabled)
	}
	if update.Config != nil {
		u.SetConfig(*update.Config)
	}

	src, err := u.Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf("%w: item source %s", ErrNotFound, id)
		}
		return nil, fmt.Errorf("failed to update item source %s: %w", id, err)
	}
	result := itemSourceToData(src)
	return &result, nil
}

// DeleteItemSource removes an item source by UUID string.
func (r *EntRepository) DeleteItemSource(ctx context.Context, id string) error {
	parsedID, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("%w: invalid id %q: %v", ErrNotFound, id, err)
	}

	err = r.client.ItemSource.DeleteOneID(parsedID).Exec(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return fmt.Errorf("%w: item source %s", ErrNotFound, id)
		}
		return fmt.Errorf("failed to delete item source %s: %w", id, err)
	}
	return nil
}

// --- Sync helpers ---

// GetItemSourceByID retrieves a raw *ent.ItemSource by UUID string.
func (r *EntRepository) GetItemSourceByID(ctx context.Context, id string) (*ent.ItemSource, error) {
	parsedID, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid id %q: %v", ErrNotFound, id, err)
	}
	src, err := r.client.ItemSource.Get(ctx, parsedID)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf("%w: item source %s", ErrNotFound, id)
		}
		return nil, fmt.Errorf("failed to get item source %s: %w", id, err)
	}
	return src, nil
}

// GetBacklogItemByExternalID retrieves a BacklogItem by its external_id, or nil if not found.
func (r *EntRepository) GetBacklogItemByExternalID(ctx context.Context, externalID string) (*ent.BacklogItem, error) {
	item, err := r.client.BacklogItem.Query().
		Where(backlogitem.ExternalID(externalID)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to query backlog item by external_id %q: %w", externalID, err)
	}
	return item, nil
}

// UpdateItemSourceSync updates the sync_cursor and last_synced_at on an ItemSource.
func (r *EntRepository) UpdateItemSourceSync(ctx context.Context, id string, cursor string, syncedAt time.Time) error {
	parsedID, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("invalid id %q: %w", id, err)
	}
	u := r.client.ItemSource.UpdateOneID(parsedID).SetLastSyncedAt(syncedAt)
	if cursor != "" {
		u.SetSyncCursor(cursor)
	}
	_, err = u.Save(ctx)
	if err != nil {
		return fmt.Errorf("failed to update sync state for item source %s: %w", id, err)
	}
	return nil
}

// CreateSourceSyncEvent records a completed sync run for an ItemSource.
func (r *EntRepository) CreateSourceSyncEvent(ctx context.Context, sourceID string, cursorAfter string, created, updated, skipped int, finishedAt time.Time) error {
	parsedID, err := uuid.Parse(sourceID)
	if err != nil {
		return fmt.Errorf("invalid source id %q: %w", sourceID, err)
	}

	// Verify the source exists to satisfy the Required edge constraint.
	if _, err := r.client.ItemSource.Query().
		Where(itemsource.ID(parsedID)).
		Only(ctx); err != nil {
		if ent.IsNotFound(err) {
			return fmt.Errorf("%w: item source %s", ErrNotFound, sourceID)
		}
		return fmt.Errorf("failed to verify item source %s: %w", sourceID, err)
	}

	c := r.client.SourceSyncEvent.Create().
		SetItemsCreated(created).
		SetItemsUpdated(updated).
		SetItemsSkipped(skipped).
		SetFinishedAt(finishedAt).
		SetSourceID(parsedID)
	if cursorAfter != "" {
		c.SetCursorAfter(cursorAfter)
	}
	_, err = c.Save(ctx)
	if err != nil {
		return fmt.Errorf("failed to create source sync event for source %s: %w", sourceID, err)
	}
	return nil
}
