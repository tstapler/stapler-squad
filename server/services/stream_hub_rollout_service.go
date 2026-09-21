package services

import (
	"context"

	"connectrpc.com/connect"
	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// StreamHubRolloutService handles the GetStreamHubRolloutStatus/
// CompleteStreamHubRollbackRehearsal/SetStreamHubSessionOverride/
// SetStreamHubGlobalOverride RPCs — the web UI's controls for the
// terminal-multi-connection-streaming staged rollout (Story 3.3).
// Delegated to from SessionService exactly like SlackConfigService/
// CallbackConfigService — a config-backed handler with no second
// implementation, so a concrete type per
// the `interface-pollution-checklist` skill.
//
// The global default is the "stream_hub" feature flag
// (config.StreamHubFeatureFlag, on by default) — SetStreamHubGlobalOverride
// sets or clears that flag live from the browser, no restart required.
type StreamHubRolloutService struct {
	// findInstance looks up a live managed/external instance by title, the
	// same lookup SessionService.findInstance provides. Used only to derive
	// a per-session override's TmuxPrefix (custom prefixes are rare but
	// possible); nil-safe (falls back to the default prefix) so tests and
	// callers that never wire it still work for the overwhelmingly common
	// default-prefix case.
	findInstance func(title string) *session.Instance
}

// NewStreamHubRolloutService creates a StreamHubRolloutService. findInstance
// resolves a session title to its live instance so SetStreamHubSessionOverride
// can derive the session's actual TmuxPrefix; pass nil to always assume the
// default prefix (fine for tests and any session that never customizes it).
func NewStreamHubRolloutService(findInstance func(title string) *session.Instance) *StreamHubRolloutService {
	return &StreamHubRolloutService{findInstance: findInstance}
}

// resolveSessionOverrideKey converts a human-supplied session title into the
// tmux-prefixed key StreamOwnershipLock.Resolve actually queries with (see
// streamHubSessionKey's doc comment for why this translation exists at all).
// Looks up the live instance to honor a custom TmuxPrefix when one is
// wired and the session is currently known; falls back to the default
// prefix otherwise -- correct for the default-prefix case even when the
// session doesn't exist yet (e.g. an override set in advance of a session
// that will be (re)created with this exact title).
func (s *StreamHubRolloutService) resolveSessionOverrideKey(title string) string {
	if s.findInstance != nil {
		if inst := s.findInstance(title); inst != nil {
			return tmuxSessionNameForStreamPath(inst)
		}
	}
	return streamHubSessionKey(title, "")
}

// status builds the current StreamHubRolloutStatus from live config —
// shared by all three RPCs since each returns the post-mutation status.
func (s *StreamHubRolloutService) status() *sessionv1.StreamHubRolloutStatus {
	cfg := config.LoadConfig()

	var rehearsalCompletedAt *timestamppb.Timestamp
	if cfg.RollbackRehearsalCompletedAt != nil && !cfg.RollbackRehearsalCompletedAt.IsZero() {
		rehearsalCompletedAt = timestamppb.New(*cfg.RollbackRehearsalCompletedAt)
	}

	overrides := make([]*sessionv1.StreamHubSessionOverrideEntry, 0, len(cfg.StreamHubSessionOverrides))
	for name, forceHub := range cfg.StreamHubSessionOverrides {
		overrides = append(overrides, &sessionv1.StreamHubSessionOverrideEntry{
			SessionName: name,
			ForceHub:    forceHub,
		})
	}

	var globalOverride *bool
	if v, ok := cfg.GetStreamHubGlobalOverride(); ok {
		globalOverride = &v
	}

	return &sessionv1.StreamHubRolloutStatus{
		// The STAPLER_SQUAD_USE_STREAM_HUB env var was removed in favor of
		// the "stream_hub" feature flag (GlobalOverride below) — always
		// false now.
		GlobalEnvVarSet:              false,
		RollbackRehearsalCompletedAt: rehearsalCompletedAt,
		SessionOverrides:             overrides,
		GlobalOverride:               globalOverride,
	}
}

// GetStreamHubRolloutStatus returns the current rollout status.
// +api: stream-hub-rollout:get
func (s *StreamHubRolloutService) GetStreamHubRolloutStatus(
	ctx context.Context,
	req *connect.Request[sessionv1.GetStreamHubRolloutStatusRequest],
) (*connect.Response[sessionv1.StreamHubRolloutStatus], error) {
	return connect.NewResponse(s.status()), nil
}

// CompleteStreamHubRollbackRehearsal records that the rollback rehearsal has
// been performed. Historical record only — the global default no longer
// gates on it (see config.EffectiveStreamHubEnabled).
// +api: stream-hub-rollout:complete-rehearsal
func (s *StreamHubRolloutService) CompleteStreamHubRollbackRehearsal(
	ctx context.Context,
	req *connect.Request[sessionv1.CompleteStreamHubRollbackRehearsalRequest],
) (*connect.Response[sessionv1.StreamHubRolloutStatus], error) {
	cfg := config.LoadConfig()
	if err := cfg.RecordRollbackRehearsalCompleted(); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(s.status()), nil
}

// SetStreamHubSessionOverride sets or clears a per-session canary override.
// +api: stream-hub-rollout:set-session-override
func (s *StreamHubRolloutService) SetStreamHubSessionOverride(
	ctx context.Context,
	req *connect.Request[sessionv1.SetStreamHubSessionOverrideRequest],
) (*connect.Response[sessionv1.StreamHubRolloutStatus], error) {
	cfg := config.LoadConfig()
	key := s.resolveSessionOverrideKey(req.Msg.GetSessionName())
	if err := cfg.SetStreamHubSessionOverride(key, req.Msg.ForceHub); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(s.status()), nil
}

// SetStreamHubGlobalOverride sets or clears the "stream_hub" feature flag.
// Takes effect immediately for session connections resolved after this
// call — no process restart required.
// +api: stream-hub-rollout:set-global-override
func (s *StreamHubRolloutService) SetStreamHubGlobalOverride(
	ctx context.Context,
	req *connect.Request[sessionv1.SetStreamHubGlobalOverrideRequest],
) (*connect.Response[sessionv1.StreamHubRolloutStatus], error) {
	cfg := config.LoadConfig()
	if err := cfg.SetStreamHubGlobalOverride(req.Msg.ForceHub); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(s.status()), nil
}
