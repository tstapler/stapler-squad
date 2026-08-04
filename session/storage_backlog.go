package session

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/backlogitem"
	"github.com/tstapler/stapler-squad/session/ent/backlogstatusevent"
	"github.com/tstapler/stapler-squad/session/ent/backlogstuckstate"
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
func (r *EntRepository) backlogItemForItemSession(ctx context.Context, itemSessionID uuid.UUID) (*BacklogItemData, error) {
	ownerID, err := r.client.ItemSession.Query().
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

// ItemSessionData is the input data for creating a new ItemSession.
type ItemSessionData struct {
	ItemID      string // BacklogItem UUID
	SessionUUID string
	SessionRole string
	AcSnapshot  AcCriteriaJSON
	// PipelineModeSnapshot/PipelineModeSnapshotHash freeze the resolved
	// PipelineMode slug and its content hash at the moment this session
	// first starts — see ItemSessionSummary.PipelineModeSnapshot(Hash).
	PipelineModeSnapshot     string
	PipelineModeSnapshotHash string
	TriageResult             string
	VerificationNotes        string  // Freeform verification evidence reported via request_review
	EstimatedCostUsd         float64 // Only set for headless sessions where cost is known at creation time
}

// ReviewVerdictData is the input data for saving a ReviewVerdict.
type ReviewVerdictData struct {
	ItemSessionID  string
	OverallOutcome ReviewOutcome
	PerCriterion   string // JSON
	Summary        string
	DiffHash       string
	PromptHash     string
	DiffTokenCount int
	DiffTruncated  bool
	OverrideBy     string
	OverrideReason string
	OverrideAt     *time.Time
	// CreatedAt overrides the verdict's created_at when non-zero. Only meant
	// for tests that need to backdate a verdict past
	// reviewVerdictIdleThreshold (session/backlog_lifecycle.go) without
	// sleeping in real time — the review_verdicts.created_at column is
	// Immutable() in the ent schema (no generated Update setter), so this is
	// only settable at creation time. Production callers leave this zero and
	// get the schema's Default(time.Now) behavior.
	CreatedAt time.Time
}

// --- ItemSession ---

// CreateItemSession creates a new ItemSession linked to a BacklogItem.
func (r *EntRepository) CreateItemSession(ctx context.Context, data ItemSessionData) (ItemSessionSummary, error) {
	parsedItemID, err := uuid.Parse(data.ItemID)
	if err != nil {
		return ItemSessionSummary{}, fmt.Errorf("invalid item id %q: %w", data.ItemID, err)
	}

	q := r.client.ItemSession.Create().
		SetSessionUUID(data.SessionUUID).
		SetSessionRole(data.SessionRole).
		SetBacklogItemID(parsedItemID).
		SetNillableAcSnapshot(nilIfEmpty(string(data.AcSnapshot))).
		SetPipelineModeSnapshot(data.PipelineModeSnapshot).
		SetPipelineModeSnapshotHash(data.PipelineModeSnapshotHash).
		SetNillableTriageResult(nilIfEmpty(data.TriageResult)).
		SetNillableVerificationNotes(nilIfEmpty(data.VerificationNotes))
	if data.EstimatedCostUsd > 0 {
		q = q.SetEstimatedCostUsd(data.EstimatedCostUsd)
	}
	is, err := q.Save(ctx)
	if err != nil {
		return ItemSessionSummary{}, fmt.Errorf("failed to create item session: %w", err)
	}
	// BacklogItemID is not loaded via edge on create; set it directly from the input.
	summary := itemSessionToSummary(is)
	summary.BacklogItemID = data.ItemID

	// Best-effort publish: never blocks or fails session creation itself.
	if item, lookupErr := r.GetBacklogItem(ctx, data.ItemID); lookupErr != nil {
		log.WarningLog.Printf("[EntRepository] CreateItemSession: failed to resolve backlog item %s for publish: %v", data.ItemID, lookupErr)
	} else {
		r.attachItemSessionsForPublish(ctx, item)
		r.publishItemChanged(item, BacklogItemChange{
			Kind:      ChangeSessionAttached,
			SessionID: data.SessionUUID,
		})
	}

	return summary, nil
}

// GetItemSession retrieves an ItemSession by entity UUID string. Loads the BacklogItem edge.
func (r *EntRepository) GetItemSession(ctx context.Context, id string) (ItemSessionSummary, error) {
	parsedID, err := uuid.Parse(id)
	if err != nil {
		return ItemSessionSummary{}, fmt.Errorf("%w: invalid id %q: %v", ErrNotFound, id, err)
	}

	is, err := r.client.ItemSession.Query().
		Where(itemsession.ID(parsedID)).
		WithBacklogItem().
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return ItemSessionSummary{}, fmt.Errorf("%w: item session %s", ErrNotFound, id)
		}
		return ItemSessionSummary{}, fmt.Errorf("failed to get item session %s: %w", id, err)
	}
	return itemSessionToSummary(is), nil
}

// ListItemSessions returns all ItemSessions for a given BacklogItem UUID string.
func (r *EntRepository) ListItemSessions(ctx context.Context, itemID string) ([]ItemSessionSummary, error) {
	parsedItemID, err := uuid.Parse(itemID)
	if err != nil {
		return nil, fmt.Errorf("invalid item id %q: %w", itemID, err)
	}

	sessions, err := r.client.ItemSession.Query().
		Where(itemsession.HasBacklogItemWith(backlogitem.ID(parsedItemID))).
		WithReviewVerdict().
		Order(ent.Asc(itemsession.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list item sessions for item %s: %w", itemID, err)
	}
	result := make([]ItemSessionSummary, len(sessions))
	for i, is := range sessions {
		s := itemSessionToSummary(is)
		s.BacklogItemID = itemID
		result[i] = s
	}
	return result, nil
}

// GetBaseCommitSHAsForSessions returns a map of sessionUUID → last_commit_sha for the
// given session UUIDs, including only rows where last_commit_sha is non-empty.
// Used at startup to restore dirBaseSHA for directory-mode backlog sessions.
func (r *EntRepository) GetBaseCommitSHAsForSessions(ctx context.Context, sessionUUIDs []string) (map[string]string, error) {
	rows, err := r.client.ItemSession.Query().
		Where(itemsession.SessionUUIDIn(sessionUUIDs...)).
		Select(itemsession.FieldSessionUUID, itemsession.FieldLastCommitSha).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("GetBaseCommitSHAsForSessions: %w", err)
	}
	result := make(map[string]string, len(rows))
	for _, row := range rows {
		if row.LastCommitSha != "" {
			result[row.SessionUUID] = row.LastCommitSha
		}
	}
	return result, nil
}

// GetItemSessionBySessionUUID looks up the most recent active ItemSession by session UUID alone.
// session_uuid is not unique across records (a session may be reused), so we order by
// created_at descending and take the first match. Returns ErrNotFound if no record exists.
// Loads the BacklogItem edge so BacklogItemID is populated in the returned summary.
func (r *EntRepository) GetItemSessionBySessionUUID(ctx context.Context, sessionUUID string) (ItemSessionSummary, error) {
	is, err := r.client.ItemSession.Query().
		Where(itemsession.SessionUUID(sessionUUID)).
		WithBacklogItem().
		Order(ent.Desc(itemsession.FieldCreatedAt)).
		First(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return ItemSessionSummary{}, fmt.Errorf("%w: item session for session=%s", ErrNotFound, sessionUUID)
		}
		return ItemSessionSummary{}, fmt.Errorf("failed to get item session by session uuid: %w", err)
	}
	return itemSessionToSummary(is), nil
}

// GetItemSessionBySessionAndItem looks up an ItemSession by both sessionUUID and backlog item ID.
func (r *EntRepository) GetItemSessionBySessionAndItem(ctx context.Context, sessionUUID string, itemID string) (ItemSessionSummary, error) {
	parsedItemID, err := uuid.Parse(itemID)
	if err != nil {
		return ItemSessionSummary{}, fmt.Errorf("invalid item id %q: %w", itemID, err)
	}

	is, err := r.client.ItemSession.Query().
		Where(
			itemsession.SessionUUID(sessionUUID),
			itemsession.HasBacklogItemWith(backlogitem.ID(parsedItemID)),
		).
		Order(ent.Desc(itemsession.FieldCreatedAt)).
		First(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return ItemSessionSummary{}, fmt.Errorf("%w: item session for session=%s item=%s", ErrNotFound, sessionUUID, itemID)
		}
		return ItemSessionSummary{}, fmt.Errorf("failed to get item session: %w", err)
	}
	summary := itemSessionToSummary(is)
	summary.BacklogItemID = itemID
	return summary, nil
}

// UpdateItemSessionStarted records the start time for an ItemSession.
func (r *EntRepository) UpdateItemSessionStarted(ctx context.Context, id string, startedAt time.Time) error {
	parsedID, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("invalid id %q: %w", id, err)
	}

	_, err = r.client.ItemSession.UpdateOneID(parsedID).
		SetStartedAt(startedAt).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("failed to set started_at on item session %s: %w", id, err)
	}

	// Best-effort publish: never blocks or fails the update itself. Found
	// missing by the Phase 5 spec-compliance sweep's follow-up pass over the
	// remaining publish-hook bypasses (docs/tasks/backlog-feature-improvement.md).
	if item, lookupErr := r.backlogItemForItemSession(ctx, parsedID); lookupErr != nil {
		log.WarningLog.Printf("[EntRepository] UpdateItemSessionStarted: failed to resolve owning backlog item for item session %s: %v", id, lookupErr)
	} else {
		r.attachItemSessionsForPublish(ctx, item)
		r.publishItemChanged(item, BacklogItemChange{
			Kind:          ChangeItemUpdated,
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

	_, err = r.client.ItemSession.UpdateOneID(parsedID).
		SetSessionUUID(sessionUUID).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("failed to set session_uuid on item session %s: %w", id, err)
	}

	// Best-effort publish: never blocks or fails the update itself.
	if item, lookupErr := r.backlogItemForItemSession(ctx, parsedID); lookupErr != nil {
		log.WarningLog.Printf("[EntRepository] UpdateItemSessionSessionUUID: failed to resolve owning backlog item for item session %s: %v", id, lookupErr)
	} else {
		r.attachItemSessionsForPublish(ctx, item)
		r.publishItemChanged(item, BacklogItemChange{
			Kind:      ChangeSessionAttached,
			SessionID: sessionUUID,
		})
	}

	return nil
}

// UpdateItemSessionEnded records the end time for an ItemSession.
func (r *EntRepository) UpdateItemSessionEnded(ctx context.Context, id string, endedAt time.Time) error {
	return r.UpdateItemSessionEndedWithReason(ctx, id, endedAt, "")
}

// UpdateItemSessionEndedWithReason records the end time for an ItemSession alongside
// classifyHeadlessCallError's bucket (or "" for a successful end) — see the end_reason
// schema comment for why this exists: it lets orphan-recovery sweeps tell a call killed
// by our own graceful shutdown apart from one that failed on its own merits.
func (r *EntRepository) UpdateItemSessionEndedWithReason(ctx context.Context, id string, endedAt time.Time, reason string) error {
	parsedID, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("invalid id %q: %w", id, err)
	}

	_, err = r.client.ItemSession.UpdateOneID(parsedID).
		SetEndedAt(endedAt).
		SetEndReason(reason).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("failed to set ended_at on item session %s: %w", id, err)
	}
	return nil
}

// UpdateItemSessionGitActivity updates git-related fields on an ItemSession.
func (r *EntRepository) UpdateItemSessionGitActivity(ctx context.Context, id string, sha, msg string, commitAt time.Time, commitCount int) error {
	parsedID, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("invalid id %q: %w", id, err)
	}

	_, err = r.client.ItemSession.UpdateOneID(parsedID).
		SetLastCommitSha(sha).
		SetLastCommitMessage(msg).
		SetLastCommitAt(commitAt).
		SetCommitCountSinceSpawn(commitCount).
		SetLastProgressAt(commitAt).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("failed to update git activity on item session %s: %w", id, err)
	}

	// Best-effort publish: never blocks or fails the update itself. Found
	// missing by the Phase 5 spec-compliance sweep's follow-up pass over the
	// remaining publish-hook bypasses (docs/tasks/backlog-feature-improvement.md).
	if item, lookupErr := r.backlogItemForItemSession(ctx, parsedID); lookupErr != nil {
		log.WarningLog.Printf("[EntRepository] UpdateItemSessionGitActivity: failed to resolve owning backlog item for item session %s: %v", id, lookupErr)
	} else {
		r.attachItemSessionsForPublish(ctx, item)
		r.publishItemChanged(item, BacklogItemChange{
			Kind:          ChangeItemUpdated,
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

	_, err = r.client.ItemSession.UpdateOneID(parsedID).
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
		log.WarningLog.Printf("[EntRepository] UpdateItemSessionFileTouch: failed to resolve owning backlog item for item session %s: %v", id, lookupErr)
	} else {
		r.attachItemSessionsForPublish(ctx, item)
		r.publishItemChanged(item, BacklogItemChange{
			Kind:          ChangeItemUpdated,
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

	_, err = r.client.ItemSession.UpdateOneID(parsedID).
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
		log.WarningLog.Printf("[EntRepository] UpdateItemSessionTriageResult: failed to resolve owning backlog item for item session %s: %v", id, lookupErr)
	} else {
		r.attachItemSessionsForPublish(ctx, item)
		r.publishItemChanged(item, BacklogItemChange{
			Kind:          ChangeTriageProgressUpdated,
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

	_, err = r.client.ItemSession.UpdateOneID(parsedID).
		SetVerificationNotes(verificationNotes).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("failed to update verification_notes on item session %s: %w", id, err)
	}

	// Best-effort publish: never blocks or fails the update itself. Found
	// missing by the Phase 5 spec-compliance sweep's follow-up pass over the
	// remaining publish-hook bypasses (docs/tasks/backlog-feature-improvement.md).
	if item, lookupErr := r.backlogItemForItemSession(ctx, parsedID); lookupErr != nil {
		log.WarningLog.Printf("[EntRepository] UpdateItemSessionVerificationNotes: failed to resolve owning backlog item for item session %s: %v", id, lookupErr)
	} else {
		r.attachItemSessionsForPublish(ctx, item)
		r.publishItemChanged(item, BacklogItemChange{
			Kind:          ChangeItemUpdated,
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
func (r *EntRepository) SaveReviewVerdict(ctx context.Context, itemSessionID string, verdict ReviewVerdictData) error {
	parsedSessionID, err := uuid.Parse(itemSessionID)
	if err != nil {
		return fmt.Errorf("invalid item session id %q: %w", itemSessionID, err)
	}

	tx, err := r.client.Tx(ctx)
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
		createQ := tx.ReviewVerdict.Create().
			SetOverallOutcome(string(verdict.OverallOutcome)).
			SetNillablePerCriterion(nilIfEmpty(verdict.PerCriterion)).
			SetNillableSummary(nilIfEmpty(verdict.Summary)).
			SetNillableDiffHash(nilIfEmpty(verdict.DiffHash)).
			SetNillablePromptHash(nilIfEmpty(verdict.PromptHash)).
			SetDiffTokenCount(verdict.DiffTokenCount).
			SetDiffTruncated(verdict.DiffTruncated).
			SetNillableOverrideBy(nilIfEmpty(verdict.OverrideBy)).
			SetNillableOverrideReason(nilIfEmpty(verdict.OverrideReason)).
			SetNillableOverrideAt(verdict.OverrideAt).
			SetItemSessionID(parsedSessionID)
		if !verdict.CreatedAt.IsZero() {
			createQ = createQ.SetCreatedAt(verdict.CreatedAt)
		}
		_, err = createQ.Save(ctx)
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
		log.WarningLog.Printf("[EntRepository] SaveReviewVerdict: failed to resolve owning backlog item for item session %s: %v", itemSessionID, lookupErr)
	} else {
		r.attachItemSessionsForPublish(ctx, item)
		r.publishItemChanged(item, BacklogItemChange{
			Kind:    ChangeVerdictRecorded,
			Verdict: &verdict,
		})
	}

	return nil
}

// CreateItemSessionWithVerdict atomically creates an ItemSession and its initial
// ReviewVerdict in a single transaction. If the verdict write fails the ItemSession
// is rolled back, preventing dangling sessions with no verdict.
func (r *EntRepository) CreateItemSessionWithVerdict(ctx context.Context, isData ItemSessionData, verdict ReviewVerdictData) (ItemSessionSummary, error) {
	parsedItemID, err := uuid.Parse(isData.ItemID)
	if err != nil {
		return ItemSessionSummary{}, fmt.Errorf("invalid item id %q: %w", isData.ItemID, err)
	}

	tx, err := r.client.Tx(ctx)
	if err != nil {
		return ItemSessionSummary{}, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	isq := tx.ItemSession.Create().
		SetSessionUUID(isData.SessionUUID).
		SetSessionRole(isData.SessionRole).
		SetBacklogItemID(parsedItemID).
		SetNillableAcSnapshot(nilIfEmptyJSON(isData.AcSnapshot)).
		SetPipelineModeSnapshot(isData.PipelineModeSnapshot).
		SetPipelineModeSnapshotHash(isData.PipelineModeSnapshotHash).
		SetNillableTriageResult(nilIfEmpty(isData.TriageResult))
	if isData.EstimatedCostUsd > 0 {
		isq = isq.SetEstimatedCostUsd(isData.EstimatedCostUsd)
	}
	is, err := isq.Save(ctx)
	if err != nil {
		return ItemSessionSummary{}, fmt.Errorf("create item session: %w", err)
	}

	verdictQ := tx.ReviewVerdict.Create().
		SetOverallOutcome(string(verdict.OverallOutcome)).
		SetNillablePerCriterion(nilIfEmpty(verdict.PerCriterion)).
		SetNillableSummary(nilIfEmpty(verdict.Summary)).
		SetNillableDiffHash(nilIfEmpty(verdict.DiffHash)).
		SetNillablePromptHash(nilIfEmpty(verdict.PromptHash)).
		SetDiffTokenCount(verdict.DiffTokenCount).
		SetDiffTruncated(verdict.DiffTruncated).
		SetNillableOverrideBy(nilIfEmpty(verdict.OverrideBy)).
		SetNillableOverrideReason(nilIfEmpty(verdict.OverrideReason)).
		SetNillableOverrideAt(verdict.OverrideAt).
		SetItemSessionID(is.ID)
	if !verdict.CreatedAt.IsZero() {
		verdictQ = verdictQ.SetCreatedAt(verdict.CreatedAt)
	}
	rv, err := verdictQ.Save(ctx)
	if err != nil {
		return ItemSessionSummary{}, fmt.Errorf("create review verdict: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return ItemSessionSummary{}, fmt.Errorf("commit: %w", err)
	}

	// Build summary with the verdict embedded.
	is.Edges.ReviewVerdict = rv
	summary := itemSessionToSummary(is)
	summary.BacklogItemID = isData.ItemID

	// Best-effort publish: never blocks or fails the create+verdict itself.
	// Same ChangeVerdictRecorded kind as SaveReviewVerdict's hook above so both
	// verdict-recording paths (RPC and MCP submit_review_verdict) converge on
	// one event kind, each carrying the verdict inline.
	if item, lookupErr := r.GetBacklogItem(ctx, isData.ItemID); lookupErr != nil {
		log.WarningLog.Printf("[EntRepository] CreateItemSessionWithVerdict: failed to resolve backlog item %s for publish: %v", isData.ItemID, lookupErr)
	} else {
		r.attachItemSessionsForPublish(ctx, item)
		r.publishItemChanged(item, BacklogItemChange{
			Kind:    ChangeVerdictRecorded,
			Verdict: &verdict,
		})
	}

	return summary, nil
}

// --- Reconciler ---

// ReconcileStuckItems finds in_progress items whose all linked ItemSessions have ended,
// and transitions them to review status. Returns the count of transitioned items.
// All updates are wrapped in a single transaction so they succeed or fail atomically.
func (r *EntRepository) ReconcileStuckItems(ctx context.Context) (int, error) {
	// Find in_progress items that have at least one item session, where none have nil ended_at.
	items, err := r.client.BacklogItem.Query().
		Where(
			backlogitem.Status(string(BacklogStatusInProgress)),
			backlogitem.HasItemSessions(),
			backlogitem.Not(
				backlogitem.HasItemSessionsWith(itemsession.EndedAtIsNil()),
			),
		).
		All(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to query stuck items: %w", err)
	}

	if len(items) == 0 {
		return 0, nil
	}

	tx, err := r.client.Tx(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	var transitionedIDs []uuid.UUID
	now := time.Now()
	for _, item := range items {
		note := item.Notes
		if note != "" {
			note += "\n"
		}
		note += "[auto] Transitioned to review: all work sessions ended."
		_, updateErr := tx.BacklogItem.UpdateOne(item).
			SetStatus(string(BacklogStatusReview)).
			SetUserModifiedStatusAt(now).
			SetNotes(note).
			Save(ctx)
		if updateErr != nil {
			continue
		}
		recordStatusEvent(ctx, tx.BacklogStatusEvent, item.ID, item.Status, string(BacklogStatusReview), TriggeredBySystem, "")
		transitionedIDs = append(transitionedIDs, item.ID)
	}

	if err = tx.Commit(); err != nil {
		return 0, fmt.Errorf("failed to commit reconcile transaction: %w", err)
	}

	// Best-effort publish: this reconciler mutates status via a raw
	// transaction rather than going through TransitionBacklogItemStatus, so
	// without this it would silently bypass the live-event stream — exactly
	// the "missed call site" failure mode requirements.md's Feasibility Risks
	// section calls out for internal reconcilers that touch storage directly.
	for _, id := range transitionedIDs {
		updated, getErr := r.client.BacklogItem.Get(ctx, id)
		if getErr != nil {
			log.WarningLog.Printf("[EntRepository] ReconcileStuckItems: failed to reload item %s for publish: %v", id, getErr)
			continue
		}
		result := backlogItemToData(updated)
		r.attachItemSessionsForPublish(ctx, &result)
		r.publishItemChanged(&result, BacklogItemChange{
			Kind:      ChangeStatusTransition,
			OldStatus: string(BacklogStatusInProgress),
			NewStatus: string(BacklogStatusReview),
		})
	}

	return len(transitionedIDs), nil
}

// FindReviewItemsWithoutGate returns backlog items in "review" status that have
// no review ItemSession. These are items where the review gate was never spawned
// (e.g. the headless pool was unavailable at the time of the work session exit).
// Each returned item has its ItemSessions edge loaded (work sessions only).
func (r *EntRepository) FindReviewItemsWithoutGate(ctx context.Context) ([]*ent.BacklogItem, error) {
	items, err := r.client.BacklogItem.Query().
		Where(
			backlogitem.Status(string(BacklogStatusReview)),
			backlogitem.SkipReviewGate(false),
			backlogitem.Not(backlogitem.HasItemSessionsWith(itemsession.SessionRole(SessionRoleReview))),
		).
		WithItemSessions(func(q *ent.ItemSessionQuery) {
			q.Where(itemsession.SessionRole(SessionRoleWork)).
				Order(ent.Desc(itemsession.FieldCreatedAt)).
				Limit(1)
		}).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query review items without gate: %w", err)
	}
	return items, nil
}

// FindStuckReviewItems returns backlog items in "review" status that already
// have at least one review ItemSession (so FindReviewItemsWithoutGate's "no
// gate at all" filter won't catch them) but currently have no active (EndedAt
// still nil) review or work session — i.e. nothing is in flight for the item,
// yet it never resolved to done/in_progress/pr_pending.
//
// This is the class of item left behind when AutoReopenAfterFailedReview's
// spawn attempt failed and rolled the status back to "review" (e.g. blocked by
// hasActiveWorkSession because the prior work session's underlying tmux/CLI
// session never got marked ended), or when a legacy/interactive review session
// exited without ever calling submit_review_verdict. Both cases leave the item
// permanently invisible to every other reconciler: FindReviewItemsWithoutGate
// excludes it (a review session does exist), and reconcileStaleWorkSessions
// only scans "in_progress" items. Found via manual QA against a live-data item
// stuck in review for 24+ hours with three UNVERIFIABLE re-review verdicts and
// only ever one work session on record.
func (r *EntRepository) FindStuckReviewItems(ctx context.Context) ([]*ent.BacklogItem, error) {
	items, err := r.client.BacklogItem.Query().
		Where(
			backlogitem.Status(string(BacklogStatusReview)),
			backlogitem.HasItemSessionsWith(itemsession.SessionRole(SessionRoleReview)),
			backlogitem.Not(backlogitem.HasItemSessionsWith(
				itemsession.EndedAtIsNil(),
				itemsession.SessionRoleIn(SessionRoleReview, SessionRoleWork),
			)),
		).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query stuck review items: %w", err)
	}
	return items, nil
}

// FindPRPendingItems returns backlog items in "pr_pending" status that have a
// PR number set. Used by ReconcilePRPending to poll for merged PRs.
func (r *EntRepository) FindPRPendingItems(ctx context.Context) ([]*ent.BacklogItem, error) {
	items, err := r.client.BacklogItem.Query().
		Where(
			backlogitem.Status(string(BacklogStatusPRPending)),
			backlogitem.PrNumberGT(0),
		).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query pr_pending items: %w", err)
	}
	return items, nil
}

// FindDriftedPRItems returns backlog items with a live PR reference (a
// non-zero pr_number and non-empty pr_url) whose status is neither
// "pr_pending" nor a terminal state (done/archived) — i.e. items that
// ReconcilePRPending's FindPRPendingItems can never see because it anchors
// purely on status=="pr_pending", even though the item demonstrably has a
// real PR that needs the same merge/CI polling. This happens when
// pushAndCreatePR/shipViaAgentOrFallback persist prNumber/prUrl (which they
// do unconditionally, before attempting the status transition) but the
// follow-up CAS transition to pr_pending then loses a race to some other
// legitimate concurrent event — e.g. markAbandonedReview's grace period
// firing and respawning a review pass while an agent-driven ship is still
// mid-flight, or a rework/bounce cycle that exhausts its cap before ever
// re-shipping. Confirmed live 2026-07-20 on two items (c2ad7bf3-91bf-4d47-
// 8654-0f2f20869080, PR #251; 6700a3f2-8c0d-4a98-8bbd-39515d5391b1, PR #172)
// stuck at status="review" with real, still-open PRs neither
// ReconcilePRPending nor any other reconciler was polling.
//
// Excludes items with an active (EndedAt still nil) work or review session:
// recovery must never steal an item out from under a live, still-legitimately
// -running session — mirrors AutoReopenForPRFix's/AutoRespawnReview's
// identical hasActiveWorkSession/hasActiveReviewSession guard. An item with a
// genuinely active session will naturally reappear in this query once that
// session ends without making further progress.
func (r *EntRepository) FindDriftedPRItems(ctx context.Context) ([]*ent.BacklogItem, error) {
	items, err := r.client.BacklogItem.Query().
		Where(
			backlogitem.PrNumberGT(0),
			backlogitem.PrURLNEQ(""),
			backlogitem.StatusNotIn(
				string(BacklogStatusPRPending),
				string(BacklogStatusDone),
				string(BacklogStatusArchived),
			),
			backlogitem.Not(backlogitem.HasItemSessionsWith(
				itemsession.EndedAtIsNil(),
				itemsession.SessionRoleIn(SessionRoleReview, SessionRoleWork),
			)),
		).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query drifted PR items: %w", err)
	}
	return items, nil
}

// prNumberFromURLRe extracts the trailing PR number from a GitHub PR URL,
// e.g. "https://github.com/owner/repo/pull/148" -> 148.
var prNumberFromURLRe = regexp.MustCompile(`/pull/(\d+)/?$`)

// BackfillMissingPRNumbers finds pr_pending items with a pr_url but no
// pr_number (pr_number == 0) and parses the number out of the URL. Such items
// are otherwise permanently invisible to FindPRPendingItems' PrNumberGT(0)
// filter, so ReconcilePRPending never polls them — a real stuck-forever case
// found via manual QA against live data, not something the loop itself would
// ever have surfaced. Best-effort: a URL that doesn't match is left as-is and
// logged, not treated as fatal.
func (r *EntRepository) BackfillMissingPRNumbers(ctx context.Context) (int, error) {
	items, err := r.client.BacklogItem.Query().
		Where(
			backlogitem.Status(string(BacklogStatusPRPending)),
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
		m := prNumberFromURLRe.FindStringSubmatch(item.PrURL)
		if m == nil {
			continue
		}
		num, convErr := strconv.Atoi(m[1])
		if convErr != nil || num <= 0 {
			continue
		}
		updated, updateErr := r.client.BacklogItem.UpdateOneID(item.ID).SetPrNumber(num).Save(ctx)
		if updateErr != nil {
			return backfilled, fmt.Errorf("failed to backfill pr_number for item %s: %w", item.ID, updateErr)
		}
		backfilled++

		// Best-effort publish: never blocks or fails the backfill itself. Found
		// missing by the Phase 5 spec-compliance sweep's follow-up pass over the
		// remaining publish-hook bypasses (docs/tasks/backlog-feature-improvement.md).
		result := backlogItemToData(updated)
		r.attachItemSessionsForPublish(ctx, &result)
		r.publishItemChanged(&result, BacklogItemChange{
			Kind:          ChangeItemUpdated,
			UpdatedFields: []string{"prNumber"},
		})
	}
	return backfilled, nil
}

// --- BacklogStuckState queries ---

// OpenStuckStateData is a projected, already-filtered (open + un-snoozed)
// BacklogStuckState row joined with its parent item's rendering-relevant
// fields. Returned only by FindOpenStuckStates, which applies the "open"
// (resolved_at IS NULL) and "not currently snoozed" filters at the query
// boundary — callers never need to re-check ResolvedAt/SnoozedUntil
// nullability themselves (parse-don't-validate at the repository boundary).
type OpenStuckStateData struct {
	ID                  string
	ItemID              string
	Reason              domain.StuckReason
	FirstDetectedAt     time.Time
	LastCheckedAt       time.Time
	NotifiedAt          *time.Time
	Context             string
	ItemTitle           string
	ItemStatus          BacklogStatus
	PrNumber            int
	PrURL               string
	RemediationAttempts int32
	NextRemediationAt   *time.Time
	GraceBootTime       *time.Time
}

// FindOpenStuckStates returns every BacklogStuckState row that is currently
// open (resolved_at IS NULL) and not currently snoozed (snoozed_until IS NULL
// OR snoozed_until is in the past), eager-loading the parent item so the
// projection carries title/status/pr_number/pr_url for rendering without a
// second round trip.
func (r *EntRepository) FindOpenStuckStates(ctx context.Context) ([]OpenStuckStateData, error) {
	now := time.Now()
	rows, err := r.client.BacklogStuckState.Query().
		Where(
			backlogstuckstate.ResolvedAtIsNil(),
			backlogstuckstate.Or(
				backlogstuckstate.SnoozedUntilIsNil(),
				backlogstuckstate.SnoozedUntilLT(now),
			),
		).
		WithItem().
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query open stuck states: %w", err)
	}

	result := make([]OpenStuckStateData, 0, len(rows))
	for _, row := range rows {
		data := OpenStuckStateData{
			ID:                  row.ID.String(),
			ItemID:              row.ItemID.String(),
			Reason:              domain.StuckReason(row.Reason),
			FirstDetectedAt:     row.FirstDetectedAt,
			LastCheckedAt:       row.LastCheckedAt,
			NotifiedAt:          row.NotifiedAt,
			Context:             row.Context,
			RemediationAttempts: row.RemediationAttempts,
			NextRemediationAt:   row.NextRemediationAt,
			GraceBootTime:       row.GraceBootTime,
		}
		if item := row.Edges.Item; item != nil {
			data.ItemTitle = item.Title
			data.ItemStatus = BacklogStatus(item.Status)
			data.PrNumber = item.PrNumber
			data.PrURL = item.PrURL
		}
		result = append(result, data)
	}
	return result, nil
}

// --- ReviewVerdict lookup ---

// GetMostRecentReviewVerdictForItem returns the OverallOutcome from the
// most recently created ReviewVerdict associated with any ItemSession for the
// given BacklogItem UUID. Returns "" (not an error) when no verdict exists yet.
func (r *EntRepository) GetMostRecentReviewVerdictForItem(ctx context.Context, itemID string) (ReviewOutcome, error) {
	parsedItemID, err := uuid.Parse(itemID)
	if err != nil {
		return "", fmt.Errorf("invalid item id %q: %w", itemID, err)
	}

	// Find the most recent ItemSession for this item that has a review verdict.
	is, err := r.client.ItemSession.Query().
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
	return ReviewOutcome(is.Edges.ReviewVerdict.OverallOutcome), nil
}

// GetRecentReviewVerdictSummaries returns up to limit ReviewVerdicts for the
// given BacklogItem UUID, most recent first. Reuses the existing
// ReviewVerdictSummary DTO (see repository.go) — only OverallOutcome and
// Summary are populated, since that's all callers (IsRepeatedFailure in
// stuck_decisions.go) need.
func (r *EntRepository) GetRecentReviewVerdictSummaries(ctx context.Context, itemID string, limit int) ([]ReviewVerdictSummary, error) {
	parsedItemID, err := uuid.Parse(itemID)
	if err != nil {
		return nil, fmt.Errorf("invalid item id %q: %w", itemID, err)
	}

	sessions, err := r.client.ItemSession.Query().
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

	result := make([]ReviewVerdictSummary, 0, len(sessions))
	for _, is := range sessions {
		if is.Edges.ReviewVerdict == nil {
			continue
		}
		result = append(result, ReviewVerdictSummary{
			ID:             is.Edges.ReviewVerdict.ID.String(),
			OverallOutcome: is.Edges.ReviewVerdict.OverallOutcome,
			Summary:        is.Edges.ReviewVerdict.Summary,
		})
	}
	return result, nil
}

// --- AC criterion update ---

// UpdateAcCriterionStatus updates a single acceptance criterion's status by index.
func (r *EntRepository) UpdateAcCriterionStatus(ctx context.Context, itemID string, criterionIndex int, status string, note string) error {
	parsedID, err := uuid.Parse(itemID)
	if err != nil {
		return fmt.Errorf("invalid item id %q: %w", itemID, err)
	}

	item, err := r.client.BacklogItem.Get(ctx, parsedID)
	if err != nil {
		if ent.IsNotFound(err) {
			return fmt.Errorf("%w: backlog item %s", ErrNotFound, itemID)
		}
		return fmt.Errorf("failed to get backlog item %s: %w", itemID, err)
	}

	criteria, parseErr := ParseAcCriteria(AcCriteriaJSON(item.AcceptanceCriteria))
	if parseErr != nil {
		return fmt.Errorf("failed to parse AC criteria: %w", parseErr)
	}

	if criterionIndex < 0 || criterionIndex >= len(criteria) {
		return fmt.Errorf("criterion index %d out of bounds (len=%d)", criterionIndex, len(criteria))
	}

	criteria[criterionIndex].Status = AcStatus(status)
	if note != "" {
		criteria[criterionIndex].Note = note
	}

	serialized, serErr := SerializeAcCriteria(criteria)
	if serErr != nil {
		return fmt.Errorf("failed to serialize AC criteria: %w", serErr)
	}

	updated, err := r.client.BacklogItem.UpdateOneID(parsedID).
		SetAcceptanceCriteria(string(serialized)).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("failed to save AC criteria update for item %s: %w", itemID, err)
	}

	// Best-effort publish: never blocks or fails the AC-status update itself.
	// Found missing by the same Phase 5 spec-compliance sweep that found the
	// ItemSessions eager-load regression above: this method drove the
	// "N/M done" acceptance-criteria progress badge but never emitted a
	// BacklogItemEvent at all, so live viewers never saw AC progress update
	// until their next full poll/refresh — a real event-system bypass, not
	// just a blanked field.
	result := backlogItemToData(updated)
	r.attachItemSessionsForPublish(ctx, &result)
	r.publishItemChanged(&result, BacklogItemChange{
		Kind:          ChangeItemUpdated,
		UpdatedFields: []string{"acceptanceCriteria"},
	})

	return nil
}

// --- Zombie-session review detection (pre-mortem F3) ---

// FindZombieReviewItems returns backlog items in "review" status that have an
// active (EndedAt IS NULL) review-or-work ItemSession recorded in the DB —
// exactly the class FindStuckReviewItems excludes (its "nothing in flight"
// filter requires no un-ended session at all). Each returned item eager-loads
// only that active session so the caller can verify, via an injected
// liveness checker, whether the underlying tmux/CLI process the row claims
// is active has actually gone away (a zombie: the DB row looks live, the
// process is gone). Not every returned item is a zombie — the caller must
// still check liveness per active session.
func (r *EntRepository) FindZombieReviewItems(ctx context.Context) ([]*ent.BacklogItem, error) {
	items, err := r.client.BacklogItem.Query().
		Where(
			backlogitem.Status(string(BacklogStatusReview)),
			backlogitem.HasItemSessionsWith(
				itemsession.EndedAtIsNil(),
				itemsession.SessionRoleIn(SessionRoleReview, SessionRoleWork),
			),
		).
		WithItemSessions(func(q *ent.ItemSessionQuery) {
			q.Where(itemsession.EndedAtIsNil(), itemsession.SessionRoleIn(SessionRoleReview, SessionRoleWork))
		}).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query zombie-candidate review items: %w", err)
	}
	return items, nil
}

// FindReviewItemsWithUnprocessedVerdict returns backlog items in "review" status
// whose most recent review-role ItemSession already has a terminal ReviewVerdict
// recorded. Distinct from FindZombieReviewItems: that detector requires EVERY open
// review-or-work session on the item to be confirmed dead before acting, but
// AutoReopenAfterFailedReview's live-session-reuse (a work session intentionally
// stays open polling for the verdict once the item is back in "review" — see
// docs/tasks/backlog-feature-improvement.md's "WIP limit now undercounts live
// sessions" finding) means the item never looks like a full zombie even when the
// review session itself died with its verdict never actioned (handleReviewSessionExited
// never fired — a server restart or crash mid-exit, the same class of gap as the
// crash-resilience fixes elsewhere in this package). Each returned item eager-loads
// its review-role sessions (most recent first) with their ReviewVerdict, so the
// caller can act on the newest one without a second round-trip.
func (r *EntRepository) FindReviewItemsWithUnprocessedVerdict(ctx context.Context) ([]*ent.BacklogItem, error) {
	items, err := r.client.BacklogItem.Query().
		Where(
			backlogitem.Status(string(BacklogStatusReview)),
			backlogitem.HasItemSessionsWith(
				itemsession.SessionRole(SessionRoleReview),
				itemsession.HasReviewVerdict(),
			),
		).
		WithItemSessions(func(q *ent.ItemSessionQuery) {
			q.Where(itemsession.SessionRole(SessionRoleReview)).
				WithReviewVerdict().
				Order(ent.Desc(itemsession.FieldCreatedAt))
		}).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query review items with unprocessed verdict: %w", err)
	}
	return items, nil
}

// GetMostRecentStatusEventAt returns the created_at timestamp of the most
// recent BacklogStatusEvent for itemID whose to_status equals toStatus.
// Returns (zero time, false, nil) when no such event exists (e.g. an item
// seeded directly into a status without a recorded transition). Used by the
// abandoned_review 15-minute grace check (abandonedReview pure fn) so a item
// that JUST entered review isn't flagged before the reconciler has had a
// chance to re-spawn a review gate.
func (r *EntRepository) GetMostRecentStatusEventAt(ctx context.Context, itemID string, toStatus BacklogStatus) (time.Time, bool, error) {
	parsedID, err := uuid.Parse(itemID)
	if err != nil {
		return time.Time{}, false, fmt.Errorf("invalid item id %q: %w", itemID, err)
	}
	ev, err := r.client.BacklogStatusEvent.Query().
		Where(
			backlogstatusevent.ItemID(parsedID),
			backlogstatusevent.ToStatus(string(toStatus)),
		).
		Order(ent.Desc(backlogstatusevent.FieldCreatedAt)).
		First(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return time.Time{}, false, nil
		}
		return time.Time{}, false, fmt.Errorf("failed to query most recent status event for item %s: %w", itemID, err)
	}
	return ev.CreatedAt, true, nil
}

// --- Bouncing (non-converging cycle) detection ---

// CountReviewCyclesSince counts in_progress->review BacklogStatusEvent
// transitions for itemID created at or after since — the "round trip" signal
// the bouncing detector (isBouncing) keys off.
func (r *EntRepository) CountReviewCyclesSince(ctx context.Context, itemID string, since time.Time) (int, error) {
	parsedID, err := uuid.Parse(itemID)
	if err != nil {
		return 0, fmt.Errorf("invalid item id %q: %w", itemID, err)
	}
	n, err := r.client.BacklogStatusEvent.Query().
		Where(
			backlogstatusevent.ItemID(parsedID),
			backlogstatusevent.FromStatus(string(BacklogStatusInProgress)),
			backlogstatusevent.ToStatus(string(BacklogStatusReview)),
			backlogstatusevent.CreatedAtGTE(since),
		).
		Count(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to count review cycles for item %s: %w", itemID, err)
	}
	return n, nil
}
