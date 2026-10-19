package session

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/tstapler/stapler-squad/session/domain"
)

// Wake-trigger forwarders: Storage hides its EntRepository from callers
// outside this package (server/wakes depends on these via its own Store port).

// SetWakeChangeHook forwards to the repository; see EntRepository.SetWakeChangeHook.
func (s *Storage) SetWakeChangeHook(fn func()) { s.repo.SetWakeChangeHook(fn) }

// NextWakeFireAt forwards to the repository.
func (s *Storage) NextWakeFireAt(ctx context.Context) (time.Time, bool, error) {
	return s.repo.NextWakeFireAt(ctx)
}

// ListDueWakeTriggers forwards to the repository.
func (s *Storage) ListDueWakeTriggers(ctx context.Context, now time.Time) ([]*WakeTriggerData, error) {
	return s.repo.ListDueWakeTriggers(ctx, now)
}

// ClaimWakeTrigger forwards to the repository.
func (s *Storage) ClaimWakeTrigger(ctx context.Context, id uuid.UUID, now time.Time) (bool, error) {
	return s.repo.ClaimWakeTrigger(ctx, id, now)
}

// MarkWakeMissed forwards to the repository.
func (s *Storage) MarkWakeMissed(ctx context.Context, id uuid.UUID, reason domain.WakeReason, now time.Time) (bool, error) {
	return s.repo.MarkWakeMissed(ctx, id, reason, now)
}
