package services

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"connectrpc.com/connect"
	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/deliverygate"
	"github.com/tstapler/stapler-squad/session"
)

// piSupportFlagName mirrors config.FeaturePiSupport so knownFeatureFlags below
// doesn't duplicate the literal — see config/config.go for the flag's full
// documentation and project_plans/pi-support/implementation/plan.md, Epic 2.1.
const piSupportFlagName = config.FeaturePiSupport

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

// programCLIFlagProbeFlagName is the kill switch for the ProbeProgram RPC.
// Default on; read per request via ProgramCLIFlagProbeEnabled.
const programCLIFlagProbeFlagName = "programs:cli-flag-probe"

// ProgramCLIFlagProbeEnabled reports whether ProbeProgram is allowed. It reads
// config on every call so a Settings toggle applies with no restart.
func ProgramCLIFlagProbeEnabled() bool {
	return config.LoadConfig().GetFeatureFlagWithDefault(
		programCLIFlagProbeFlagName, featureFlagDefault(programCLIFlagProbeFlagName))
}

// ProgramCLIFlagProbeGatedMethod is the SessionService method the kill switch gates.
const ProgramCLIFlagProbeGatedMethod = "ProbeProgram"

// githubPriorityAdmissionFlagName gates github.rateLimitTransport.RoundTrip's
// AdmitOrigin rejection branch (github/http_client.go). github cannot import
// this package (server/services already imports github, so the reverse would
// be a cycle), so github/http_client.go duplicates this same string literal
// under its own constant of the same name rather than importing it — mirrors
// session/instance_tmux.go's terminalResyncExecGateFastLaneFlagName
// precedent. Keep both constants' values in sync if this flag is ever
// renamed.
const githubPriorityAdmissionFlagName = "github:priority-admission-control"

// githubGraphQLMigrationFlagName gates GetPRInfoCtx's dispatch to
// GetPRInfoGraphQL instead of the gh-CLI shell-out (github/client.go). github
// cannot import this package (server/services already imports github, so the
// reverse would be a cycle), so github/client.go duplicates this same string
// literal under its own constant of the same name rather than importing it —
// mirrors githubPriorityAdmissionFlagName's precedent above. Keep both
// constants' values in sync if this flag is ever renamed.
const githubGraphQLMigrationFlagName = "github:graphql-pr-info"

// handoffSummaryFlagName is the generic-registry name for the
// restart-with-handoff-summary feature, so the frontend can discover
// HandoffSummaryConfig.Enabled up front (via GetFeatureFlags) instead of only
// finding out on the first TriggerHandoffSummary call's
// Code.FailedPrecondition (see HandoffSummaryService.TriggerHandoffSummary's
// identical EnabledOrDefault() check). Shared with HandoffSummaryFeatureController
// below so the flag name can't drift between where it's declared and where it's
// wired.
const handoffSummaryFlagName = "handoff-summary"

// HandoffSummaryFeatureController is a read-only FeatureController for
// handoffSummaryFlagName: IsEnabled defers to config.HandoffSummaryConfig's
// EnabledOrDefault, the same real source of truth
// HandoffSummaryService.TriggerHandoffSummary already gates on. The generic
// feature-flags map (config.json's top-level feature_flags key) is
// deliberately NOT the source of truth here -- config.json's own
// handoff_summary.enabled key is -- so this flag is exposed for GetFeatureFlags
// visibility only. Enable/Disable both refuse: this flag's real setting has
// no in-process runtime toggle for UpdateFeatureFlag to drive, and letting the
// call silently "succeed" while IsEnabled stays keyed to a different config
// field would make GetFeatureFlags lie about the effect of an
// UpdateFeatureFlag call that just apparently succeeded. Exported (unlike
// this file's other flag machinery) because server/dependencies.go, not this
// package, is where every other SetFeatureController call is wired.
type HandoffSummaryFeatureController struct{}

func (HandoffSummaryFeatureController) Enable(context.Context) error {
	return fmt.Errorf("%q is read-only via this API: set handoff_summary.enabled in config.json directly", handoffSummaryFlagName)
}

func (HandoffSummaryFeatureController) Disable() error {
	return fmt.Errorf("%q is read-only via this API: set handoff_summary.enabled in config.json directly", handoffSummaryFlagName)
}

func (HandoffSummaryFeatureController) IsEnabled() bool {
	return config.LoadConfig().HandoffSummary.EnabledOrDefault()
}

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

// terminalResyncVisibilityScopeFlagName, terminalResyncStaggerFlagName, and
// terminalResyncBatchingFlagName are pure client-side concerns (see
// connectrpc_websocket_test.go's allTerminalResyncFlagNames doc comment) with no Go
// production call site to share a constant with — they're named here purely for
// consistency with the other four terminal:resync-* flags above, and so
// knownFeatureFlags/allTerminalResyncFlagNames reference one declaration instead of a
// raw string literal.
const terminalResyncVisibilityScopeFlagName = "terminal:resync-visibility-scope"
const terminalResyncStaggerFlagName = "terminal:resync-stagger"
const terminalResyncBatchingFlagName = "terminal:resync-batching"

const worktreeChangeDetectionFlagName = "vcs:worktree-change-detection"

// terminalAppScrollForwardingClaudeFlagName mirrors
// config.FeatureAppScrollForwardingClaude so knownFeatureFlags below doesn't
// duplicate the literal -- see config/config.go for the flag's full
// documentation and
// project_plans/app-scrollback-forwarding/implementation/plan.md, Epic 1.5.
const terminalAppScrollForwardingClaudeFlagName = config.FeatureAppScrollForwardingClaude

// notificationTrayV2FlagName gates the capped toast deck and the notification tray in
// the web app (read there as a live flag via GetFeatureFlags). The server only registers
// it; the client mirrors this literal in lib/notification-policy.ts. Default: off.
const notificationTrayV2FlagName = "notification_tray_v2"

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
// defaultValue is what GetFeatureFlags (below) reports, and what the flag's real call
// site should resolve to via GetFeatureFlagWithDefault, when the flag has never been
// explicitly persisted — Go's zero value (false) for any entry that omits the field, so
// only a flag graduating to "on by default" (like terminalResyncExecGateFastLaneFlagName)
// needs to set it.
var knownFeatureFlags = []struct {
	name         string
	description  string
	defaultValue bool
	// scopes lists the per-scope overrides the flag accepts (nil: global only).
	scopes []string
}{
	{
		name:         programCLIFlagProbeFlagName,
		description:  "Check that a program exists on the server and read its --help flags (Program Config and session creation). Turn off to disable the ProbeProgram RPC immediately. Default: on.",
		defaultValue: true,
	},
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
		name:        crossHostClaimDedupFlagName,
		description: "Cross-host duplicate-work prevention: before importing a GitHub issue or creating an item with an external URL, and before dequeuing a queued item, check whether another stapler-squad host already claimed that URL. A blocked import offers 'Import anyway' with an audited reason; a blocked dequeue shows up as a stuck item with an override. Claims are always recorded regardless. Default: off.",
	},
	{
		name:        handoffSummaryFlagName,
		description: "Restart-with-summary: generate an AI handoff summary and restart into a fresh session. Read-only here -- the real toggle is config.json's handoff_summary.enabled key. Default: on.",
	},
	{
		name:        terminalResyncVisibilityScopeFlagName,
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
		name:         terminalResyncExecGateFastLaneFlagName,
		description:  "Route resync's tmux subprocess calls through a dedicated fast-lane slot pool (see TmuxExecGateConfig.ResyncFastLaneSlots) instead of contending with other tmux exec traffic for the shared gate. Default: on (2026-08-25) — without it, a resync-triggered resize's sequential subprocess calls can exceed the client's 4s stall watchdog under exec-gate contention, forcing an unnecessary disconnect+reconnect. Persist an explicit false to opt back out.",
		defaultValue: true,
	},
	{
		name:        terminalResyncStaggerFlagName,
		description: "Stagger resync bursts across multiple terminals instead of firing them all simultaneously, reducing thundering-herd load on the tmux server. Default: off.",
	},
	{
		name:        terminalResyncCompressionFlagName,
		description: "Compress terminal resync payloads on the wire to reduce bandwidth for large scrollback resyncs. Default: off.",
	},
	{
		name:        terminalResyncBatchingFlagName,
		description: "Batch multiple terminals' resync requests into a single round trip instead of issuing one request per terminal. Default: off.",
	},
	{
		name:        githubPriorityAdmissionFlagName,
		description: "Priority-aware admission control for outbound GitHub API calls: background pollers/sync back off once a resource's (core/search/graphql) remaining quota drops below a reserved headroom, so interactive GitHub actions (merge, comment, refresh) keep succeeding. Protects this stapler-squad instance's own interactive GitHub calls only — does not coordinate with other machines sharing the same GitHub token. Default: off.",
	},
	{
		name:        githubGraphQLMigrationFlagName,
		description: "Route GetPRInfoCtx's PR-metadata fetch through a single native GraphQL request instead of the `gh pr view` CLI shell-out, cutting the REST-equivalent call count for PR status refreshes. Default: off.",
	},
	{
		name:        worktreeChangeDetectionFlagName,
		description: "Watch each session's .git dir via fsnotify and run a staggered 15s periodic cheap dirty/HEAD check to invalidate the diff-stats and VCS-status caches, letting both widen from a 15s to a 5-minute TTL. Applies to newly-created worktrees only; already-open sessions keep today's 15s pure-TTL behavior until restarted. Default: off.",
	},
	{
		name:        terminalAppScrollForwardingClaudeFlagName,
		description: "Forward Claude Code's own PageUp scroll keybinding into its fullscreen conversation view instead of tmux-native scrollback capture, for eligible sessions (AppScrollGate: adapter coverage, alt-screen active, idle status, exactly one connected viewer). Flag-off always falls through to the unchanged tmux-native scrollback path, regardless of AppScrollGate's verdict. Default: off.",
	},
	{
		name:        piSupportFlagName,
		description: "pi coding agent support: program picker entry, resume across restarts, and approval-rule enforcement parity with Claude Code. Default: off. Disabling does not remove an already-installed global pi approval extension — see the settings UI warning.",
	},
	{
		name:        config.TriageGuidanceHaltFeatureFlag,
		description: "Automated triage halts and asks a durable guidance request instead of guessing when a backlog item is genuinely ambiguous. Default: off — baseline guess-and-proceed triage behavior is unchanged until enabled.",
	},
	{
		name:        config.DiagnoseNudgeFeatureFlag,
		description: "Diagnose & Nudge: allow a dispatched diagnostic agent to autonomously send a redirect message (diagnose_nudge_session) to a linked stuck session. Read fresh at the write instant, so flipping this off blocks an already-dispatched agent too. Default: off — until enabled, a dispatch can still investigate and file a bug or post a note, but never nudge.",
	},
	{
		name:        notificationTrayV2FlagName,
		description: "Notification tray v2: cap the toast deck at 3 (1 on phones) with a \"+N more\" chip and one \"Move all to tray\" control, instead of the uncapped legacy toast list. Applies on the next toast render, no reload. Default: off.",
	},
	{
		name:        config.HiddenSessionGateFeatureFlag,
		description: "Hidden-session delivery gate: a hidden session (review, diagnose) notifies only for failures and needs-human events; routine completions are dropped on every channel. Off keeps today's behavior and only counts what would have been suppressed (see the status line). Can be enabled only while the stats writer runs, so the soak is recorded. Can be overridden per hidden-session kind (review, diagnose, other). Default: off.",
		scopes:      gateFlagScopes(),
	},
	{
		name:         terminalWriteLeaseFlagName,
		description:  "Per-session terminal write lease: the driver's prompt and answer keys, steers, nudges, MCP writes and rate-limit recovery take turns writing to a session's terminal instead of interleaving. Turn off only to restore the pre-lease behavior if the lease misbehaves (every write then proceeds unserialized). Applies at once; global only. Default: on.",
		defaultValue: true,
	},
	{
		name:         hiddenSessionReadonlyGuardsFlagName,
		description:  "Hidden-session write guards: the UI's write RPCs (terminal input, steer, restart, workspace switch, program and auto-approve changes) refuse a hidden (background) session, except the backlog Steer of a live review session, which stays audited. Turn off only to restore the previous behavior if a guard blocks a legitimate action; every write allowed while off is recorded in the audit log. Applies at once; global only. Default: on.",
		defaultValue: true,
	},
}

// featureFlagDefault looks up name's defaultValue in knownFeatureFlags — the single
// source of truth both GetFeatureFlags (the registry listing) and a flag's own call
// site (e.g. currentResyncOptions' UseFastLane) resolve against, so the two can never
// drift on what "default" means for a given flag. Returns false for an unregistered
// name, matching GetFeatureFlag's own "absent means false" convention.
func featureFlagDefault(name string) bool {
	for _, kf := range knownFeatureFlags {
		if kf.name == name {
			return kf.defaultValue
		}
	}
	return false
}

// FeatureFlagService handles GetFeatureFlags and UpdateFeatureFlag RPCs.
// It owns the knownFeatureFlags registry and the per-name FeatureController map.
// Extracted from SessionService per ADR-001 (UpdateFeatureFlag exceeds the 30-line threshold).
type FeatureFlagService struct {
	// featureControllers maps feature flag names to their runtime controllers.
	// Wired via SetFeatureController. May be nil for features that only need
	// config-file persistence (no in-process component to toggle).
	featureControllers map[string]FeatureController

	// statusDetailSlots maps feature flag names to the ordered contributions to
	// their status line (e.g. why the flag is currently off). SetStatusDetailProvider
	// owns one named slot per flag; AddStatusDetailSource appends independent
	// ones. GetFeatureFlags joins the non-empty results with "; ".
	statusDetailSlots map[string][]statusDetailSlot

	// enableGuards maps flag names to a precondition checked only when the flag
	// is being enabled; a non-empty return refuses the flip with FailedPrecondition.
	enableGuards map[string]func() string

	// observer is told after every persisted change so a cache can reload at once.
	observer deliverygate.FlagObserver

	// audit and auditPolicies record flag_change lines for flags with a policy.
	audit         *AuditSink
	auditPolicies map[string]FlagAuditPolicy
	flagSeq       int64 // persisted order of flips; incremented only under updateMu

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
	f.setStatusDetailSlot(name, providerSlotKey, fn)
}

// providerSlotKey names the slot SetStatusDetailProvider owns; a second call
// replaces it in place so the setter keeps its replace-by-name meaning.
const providerSlotKey = "provider"

type statusDetailSlot struct {
	key string
	fn  func() string
}

func (f *FeatureFlagService) setStatusDetailSlot(name, key string, fn func() string) {
	if f.statusDetailSlots == nil {
		f.statusDetailSlots = make(map[string][]statusDetailSlot)
	}
	slots := f.statusDetailSlots[name]
	for i := range slots {
		if key != "" && slots[i].key == key {
			slots[i].fn = fn
			return
		}
	}
	f.statusDetailSlots[name] = append(slots, statusDetailSlot{key: key, fn: fn})
}

// AddStatusDetailSource appends an independent contribution to name's status
// line. Several sources (stats writer, explicit global false, per-kind false)
// each speak for the same flag without replacing one another.
func (f *FeatureFlagService) AddStatusDetailSource(name string, fn func() string) {
	f.setStatusDetailSlot(name, "", fn)
}

func (f *FeatureFlagService) statusDetailFor(name string) string {
	var parts []string
	for _, s := range f.statusDetailSlots[name] {
		if d := s.fn(); d != "" {
			parts = append(parts, d)
		}
	}
	return strings.Join(parts, "; ")
}

// SetEnableGuard installs a precondition for enabling name. It is checked only
// on the way on: disabling and clearing are never refused by it.
func (f *FeatureFlagService) SetEnableGuard(name string, guard func() string) {
	if f.enableGuards == nil {
		f.enableGuards = make(map[string]func() string)
	}
	f.enableGuards[name] = guard
}

// SetFlagObserver wires the cache notified after every persisted change.
func (f *FeatureFlagService) SetFlagObserver(o deliverygate.FlagObserver) { f.observer = o }

// SetAudit wires the audit sink and the per-flag audit policy.
func (f *FeatureFlagService) SetAudit(sink *AuditSink, policies map[string]FlagAuditPolicy) {
	f.audit, f.auditPolicies = sink, policies
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
		enabled := cfg.GetFeatureFlagWithDefault(kf.name, kf.defaultValue)
		// If a controller is wired, its live state is the source of truth.
		if ctrl, ok := f.featureControllers[kf.name]; ok {
			enabled = ctrl.IsEnabled()
		}
		flags = append(flags, &sessionv1.FeatureFlag{
			Name:         kf.name,
			Enabled:      enabled,
			Description:  kf.description,
			StatusDetail: f.statusDetailFor(kf.name),
			Scopes:       flagScopeReadback(cfg, kf.name, kf.scopes),
		})
	}

	return connect.NewResponse(&sessionv1.GetFeatureFlagsResponse{Flags: flags}), nil
}

// +api: feature-flags:update
// UpdateFeatureFlag applies one validated mutation to a named feature flag and
// persists it: a global set (the legacy shape), a per-scope set, a scope clear
// or a reset of the explicit global value to the registry default.
func (f *FeatureFlagService) UpdateFeatureFlag(
	ctx context.Context,
	req *connect.Request[sessionv1.UpdateFeatureFlagRequest],
) (*connect.Response[sessionv1.UpdateFeatureFlagResponse], error) {
	name := req.Msg.GetName()

	// Validate that the flag name is known.
	known := false
	var description string
	var scopes []string
	for _, kf := range knownFeatureFlags {
		if kf.name == name {
			known = true
			description = kf.description
			scopes = kf.scopes
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
	op, err := parseFlagOp(req.Msg, scopes)
	if err != nil {
		return nil, err
	}

	// Only the way on is guarded: disabling, clearing a scope and resetting the
	// global value are never refused by a precondition.
	if op.enabling() {
		if guard, ok := f.enableGuards[name]; ok {
			if reason := guard(); reason != "" {
				return nil, connect.NewError(connect.CodeFailedPrecondition,
					fmt.Errorf("cannot enable %q: %s", name, reason))
			}
		}
	}

	// The loosening flip's durable `requested` line is appended before the
	// update mutex so a stalled fsync cannot queue a kill switch behind it. The
	// deferred `result` line is registered before the lock, so it runs after
	// the unlock.
	audit, err := f.auditBegin(ctx, op, flagRequestFields(ctx, req.Peer().Addr, req.Header()))
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("flag change not audited, nothing persisted: %w", err))
	}
	defer audit.finish()

	// Serialize the whole persist-toggle-rollback sequence: without this, two concurrent
	// UpdateFeatureFlag calls for the same name could interleave such that a slower
	// caller's rollback (after its own controller failure) overwrites a faster caller's
	// already-successful, already-persisted toggle.
	f.updateMu.Lock()
	defer f.updateMu.Unlock()

	if op.scope != "" {
		err = f.persistScoped(ctx, op, audit)
	} else {
		err = f.persistAndApply(ctx, op, audit)
	}
	if err != nil {
		return nil, err
	}

	cfg := config.LoadConfig()
	enabled := cfg.GetFeatureFlagWithDefault(name, featureFlagDefault(name))
	if op.scope == "" && op.isSet() {
		enabled = op.enabled // the controller-backed state, as before this change
	}
	log.Info("feature flag updated", "feature", name, "enabled", enabled, "scope", op.auditScope(), "mutation", op.mutationName())
	return connect.NewResponse(&sessionv1.UpdateFeatureFlagResponse{
		Flag: &sessionv1.FeatureFlag{
			Name:         name,
			Enabled:      enabled,
			Description:  description,
			StatusDetail: f.statusDetailFor(name),
			Scopes:       flagScopeReadback(cfg, name, scopes),
		},
	}), nil
}

// persistAndApply persists the global value (a set, or the deletion of the
// explicit key for RESET_GLOBAL), toggles the controller and rolls back on its
// failure. The caller holds updateMu. It records the true previous value (an
// absent key is the registered default, not an explicit false) and, when the key
// was absent, rolls back by deleting it so a default-on flag never ends a failed
// flip as an explicit false.
func (f *FeatureFlagService) persistAndApply(ctx context.Context, op flagOp, audit *flagAudit) error {
	name, enabled := op.name, op.enabled
	cfg := config.LoadConfig()
	previous, hadKey := cfg.GetFeatureFlagOverride(name)
	if !hadKey {
		previous = featureFlagDefault(name)
	}
	if audit.active() {
		f.flagSeq++
		audit.seq, audit.previous = f.flagSeq, previous
	}
	var err error
	if op.isSet() {
		err = cfg.SetFeatureFlag(name, enabled)
	} else {
		err = cfg.DeleteFeatureFlag(name)
	}
	if err != nil {
		return connect.NewError(connect.CodeInternal, fmt.Errorf("failed to persist feature flag: %w", err))
	}
	ctrl, ok := f.featureControllers[name]
	if !ok {
		f.finishApplied(op, audit)
		return nil
	}
	// A failed toggle must not leave disk config and in-memory state disagreeing
	// about whether the feature is on: GetFeatureFlags and RPC-gating
	// interceptors read from different sources.
	verb, ctrlErr := "disable", error(nil)
	if enabled {
		verb, ctrlErr = "enable", ctrl.Enable(ctx)
	} else {
		ctrlErr = ctrl.Disable()
	}
	if ctrlErr == nil {
		f.finishApplied(op, audit)
		return nil
	}
	log.Error("feature controller toggle failed, rolling back persisted flag",
		"feature", name, "enabled", enabled, "err", ctrlErr)
	if rollbackErr := rollbackFlag(cfg, name, previous, hadKey); rollbackErr != nil {
		log.Error("failed to roll back feature flag after controller error", "feature", name, "err", rollbackErr)
		if audit.active() {
			audit.outcome = flagOutcomeControllerFailed
		}
		return connect.NewError(connect.CodeInternal,
			fmt.Errorf("failed to %s feature %q: %w (rollback also failed, disk state may be inconsistent: %v)",
				verb, name, ctrlErr, rollbackErr))
	}
	if audit.active() {
		audit.outcome = flagOutcomeRolledBack
	}
	f.notifyObserver(name)
	return connect.NewError(connect.CodeInternal, fmt.Errorf("failed to %s feature %q: %w", verb, name, ctrlErr))
}

func rollbackFlag(cfg *config.Config, name string, previous, hadKey bool) error {
	if hadKey {
		return cfg.SetFeatureFlag(name, previous)
	}
	return cfg.DeleteFeatureFlag(name)
}

// finishApplied records the applied outcome and tells the observer which scope
// and mutation changed, so history can tell a clear or reset from a set false.
func (f *FeatureFlagService) finishApplied(op flagOp, audit *flagAudit) {
	if audit.active() {
		audit.outcome = flagOutcomeApplied
	}
	if mo, ok := f.observer.(deliverygate.FlagMutationObserver); ok {
		mo.OnFlagMutation(op.name, op.auditScope(), op.mutationName())
		return
	}
	f.notifyObserver(op.name)
}

func (f *FeatureFlagService) notifyObserver(name string) {
	if f.observer != nil {
		f.observer.OnFlagChanged(name)
	}
}
