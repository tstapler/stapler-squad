package session

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/backlogactivitynote"
	"github.com/tstapler/stapler-squad/session/ent/backlogitem"
	"github.com/tstapler/stapler-squad/session/ent/backlogitemdependency"
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

// SetItemChangePublisher wires an ItemChangePublisher into this repository so
// its backlog mutation methods (TransitionBacklogItemStatus, UpdateBacklogItem,
// ArchiveBacklogItem, DeleteBacklogItem, SaveReviewVerdict,
// CreateItemSessionWithVerdict, CreateItemSession,
// UpdateItemSessionSessionUUID, UpdateItemSessionTriageResult) can publish a
// best-effort change notification after each successful mutation. Called via
// Storage.SetItemChangePublisher's forwarding method (session/storage.go),
// which is the only entry point server/dependencies.go has since it holds a
// *Storage, not a concrete *EntRepository.
func (r *EntRepository) SetItemChangePublisher(p ItemChangePublisher) {
	r.itemChangePublisher = p
}

// recordStatusEvent appends an immutable BacklogStatusEvent audit row. Pass
// r.client.BacklogStatusEvent for a standalone write, or tx.BacklogStatusEvent to
// write inside an existing transaction — required when called from ReconcileStuckItems
// since SQLite is capped to one connection (SetMaxOpenConns(1)) and calling back through
// the non-tx client would self-deadlock. A write failure is logged, not returned: an
// audit-log gap must never block the status transition itself.
func recordStatusEvent(ctx context.Context, evClient *ent.BacklogStatusEventClient, itemID uuid.UUID, fromStatus, toStatus, triggeredBy, note string) {
	evCreate := evClient.Create().
		SetItemID(itemID).
		SetFromStatus(fromStatus).
		SetToStatus(toStatus).
		SetTriggeredBy(triggeredBy)
	if note != "" {
		evCreate = evCreate.SetNote(note)
	}
	if _, err := evCreate.Save(ctx); err != nil {
		log.ErrorLog.Printf("[recordStatusEvent] failed to record item=%s %s->%s triggeredBy=%s: %v", itemID, fromStatus, toStatus, triggeredBy, err)
	}
}

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
		DiffHash:       rv.DiffHash,
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
		BaseCommitSha:            is.BaseCommitSha,
		LastCommitSha:            is.LastCommitSha,
		LastCommitMessage:        is.LastCommitMessage,
		CommitCountSinceSpawn:    is.CommitCountSinceSpawn,
		StartedAt:                is.StartedAt,
		EndedAt:                  is.EndedAt,
		EndReason:                is.EndReason,
		FailureCapturePath:       is.FailureCapturePath,
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
		ClaimantHostID:           is.ClaimantHostID,
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

// activityNoteToData maps an *ent.BacklogActivityNote to an ActivityNoteData DTO.
func activityNoteToData(n *ent.BacklogActivityNote) ActivityNoteData {
	return ActivityNoteData{
		ID:                 n.ID.String(),
		Message:            n.Message,
		AuthorSessionUUID:  n.AuthorSessionUUID,
		AuthorSessionTitle: n.AuthorSessionTitle,
		CreatedAt:          n.CreatedAt,
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
		PublicIDRaw:                  item.PublicID,
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
		Category:                     item.Category,
		PlanApproved:                 item.PlanApproved,
		PlanApprovedAt:               item.PlanApprovedAt,
		QueuedAt:                     item.QueuedAt,
		QueuedAutonomous:             item.QueuedAutonomous,
		PlanArtifactsPath:            item.PlanArtifactsPath,
		PlanRejectionReason:          item.PlanRejectionReason,
		PlanRejectedAt:               item.PlanRejectedAt,
		Notes:                        item.Notes,
		ExternalID:                   item.ExternalID,
		ExternalURL:                  item.ExternalURL,
		Labels:                       item.Labels,
		ArchivedAt:                   item.ArchivedAt,
		PrURL:                        item.PrURL,
		PrNumber:                     item.PrNumber,
		ShippedCheckConclusion:       item.ShippedCheckConclusion,
		ShippedApprovedCount:         item.ShippedApprovedCount,
		ShippedChangesReqCount:       item.ShippedChangesReqCount,
		ShippedSnapshotAt:            item.ShippedSnapshotAt,
		PrFeedbackAddressedAt:        item.PrFeedbackAddressedAt,
		GitHubSyncedIssueUpdatedAt:   item.GithubSyncedIssueUpdatedAt,
		UserModifiedFields:           item.UserModifiedFields,
		ShippedFileStats:             item.ShippedFileStats,
		ShippedSnapshotCaptureFailed: item.ShippedSnapshotCaptureFailed,
		ReworkCapOverride:            item.ReworkCapOverride,
		NextWorkflowID:               item.NextWorkflowID,
		ChainFired:                   item.ChainFired,
		ChainedAt:                    item.ChainedAt,
		TriggeredByChainDepth:        item.TriggeredByChainDepth,
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
	// Propagate eagerly-loaded activity notes when present (see StatusEvents above).
	if item.Edges.ActivityNotes != nil {
		data.ActivityNotes = make([]ActivityNoteData, len(item.Edges.ActivityNotes))
		for i, n := range item.Edges.ActivityNotes {
			data.ActivityNotes[i] = activityNoteToData(n)
		}
	}
	// Propagate eagerly-loaded item sessions when present. Note: the
	// ReviewVerdict sub-edge must ALSO have been loaded on each item.Edges.ItemSessions[i]
	// (e.g. via .WithItemSessions(func(q) { q.WithReviewVerdict() })) for
	// gateVerdict/gateVerdictSummary/triageStatus — which the frontend derives
	// entirely from ItemSessions (web-app/src/lib/hooks/useBacklogService.ts's
	// mapBacklogItem) — to come through non-empty. Most callers that need this
	// populated correctly go through attachItemSessionsForPublish instead of
	// relying on this edge being loaded on the entity passed in here.
	if item.Edges.ItemSessions != nil {
		data.ItemSessions = make([]ItemSessionSummary, len(item.Edges.ItemSessions))
		for i, is := range item.Edges.ItemSessions {
			s := itemSessionToSummary(is)
			s.BacklogItemID = data.ID
			data.ItemSessions[i] = s
		}
	}
	return data
}

func itemSourceToData(src *ent.ItemSource) ItemSourceData {
	data := ItemSourceData{
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

	// Story 1.1 AC: every newly created backlog item is minted a
	// BacklogItemID at create time, so only rows that pre-date this feature
	// ever need Story 1.4's backfill.
	publicID, err := NewBacklogItemID()
	if err != nil {
		return nil, fmt.Errorf("failed to generate backlog item public id: %w", err)
	}

	c := r.client.BacklogItem.Create().
		SetTitle(data.Title).
		SetPublicID(publicID.String()).
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
		SetCategory(data.Category).
		SetPlanApproved(data.PlanApproved).
		SetNillablePlanApprovedAt(data.PlanApprovedAt).
		SetNillableQueuedAt(data.QueuedAt).
		SetQueuedAutonomous(data.QueuedAutonomous).
		SetNillablePlanArtifactsPath(&data.PlanArtifactsPath).
		SetNillablePlanRejectionReason(&data.PlanRejectionReason).
		SetNillablePlanRejectedAt(data.PlanRejectedAt).
		SetNillableNotes(&data.Notes).
		SetNillableExternalID(&data.ExternalID).
		SetNillableExternalURL(&data.ExternalURL).
		SetLabels(data.Labels).
		SetNillableArchivedAt(data.ArchivedAt).
		SetNillableReworkCapOverride(data.ReworkCapOverride).
		SetNillableGithubSyncedIssueUpdatedAt(data.GitHubSyncedIssueUpdatedAt)

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

// GetBacklogItem retrieves a backlog item by either its legacy UUID string
// or its bl_-prefixed public id (see resolveBacklogItemLookup).
func (r *EntRepository) GetBacklogItem(ctx context.Context, id string) (*BacklogItemData, error) {
	parsedID, err := r.resolveBacklogItemLookup(ctx, id)
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
		WithActivityNotes(func(q *ent.BacklogActivityNoteQuery) {
			q.Order(ent.Asc(backlogactivitynote.FieldCreatedAt))
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

// GetRepoPathAndLatestCompletedWorkSessionCommits returns itemID's RepoPath
// plus the Base/LastCommitSha of its most recent completed (session_role ==
// work, ended_at set) ItemSession — the minimal data
// Storage.ComputeCurrentDiffHash needs. Unlike GetBacklogItem (which also
// eager-loads StatusEvents/ProgressNotes) and ListItemSessions (unbounded,
// eager-loads ReviewVerdict for every session), this pushes the "most recent
// completed work session" filter/order/limit into SQL — two bounded, no-edge
// queries regardless of how many sessions/events/notes the item has
// accumulated, rather than O(sessions_for_item) work on every review-verdict
// save. baseSHA/headSHA are both "" (no error) when the item has no
// completed work session yet.
func (r *EntRepository) GetRepoPathAndLatestCompletedWorkSessionCommits(ctx context.Context, itemID string) (repoPath, baseSHA, headSHA string, err error) {
	parsedID, err := r.resolveBacklogItemLookup(ctx, itemID)
	if err != nil {
		return "", "", "", fmt.Errorf("%w: invalid id %q: %v", ErrNotFound, itemID, err)
	}

	item, err := r.client.BacklogItem.Query().
		Where(backlogitem.ID(parsedID)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return "", "", "", fmt.Errorf("%w: backlog item %s", ErrNotFound, itemID)
		}
		return "", "", "", fmt.Errorf("failed to get backlog item %s: %w", itemID, err)
	}

	ws, err := r.client.ItemSession.Query().
		Where(
			itemsession.HasBacklogItemWith(backlogitem.ID(parsedID)),
			itemsession.SessionRoleEQ(SessionRoleWork),
			itemsession.EndedAtNotNil(),
		).
		Order(ent.Desc(itemsession.FieldCreatedAt)).
		First(ctx)
	if ent.IsNotFound(err) {
		return item.RepoPath, "", "", nil
	}
	if err != nil {
		return "", "", "", fmt.Errorf("failed to query latest completed work session for item %s: %w", itemID, err)
	}
	return item.RepoPath, ws.BaseCommitSha, ws.LastCommitSha, nil
}

// AddBacklogItemDependency records that edge.BlockedID may not be
// dequeued/started until edge.BlockerID reaches a resolved status — see
// UnresolvedBlockerItemIDs for what counts as resolved (done or archived).
// Adding an already-existing pair is a no-op (upsert against the unique
// (blocker_id, blocked_id) index). Returns ErrDependencyCycle if the new
// edge would create a cycle, including the degenerate self-dependency case
// (BlockerID == BlockedID).
func (r *EntRepository) AddBacklogItemDependency(ctx context.Context, edge BacklogItemDependencyEdge) error {
	blockerID, err := r.resolveBacklogItemLookup(ctx, edge.BlockerID)
	if err != nil {
		return fmt.Errorf("%w: invalid blocker id %q: %v", ErrNotFound, edge.BlockerID, err)
	}
	blockedID, err := r.resolveBacklogItemLookup(ctx, edge.BlockedID)
	if err != nil {
		return fmt.Errorf("%w: invalid blocked id %q: %v", ErrNotFound, edge.BlockedID, err)
	}
	if blockerID == blockedID {
		return ErrDependencyCycle
	}

	// The existence check, cycle check, and insert must all observe the same
	// consistent snapshot of the dependency graph and commit atomically —
	// otherwise two concurrent calls adding opposite-direction edges can each
	// pass the cycle check before either commits, together closing a cycle
	// the check was supposed to prevent (TOCTOU race). Run everything inside
	// one transaction, following the pattern established by MarkStuck above.
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("add backlog item dependency: begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	existingIDs, err := tx.BacklogItem.Query().
		Where(backlogitem.IDIn(blockerID, blockedID)).
		IDs(ctx)
	if err != nil {
		return fmt.Errorf("failed to verify backlog items exist for dependency %s -> %s: %w", edge.BlockerID, edge.BlockedID, err)
	}
	found := make(map[uuid.UUID]bool, len(existingIDs))
	for _, id := range existingIDs {
		found[id] = true
	}
	if !found[blockerID] {
		return fmt.Errorf("%w: blocker item %s does not exist", ErrNotFound, edge.BlockerID)
	}
	if !found[blockedID] {
		return fmt.Errorf("%w: blocked item %s does not exist", ErrNotFound, edge.BlockedID)
	}

	wouldCycle, err := dependencyReachable(ctx, tx.BacklogItemDependency, blockedID, blockerID)
	if err != nil {
		return fmt.Errorf("failed to check backlog item dependency cycle: %w", err)
	}
	if wouldCycle {
		return ErrDependencyCycle
	}

	err = tx.BacklogItemDependency.Create().
		SetBlockerID(blockerID).
		SetBlockedID(blockedID).
		OnConflictColumns(backlogitemdependency.FieldBlockerID, backlogitemdependency.FieldBlockedID).
		DoNothing().
		Exec(ctx)
	// On SQLite, DO NOTHING on a conflicting row leaves the RETURNING clause
	// with no row to return, which the driver surfaces as sql.ErrNoRows
	// rather than a constraint error — treat it the same as the no-op it is.
	if err != nil && !ent.IsConstraintError(err) && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("failed to add backlog item dependency %s -> %s: %w", edge.BlockerID, edge.BlockedID, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("add backlog item dependency: commit: %w", err)
	}
	return nil
}

// dependencyReachable reports whether target is reachable from start by
// following existing blocker->blocked edges forward (a breadth-first
// traversal of "what does this item block, transitively"). Used to detect
// whether adding a new blocker->blocked edge would close a cycle: the new
// edge closes a cycle exactly when blocked (start) can already reach blocker
// (target) through existing edges. Takes the BacklogItemDependency client
// explicitly (rather than reading r.client) so callers can pass a
// transaction's client and have the traversal observe that transaction's
// consistent snapshot instead of racing concurrent commits.
func dependencyReachable(ctx context.Context, client *ent.BacklogItemDependencyClient, start, target uuid.UUID) (bool, error) {
	visited := map[uuid.UUID]bool{start: true}
	frontier := []uuid.UUID{start}
	for len(frontier) > 0 {
		rows, err := client.Query().
			Where(backlogitemdependency.BlockerIDIn(frontier...)).
			All(ctx)
		if err != nil {
			return false, err
		}
		var next []uuid.UUID
		for _, row := range rows {
			if row.BlockedID == target {
				return true, nil
			}
			if !visited[row.BlockedID] {
				visited[row.BlockedID] = true
				next = append(next, row.BlockedID)
			}
		}
		frontier = next
	}
	return false, nil
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
			return nil, fmt.Errorf("%w: invalid id %q: %v", ErrNotFound, id, err)
		}
		parsedByString[parsed] = id
		parsedIDs = append(parsedIDs, parsed)
	}

	rows, err := r.client.BacklogItemDependency.Query().
		Where(
			backlogitemdependency.BlockedIDIn(parsedIDs...),
			backlogitemdependency.HasBlockerWith(
				backlogitem.StatusNotIn(string(BacklogStatusDone), string(BacklogStatusArchived)),
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
		return nil, fmt.Errorf("%w: invalid id %q: %v", ErrNotFound, itemID, err)
	}

	rows, err := r.client.BacklogItemDependency.Query().
		Where(
			backlogitemdependency.BlockedIDEQ(parsed),
			backlogitemdependency.HasBlockerWith(
				backlogitem.StatusNotIn(string(BacklogStatusDone), string(BacklogStatusArchived)),
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

	result := make([]BacklogItemData, len(items))
	idIndex := make(map[string]int, len(items))
	parsedUUIDs := make([]uuid.UUID, 0, len(items))
	for i, item := range items {
		result[i] = backlogItemToData(item)
		idIndex[item.ID.String()] = i
		parsedUUIDs = append(parsedUUIDs, item.ID)
	}
	if len(parsedUUIDs) == 0 {
		return result, nil
	}

	// Eager-load item sessions via a single batched query, matching
	// ListBacklogItemSummaries's pattern, so WatchBacklogItems' fresh-connection
	// snapshot branch never publishes items with dropped ItemSessions.
	sessions, err := r.client.ItemSession.Query().
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
		result[idx].ItemSessions = append(result[idx].ItemSessions, itemSessionToSummary(s))
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
			ExternalURL:        item.ExternalURL,
			Labels:             item.Labels,
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
	parsedID, err := r.resolveBacklogItemLookup(ctx, id)
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
	if update.Category != nil {
		u.SetCategory(*update.Category)
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
	if update.QueuedAt != nil {
		u.SetQueuedAt(*update.QueuedAt)
	}
	if update.QueuedAutonomous != nil {
		u.SetQueuedAutonomous(*update.QueuedAutonomous)
	}
	if update.PlanArtifactsPath != nil {
		u.SetPlanArtifactsPath(*update.PlanArtifactsPath)
	}
	if update.PlanRejectionReason != nil {
		u.SetPlanRejectionReason(*update.PlanRejectionReason)
	}
	if update.ClearPlanRejectedAt {
		u.ClearPlanRejectedAt()
	} else if update.PlanRejectedAt != nil {
		u.SetPlanRejectedAt(*update.PlanRejectedAt)
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
	if update.ClearPrFeedbackAddressedAt {
		u.ClearPrFeedbackAddressedAt()
	} else if update.PrFeedbackAddressedAt != nil {
		u.SetPrFeedbackAddressedAt(*update.PrFeedbackAddressedAt)
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
	if update.ExternalURL != nil {
		u.SetExternalURL(*update.ExternalURL)
	}
	if update.Labels != nil {
		u.SetLabels(*update.Labels)
	}
	if update.ClearGitHubSyncedIssueUpdatedAt {
		u.ClearGithubSyncedIssueUpdatedAt()
	} else if update.GitHubSyncedIssueUpdatedAt != nil {
		u.SetGithubSyncedIssueUpdatedAt(*update.GitHubSyncedIssueUpdatedAt)
	}
	if update.UserModifiedFields != nil {
		u.SetUserModifiedFields(*update.UserModifiedFields)
	}
	if update.ClearNextWorkflowID {
		u.ClearNextWorkflowID()
	} else if update.NextWorkflowID != nil {
		u.SetNextWorkflowID(*update.NextWorkflowID)
	}
	if update.ChainFired != nil {
		u.SetChainFired(*update.ChainFired)
	}
	if update.ClearChainedAt {
		u.ClearChainedAt()
	} else if update.ChainedAt != nil {
		u.SetChainedAt(*update.ChainedAt)
	}
	if update.TriggeredByChainDepth != nil {
		u.SetTriggeredByChainDepth(*update.TriggeredByChainDepth)
	}

	item, err := u.Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to update backlog item %s: %w", id, err)
	}
	result := backlogItemToData(item)

	// Best-effort publish: never blocks or fails the update itself.
	r.publishItemChanged(ctx, &result, BacklogItemChange{
		Kind:          ChangeItemUpdated,
		UpdatedFields: updatedFieldsFromBacklogItemUpdate(update),
	})

	return &result, nil
}

// updatedFieldsFromBacklogItemUpdate computes which fields a BacklogItemUpdate
// actually sets (non-nil pointer params), for the ChangeItemUpdated publish
// hook (Story 2.2.1). Field names use lowerCamelCase to match the JSON/proto
// naming convention used elsewhere on the wire. A call with every field left
// nil (a no-op update) returns an empty, non-nil slice rather than fabricating
// any field name.
func updatedFieldsFromBacklogItemUpdate(update BacklogItemUpdate) []string {
	fields := make([]string, 0, 24)
	if update.Title != nil {
		fields = append(fields, "title")
	}
	if update.Description != nil {
		fields = append(fields, "description")
	}
	if update.AcceptanceCriteria != nil {
		fields = append(fields, "acceptanceCriteria")
	}
	if update.Priority != nil {
		fields = append(fields, "priority")
	}
	if update.RepoPath != nil {
		fields = append(fields, "repoPath")
	}
	if update.SkipReviewGate != nil {
		fields = append(fields, "skipReviewGate")
	}
	if update.SkipPlanning != nil {
		fields = append(fields, "skipPlanning")
	}
	if update.AutoSpawnSession != nil {
		fields = append(fields, "autoSpawnSession")
	}
	if update.AutoCreatePR != nil {
		fields = append(fields, "autoCreatePR")
	}
	if update.PipelineMode != nil {
		fields = append(fields, "pipelineMode")
	}
	if update.Category != nil {
		fields = append(fields, "category")
	}
	if update.Notes != nil {
		fields = append(fields, "notes")
	}
	if update.PlanApproved != nil {
		fields = append(fields, "planApproved")
	}
	if update.PlanApprovedAt != nil {
		fields = append(fields, "planApprovedAt")
	}
	if update.QueuedAt != nil {
		fields = append(fields, "queuedAt")
	}
	if update.QueuedAutonomous != nil {
		fields = append(fields, "queuedAutonomous")
	}
	if update.PlanArtifactsPath != nil {
		fields = append(fields, "planArtifactsPath")
	}
	if update.PlanRejectionReason != nil {
		fields = append(fields, "planRejectionReason")
	}
	if update.PlanRejectedAt != nil || update.ClearPlanRejectedAt {
		fields = append(fields, "planRejectedAt")
	}
	if update.PrURL != nil {
		fields = append(fields, "prUrl")
	}
	if update.PrNumber != nil {
		fields = append(fields, "prNumber")
	}
	if update.ShippedCheckConclusion != nil {
		fields = append(fields, "shippedCheckConclusion")
	}
	if update.ShippedApprovedCount != nil {
		fields = append(fields, "shippedApprovedCount")
	}
	if update.ShippedChangesReqCount != nil {
		fields = append(fields, "shippedChangesReqCount")
	}
	if update.ShippedSnapshotAt != nil {
		fields = append(fields, "shippedSnapshotAt")
	}
	if update.PrFeedbackAddressedAt != nil || update.ClearPrFeedbackAddressedAt {
		fields = append(fields, "prFeedbackAddressedAt")
	}
	if update.ShippedFileStats != nil {
		fields = append(fields, "shippedFileStats")
	}
	if update.ShippedSnapshotCaptureFailed != nil {
		fields = append(fields, "shippedSnapshotCaptureFailed")
	}
	if update.ReworkCapOverride != nil {
		fields = append(fields, "reworkCapOverride")
	}
	if update.ExternalURL != nil {
		fields = append(fields, "externalUrl")
	}
	if update.Labels != nil {
		fields = append(fields, "labels")
	}
	if update.GitHubSyncedIssueUpdatedAt != nil || update.ClearGitHubSyncedIssueUpdatedAt {
		fields = append(fields, "gitHubSyncedIssueUpdatedAt")
	}
	if update.UserModifiedFields != nil {
		fields = append(fields, "userModifiedFields")
	}
	if update.NextWorkflowID != nil || update.ClearNextWorkflowID {
		fields = append(fields, "nextWorkflowId")
	}
	if update.ChainFired != nil {
		fields = append(fields, "chainFired")
	}
	if update.ChainedAt != nil || update.ClearChainedAt {
		fields = append(fields, "chainedAt")
	}
	if update.TriggeredByChainDepth != nil {
		fields = append(fields, "triggeredByChainDepth")
	}
	return fields
}

// ArchiveBacklogItem sets the archived_at timestamp on a backlog item.
func (r *EntRepository) ArchiveBacklogItem(ctx context.Context, id string) (*BacklogItemData, error) {
	parsedID, err := r.resolveBacklogItemLookup(ctx, id)
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

	recordStatusEvent(ctx, r.client.BacklogStatusEvent, parsedID, current.Status, string(BacklogStatusArchived), TriggeredByUser, "")

	result := backlogItemToData(item)

	// Best-effort publish: never blocks or fails the archive itself.
	r.publishItemChanged(ctx, &result, BacklogItemChange{
		Kind:       ChangeItemArchived,
		ArchivedAt: result.ArchivedAt,
	})

	return &result, nil
}

// UnarchiveBacklogItem clears archived_at and restores the item to the
// "idea" status — the sole valid archived-> reopen transition. It does not
// attempt to restore whatever status the item held before archiving (no
// history-based restoration): the item simply re-enters the idea column
// needing a fresh session, matching UnarchiveSession's identical
// unconditional-flip precedent (server/services/session_service.go) rather
// than erroring or no-op'ing on an already-non-archived item.
func (r *EntRepository) UnarchiveBacklogItem(ctx context.Context, id string) (*BacklogItemData, error) {
	parsedID, err := r.resolveBacklogItemLookup(ctx, id)
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
		ClearArchivedAt().
		SetStatus(string(BacklogStatusIdea)).
		SetUserModifiedStatusAt(now).
		Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf("%w: backlog item %s", ErrNotFound, id)
		}
		return nil, fmt.Errorf("failed to unarchive backlog item %s: %w", id, err)
	}

	recordStatusEvent(ctx, r.client.BacklogStatusEvent, parsedID, current.Status, string(BacklogStatusIdea), TriggeredByUser, "")

	result := backlogItemToData(item)

	// Best-effort publish: never blocks or fails the unarchive itself. A full
	// status-transition event (not a lightweight archived-only shape) so
	// "Show Archived" list views in other open tabs live-update.
	r.attachItemSessionsForPublish(ctx, &result)
	r.publishItemChanged(ctx, &result, BacklogItemChange{
		Kind:      ChangeStatusTransition,
		OldStatus: current.Status,
		NewStatus: string(BacklogStatusIdea),
	})

	return &result, nil
}

// DeleteBacklogItem permanently removes an item and all its child records.
func (r *EntRepository) DeleteBacklogItem(ctx context.Context, id string) error {
	parsedID, err := r.resolveBacklogItemLookup(ctx, id)
	if err != nil {
		return fmt.Errorf("%w: invalid id %q: %v", ErrNotFound, id, err)
	}

	// Fetch the item before deleting so a ChangeItemRemoved event can carry a
	// snapshot of it — once DeleteOneID succeeds below, the row is gone and
	// can no longer be read back.
	existing, err := r.client.BacklogItem.Get(ctx, parsedID)
	if err != nil {
		if ent.IsNotFound(err) {
			return fmt.Errorf("%w: backlog item %s", ErrNotFound, id)
		}
		return fmt.Errorf("failed to get backlog item %s: %w", id, err)
	}

	// Build the publish snapshot (including ItemSessions) now, before this
	// item's sessions are deleted below — attachItemSessionsForPublish's
	// ListItemSessions query would return nothing once they're gone.
	result := backlogItemToData(existing)
	r.attachItemSessionsForPublish(ctx, &result)

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

	// Best-effort publish: never blocks or fails the delete itself. result was
	// built above, before the item's sessions were deleted — use
	// publishItemChangedSnapshot directly (not publishItemChanged) so this
	// pre-captured snapshot isn't clobbered by a re-query that would now find
	// no sessions.
	r.publishItemChangedSnapshot(&result, BacklogItemChange{
		Kind: ChangeItemRemoved,
	})

	return nil
}

// TransitionBacklogItemStatus changes the status of a backlog item with
// optional precondition.
//
// The precondition is enforced as a genuine SQL-level compare-and-swap: it is
// folded into the UPDATE statement's own WHERE clause (status = ? AND
// updated_at = ?, via the bulk Update().Where(...) builder — not
// UpdateOneID, which cannot scope beyond id) rather than checked against a
// separately-fetched row beforehand. Save() on a bulk update returns the
// affected row count instead of the updated entity, so the row is re-fetched
// after a successful write.
//
// The previous implementation did Get() → check-in-Go → UpdateOneID().Save(),
// a read-then-write race: nothing stopped a second, concurrent caller's write
// from landing in the gap between this call's read and its write, so a
// precondition that was true at read time could be false (and silently
// ignored) by write time. Two concrete incidents motivated closing this: a
// stale AutoReopenAfterFailedReview call reopened an item that had, in the
// meantime, already legitimately shipped to "done" (see
// docs/bugs/fixed/BUG-026-backlog-transition-status-toctou-reopen.md, live
// 2026-07-20 repro, item 0fd4a940, PR #176), and the backlog work-item queue
// feature's concurrent-dequeue-claim test found the same race could
// double-claim a single queued item between two dequeue sweeps (PR #199).
func (r *EntRepository) TransitionBacklogItemStatus(ctx context.Context, id string, toStatus BacklogStatus, precondition *BacklogItemPrecondition, triggeredBy string) (*BacklogItemData, error) {
	parsedID, err := r.resolveBacklogItemLookup(ctx, id)
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
	setter := update.
		SetStatus(string(toStatus)).
		SetUserModifiedStatusAt(now)
	// webhook-triggers Epic 6.1 (AC5/AC9): chained_at is persisted atomically
	// with the terminal done transition, in the same UPDATE, whenever a chain
	// is already configured (next_workflow_id was set at chain-configuration
	// time — see NextWorkflowID's doc comment) — this is the crash-consistent
	// eligibility timestamp TriggerChainReconciler's maxChainWaitDuration
	// ceiling measures age against, independent of whether/when the fire
	// itself later succeeds. current (fetched above, before this write) is the
	// only place NextWorkflowID is visible before this transition's own SELECT
	// on affected==0 below.
	if toStatus == BacklogStatusDone && current.NextWorkflowID != nil {
		setter = setter.SetChainedAt(now)
	}
	affected, err := setter.Save(ctx)
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
	note := ""
	if precondition != nil {
		note = precondition.Note
	}
	recordStatusEvent(ctx, r.client.BacklogStatusEvent, parsedID, fromStatus, string(toStatus), triggeredBy, note)

	result := backlogItemToData(item)

	// Best-effort publish: never blocks or fails the transition itself.
	r.publishItemChanged(ctx, &result, BacklogItemChange{
		Kind:      ChangeStatusTransition,
		OldStatus: fromStatus,
		NewStatus: string(toStatus),
	})

	// AC4 (webhook-triggers Phase 5): fire the on_session_complete callback after
	// the transition has already committed and this function is about to return —
	// dispatchCallback is itself non-blocking (bounded semaphore + go), so this
	// adds no latency to the caller and a delivery failure can never roll back or
	// corrupt the status transition above.
	if toStatus == BacklogStatusDone {
		r.dispatchCallback("session_complete", map[string]any{
			"event":       "session_complete",
			"item_id":     result.ID,
			"title":       result.Title,
			"status":      string(toStatus),
			"occurred_at": time.Now(),
		})

		// AC5/AC9 (webhook-triggers Phase 6): dispatch the pipeline-chain
		// continuation, same non-blocking shape and same "after the write
		// already committed, right before this function returns" placement as
		// the on_session_complete callback dispatch above — dispatchChainFire
		// itself launches a goroutine (bounded semaphore + go, see
		// ChainFirer.Dispatch), so this call returns immediately and the
		// expensive CreateSession work for the chained session happens well
		// after TransitionBacklogItemStatus has already returned to its own
		// caller (AC9's exact requirement).
		if result.NextWorkflowID != nil && !result.ChainFired {
			r.dispatchChainFire(&result)
		}
	}

	return &result, nil
}

// ClaimChainFire atomically claims item id's pipeline chain-fire attempt by
// flipping chain_fired from false to true, conditioned on both chain_fired
// still being false and updated_at still matching expectedUpdatedAt — a
// genuine SQL-level compare-and-swap folded into the UPDATE's own WHERE
// clause, the same pattern (and for the identical reason) as
// TransitionBacklogItemStatus's precondition handling above (see that
// method's doc comment, BUG-026): a Get-then-check-in-Go-then-write sequence
// (which UpdateBacklogItem's precondition parameter still is, deliberately
// not reused here) leaves a race window wide enough for two concurrent
// callers to both pass the check and both issue their own write. Used by
// ChainFirer.Fire to guarantee at most one goroutine ever reaches
// TriggerFirer.FireTriggerChained for a given item (webhook-triggers Task
// 6.2.1d — the happy-path async dispatch and TriggerChainReconciler's
// periodic sweep can otherwise race on the same item).
//
// Returns claimed=false (not an error) when another caller already won the
// claim or the row's updated_at moved for any other reason — callers must
// treat that as "someone else is handling this," not a failure.
func (r *EntRepository) ClaimChainFire(ctx context.Context, id string, expectedUpdatedAt time.Time) (claimed bool, err error) {
	parsedID, err := r.resolveBacklogItemLookup(ctx, id)
	if err != nil {
		return false, fmt.Errorf("%w: invalid id %q: %v", ErrNotFound, id, err)
	}

	affected, err := r.client.BacklogItem.Update().
		Where(backlogitem.ID(parsedID), backlogitem.ChainFired(false), backlogitem.UpdatedAtEQ(expectedUpdatedAt)).
		SetChainFired(true).
		Save(ctx)
	if err != nil {
		return false, fmt.Errorf("claim chain fire for item %s: %w", id, err)
	}
	if affected == 0 {
		return false, nil
	}

	// Best-effort publish: never blocks or fails the claim itself.
	if item, getErr := r.client.BacklogItem.Get(ctx, parsedID); getErr == nil {
		result := backlogItemToData(item)
		r.publishItemChanged(ctx, &result, BacklogItemChange{
			Kind:          ChangeItemUpdated,
			UpdatedFields: []string{"chainFired"},
		})
	}
	return true, nil
}

// RevertChainFireClaim unconditionally resets chain_fired back to false —
// used by ChainFirer.Fire to release its claim after a subsequent
// FireTriggerChained attempt fails, so TriggerChainReconciler retries on its
// next tick. Safe to call unconditionally (no precondition): only the
// goroutine that just won ClaimChainFire's claim for this item can ever reach
// this call, so nothing else can be racing this specific revert.
func (r *EntRepository) RevertChainFireClaim(ctx context.Context, id string) error {
	parsedID, err := r.resolveBacklogItemLookup(ctx, id)
	if err != nil {
		return fmt.Errorf("%w: invalid id %q: %v", ErrNotFound, id, err)
	}
	if _, err := r.client.BacklogItem.UpdateOneID(parsedID).SetChainFired(false).Save(ctx); err != nil {
		return fmt.Errorf("revert chain fire claim for item %s: %w", id, err)
	}
	return nil
}

// TransitionBacklogItemStatusWithPRFields atomically transitions a backlog
// item's status while also persisting its PrURL/PrNumber, as a single
// UPDATE ... WHERE statement — the same CAS precondition guards both writes
// together, so a reader can never observe the status having changed without
// the PR fields already being set, or vice versa.
//
// This exists to close a narrower race than the one
// Storage.SetBacklogItemPRAndTransition originally had: an earlier fix
// reordered that function to run the status transition (review ->
// pr_pending) before a separate PrURL/PrNumber field write, which fixed the
// lost-update bug (two racing callers clobbering each other's PR number) but
// left a smaller gap — between the transition committing and the field write
// committing, a concurrent reader could observe status=pr_pending with
// PrNumber==0. That exact shape is what the pr_pending_no_pr / BUG-040 stuck
// detector (reconcilePRPendingWithoutPRItems, session/backlog_lifecycle.go)
// exists to flag as a HIGH-priority, non-auto-recoverable alert, and its
// resolution condition (selfHealStuck) is anchored on the item leaving
// pr_pending entirely — so a reconcile tick landing in that multi-millisecond
// window could raise a spurious stuck alert that stays open for days on a
// perfectly healthy item. Folding both writes into one atomic UPDATE removes
// the window rather than narrowing it.
//
// Not part of the Repository interface — Storage type-asserts s.repo to
// *EntRepository to call this, the same pattern already used for
// ListItemSessions (see server/mcp/tools_backlog.go's listItemSessionsFn doc
// comment): EntRepository has no second real implementation to abstract
// this over, so adding it to the interface would be pure speculation
// (see .claude/rules/interface-pollution-checklist.md).
func (r *EntRepository) TransitionBacklogItemStatusWithPRFields(ctx context.Context, id string, toStatus BacklogStatus, prURL string, prNumber int, precondition *BacklogItemPrecondition, triggeredBy string) (*BacklogItemData, error) {
	parsedID, err := r.resolveBacklogItemLookup(ctx, id)
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
		SetPrURL(prURL).
		SetPrNumber(prNumber).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to transition backlog item %s status with PR fields: %w", id, err)
	}
	if affected == 0 {
		// The row either no longer exists or the precondition no longer holds —
		// re-fetch to report which. Mirrors TransitionBacklogItemStatus's
		// identical affected==0 handling.
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

	fromStatus := current.Status
	if precondition != nil && precondition.ExpectedStatus != "" {
		fromStatus = precondition.ExpectedStatus
	}
	note := ""
	if precondition != nil {
		note = precondition.Note
	}
	recordStatusEvent(ctx, r.client.BacklogStatusEvent, parsedID, fromStatus, string(toStatus), triggeredBy, note)

	result := backlogItemToData(item)

	// Best-effort publish: never blocks or fails the transition itself.
	r.publishItemChanged(ctx, &result, BacklogItemChange{
		Kind:      ChangeStatusTransition,
		OldStatus: fromStatus,
		NewStatus: string(toStatus),
	})

	return &result, nil
}

// publishItemChanged eager-loads item's ItemSessions (see
// attachItemSessionsForPublish) and then publishes via
// publishItemChangedSnapshot. This is the entry point every call site should
// use except DeleteBacklogItem, whose snapshot must be captured before the
// item's sessions are deleted and therefore cannot be re-queried here — see
// publishItemChangedSnapshot's doc comment.
func (r *EntRepository) publishItemChanged(ctx context.Context, item *BacklogItemData, change BacklogItemChange) {
	r.attachItemSessionsForPublish(ctx, item)
	r.publishItemChangedSnapshot(item, change)
}

// publishItemChangedSnapshot is a defense-in-depth wrapper around
// r.itemChangePublisher.PublishItemChanged: nil-checked (a publisher may not
// be wired, e.g. in tests or before server/dependencies.go calls
// SetItemChangePublisher) and recover()-guarded at the call site itself, so a
// panic can never propagate into a hooked repository method's return path —
// regardless of whether itemChangePublisher is the real adapter
// (server/services.BacklogItemEventPublisher, which already recovers inside
// its own body per Task 1.3.2b) or some other ItemChangePublisher
// implementation that doesn't. Task 2.1.1d's regression test proves this
// holds end-to-end even when a raw, unwrapped test double panics.
//
// Publishes item as-is, without eager-loading ItemSessions first. Two call
// sites use this directly: DeleteBacklogItem, which must build its publish
// snapshot (including ItemSessions) before deleting the item's sessions and
// the item itself, so re-querying at publish time (as publishItemChanged
// does) would wrongly observe them as already gone; and AppendActivityNote,
// whose ChangeActivityNoteAdded event never carries a full item snapshot
// (ADR-001/ADR-002 — see project_plans/backlog-item-activity-log), so there
// is nothing extra to eager-load beyond the Status/RepoPath it reads itself.
func (r *EntRepository) publishItemChangedSnapshot(item *BacklogItemData, change BacklogItemChange) {
	if r.itemChangePublisher == nil {
		return
	}
	defer func() {
		if rec := recover(); rec != nil {
			log.WarningLog.Printf("[EntRepository] itemChangePublisher.PublishItemChanged panicked (recovered): %v", rec)
		}
	}()
	r.itemChangePublisher.PublishItemChanged(item, change)
}

// SetCallbackDispatcher wires a CallbackDispatcher into this repository so
// TransitionBacklogItemStatus (on_session_complete) and
// BacklogLifecycleListener.reconcileStaleWorkSessions (on_session_stale, which is
// handed this *EntRepository directly) can fire outbound callbacks
// (webhook-triggers Phase 5). Called via Storage.SetCallbackDispatcher's
// forwarding method, the same pattern SetItemChangePublisher uses.
func (r *EntRepository) SetCallbackDispatcher(d CallbackDispatcher) {
	r.callbackDispatcher = d
}

// dispatchCallback is a defense-in-depth wrapper around
// r.callbackDispatcher.Dispatch: nil-checked (a dispatcher may not be wired, e.g.
// in tests or before server/dependencies.go calls SetCallbackDispatcher) and
// recover()-guarded, mirroring publishItemChanged's shape — a panic in Dispatch
// (or any CallbackDispatcher implementation) must never propagate into a hooked
// repository method's return path. Dispatch itself is expected to be
// non-blocking (bounded semaphore + go, per CallbackDispatcher's doc comment).
func (r *EntRepository) dispatchCallback(eventType string, payload any) {
	if r.callbackDispatcher == nil {
		return
	}
	defer func() {
		if rec := recover(); rec != nil {
			log.WarningLog.Printf("[EntRepository] callbackDispatcher.Dispatch panicked (recovered): %v", rec)
		}
	}()
	r.callbackDispatcher.Dispatch(eventType, payload)
}

// SetChainFirer wires a ChainFirer into this repository so
// TransitionBacklogItemStatus can dispatch the pipeline-chain fire
// immediately after a "done" transition commits (webhook-triggers Phase 6,
// AC5/AC9). Called via Storage.WireChainFirer's forwarding call in
// server/dependencies.go, the same pattern SetCallbackDispatcher uses.
func (r *EntRepository) SetChainFirer(f *ChainFirer) {
	r.chainFirer = f
}

// dispatchChainFire is a defense-in-depth wrapper around r.chainFirer.Dispatch:
// nil-checked (a ChainFirer may not be wired, e.g. in tests or before
// server/dependencies.go calls SetChainFirer) and recover()-guarded, mirroring
// dispatchCallback's shape — a panic in Dispatch (which is itself already
// non-blocking: bounded semaphore + go, see ChainFirer.Dispatch) must never
// propagate into TransitionBacklogItemStatus's own return path.
//
// ChainFirer.Dispatch itself takes no ctx (see its doc comment) — it always
// derives its own context.WithTimeout(context.Background(), chainFireTimeout)
// for the goroutine it spawns, because by the time that goroutine's
// CreateSession call actually runs, the caller's own ctx (e.g. an RPC
// handler's request-scoped context) may already be cancelled — AC9 requires
// this fire to survive past the transition call's own lifetime, not be tied
// to it.
func (r *EntRepository) dispatchChainFire(item *BacklogItemData) {
	if r.chainFirer == nil {
		return
	}
	defer func() {
		if rec := recover(); rec != nil {
			log.WarningLog.Printf("[EntRepository] chainFirer.Dispatch panicked (recovered): %v", rec)
		}
	}()
	r.chainFirer.Dispatch(item)
}

// attachItemSessionsForPublish best-effort loads and attaches this item's
// ItemSessions (with ReviewVerdict) onto data before it's handed to
// publishItemChanged.
//
// Every publish-hook call site builds its BacklogItemData from a query or
// mutation (Get/UpdateOneID().Save()) that does NOT eager-load the
// item_sessions edge — unlike the REST list/detail read paths
// (ListBacklogItemSummaries, GetBacklogItem's sibling patterns), which do.
// backlogItemToProto (server/services/backlog_service.go) only populates the
// wire ItemSessions field "when eagerly loaded", and the frontend derives
// gateVerdict/gateVerdictSummary/triageStatus entirely from itemSessions
// (web-app/src/lib/hooks/useBacklogService.ts's mapBacklogItem) — so without
// this, every live BacklogItemEvent would silently blank those fields in the
// Redux store (backlogItemsSlice's upsertItem does a wholesale replace, not a
// field merge) even though the initial REST snapshot always carries them
// correctly. This was a confirmed regression found by a Phase 5
// spec-compliance sweep (docs/tasks/backlog-feature-improvement.md) — actively
// worse than the pre-event-driven shouldPoll baseline.
//
// Reuses ListItemSessions (the same tested query the REST list/detail paths
// already run) so the live-event snapshot and the REST snapshot are always
// built from identical data, rather than duplicating the edge-loading query
// at each of the ~10 call sites. Best-effort: a lookup failure is logged and
// skipped, never fails the mutation that already succeeded.
func (r *EntRepository) attachItemSessionsForPublish(ctx context.Context, data *BacklogItemData) {
	if data == nil || data.ID == "" {
		return
	}
	sessions, err := r.ListItemSessions(ctx, data.ID)
	if err != nil {
		log.WarningLog.Printf("[EntRepository] attachItemSessionsForPublish: failed to load item sessions for item %s: %v", data.ID, err)
		return
	}
	data.ItemSessions = sessions
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
	parsedID, err := r.resolveBacklogItemLookup(ctx, itemID)
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
	parsedID, err := r.resolveBacklogItemLookup(ctx, itemID)
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
	parsedID, err := r.resolveBacklogItemLookup(ctx, itemID)
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
	parsedID, err := r.resolveBacklogItemLookup(ctx, itemID)
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
	parsedID, err := r.resolveBacklogItemLookup(ctx, itemID)
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
	parsedID, err := r.resolveBacklogItemLookup(ctx, itemID)
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
	parsedID, err := r.resolveBacklogItemLookup(ctx, itemID)
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
	parsedItemID, err := r.resolveBacklogItemLookup(ctx, itemID)
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
	parsedItemID, err := r.resolveBacklogItemLookup(ctx, itemID)
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

// --- Activity note history (ADR-001) ---

// mapAppendActivityNoteCreateError classifies an error returned from
// AppendActivityNote's BacklogActivityNote.Create().Save(ctx) call. A
// foreign-key-violation constraint error means itemID was deleted between
// AppendActivityNote's earlier existence check and this Create() call (a
// real but non-deterministic-to-test race) and is wrapped as ErrNotFound so
// callers (post_backlog_update, via Storage.AppendActivityNote) can map it to
// ErrItemNotFound the same way the earlier, more common not-found path is
// mapped. Any other error is wrapped as a plain internal error. Extracted as
// its own function (rather than inlined in AppendActivityNote) specifically
// so this classification can be unit-tested without reproducing the
// underlying delete-between-steps race.
func mapAppendActivityNoteCreateError(err error, itemID string) error {
	if ent.IsConstraintError(err) {
		return fmt.Errorf("backlog item %s not found: %w", itemID, ErrNotFound)
	}
	return fmt.Errorf("failed to append activity note for item %s: %w", itemID, err)
}

// AppendActivityNote records a single post_backlog_update call as an
// immutable, append-only history entry. This is the one activity-note write
// path invoked by the ungated post_backlog_update tool — any session, with
// or without STAPLER_SESSION_UUID, whether or not linked to the item, can
// call it. Unlike AppendProgressNote, this DOES publish an item-changed
// event: there is no separate criterion-status write alongside it to carry
// that responsibility, so if AppendActivityNote didn't publish, a live
// WatchBacklogItems subscriber would never learn a note was posted.
//
// Before publishing, this does a cheap, read-only, field-scoped query for
// the item's Status and RepoPath and populates them on the published
// snapshot. This exists solely so backlogItemMatchesFilters
// (server/services/backlog_service_events.go) doesn't silently drop this
// event for WatchBacklogItems callers with a non-empty status/category
// filter — see Blocker 2 in implementation/adversarial-review.md. It is a
// read, not a read-modify-write, so it introduces no new concurrency hazard.
// publishItemChangedSnapshot (not the publishItemChanged convenience
// wrapper) is used deliberately: this event's payload never carries a full
// item snapshot (ADR-002), so there is nothing for attachItemSessionsForPublish
// to usefully eager-load beyond the two scalar fields already fetched below.
func (r *EntRepository) AppendActivityNote(ctx context.Context, itemID, authorSessionUUID, authorSessionTitle, message string) error {
	parsedItemID, err := r.resolveBacklogItemLookup(ctx, itemID)
	if err != nil {
		return fmt.Errorf("invalid item id %q: %w", itemID, err)
	}

	itemFields, err := r.client.BacklogItem.Query().
		Where(backlogitem.ID(parsedItemID)).
		Select(backlogitem.FieldStatus, backlogitem.FieldRepoPath).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return fmt.Errorf("backlog item %s not found: %w", itemID, ErrNotFound)
		}
		return fmt.Errorf("failed to look up backlog item %s: %w", itemID, err)
	}

	created, err := r.client.BacklogActivityNote.Create().
		SetItemID(parsedItemID).
		SetMessage(message).
		SetAuthorSessionUUID(authorSessionUUID).
		SetAuthorSessionTitle(authorSessionTitle).
		Save(ctx)
	if err != nil {
		return mapAppendActivityNoteCreateError(err, itemID)
	}

	note := activityNoteToData(created)
	r.publishItemChangedSnapshot(&BacklogItemData{
		ID:       parsedItemID.String(),
		Status:   itemFields.Status,
		RepoPath: itemFields.RepoPath,
	}, BacklogItemChange{Kind: ChangeActivityNoteAdded, ActivityNote: &note})
	return nil
}

// ListActivityNotesForItem returns the full append-only activity-note history
// for a backlog item, ordered by created_at ascending (oldest first).
func (r *EntRepository) ListActivityNotesForItem(ctx context.Context, itemID string) ([]ActivityNoteData, error) {
	parsedItemID, err := r.resolveBacklogItemLookup(ctx, itemID)
	if err != nil {
		return nil, fmt.Errorf("invalid item id %q: %w", itemID, err)
	}

	notes, err := r.client.BacklogActivityNote.Query().
		Where(backlogactivitynote.HasItemWith(backlogitem.ID(parsedItemID))).
		Order(ent.Asc(backlogactivitynote.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list activity notes for item %s: %w", itemID, err)
	}
	result := make([]ActivityNoteData, len(notes))
	for i, n := range notes {
		result[i] = activityNoteToData(n)
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
		SetForwardSyncEnabled(data.ForwardSyncEnabled).
		SetBackwardSyncEnabled(data.BackwardSyncEnabled).
		SetNillableForwardSyncCloseLabel(&data.ForwardSyncCloseLabel).
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

// GetBacklogItemsByExternalIDs batches GetBacklogItemByExternalID's lookup
// across many external IDs at once (a single IN query rather than one query
// per ID), scoped to sourceID the same way. Used by
// SyncLoop.PreviewBackwardSyncImpact, which previously issued one
// GetBacklogItemByExternalID call per closed issue in a loop — an N+1 query
// pattern against an index that isn't composite with the source FK. Returns
// a map keyed by external_id; IDs with no matching local item are simply
// absent from the map (not an error), matching the "not locally-imported,
// exclude it" semantics the per-item lookup had.
func (r *EntRepository) GetBacklogItemsByExternalIDs(ctx context.Context, sourceID string, externalIDs []string) (map[string]*ent.BacklogItem, error) {
	result := make(map[string]*ent.BacklogItem, len(externalIDs))
	if len(externalIDs) == 0 {
		return result, nil
	}

	parsedSourceID, err := uuid.Parse(sourceID)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid source id %q: %v", ErrNotFound, sourceID, err)
	}

	items, err := r.client.BacklogItem.Query().
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

// RecordSourceSyncFailure persists a zero-item sync-history row recording a
// forward-sync failure (e.g. the forward-sync EventBus subscriber's CloseIssue
// call erroring — see server/services/backlog_github_forward_sync.go), so the
// failure is queryable via ListSourceSyncEvents / the Settings UI's row-level
// warning (Story 4.3.2) instead of only appearing in server logs. Mirrors
// CreateSourceSyncEvent's error-message convention used by SyncOne's own
// fetch-failure path, but for the write direction (forward sync) rather than
// the read direction (Fetch).
func (r *EntRepository) RecordSourceSyncFailure(ctx context.Context, sourceID string, message string) error {
	now := time.Now()
	return r.CreateSourceSyncEvent(ctx, sourceID, "", 0, 0, 0, 1, message, now, now)
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
