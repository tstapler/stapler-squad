package session

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/backlogstore"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/backlogitem"
	"github.com/tstapler/stapler-squad/session/ent/backlogstage"
	"github.com/tstapler/stapler-squad/session/ent/stagetransition"
	"github.com/tstapler/stapler-squad/session/instance"
)

// statusEventInput bundles recordStatusEvent's write fields — kept as one
// struct rather than a long same-typed-string parameter list (see the
// primitive-obsession-checklist skill).
type statusEventInput struct {
	evClient *ent.BacklogStatusEventClient
	itemID   uuid.UUID
	// fromStatus/toStatus/triggeredBy/note/stageNameSnapshot mirror the
	// BacklogStatusEvent ent schema fields 1:1 — see that schema
	// (session/ent/schema/backlog_status_event.go) for field semantics.
	fromStatus, toStatus, triggeredBy, note, stageNameSnapshot string
	// allowedTransitionsSnapshot is ADR-004's sibling snapshot to
	// stageNameSnapshot — see resolveStageSnapshotFields.
	allowedTransitionsSnapshot []string
}

// recordStatusEvent appends an immutable BacklogStatusEvent audit row. Pass
// r.client.BacklogStatusEvent for a standalone write, or tx.BacklogStatusEvent to
// write inside an existing transaction — required when called from ReconcileStuckItems
// since the writer is capped to one connection (SetMaxOpenConns(1)) and a write through
// the non-tx client would self-deadlock. A write failure is logged, not returned: an
// audit-log gap must never block the status transition itself. in.stageNameSnapshot is
// Epic 2.5's frozen-at-entry StageConfigSnapshot name (empty when the destination
// status has no matching BacklogStage row, e.g. before seeding has run), and
// in.allowedTransitionsSnapshot is ADR-004's sibling snapshot — both are
// resolved together by resolveStageSnapshotFields.
func recordStatusEvent(ctx context.Context, in statusEventInput) {
	evCreate := in.evClient.Create().
		SetItemID(in.itemID).
		SetFromStatus(in.fromStatus).
		SetToStatus(in.toStatus).
		SetTriggeredBy(in.triggeredBy)
	if in.note != "" {
		evCreate = evCreate.SetNote(in.note)
	}
	if in.stageNameSnapshot != "" {
		evCreate = evCreate.SetStageNameSnapshot(in.stageNameSnapshot)
	}
	if in.allowedTransitionsSnapshot != nil {
		evCreate = evCreate.SetAllowedTransitionsSnapshot(in.allowedTransitionsSnapshot)
	}
	if _, err := evCreate.Save(ctx); err != nil {
		log.ErrorLog().Printf("[recordStatusEvent] failed to record item=%s %s->%s triggeredBy=%s: %v", in.itemID, in.fromStatus, in.toStatus, in.triggeredBy, err)
	}
}

// resolveStageSnapshotFields looks up the BacklogStage row matching toStatus's
// slug once, returning both its human-readable Name (Epic 2.5's
// stage_name_snapshot audit-trail column) and its enabled outgoing
// StageTransition destination slugs (ADR-004's allowed_transitions_snapshot
// sibling column) — merged from what were two independent by-slug queries so
// each recordStatusEvent call site fetches the BacklogStage row once instead
// of twice. Both values are captured at transition-write time so history
// keeps rendering the stage's original name/edges even after that row is
// later renamed, edited, or deleted, letting CanTransition/AllowedTransitions/
// PendingGates (session/configured_workflow_engine.go) fall back to an item's
// own frozen graph. Returns "", nil (never an error) when toStatus has no
// matching BacklogStage row — e.g. the EnsureBuiltInWorkflowStages seed hasn't
// run yet — since a resolution miss must never block the transition itself,
// mirroring recordStatusEvent's own best-effort discipline; allowedSlugs is an
// empty, non-nil slice when the stage exists but has zero enabled outgoing
// transitions. Takes the raw ent clients rather than *ent.Client so callers
// already inside a transaction can pass tx.BacklogStage/tx.StageTransition —
// the same "standalone client or tx client" seam recordStatusEvent's evClient
// parameter uses (see its doc comment).
func resolveStageSnapshotFields(ctx context.Context, stageClient *ent.BacklogStageClient, transitionClient *ent.StageTransitionClient, toStatus domain.BacklogStatus) (name string, allowedSlugs []string) {
	stage, err := stageClient.Query().Where(backlogstage.Slug(string(toStatus))).Only(ctx)
	if err != nil {
		return "", nil
	}
	edges, err := transitionClient.Query().
		Where(stagetransition.FromStageID(stage.ID), stagetransition.Enabled(true)).
		WithToStage().
		All(ctx)
	if err != nil {
		return stage.Name, nil
	}
	slugs := make([]string, 0, len(edges))
	for _, e := range edges {
		if e.Edges.ToStage == nil || !e.Edges.ToStage.Enabled {
			continue
		}
		slugs = append(slugs, e.Edges.ToStage.Slug)
	}
	sort.Strings(slugs)
	return stage.Name, slugs
}

// --- converters ---

// --- BacklogItem CRUD ---

// CreateBacklogItem inserts a new backlog item.
func (r *EntRepository) CreateBacklogItem(ctx context.Context, data backlogdto.BacklogItemData) (*backlogdto.BacklogItemData, error) {
	priority := data.Priority
	if priority == 0 {
		priority = 3
	}
	status := data.Status
	if status == "" {
		status = string(domain.BacklogStatusIdea)
	}

	// Story 1.1 AC: every newly created backlog item is minted a
	// BacklogItemID at create time, so only rows that pre-date this feature
	// ever need Story 1.4's backfill.
	publicID, err := backlogdto.NewBacklogItemID()
	if err != nil {
		return nil, fmt.Errorf("failed to generate backlog item public id: %w", err)
	}

	c := r.Client.BacklogItem.Create().
		SetTitle(data.Title).
		SetPublicID(publicID.String()).
		SetNillableDescription(&data.Description).
		SetNillableAcceptanceCriteria(nilIfEmptyJSON(data.AcceptanceCriteria)).
		SetPriority(priority).
		SetStatus(status).
		SetNillableRepoPath(&data.RepoPath).
		SetNillableBaseBranch(&data.BaseBranch).
		SetSkipReviewGate(data.SkipReviewGate).
		SetSkipPlanning(data.SkipPlanning).
		SetAutoSpawnSession(data.AutoSpawnSession).
		SetAutoCreatePr(data.AutoCreatePR).
		SetAutoApprovePlan(data.AutoApprovePlan).
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
		SetNillableCostBudgetThresholdUsd(data.CostBudgetThresholdUsd).
		SetNillableGithubSyncedIssueUpdatedAt(data.GitHubSyncedIssueUpdatedAt).
		SetClaimingHostID(data.ClaimingHostID).
		SetGithubLastProgressCommentStatus(data.GitHubLastProgressCommentStatus)

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
	result := backlogstore.BacklogItemToData(item)
	return &result, nil
}

// rawArchiveBacklogItem sets the archived_at timestamp and status on a backlog
// item. precondition may be nil (no CAS check — used by the pre-existing
// UI-driven callers, which archive by explicit operator action and never
// needed one). triggeredBy/note flow into the same BacklogStatusEvent audit
// trail TransitionBacklogItemStatus writes, so a non-"user" trigger (e.g. an
// agent auto-archiving a verified duplicate) is attributed correctly instead
// of always showing as a manual action — see reportDuplicate's unclaimed-item
// path in server/mcp/tools_backlog.go.
func (r *EntRepository) rawArchiveBacklogItem(ctx context.Context, id string, precondition *backlogdto.BacklogItemPrecondition, triggeredBy, note string) (*backlogdto.BacklogItemData, error) {
	parsedID, err := r.ResolveBacklogItemLookup(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid id %q: %v", domain.ErrNotFound, id, err)
	}

	current, err := r.Client.BacklogItem.Get(ctx, parsedID)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf("%w: backlog item %s", domain.ErrNotFound, id)
		}
		return nil, fmt.Errorf("failed to get backlog item %s: %w", id, err)
	}

	update := r.Client.BacklogItem.Update().Where(backlogitem.ID(parsedID))
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
		SetArchivedAt(now).
		SetStatus(string(domain.BacklogStatusArchived)).
		SetUserModifiedStatusAt(now).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to archive backlog item %s: %w", id, err)
	}
	if affected == 0 {
		// Same race disambiguation as TransitionBacklogItemStatus: re-fetch
		// against fresh data to report whether the row vanished or the
		// precondition no longer holds.
		latest, getErr := r.Client.BacklogItem.Get(ctx, parsedID)
		if getErr != nil {
			if ent.IsNotFound(getErr) {
				return nil, fmt.Errorf("%w: backlog item %s", domain.ErrNotFound, id)
			}
			return nil, fmt.Errorf("failed to get backlog item %s: %w", id, getErr)
		}
		if precondition != nil && precondition.ExpectedStatus != "" && latest.Status != precondition.ExpectedStatus {
			return nil, fmt.Errorf("%w: expected status %q, got %q", domain.ErrPreconditionFailed, precondition.ExpectedStatus, latest.Status)
		}
		return nil, fmt.Errorf("%w: updated_at mismatch", domain.ErrPreconditionFailed)
	}

	item, err := r.Client.BacklogItem.Get(ctx, parsedID)
	if err != nil {
		return nil, fmt.Errorf("failed to reload backlog item %s after archive: %w", id, err)
	}

	stageNameSnapshot, allowedTransitionsSnapshot := resolveStageSnapshotFields(ctx, r.Client.BacklogStage, r.Client.StageTransition, domain.BacklogStatusArchived)
	recordStatusEvent(ctx, statusEventInput{
		evClient:                   r.Client.BacklogStatusEvent,
		itemID:                     parsedID,
		fromStatus:                 current.Status,
		toStatus:                   string(domain.BacklogStatusArchived),
		triggeredBy:                triggeredBy,
		note:                       note,
		stageNameSnapshot:          stageNameSnapshot,
		allowedTransitionsSnapshot: allowedTransitionsSnapshot,
	})

	result := backlogstore.BacklogItemToData(item)

	// Best-effort publish: never blocks or fails the archive itself.
	r.PublishItemChanged(ctx, &result, backlogdto.BacklogItemChange{
		Kind:       backlogdto.ChangeItemArchived,
		ArchivedAt: result.ArchivedAt,
	})

	return &result, nil
}

// rawUnarchiveBacklogItem clears archived_at and restores the item to the
// "idea" status — the sole valid archived-> reopen transition. It does not
// attempt to restore whatever status the item held before archiving (no
// history-based restoration): the item simply re-enters the idea column
// needing a fresh session, matching UnarchiveSession's identical
// unconditional-flip precedent (server/services/session_service.go) rather
// than erroring or no-op'ing on an already-non-archived item.
//
// expectedStatus, when non-empty, makes the flip a compare-and-swap on the
// current status (ErrPreconditionFailed on a lost race); "" keeps it unconditional.
func (r *EntRepository) rawUnarchiveBacklogItem(ctx context.Context, id, expectedStatus string) (*backlogdto.BacklogItemData, error) {
	parsedID, err := r.ResolveBacklogItemLookup(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid id %q: %v", domain.ErrNotFound, id, err)
	}

	current, err := r.Client.BacklogItem.Get(ctx, parsedID)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf("%w: backlog item %s", domain.ErrNotFound, id)
		}
		return nil, fmt.Errorf("failed to get backlog item %s: %w", id, err)
	}

	update := r.Client.BacklogItem.Update().Where(backlogitem.ID(parsedID))
	if expectedStatus != "" {
		update = update.Where(backlogitem.StatusEQ(expectedStatus))
	}
	now := time.Now()
	affected, err := update.
		ClearArchivedAt().
		SetStatus(string(domain.BacklogStatusIdea)).
		SetUserModifiedStatusAt(now).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to unarchive backlog item %s: %w", id, err)
	}
	if affected == 0 {
		latest, getErr := r.Client.BacklogItem.Get(ctx, parsedID)
		if getErr != nil {
			if ent.IsNotFound(getErr) {
				return nil, fmt.Errorf("%w: backlog item %s", domain.ErrNotFound, id)
			}
			return nil, fmt.Errorf("failed to get backlog item %s: %w", id, getErr)
		}
		return nil, fmt.Errorf("%w: expected status %q, got %q", domain.ErrPreconditionFailed, expectedStatus, latest.Status)
	}
	item, err := r.Client.BacklogItem.Get(ctx, parsedID)
	if err != nil {
		return nil, fmt.Errorf("failed to reload backlog item %s after unarchive: %w", id, err)
	}

	stageNameSnapshot, allowedTransitionsSnapshot := resolveStageSnapshotFields(ctx, r.Client.BacklogStage, r.Client.StageTransition, domain.BacklogStatusIdea)
	recordStatusEvent(ctx, statusEventInput{
		evClient:                   r.Client.BacklogStatusEvent,
		itemID:                     parsedID,
		fromStatus:                 current.Status,
		toStatus:                   string(domain.BacklogStatusIdea),
		triggeredBy:                instance.TriggeredByUser,
		stageNameSnapshot:          stageNameSnapshot,
		allowedTransitionsSnapshot: allowedTransitionsSnapshot,
	})

	result := backlogstore.BacklogItemToData(item)

	// Best-effort publish: never blocks or fails the unarchive itself. A full
	// status-transition event (not a lightweight archived-only shape) so
	// "Show Archived" list views in other open tabs live-update.
	r.AttachItemSessionsForPublish(ctx, &result)
	r.PublishItemChanged(ctx, &result, backlogdto.BacklogItemChange{
		Kind:      backlogdto.ChangeStatusTransition,
		OldStatus: current.Status,
		NewStatus: string(domain.BacklogStatusIdea),
	})

	return &result, nil
}

// rawTransitionBacklogItemStatus changes the status of a backlog item with
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
func (r *EntRepository) RawTransitionBacklogItemStatus(ctx context.Context, id string, toStatus domain.BacklogStatus, precondition *backlogdto.BacklogItemPrecondition, triggeredBy string) (*backlogdto.BacklogItemData, error) {
	return r.rawTransitionBacklogItemStatusWithNote(ctx, id, toStatus, precondition, triggeredBy, "")
}

// applyStatusPreconditionWhere folds a precondition's expected status and
// updated_at into the UPDATE's WHERE clause.
func applyStatusPreconditionWhere(update *ent.BacklogItemUpdate, precondition *backlogdto.BacklogItemPrecondition) *ent.BacklogItemUpdate {
	if precondition == nil {
		return update
	}
	if precondition.ExpectedStatus != "" {
		update = update.Where(backlogitem.StatusEQ(precondition.ExpectedStatus))
	}
	if precondition.ExpectedUpdatedAt != nil {
		update = update.Where(backlogitem.UpdatedAtEQ(*precondition.ExpectedUpdatedAt))
	}
	return update
}

// rawTransitionBacklogItemStatusWithNote is rawTransitionBacklogItemStatus that
// also appends appendNote (when non-empty) to the item's Notes in the same
// UPDATE, so the status, the note and the status event commit together.
func (r *EntRepository) rawTransitionBacklogItemStatusWithNote(ctx context.Context, id string, toStatus domain.BacklogStatus, precondition *backlogdto.BacklogItemPrecondition, triggeredBy, appendNote string) (*backlogdto.BacklogItemData, error) {
	parsedID, err := r.ResolveBacklogItemLookup(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid id %q: %v", domain.ErrNotFound, id, err)
	}

	current, err := r.Client.BacklogItem.Get(ctx, parsedID)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf("%w: backlog item %s", domain.ErrNotFound, id)
		}
		return nil, fmt.Errorf("failed to get backlog item %s: %w", id, err)
	}

	update := r.Client.BacklogItem.Update().Where(backlogitem.ID(parsedID))
	update = applyStatusPreconditionWhere(update, precondition)
	// The note is appended to the Notes read above; pin updated_at so a
	// concurrent note edit makes the write lose (callers retry on
	// ErrPreconditionFailed) instead of being overwritten.
	if appendNote != "" && (precondition == nil || precondition.ExpectedUpdatedAt == nil) {
		update = update.Where(backlogitem.UpdatedAtEQ(current.UpdatedAt))
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
	if toStatus == domain.BacklogStatusDone && current.NextWorkflowID != nil {
		setter = setter.SetChainedAt(now)
	}
	if appendNote != "" {
		notes := current.Notes
		if notes != "" {
			notes += "\n"
		}
		setter = setter.SetNotes(notes + appendNote)
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
		latest, getErr := r.Client.BacklogItem.Get(ctx, parsedID)
		if getErr != nil {
			if ent.IsNotFound(getErr) {
				return nil, fmt.Errorf("%w: backlog item %s", domain.ErrNotFound, id)
			}
			return nil, fmt.Errorf("failed to get backlog item %s: %w", id, getErr)
		}
		if precondition != nil && precondition.ExpectedStatus != "" && latest.Status != precondition.ExpectedStatus {
			return nil, fmt.Errorf("%w: expected status %q, got %q", domain.ErrPreconditionFailed, precondition.ExpectedStatus, latest.Status)
		}
		return nil, fmt.Errorf("%w: updated_at mismatch", domain.ErrPreconditionFailed)
	}

	item, err := r.Client.BacklogItem.Get(ctx, parsedID)
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
	stageNameSnapshot, allowedTransitionsSnapshot := resolveStageSnapshotFields(ctx, r.Client.BacklogStage, r.Client.StageTransition, toStatus)
	recordStatusEvent(ctx, statusEventInput{
		evClient:                   r.Client.BacklogStatusEvent,
		itemID:                     parsedID,
		fromStatus:                 fromStatus,
		toStatus:                   string(toStatus),
		triggeredBy:                triggeredBy,
		note:                       note,
		stageNameSnapshot:          stageNameSnapshot,
		allowedTransitionsSnapshot: allowedTransitionsSnapshot,
	})

	result := backlogstore.BacklogItemToData(item)

	// Best-effort publish: never blocks or fails the transition itself.
	r.PublishItemChanged(ctx, &result, backlogdto.BacklogItemChange{
		Kind:      backlogdto.ChangeStatusTransition,
		OldStatus: fromStatus,
		NewStatus: string(toStatus),
	})

	// AC4 (webhook-triggers Phase 5): fire the on_session_complete callback after
	// the transition has already committed and this function is about to return —
	// dispatchCallback is itself non-blocking (bounded semaphore + go), so this
	// adds no latency to the caller and a delivery failure can never roll back or
	// corrupt the status transition above.
	if toStatus == domain.BacklogStatusDone {
		r.DispatchCallback("session_complete", map[string]any{
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
	parsedID, err := r.ResolveBacklogItemLookup(ctx, id)
	if err != nil {
		return false, fmt.Errorf("%w: invalid id %q: %v", domain.ErrNotFound, id, err)
	}

	affected, err := r.Client.BacklogItem.Update().
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
	if item, getErr := r.Client.BacklogItem.Get(ctx, parsedID); getErr == nil {
		result := backlogstore.BacklogItemToData(item)
		r.PublishItemChanged(ctx, &result, backlogdto.BacklogItemChange{
			Kind:          backlogdto.ChangeItemUpdated,
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
	parsedID, err := r.ResolveBacklogItemLookup(ctx, id)
	if err != nil {
		return fmt.Errorf("%w: invalid id %q: %v", domain.ErrNotFound, id, err)
	}
	if _, err := r.Client.BacklogItem.UpdateOneID(parsedID).SetChainFired(false).Save(ctx); err != nil {
		return fmt.Errorf("revert chain fire claim for item %s: %w", id, err)
	}
	return nil
}

// rawTransitionBacklogItemStatusWithPRFields atomically transitions a backlog
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
// (see the `interface-pollution-checklist` skill).
func (r *EntRepository) rawTransitionBacklogItemStatusWithPRFields(ctx context.Context, id string, toStatus domain.BacklogStatus, prURL string, prNumber int, precondition *backlogdto.BacklogItemPrecondition, triggeredBy string, link *itemPRLinkWrite) (*backlogdto.BacklogItemData, error) {
	parsedID, err := r.ResolveBacklogItemLookup(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid id %q: %v", domain.ErrNotFound, id, err)
	}

	current, err := r.Client.BacklogItem.Get(ctx, parsedID)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf("%w: backlog item %s", domain.ErrNotFound, id)
		}
		return nil, fmt.Errorf("failed to get backlog item %s: %w", id, err)
	}

	// The item UPDATE and the item-direct link write share one transaction, so a
	// failure of either leaves neither (BUG-058 narrowed). Only tx is used until
	// commit or rollback: a second connection would deadlock a single-writer sqlite.
	tx, err := r.Client.Tx(ctx)
	if err != nil {
		return nil, fmt.Errorf("transition with PR fields: begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	update := tx.BacklogItem.Update().Where(backlogitem.ID(parsedID))
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
		_ = tx.Rollback()
		latest, getErr := r.Client.BacklogItem.Get(ctx, parsedID)
		if getErr != nil {
			if ent.IsNotFound(getErr) {
				return nil, fmt.Errorf("%w: backlog item %s", domain.ErrNotFound, id)
			}
			return nil, fmt.Errorf("failed to get backlog item %s: %w", id, getErr)
		}
		if precondition != nil && precondition.ExpectedStatus != "" && latest.Status != precondition.ExpectedStatus {
			return nil, fmt.Errorf("%w: expected status %q, got %q", domain.ErrPreconditionFailed, precondition.ExpectedStatus, latest.Status)
		}
		return nil, fmt.Errorf("%w: updated_at mismatch", domain.ErrPreconditionFailed)
	}
	if err := link.writeTx(ctx, tx, parsedID.String(), r.AfterLinkHook()); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("transition with PR fields: commit: %w", err)
	}

	item, err := r.Client.BacklogItem.Get(ctx, parsedID)
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
	stageNameSnapshot, allowedTransitionsSnapshot := resolveStageSnapshotFields(ctx, r.Client.BacklogStage, r.Client.StageTransition, toStatus)
	recordStatusEvent(ctx, statusEventInput{
		evClient:                   r.Client.BacklogStatusEvent,
		itemID:                     parsedID,
		fromStatus:                 fromStatus,
		toStatus:                   string(toStatus),
		triggeredBy:                triggeredBy,
		note:                       note,
		stageNameSnapshot:          stageNameSnapshot,
		allowedTransitionsSnapshot: allowedTransitionsSnapshot,
	})

	result := backlogstore.BacklogItemToData(item)

	// Best-effort publish: never blocks or fails the transition itself.
	r.PublishItemChanged(ctx, &result, backlogdto.BacklogItemChange{
		Kind:      backlogdto.ChangeStatusTransition,
		OldStatus: fromStatus,
		NewStatus: string(toStatus),
	})

	return &result, nil
}

// --- BacklogStuckState (durable stuck-state bookkeeping) ---

// --- Progress note history ---

// --- Activity note history (ADR-001) ---
