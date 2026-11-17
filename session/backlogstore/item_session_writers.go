package backlogstore

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/itemsession"
	"github.com/tstapler/stapler-squad/session/ent/reviewverdict"
)

// backlogItemForItemSession resolves the BacklogItemData owning the given
// ItemSession id, for publish hooks that only have an ItemSession id in hand
// (UpdateItemSessionSessionUUID, UpdateItemSessionTriageResult, SaveReviewVerdict)
// rather than a BacklogItem id directly. Uses the same backlog_item edge
// GetItemSession's .WithBacklogItem() query already loads. Callers must treat
// a returned error as best-effort: log it and skip the publish call rather
// than failing the mutation that already succeeded.
func (b *Backlog) backlogItemForItemSession(ctx context.Context, itemSessionID uuid.UUID) (*backlogdto.BacklogItemData, error) {
	ownerID, err := b.client.ItemSession.Query().
		Where(itemsession.ID(itemSessionID)).
		QueryBacklogItem().
		OnlyID(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve owning backlog item for item session %s: %w", itemSessionID, err)
	}
	item, err := b.GetBacklogItem(ctx, ownerID.String())
	if err != nil {
		return nil, err
	}
	return item, nil
}

// CreateItemSession creates a new ItemSession linked to a BacklogItem.
func (b *Backlog) CreateItemSession(ctx context.Context, data backlogdto.ItemSessionData) (backlogdto.ItemSessionSummary, error) {
	parsedItemID, err := b.ResolveBacklogItemLookup(ctx, data.ItemID)
	if err != nil {
		return backlogdto.ItemSessionSummary{}, fmt.Errorf("invalid item id %q: %w", data.ItemID, err)
	}

	q := b.client.ItemSession.Create().
		SetSessionUUID(data.SessionUUID).
		SetSessionRole(data.SessionRole).
		SetConversationUUID(data.ConversationUUID).
		SetBranchName(data.BranchName).
		SetBacklogItemID(parsedItemID).
		SetNillableAcSnapshot(nilIfEmpty(string(data.AcSnapshot))).
		SetPipelineModeSnapshot(data.PipelineModeSnapshot).
		SetPipelineModeSnapshotHash(data.PipelineModeSnapshotHash).
		SetResolvedProgram(data.ResolvedProgram).
		SetResolvedModel(data.ResolvedModel).
		SetExecutorSnapshotHash(data.ExecutorSnapshotHash).
		SetConfiguredProgram(data.ConfiguredProgram).
		SetExecutorFallbackReason(data.ExecutorFallbackReason).
		SetNillableTriageResult(nilIfEmpty(data.TriageResult)).
		SetNillableVerificationNotes(nilIfEmpty(data.VerificationNotes)).
		SetDiffTruncated(data.DiffTruncated).
		SetClaimantHostID(data.ClaimantHostID)
	if data.EstimatedCostUsd > 0 {
		q = q.SetEstimatedCostUsd(data.EstimatedCostUsd)
	}
	if data.CostUnpriced {
		q = q.SetCostPriced(false)
	}
	is, err := q.Save(ctx)
	if err != nil {
		return backlogdto.ItemSessionSummary{}, fmt.Errorf("failed to create item session: %w", err)
	}
	// BacklogItemID is not loaded via edge on create; set it directly from the input.
	summary := ItemSessionToSummary(is)
	summary.BacklogItemID = data.ItemID

	// Best-effort publish: never blocks or fails session creation itself.
	if item, lookupErr := b.GetBacklogItem(ctx, data.ItemID); lookupErr != nil {
		log.WarningLog().Printf("[EntRepository] CreateItemSession: failed to resolve backlog item %s for publish: %v", data.ItemID, lookupErr)
	} else {
		b.PublishItemChanged(ctx, item, backlogdto.BacklogItemChange{
			Kind:           backlogdto.ChangeSessionAttached,
			SessionID:      data.SessionUUID,
			ClaimantHostID: data.ClaimantHostID,
		})
	}

	return summary, nil
}

// UpdateItemSessionStarted records the start time for an ItemSession.
func (b *Backlog) UpdateItemSessionStarted(ctx context.Context, id string, startedAt time.Time) error {
	parsedID, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("invalid id %q: %w", id, err)
	}

	_, err = b.client.ItemSession.UpdateOneID(parsedID).
		SetStartedAt(startedAt).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("failed to set started_at on item session %s: %w", id, err)
	}

	// Best-effort publish: never blocks or fails the update itself. Found
	// missing by the Phase 5 spec-compliance sweep's follow-up pass over the
	// remaining publish-hook bypasses (docs/tasks/backlog-feature-improvement.md).
	if item, lookupErr := b.backlogItemForItemSession(ctx, parsedID); lookupErr != nil {
		log.WarningLog().Printf("[EntRepository] UpdateItemSessionStarted: failed to resolve owning backlog item for item session %s: %v", id, lookupErr)
	} else {
		b.PublishItemChanged(ctx, item, backlogdto.BacklogItemChange{
			Kind:          backlogdto.ChangeItemUpdated,
			UpdatedFields: []string{"itemSessions"},
		})
	}

	return nil
}

// UpdateItemSessionSessionUUID updates the session_uuid field on an existing ItemSession.
func (b *Backlog) UpdateItemSessionSessionUUID(ctx context.Context, id string, sessionUUID string) error {
	parsedID, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("invalid id %q: %w", id, err)
	}

	_, err = b.client.ItemSession.UpdateOneID(parsedID).
		SetSessionUUID(sessionUUID).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("failed to set session_uuid on item session %s: %w", id, err)
	}

	// Best-effort publish: never blocks or fails the update itself.
	if item, lookupErr := b.backlogItemForItemSession(ctx, parsedID); lookupErr != nil {
		log.WarningLog().Printf("[EntRepository] UpdateItemSessionSessionUUID: failed to resolve owning backlog item for item session %s: %v", id, lookupErr)
	} else {
		b.PublishItemChanged(ctx, item, backlogdto.BacklogItemChange{
			Kind:      backlogdto.ChangeSessionAttached,
			SessionID: sessionUUID,
		})
	}

	return nil
}

// UpdateItemSessionGitActivity records the item session's *current* tip commit
// and the count of commits it has authored since its base. Called repeatedly by
// refreshWorkSessionGitActivity (session/backlog_lifecycle.go) while the session
// is active, so downstream "has this session's work landed on main?" checks read
// live data. To record the spawn-time baseline instead, use
// SetItemSessionBaseCommit.
func (b *Backlog) UpdateItemSessionGitActivity(ctx context.Context, id string, sha, msg string, commitAt time.Time, commitCount int) error {
	parsedID, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("invalid id %q: %w", id, err)
	}

	// last_progress_at records when progress was *observed*, not when the commit
	// was authored — deliberately time.Now() rather than commitAt. Git author
	// timestamps survive rebases and cherry-picks, and this repo rebases session
	// worktrees onto main routinely, so feeding an author date into the staleness
	// clock (reconcileStaleWorkSessions, which reads this field) would push it
	// backwards on a rebase and falsely flag a healthy, actively-committing
	// session as stale_work — triggering automated remediation against a session
	// that is working fine. Observing a new commit right now IS progress now.
	// last_commit_at keeps the true author time for display.
	_, err = b.client.ItemSession.UpdateOneID(parsedID).
		SetLastCommitSha(sha).
		SetLastCommitMessage(msg).
		SetLastCommitAt(commitAt).
		SetCommitCountSinceSpawn(commitCount).
		SetLastProgressAt(time.Now()).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("failed to update git activity on item session %s: %w", id, err)
	}

	// Best-effort publish: never blocks or fails the update itself. Found
	// missing by the Phase 5 spec-compliance sweep's follow-up pass over the
	// remaining publish-hook bypasses (docs/tasks/backlog-feature-improvement.md).
	if item, lookupErr := b.backlogItemForItemSession(ctx, parsedID); lookupErr != nil {
		log.WarningLog().Printf("[EntRepository] UpdateItemSessionGitActivity: failed to resolve owning backlog item for item session %s: %v", id, lookupErr)
	} else {
		b.PublishItemChanged(ctx, item, backlogdto.BacklogItemChange{
			Kind:          backlogdto.ChangeItemUpdated,
			UpdatedFields: []string{"itemSessions"},
		})
	}

	return nil
}

// UpdateItemSessionFileTouch updates the last file touch timestamp on an ItemSession.
func (b *Backlog) UpdateItemSessionFileTouch(ctx context.Context, id string, touchAt time.Time) error {
	parsedID, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("invalid id %q: %w", id, err)
	}

	_, err = b.client.ItemSession.UpdateOneID(parsedID).
		SetLastFileTouchAt(touchAt).
		SetLastProgressAt(touchAt).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("failed to update file touch on item session %s: %w", id, err)
	}

	// Best-effort publish: never blocks or fails the update itself. Found
	// missing by the Phase 5 spec-compliance sweep's follow-up pass over the
	// remaining publish-hook bypasses (docs/tasks/backlog-feature-improvement.md).
	if item, lookupErr := b.backlogItemForItemSession(ctx, parsedID); lookupErr != nil {
		log.WarningLog().Printf("[EntRepository] UpdateItemSessionFileTouch: failed to resolve owning backlog item for item session %s: %v", id, lookupErr)
	} else {
		b.PublishItemChanged(ctx, item, backlogdto.BacklogItemChange{
			Kind:          backlogdto.ChangeItemUpdated,
			UpdatedFields: []string{"itemSessions"},
		})
	}

	return nil
}

// UpdateItemSessionTriageResult stores the triage result JSON payload on an ItemSession.
func (b *Backlog) UpdateItemSessionTriageResult(ctx context.Context, id string, triageResult string) error {
	parsedID, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("invalid id %q: %w", id, err)
	}

	_, err = b.client.ItemSession.UpdateOneID(parsedID).
		SetTriageResult(triageResult).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("failed to update triage_result on item session %s: %w", id, err)
	}

	// Best-effort publish: never blocks or fails the triage-result write
	// itself. The owning item lookup can fail on a legitimate edge case (the
	// ItemSession row was deleted concurrently) — that's logged and skipped,
	// not fatal, same "publish is best-effort" guarantee as every other hook.
	if item, lookupErr := b.backlogItemForItemSession(ctx, parsedID); lookupErr != nil {
		log.WarningLog().Printf("[EntRepository] UpdateItemSessionTriageResult: failed to resolve owning backlog item for item session %s: %v", id, lookupErr)
	} else {
		b.PublishItemChanged(ctx, item, backlogdto.BacklogItemChange{
			Kind:          backlogdto.ChangeTriageProgressUpdated,
			UpdatedFields: []string{"triageResultSummary"},
		})
	}

	return nil
}

// UpdateItemSessionVerificationNotes stores the verification evidence reported via
// request_review (commands run, manual checks performed) on an ItemSession.
func (b *Backlog) UpdateItemSessionVerificationNotes(ctx context.Context, id string, verificationNotes string) error {
	parsedID, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("invalid id %q: %w", id, err)
	}

	_, err = b.client.ItemSession.UpdateOneID(parsedID).
		SetVerificationNotes(verificationNotes).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("failed to update verification_notes on item session %s: %w", id, err)
	}

	// Best-effort publish: never blocks or fails the update itself. Found
	// missing by the Phase 5 spec-compliance sweep's follow-up pass over the
	// remaining publish-hook bypasses (docs/tasks/backlog-feature-improvement.md).
	if item, lookupErr := b.backlogItemForItemSession(ctx, parsedID); lookupErr != nil {
		log.WarningLog().Printf("[EntRepository] UpdateItemSessionVerificationNotes: failed to resolve owning backlog item for item session %s: %v", id, lookupErr)
	} else {
		b.PublishItemChanged(ctx, item, backlogdto.BacklogItemChange{
			Kind:          backlogdto.ChangeItemUpdated,
			UpdatedFields: []string{"itemSessions"},
		})
	}

	return nil
}

// SaveReviewVerdict upserts a ReviewVerdict for a given ItemSession.
// The query-then-create/update is wrapped in a transaction to prevent a
// check-then-act race condition when concurrent callers save verdicts for the
// same item session.
func (b *Backlog) SaveReviewVerdict(ctx context.Context, itemSessionID string, verdict backlogdto.ReviewVerdictData) error {
	parsedSessionID, err := uuid.Parse(itemSessionID)
	if err != nil {
		return fmt.Errorf("invalid item session id %q: %w", itemSessionID, err)
	}

	tx, err := b.client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// Try to find existing verdict for this item session within the transaction.
	existing, queryErr := tx.ReviewVerdict.Query().
		Where(reviewverdict.HasItemSessionWith(itemsession.ID(parsedSessionID))).
		Only(ctx)

	if queryErr != nil && !ent.IsNotFound(queryErr) {
		return fmt.Errorf("failed to query existing review verdict: %w", queryErr)
	}

	if ent.IsNotFound(queryErr) || existing == nil {
		// Create new verdict.
		_, err = tx.ReviewVerdict.Create().
			SetOverallOutcome(string(verdict.OverallOutcome)).
			SetNillablePerCriterion(nilIfEmpty(verdict.PerCriterion)).
			SetNillableSummary(nilIfEmpty(verdict.Summary)).
			SetNillableDiffHash(nilIfEmpty(verdict.DiffHash)).
			SetNillablePromptHash(nilIfEmpty(verdict.PromptHash)).
			SetDiffTokenCount(verdict.DiffTokenCount).
			SetDiffTruncated(verdict.DiffTruncated).
			SetNillableReviewedCommitSha(nilIfEmpty(verdict.ReviewedCommitSha)).
			SetNillableOverrideBy(nilIfEmpty(verdict.OverrideBy)).
			SetNillableOverrideReason(nilIfEmpty(verdict.OverrideReason)).
			SetNillableOverrideAt(verdict.OverrideAt).
			SetItemSessionID(parsedSessionID).
			Save(ctx)
		if err != nil {
			return fmt.Errorf("failed to create review verdict: %w", err)
		}
	} else {
		// Update existing verdict.
		_, err = tx.ReviewVerdict.UpdateOne(existing).
			SetOverallOutcome(string(verdict.OverallOutcome)).
			SetNillablePerCriterion(nilIfEmpty(verdict.PerCriterion)).
			SetNillableSummary(nilIfEmpty(verdict.Summary)).
			SetNillableDiffHash(nilIfEmpty(verdict.DiffHash)).
			SetNillablePromptHash(nilIfEmpty(verdict.PromptHash)).
			SetDiffTokenCount(verdict.DiffTokenCount).
			SetDiffTruncated(verdict.DiffTruncated).
			SetNillableReviewedCommitSha(nilIfEmpty(verdict.ReviewedCommitSha)).
			SetNillableOverrideBy(nilIfEmpty(verdict.OverrideBy)).
			SetNillableOverrideReason(nilIfEmpty(verdict.OverrideReason)).
			SetNillableOverrideAt(verdict.OverrideAt).
			Save(ctx)
		if err != nil {
			return fmt.Errorf("failed to update review verdict: %w", err)
		}
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit review verdict transaction: %w", err)
	}

	// Best-effort publish: never blocks or fails the verdict save itself. The
	// verdict travels IN the payload (not via a client-side join against
	// item_sessions) — see BacklogItemChange.Verdict's doc comment.
	if item, lookupErr := b.backlogItemForItemSession(ctx, parsedSessionID); lookupErr != nil {
		log.WarningLog().Printf("[EntRepository] SaveReviewVerdict: failed to resolve owning backlog item for item session %s: %v", itemSessionID, lookupErr)
	} else {
		b.PublishItemChanged(ctx, item, backlogdto.BacklogItemChange{
			Kind:    backlogdto.ChangeVerdictRecorded,
			Verdict: &verdict,
		})
	}

	return nil
}

// CreateItemSessionWithVerdict atomically creates an ItemSession and its initial
// ReviewVerdict in a single transaction. If the verdict write fails the ItemSession
// is rolled back, preventing dangling sessions with no verdict.
func (b *Backlog) CreateItemSessionWithVerdict(ctx context.Context, isData backlogdto.ItemSessionData, verdict backlogdto.ReviewVerdictData) (backlogdto.ItemSessionSummary, error) {
	parsedItemID, err := b.ResolveBacklogItemLookup(ctx, isData.ItemID)
	if err != nil {
		return backlogdto.ItemSessionSummary{}, fmt.Errorf("invalid item id %q: %w", isData.ItemID, err)
	}

	tx, err := b.client.Tx(ctx)
	if err != nil {
		return backlogdto.ItemSessionSummary{}, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	isq := tx.ItemSession.Create().
		SetSessionUUID(isData.SessionUUID).
		SetSessionRole(isData.SessionRole).
		SetConversationUUID(isData.ConversationUUID).
		SetBacklogItemID(parsedItemID).
		SetNillableAcSnapshot(nilIfEmptyJSON(isData.AcSnapshot)).
		SetPipelineModeSnapshot(isData.PipelineModeSnapshot).
		SetPipelineModeSnapshotHash(isData.PipelineModeSnapshotHash).
		SetResolvedProgram(isData.ResolvedProgram).
		SetResolvedModel(isData.ResolvedModel).
		SetExecutorSnapshotHash(isData.ExecutorSnapshotHash).
		SetConfiguredProgram(isData.ConfiguredProgram).
		SetExecutorFallbackReason(isData.ExecutorFallbackReason).
		SetNillableTriageResult(nilIfEmpty(isData.TriageResult))
	if isData.EstimatedCostUsd > 0 {
		isq = isq.SetEstimatedCostUsd(isData.EstimatedCostUsd)
	}
	if isData.CostUnpriced {
		isq = isq.SetCostPriced(false)
	}
	is, err := isq.Save(ctx)
	if err != nil {
		return backlogdto.ItemSessionSummary{}, fmt.Errorf("create item session: %w", err)
	}

	rv, err := tx.ReviewVerdict.Create().
		SetOverallOutcome(string(verdict.OverallOutcome)).
		SetNillablePerCriterion(nilIfEmpty(verdict.PerCriterion)).
		SetNillableSummary(nilIfEmpty(verdict.Summary)).
		SetNillableDiffHash(nilIfEmpty(verdict.DiffHash)).
		SetNillablePromptHash(nilIfEmpty(verdict.PromptHash)).
		SetDiffTokenCount(verdict.DiffTokenCount).
		SetDiffTruncated(verdict.DiffTruncated).
		SetNillableReviewedCommitSha(nilIfEmpty(verdict.ReviewedCommitSha)).
		SetNillableOverrideBy(nilIfEmpty(verdict.OverrideBy)).
		SetNillableOverrideReason(nilIfEmpty(verdict.OverrideReason)).
		SetNillableOverrideAt(verdict.OverrideAt).
		SetItemSessionID(is.ID).
		Save(ctx)
	if err != nil {
		return backlogdto.ItemSessionSummary{}, fmt.Errorf("create review verdict: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return backlogdto.ItemSessionSummary{}, fmt.Errorf("commit: %w", err)
	}

	// Build summary with the verdict embedded.
	is.Edges.ReviewVerdict = rv
	summary := ItemSessionToSummary(is)
	summary.BacklogItemID = isData.ItemID

	// Best-effort publish: never blocks or fails the create+verdict itself.
	// Same ChangeVerdictRecorded kind as SaveReviewVerdict's hook above so both
	// verdict-recording paths (RPC and MCP submit_review_verdict) converge on
	// one event kind, each carrying the verdict inline.
	if item, lookupErr := b.GetBacklogItem(ctx, isData.ItemID); lookupErr != nil {
		log.WarningLog().Printf("[EntRepository] CreateItemSessionWithVerdict: failed to resolve backlog item %s for publish: %v", isData.ItemID, lookupErr)
	} else {
		b.PublishItemChanged(ctx, item, backlogdto.BacklogItemChange{
			Kind:    backlogdto.ChangeVerdictRecorded,
			Verdict: &verdict,
		})
	}

	return summary, nil
}
