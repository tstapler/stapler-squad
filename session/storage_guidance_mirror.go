package session

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/tstapler/stapler-squad/session/domain"
)

// CreateMirroredGuidanceRequest forwards to EntRepository.CreateMirroredGuidanceRequest.
func (s *Storage) CreateMirroredGuidanceRequest(ctx context.Context, req domain.GuidanceRequest, capLimit int) (*GuidanceRequestData, error) {
	return s.repo.CreateMirroredGuidanceRequest(ctx, req, capLimit)
}

// SetGuidanceDelivery forwards to EntRepository.SetDelivery.
func (s *Storage) SetGuidanceDelivery(ctx context.Context, id uuid.UUID, route domain.DeliveryRoute, state domain.DeliveryState, addAttempts int) (bool, error) {
	return s.repo.SetDelivery(ctx, id, route, state, addAttempts)
}

// ListOpenGuidanceRequestsByAsker forwards to EntRepository.ListOpenGuidanceRequestsByAsker.
func (s *Storage) ListOpenGuidanceRequestsByAsker(ctx context.Context, askerSessionUUID string, sources ...domain.QuestionSource) ([]*GuidanceRequestData, error) {
	return s.repo.ListOpenGuidanceRequestsByAsker(ctx, askerSessionUUID, sources...)
}

// PurgeTerminalGuidanceQuestions forwards to EntRepository.PurgeTerminalGuidanceQuestions.
func (s *Storage) PurgeTerminalGuidanceQuestions(ctx context.Context, cutoff time.Time) (int, error) {
	return s.repo.PurgeTerminalGuidanceQuestions(ctx, cutoff)
}

// ListGuidanceByDelivery forwards to EntRepository.ListGuidanceByDelivery.
func (s *Storage) ListGuidanceByDelivery(ctx context.Context, route domain.DeliveryRoute, state domain.DeliveryState) ([]*GuidanceRequestData, error) {
	return s.repo.ListGuidanceByDelivery(ctx, route, state)
}
