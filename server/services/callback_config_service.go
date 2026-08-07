package services

import (
	"context"
	"fmt"

	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"

	"connectrpc.com/connect"
)

// CallbackConfigService handles the GetCallbackConfig/UpdateCallbackConfig RPCs
// (webhook-triggers Phase 5, FR7). Delegated to from SessionService exactly like
// DefaultsService — a config-backed handler with no second implementation, so a
// concrete type per .claude/rules/interface-pollution-checklist.md.
type CallbackConfigService struct{}

// NewCallbackConfigService creates a CallbackConfigService.
func NewCallbackConfigService() *CallbackConfigService {
	return &CallbackConfigService{}
}

// GetCallbackConfig reports which of the three outbound-callback URLs are configured.
// The URLs themselves are never returned (AC4 partial — config side).
func (c *CallbackConfigService) GetCallbackConfig(
	ctx context.Context,
	req *connect.Request[sessionv1.GetCallbackConfigRequest],
) (*connect.Response[sessionv1.GetCallbackConfigResponse], error) {
	cfg := config.LoadConfig()
	return connect.NewResponse(&sessionv1.GetCallbackConfigResponse{
		Config: callbackConfigToProto(cfg.Callbacks),
	}), nil
}

// UpdateCallbackConfig sets one or more outbound-callback URLs. Each provided
// (non-nil) URL is SSRF-validated via ValidateCallbackURL before being persisted
// (AC11, config-save half) — an empty string clears/disables that callback; an unset
// field leaves the existing value unchanged.
func (c *CallbackConfigService) UpdateCallbackConfig(
	ctx context.Context,
	req *connect.Request[sessionv1.UpdateCallbackConfigRequest],
) (*connect.Response[sessionv1.UpdateCallbackConfigResponse], error) {
	cfg := config.LoadConfig()

	if req.Msg.OnSessionCompleteUrl != nil {
		url := req.Msg.GetOnSessionCompleteUrl()
		if url != "" {
			if err := ValidateCallbackURL(ctx, url); err != nil {
				return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("on_session_complete_url: %w", err))
			}
		}
		cfg.Callbacks.OnSessionCompleteURL = url
	}
	if req.Msg.OnSessionStaleUrl != nil {
		url := req.Msg.GetOnSessionStaleUrl()
		if url != "" {
			if err := ValidateCallbackURL(ctx, url); err != nil {
				return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("on_session_stale_url: %w", err))
			}
		}
		cfg.Callbacks.OnSessionStaleURL = url
	}
	if req.Msg.OnQueueItemCreatedUrl != nil {
		url := req.Msg.GetOnQueueItemCreatedUrl()
		if url != "" {
			if err := ValidateCallbackURL(ctx, url); err != nil {
				return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("on_queue_item_created_url: %w", err))
			}
		}
		cfg.Callbacks.OnQueueItemCreatedURL = url
	}

	if err := config.SaveConfig(cfg); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to save config: %w", err))
	}

	log.Info("updated callback config",
		"on_session_complete_configured", cfg.Callbacks.OnSessionCompleteURL != "",
		"on_session_stale_configured", cfg.Callbacks.OnSessionStaleURL != "",
		"on_queue_item_created_configured", cfg.Callbacks.OnQueueItemCreatedURL != "")

	return connect.NewResponse(&sessionv1.UpdateCallbackConfigResponse{
		Config: callbackConfigToProto(cfg.Callbacks),
	}), nil
}

// callbackConfigToProto converts a config.CallbackConfig to its masked (booleans-only)
// proto representation — never echoes the raw URL, per CallbackConfigProto's doc comment.
func callbackConfigToProto(cb config.CallbackConfig) *sessionv1.CallbackConfigProto {
	return &sessionv1.CallbackConfigProto{
		OnSessionCompleteConfigured:  cb.OnSessionCompleteURL != "",
		OnSessionStaleConfigured:     cb.OnSessionStaleURL != "",
		OnQueueItemCreatedConfigured: cb.OnQueueItemCreatedURL != "",
	}
}
