package services

import (
	"context"

	"errors"

	"connectrpc.com/connect"
	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/tymux"
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
type TymuxRolloutService struct {
	// detectDaemon and replaceDaemon are seams so tests need no real daemon.
	detectDaemon  func(context.Context, tymux.DaemonConfig) tymux.DaemonVersionInfo
	replaceDaemon func(ctx context.Context, cfg tymux.DaemonConfig, allowInterrupt bool) (tymux.DaemonVersionInfo, error)
}

// NewTymuxRolloutService creates a TymuxRolloutService.
func NewTymuxRolloutService() *TymuxRolloutService {
	return &TymuxRolloutService{detectDaemon: tymux.DetectDaemonVersions, replaceDaemon: tymux.ReplaceDaemon}
}

var tymuxdDirections = map[tymux.VersionDirection]sessionv1.TymuxdVersionDirection{
	tymux.VersionSame:      sessionv1.TymuxdVersionDirection_TYMUXD_VERSION_DIRECTION_SAME,
	tymux.VersionUpgrade:   sessionv1.TymuxdVersionDirection_TYMUXD_VERSION_DIRECTION_UPGRADE,
	tymux.VersionDowngrade: sessionv1.TymuxdVersionDirection_TYMUXD_VERSION_DIRECTION_DOWNGRADE,
}

func daemonVersionProto(info tymux.DaemonVersionInfo) *sessionv1.TymuxdVersionInfo {
	return &sessionv1.TymuxdVersionInfo{
		Running:          info.Running,
		Pid:              int32(info.PID), // #nosec G115 -- pids fit in int32
		RunningVersion:   info.RunningVersion,
		InstalledVersion: info.InstalledVersion,
		Direction:        tymuxdDirections[info.Direction], // zero value (UNSPECIFIED) for unknown
	}
}

// status builds the current TymuxRolloutStatus from live config — shared by
// all four RPCs since each returns the post-mutation status.
func (s *TymuxRolloutService) status() *sessionv1.TymuxRolloutStatus {
	cfg := config.LoadConfig()
	session.RecordBackendInfo(cfg)

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
	status := s.status()
	status.Daemon = daemonVersionProto(s.detectDaemon(ctx, tymux.ResolveDaemonConfig()))
	return connect.NewResponse(status), nil
}

// ReplaceTymuxd stops the running tymuxd and starts the configured binary in its
// place (an upgrade or downgrade), refusing while sessions are live unless the
// caller opts in.
// +api: tymux-rollout:replace-daemon
func (s *TymuxRolloutService) ReplaceTymuxd(
	ctx context.Context,
	req *connect.Request[sessionv1.ReplaceTymuxdRequest],
) (*connect.Response[sessionv1.TymuxRolloutStatus], error) {
	after, err := s.replaceDaemon(ctx, tymux.ResolveDaemonConfig(), req.Msg.GetAllowInterruptLiveSessions())
	if errors.Is(err, tymux.ErrLiveSessionsWouldBeInterrupted) {
		return nil, connect.NewError(connect.CodeFailedPrecondition, err)
	}
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	status := s.status()
	status.Daemon = daemonVersionProto(after)
	return connect.NewResponse(status), nil
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
