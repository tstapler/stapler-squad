package services

import (
	"context"

	"connectrpc.com/connect"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/services/escapecode"
	"github.com/tstapler/stapler-squad/server/services/taggingrules"
)

// escapeQueries is built per call so it sees the client and reader installed after construction.
func (s *SessionService) escapeQueries() *escapecode.QueryService {
	return escapecode.NewQueryService(s.escapeEventsClient, s.escapeAggregates)
}

// tagClassifier is built per call because SetSessionTagPoller runs after construction.
// A nil poller must reach the service as a nil interface, not a typed nil pointer.
func (s *SessionService) tagClassifier() *taggingrules.ClassifierService {
	if s.sessionTagPoller == nil {
		return taggingrules.NewClassifierService(nil)
	}
	return taggingrules.NewClassifierService(s.sessionTagPoller)
}

// QueryEscapeAnalytics returns paginated escape event records for a session.
// +api: escape:query
func (s *SessionService) QueryEscapeAnalytics(
	ctx context.Context,
	req *connect.Request[sessionv1.QueryEscapeAnalyticsRequest],
) (*connect.Response[sessionv1.QueryEscapeAnalyticsResponse], error) {
	return s.escapeQueries().QueryEscapeAnalytics(ctx, req)
}

// GetEscapeAnalyticsSummary returns aggregate escape sequence statistics for a session.
// +api: escape:summary
func (s *SessionService) GetEscapeAnalyticsSummary(
	ctx context.Context,
	req *connect.Request[sessionv1.GetEscapeAnalyticsSummaryRequest],
) (*connect.Response[sessionv1.GetEscapeAnalyticsSummaryResponse], error) {
	return s.escapeQueries().GetEscapeAnalyticsSummary(ctx, req)
}

// GetEscapeAnalyticsGlobalSummary returns escape sequence statistics across all sessions.
// +api: analytics:get-escape-global-summary
func (s *SessionService) GetEscapeAnalyticsGlobalSummary(
	ctx context.Context,
	req *connect.Request[sessionv1.GetEscapeAnalyticsGlobalSummaryRequest],
) (*connect.Response[sessionv1.GetEscapeAnalyticsGlobalSummaryResponse], error) {
	return s.escapeQueries().GetEscapeAnalyticsGlobalSummary(ctx, req)
}

// GetTaggingClassifierConfig returns the effective LLM model hierarchy for session tagging.
// +api: tagging-classifier:get-config
func (s *SessionService) GetTaggingClassifierConfig(
	ctx context.Context,
	req *connect.Request[sessionv1.GetTaggingClassifierConfigRequest],
) (*connect.Response[sessionv1.GetTaggingClassifierConfigResponse], error) {
	return s.tagClassifier().GetTaggingClassifierConfig(ctx, req)
}

// UpdateTaggingClassifierConfig persists and hot-applies a new model hierarchy.
// +api: tagging-classifier:update-config
func (s *SessionService) UpdateTaggingClassifierConfig(
	ctx context.Context,
	req *connect.Request[sessionv1.UpdateTaggingClassifierConfigRequest],
) (*connect.Response[sessionv1.UpdateTaggingClassifierConfigResponse], error) {
	return s.tagClassifier().UpdateTaggingClassifierConfig(ctx, req)
}

// ReclassifySessionTags re-runs the LLM tag classifier for one session.
// +api: tagging-classifier:reclassify-session
func (s *SessionService) ReclassifySessionTags(
	ctx context.Context,
	req *connect.Request[sessionv1.ReclassifySessionTagsRequest],
) (*connect.Response[sessionv1.ReclassifySessionTagsResponse], error) {
	return s.tagClassifier().ReclassifySessionTags(ctx, req)
}
