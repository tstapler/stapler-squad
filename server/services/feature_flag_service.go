package services

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"connectrpc.com/connect"
	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/conditional"
	"github.com/tstapler/stapler-squad/server/deliverygate"
	"github.com/tstapler/stapler-squad/server/services/featureflags"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/hostclaim"
)

// piSupportFlagName mirrors config.FeaturePiSupport so the flag registry (feature_flag_specs.go)
// doesn't duplicate the literal — see config/config.go for the flag's full
// documentation and project_plans/pi-support/implementation/plan.md, Epic 2.1.
const piSupportFlagName = config.FeaturePiSupport

// blockApprovalOnCIFailureFlagName is shared between the flag registry (feature_flag_specs.go) and
// ApprovalService.ResolveApproval's CI-red guard (approval_service.go) so the flag
// name can't drift between where it's declared and where it's read.
const blockApprovalOnCIFailureFlagName = "review:block-approval-on-ci-failure"

// BacklogFlagEnabled reports the live state of the backlog flag, read per request.
func BacklogFlagEnabled() bool {
	return config.LoadConfig().GetFeatureFlag(featureflags.BacklogFlagName)
}

// programCLIFlagProbeFlagName is the kill switch for the ProbeProgram RPC.
// Default on; read per request via ProgramCLIFlagProbeEnabled.
const programCLIFlagProbeFlagName = "programs:cli-flag-probe"

// ProgramCLIFlagProbeEnabled reports whether ProbeProgram is allowed. It reads
// config on every call so a Settings toggle applies with no restart.
func ProgramCLIFlagProbeEnabled() bool {
	return config.LoadConfig().GetFeatureFlagWithDefault(
		programCLIFlagProbeFlagName, FeatureFlagDefault(programCLIFlagProbeFlagName))
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

const worktreeChangeDetectionFlagName = "vcs:worktree-change-detection"

// workspacePeersBlockFor is the single feature-flag gate for the workspace-peers nudge,
// called by both SessionService.workspacePeersBlockFor (session_service.go) and
// BacklogService.workspacePeersBlockFor (backlog_service_triage.go) so the two callers can't
// drift on the gate itself, the same way session.WorkspacePeersBlockForPath already keeps
// them from drifting on how the nudge is rendered.
func WorkspacePeersBlockFor(ctx context.Context, storage *session.Storage, repoPath string) string {
	if !config.LoadConfig().GetFeatureFlag(featureflags.WorkspacePeersNudgeFlagName) {
		return ""
	}
	return session.WorkspacePeersBlockForPath(ctx, storage, repoPath)
}

// Files-viewer flags (ADR-031, ADR-036). All global-only; read per request by the
// Files*Enabled helpers below, which take no session argument.
const (
	filesEditingFlagName    = "files:editing"
	filesViewerV2FlagName   = "files:viewer-v2"
	filesMergePhoneFlagName = "files:merge-phone"
	filesRangeReadFlagName  = "files:range-read"
	filesDeltaWriteFlagName = "files:delta-write"
	filesEditLargeFlagName  = "files:edit-large"
)

func filesFlagEnabled(name string) bool {
	return config.LoadConfig().GetFeatureFlagWithDefault(name, FeatureFlagDefault(name))
}

// FilesEditingEnabled reports whether file editing (WriteFile) is allowed. Default: off.
func FilesEditingEnabled(_ context.Context) bool { return filesFlagEnabled(filesEditingFlagName) }

// FilesViewerV2Enabled reports whether the revamped Files viewer is on. Default: on.
func FilesViewerV2Enabled(_ context.Context) bool { return filesFlagEnabled(filesViewerV2FlagName) }

// FilesMergePhoneEnabled reports whether the phone merge UI is on. Default: on.
func FilesMergePhoneEnabled(_ context.Context) bool { return filesFlagEnabled(filesMergePhoneFlagName) }

// FilesRangeReadEnabled reports whether ranged large-file reads are on (ADR-036). Default: off.
func FilesRangeReadEnabled(_ context.Context) bool { return filesFlagEnabled(filesRangeReadFlagName) }

// FilesDeltaWriteEnabled reports whether delta writes are on (ADR-036). Default: off.
func FilesDeltaWriteEnabled(_ context.Context) bool { return filesFlagEnabled(filesDeltaWriteFlagName) }

// FilesEditLargeEnabled reports whether large-file editing is on (ADR-036). Default: off.
func FilesEditLargeEnabled(_ context.Context) bool { return filesFlagEnabled(filesEditLargeFlagName) }

// workflowEngineModeTTL is how long a resolved engine mode is reused, so the
// transition hot path never reads config per call.
const workflowEngineModeTTL = time.Second

// WorkflowEngineModeFromConfig resolves the engine mode from cfg, reading both
// flags with their registered defaults (never the plain GetFeatureFlag, which
// returns false for an unset flag and would silently turn shadow off).
func WorkflowEngineModeFromConfig(cfg *config.Config) session.EngineMode {
	return session.ModeFromFlags(
		cfg.GetFeatureFlagWithDefault(config.WorkflowEngineShadowFeatureFlag, FeatureFlagDefault(config.WorkflowEngineShadowFeatureFlag)),
		cfg.GetFeatureFlagWithDefault(config.WorkflowEngineEnforceFeatureFlag, FeatureFlagDefault(config.WorkflowEngineEnforceFeatureFlag)),
	)
}

// NewWorkflowEngineModeSource is the production mode reader: live config,
// cached for workflowEngineModeTTL. A nil clock uses real time.
func NewWorkflowEngineModeSource(load func() *config.Config, clock hostclaim.Clock) *session.CachedModeSource {
	return session.NewCachedModeSource(func() session.EngineMode { return WorkflowEngineModeFromConfig(load()) }, workflowEngineModeTTL, clock)
}

// NewWorkflowEngineClassModeSource is the production per-class mode reader:
// each class resolves its own enforce flag and the shared shadow flag from live
// config, cached for workflowEngineModeTTL. A nil clock uses real time.
func NewWorkflowEngineClassModeSource(load func() *config.Config, clock hostclaim.Clock) *session.FlagClassModeSource {
	return session.NewFlagClassModeSource(load, workflowEngineModeTTL, clock)
}

// WorkflowEngineShadowStatusDetail renders "Shadow: N divergences/24h" for the
// shadow flag's status line, counted from the ledger's repro ring (the last 200
// divergences), so a busy day can read as a floor rather than an exact count.
func WorkflowEngineShadowStatusDetail(snapshot func() session.ShadowLedger, now func() time.Time) func() string {
	return func() string {
		cutoff := now().Add(-24 * time.Hour)
		n := 0
		for _, r := range snapshot().Repro {
			if r.At.After(cutoff) {
				n++
			}
		}
		return fmt.Sprintf("Shadow: %d divergences/24h", n)
	}
}

// FeatureFlagDefault looks up name's default in the process registry installed by
// featureflags.SetCurrent at startup — the single source of truth both GetFeatureFlags
// and a flag's own call site (e.g. currentResyncOptions' UseFastLane) resolve against.
// Returns false for an unregistered name; panics if no registry was installed.
func FeatureFlagDefault(name string) bool {
	return featureflags.Current().Default(name)
}

// FlagRegistry builds the process registry from the services-owned specs plus the
// specs contributed by the composition root (leaf packages' own flags).
func FlagRegistry(extra ...[]featureflags.Spec) *featureflags.Registry {
	specs := coreFlagSpecs()
	for _, e := range extra {
		specs = append(specs, e...)
	}
	return featureflags.NewRegistry(specs)
}

// FeatureFlagService handles GetFeatureFlags and UpdateFeatureFlag RPCs.
// It reads the featureflags registry (featureflags.Current) and owns the per-name FeatureController map.
// Extracted from SessionService per ADR-001 (UpdateFeatureFlag exceeds the 30-line threshold).
type FeatureFlagService struct {
	// featureControllers maps feature flag names to their runtime controllers.
	// Wired via SetFeatureController. May be nil for features that only need
	// config-file persistence (no in-process component to toggle).
	featureControllers map[string]featureflags.FeatureController

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
	auditPolicies map[string]featureflags.FlagAuditPolicy
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
func (f *FeatureFlagService) SetFeatureController(name string, c featureflags.FeatureController) {
	if f.featureControllers == nil {
		f.featureControllers = make(map[string]featureflags.FeatureController)
	}
	f.featureControllers[name] = c
}

// SetStatusDetailProvider wires an optional status-detail provider for the
// named feature flag. GetFeatureFlags calls fn on every request and populates
// FeatureFlag.StatusDetail with its result (empty string when fn returns "").
func (f *FeatureFlagService) SetStatusDetailProvider(name string, fn func() string) {
	f.setStatusDetailSlot(name, providerSlotKey, ignoreConfig(fn))
}

// providerSlotKey names the slot SetStatusDetailProvider owns; a second call
// replaces it in place so the setter keeps its replace-by-name meaning.
const providerSlotKey = "provider"

type statusDetailSlot struct {
	key string
	fn  func(*config.Config) string
}

// ignoreConfig adapts a config-free source to the cfg-taking slot signature.
func ignoreConfig(fn func() string) func(*config.Config) string {
	return func(*config.Config) string { return fn() }
}

func (f *FeatureFlagService) setStatusDetailSlot(name, key string, fn func(*config.Config) string) {
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
	f.setStatusDetailSlot(name, "", ignoreConfig(fn))
}

// AddConfigStatusDetailSource is AddStatusDetailSource for a source that reads
// config.json: it is handed the one snapshot GetFeatureFlags loaded, so listing
// flags parses the file once however many sources are wired.
func (f *FeatureFlagService) AddConfigStatusDetailSource(name string, fn func(*config.Config) string) {
	f.setStatusDetailSlot(name, "", fn)
}

func (f *FeatureFlagService) statusDetailFor(name string, cfg *config.Config) string {
	parts := autonomyFlagStatusDetail(name, cfg)
	for _, s := range f.statusDetailSlots[name] {
		if d := s.fn(cfg); d != "" {
			parts = append(parts, d)
		}
	}
	return strings.Join(parts, "; ")
}

// autonomyFlagStatusDetail reports, for the two nudge flags, why a flag that is
// On does nothing ("inactive: requires <flag>", parsed by the Features page) and
// whether its value is the registry default or was set explicitly.
func autonomyFlagStatusDetail(name string, cfg *config.Config) []string {
	if name != config.DiagnoseNudgeFeatureFlag && name != config.AutonomousStallNudgeFeatureFlag {
		return nil
	}
	var parts []string
	if name == config.AutonomousStallNudgeFeatureFlag &&
		config.EffectiveAutonomousStallNudgeEnabled(cfg) && !config.EffectiveDiagnoseNudgeEnabled(cfg) {
		parts = append(parts, "inactive: requires "+config.DiagnoseNudgeFeatureFlag)
	}
	onOff := func(b bool) string {
		if b {
			return "On"
		}
		return "Off"
	}
	if value, explicit := cfg.FeatureFlags[name]; explicit {
		parts = append(parts, "Set explicitly: "+onOff(value))
	} else {
		parts = append(parts, "Default: "+onOff(FeatureFlagDefault(name))+" (not set explicitly)")
	}
	return parts
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
func (f *FeatureFlagService) SetAudit(sink *AuditSink, policies map[string]featureflags.FlagAuditPolicy) {
	f.audit, f.auditPolicies = sink, policies
}

// MergeAuditPolicies adds per-flag audit policies to those SetAudit installed.
func (f *FeatureFlagService) MergeAuditPolicies(policies map[string]featureflags.FlagAuditPolicy) {
	if f.auditPolicies == nil {
		f.auditPolicies = make(map[string]featureflags.FlagAuditPolicy, len(policies))
	}
	for k, v := range policies {
		f.auditPolicies[k] = v
	}
}

// +api: feature-flags:list
// GetFeatureFlags returns all known feature flags and their current state.
func (f *FeatureFlagService) GetFeatureFlags(
	ctx context.Context,
	req *connect.Request[sessionv1.GetFeatureFlagsRequest],
) (*connect.Response[sessionv1.GetFeatureFlagsResponse], error) {
	cfg := config.LoadConfig()

	specs := featureflags.Current().Specs()
	flags := make([]*sessionv1.FeatureFlag, 0, len(specs))
	for _, kf := range specs {
		enabled := cfg.GetFeatureFlagWithDefault(kf.Name, kf.Default)
		// If a controller is wired, its live state is the source of truth.
		if ctrl, ok := f.featureControllers[kf.Name]; ok {
			enabled = ctrl.IsEnabled()
		}
		flags = append(flags, &sessionv1.FeatureFlag{
			Name:         kf.Name,
			Enabled:      enabled,
			Description:  kf.Description,
			StatusDetail: f.statusDetailFor(kf.Name, cfg),
			Scopes:       flagScopeReadback(cfg, kf.Name, kf.Scopes),
		})
	}

	full := &sessionv1.GetFeatureFlagsResponse{Flags: flags}
	etag, notModified := conditional.Check("GetFeatureFlags", conditional.Validator(req.Msg.GetIfNoneMatch()), full)
	if notModified {
		return connect.NewResponse(&sessionv1.GetFeatureFlagsResponse{Etag: string(etag), NotModified: true}), nil
	}
	full.Etag = string(etag)
	return connect.NewResponse(full), nil
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
	spec, known := featureflags.Current().Lookup(name)
	description, scopes := spec.Description, spec.Scopes
	if !known {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("unknown feature flag %q: valid flags are %v", name, featureflags.Current().Names()))
	}
	op, err := parseFlagOp(req.Msg, scopes)
	if err != nil {
		return nil, err
	}

	// Only the way on is guarded: disabling and resetting the global value are
	// never refused. Clearing a scope is guarded only when it turns the kind on.
	if op.enabling() || clearTurnsKindOn(op) {
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
	endAuditSpan := flagSpan(ctx, op, flagPhaseAudit)
	audit, err := f.auditBegin(ctx, op, flagRequestFields(ctx, req.Peer().Addr, req.Header()))
	endAuditSpan()
	if err != nil {
		log.Error("feature flag change not audited, nothing persisted", "feature", name, "err", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("flag change not audited, nothing persisted"))
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
	enabled := cfg.GetFeatureFlagWithDefault(name, FeatureFlagDefault(name))
	if op.scope == "" && op.isSet() {
		enabled = op.enabled // the controller-backed state, as before this change
	}
	log.Info("feature flag updated", "feature", name, "enabled", enabled, "scope", op.auditScope(), "mutation", op.mutationName())
	return connect.NewResponse(&sessionv1.UpdateFeatureFlagResponse{
		Flag: &sessionv1.FeatureFlag{
			Name:         name,
			Enabled:      enabled,
			Description:  description,
			StatusDetail: f.statusDetailFor(name, cfg),
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
	var previous, hadKey bool
	endPersistSpan := flagSpan(ctx, op, flagPhasePersist)
	err := config.UpdateConfig(func(cfg *config.Config) error {
		previous, hadKey = cfg.GetFeatureFlagOverride(name)
		if !hadKey {
			previous = FeatureFlagDefault(name)
		}
		// In-memory mutators: UpdateConfig saves once, so the saving setters
		// here would fsync config.json twice per flip.
		if op.isSet() {
			cfg.ApplyFeatureFlag(name, enabled)
		} else {
			cfg.RemoveFeatureFlag(name)
		}
		return nil
	})
	endPersistSpan()
	if audit.active() {
		f.flagSeq++
		audit.seq, audit.previous = f.flagSeq, previous
	}
	if err != nil {
		return connect.NewError(connect.CodeInternal, fmt.Errorf("failed to persist feature flag: %w", err))
	}
	ctrl, ok := f.featureControllers[name]
	if !ok {
		f.finishApplied(op, audit)
		return nil
	}
	defer flagSpan(ctx, op, flagPhaseApply)()
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
	if rollbackErr := rollbackFlag(name, previous, hadKey); rollbackErr != nil {
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

func rollbackFlag(name string, previous, hadKey bool) error {
	return config.UpdateConfig(func(cfg *config.Config) error {
		if hadKey {
			cfg.ApplyFeatureFlag(name, previous)
		} else {
			cfg.RemoveFeatureFlag(name)
		}
		return nil
	})
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
