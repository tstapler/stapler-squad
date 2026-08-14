package services

import (
	"context"
	"fmt"
	"sync"

	"connectrpc.com/connect"
	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
)

// sddDefaultPipelineFlagName is shared between knownFeatureFlags below and
// CreateBacklogItem's default-resolution branch (backlog_service_lifecycle.go)
// so the flag name can't drift between where it's declared and where it's
// read.
const sddDefaultPipelineFlagName = "backlog:sdd-default-pipeline"

// blockApprovalOnCIFailureFlagName is shared between knownFeatureFlags below and
// ApprovalService.ResolveApproval's CI-red guard (approval_service.go) so the flag
// name can't drift between where it's declared and where it's read.
const blockApprovalOnCIFailureFlagName = "review:block-approval-on-ci-failure"

// workspacePeersNudgeFlagName is shared between knownFeatureFlags below and
// workspacePeersBlockFor below so the flag name can't drift between where it's declared and
// where it's read.
const workspacePeersNudgeFlagName = "session:workspace-peers-nudge"

// terminalResyncCorrelationIDFlagName is shared between knownFeatureFlags below and
// handleCurrentPaneRequest's resync_id echo (connectrpc_websocket.go) so the flag name
// can't drift between where it's declared and where it's read.
const terminalResyncCorrelationIDFlagName = "terminal:resync-correlation-id"

// terminalResyncSkipStaleDimensionSlowpathFlagName is shared between knownFeatureFlags
// below and handleCurrentPaneRequest's stale-dimension skip branch (connectrpc_websocket.go)
// so the flag name can't drift between where it's declared and where it's read.
const terminalResyncSkipStaleDimensionSlowpathFlagName = "terminal:resync-skip-stale-dimension-slowpath"

// terminalResyncExecGateFastLaneFlagName is shared between knownFeatureFlags below and
// currentResyncOptions' UseFastLane field (connectrpc_websocket.go) so the flag name can't
// drift between where it's declared and where it's read. session/instance_tmux.go duplicates
// this literal (as terminalResyncExecGateFastLaneFlagName there too) rather than importing it,
// since session cannot import server/services without creating an import cycle — keep both
// in sync if this flag is ever renamed.
const terminalResyncExecGateFastLaneFlagName = "terminal:resync-exec-gate-fast-lane"

// terminalResyncCompressionFlagName is shared between knownFeatureFlags below and
// writeCurrentPaneResponse's envelope-compression branch (connectrpc_websocket.go) so the
// flag name can't drift between where it's declared and where it's read.
const terminalResyncCompressionFlagName = "terminal:resync-compression"

// workspacePeersBlockFor is the single feature-flag gate for the workspace-peers nudge,
// called by both SessionService.workspacePeersBlockFor (session_service.go) and
// BacklogService.workspacePeersBlockFor (backlog_service_triage.go) so the two callers can't
// drift on the gate itself, the same way session.WorkspacePeersBlockForPath already keeps
// them from drifting on how the nudge is rendered.
func workspacePeersBlockFor(ctx context.Context, storage *session.Storage, repoPath string) string {
	if !config.LoadConfig().GetFeatureFlag(workspacePeersNudgeFlagName) {
		return ""
	}
	return session.WorkspacePeersBlockForPath(ctx, storage, repoPath)
}

// knownFeatureFlags is the authoritative list of feature flags exposed via the RPC API.
// Moved here from session_service.go (ADR-001: single-concern cluster gets its own file).
var knownFeatureFlags = []struct {
	name        string
	description string
}{
	{
		name:        "backlog",
		description: "Backlog management with external sync sources and AI-driven triage",
	},
	{
		name:        "browser-passthrough",
		description: "Browser passthrough: stream Chrome/Chromium via CDP in the Browser tab",
	},
	{
		name:        "backlog:conversation-view",
		description: "Show JSONL conversation messages in the session monitor (default: terminal scrollback view)",
	},
	{
		name:        "unfinished:mmap-index",
		description: "Use the mmap-backed pack-index loader for the /unfinished scanner's git storage (session/unfinished/gogitstore) instead of the copy-based loader. See session/unfinished/design/mmap-activation-runbook.md before enabling.",
	},
	{
		name:        sddDefaultPipelineFlagName,
		description: "New backlog items with no explicitly chosen pipeline mode default to the 'sdd' pipeline mode (research, plan, validate, implement, and an adversarial verify pass before review) instead of the flat default pipeline. Never affects existing items or an item with any explicit pipeline_mode value, including an explicit empty one.",
	},
	{
		name:        blockApprovalOnCIFailureFlagName,
		description: "Block manual Approve when the session's branch has failing GitHub CI. Shows a visible inline explanation instead of a silent no-op; a reviewer can still bypass it per-approval via 'Approve anyway' (audited). Sessions with no associated PR are unaffected. Default: off.",
	},
	{
		name:        workspacePeersNudgeFlagName,
		description: "Auto-inject an 'Other Active Sessions In This Workspace' nudge into every new session's initial prompt. Off by default — use the list_workspace_peers MCP tool on demand instead. Default: off.",
	},
	{
		name:        "terminal:resync-visibility-scope",
		description: "Scope terminal resync-on-visibility-change to only the terminal instance actually in the foreground, instead of every mounted terminal. Applies to newly-focused terminals only — already-open tabs need a reload to pick up the change. Default: off.",
	},
	{
		name:        terminalResyncCorrelationIDFlagName,
		description: "Tag each terminal resync request/reply pair with a correlation ID so a stale reply from an earlier resync can't be misapplied to a later one. Not live-updated on already-open tabs. Default: off.",
	},
	{
		name:        terminalResyncSkipStaleDimensionSlowpathFlagName,
		description: "Skip the stale-dimension slow path for backgrounded terminals during resync, avoiding unnecessary pane-size recalculation for terminals not currently visible. Not live-updated on already-open tabs. Default: off.",
	},
	{
		name:        terminalResyncExecGateFastLaneFlagName,
		description: "Route resync's tmux subprocess calls through a dedicated fast-lane slot pool (see TmuxExecGateConfig.ResyncFastLaneSlots) instead of contending with other tmux exec traffic for the shared gate. Default: off.",
	},
	{
		name:        "terminal:resync-stagger",
		description: "Stagger resync bursts across multiple terminals instead of firing them all simultaneously, reducing thundering-herd load on the tmux server. Default: off.",
	},
	{
		name:        terminalResyncCompressionFlagName,
		description: "Compress terminal resync payloads on the wire to reduce bandwidth for large scrollback resyncs. Default: off.",
	},
	{
		name:        "terminal:resync-batching",
		description: "Batch multiple terminals' resync requests into a single round trip instead of issuing one request per terminal. Default: off.",
	},
}

// FeatureFlagService handles GetFeatureFlags and UpdateFeatureFlag RPCs.
// It owns the knownFeatureFlags registry and the per-name FeatureController map.
// Extracted from SessionService per ADR-001 (UpdateFeatureFlag exceeds the 30-line threshold).
type FeatureFlagService struct {
	// featureControllers maps feature flag names to their runtime controllers.
	// Wired via SetFeatureController. May be nil for features that only need
	// config-file persistence (no in-process component to toggle).
	featureControllers map[string]FeatureController

	// statusDetailProviders maps feature flag names to a function returning an
	// optional human-readable status line (e.g. why the flag is currently off).
	// Wired via SetStatusDetailProvider. Absent name -> "".
	statusDetailProviders map[string]func() string

	// updateMu serializes UpdateFeatureFlag's read-toggle-rollback sequence so two
	// concurrent toggles of the same flag can't race: without this, a slow caller's
	// rollback (after its own controller failure) could stomp a faster caller's
	// already-successful, already-persisted toggle, reintroducing disk/controller
	// divergence via a different trigger than the one this rollback logic closes.
	updateMu sync.Mutex
}

// NewFeatureFlagService creates a FeatureFlagService. Call SetFeatureController
// for each flag that has an in-process runtime component.
func NewFeatureFlagService() *FeatureFlagService {
	return &FeatureFlagService{}
}

// SetFeatureController wires a runtime controller for the named feature flag.
// When UpdateFeatureFlag is called for this name, the controller's Enable/Disable
// methods are invoked in addition to persisting the flag to config.
func (f *FeatureFlagService) SetFeatureController(name string, c FeatureController) {
	if f.featureControllers == nil {
		f.featureControllers = make(map[string]FeatureController)
	}
	f.featureControllers[name] = c
}

// SetStatusDetailProvider wires an optional status-detail provider for the
// named feature flag. GetFeatureFlags calls fn on every request and populates
// FeatureFlag.StatusDetail with its result (empty string when fn returns "").
func (f *FeatureFlagService) SetStatusDetailProvider(name string, fn func() string) {
	if f.statusDetailProviders == nil {
		f.statusDetailProviders = make(map[string]func() string)
	}
	f.statusDetailProviders[name] = fn
}

// +api: feature-flags:list
// GetFeatureFlags returns all known feature flags and their current state.
func (f *FeatureFlagService) GetFeatureFlags(
	ctx context.Context,
	req *connect.Request[sessionv1.GetFeatureFlagsRequest],
) (*connect.Response[sessionv1.GetFeatureFlagsResponse], error) {
	cfg := config.LoadConfig()

	flags := make([]*sessionv1.FeatureFlag, 0, len(knownFeatureFlags))
	for _, kf := range knownFeatureFlags {
		enabled := false
		if cfg.FeatureFlags != nil {
			enabled = cfg.FeatureFlags[kf.name]
		}
		// If a controller is wired, its live state is the source of truth.
		if ctrl, ok := f.featureControllers[kf.name]; ok {
			enabled = ctrl.IsEnabled()
		}
		var statusDetail string
		if provider, ok := f.statusDetailProviders[kf.name]; ok {
			statusDetail = provider()
		}
		flags = append(flags, &sessionv1.FeatureFlag{
			Name:         kf.name,
			Enabled:      enabled,
			Description:  kf.description,
			StatusDetail: statusDetail,
		})
	}

	return connect.NewResponse(&sessionv1.GetFeatureFlagsResponse{Flags: flags}), nil
}

// +api: feature-flags:update
// UpdateFeatureFlag enables or disables a named feature flag and persists the change.
func (f *FeatureFlagService) UpdateFeatureFlag(
	ctx context.Context,
	req *connect.Request[sessionv1.UpdateFeatureFlagRequest],
) (*connect.Response[sessionv1.UpdateFeatureFlagResponse], error) {
	name := req.Msg.GetName()
	enabled := req.Msg.GetEnabled()

	// Validate that the flag name is known.
	known := false
	var description string
	for _, kf := range knownFeatureFlags {
		if kf.name == name {
			known = true
			description = kf.description
			break
		}
	}
	if !known {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("unknown feature flag %q: valid flags are %v", name, func() []string {
				names := make([]string, 0, len(knownFeatureFlags))
				for _, kf := range knownFeatureFlags {
					names = append(names, kf.name)
				}
				return names
			}()))
	}

	// Serialize the whole persist-toggle-rollback sequence: without this, two concurrent
	// UpdateFeatureFlag calls for the same name could interleave such that a slower
	// caller's rollback (after its own controller failure) overwrites a faster caller's
	// already-successful, already-persisted toggle.
	f.updateMu.Lock()
	defer f.updateMu.Unlock()

	// Persist to config. SetFeatureFlag handles its own map initialisation and
	// calls SaveConfig atomically, avoiding a separate LoadConfig→modify→SaveConfig
	// sequence that would race under concurrent UpdateFeatureFlag calls.
	cfg := config.LoadConfig()
	previousEnabled := cfg.GetFeatureFlag(name)
	if err := cfg.SetFeatureFlag(name, enabled); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to persist feature flag: %w", err))
	}

	// Toggle the in-process controller if one is wired. If the controller fails to apply the
	// new state, roll back the just-persisted disk flag so disk config and in-memory state can
	// never diverge — a failed toggle must not leave the two disagreeing about whether the
	// feature is on, since GetFeatureFlags/RPC-gating interceptors read from different sources.
	if ctrl, ok := f.featureControllers[name]; ok {
		var ctrlErr error
		verb := "disable"
		if enabled {
			verb = "enable"
			ctrlErr = ctrl.Enable(ctx)
		} else {
			ctrlErr = ctrl.Disable()
		}
		if ctrlErr != nil {
			log.Error("feature controller toggle failed, rolling back persisted flag",
				"feature", name, "enabled", enabled, "err", ctrlErr)
			if rollbackErr := cfg.SetFeatureFlag(name, previousEnabled); rollbackErr != nil {
				log.Error("failed to roll back feature flag after controller error",
					"feature", name, "err", rollbackErr)
				return nil, connect.NewError(connect.CodeInternal,
					fmt.Errorf("failed to %s feature %q: %w (rollback also failed, disk state may be inconsistent: %v)",
						verb, name, ctrlErr, rollbackErr))
			}
			return nil, connect.NewError(connect.CodeInternal,
				fmt.Errorf("failed to %s feature %q: %w", verb, name, ctrlErr))
		}
	}

	log.Info("feature flag updated", "feature", name, "enabled", enabled)

	var statusDetail string
	if provider, ok := f.statusDetailProviders[name]; ok {
		statusDetail = provider()
	}

	return connect.NewResponse(&sessionv1.UpdateFeatureFlagResponse{
		Flag: &sessionv1.FeatureFlag{
			Name:         name,
			Enabled:      enabled,
			Description:  description,
			StatusDetail: statusDetail,
		},
	}), nil
}
