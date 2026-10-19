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

// The forwarders below complete the surface server/wakes' dispatcher Store needs.

// CreateWakeTrigger forwards to the repository.
func (s *Storage) CreateWakeTrigger(ctx context.Context, in CreateWakeTriggerInput) (*WakeTriggerData, bool, error) {
	return s.repo.CreateWakeTrigger(ctx, in)
}

// GetWakeTrigger forwards to the repository.
func (s *Storage) GetWakeTrigger(ctx context.Context, id uuid.UUID) (*WakeTriggerData, error) {
	return s.repo.GetWakeTrigger(ctx, id)
}

// MarkWakeDelivered forwards to the repository.
func (s *Storage) MarkWakeDelivered(ctx context.Context, id uuid.UUID, now, nextFire time.Time) (bool, error) {
	return s.repo.MarkWakeDelivered(ctx, id, now, nextFire)
}

// MarkWakeNeedsRevive forwards to the repository.
func (s *Storage) MarkWakeNeedsRevive(ctx context.Context, id uuid.UUID, now time.Time) (bool, error) {
	return s.repo.MarkWakeNeedsRevive(ctx, id, now)
}

// ReadmitWakeTrigger forwards to the repository.
func (s *Storage) ReadmitWakeTrigger(ctx context.Context, id uuid.UUID, now time.Time) (bool, error) {
	return s.repo.ReadmitWakeTrigger(ctx, id, now)
}

// MarkWakeSkipped forwards to the repository.
func (s *Storage) MarkWakeSkipped(ctx context.Context, id uuid.UUID, reason domain.WakeReason, now time.Time) (bool, error) {
	return s.repo.MarkWakeSkipped(ctx, id, reason, now)
}

// DeferWakeTrigger forwards to the repository.
func (s *Storage) DeferWakeTrigger(ctx context.Context, id uuid.UUID, nextFireAt time.Time) (int, bool, error) {
	return s.repo.DeferWakeTrigger(ctx, id, nextFireAt)
}

// SkipWakeOccurrence forwards to the repository.
func (s *Storage) SkipWakeOccurrence(ctx context.Context, id uuid.UUID, nextFireAt time.Time) (bool, error) {
	return s.repo.SkipWakeOccurrence(ctx, id, nextFireAt)
}

// RecordWakeDeliveryAttempt forwards to the repository.
func (s *Storage) RecordWakeDeliveryAttempt(ctx context.Context, id uuid.UUID) error {
	return s.repo.RecordWakeDeliveryAttempt(ctx, id)
}

// ListPendingWakeTriggers forwards to the repository.
func (s *Storage) ListPendingWakeTriggers(ctx context.Context) ([]*WakeTriggerData, error) {
	return s.repo.ListPendingWakeTriggers(ctx)
}

// ListStaleClaimedWakeTriggers forwards to the repository.
func (s *Storage) ListStaleClaimedWakeTriggers(ctx context.Context, claimedBefore time.Time) ([]*WakeTriggerData, error) {
	return s.repo.ListStaleClaimedWakeTriggers(ctx, claimedBefore)
}

// ListNeedsReviveWakeTriggers forwards to the repository.
func (s *Storage) ListNeedsReviveWakeTriggers(ctx context.Context) ([]*WakeTriggerData, error) {
	return s.repo.ListNeedsReviveWakeTriggers(ctx)
}

// SweepTerminalWakeTriggers forwards to the repository.
func (s *Storage) SweepTerminalWakeTriggers(ctx context.Context, now time.Time, retention time.Duration) (int, error) {
	return s.repo.SweepTerminalWakeTriggers(ctx, now, retention)
}

// CancelWakeTrigger forwards to the repository.
func (s *Storage) CancelWakeTrigger(ctx context.Context, id uuid.UUID, now time.Time) (bool, error) {
	return s.repo.CancelWakeTrigger(ctx, id, now)
}

// ListWakeTriggersByOwner forwards to the repository.
func (s *Storage) ListWakeTriggersByOwner(ctx context.Context, owner string, statuses ...domain.WakeStatus) ([]*WakeTriggerData, error) {
	return s.repo.ListWakeTriggersByOwner(ctx, owner, statuses...)
}
