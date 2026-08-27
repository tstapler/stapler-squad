package services

import (
	"context"
	"os"

	"connectrpc.com/connect"
	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// TymuxRolloutService handles the GetTymuxRolloutStatus/
// CompleteTymuxRollbackRehearsal/SetTymuxSessionOverride RPCs — the
// operator-facing controls for the tymux-bundled-integration staged rollout
// (Epic 3.3), mirroring StreamHubRolloutService's shape exactly: a
// config-backed handler with no second implementation, so a concrete type
// per .claude/rules/interface-pollution-checklist.md.
//
// The global STAPLER_SQUAD_USE_TYMUX default is deliberately NOT settable
// here — it's env-var-gated and requires a process restart by design (see
// resolveStartupBackend's callers in main.go), keeping the final
// backend-switch a conscious operator action rather than a UI toggle that
// could silently change live session-creation behavior for every new
// session. What this service exposes is everything that IS safe to change
// live: the rollback-rehearsal completion gate (config.json-backed, wired
// today via config.RecordTymuxRollbackRehearsalCompleted from Epic 3.1) and
// per-session canary overrides (config.SetTymuxSessionOverride/
// TymuxSessionOverrides, Phase 4 Epic 4.1).
type TymuxRolloutService struct{}

// NewTymuxRolloutService creates a TymuxRolloutService.
func NewTymuxRolloutService() *TymuxRolloutService {
	return &TymuxRolloutService{}
}

// status builds the current TymuxRolloutStatus from live config plus the
// process environment — shared by all three RPCs since each returns the
// post-mutation status.
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

	return &sessionv1.TymuxRolloutStatus{
		GlobalEnvVarSet:              os.Getenv("STAPLER_SQUAD_USE_TYMUX") == "true",
		RollbackRehearsalCompletedAt: rehearsalCompletedAt,
		SessionOverrides:             overrides,
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
// rehearsal has been performed, unblocking the global
// STAPLER_SQUAD_USE_TYMUX default from resolving to true
// (config.ResolveGlobalTymuxDefault).
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
