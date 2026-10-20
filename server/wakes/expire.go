package wakes

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

type overdueStore interface {
	ListPendingWakeTriggers(ctx context.Context) ([]*session.WakeTriggerData, error)
	MarkWakeMissed(ctx context.Context, id uuid.UUID, reason domain.WakeReason, now time.Time) (bool, error)
}

// ExpireOverdue ends every pending row past expires_at as missed(expired) and
// raises one failure notification per row it ended. It is the sweep for a
// stopped dispatcher; when none runs, the startup reconcile is the backstop.
// MarkWakeMissed is a compare-and-set, so a running dispatcher and this sweep
// never both report one row.
func ExpireOverdue(ctx context.Context, store overdueStore, notifier Notifier, now time.Time) (int, error) {
	rows, err := store.ListPendingWakeTriggers(ctx)
	if err != nil {
		return 0, fmt.Errorf("wakes: list pending for expiry: %w", err)
	}
	ended := 0
	for _, row := range rows {
		if row.ExpiresAt == nil || row.ExpiresAt.After(now) {
			continue
		}
		id, err := uuid.Parse(row.ID)
		if err != nil {
			continue
		}
		applied, err := store.MarkWakeMissed(ctx, id, domain.WakeReasonExpired, now)
		if err != nil {
			return ended, fmt.Errorf("wakes: mark %s missed: %w", row.ID, err)
		}
		if !applied {
			continue
		}
		ended++
		if notifier != nil {
			notifier.Notify(failureNotification(row, domain.WakeStatusMissed, domain.WakeReasonExpired))
		}
	}
	return ended, nil
}
