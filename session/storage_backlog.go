package session

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/backlogitem"
	"github.com/tstapler/stapler-squad/session/ent/itemsession"
	"github.com/tstapler/stapler-squad/session/ent/reviewverdict"
)

// ItemSessionData is the input data for creating a new ItemSession.
type ItemSessionData struct {
	ItemID            string // BacklogItem UUID
	SessionUUID       string
	SessionRole       string
	AcSnapshot        AcCriteriaJSON
	TriageResult      string
	VerificationNotes string  // Freeform verification evidence reported via request_review
	EstimatedCostUsd  float64 // Only set for headless sessions where cost is known at creation time
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
	return nil
}

// UpdateItemSessionEnded records the end time for an ItemSession.
func (r *EntRepository) UpdateItemSessionEnded(ctx context.Context, id string, endedAt time.Time) error {
	parsedID, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("invalid id %q: %w", id, err)
	}

	_, err = r.client.ItemSession.UpdateOneID(parsedID).
		SetEndedAt(endedAt).
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
		_, err = tx.ReviewVerdict.Create().
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
		SetNillableTriageResult(nilIfEmpty(isData.TriageResult))
	if isData.EstimatedCostUsd > 0 {
		isq = isq.SetEstimatedCostUsd(isData.EstimatedCostUsd)
	}
	is, err := isq.Save(ctx)
	if err != nil {
		return ItemSessionSummary{}, fmt.Errorf("create item session: %w", err)
	}

	rv, err := tx.ReviewVerdict.Create().
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
		SetItemSessionID(is.ID).
		Save(ctx)
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

	count := 0
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
		count++
	}

	if err = tx.Commit(); err != nil {
		return 0, fmt.Errorf("failed to commit reconcile transaction: %w", err)
	}
	return count, nil
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

	_, err = r.client.BacklogItem.UpdateOneID(parsedID).
		SetAcceptanceCriteria(string(serialized)).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("failed to save AC criteria update for item %s: %w", itemID, err)
	}
	return nil
}
