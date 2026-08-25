package services

import (
	"context"
	"os"

	"connectrpc.com/connect"
	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// StreamHubRolloutService handles the GetStreamHubRolloutStatus/
// CompleteStreamHubRollbackRehearsal/SetStreamHubSessionOverride RPCs — the
// web UI's controls for the terminal-multi-connection-streaming staged
// rollout (Story 3.3). Delegated to from SessionService exactly like
// SlackConfigService/CallbackConfigService — a config-backed handler with no
// second implementation, so a concrete type per
// .claude/rules/interface-pollution-checklist.md.
//
// The global STAPLER_SQUAD_USE_STREAM_HUB default is deliberately NOT
// settable here — it's env-var-gated and requires a process restart by
// design (see useStreamHub's doc comment in connectrpc_websocket.go),
// keeping the final rollout step a conscious operator action rather than a
// UI toggle that could silently change live terminal-streaming behavior for
// every connected session. What this service exposes is everything that IS
// safe to change live: the rollback-rehearsal completion gate and
// per-session canary overrides, both config.json-backed.
type StreamHubRolloutService struct{}

// NewStreamHubRolloutService creates a StreamHubRolloutService.
func NewStreamHubRolloutService() *StreamHubRolloutService {
	return &StreamHubRolloutService{}
}

// status builds the current StreamHubRolloutStatus from live config plus the
// process environment — shared by all three RPCs since each returns the
// post-mutation status.
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

	return &sessionv1.StreamHubRolloutStatus{
		GlobalEnvVarSet:              os.Getenv("STAPLER_SQUAD_USE_STREAM_HUB") == "true",
		RollbackRehearsalCompletedAt: rehearsalCompletedAt,
		SessionOverrides:             overrides,
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
// been performed, unblocking the global default from resolving to true.
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
	if err := cfg.SetStreamHubSessionOverride(req.Msg.GetSessionName(), req.Msg.ForceHub); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(s.status()), nil
}
