package services

import (
	"context"
	"fmt"
	"sync"

	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"

	"connectrpc.com/connect"
)

// CallbackConfigService handles the GetCallbackConfig/UpdateCallbackConfig RPCs
// (webhook-triggers Phase 5, FR7). Delegated to from SessionService exactly like
// DefaultsService — a config-backed handler with no second implementation, so a
// concrete type per .claude/rules/interface-pollution-checklist.md.
type CallbackConfigService struct {
	// sharedCfg and sharedCfgMu, when wired via SetSharedCallbackConfig, are the
	// SAME *config.Config instance (and guarding mutex) CallbackDispatcher reads
	// Callbacks from at Dispatch time (server/dependencies.go wires this with
	// CallbackDispatcher's own cfg pointer and ConfigMu()). Mirrors
	// DefaultsService.sharedBacklogCfg's doc comment (PR #199 review F1) applied
	// to the identical bug: without this, UpdateCallbackConfig's own fresh
	// config.LoadConfig()+SaveConfig() only ever touched a config instance
	// nobody else read from — CallbackDispatcher's cfg was loaded once at
	// process start and never observed the save, so a saved callback URL took
	// effect only after a restart. nil-safe — tests that don't call
	// SetSharedCallbackConfig keep this handler's pre-existing disk-only
	// behavior.
	sharedCfg   *config.Config
	sharedCfgMu *sync.RWMutex
}

// NewCallbackConfigService creates a CallbackConfigService.
func NewCallbackConfigService() *CallbackConfigService {
	return &CallbackConfigService{}
}

// SetSharedCallbackConfig wires the live *config.Config instance (and its
// guarding mutex) that CallbackDispatcher.Dispatch reads callback URLs from —
// see sharedCfg's doc comment for why this is needed. Called once from
// server/dependencies.go with the exact same *config.Config pointer and
// *sync.RWMutex passed to services.NewCallbackDispatcher /
// CallbackDispatcher.ConfigMu.
func (c *CallbackConfigService) SetSharedCallbackConfig(cfg *config.Config, mu *sync.RWMutex) {
	c.sharedCfg = cfg
	c.sharedCfgMu = mu
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

	// Propagate the saved URLs onto CallbackDispatcher's live config instance so
	// the change takes effect on the very next Dispatch call instead of
	// requiring a process restart — see sharedCfg's doc comment (this bug's
	// fix, mirroring PR #199 review F1's pattern for BacklogService).
	if c.sharedCfg != nil {
		c.sharedCfgMu.Lock()
		c.sharedCfg.Callbacks = cfg.Callbacks
		c.sharedCfgMu.Unlock()
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
