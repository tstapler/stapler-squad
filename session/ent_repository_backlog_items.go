package session

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gofrs/flock"
	"github.com/google/uuid"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/backlogstore"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/backlogitem"
	"github.com/tstapler/stapler-squad/session/ent/itemsession"
	"github.com/tstapler/stapler-squad/session/ent/reviewverdict"
)

// backlogItemPublicIDBackfillLockFileName is the flock coordination file
// guarding BackfillBacklogItemPublicIDs against two processes racing to
// mint different public_ids for the same row.
const backlogItemPublicIDBackfillLockFileName = "backlog_item_public_id_backfill.lock"

// backlogItemPublicIDBackfillLockTimeout bounds how long
// BackfillBacklogItemPublicIDs waits to acquire its flock before giving up.
// Mirrors hostRegistryLockTimeout (session/host_registry.go).
const backlogItemPublicIDBackfillLockTimeout = 5 * time.Second

// backfillLockFilePath returns the filesystem path to use for a
// flock-guarded cross-process backfill lock named name, derived from
// r.dbPath. Returns "" when there is no real on-disk directory to
// coordinate on: a "file:" URI DSN (e.g. the shared-cache in-memory
// database some tests use) or an EntRepository constructed via
// NewEntRepositoryFromClient with no dbPath set at all. Callers must treat
// "" as "skip locking" — there is no filesystem location to coordinate on,
// and no plausible second OS process to race against in either case.
func (r *EntRepository) backfillLockFilePath(name string) string {
	// A bare ":memory:" DSN has no directory; filepath.Dir would resolve it to the
	// process cwd and litter the working tree with a lock file.
	if r.DbPath == "" || r.DbPath == ":memory:" || strings.HasPrefix(r.DbPath, "file:") {
		return ""
	}
	return filepath.Join(filepath.Dir(r.DbPath), name)
}

// BackfillBacklogItemPublicIDs assigns a freshly minted BacklogItemID to
// every pre-existing BacklogItem row that predates this feature and so has
// no public_id (Story 1.4,
// project_plans/backlog-deep-linking/implementation/plan.md). Idempotent:
// rows that already have a public_id are excluded by the query itself
// (backlogitem.PublicIDIsNil(), per session/ent/schema/backlog_item.go's
// documented NULL-vs-empty-string representation), so re-running against an
// already-backfilled dataset touches no rows. Guarded by a flock-based
// exclusive lock — see backfillLockFilePath — so two instances racing on the
// same state dir can't each assign a different id to the same row; when no
// real lock path is available the backfill still runs, unguarded, since
// there's no second process to race against in that case.
func (r *EntRepository) BackfillBacklogItemPublicIDs(ctx context.Context) error {
	lockPath := r.backfillLockFilePath(backlogItemPublicIDBackfillLockFileName)
	if lockPath == "" {
		return r.backfillBacklogItemPublicIDsLocked(ctx)
	}

	fl := flock.New(lockPath)
	lockCtx, cancel := context.WithTimeout(ctx, backlogItemPublicIDBackfillLockTimeout)
	defer cancel()
	locked, err := fl.TryLockContext(lockCtx, 100*time.Millisecond)
	if err != nil {
		return fmt.Errorf("failed to acquire backlog item public id backfill lock: %w", err)
	}
	if !locked {
		return fmt.Errorf("could not acquire backlog item public id backfill lock within timeout")
	}
	defer func() { _ = fl.Unlock() }()

	return r.backfillBacklogItemPublicIDsLocked(ctx)
}

// backfillBacklogItemPublicIDsLocked does the actual query-and-save work for
// BackfillBacklogItemPublicIDs. Split out so the exported method can wrap it
// with (or, when no real lock path exists, skip) the flock guard without
// duplicating the query/save logic.
func (r *EntRepository) backfillBacklogItemPublicIDsLocked(ctx context.Context) error {
	rows, err := r.Client.BacklogItem.Query().
		Where(backlogitem.PublicIDIsNil()).
		All(ctx)
	if err != nil {
		// Table may not exist yet (fresh DB before schema.Create) — ignore,
		// mirroring migrations.PRURLBackfill's same defensive posture.
		return nil //nolint:nilerr
	}

	var migrated int
	for _, row := range rows {
		// Confirm absence per row via the centralizing accessor (not a
		// second raw == "" check) per plan.md's Story 1.4 note — guards
		// against a row that was assigned a public_id (e.g. by a
		// concurrent CreateBacklogItem call) between the query above and
		// this loop iteration.
		data := &backlogdto.BacklogItemData{PublicIDRaw: row.PublicID}
		if _, ok := data.PublicID(); ok {
			continue
		}

		newID, err := backlogdto.NewBacklogItemID()
		if err != nil {
			return fmt.Errorf("failed to mint backlog item public id for row %s: %w", row.ID, err)
		}
		if _, saveErr := r.Client.BacklogItem.UpdateOneID(row.ID).
			SetPublicID(newID.String()).
			Save(ctx); saveErr != nil {
			log.WarningLog().Printf("[Migration] backlog item public id backfill: item=%s: %v", row.ID, saveErr)
			continue
		}
		migrated++
	}
	if migrated > 0 {
		log.InfoLog().Printf("[Migration] backlog item public id backfill: populated %d row(s)", migrated)
	}
	return nil
}

// backlogItemForItemSession resolves the BacklogItemData owning the given
// ItemSession id, for publish hooks that only have an ItemSession id in hand
// (UpdateItemSessionSessionUUID, UpdateItemSessionTriageResult, SaveReviewVerdict)
// rather than a BacklogItem id directly. Uses the same backlog_item edge
// GetItemSession's .WithBacklogItem() query already loads. Callers must treat
// a returned error as best-effort: log it and skip the publish call rather
// than failing the mutation that already succeeded.
func (r *EntRepository) backlogItemForItemSession(ctx context.Context, itemSessionID uuid.UUID) (*backlogdto.BacklogItemData, error) {
	ownerID, err := r.Client.ItemSession.Query().
		Where(itemsession.ID(itemSessionID)).
		QueryBacklogItem().
		OnlyID(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve owning backlog item for item session %s: %w", itemSessionID, err)
	}
	item, err := r.GetBacklogItem(ctx, ownerID.String())
	if err != nil {
		return nil, err
	}
	return item, nil
}

// --- ItemSession ---

// CreateItemSession creates a new ItemSession linked to a BacklogItem.
func (r *EntRepository) CreateItemSession(ctx context.Context, data backlogdto.ItemSessionData) (backlogdto.ItemSessionSummary, error) {
	parsedItemID, err := r.ResolveBacklogItemLookup(ctx, data.ItemID)
	if err != nil {
		return backlogdto.ItemSessionSummary{}, fmt.Errorf("invalid item id %q: %w", data.ItemID, err)
	}

	q := r.Client.ItemSession.Create().
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
	summary := backlogstore.ItemSessionToSummary(is)
	summary.BacklogItemID = data.ItemID

	// Best-effort publish: never blocks or fails session creation itself.
	if item, lookupErr := r.GetBacklogItem(ctx, data.ItemID); lookupErr != nil {
		log.WarningLog().Printf("[EntRepository] CreateItemSession: failed to resolve backlog item %s for publish: %v", data.ItemID, lookupErr)
	} else {
		r.publishItemChanged(ctx, item, backlogdto.BacklogItemChange{
			Kind:           backlogdto.ChangeSessionAttached,
			SessionID:      data.SessionUUID,
			ClaimantHostID: data.ClaimantHostID,
		})
	}

	return summary, nil
}

// UpdateItemSessionStarted records the start time for an ItemSession.
func (r *EntRepository) UpdateItemSessionStarted(ctx context.Context, id string, startedAt time.Time) error {
	parsedID, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("invalid id %q: %w", id, err)
	}

	_, err = r.Client.ItemSession.UpdateOneID(parsedID).
		SetStartedAt(startedAt).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("failed to set started_at on item session %s: %w", id, err)
	}

	// Best-effort publish: never blocks or fails the update itself. Found
	// missing by the Phase 5 spec-compliance sweep's follow-up pass over the
	// remaining publish-hook bypasses (docs/tasks/backlog-feature-improvement.md).
	if item, lookupErr := r.backlogItemForItemSession(ctx, parsedID); lookupErr != nil {
		log.WarningLog().Printf("[EntRepository] UpdateItemSessionStarted: failed to resolve owning backlog item for item session %s: %v", id, lookupErr)
	} else {
		r.publishItemChanged(ctx, item, backlogdto.BacklogItemChange{
			Kind:          backlogdto.ChangeItemUpdated,
			UpdatedFields: []string{"itemSessions"},
		})
	}

	return nil
}

// UpdateItemSessionSessionUUID updates the session_uuid field on an existing ItemSession.
func (r *EntRepository) UpdateItemSessionSessionUUID(ctx context.Context, id string, sessionUUID string) error {
	parsedID, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("invalid id %q: %w", id, err)
	}

	_, err = r.Client.ItemSession.UpdateOneID(parsedID).
		SetSessionUUID(sessionUUID).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("failed to set session_uuid on item session %s: %w", id, err)
	}

	// Best-effort publish: never blocks or fails the update itself.
	if item, lookupErr := r.backlogItemForItemSession(ctx, parsedID); lookupErr != nil {
		log.WarningLog().Printf("[EntRepository] UpdateItemSessionSessionUUID: failed to resolve owning backlog item for item session %s: %v", id, lookupErr)
	} else {
		r.publishItemChanged(ctx, item, backlogdto.BacklogItemChange{
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
func (r *EntRepository) UpdateItemSessionGitActivity(ctx context.Context, id string, sha, msg string, commitAt time.Time, commitCount int) error {
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
	_, err = r.Client.ItemSession.UpdateOneID(parsedID).
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
	if item, lookupErr := r.backlogItemForItemSession(ctx, parsedID); lookupErr != nil {
		log.WarningLog().Printf("[EntRepository] UpdateItemSessionGitActivity: failed to resolve owning backlog item for item session %s: %v", id, lookupErr)
	} else {
		r.publishItemChanged(ctx, item, backlogdto.BacklogItemChange{
			Kind:          backlogdto.ChangeItemUpdated,
			UpdatedFields: []string{"itemSessions"},
		})
	}

	return nil
}

// UpdateItemSessionFileTouch updates the last file touch timestamp on an ItemSession.
func (r *EntRepository) UpdateItemSessionFileTouch(ctx context.Context, id string, touchAt time.Time) error {
	parsedID, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("invalid id %q: %w", id, err)
	}

	_, err = r.Client.ItemSession.UpdateOneID(parsedID).
		SetLastFileTouchAt(touchAt).
		SetLastProgressAt(touchAt).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("failed to update file touch on item session %s: %w", id, err)
	}

	// Best-effort publish: never blocks or fails the update itself. Found
	// missing by the Phase 5 spec-compliance sweep's follow-up pass over the
	// remaining publish-hook bypasses (docs/tasks/backlog-feature-improvement.md).
	if item, lookupErr := r.backlogItemForItemSession(ctx, parsedID); lookupErr != nil {
		log.WarningLog().Printf("[EntRepository] UpdateItemSessionFileTouch: failed to resolve owning backlog item for item session %s: %v", id, lookupErr)
	} else {
		r.publishItemChanged(ctx, item, backlogdto.BacklogItemChange{
			Kind:          backlogdto.ChangeItemUpdated,
			UpdatedFields: []string{"itemSessions"},
		})
	}

	return nil
}

// UpdateItemSessionTriageResult stores the triage result JSON payload on an ItemSession.
func (r *EntRepository) UpdateItemSessionTriageResult(ctx context.Context, id string, triageResult string) error {
	parsedID, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("invalid id %q: %w", id, err)
	}

	_, err = r.Client.ItemSession.UpdateOneID(parsedID).
		SetTriageResult(triageResult).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("failed to update triage_result on item session %s: %w", id, err)
	}

	// Best-effort publish: never blocks or fails the triage-result write
	// itself. The owning item lookup can fail on a legitimate edge case (the
	// ItemSession row was deleted concurrently) — that's logged and skipped,
	// not fatal, same "publish is best-effort" guarantee as every other hook.
	if item, lookupErr := r.backlogItemForItemSession(ctx, parsedID); lookupErr != nil {
		log.WarningLog().Printf("[EntRepository] UpdateItemSessionTriageResult: failed to resolve owning backlog item for item session %s: %v", id, lookupErr)
	} else {
		r.publishItemChanged(ctx, item, backlogdto.BacklogItemChange{
			Kind:          backlogdto.ChangeTriageProgressUpdated,
			UpdatedFields: []string{"triageResultSummary"},
		})
	}

	return nil
}

// UpdateItemSessionVerificationNotes stores the verification evidence reported via
// request_review (commands run, manual checks performed) on an ItemSession.
func (r *EntRepository) UpdateItemSessionVerificationNotes(ctx context.Context, id string, verificationNotes string) error {
	parsedID, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("invalid id %q: %w", id, err)
	}

	_, err = r.Client.ItemSession.UpdateOneID(parsedID).
		SetVerificationNotes(verificationNotes).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("failed to update verification_notes on item session %s: %w", id, err)
	}

	// Best-effort publish: never blocks or fails the update itself. Found
	// missing by the Phase 5 spec-compliance sweep's follow-up pass over the
	// remaining publish-hook bypasses (docs/tasks/backlog-feature-improvement.md).
	if item, lookupErr := r.backlogItemForItemSession(ctx, parsedID); lookupErr != nil {
		log.WarningLog().Printf("[EntRepository] UpdateItemSessionVerificationNotes: failed to resolve owning backlog item for item session %s: %v", id, lookupErr)
	} else {
		r.publishItemChanged(ctx, item, backlogdto.BacklogItemChange{
			Kind:          backlogdto.ChangeItemUpdated,
			UpdatedFields: []string{"itemSessions"},
		})
	}

	return nil
}

// --- ReviewVerdict ---

// SaveReviewVerdict upserts a ReviewVerdict for a given ItemSession.
// The query-then-create/update is wrapped in a transaction to prevent a
// check-then-act race condition when concurrent callers save verdicts for the
// same item session.
func (r *EntRepository) SaveReviewVerdict(ctx context.Context, itemSessionID string, verdict backlogdto.ReviewVerdictData) error {
	parsedSessionID, err := uuid.Parse(itemSessionID)
	if err != nil {
		return fmt.Errorf("invalid item session id %q: %w", itemSessionID, err)
	}

	tx, err := r.Client.Tx(ctx)
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
	if item, lookupErr := r.backlogItemForItemSession(ctx, parsedSessionID); lookupErr != nil {
		log.WarningLog().Printf("[EntRepository] SaveReviewVerdict: failed to resolve owning backlog item for item session %s: %v", itemSessionID, lookupErr)
	} else {
		r.publishItemChanged(ctx, item, backlogdto.BacklogItemChange{
			Kind:    backlogdto.ChangeVerdictRecorded,
			Verdict: &verdict,
		})
	}

	return nil
}

// CreateItemSessionWithVerdict atomically creates an ItemSession and its initial
// ReviewVerdict in a single transaction. If the verdict write fails the ItemSession
// is rolled back, preventing dangling sessions with no verdict.
func (r *EntRepository) CreateItemSessionWithVerdict(ctx context.Context, isData backlogdto.ItemSessionData, verdict backlogdto.ReviewVerdictData) (backlogdto.ItemSessionSummary, error) {
	parsedItemID, err := r.ResolveBacklogItemLookup(ctx, isData.ItemID)
	if err != nil {
		return backlogdto.ItemSessionSummary{}, fmt.Errorf("invalid item id %q: %w", isData.ItemID, err)
	}

	tx, err := r.Client.Tx(ctx)
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
	summary := backlogstore.ItemSessionToSummary(is)
	summary.BacklogItemID = isData.ItemID

	// Best-effort publish: never blocks or fails the create+verdict itself.
	// Same ChangeVerdictRecorded kind as SaveReviewVerdict's hook above so both
	// verdict-recording paths (RPC and MCP submit_review_verdict) converge on
	// one event kind, each carrying the verdict inline.
	if item, lookupErr := r.GetBacklogItem(ctx, isData.ItemID); lookupErr != nil {
		log.WarningLog().Printf("[EntRepository] CreateItemSessionWithVerdict: failed to resolve backlog item %s for publish: %v", isData.ItemID, lookupErr)
	} else {
		r.publishItemChanged(ctx, item, backlogdto.BacklogItemChange{
			Kind:    backlogdto.ChangeVerdictRecorded,
			Verdict: &verdict,
		})
	}

	return summary, nil
}

// --- Reconciler ---

// BackfillMissingPRNumbers finds pr_pending items with a pr_url but no
// pr_number (pr_number == 0) and parses the number out of the URL. Such items
// are otherwise permanently invisible to FindPRPendingItems' PrNumberGT(0)
// filter, so ReconcilePRPending never polls them — a real stuck-forever case
// found via manual QA against live data, not something the loop itself would
// ever have surfaced. Best-effort: a URL that doesn't match is left as-is and
// logged, not treated as fatal.
func (r *EntRepository) BackfillMissingPRNumbers(ctx context.Context) (int, error) {
	items, err := r.Client.BacklogItem.Query().
		Where(
			backlogitem.Status(string(domain.BacklogStatusPRPending)),
			backlogitem.PrNumberEQ(0),
			backlogitem.PrURLNotNil(),
			backlogitem.PrURLNEQ(""),
		).
		All(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to query items with missing pr_number: %w", err)
	}

	backfilled := 0
	for _, item := range items {
		m := backlogdto.PrNumberFromURLRe.FindStringSubmatch(item.PrURL)
		if m == nil {
			continue
		}
		num, convErr := strconv.Atoi(m[1])
		if convErr != nil || num <= 0 {
			continue
		}
		updated, updateErr := r.Client.BacklogItem.UpdateOneID(item.ID).SetPrNumber(num).Save(ctx)
		if updateErr != nil {
			return backfilled, fmt.Errorf("failed to backfill pr_number for item %s: %w", item.ID, updateErr)
		}
		backfilled++

		// Best-effort publish: never blocks or fails the backfill itself. Found
		// missing by the Phase 5 spec-compliance sweep's follow-up pass over the
		// remaining publish-hook bypasses (docs/tasks/backlog-feature-improvement.md).
		result := backlogstore.BacklogItemToData(updated)
		r.publishItemChanged(ctx, &result, backlogdto.BacklogItemChange{
			Kind:          backlogdto.ChangeItemUpdated,
			UpdatedFields: []string{"prNumber"},
		})
	}
	return backfilled, nil
}

// --- BacklogStuckState queries ---

// --- ReviewVerdict lookup ---

// --- AC criterion update ---

// UpdateAcCriterionStatus updates a single acceptance criterion's status by index.
func (r *EntRepository) UpdateAcCriterionStatus(ctx context.Context, itemID string, criterionIndex int, status string, note string) error {
	return r.mutateAcCriterion(ctx, itemID, criterionIndex, acChange{newStatus: status, reason: note}, func(c *domain.AcCriterion) error {
		if c.Status == domain.AcStatusWaived {
			return backlogdto.ErrAcCriterionWaived
		}
		c.Status = domain.AcStatus(status)
		if note != "" {
			c.Note = note
		}
		return nil
	})
}

// WaiveAcCriterion marks a criterion waived with a non-empty reason. Refused (and audited) when
// the criterion is already done or already waived.
func (r *EntRepository) WaiveAcCriterion(ctx context.Context, itemID string, criterionIndex int, reason string) error {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return backlogdto.ErrWaiverReasonRequired
	}
	actor := domain.AcActorFrom(ctx)
	return r.mutateAcCriterion(ctx, itemID, criterionIndex, acChange{newStatus: string(domain.AcStatusWaived), reason: reason}, func(c *domain.AcCriterion) error {
		switch c.Status {
		case domain.AcStatusDone:
			return backlogdto.ErrAcCriterionAlreadyDone
		case domain.AcStatusWaived:
			return backlogdto.ErrAcCriterionAlreadyWaived
		}
		c.Status = domain.AcStatusWaived
		c.WaiverReason = reason
		c.WaivedBy = actor.Label()
		c.WaivedAt = time.Now().UTC().Format(time.RFC3339)
		return nil
	})
}

// acChange describes the requested status change so it can be audited whatever its outcome.
type acChange struct {
	newStatus string
	reason    string
}

// mutateAcCriterion loads the item's criteria, applies mutate to the one at
// criterionIndex, and persists the result together with an AcAuditEvent in one
// transaction. A mutation that reaches an existing item but is refused still
// leaves an audit row (outcome "rejected:...").
func (r *EntRepository) mutateAcCriterion(ctx context.Context, itemID string, criterionIndex int, change acChange, mutate func(*domain.AcCriterion) error) error {
	parsedID, err := r.ResolveBacklogItemLookup(ctx, itemID)
	if err != nil {
		if ent.IsNotFound(err) {
			return fmt.Errorf("%w: backlog item %s", domain.ErrNotFound, itemID)
		}
		return fmt.Errorf("invalid item id %q: %w", itemID, err)
	}

	tx, err := r.Client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("begin AC update transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	item, err := tx.BacklogItem.Get(ctx, parsedID)
	if err != nil {
		if ent.IsNotFound(err) {
			return fmt.Errorf("%w: backlog item %s", domain.ErrNotFound, itemID)
		}
		return fmt.Errorf("failed to get backlog item %s: %w", itemID, err)
	}

	criteria, parseErr := domain.ParseAcCriteria(domain.AcCriteriaJSON(item.AcceptanceCriteria))
	if parseErr != nil {
		return fmt.Errorf("failed to parse AC criteria: %w", parseErr)
	}

	audit := func(prev, outcome string) error {
		return r.CreateAcAuditEvent(ctx, tx.AcAuditEvent, backlogdto.AcAuditEventData{
			ItemID: parsedID.String(), ItemPublicID: item.PublicID, CriterionIndex: criterionIndex, PreviousStatus: prev,
			NewStatus: change.newStatus, Reason: change.reason, Outcome: outcome,
		})
	}
	reject := func(prev string, cause error) error {
		if err := audit(prev, "rejected:"+cause.Error()); err != nil {
			return errors.Join(cause, err)
		}
		if err := tx.Commit(); err != nil {
			return errors.Join(cause, err)
		}
		return cause
	}

	if criterionIndex < 0 || criterionIndex >= len(criteria) {
		return reject("", fmt.Errorf("%w: index %d (len=%d)", backlogdto.ErrAcCriterionOutOfRange, criterionIndex, len(criteria)))
	}
	prev := string(criteria[criterionIndex].Status)

	if err := mutate(&criteria[criterionIndex]); err != nil {
		return reject(prev, err)
	}

	serialized, serErr := domain.SerializeAcCriteria(criteria)
	if serErr != nil {
		return fmt.Errorf("failed to serialize AC criteria: %w", serErr)
	}

	updated, err := tx.BacklogItem.UpdateOneID(parsedID).
		SetAcceptanceCriteria(string(serialized)).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("failed to save AC criteria update for item %s: %w", itemID, err)
	}
	// Only a real status transition is a mutation worth an audit row; a repeated report of the
	// same status (or a note-only update) is not.
	if prev != change.newStatus {
		if err := audit(prev, "applied"); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit AC update: %w", err)
	}

	// Best-effort publish so live viewers see AC progress without a full refresh.
	result := backlogstore.BacklogItemToData(updated)
	r.publishItemChanged(ctx, &result, backlogdto.BacklogItemChange{
		Kind:          backlogdto.ChangeItemUpdated,
		UpdatedFields: []string{"acceptanceCriteria"},
	})

	return nil
}

// --- Zombie-session review detection (pre-mortem F3) ---

// --- Bouncing (non-converging cycle) detection ---

// Jules-specific queries (ListOpenJulesItemSessions, CountJulesItemSessionsSince,
// TouchItemSessionProgress) live in ent_repository_backlog_jules.go, split out to
// keep this file under the file-length-limit gate.
