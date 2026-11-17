package backlogstore

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/ent"
)

// UpdateAcCriterionStatus updates a single acceptance criterion's status by index.
func (b *Backlog) UpdateAcCriterionStatus(ctx context.Context, itemID string, criterionIndex int, status string, note string) error {
	return b.mutateAcCriterion(ctx, itemID, criterionIndex, acChange{newStatus: status, reason: note}, func(c *domain.AcCriterion) error {
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
func (b *Backlog) WaiveAcCriterion(ctx context.Context, itemID string, criterionIndex int, reason string) error {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return backlogdto.ErrWaiverReasonRequired
	}
	actor := domain.AcActorFrom(ctx)
	return b.mutateAcCriterion(ctx, itemID, criterionIndex, acChange{newStatus: string(domain.AcStatusWaived), reason: reason}, func(c *domain.AcCriterion) error {
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
func (b *Backlog) mutateAcCriterion(ctx context.Context, itemID string, criterionIndex int, change acChange, mutate func(*domain.AcCriterion) error) error {
	parsedID, err := b.ResolveBacklogItemLookup(ctx, itemID)
	if err != nil {
		if ent.IsNotFound(err) {
			return fmt.Errorf("%w: backlog item %s", domain.ErrNotFound, itemID)
		}
		return fmt.Errorf("invalid item id %q: %w", itemID, err)
	}

	tx, err := b.client.Tx(ctx)
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
		return b.CreateAcAuditEvent(ctx, tx.AcAuditEvent, backlogdto.AcAuditEventData{
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
	result := BacklogItemToData(updated)
	b.PublishItemChanged(ctx, &result, backlogdto.BacklogItemChange{
		Kind:          backlogdto.ChangeItemUpdated,
		UpdatedFields: []string{"acceptanceCriteria"},
	})

	return nil
}
