package session

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/tstapler/stapler-squad/session/domain"
)

// ListWakeTriggersByStatus forwards to the repository.
func (s *Storage) ListWakeTriggersByStatus(ctx context.Context, statuses ...domain.WakeStatus) ([]*WakeTriggerData, error) {
	return s.repo.ListWakeTriggersByStatus(ctx, statuses...)
}

// ListPeerGuidanceRequests forwards to the repository.
func (s *Storage) ListPeerGuidanceRequests(ctx context.Context, since time.Time) ([]*GuidanceRequestData, error) {
	return s.repo.ListPeerGuidanceRequests(ctx, since)
}

// ListOpenPeerGuidanceRequests forwards to the repository.
func (s *Storage) ListOpenPeerGuidanceRequests(ctx context.Context) ([]*GuidanceRequestData, error) {
	return s.repo.ListOpenPeerGuidanceRequests(ctx)
}

// AnswerGuidanceRequestOnBehalf forwards to the repository.
func (s *Storage) AnswerGuidanceRequestOnBehalf(ctx context.Context, id uuid.UUID, answer string) (bool, error) {
	return s.repo.AnswerGuidanceRequestOnBehalf(ctx, id, answer)
}
