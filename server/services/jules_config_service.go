package services

// jules_config_service.go — Epic 2.4: GetJulesConfig / UpdateJulesConfig /
// TestJulesConnection / ConfirmEgressConsent, the RPCs that let a user set
// up and inspect the Jules integration entirely from the web UI. Modeled on
// slack_config_service.go (Get/Update/Test triple, a masked-boolean
// convention for secrets). See
// project_plans/google-jules-integration/implementation/plan.md's Epic 2.4
// for the full story/task breakdown this file implements.

import (
	"context"
	"fmt"
	"sync"

	connect "connectrpc.com/connect"

	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/jules"
)

// julesAPIKeyClearSentinel is UpdateJulesConfigRequest.ApiKey's magic value
// meaning "delete the stored key", distinct from "" ("leave unchanged") —
// the same empty-vs-explicit-clear convention UpdateSlackConfig uses via its
// separate clear_webhook_url bool, spelled here as a sentinel string instead
// since JulesConfigProto has only the one api_key field to carry both the
// new-value and clear intents (Task 2.4.1b).
const julesAPIKeyClearSentinel = "__clear__"

// julesKeyManager is the narrow slice of *jules.KeyringTokenSource
// JulesConfigService needs — declared locally so tests can fake it without a
// real OS keychain (mirrors this codebase's consumer-owned-interface
// convention, e.g. julesSessionCreator in jules_dispatch_service.go).
type julesKeyManager interface {
	APIKey(ctx context.Context) (jules.JulesAPIKey, error)
	SetJulesAPIKey(ctx context.Context, key jules.JulesAPIKey) error
	DeleteJulesAPIKey(ctx context.Context) error
}

// julesSourceResolver is the narrow slice of *jules.JulesSourceRegistry
// TestJulesConnection needs.
type julesSourceResolver interface {
	Resolve(ctx context.Context, owner, repo string) (jules.JulesSourceName, error)
}

// julesAuthReconnectReporter is the narrow slice of *session.JulesSessionPoller
// GetJulesConfig needs — declared locally (rather than importing the
// session package's concrete type into a request-scoped field) so a nil
// dependency (feature off / poller not started) is trivially representable
// and a test can fake it without a real poller. *session.JulesSessionPoller
// satisfies this structurally.
type julesAuthReconnectReporter interface {
	AuthReconnectRequired() bool
}

// julesUsageSnapshotter is the narrow slice of *JulesUsageCounter
// GetJulesConfig needs (Task 4.1.1a) — declared locally so a nil dependency
// (feature off / counter not wired) is trivially representable and a test
// can fake it without a real JulesUsageCounter. *JulesUsageCounter satisfies
// this structurally.
type julesUsageSnapshotter interface {
	Snapshot() JulesUsageSnapshot
}

// JulesConfigService handles the GetJulesConfig/UpdateJulesConfig/
// TestJulesConnection/ConfirmEgressConsent RPCs. keys and sources are
// swapped by SetJulesDependencies once real Jules dependencies exist
// (server/dependencies.go, Task 2.4.4a); poller is nil until the poller
// itself is started, and every method here is nil-safe with respect to it.
type JulesConfigService struct {
	// cfgMu serializes read-modify-write config.json updates so
	// UpdateJulesConfig and ConfirmEgressConsent (Task 2.4.2b, the ONLY two
	// writers of Jules config state) never race each other.
	cfgMu sync.Mutex

	keys    julesKeyManager
	sources julesSourceResolver
	poller  julesAuthReconnectReporter
	usage   julesUsageSnapshotter
}

// NewJulesConfigService constructs a JulesConfigService. keys and sources
// may be nil (feature not configured yet — UpdateJulesConfig's api_key
// handling and TestJulesConnection then fail loudly rather than silently
// no-op); poller may be nil (feature off / not yet started), in which case
// auth_reconnect_required is always false.
func NewJulesConfigService(keys julesKeyManager, sources julesSourceResolver, poller julesAuthReconnectReporter) *JulesConfigService {
	return &JulesConfigService{keys: keys, sources: sources, poller: poller}
}

// SetPoller rewires the live JulesSessionPoller dependency post-construction
// — needed because the poller (server/dependencies.go, Task 2.4.4a) may be
// constructed after JulesConfigService in the dependency graph. nil is a
// valid value (feature off).
func (s *JulesConfigService) SetPoller(poller julesAuthReconnectReporter) {
	s.poller = poller
}

// SetUsageCounter rewires the live *JulesUsageCounter dependency
// post-construction (Task 4.1.1a), for the same reason SetPoller exists:
// server/dependencies.go constructs the counter after JulesConfigService in
// the dependency graph. nil is a valid value (feature off / not yet wired),
// in which case GetJulesConfig's usage field reports all zeros.
func (s *JulesConfigService) SetUsageCounter(usage julesUsageSnapshotter) {
	s.usage = usage
}

// GetJulesConfig returns the current Jules configuration. The API key is
// never returned — only has_api_key. auth_reconnect_required is read live
// from the poller dependency, not from persisted config.
// +api: jules:get-config
func (s *JulesConfigService) GetJulesConfig(
	ctx context.Context,
	req *connect.Request[sessionv1.GetJulesConfigRequest],
) (*connect.Response[sessionv1.GetJulesConfigResponse], error) {
	cfg := config.LoadConfig()
	return connect.NewResponse(&sessionv1.GetJulesConfigResponse{
		Config: s.julesConfigToProto(ctx, cfg),
	}), nil
}

// UpdateJulesConfig updates the Jules configuration. api_key routes to
// jules.KeyringTokenSource (empty = leave unchanged, julesAPIKeyClearSentinel
// = delete); every other field routes to config.Config and is persisted to
// config.json. The key itself is never written to config.json.
// +api: jules:update-config
func (s *JulesConfigService) UpdateJulesConfig(
	ctx context.Context,
	req *connect.Request[sessionv1.UpdateJulesConfigRequest],
) (*connect.Response[sessionv1.UpdateJulesConfigResponse], error) {
	switch req.Msg.ApiKey {
	case "":
		// Leave unchanged.
	case julesAPIKeyClearSentinel:
		if s.keys == nil {
			return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("jules key storage is not available"))
		}
		if err := s.keys.DeleteJulesAPIKey(ctx); err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to clear jules api key: %w", err))
		}
	default:
		if s.keys == nil {
			return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("jules key storage is not available"))
		}
		key, err := jules.ParseJulesAPIKey(req.Msg.ApiKey)
		if err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
		if err := s.keys.SetJulesAPIKey(ctx, key); err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to store jules api key: %w", err))
		}
	}

	s.cfgMu.Lock()
	defer s.cfgMu.Unlock()

	cfg := config.LoadConfig()
	cfg.Jules.Enabled = req.Msg.Enabled
	cfg.Jules.MaxConcurrentJulesSessions = int(req.Msg.MaxConcurrentJulesSessions)
	cfg.Jules.MaxJulesSessionsPerDay = int(req.Msg.MaxJulesSessionsPerDay)

	if err := config.SaveConfig(cfg); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to save config: %w", err))
	}

	return connect.NewResponse(&sessionv1.UpdateJulesConfigResponse{
		Config: s.julesConfigToProto(ctx, cfg),
	}), nil
}

// TestJulesConnection checks whether repo_path's GitHub remote is registered
// as a Jules source, using the currently-configured API key (via the source
// registry's Client dependency — no key material passes through this RPC).
// On a miss, sources.Resolve's own error already names the owner/repo and
// points at jules.google.com (jules/source_registry.go), so it is surfaced
// verbatim rather than replaced with a generic message.
// +api: jules:test-connection
func (s *JulesConfigService) TestJulesConnection(
	ctx context.Context,
	req *connect.Request[sessionv1.TestJulesConnectionRequest],
) (*connect.Response[sessionv1.TestJulesConnectionResponse], error) {
	if req.Msg.RepoPath == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("repo_path is required"))
	}
	if s.sources == nil {
		return connect.NewResponse(&sessionv1.TestJulesConnectionResponse{
			Ok:      false,
			Message: "jules is not configured on this server",
		}), nil
	}

	ref, err := resolveJulesOwnerRepo(req.Msg.RepoPath)
	if err != nil {
		return connect.NewResponse(&sessionv1.TestJulesConnectionResponse{
			Ok:      false,
			Message: err.Error(),
		}), nil
	}

	if _, err := s.sources.Resolve(ctx, ref.Owner(), ref.Repo()); err != nil {
		return connect.NewResponse(&sessionv1.TestJulesConnectionResponse{
			Ok:      false,
			Message: err.Error(),
		}), nil
	}

	return connect.NewResponse(&sessionv1.TestJulesConnectionResponse{Ok: true}), nil
}

// ConfirmEgressConsent is the ONLY function in the codebase that appends to
// JulesConfig.EgressAcknowledgedRepos — checkEgressConsent in
// jules_dispatch_service.go (Story 2.2.3) only ever reads it. Idempotent:
// re-confirming an already-acknowledged repo adds no duplicate entry.
// +api: jules:confirm-egress-consent
func (s *JulesConfigService) ConfirmEgressConsent(
	ctx context.Context,
	req *connect.Request[sessionv1.ConfirmEgressConsentRequest],
) (*connect.Response[sessionv1.ConfirmEgressConsentResponse], error) {
	if req.Msg.RepoPath == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("repo_path is required"))
	}

	s.cfgMu.Lock()
	defer s.cfgMu.Unlock()

	cfg := config.LoadConfig()
	for _, acked := range cfg.Jules.EgressAcknowledgedRepos {
		if acked == req.Msg.RepoPath {
			return connect.NewResponse(&sessionv1.ConfirmEgressConsentResponse{
				EgressAcknowledgedRepos: cfg.Jules.EgressAcknowledgedRepos,
			}), nil
		}
	}

	cfg.Jules.EgressAcknowledgedRepos = append(cfg.Jules.EgressAcknowledgedRepos, req.Msg.RepoPath)
	if err := config.SaveConfig(cfg); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to save config: %w", err))
	}

	return connect.NewResponse(&sessionv1.ConfirmEgressConsentResponse{
		EgressAcknowledgedRepos: cfg.Jules.EgressAcknowledgedRepos,
	}), nil
}

// RevokeEgressConsent is the ONLY function in the codebase that removes an
// entry from JulesConfig.EgressAcknowledgedRepos — the removal-side
// counterpart to ConfirmEgressConsent above, both under cfgMu so the two
// writers of Jules config state never race each other. Idempotent: revoking
// a repo that isn't currently acknowledged is a no-op, not an error.
// +api: jules:revoke-egress-consent
func (s *JulesConfigService) RevokeEgressConsent(
	ctx context.Context,
	req *connect.Request[sessionv1.RevokeEgressConsentRequest],
) (*connect.Response[sessionv1.RevokeEgressConsentResponse], error) {
	if req.Msg.RepoPath == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("repo_path is required"))
	}

	s.cfgMu.Lock()
	defer s.cfgMu.Unlock()

	cfg := config.LoadConfig()
	remaining := make([]string, 0, len(cfg.Jules.EgressAcknowledgedRepos))
	for _, acked := range cfg.Jules.EgressAcknowledgedRepos {
		if acked != req.Msg.RepoPath {
			remaining = append(remaining, acked)
		}
	}

	if len(remaining) == len(cfg.Jules.EgressAcknowledgedRepos) {
		// Nothing to remove — return current state without a redundant save.
		return connect.NewResponse(&sessionv1.RevokeEgressConsentResponse{
			EgressAcknowledgedRepos: cfg.Jules.EgressAcknowledgedRepos,
		}), nil
	}

	cfg.Jules.EgressAcknowledgedRepos = remaining
	if err := config.SaveConfig(cfg); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to save config: %w", err))
	}

	return connect.NewResponse(&sessionv1.RevokeEgressConsentResponse{
		EgressAcknowledgedRepos: cfg.Jules.EgressAcknowledgedRepos,
	}), nil
}

// julesConfigToProto converts the config-layer JulesConfig plus live
// has_api_key/auth_reconnect_required state into the masked proto
// representation shared by GetJulesConfig and UpdateJulesConfig.
// auth_reconnect_required is read from s.poller at call time, never from
// cfg — it is process/runtime state, not persisted config (Task 2.4.1b).
func (s *JulesConfigService) julesConfigToProto(ctx context.Context, cfg *config.Config) *sessionv1.JulesConfigProto {
	hasKey := false
	if s.keys != nil {
		if _, err := s.keys.APIKey(ctx); err == nil {
			hasKey = true
		}
	}

	authReconnectRequired := false
	if s.poller != nil {
		authReconnectRequired = s.poller.AuthReconnectRequired()
	}

	var usage JulesUsageSnapshot
	if s.usage != nil {
		usage = s.usage.Snapshot()
	}

	return &sessionv1.JulesConfigProto{
		Enabled:                    cfg.Jules.Enabled,
		HasApiKey:                  hasKey,
		EgressAcknowledgedRepos:    cfg.Jules.EgressAcknowledgedRepos,
		MaxConcurrentJulesSessions: int32(cfg.Jules.MaxConcurrentJulesSessions),
		MaxJulesSessionsPerDay:     int32(cfg.Jules.MaxJulesSessionsPerDay),
		AuthReconnectRequired:      authReconnectRequired,
		Usage: &sessionv1.JulesUsageProto{
			SessionDispatched: usage.SessionDispatched,
			SessionCompleted:  usage.SessionCompleted,
			SessionFailed:     usage.SessionFailed,
			ApiRateLimited:    usage.APIRateLimited,
			ApiError:          usage.APIError,
		},
	}
}
