package session

import (
	"context"
	"errors"
	"fmt"
	"strings"

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
	// UpdateOutcome transitions an existing row — identified by its (workflow_id,
	// delivery_id) composite key, the same key Create claims atomically — to a final
	// outcome, optionally setting sessionID/errMsg (empty strings leave those fields
	// untouched). Used by the webhook handlers (Epic 2.2/2.3) to move a freshly-claimed
	// "pending" row to "fired_success"/"fired_failed" after the fire attempt completes.
	// Returns an error if no row matches the key.
	UpdateOutcome(ctx context.Context, workflowID uuid.UUID, deliveryID, outcome, sessionID, errMsg string) error
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
		if isDuplicateDeliveryConstraintError(err) {
			return fmt.Errorf("%w: workflow_id=%v delivery_id=%q", ErrDuplicateDelivery, input.WorkflowID, input.DeliveryID)
		}
		return fmt.Errorf("create trigger fire event: %w", err)
	}
	return nil
}

// duplicateDeliveryIndexColumns are the two columns of the unique index ent generates
// for index.Fields("workflow_id", "delivery_id").Unique() (schema/trigger_fire_event.go)
// — the only constraint on this table today that legitimately means "duplicate
// delivery." SQLite's constraint-failure message names "table.column" pairs (e.g.
// "UNIQUE constraint failed: trigger_fire_events.workflow_id,
// trigger_fire_events.delivery_id"), not the ent-generated index name, so both column
// references are checked rather than the index identifier itself.
var duplicateDeliveryIndexColumns = []string{"trigger_fire_events.workflow_id", "trigger_fire_events.delivery_id"}

// isDuplicateDeliveryConstraintError reports whether err is specifically the
// (workflow_id, delivery_id) unique index violation, not just any constraint failure —
// ent.IsConstraintError also matches FK and CHECK violations, which would otherwise be
// silently mis-reported as ErrDuplicateDelivery if such a constraint is ever added to
// this table in the future.
func isDuplicateDeliveryConstraintError(err error) bool {
	if !ent.IsConstraintError(err) {
		return false
	}
	msg := err.Error()
	for _, col := range duplicateDeliveryIndexColumns {
		if !strings.Contains(msg, col) {
			return false
		}
	}
	return true
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

// UpdateOutcome transitions the TriggerFireEvent row matching (workflowID, deliveryID)
// to outcome, optionally setting sessionID/errMsg.
func (r *EntTriggerFireEventRepository) UpdateOutcome(ctx context.Context, workflowID uuid.UUID, deliveryID, outcome, sessionID, errMsg string) error {
	u := r.client.TriggerFireEvent.Update().
		Where(
			triggerfireevent.WorkflowID(workflowID),
			triggerfireevent.DeliveryID(deliveryID),
		).
		SetOutcome(outcome)
	if sessionID != "" {
		u.SetSessionID(sessionID)
	}
	if errMsg != "" {
		u.SetErrorMessage(errMsg)
	}

	n, err := u.Save(ctx)
	if err != nil {
		return fmt.Errorf("update trigger fire event outcome (workflow_id=%s delivery_id=%q): %w", workflowID, deliveryID, err)
	}
	if n == 0 {
		return fmt.Errorf("update trigger fire event outcome: no row found for workflow_id=%s delivery_id=%q", workflowID, deliveryID)
	}
	return nil
}
