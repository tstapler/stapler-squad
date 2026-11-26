package server

import (
	"context"
	"os"
	"path/filepath"

	sessionjules "github.com/tstapler/stapler-squad/session/jules"
	"github.com/tstapler/stapler-squad/session/stagecfg"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/jules"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/server/services/backlogsvc"
	"github.com/tstapler/stapler-squad/server/services/triggerlimit"
	"github.com/tstapler/stapler-squad/server/workflows"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/backlog"
	"github.com/tstapler/stapler-squad/session/backlog/chains"
	"github.com/tstapler/stapler-squad/session/backlog/pipeline"
	"github.com/tstapler/stapler-squad/session/cdp"
	"github.com/tstapler/stapler-squad/session/instance"
	"github.com/tstapler/stapler-squad/session/tcc"
	"github.com/tstapler/stapler-squad/session/vnc"
	"github.com/tstapler/stapler-squad/session/wtconsistency"
)

// wireTCCEnrichment installs the TCC consent gate and worktree enricher before
// instances load (ADR-038).
func wireTCCEnrichment(cfg *config.Config, sessionService *services.SessionService) {
	// DetectWorktree's open() can block on an unreadable (TCC-protected) session
	// path, so instance loading consults this enricher's cache instead of probing
	// inline. Registered before LoadInstances; stopped by a server shutdown hook. ADR-038.
	home, homeErr := os.UserHomeDir()
	if homeErr != nil {
		home = "/__unresolved_home__"
	}
	gate := tcc.NewConsentGate(tcc.LexicalPolicy(home), cfg.TCCEnrichmentConsent, func(id string) error {
		return config.UpdateConfig(func(c *config.Config) error {
			if c.TCCEnrichmentConsent == nil {
				c.TCCEnrichmentConsent = map[string]bool{}
			}
			c.TCCEnrichmentConsent[id] = true
			return nil
		})
	})
	enricherConfig := instance.WorktreeEnricherConfig{}
	// Isolated test/e2e instances never hit real protected roots: gate is not_needed.
	tccGateEnabled := cfg.TCCEnrichmentConsentEnabled() && !config.IsIsolatedInstance()
	if cfg.TCCEnrichmentConsentEnabled() && !tccGateEnabled {
		log.Info("TCC enrichment consent gate off: isolated/test instance")
	}
	gate.SetDeniedStore(cfg.TCCEnrichmentDenied, func(id string, denied bool) error {
		return config.UpdateConfig(func(c *config.Config) error {
			if c.TCCEnrichmentDenied == nil {
				c.TCCEnrichmentDenied = map[string]bool{}
			}
			if denied {
				c.TCCEnrichmentDenied[id] = true
			} else {
				delete(c.TCCEnrichmentDenied, id)
			}
			return nil
		})
	})
	enricherConfig.OnResult = gate.ObserveResult
	if tccGateEnabled {
		enricherConfig.Ready = gate.Ready
		enricherConfig.Probe = session.ConsentEnrichmentProbe(gate)
	}
	enricher := installWorktreeEnricher(context.Background(), enricherConfig)
	sessionService.SetTCCEnrichment(gate, enricher, tccGateEnabled)
}

// startHousekeeping starts the worktree consistency sweeper and removes stale user-level
// backlog commands.
func startHousekeeping(storage *session.Storage, eventBus *events.EventBus) {
	// Worktree consistency sweeper: reconciles missing/incorrect Worktree ent rows
	// against live `git worktree` state, gated by FeatureFlagWorktreeConsistencySweep
	// (default off). config.LoadConfig is passed directly, not a closure over cfg, so
	// the flag can be flipped live with no restart (matches quotaGate/julesDispatchSvc).
	go wtconsistency.StartWorktreeConsistencySweeper(context.Background(), storage, &services.EventBusNotifier{Bus: eventBus}, config.LoadConfig)

	// Per-item /backlog/* commands live in session worktrees only; a user-scope copy is stale
	// scaffolding from an old build and shadows the real ones with a wrong item ID.
	if home, hErr := os.UserHomeDir(); hErr == nil {
		if removed, rmErr := pipeline.RemoveStaleUserLevelBacklogCommands(home); rmErr != nil {
			log.WarningLog().Printf("failed to remove stale user-level backlog commands: %v", rmErr)
		} else if removed {
			log.InfoLog().Printf("removed stale user-level ~/.claude/commands/backlog (per-item commands belong in session worktrees)")
		}
	}
}

// checkBrowserDeps probes VNC and CDP availability once at startup (non-fatal).
func checkBrowserDeps() (vnc.DepsResult, cdp.DepsResult) {
	// Check VNC dependencies once at startup so the server knows whether browser
	// passthrough is available on this host. Non-fatal: Missing deps log a warning.
	vncDeps := vnc.CheckDependencies()
	if !vncDeps.Available {
		log.Warn("VNC browser passthrough unavailable", "reason", vncDeps.Reason, "missing", vncDeps.Missing)
	} else {
		log.Info("VNC browser passthrough available")
	}

	// Check CDP (Chrome) dependencies once at startup. Non-fatal.
	cdpDeps := cdp.CheckDependencies()
	if !cdpDeps.Available {
		log.Warn("CDP browser streaming unavailable", "reason", cdpDeps.Reason)
	} else {
		log.Info("CDP browser streaming available", "chrome", cdpDeps.ChromePath)
	}

	return vncDeps, cdpDeps
}

// buildWorkflows builds the workflow repository, scheduler and service plus the chain
// firer; the scheduler and trigger repo are nil when no ent client is available.
func buildWorkflows(svc *ServiceDeps, cfg *config.Config, backlogSvc *backlogsvc.BacklogService,
	backlogLifecycleListener *backlog.BacklogLifecycleListener) (stagecfg.WorkflowRepository, *workflows.Scheduler, session.TriggerFireEventRepository) {
	storage, eventBus, sessionService := svc.Storage, svc.EventBus, svc.SessionService
	// Initialize WorkflowRepository using the ent client from storage.
	// Nil-safe: when the storage is not ent-backed (e.g. tests), WorkflowRepo is nil.
	var workflowRepo stagecfg.WorkflowRepository
	if entClient := storage.GetEntClient(); entClient != nil {
		workflowRepo = stagecfg.NewEntWorkflowRepository(entClient)
		log.Info("WorkflowRepository initialized")
	} else {
		log.Warn("WorkflowRepository unavailable: storage has no ent client")
	}

	// Initialize WorkflowScheduler and WorkflowService with deferred injection.
	// Order: SessionService → WorkflowScheduler → WorkflowService → SessionService.SetWorkflowService
	var workflowScheduler *workflows.Scheduler
	// triggerFireEventRepo is hoisted out of the block below (rather than declared
	// inline) so it's available to RuntimeDeps for the inbound webhook handlers
	// (webhook-triggers Epic 2.2/2.3), which are wired later in server.go independently
	// of the Scheduler/WorkflowService construction above.
	var triggerFireEventRepo session.TriggerFireEventRepository
	if workflowRepo != nil {
		workflowScheduler = workflows.NewScheduler(workflowRepo, sessionService, eventBus)
		// Model-family overrides (e.g. bumping "sonnet"'s "latest" to a new release)
		// live in an optional JSON file so they take effect without a redeploy —
		// same shape as the pricing_overrides.json wiring above.
		if configDir, cfgErr := config.GetConfigDir(); cfgErr == nil {
			overridePath := filepath.Join(configDir, "model_family_overrides.json")
			if families, loadErr := workflows.LoadModelFamilyOverride(overridePath); loadErr == nil {
				workflowScheduler.SetModelFamilies(families)
				// Share the same override with BacklogService's stage-executor
				// resolution (backlog-stage-execution-costs Epic 2.3) so a
				// "family:opus"-style alias resolves identically for both fire
				// paths. backlogSvc defaults to workflows.DefaultModelFamilies()
				// (NewBacklogService) when workflowRepo is nil and this block
				// never runs at all.
				backlogSvc.SetModelFamilies(families)
			} else if !os.IsNotExist(loadErr) {
				log.Warn("failed to load model family override, using defaults", "path", overridePath, "err", loadErr)
			}
		} else {
			log.Warn("failed to resolve config dir, skipping model family override, using defaults", "err", cfgErr)
		}
		workflowSvc := services.NewWorkflowService(workflowRepo, workflowScheduler, storage)
		workflowSvc.SetEventBus(eventBus)
		if attDir, attErr := cfg.WorkflowAttachmentDirOrDefault(); attErr != nil {
			log.Warn("cannot resolve workflow attachment dir; workflow run attachments disabled", "err", attErr)
		} else {
			workflowSvc.SetAttachmentDir(attDir)
		}
		sessionService.SetWorkflowService(workflowSvc)
		sessionService.SetWorkflowRepository(workflowRepo)
		// Close the WIP-gate bypass (webhook-triggers Epic 1.3): every trigger-fired
		// session must pass the same admission check BacklogService's own spawn path
		// enforces. backlogSvc is constructed earlier in this function (see
		// services.NewBacklogService above).
		workflowScheduler.SetAdmissionGate(backlogSvc)
		// Per-Workflow rate limit (webhook-triggers Epic 2.4.2) — shared across cron/
		// manual fires (FireNow) and the inbound webhook handlers (github_push/generic),
		// both of which route through the shared Scheduler.FireTrigger (FireNow is a
		// thin wrapper around it — see server/workflows/scheduler.go).
		workflowScheduler.SetRateLimiter(triggerlimit.NewTriggerRateLimiter())
		if entClient := storage.GetEntClient(); entClient != nil {
			fireEventRepo := session.NewEntTriggerFireEventRepository(entClient)
			workflowScheduler.SetTriggerFireEventRepo(fireEventRepo)
			workflowSvc.SetTriggerFireEventRepo(fireEventRepo)
			triggerFireEventRepo = fireEventRepo
		}
		log.Info("WorkflowService and WorkflowScheduler initialized")

		// ChainFirer + TriggerChainReconciler (webhook-triggers Phase 6, Epic 6.2/
		// 6.3): pipeline chaining needs the same three collaborators as the
		// Scheduler itself (workflowRepo to resolve NextWorkflowID,
		// triggerFireEventRepo for the audit trail, workflowScheduler.
		// FireTriggerChained as the actual fire path), so it can only be wired
		// once all three exist above. Wired to both EntRepository (the happy-path
		// dispatcher, called from TransitionBacklogItemStatus right after a
		// "done" transition commits — AC9) and BacklogLifecycleListener (the
		// restart-recovery reconciler, running on the existing 60s tick).
		if triggerFireEventRepo != nil {
			if chainFirer := chains.WireChainFirer(storage, workflowRepo, triggerFireEventRepo, workflowScheduler, cfg); chainFirer != nil {
				backlogLifecycleListener.SetChainReconciler(chains.NewTriggerChainReconciler(chainFirer))
				log.Info("ChainFirer and TriggerChainReconciler initialized")
			}
		}
	} else {
		log.Warn("WorkflowScheduler disabled: no workflow repository available")
	}

	return workflowRepo, workflowScheduler, triggerFireEventRepo
}

// buildJules wires the Jules key source and, when enabled with a resolvable key, the
// dispatch service and session poller (nil poller otherwise).
func buildJules(svc *ServiceDeps, cfg *config.Config, backlogSvc *backlogsvc.BacklogService) *sessionjules.JulesSessionPoller {
	storage, sessionService := svc.Storage, svc.SessionService
	// Jules dispatch-and-poll integration (google-jules-integration Epic 2.4.4).
	// julesKeys (the OS-keychain token source) is always constructed — cheap,
	// no network I/O — so GetJulesConfig/UpdateJulesConfig work even before
	// the feature is enabled (a user sets the key first, then flips Enabled).
	// The client/source-registry/dispatch-service/poller quartet is only
	// built once Enabled is true AND the key actually resolves at startup;
	// any other outcome (disabled, or an unreadable keychain) degrades the
	// feature, not the server — logged once at Info, everything else
	// unaffected.
	// julesUsage (JulesUsageCounter, Task 4.1.1a) is constructed unconditionally
	// — cheap, no I/O — so GetJulesConfig always has a live (all-zero until
	// something happens) usage snapshot to report, matching julesKeys above.
	julesUsage := services.NewJulesUsageCounter()
	sessionService.SetJulesUsageCounter(julesUsage)

	julesKeys := jules.NewKeyringTokenSource()
	// Rewire SessionService's credChain (built during sessionService's own
	// construction above, before julesKeys existed to share) onto this same
	// instance -- otherwise CredentialChain's JulesCredentialSource holds a
	// second, independent *jules.KeyringTokenSource over the same OS keychain
	// entry, undermining the single-cache/circuit-breaker/singleflight-group
	// invariant documented on jules.KeyringTokenSource. See
	// server/services/credkit/credentials.go's SetJulesTokenSource.
	sessionService.SetJulesKeyringTokenSource(julesKeys)
	var julesSourceRegistry *jules.JulesSourceRegistry
	var julesPoller *sessionjules.JulesSessionPoller
	switch {
	case !cfg.Jules.Enabled:
		log.Info("jules disabled", "reason", "feature not enabled")
	default:
		if _, keyErr := julesKeys.APIKey(context.Background()); keyErr != nil {
			log.Info("jules disabled", "reason", "api key not resolvable", "err", keyErr)
		} else {
			julesClient := jules.NewClient(julesKeys)
			julesSourceRegistry = jules.NewJulesSourceRegistry(julesClient)
			// config.LoadConfig() re-reads config.json from disk on every call
			// (same pattern as quotaGate's QuotaConfig accessor above) — cfg here
			// is the boot-time snapshot passed into BuildRuntimeDeps and is never
			// refreshed, so passing it directly would freeze every Jules config
			// field (egress consent, spend caps) at boot, meaning
			// ConfirmEgressConsent/UpdateJulesConfig/RevokeEgressConsent's writes
			// (jules_config_service.go) would never be observed without a
			// restart.
			julesDispatchSvc := services.NewJulesDispatchService(storage, backlogSvc, julesClient, julesSourceRegistry, config.LoadConfig)
			julesDispatchSvc.SetUsageCounter(julesUsage)
			backlogSvc.SetJulesDispatcher(julesDispatchSvc)
			julesPoller = sessionjules.NewJulesSessionPoller(julesClient, storage, sessionjules.DefaultJulesSessionPollerConfig())
			julesPoller.SetUsageCounter(julesUsage)
		}
	}
	// Passed as separate nil literals (not the possibly-nil *jules.JulesSourceRegistry/
	// *sessionjules.JulesSessionPoller pointers directly) when unconfigured, to avoid handing
	// SetJulesConfigDependencies a non-nil interface wrapping a nil pointer — its own
	// nil checks (julesConfigToProto, TestJulesConnection) compare the interface itself
	// to nil, which a typed-nil pointer would defeat.
	if julesSourceRegistry != nil {
		sessionService.SetJulesConfigDependencies(julesKeys, julesSourceRegistry, julesPoller)
	} else {
		sessionService.SetJulesConfigDependencies(julesKeys, nil, nil)
	}

	return julesPoller
}
