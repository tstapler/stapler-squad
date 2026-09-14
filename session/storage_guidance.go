package session

import (
	"context"

	"github.com/google/uuid"

	"github.com/tstapler/stapler-squad/session/domain"
)

// This file forwards *EntRepository's GuidanceRequest methods
// (session/ent_repository_guidance.go) through *Storage, mirroring
// GetBacklogItem's forwarding shape — server/mcp and server/services hold a
// *Storage, not a *EntRepository directly, per this codebase's existing
// convention (see e.g. BacklogService/GitHubService's constructors).

// CreateGuidanceRequest forwards to EntRepository.CreateGuidanceRequest.
func (s *Storage) CreateGuidanceRequest(ctx context.Context, in CreateGuidanceRequestInput) (*GuidanceRequestData, error) {
	return s.repo.CreateGuidanceRequest(ctx, in)
}

// AnswerGuidanceRequest forwards to EntRepository.AnswerGuidanceRequest.
func (s *Storage) AnswerGuidanceRequest(ctx context.Context, id uuid.UUID, answer string) (bool, error) {
	return s.repo.AnswerGuidanceRequest(ctx, id, answer)
}

// MarkGuidanceRequestNotified forwards to EntRepository.MarkGuidanceRequestNotified.
func (s *Storage) MarkGuidanceRequestNotified(ctx context.Context, id uuid.UUID) (bool, error) {
	return s.repo.MarkGuidanceRequestNotified(ctx, id)
}

// CancelGuidanceRequest forwards to EntRepository.CancelGuidanceRequest.
func (s *Storage) CancelGuidanceRequest(ctx context.Context, id uuid.UUID, reason string) (bool, error) {
	return s.repo.CancelGuidanceRequest(ctx, id, reason)
}

// GetGuidanceRequest forwards to EntRepository.GetGuidanceRequest.
func (s *Storage) GetGuidanceRequest(ctx context.Context, id uuid.UUID) (*GuidanceRequestData, error) {
	return s.repo.GetGuidanceRequest(ctx, id)
}

// ListPendingGuidanceRequests forwards to EntRepository.ListPendingGuidanceRequests.
func (s *Storage) ListPendingGuidanceRequests(ctx context.Context, scope domain.RequestScope, scopeKey string, cap int) ([]*GuidanceRequestData, int, int, error) {
	return s.repo.ListPendingGuidanceRequests(ctx, scope, scopeKey, cap)
}

// ListGuidanceRequestsForScope forwards to EntRepository.ListGuidanceRequestsForScope.
func (s *Storage) ListGuidanceRequestsForScope(ctx context.Context, scope domain.RequestScope, scopeKey string) ([]*GuidanceRequestData, int, int, error) {
	return s.repo.ListGuidanceRequestsForScope(ctx, scope, scopeKey)
}

// ListAllPendingGuidanceRequests forwards to EntRepository.ListAllPendingGuidanceRequests.
func (s *Storage) ListAllPendingGuidanceRequests(ctx context.Context) ([]*GuidanceRequestData, error) {
	return s.repo.ListAllPendingGuidanceRequests(ctx)
}
