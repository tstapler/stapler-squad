package services

import (
	"context"

	"connectrpc.com/connect"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/services/backgroundmodels"
)

// GetBackgroundModels returns the model/effort overrides plus the built-in defaults.
// +api: background-models:get
func (s *SessionService) GetBackgroundModels(
	ctx context.Context,
	req *connect.Request[sessionv1.GetBackgroundModelsRequest],
) (*connect.Response[sessionv1.GetBackgroundModelsResponse], error) {
	return backgroundmodels.Service{}.GetBackgroundModels(ctx, req)
}

// UpdateBackgroundModels validates and persists the model/effort overrides.
// +api: background-models:update
func (s *SessionService) UpdateBackgroundModels(
	ctx context.Context,
	req *connect.Request[sessionv1.UpdateBackgroundModelsRequest],
) (*connect.Response[sessionv1.UpdateBackgroundModelsResponse], error) {
	return backgroundmodels.Service{}.UpdateBackgroundModels(ctx, req)
}
