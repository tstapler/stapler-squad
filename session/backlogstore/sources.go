package backlogstore

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/backlogitem"
	"github.com/tstapler/stapler-squad/session/ent/itemsource"
	"github.com/tstapler/stapler-squad/session/ent/sourcesyncevent"
)

// --- ItemSource CRUD ---

// CreateItemSource registers a new external item source.
func (b *Backlog) CreateItemSource(ctx context.Context, data backlogdto.ItemSourceData) (*backlogdto.ItemSourceData, error) {
	src, err := b.client.ItemSource.Create().
		SetPluginID(data.PluginID).
		SetDisplayName(data.DisplayName).
		SetNillableConfig(&data.Config).
		SetEnabled(data.Enabled).
		SetForwardSyncEnabled(data.ForwardSyncEnabled).
		SetBackwardSyncEnabled(data.BackwardSyncEnabled).
		SetNillableForwardSyncCloseLabel(&data.ForwardSyncCloseLabel).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create item source: %w", err)
	}
	result := ItemSourceToData(src)
	return &result, nil
}

// ListItemSources returns all registered item sources.
func (b *Backlog) ListItemSources(ctx context.Context) ([]backlogdto.ItemSourceData, error) {
	//nolint:entfullscan small, bounded-by-nature table (one row per registered source), intentionally returns all.
	sources, err := b.client.ItemSource.Query().All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list item sources: %w", err)
	}
	result := make([]backlogdto.ItemSourceData, len(sources))
	for i, src := range sources {
		result[i] = ItemSourceToData(src)
	}
	return result, nil
}

// UpdateItemSource modifies an existing item source.
func (b *Backlog) UpdateItemSource(ctx context.Context, id string, update backlogdto.ItemSourceUpdate) (*backlogdto.ItemSourceData, error) {
	parsedID, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid id %q: %v", domain.ErrNotFound, id, err)
	}

	u := b.client.ItemSource.UpdateOneID(parsedID)
	if update.DisplayName != nil {
		u.SetDisplayName(*update.DisplayName)
	}
	if update.Enabled != nil {
		u.SetEnabled(*update.Enabled)
	}
	if update.ForwardSyncEnabled != nil {
		u.SetForwardSyncEnabled(*update.ForwardSyncEnabled)
	}
	if update.BackwardSyncEnabled != nil {
		u.SetBackwardSyncEnabled(*update.BackwardSyncEnabled)
	}
	if update.ForwardSyncCloseLabel != nil {
		u.SetForwardSyncCloseLabel(*update.ForwardSyncCloseLabel)
	}
	if update.Config != nil {
		u.SetConfig(*update.Config)
	}

	src, err := u.Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf("%w: item source %s", domain.ErrNotFound, id)
		}
		return nil, fmt.Errorf("failed to update item source %s: %w", id, err)
	}
	result := ItemSourceToData(src)
	return &result, nil
}

// DeleteItemSource removes an item source by UUID string.
func (b *Backlog) DeleteItemSource(ctx context.Context, id string) error {
	parsedID, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("%w: invalid id %q: %v", domain.ErrNotFound, id, err)
	}

	err = b.client.ItemSource.DeleteOneID(parsedID).Exec(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return fmt.Errorf("%w: item source %s", domain.ErrNotFound, id)
		}
		return fmt.Errorf("failed to delete item source %s: %w", id, err)
	}
	return nil
}

// --- Sync helpers ---

// GetItemSourceByID retrieves a raw *ent.ItemSource by UUID string.
func (b *Backlog) GetItemSourceByID(ctx context.Context, id string) (*ent.ItemSource, error) {
	parsedID, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid id %q: %v", domain.ErrNotFound, id, err)
	}
	src, err := b.client.ItemSource.Get(ctx, parsedID)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf("%w: item source %s", domain.ErrNotFound, id)
		}
		return nil, fmt.Errorf("failed to get item source %s: %w", id, err)
	}
	return src, nil
}

// GetBacklogItemByExternalID retrieves a BacklogItem by its external_id, scoped
// to sourceID. External IDs (e.g. GitHub issue/PR numbers) are only unique
// within their source, not globally — two different repos can both have an
// issue #1, so this must never match across sources.
func (b *Backlog) GetBacklogItemByExternalID(ctx context.Context, sourceID, externalID string) (*ent.BacklogItem, error) {
	parsedSourceID, err := uuid.Parse(sourceID)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid source id %q: %v", domain.ErrNotFound, sourceID, err)
	}
	item, err := b.client.BacklogItem.Query().
		Where(backlogitem.ExternalID(externalID), backlogitem.HasSourceWith(itemsource.ID(parsedSourceID))).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("failed to query backlog item by external_id %q: %w", externalID, err)
	}
	return item, nil
}

// GetBacklogItemsByExternalIDs batches GetBacklogItemByExternalID's lookup
// across many external IDs at once (a single IN query rather than one query
// per ID), scoped to sourceID the same way. Used by
// SyncLoop.PreviewBackwardSyncImpact, which previously issued one
// GetBacklogItemByExternalID call per closed issue in a loop — an N+1 query
// pattern against an index that isn't composite with the source FK. Returns
// a map keyed by external_id; IDs with no matching local item are simply
// absent from the map (not an error), matching the "not locally-imported,
// exclude it" semantics the per-item lookup had.
func (b *Backlog) GetBacklogItemsByExternalIDs(ctx context.Context, sourceID string, externalIDs []string) (map[string]*ent.BacklogItem, error) {
	result := make(map[string]*ent.BacklogItem, len(externalIDs))
	if len(externalIDs) == 0 {
		return result, nil
	}

	parsedSourceID, err := uuid.Parse(sourceID)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid source id %q: %v", domain.ErrNotFound, sourceID, err)
	}

	items, err := b.client.BacklogItem.Query().
		Where(backlogitem.ExternalIDIn(externalIDs...), backlogitem.HasSourceWith(itemsource.ID(parsedSourceID))).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to batch query backlog items by external_ids: %w", err)
	}

	for _, item := range items {
		result[item.ExternalID] = item
	}
	return result, nil
}

// MaxSourceSyncEventsHistory caps how many sync history rows a single
// GetSyncHistory call returns, so a long-lived, frequently-synced source
// doesn't grow into an unbounded response.
const MaxSourceSyncEventsHistory = 200

// ListSourceSyncEvents returns sync history events for an item source, most recent
// first, capped at MaxSourceSyncEventsHistory rows. truncated is true when older
// events exist beyond the cap — callers should surface this to avoid silently
// hiding history for sources with long or frequent sync runs.
func (b *Backlog) ListSourceSyncEvents(ctx context.Context, sourceID string) ([]backlogdto.SourceSyncEventData, bool, error) {
	parsedID, err := uuid.Parse(sourceID)
	if err != nil {
		return nil, false, fmt.Errorf("invalid source id %q: %w", sourceID, err)
	}
	// Fetch one extra row to distinguish "exactly at the cap" from "more exist beyond it".
	rawEvents, err := b.client.SourceSyncEvent.Query().
		Where(sourcesyncevent.HasSourceWith(itemsource.ID(parsedID))).
		Order(ent.Desc(sourcesyncevent.FieldStartedAt)).
		Limit(MaxSourceSyncEventsHistory + 1).
		All(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("failed to list sync events for source %s: %w", sourceID, err)
	}
	truncated := len(rawEvents) > MaxSourceSyncEventsHistory
	if truncated {
		rawEvents = rawEvents[:MaxSourceSyncEventsHistory]
	}
	events := make([]backlogdto.SourceSyncEventData, len(rawEvents))
	for i, ev := range rawEvents {
		events[i] = sourceSyncEventToData(ev)
	}
	return events, truncated, nil
}

// CreateSourceSyncEvent records a completed (or failed) sync run for an
// ItemSource. errMsg should be non-empty only when the sync run failed
// outright (e.g. the plugin's Fetch call errored); errored counts per-item
// failures within an otherwise-successful fetch.
func (b *Backlog) CreateSourceSyncEvent(ctx context.Context, sourceID string, cursorAfter string, created, updated, skipped, errored int, errMsg string, startedAt, finishedAt time.Time) error {
	parsedID, err := uuid.Parse(sourceID)
	if err != nil {
		return fmt.Errorf("invalid source id %q: %w", sourceID, err)
	}

	// Verify the source exists to satisfy the Required edge constraint.
	if _, err := b.client.ItemSource.Query().
		Where(itemsource.ID(parsedID)).
		Only(ctx); err != nil {
		if ent.IsNotFound(err) {
			return fmt.Errorf("%w: item source %s", domain.ErrNotFound, sourceID)
		}
		return fmt.Errorf("failed to verify item source %s: %w", sourceID, err)
	}

	c := b.client.SourceSyncEvent.Create().
		SetItemsCreated(created).
		SetItemsUpdated(updated).
		SetItemsSkipped(skipped).
		SetItemsErrored(errored).
		SetStartedAt(startedAt).
		SetFinishedAt(finishedAt).
		SetSourceID(parsedID)
	if errMsg != "" {
		c.SetErrorMessage(errMsg)
	}
	if cursorAfter != "" {
		c.SetCursorAfter(cursorAfter)
	}
	_, err = c.Save(ctx)
	if err != nil {
		return fmt.Errorf("failed to create source sync event for source %s: %w", sourceID, err)
	}
	return nil
}

// RecordSourceSyncFailure persists a zero-item sync-history row recording a
// forward-sync failure (e.g. the forward-sync EventBus subscriber's CloseIssue
// call erroring — see server/services/backlog_github_forward_sync.go), so the
// failure is queryable via ListSourceSyncEvents / the Settings UI's row-level
// warning (Story 4.3.2) instead of only appearing in server logs. Mirrors
// CreateSourceSyncEvent's error-message convention used by SyncOne's own
// fetch-failure path, but for the write direction (forward sync) rather than
// the read direction (Fetch).
func (b *Backlog) RecordSourceSyncFailure(ctx context.Context, sourceID string, message string) error {
	now := time.Now()
	return b.CreateSourceSyncEvent(ctx, sourceID, "", 0, 0, 0, 1, message, now, now)
}

// FinishSourceSync atomically advances an ItemSource's sync cursor/last_synced_at
// and records the SourceSyncEvent for a successful sync run. Wrapping both
// writes in one transaction prevents a crash between them from leaving the
// cursor advanced with no corresponding history row — which would silently
// hide the fact that a batch of items was processed (or dropped) in that run.
func (b *Backlog) FinishSourceSync(ctx context.Context, sourceID string, cursorAfter string, created, updated, skipped, errored int, startedAt, finishedAt time.Time) error {
	parsedID, err := uuid.Parse(sourceID)
	if err != nil {
		return fmt.Errorf("invalid source id %q: %w", sourceID, err)
	}

	tx, err := b.client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	u := tx.ItemSource.UpdateOneID(parsedID).SetLastSyncedAt(finishedAt)
	if cursorAfter != "" {
		u.SetSyncCursor(cursorAfter)
	}
	if _, err := u.Save(ctx); err != nil {
		if ent.IsNotFound(err) {
			return fmt.Errorf("%w: item source %s", domain.ErrNotFound, sourceID)
		}
		return fmt.Errorf("failed to update sync state for item source %s: %w", sourceID, err)
	}

	c := tx.SourceSyncEvent.Create().
		SetItemsCreated(created).
		SetItemsUpdated(updated).
		SetItemsSkipped(skipped).
		SetItemsErrored(errored).
		SetStartedAt(startedAt).
		SetFinishedAt(finishedAt).
		SetSourceID(parsedID)
	if cursorAfter != "" {
		c.SetCursorAfter(cursorAfter)
	}
	if _, err := c.Save(ctx); err != nil {
		return fmt.Errorf("failed to create source sync event for source %s: %w", sourceID, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit sync finish transaction: %w", err)
	}
	return nil
}

// sourceSyncEventToData maps an *ent.SourceSyncEvent to a SourceSyncEventData DTO.
func sourceSyncEventToData(e *ent.SourceSyncEvent) backlogdto.SourceSyncEventData {
	return backlogdto.SourceSyncEventData{
		ID:           e.ID.String(),
		ItemsCreated: e.ItemsCreated,
		ItemsUpdated: e.ItemsUpdated,
		ItemsSkipped: e.ItemsSkipped,
		ItemsErrored: e.ItemsErrored,
		ErrorMessage: e.ErrorMessage,
		CursorAfter:  e.CursorAfter,
		StartedAt:    e.StartedAt,
		FinishedAt:   e.FinishedAt,
	}
}

func ItemSourceToData(src *ent.ItemSource) backlogdto.ItemSourceData {
	data := backlogdto.ItemSourceData{
		ID:                    src.ID.String(),
		PluginID:              src.PluginID,
		DisplayName:           src.DisplayName,
		Config:                src.Config,
		Enabled:               src.Enabled,
		ForwardSyncEnabled:    src.ForwardSyncEnabled,
		BackwardSyncEnabled:   src.BackwardSyncEnabled,
		ForwardSyncCloseLabel: src.ForwardSyncCloseLabel,
		LastSyncedAt:          src.LastSyncedAt,
		CreatedAt:             src.CreatedAt,
		UpdatedAt:             src.UpdatedAt,
	}
	// TokenConfigured: true when the config JSON contains a non-empty "token" key.
	data.TokenConfigured = src.Config != "" && strings.Contains(src.Config, `"token"`)
	return data
}
