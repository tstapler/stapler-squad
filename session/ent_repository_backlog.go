package session

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/backlogitem"
	"github.com/tstapler/stapler-squad/session/ent/backlogprogressnote"
	"github.com/tstapler/stapler-squad/session/ent/backlogstatusevent"
	"github.com/tstapler/stapler-squad/session/ent/backlogstuckstate"
	"github.com/tstapler/stapler-squad/session/ent/itemsession"
	"github.com/tstapler/stapler-squad/session/ent/itemsource"
	"github.com/tstapler/stapler-squad/session/ent/predicate"
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
		ID:                       is.ID.String(),
		BacklogItemID:            backlogItemID,
		SessionUUID:              is.SessionUUID,
		Role:                     is.SessionRole,
		AcSnapshot:               AcCriteriaJSON(is.AcSnapshot),
		PipelineModeSnapshot:     is.PipelineModeSnapshot,
		PipelineModeSnapshotHash: is.PipelineModeSnapshotHash,
		LastCommitSha:            is.LastCommitSha,
		LastCommitMessage:        is.LastCommitMessage,
		CommitCountSinceSpawn:    is.CommitCountSinceSpawn,
		StartedAt:                is.StartedAt,
		EndedAt:                  is.EndedAt,
		LastCommitAt:             is.LastCommitAt,
		LastFileTouchAt:          is.LastFileTouchAt,
		LastProgressAt:           is.LastProgressAt,
		CreatedAt:                is.CreatedAt,
		EstimatedCostUsd:         is.EstimatedCostUsd,
		TriageResult:             is.TriageResult,
		TriageResultSummary:      triageResultSummary,
		VerificationNotes:        is.VerificationNotes,
		OverallOutcome:           overallOutcome,
		ReviewVerdict:            reviewVerdictToSummary(is.Edges.ReviewVerdict),
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

// progressNoteToData maps an *ent.BacklogProgressNote to a ProgressNoteData DTO.
func progressNoteToData(n *ent.BacklogProgressNote) ProgressNoteData {
	return ProgressNoteData{
		ID:             n.ID.String(),
		CriterionIndex: n.CriterionIndex,
		Note:           n.Note,
		Status:         n.Status,
		CreatedAt:      n.CreatedAt,
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
		ID:                           item.ID.String(),
		Title:                        item.Title,
		Description:                  item.Description,
		AcceptanceCriteria:           AcCriteriaJSON(item.AcceptanceCriteria),
		Priority:                     item.Priority,
		Status:                       item.Status,
		RepoPath:                     item.RepoPath,
		SkipReviewGate:               item.SkipReviewGate,
		SkipPlanning:                 item.SkipPlanning,
		AutoSpawnSession:             item.AutoSpawnSession,
		AutoCreatePR:                 item.AutoCreatePr,
		PipelineMode:                 item.PipelineMode,
		PlanApproved:                 item.PlanApproved,
		PlanApprovedAt:               item.PlanApprovedAt,
		PlanArtifactsPath:            item.PlanArtifactsPath,
		Notes:                        item.Notes,
		ExternalID:                   item.ExternalID,
		ArchivedAt:                   item.ArchivedAt,
		PrURL:                        item.PrURL,
		PrNumber:                     item.PrNumber,
		ShippedCheckConclusion:       item.ShippedCheckConclusion,
		ShippedApprovedCount:         item.ShippedApprovedCount,
		ShippedChangesReqCount:       item.ShippedChangesReqCount,
		ShippedSnapshotAt:            item.ShippedSnapshotAt,
		ShippedFileStats:             item.ShippedFileStats,
		ShippedSnapshotCaptureFailed: item.ShippedSnapshotCaptureFailed,
		ReworkCapOverride:            item.ReworkCapOverride,
		CreatedAt:                    item.CreatedAt,
		UpdatedAt:                    item.UpdatedAt,
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
	// Propagate eagerly-loaded progress notes when present (see StatusEvents above).
	if item.Edges.ProgressNotes != nil {
		data.ProgressNotes = make([]ProgressNoteData, len(item.Edges.ProgressNotes))
		for i, n := range item.Edges.ProgressNotes {
			data.ProgressNotes[i] = progressNoteToData(n)
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
		SetAutoSpawnSession(data.AutoSpawnSession).
		SetAutoCreatePr(data.AutoCreatePR).
		SetPipelineMode(data.PipelineMode).
		SetPlanApproved(data.PlanApproved).
		SetNillablePlanApprovedAt(data.PlanApprovedAt).
		SetNillablePlanArtifactsPath(&data.PlanArtifactsPath).
		SetNillableNotes(&data.Notes).
		SetNillableExternalID(&data.ExternalID).
		SetNillableArchivedAt(data.ArchivedAt).
		SetNillableReworkCapOverride(data.ReworkCapOverride)

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
		WithProgressNotes(func(q *ent.BacklogProgressNoteQuery) {
			q.Order(ent.Asc(backlogprogressnote.FieldCreatedAt))
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

// excludedTerminalStatuses returns the statuses filter.ExcludeDone/ExcludeArchived
// ask to exclude from a default (no explicit Statuses) query, as a slice for
// StatusNotIn. The two flags are independent — either, both, or neither may be
// set — see BacklogItemFilter's doc comments.
func excludedTerminalStatuses(filter BacklogItemFilter) []string {
	var excluded []string
	if filter.ExcludeDone {
		excluded = append(excluded, string(BacklogStatusDone))
	}
	if filter.ExcludeArchived {
		excluded = append(excluded, string(BacklogStatusArchived))
	}
	return excluded
}

// FindDoneItemsOlderThan returns backlog items currently in "done" status
// whose most recent transition INTO "done" happened at or before cutoff.
// Used by the auto-archive sweep (archiveStaleDoneItems in
// backlog_lifecycle.go) to find items eligible for automatic archival.
//
// Deliberately keys off the status-event history rather than UpdatedAt:
// UpdatedAt changes on any field edit (progress notes, notification flags,
// etc.), which would reset — and so make unreliable — a clock meant to
// measure "how long has this been done". TransitionBacklogItemStatus always
// appends an audit BacklogStatusEvent row on every transition, so this reuses
// existing infrastructure instead of adding a dedicated done_at column.
//
// An item whose status-event history has no toStatus=="done" record is
// skipped (never considered eligible) rather than defaulting to "always
// eligible" — this should not happen in practice, since every transition
// through TransitionBacklogItemStatus writes one, but guards a partially-
// migrated or directly-seeded row against aging out on an unrelated basis.
func (r *EntRepository) FindDoneItemsOlderThan(ctx context.Context, cutoff time.Time) ([]BacklogItemData, error) {
	items, err := r.client.BacklogItem.Query().
		Where(backlogitem.StatusEQ(string(BacklogStatusDone))).
		WithStatusEvents(func(q *ent.BacklogStatusEventQuery) {
			q.Order(ent.Desc(backlogstatusevent.FieldCreatedAt))
		}).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("find done items older than cutoff: %w", err)
	}

	var result []BacklogItemData
	for _, item := range items {
		var lastDoneAt time.Time
		found := false
		for _, ev := range item.Edges.StatusEvents {
			if ev.ToStatus == string(BacklogStatusDone) {
				lastDoneAt = ev.CreatedAt
				found = true
				break // events ordered desc by CreatedAt — first match is most recent
			}
		}
		if !found || lastDoneAt.After(cutoff) {
			continue
		}
		result = append(result, backlogItemToData(item))
	}
	return result, nil
}

// ListBacklogItems returns backlog items with optional filtering.
func (r *EntRepository) ListBacklogItems(ctx context.Context, filter BacklogItemFilter) ([]BacklogItemData, error) {
	q := r.client.BacklogItem.Query()

	if len(filter.Statuses) > 0 {
		q = q.Where(backlogitem.StatusIn(filter.Statuses...))
	} else if excluded := excludedTerminalStatuses(filter); len(excluded) > 0 {
		q = q.Where(backlogitem.StatusNotIn(excluded...))
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
	} else if excluded := excludedTerminalStatuses(filter); len(excluded) > 0 {
		q = q.Where(backlogitem.StatusNotIn(excluded...))
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
	if update.AutoSpawnSession != nil {
		u.SetAutoSpawnSession(*update.AutoSpawnSession)
	}
	if update.AutoCreatePR != nil {
		u.SetAutoCreatePr(*update.AutoCreatePR)
	}
	if update.PipelineMode != nil {
		u.SetPipelineMode(*update.PipelineMode)
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
	if update.ShippedCheckConclusion != nil {
		u.SetShippedCheckConclusion(*update.ShippedCheckConclusion)
	}
	if update.ShippedApprovedCount != nil {
		u.SetShippedApprovedCount(*update.ShippedApprovedCount)
	}
	if update.ShippedChangesReqCount != nil {
		u.SetShippedChangesReqCount(*update.ShippedChangesReqCount)
	}
	if update.ShippedSnapshotAt != nil {
		u.SetShippedSnapshotAt(*update.ShippedSnapshotAt)
	}
	if update.ShippedFileStats != nil {
		u.SetShippedFileStats(*update.ShippedFileStats)
	}
	if update.ShippedSnapshotCaptureFailed != nil {
		u.SetShippedSnapshotCaptureFailed(*update.ShippedSnapshotCaptureFailed)
	}
	if update.ReworkCapOverride != nil {
		u.SetReworkCapOverride(*update.ReworkCapOverride)
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

// TransitionBacklogItemStatus changes the status of a backlog item with
// optional precondition.
//
// The precondition is enforced as a genuine compare-and-swap: it is folded
// into the UPDATE statement's WHERE clause (status = ? AND updated_at = ?)
// rather than checked against a separately-fetched row beforehand. The
// previous implementation did Get() → check-in-Go → UpdateOneID().Save(),
// which is a read-then-write race — nothing stopped a second, concurrent
// caller's write from landing in the gap between this call's read and its
// write, so a precondition that was true at read time could be false (and
// silently ignored) by write time. That TOCTOU window is exactly what let a
// stale AutoReopenAfterFailedReview call (reading the item while it was
// still "review", then queued/delayed behind other work) reopen an item that
// had, in the meantime, already legitimately shipped to "done" — see
// docs/bugs/fixed/BUG-026-backlog-transition-status-toctou-reopen.md for the
// live 2026-07-20 repro (item 0fd4a940, PR #176) this closes.
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

	update := r.client.BacklogItem.Update().Where(backlogitem.ID(parsedID))
	if precondition != nil {
		if precondition.ExpectedStatus != "" {
			update = update.Where(backlogitem.StatusEQ(precondition.ExpectedStatus))
		}
		if precondition.ExpectedUpdatedAt != nil {
			update = update.Where(backlogitem.UpdatedAtEQ(*precondition.ExpectedUpdatedAt))
		}
	}

	now := time.Now()
	affected, err := update.
		SetStatus(string(toStatus)).
		SetUserModifiedStatusAt(now).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to transition backlog item %s status: %w", id, err)
	}
	if affected == 0 {
		// The row either no longer exists or the precondition no longer holds —
		// re-fetch to report which. A concurrent writer may have won the race in
		// the instant between our Get above and this UPDATE, so the check must be
		// against fresh data, not `current`.
		latest, getErr := r.client.BacklogItem.Get(ctx, parsedID)
		if getErr != nil {
			if ent.IsNotFound(getErr) {
				return nil, fmt.Errorf("%w: backlog item %s", ErrNotFound, id)
			}
			return nil, fmt.Errorf("failed to get backlog item %s: %w", id, getErr)
		}
		if precondition != nil && precondition.ExpectedStatus != "" && latest.Status != precondition.ExpectedStatus {
			return nil, fmt.Errorf("%w: expected status %q, got %q", ErrPreconditionFailed, precondition.ExpectedStatus, latest.Status)
		}
		return nil, fmt.Errorf("%w: updated_at mismatch", ErrPreconditionFailed)
	}

	item, err := r.client.BacklogItem.Get(ctx, parsedID)
	if err != nil {
		return nil, fmt.Errorf("failed to reload backlog item %s after transition: %w", id, err)
	}

	// Append an immutable audit record for this transition. When the
	// precondition asserted an expected status, that value is what the row
	// atomically held the instant this write landed (guaranteed by the WHERE
	// clause above) — more reliable than `current.Status` from the earlier,
	// non-atomic Get. Unconditional transitions (no precondition) fall back to
	// the earlier read, same as before this fix; best-effort either way.
	fromStatus := current.Status
	if precondition != nil && precondition.ExpectedStatus != "" {
		fromStatus = precondition.ExpectedStatus
	}
	evCreate := r.client.BacklogStatusEvent.Create().
		SetItemID(parsedID).
		SetFromStatus(fromStatus).
		SetToStatus(string(toStatus)).
		SetTriggeredBy(TriggeredBySystem)
	if precondition != nil && precondition.Note != "" {
		evCreate = evCreate.SetNote(precondition.Note)
	}
	if _, evErr := evCreate.Save(ctx); evErr != nil {
		// Non-fatal: audit log failure should not block the transition itself.
		_ = evErr
	}

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

// RecordRemediationAttempt records that an automated (or operator-triggered,
// see TriggerRemediationNow) remediation attempt was just made for an open
// (item_id, reason) row: sets remediation_attempts to attempts and
// next_remediation_at to nextAt (nil once attempts has hit the cap — see
// nextRemediationAt in backlog_remediation.go). Callers compute attempts/nextAt
// themselves (via the shared backoff gate) rather than this method
// incrementing in place, so a single code path (evaluateRemediation) owns the
// backoff-schedule arithmetic. Scoped to WHERE resolved_at IS NULL, matching
// every other stuck-state write in this file — a row that resolved between
// the gate's read and this write is left alone rather than resurrected.
func (r *EntRepository) RecordRemediationAttempt(ctx context.Context, itemID string, reason domain.StuckReason, attempts int32, nextAt *time.Time) (bool, error) {
	parsedID, err := uuid.Parse(itemID)
	if err != nil {
		return false, fmt.Errorf("%w: invalid id %q: %v", ErrNotFound, itemID, err)
	}

	update := r.client.BacklogStuckState.Update().
		Where(
			backlogstuckstate.ItemID(parsedID),
			backlogstuckstate.Reason(string(reason)),
			backlogstuckstate.ResolvedAtIsNil(),
		).
		SetRemediationAttempts(attempts)
	if nextAt != nil {
		update = update.SetNextRemediationAt(*nextAt)
	} else {
		update = update.ClearNextRemediationAt()
	}

	n, err := update.Save(ctx)
	if err != nil {
		return false, fmt.Errorf("record remediation attempt %s/%s: %w", itemID, reason, err)
	}
	return n > 0, nil
}

// RecordRemediationRestartGrace records that itemID/reason's open row just
// consumed its one-per-boot restart-grace pass (see evaluateRemediation):
// sets grace_boot_time to bootTime WITHOUT touching remediation_attempts or
// next_remediation_at — a grace pass lets the wrapped remediation action run
// without spending any of the row's 5-attempt budget.
func (r *EntRepository) RecordRemediationRestartGrace(ctx context.Context, itemID string, reason domain.StuckReason, bootTime time.Time) (bool, error) {
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
		SetGraceBootTime(bootTime).
		Save(ctx)
	if err != nil {
		return false, fmt.Errorf("record remediation restart grace %s/%s: %w", itemID, reason, err)
	}
	return n > 0, nil
}

// ResetStuckRemediation clears the automated-remediation counters on a single
// open (item_id, reason) row: remediation_attempts back to 0,
// next_remediation_at and notified_at cleared. Clearing notified_at (in
// addition to the remediation counters) lets a fresh notify+respawn cycle
// fire on the very next detector tick instead of waiting on stale dedup
// state — the same reasoning as MarkStuck's reopen-in-place path. A no-op
// (false, nil), not an error, when no open row matches (item_id, reason).
func (r *EntRepository) ResetStuckRemediation(ctx context.Context, itemID string, reason domain.StuckReason) (bool, error) {
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
		SetRemediationAttempts(0).
		ClearNextRemediationAt().
		ClearNotifiedAt().
		Save(ctx)
	if err != nil {
		return false, fmt.Errorf("reset stuck remediation %s/%s: %w", itemID, reason, err)
	}
	return n > 0, nil
}

// BulkResetStuckRemediation applies ResetStuckRemediation's reset (attempts
// -> 0, next_remediation_at/notified_at cleared) to every open BacklogStuckState
// row, optionally filtered to a single reason (reason == nil means "every
// reason") and, when onlyParked is true (the default from the RPC layer),
// restricted to rows that actually hit the attempt cap
// (remediation_attempts >= MaxRemediationAttempts) — the "something upstream
// broke a batch of these, give them all a fresh shot" admin action. Returns
// the number of rows reset.
func (r *EntRepository) BulkResetStuckRemediation(ctx context.Context, reason *domain.StuckReason, onlyParked bool) (int, error) {
	predicates := []predicate.BacklogStuckState{backlogstuckstate.ResolvedAtIsNil()}
	if reason != nil {
		predicates = append(predicates, backlogstuckstate.Reason(string(*reason)))
	}
	if onlyParked {
		predicates = append(predicates, backlogstuckstate.RemediationAttemptsGTE(MaxRemediationAttempts))
	}

	n, err := r.client.BacklogStuckState.Update().
		Where(predicates...).
		SetRemediationAttempts(0).
		ClearNextRemediationAt().
		ClearNotifiedAt().
		Save(ctx)
	if err != nil {
		return 0, fmt.Errorf("bulk reset stuck remediation: %w", err)
	}
	return n, nil
}

// --- Progress note history ---

// AppendProgressNote records a single report_progress call as an immutable history
// entry, in addition to (not instead of) the current-note-per-criterion stored on
// BacklogItem.AcceptanceCriteria. Callers should treat failures here as best-effort:
// the history is an enrichment for reviewers, not part of report_progress's primary
// contract of updating the criterion's current status/note.
func (r *EntRepository) AppendProgressNote(ctx context.Context, itemID string, criterionIndex int, note, status string) error {
	parsedItemID, err := uuid.Parse(itemID)
	if err != nil {
		return fmt.Errorf("invalid item id %q: %w", itemID, err)
	}

	_, err = r.client.BacklogProgressNote.Create().
		SetItemID(parsedItemID).
		SetCriterionIndex(criterionIndex).
		SetNote(note).
		SetStatus(status).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("failed to append progress note for item %s: %w", itemID, err)
	}
	return nil
}

// ListProgressNotesForItem returns the full append-only history of report_progress
// calls for a backlog item, ordered by created_at ascending (oldest first).
func (r *EntRepository) ListProgressNotesForItem(ctx context.Context, itemID string) ([]ProgressNoteData, error) {
	parsedItemID, err := uuid.Parse(itemID)
	if err != nil {
		return nil, fmt.Errorf("invalid item id %q: %w", itemID, err)
	}

	notes, err := r.client.BacklogProgressNote.Query().
		Where(backlogprogressnote.HasItemWith(backlogitem.ID(parsedItemID))).
		Order(ent.Asc(backlogprogressnote.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list progress notes for item %s: %w", itemID, err)
	}
	result := make([]ProgressNoteData, len(notes))
	for i, n := range notes {
		result[i] = progressNoteToData(n)
	}
	return result, nil
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
