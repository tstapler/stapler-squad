package wakes

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/tstapler/stapler-squad/session"
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
	row, err = d.loadWake(ctx, id)
	if err != nil {
		return nil, false, err
	}
	if row.OwnerSessionUUID != owner {
		return nil, false, ErrWakeNotOwner
	}
	return d.cancelRow(ctx, row)
}
