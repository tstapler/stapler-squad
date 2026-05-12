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
	ItemID       string // BacklogItem UUID
	SessionUUID  string
	SessionRole  string
	AcSnapshot   string // JSON
	TriageResult string
}

// ReviewVerdictData is the input data for saving a ReviewVerdict.
type ReviewVerdictData struct {
	ItemSessionID  string
	OverallOutcome string
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
func (r *EntRepository) CreateItemSession(ctx context.Context, data ItemSessionData) (*ent.ItemSession, error) {
	parsedItemID, err := uuid.Parse(data.ItemID)
	if err != nil {
		return nil, fmt.Errorf("invalid item id %q: %w", data.ItemID, err)
	}

	is, err := r.client.ItemSession.Create().
		SetSessionUUID(data.SessionUUID).
		SetSessionRole(data.SessionRole).
		SetBacklogItemID(parsedItemID).
		SetNillableAcSnapshot(&data.AcSnapshot).
		SetNillableTriageResult(&data.TriageResult).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create item session: %w", err)
	}
	return is, nil
}

// GetItemSession retrieves an ItemSession by UUID string.
func (r *EntRepository) GetItemSession(ctx context.Context, id string) (*ent.ItemSession, error) {
	parsedID, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid id %q: %v", ErrNotFound, id, err)
	}

	is, err := r.client.ItemSession.Get(ctx, parsedID)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf("%w: item session %s", ErrNotFound, id)
		}
		return nil, fmt.Errorf("failed to get item session %s: %w", id, err)
	}
	return is, nil
}

// ListItemSessions returns all ItemSessions for a given BacklogItem UUID string.
func (r *EntRepository) ListItemSessions(ctx context.Context, itemID string) ([]*ent.ItemSession, error) {
	parsedItemID, err := uuid.Parse(itemID)
	if err != nil {
		return nil, fmt.Errorf("invalid item id %q: %w", itemID, err)
	}

	sessions, err := r.client.ItemSession.Query().
		Where(itemsession.HasBacklogItemWith(backlogitem.ID(parsedItemID))).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list item sessions for item %s: %w", itemID, err)
	}
	return sessions, nil
}

// GetItemSessionBySessionUUID looks up the most recent active ItemSession by session UUID alone.
// Returns ErrNotFound if no matching record exists. Loads the BacklogItem edge.
func (r *EntRepository) GetItemSessionBySessionUUID(ctx context.Context, sessionUUID string) (*ent.ItemSession, error) {
	is, err := r.client.ItemSession.Query().
		Where(itemsession.SessionUUID(sessionUUID)).
		WithBacklogItem().
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf("%w: item session for session=%s", ErrNotFound, sessionUUID)
		}
		return nil, fmt.Errorf("failed to get item session by session uuid: %w", err)
	}
	return is, nil
}

// GetItemSessionBySessionAndItem looks up an ItemSession by both sessionUUID and backlog item ID.
func (r *EntRepository) GetItemSessionBySessionAndItem(ctx context.Context, sessionUUID string, itemID string) (*ent.ItemSession, error) {
	parsedItemID, err := uuid.Parse(itemID)
	if err != nil {
		return nil, fmt.Errorf("invalid item id %q: %w", itemID, err)
	}

	is, err := r.client.ItemSession.Query().
		Where(
			itemsession.SessionUUID(sessionUUID),
			itemsession.HasBacklogItemWith(backlogitem.ID(parsedItemID)),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf("%w: item session for session=%s item=%s", ErrNotFound, sessionUUID, itemID)
		}
		return nil, fmt.Errorf("failed to get item session: %w", err)
	}
	return is, nil
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

// --- ReviewVerdict ---

// SaveReviewVerdict upserts a ReviewVerdict for a given ItemSession.
func (r *EntRepository) SaveReviewVerdict(ctx context.Context, itemSessionID string, verdict ReviewVerdictData) (*ent.ReviewVerdict, error) {
	parsedSessionID, err := uuid.Parse(itemSessionID)
	if err != nil {
		return nil, fmt.Errorf("invalid item session id %q: %w", itemSessionID, err)
	}

	// Try to find existing verdict for this item session.
	existing, queryErr := r.client.ReviewVerdict.Query().
		Where(reviewverdict.HasItemSessionWith(itemsession.ID(parsedSessionID))).
		Only(ctx)

	if queryErr != nil && !ent.IsNotFound(queryErr) {
		return nil, fmt.Errorf("failed to query existing review verdict: %w", queryErr)
	}

	if ent.IsNotFound(queryErr) || existing == nil {
		// Create new verdict.
		rv, saveErr := r.client.ReviewVerdict.Create().
			SetOverallOutcome(verdict.OverallOutcome).
			SetNillablePerCriterion(&verdict.PerCriterion).
			SetNillableSummary(&verdict.Summary).
			SetNillableDiffHash(&verdict.DiffHash).
			SetNillablePromptHash(&verdict.PromptHash).
			SetDiffTokenCount(verdict.DiffTokenCount).
			SetDiffTruncated(verdict.DiffTruncated).
			SetNillableOverrideBy(&verdict.OverrideBy).
			SetNillableOverrideReason(&verdict.OverrideReason).
			SetNillableOverrideAt(verdict.OverrideAt).
			SetItemSessionID(parsedSessionID).
			Save(ctx)
		if saveErr != nil {
			return nil, fmt.Errorf("failed to create review verdict: %w", saveErr)
		}
		return rv, nil
	}

	// Update existing verdict.
	rv, saveErr := r.client.ReviewVerdict.UpdateOne(existing).
		SetOverallOutcome(verdict.OverallOutcome).
		SetNillablePerCriterion(&verdict.PerCriterion).
		SetNillableSummary(&verdict.Summary).
		SetNillableDiffHash(&verdict.DiffHash).
		SetNillablePromptHash(&verdict.PromptHash).
		SetDiffTokenCount(verdict.DiffTokenCount).
		SetDiffTruncated(verdict.DiffTruncated).
		SetNillableOverrideBy(&verdict.OverrideBy).
		SetNillableOverrideReason(&verdict.OverrideReason).
		SetNillableOverrideAt(verdict.OverrideAt).
		Save(ctx)
	if saveErr != nil {
		return nil, fmt.Errorf("failed to update review verdict: %w", saveErr)
	}
	return rv, nil
}

// --- Reconciler ---

// ReconcileStuckItems finds in_progress items whose all linked ItemSessions have ended,
// and transitions them to review status. Returns the count of transitioned items.
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

	count := 0
	now := time.Now()
	for _, item := range items {
		note := item.Notes
		if note != "" {
			note += "\n"
		}
		note += "[auto] Transitioned to review: all work sessions ended."
		_, updateErr := r.client.BacklogItem.UpdateOne(item).
			SetStatus(string(BacklogStatusReview)).
			SetUserModifiedStatusAt(now).
			SetNotes(note).
			Save(ctx)
		if updateErr != nil {
			continue
		}
		count++
	}
	return count, nil
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

	criteria, parseErr := ParseAcCriteria(item.AcceptanceCriteria)
	if parseErr != nil {
		return fmt.Errorf("failed to parse AC criteria: %w", parseErr)
	}

	if criterionIndex < 0 || criterionIndex >= len(criteria) {
		return fmt.Errorf("criterion index %d out of bounds (len=%d)", criterionIndex, len(criteria))
	}

	criteria[criterionIndex].Status = status
	if note != "" {
		criteria[criterionIndex].Text = criteria[criterionIndex].Text + " [" + note + "]"
	}

	serialized, serErr := SerializeAcCriteria(criteria)
	if serErr != nil {
		return fmt.Errorf("failed to serialize AC criteria: %w", serErr)
	}

	_, err = r.client.BacklogItem.UpdateOneID(parsedID).
		SetAcceptanceCriteria(serialized).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("failed to save AC criteria update for item %s: %w", itemID, err)
	}
	return nil
}
