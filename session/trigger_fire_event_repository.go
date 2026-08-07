package session

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/triggerfireevent"
)

// ErrDuplicateDelivery is returned by TriggerFireEventRepository.Create when the
// (workflow_id, delivery_id) composite unique index rejects a second concurrent
// insert for the same delivery — see trigger_fire_event.go's schema comment (Epic
// 1.2, pre-mortem P1 #1). Callers must attempt Create first (atomic insert-or-
// conflict) rather than pre-checking existence, which would be a TOCTOU race.
var ErrDuplicateDelivery = errors.New("duplicate delivery")

// TriggerFireEventInput holds the fields for recording a single trigger evaluation
// attempt (fired/no-match/rejected).
type TriggerFireEventInput struct {
	// WorkflowID is nil when the request was rejected before a Workflow could be
	// resolved (e.g. an unknown webhook slug).
	WorkflowID   *uuid.UUID
	Outcome      string // "fired_success" | "fired_failed" | "no_match" | "rejected"
	DeliveryID   string // "" when not applicable (e.g. cron fires) — left unset, not stored as ""
	SessionID    string
	ErrorMessage string
}

// TriggerFireEventRepository defines persistence operations for the trigger-fire audit trail.
type TriggerFireEventRepository interface {
	// Create inserts a new TriggerFireEvent row. Returns ErrDuplicateDelivery when the
	// (workflow_id, delivery_id) unique constraint is violated by a concurrent duplicate
	// delivery.
	Create(ctx context.Context, input TriggerFireEventInput) error
	// ListByWorkflow returns the most recent TriggerFireEvent rows for workflowID,
	// newest first, capped at limit.
	ListByWorkflow(ctx context.Context, workflowID uuid.UUID, limit int) ([]*ent.TriggerFireEvent, error)
}

// EntTriggerFireEventRepository implements TriggerFireEventRepository using the ent ORM.
type EntTriggerFireEventRepository struct {
	client *ent.Client
}

// NewEntTriggerFireEventRepository creates a new ent-backed TriggerFireEvent repository.
func NewEntTriggerFireEventRepository(client *ent.Client) *EntTriggerFireEventRepository {
	return &EntTriggerFireEventRepository{client: client}
}

// Create inserts a new TriggerFireEvent row.
func (r *EntTriggerFireEventRepository) Create(ctx context.Context, input TriggerFireEventInput) error {
	c := r.client.TriggerFireEvent.Create().
		SetOutcome(input.Outcome)

	if input.WorkflowID != nil {
		c.SetWorkflowID(*input.WorkflowID)
	}
	if input.DeliveryID != "" {
		c.SetDeliveryID(input.DeliveryID)
	}
	if input.SessionID != "" {
		c.SetSessionID(input.SessionID)
	}
	if input.ErrorMessage != "" {
		c.SetErrorMessage(input.ErrorMessage)
	}

	if _, err := c.Save(ctx); err != nil {
		if ent.IsConstraintError(err) {
			return fmt.Errorf("%w: workflow_id=%v delivery_id=%q", ErrDuplicateDelivery, input.WorkflowID, input.DeliveryID)
		}
		return fmt.Errorf("create trigger fire event: %w", err)
	}
	return nil
}

// ListByWorkflow returns the most recent TriggerFireEvent rows for workflowID, newest
// first, capped at limit (a value <= 0 defaults to 100).
func (r *EntTriggerFireEventRepository) ListByWorkflow(ctx context.Context, workflowID uuid.UUID, limit int) ([]*ent.TriggerFireEvent, error) {
	if limit <= 0 {
		limit = 100
	}
	events, err := r.client.TriggerFireEvent.Query().
		Where(triggerfireevent.WorkflowID(workflowID)).
		Order(ent.Desc(triggerfireevent.FieldCreatedAt)).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list trigger fire events for workflow %s: %w", workflowID, err)
	}
	return events, nil
}
