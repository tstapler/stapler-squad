package session

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/backlogitem"
	"github.com/tstapler/stapler-squad/session/ent/backlogstatusevent"
	"github.com/tstapler/stapler-squad/session/ent/backlogstuckstate"
	"github.com/tstapler/stapler-squad/session/ent/itemsession"
	"github.com/tstapler/stapler-squad/session/ent/itemsource"
	"github.com/tstapler/stapler-squad/session/ent/reviewverdict"
	entSession "github.com/tstapler/stapler-squad/session/ent/session"
	"github.com/tstapler/stapler-squad/session/ent/sourcesyncevent"
)

// --- converters ---

// reviewVerdictToSummary maps an *ent.ReviewVerdict to a ReviewVerdictSummary DTO.
// Returns nil when rv is nil so callers can check the pointer directly.
func reviewVerdictToSummary(rv *ent.ReviewVerdict) *ReviewVerdictSummary {
	if rv == nil {
		return nil
	}
	return &ReviewVerdictSummary{
		ID:             rv.ID.String(),
		OverallOutcome: rv.OverallOutcome,
		PerCriterion:   rv.PerCriterion,
		Summary:        rv.Summary,
		DiffTokenCount: rv.DiffTokenCount,
		DiffTruncated:  rv.DiffTruncated,
		OverrideBy:     rv.OverrideBy,
		OverrideReason: rv.OverrideReason,
		OverrideAt:     rv.OverrideAt,
		CreatedAt:      rv.CreatedAt,
	}
}

// itemSessionToSummary maps an *ent.ItemSession to an ItemSessionSummary DTO.
// The BacklogItem edge must be eagerly loaded (WithBacklogItem) for BacklogItemID to be populated.
// The ReviewVerdict edge must be eagerly loaded (WithReviewVerdict) for ReviewVerdict to be non-nil.
func itemSessionToSummary(is *ent.ItemSession) ItemSessionSummary {
	var backlogItemID string
	if is.Edges.BacklogItem != nil {
		backlogItemID = is.Edges.BacklogItem.ID.String()
	}

	// Parse TriageResultSummary from the triage_result JSON column.
	var triageResultSummary string
	if is.TriageResult != "" {
		var tr struct {
			Summary string `json:"summary"`
		}
		_ = json.Unmarshal([]byte(is.TriageResult), &tr)
		triageResultSummary = tr.Summary
	}

	// OverallOutcome from the linked ReviewVerdict (empty when no verdict exists yet).
	var overallOutcome string
	if is.Edges.ReviewVerdict != nil {
		overallOutcome = is.Edges.ReviewVerdict.OverallOutcome
	}

	return ItemSessionSummary{
		ID:                    is.ID.String(),
		BacklogItemID:         backlogItemID,
		SessionUUID:           is.SessionUUID,
		Role:                  is.SessionRole,
		AcSnapshot:            AcCriteriaJSON(is.AcSnapshot),
		LastCommitSha:         is.LastCommitSha,
		LastCommitMessage:     is.LastCommitMessage,
		CommitCountSinceSpawn: is.CommitCountSinceSpawn,
		StartedAt:             is.StartedAt,
		EndedAt:               is.EndedAt,
		LastCommitAt:          is.LastCommitAt,
		LastFileTouchAt:       is.LastFileTouchAt,
		LastProgressAt:        is.LastProgressAt,
		CreatedAt:             is.CreatedAt,
		EstimatedCostUsd:      is.EstimatedCostUsd,
		TriageResult:          is.TriageResult,
		TriageResultSummary:   triageResultSummary,
		VerificationNotes:     is.VerificationNotes,
		OverallOutcome:        overallOutcome,
		ReviewVerdict:         reviewVerdictToSummary(is.Edges.ReviewVerdict),
	}
}

// backlogStatusEventToData maps an *ent.BacklogStatusEvent to a BacklogStatusEventData DTO.
func backlogStatusEventToData(e *ent.BacklogStatusEvent) BacklogStatusEventData {
	return BacklogStatusEventData{
		ID:          e.ID.String(),
		FromStatus:  e.FromStatus,
		ToStatus:    e.ToStatus,
		TriggeredBy: e.TriggeredBy,
		Note:        e.Note,
		CreatedAt:   e.CreatedAt,
	}
}

// sourceSyncEventToData maps an *ent.SourceSyncEvent to a SourceSyncEventData DTO.
func sourceSyncEventToData(e *ent.SourceSyncEvent) SourceSyncEventData {
	return SourceSyncEventData{
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

func backlogItemToData(item *ent.BacklogItem) BacklogItemData {
	data := BacklogItemData{
		ID:                 item.ID.String(),
		Title:              item.Title,
		Description:        item.Description,
		AcceptanceCriteria: AcCriteriaJSON(item.AcceptanceCriteria),
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
		PrURL:              item.PrURL,
		PrNumber:           item.PrNumber,
		CreatedAt:          item.CreatedAt,
		UpdatedAt:          item.UpdatedAt,
	}
	// Resolve source ID from the eager-loaded edge when available.
	if item.Edges.Source != nil {
		data.SourceID = item.Edges.Source.ID.String()
	}
	// Propagate eagerly-loaded status events when present.
	if item.Edges.StatusEvents != nil {
		data.StatusEvents = make([]BacklogStatusEventData, len(item.Edges.StatusEvents))
		for i, ev := range item.Edges.StatusEvents {
			data.StatusEvents[i] = backlogStatusEventToData(ev)
		}
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
		SetNillableAcceptanceCriteria(nilIfEmptyJSON(data.AcceptanceCriteria)).
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
		WithStatusEvents(func(q *ent.BacklogStatusEventQuery) {
			q.Order(ent.Asc(backlogstatusevent.FieldCreatedAt))
		}).
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

// ListBacklogItemSummaries returns lightweight BacklogItemSummary values for list views.
// Three-phase: (1) scalar fields via .All(), (2) item sessions + review verdicts via edge loading.
func (r *EntRepository) ListBacklogItemSummaries(ctx context.Context, filter BacklogItemFilter) ([]BacklogItemSummary, error) {
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
	items, err := q.All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list backlog item summaries: %w", err)
	}
	summaries := make([]BacklogItemSummary, len(items))
	idIndex := make(map[string]int, len(items))
	parsedUUIDs := make([]uuid.UUID, 0, len(items))
	for i, item := range items {
		summaries[i] = BacklogItemSummary{
			ID:                 item.ID.String(),
			ExternalID:         item.ExternalID,
			Title:              item.Title,
			Status:             BacklogStatus(item.Status),
			Priority:           item.Priority,
			RepoPath:           item.RepoPath,
			AcceptanceCriteria: AcCriteriaJSON(item.AcceptanceCriteria),
			Notes:              item.Notes,
			PrURL:              item.PrURL,
			PrNumber:           item.PrNumber,
			CreatedAt:          item.CreatedAt,
			UpdatedAt:          item.UpdatedAt,
			ArchivedAt:         item.ArchivedAt,
		}
		idIndex[item.ID.String()] = i
		parsedUUIDs = append(parsedUUIDs, item.ID)
	}
	if len(parsedUUIDs) == 0 {
		return summaries, nil
	}

	// Phase 2+3: load item sessions with review verdicts via edge loading.
	sessions, err := r.client.ItemSession.Query().
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
		summaries[idx].ItemSessions = append(summaries[idx].ItemSessions, itemSessionToSummary(s))
	}
	return summaries, nil
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
		u.SetAcceptanceCriteria(string(*update.AcceptanceCriteria))
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
	if update.PrURL != nil {
		u.SetPrURL(*update.PrURL)
	}
	if update.PrNumber != nil {
		u.SetPrNumber(*update.PrNumber)
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

	current, err := r.client.BacklogItem.Get(ctx, parsedID)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf("%w: backlog item %s", ErrNotFound, id)
		}
		return nil, fmt.Errorf("failed to get backlog item %s: %w", id, err)
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

	r.recordStatusEvent(ctx, r.client.BacklogStatusEvent, parsedID, current.Status, string(BacklogStatusArchived), TriggeredByUser, "")

	result := backlogItemToData(item)
	return &result, nil
}

// recordStatusEvent appends an immutable BacklogStatusEvent audit row for a status
// transition. client may be r.client.BacklogStatusEvent (non-tx callers) or a
// tx.BacklogStatusEvent (callers already inside a transaction) — both share the same
// generated client type. A write failure is logged and otherwise swallowed: the audit
// trail is best-effort and must never fail the status transition it's recording.
func (r *EntRepository) recordStatusEvent(ctx context.Context, client *ent.BacklogStatusEventClient, itemID uuid.UUID, fromStatus, toStatus, triggeredBy, note string) {
	evCreate := client.Create().
		SetItemID(itemID).
		SetFromStatus(fromStatus).
		SetToStatus(toStatus).
		SetTriggeredBy(triggeredBy)
	if note != "" {
		evCreate = evCreate.SetNote(note)
	}
	if _, evErr := evCreate.Save(ctx); evErr != nil {
		log.ErrorLog.Printf("[EntRepository] failed to record status event item=%s from=%s to=%s: %v", itemID, fromStatus, toStatus, evErr)
	}
}

// DeleteBacklogItem permanently removes an item and all its child records.
func (r *EntRepository) DeleteBacklogItem(ctx context.Context, id string) error {
	parsedID, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("%w: invalid id %q: %v", ErrNotFound, id, err)
	}

	// Resolve item_session IDs first so we can delete their review_verdicts.
	itemSessionIDs, err := r.client.ItemSession.Query().
		Where(itemsession.HasBacklogItemWith(backlogitem.ID(parsedID))).
		IDs(ctx)
	if err != nil {
		return fmt.Errorf("failed to query item sessions for backlog item %s: %w", id, err)
	}

	if len(itemSessionIDs) > 0 {
		_, err = r.client.ReviewVerdict.Delete().
			Where(reviewverdict.HasItemSessionWith(itemsession.IDIn(itemSessionIDs...))).
			Exec(ctx)
		if err != nil {
			return fmt.Errorf("failed to delete review verdicts for backlog item %s: %w", id, err)
		}

		_, err = r.client.ItemSession.Delete().
			Where(itemsession.IDIn(itemSessionIDs...)).
			Exec(ctx)
		if err != nil {
			return fmt.Errorf("failed to delete item sessions for backlog item %s: %w", id, err)
		}
	}

	err = r.client.BacklogItem.DeleteOneID(parsedID).Exec(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return fmt.Errorf("%w: backlog item %s", ErrNotFound, id)
		}
		return fmt.Errorf("failed to delete backlog item %s: %w", id, err)
	}
	return nil
}

// TransitionBacklogItemStatus changes the status of a backlog item with optional precondition.
func (r *EntRepository) TransitionBacklogItemStatus(ctx context.Context, id string, toStatus BacklogStatus, precondition *BacklogItemPrecondition, triggeredBy string) (*BacklogItemData, error) {
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

	// Append an immutable audit record for this transition.
	var note string
	if precondition != nil {
		note = precondition.Note
	}
	r.recordStatusEvent(ctx, r.client.BacklogStatusEvent, parsedID, current.Status, string(toStatus), triggeredBy, note)

	result := backlogItemToData(item)
	return &result, nil
}

// --- BacklogStuckState (durable stuck-state bookkeeping) ---

// MarkStuck opens, refreshes, or reopens a durable BacklogStuckState row for
// the given item + reason via a resolve-in-place upsert on the (item_id,
// reason) unique index — there is exactly one row per pair at all times.
//
// A best-effort item-status precondition is applied before writing: if the
// item's current status does not equal expectedStatus, MarkStuck returns
// (false, nil) without writing. This precondition is NOT atomic with the
// write itself (a concurrent transition can still race in between); the
// self-heal sweep (reconcile pipeline, Phase 2) is the correctness backstop
// for any stale write that still lands.
//
// Row semantics on conflict with an existing (item_id, reason) row:
//   - OPEN row (resolved_at IS NULL): only last_checked_at and context are
//     refreshed. first_detected_at and notified_at are left untouched, so
//     notify-once dedup and the "stuck for N" duration both survive repeated
//     ticks.
//   - RESOLVED row (resolved_at IS NOT NULL): the SAME row is reopened in
//     place — resolved_at and notified_at are cleared and first_detected_at
//     is reset to now — never a second row for the same pair.
//
// Implementation note: this is two atomic statements (an INSERT ... ON
// CONFLICT upsert, then a conditional UPDATE ... WHERE resolved_at IS NOT
// NULL) inside one DB transaction, rather than a single raw SQL statement.
// Ent's generated upsert Update() callback has no portable way to express a
// per-row CASE WHEN keyed off the pre-existing resolved_at value without
// hand-written dialect-specific SQL, so the reopen adjustment is split into
// its own atomic, idempotent conditional UPDATE. Row-dedup itself — the
// concurrency-sensitive part — is still guaranteed by the single upsert
// statement; there is no read-then-write for detecting whether the row
// exists.
func (r *EntRepository) MarkStuck(ctx context.Context, itemID string, reason domain.StuckReason, expectedStatus BacklogStatus, stuckContext string) (applied bool, err error) {
	if !reason.IsValid() {
		return false, fmt.Errorf("invalid stuck reason %q", reason)
	}
	parsedID, err := uuid.Parse(itemID)
	if err != nil {
		return false, fmt.Errorf("%w: invalid id %q: %v", ErrNotFound, itemID, err)
	}

	current, err := r.client.BacklogItem.Get(ctx, parsedID)
	if err != nil {
		if ent.IsNotFound(err) {
			return false, fmt.Errorf("%w: backlog item %s", ErrNotFound, itemID)
		}
		return false, fmt.Errorf("failed to get backlog item %s: %w", itemID, err)
	}
	if current.Status != string(expectedStatus) {
		// Best-effort precondition mismatch: not an error, just not applied.
		return false, nil
	}

	now := time.Now()
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return false, fmt.Errorf("mark stuck: begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	err = tx.BacklogStuckState.Create().
		SetItemID(parsedID).
		SetReason(string(reason)).
		SetFirstDetectedAt(now).
		SetLastCheckedAt(now).
		SetContext(stuckContext).
		OnConflictColumns(backlogstuckstate.FieldItemID, backlogstuckstate.FieldReason).
		Update(func(u *ent.BacklogStuckStateUpsert) {
			u.SetLastCheckedAt(now)
			u.SetContext(stuckContext)
		}).
		Exec(ctx)
	if err != nil {
		return false, fmt.Errorf("mark stuck: upsert %s/%s: %w", itemID, reason, err)
	}

	// Reopen-in-place: only touches a row that was already resolved. This is
	// its own atomic, idempotent conditional UPDATE — running it unconditionally
	// after the upsert above is safe because it only ever affects a resolved row.
	if _, err := tx.BacklogStuckState.Update().
		Where(
			backlogstuckstate.ItemID(parsedID),
			backlogstuckstate.Reason(string(reason)),
			backlogstuckstate.ResolvedAtNotNil(),
		).
		ClearResolvedAt().
		ClearNotifiedAt().
		SetFirstDetectedAt(now).
		Save(ctx); err != nil {
		return false, fmt.Errorf("mark stuck: reopen %s/%s: %w", itemID, reason, err)
	}

	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("mark stuck: commit: %w", err)
	}
	return true, nil
}

// ResolveStuck atomically, idempotently closes an open BacklogStuckState row
// via a single conditional UPDATE ... WHERE resolved_at IS NULL. Returns
// whether a row was actually resolved by this call; resolving an
// already-resolved or nonexistent (item_id, reason) row is a no-op, not an
// error, and never overwrites an existing resolved_at.
func (r *EntRepository) ResolveStuck(ctx context.Context, itemID string, reason domain.StuckReason) (bool, error) {
	parsedID, err := uuid.Parse(itemID)
	if err != nil {
		return false, fmt.Errorf("%w: invalid id %q: %v", ErrNotFound, itemID, err)
	}

	n, err := r.client.BacklogStuckState.Update().
		Where(
			backlogstuckstate.ItemID(parsedID),
			backlogstuckstate.Reason(string(reason)),
			backlogstuckstate.ResolvedAtIsNil(),
		).
		SetResolvedAt(time.Now()).
		Save(ctx)
	if err != nil {
		return false, fmt.Errorf("resolve stuck %s/%s: %w", itemID, reason, err)
	}
	return n > 0, nil
}

// MarkStuckNotified sets notified_at=now on an open, not-yet-notified stuck
// row — the durable notify-once dedup write, called once after a stuck
// notification has actually been sent. A no-op (not an error) if the row is
// already notified or doesn't exist.
func (r *EntRepository) MarkStuckNotified(ctx context.Context, itemID string, reason domain.StuckReason) (bool, error) {
	parsedID, err := uuid.Parse(itemID)
	if err != nil {
		return false, fmt.Errorf("%w: invalid id %q: %v", ErrNotFound, itemID, err)
	}

	n, err := r.client.BacklogStuckState.Update().
		Where(
			backlogstuckstate.ItemID(parsedID),
			backlogstuckstate.Reason(string(reason)),
			backlogstuckstate.NotifiedAtIsNil(),
		).
		SetNotifiedAt(time.Now()).
		Save(ctx)
	if err != nil {
		return false, fmt.Errorf("mark stuck notified %s/%s: %w", itemID, reason, err)
	}
	return n > 0, nil
}

// SnoozeStuckState sets snoozed_until on an open BacklogStuckState row via a
// single atomic conditional UPDATE ... WHERE resolved_at IS NULL, matching the
// ResolveStuck pattern. Returns whether a row was actually updated by this
// call; snoozing a nonexistent or already-resolved (item_id, reason) row is a
// no-op, not an error. A snoozed-until-past-now value simply un-snoozes the
// row on the next FindOpenStuckStates read (its predicate is
// snoozed_until IS NULL OR snoozed_until < now).
func (r *EntRepository) SnoozeStuckState(ctx context.Context, itemID string, reason domain.StuckReason, until time.Time) (bool, error) {
	parsedID, err := uuid.Parse(itemID)
	if err != nil {
		return false, fmt.Errorf("%w: invalid id %q: %v", ErrNotFound, itemID, err)
	}

	n, err := r.client.BacklogStuckState.Update().
		Where(
			backlogstuckstate.ItemID(parsedID),
			backlogstuckstate.Reason(string(reason)),
			backlogstuckstate.ResolvedAtIsNil(),
		).
		SetSnoozedUntil(until).
		Save(ctx)
	if err != nil {
		return false, fmt.Errorf("snooze stuck %s/%s: %w", itemID, reason, err)
	}
	return n > 0, nil
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

// GetBacklogItemByExternalID retrieves a BacklogItem by its external_id, scoped
// to sourceID. External IDs (e.g. GitHub issue/PR numbers) are only unique
// within their source, not globally — two different repos can both have an
// issue #1, so this must never match across sources.
func (r *EntRepository) GetBacklogItemByExternalID(ctx context.Context, sourceID, externalID string) (*ent.BacklogItem, error) {
	parsedSourceID, err := uuid.Parse(sourceID)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid source id %q: %v", ErrNotFound, sourceID, err)
	}
	item, err := r.client.BacklogItem.Query().
		Where(backlogitem.ExternalID(externalID), backlogitem.HasSourceWith(itemsource.ID(parsedSourceID))).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to query backlog item by external_id %q: %w", externalID, err)
	}
	return item, nil
}

// maxSourceSyncEventsHistory caps how many sync history rows a single
// GetSyncHistory call returns, so a long-lived, frequently-synced source
// doesn't grow into an unbounded response.
const maxSourceSyncEventsHistory = 200

// ListSourceSyncEvents returns sync history events for an item source, most recent
// first, capped at maxSourceSyncEventsHistory rows. truncated is true when older
// events exist beyond the cap — callers should surface this to avoid silently
// hiding history for sources with long or frequent sync runs.
func (r *EntRepository) ListSourceSyncEvents(ctx context.Context, sourceID string) ([]SourceSyncEventData, bool, error) {
	parsedID, err := uuid.Parse(sourceID)
	if err != nil {
		return nil, false, fmt.Errorf("invalid source id %q: %w", sourceID, err)
	}
	// Fetch one extra row to distinguish "exactly at the cap" from "more exist beyond it".
	rawEvents, err := r.client.SourceSyncEvent.Query().
		Where(sourcesyncevent.HasSourceWith(itemsource.ID(parsedID))).
		Order(ent.Desc(sourcesyncevent.FieldStartedAt)).
		Limit(maxSourceSyncEventsHistory + 1).
		All(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("failed to list sync events for source %s: %w", sourceID, err)
	}
	truncated := len(rawEvents) > maxSourceSyncEventsHistory
	if truncated {
		rawEvents = rawEvents[:maxSourceSyncEventsHistory]
	}
	events := make([]SourceSyncEventData, len(rawEvents))
	for i, ev := range rawEvents {
		events[i] = sourceSyncEventToData(ev)
	}
	return events, truncated, nil
}

// CreateSourceSyncEvent records a completed (or failed) sync run for an
// ItemSource. errMsg should be non-empty only when the sync run failed
// outright (e.g. the plugin's Fetch call errored); errored counts per-item
// failures within an otherwise-successful fetch.
func (r *EntRepository) CreateSourceSyncEvent(ctx context.Context, sourceID string, cursorAfter string, created, updated, skipped, errored int, errMsg string, startedAt, finishedAt time.Time) error {
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

// FinishSourceSync atomically advances an ItemSource's sync cursor/last_synced_at
// and records the SourceSyncEvent for a successful sync run. Wrapping both
// writes in one transaction prevents a crash between them from leaving the
// cursor advanced with no corresponding history row — which would silently
// hide the fact that a batch of items was processed (or dropped) in that run.
func (r *EntRepository) FinishSourceSync(ctx context.Context, sourceID string, cursorAfter string, created, updated, skipped, errored int, startedAt, finishedAt time.Time) error {
	parsedID, err := uuid.Parse(sourceID)
	if err != nil {
		return fmt.Errorf("invalid source id %q: %w", sourceID, err)
	}

	tx, err := r.client.Tx(ctx)
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
			return fmt.Errorf("%w: item source %s", ErrNotFound, sourceID)
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

// GetAllItemSessionsWithBacklogInfo returns all item sessions joined with their parent
// backlog item's ID, title, and status. Used by the Insights dashboard index.
func (r *EntRepository) GetAllItemSessionsWithBacklogInfo(ctx context.Context) ([]ItemSessionBacklogEntry, error) {
	sessions, err := r.client.ItemSession.Query().
		WithBacklogItem().
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query item sessions with backlog info: %w", err)
	}
	results := make([]ItemSessionBacklogEntry, 0, len(sessions))
	for _, is := range sessions {
		if is.Edges.BacklogItem == nil {
			continue
		}
		results = append(results, ItemSessionBacklogEntry{
			SessionUUID: is.SessionUUID,
			SessionRole: is.SessionRole,
			ItemID:      is.Edges.BacklogItem.ID.String(),
			ItemTitle:   is.Edges.BacklogItem.Title,
			ItemStatus:  is.Edges.BacklogItem.Status,
		})
	}
	return results, nil
}

// GetWorktreeDataBySessionUUID returns the git worktree data for the Session with
// the given UUID. Returns an empty GitWorktreeData (no error) if the session does
// not exist or is a directory-mode session without a dedicated worktree.
func (r *EntRepository) GetWorktreeDataBySessionUUID(ctx context.Context, sessionUUID string) (GitWorktreeData, error) {
	sess, err := r.client.Session.Query().
		Where(entSession.UUID(sessionUUID)).
		WithWorktree().
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return GitWorktreeData{}, nil
		}
		return GitWorktreeData{}, fmt.Errorf("GetWorktreeDataBySessionUUID %q: %w", sessionUUID, err)
	}
	if sess.Edges.Worktree == nil {
		return GitWorktreeData{}, nil
	}
	return GitWorktreeData{
		RepoPath:      sess.Edges.Worktree.RepoPath,
		WorktreePath:  sess.Edges.Worktree.WorktreePath,
		SessionName:   sess.Edges.Worktree.SessionName,
		BranchName:    sess.Edges.Worktree.BranchName,
		BaseCommitSHA: sess.Edges.Worktree.BaseCommitSha,
	}, nil
}
