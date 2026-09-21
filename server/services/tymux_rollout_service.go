package services

import (
	"context"

	"connectrpc.com/connect"
	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// TymuxRolloutService handles the GetTymuxRolloutStatus/
// CompleteTymuxRollbackRehearsal/SetTymuxSessionOverride/
// SetTymuxGlobalOverride RPCs — the operator-facing controls for the
// tymux-bundled-integration rollout, mirroring StreamHubRolloutService's
// shape exactly: a config-backed handler with no second implementation, so
// a concrete type per the `interface-pollution-checklist` skill.
//
// The global default is the "tymux" feature flag (config.TymuxFeatureFlag,
// off by default) — SetTymuxGlobalOverride sets or clears that flag live
// from the browser, no restart required.
type TymuxRolloutService struct{}

// NewTymuxRolloutService creates a TymuxRolloutService.
func NewTymuxRolloutService() *TymuxRolloutService {
	return &TymuxRolloutService{}
}

// status builds the current TymuxRolloutStatus from live config — shared by
// all four RPCs since each returns the post-mutation status.
func (s *TymuxRolloutService) status() *sessionv1.TymuxRolloutStatus {
	cfg := config.LoadConfig()

	var rehearsalCompletedAt *timestamppb.Timestamp
	if cfg.TymuxRollbackRehearsalCompletedAt != nil && !cfg.TymuxRollbackRehearsalCompletedAt.IsZero() {
		rehearsalCompletedAt = timestamppb.New(*cfg.TymuxRollbackRehearsalCompletedAt)
	}

	overrides := make([]*sessionv1.TymuxSessionOverrideEntry, 0, len(cfg.TymuxSessionOverrides))
	for name, forceTymux := range cfg.TymuxSessionOverrides {
		overrides = append(overrides, &sessionv1.TymuxSessionOverrideEntry{
			SessionName: name,
			ForceTymux:  forceTymux,
		})
	}

	var globalOverride *bool
	if v, ok := cfg.GetTymuxGlobalOverride(); ok {
		globalOverride = &v
	}

	return &sessionv1.TymuxRolloutStatus{
		// The STAPLER_SQUAD_USE_TYMUX env var was removed in favor of the
		// "tymux" feature flag (GlobalOverride below) — always false now.
		GlobalEnvVarSet:              false,
		RollbackRehearsalCompletedAt: rehearsalCompletedAt,
		SessionOverrides:             overrides,
		GlobalOverride:               globalOverride,
	}
}

// GetTymuxRolloutStatus returns the current rollout status.
// +api: tymux-rollout:get
func (s *TymuxRolloutService) GetTymuxRolloutStatus(
	ctx context.Context,
	req *connect.Request[sessionv1.GetTymuxRolloutStatusRequest],
) (*connect.Response[sessionv1.TymuxRolloutStatus], error) {
	return connect.NewResponse(s.status()), nil
}

// CompleteTymuxRollbackRehearsal records that the tymux backend's rollback
// rehearsal has been performed. Historical record only — the global default
// no longer gates on it (see config.EffectiveTymuxEnabled).
// +api: tymux-rollout:complete-rehearsal
func (s *TymuxRolloutService) CompleteTymuxRollbackRehearsal(
	ctx context.Context,
	req *connect.Request[sessionv1.CompleteTymuxRollbackRehearsalRequest],
) (*connect.Response[sessionv1.TymuxRolloutStatus], error) {
	cfg := config.LoadConfig()
	if err := cfg.RecordTymuxRollbackRehearsalCompleted(); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(s.status()), nil
}

// SetTymuxSessionOverride sets or clears a per-session canary override.
// req.Msg.ForceTymux is presence-tracked (proto3 optional): unset clears any
// existing override for the session (falls back to the global default),
// while a present true/false pins the session onto tymux/legacy tmux
// respectively — see config.SetTymuxSessionOverride's doc comment for the
// tri-state semantics this passes straight through.
// +api: tymux-rollout:set-session-override
func (s *TymuxRolloutService) SetTymuxSessionOverride(
	ctx context.Context,
	req *connect.Request[sessionv1.SetTymuxSessionOverrideRequest],
) (*connect.Response[sessionv1.TymuxRolloutStatus], error) {
	cfg := config.LoadConfig()
	if err := cfg.SetTymuxSessionOverride(req.Msg.GetSessionName(), req.Msg.ForceTymux); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(s.status()), nil
}

// SetTymuxGlobalOverride sets or clears the "tymux" feature flag. Takes
// effect immediately for sessions created after this call — no process
// restart required.
// +api: tymux-rollout:set-global-override
func (s *TymuxRolloutService) SetTymuxGlobalOverride(
	ctx context.Context,
	req *connect.Request[sessionv1.SetTymuxGlobalOverrideRequest],
) (*connect.Response[sessionv1.TymuxRolloutStatus], error) {
	cfg := config.LoadConfig()
	if err := cfg.SetTymuxGlobalOverride(req.Msg.ForceTymux); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(s.status()), nil
}
