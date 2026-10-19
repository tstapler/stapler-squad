package wakes

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

// Errors the owner-facing API returns; the MCP tools map them to typed results.
var (
	ErrWakeNotFound = errors.New("wake not found")
	// ErrWakeNotOwner: the wake exists but belongs to another session.
	ErrWakeNotOwner = errors.New("wake belongs to another session")
)

// chainWindow is how recently a wake must have been delivered to an owner for
// a new wake from that owner to count as scheduled from the woken turn.
const chainWindow = 15 * time.Minute

// Limits returns the effective, clamped limits table.
func (d *Dispatcher) Limits() Limits { return d.limits }

// NextGeneration is the chain generation of a wake the owner schedules now:
// one more than its most recent wake delivered within chainWindow, else 0. The
// server infers it because an agent that could declare its own generation could
// reset it (Wake limits: the chain cap bounds a woken turn rescheduling itself).
func (d *Dispatcher) NextGeneration(ctx context.Context, owner string) (int, error) {
	rows, err := d.cfg.Store.ListWakeTriggersByOwner(ctx, owner)
	if err != nil {
		return 0, fmt.Errorf("next wake generation for %s: %w", owner, err)
	}
	cutoff := d.cfg.Clock.Now().Add(-chainWindow)
	next := 0
	for _, r := range rows {
		if r.DeliveredAt != nil && r.DeliveredAt.After(cutoff) {
			next = max(next, r.Generation+1)
		}
	}
	return next, nil
}

// ListByOwner returns the owner's triggers, newest first.
func (d *Dispatcher) ListByOwner(ctx context.Context, owner string) ([]*session.WakeTriggerData, error) {
	return d.cfg.Store.ListWakeTriggersByOwner(ctx, owner)
}

// Cancel cancels the owner's own live trigger id. A trigger of another owner is
// ErrWakeNotOwner and is left untouched; one already ended reports
// cancelled=false with its row.
func (d *Dispatcher) Cancel(ctx context.Context, owner, id string) (row *session.WakeTriggerData, cancelled bool, err error) {
	parsed, err := uuid.Parse(id)
	if err != nil {
		return nil, false, ErrWakeNotFound
	}
	row, err = d.cfg.Store.GetWakeTrigger(ctx, parsed)
	if err != nil {
		if errors.Is(err, session.ErrNotFound) {
			return nil, false, ErrWakeNotFound
		}
		return nil, false, fmt.Errorf("cancel wake: %w", err)
	}
	if row.OwnerSessionUUID != owner {
		return nil, false, ErrWakeNotOwner
	}
	applied, err := d.cfg.Store.CancelWakeTrigger(ctx, parsed, d.cfg.Clock.Now())
	if err != nil {
		return nil, false, fmt.Errorf("cancel wake: %w", err)
	}
	if applied {
		d.index.remove(row)
		d.delivery.drop(row.ID)
		d.limiter.release(row.ID)
		d.cfg.Metrics.Inc(MetricTrigger, string(row.Type), string(domain.WakeStatusCancelled))
	}
	row, err = d.cfg.Store.GetWakeTrigger(ctx, parsed)
	if err != nil {
		return nil, applied, fmt.Errorf("cancel wake: reread: %w", err)
	}
	return row, applied, nil
}
