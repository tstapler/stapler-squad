package wakes

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

// OperatorCancelReason is stored as the cancel reason of a peer-help request
// the operator withdraws.
const OperatorCancelReason = "operator_cancelled"

// CancelAny cancels trigger id on the operator's behalf: unlike Cancel it
// checks no owner. A trigger that already ended reports cancelled=false with
// its row (delivered_at says whether it fired first).
func (d *Dispatcher) CancelAny(ctx context.Context, id string) (row *session.WakeTriggerData, cancelled bool, err error) {
	row, err = d.loadWake(ctx, id)
	if err != nil {
		return nil, false, err
	}
	return d.cancelRow(ctx, row)
}

// loadWake reads trigger id; an unparsable or unknown id is ErrWakeNotFound.
func (d *Dispatcher) loadWake(ctx context.Context, id string) (*session.WakeTriggerData, error) {
	parsed, err := uuid.Parse(id)
	if err != nil {
		return nil, ErrWakeNotFound
	}
	row, err := d.cfg.Store.GetWakeTrigger(ctx, parsed)
	if errors.Is(err, session.ErrNotFound) {
		return nil, ErrWakeNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("cancel wake: %w", err)
	}
	return row, nil
}

// cancelRow applies the guarded cancel to row and re-reads it.
func (d *Dispatcher) cancelRow(ctx context.Context, row *session.WakeTriggerData) (*session.WakeTriggerData, bool, error) {
	parsed, err := uuid.Parse(row.ID)
	if err != nil {
		return nil, false, ErrWakeNotFound
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
