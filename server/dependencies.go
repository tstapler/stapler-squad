package server

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/log"
	warren "github.com/tstapler/stapler-squad/pkg/warren"
	"github.com/tstapler/stapler-squad/server/analytics"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/scrollback"
	"github.com/tstapler/stapler-squad/session/tmux"
	"github.com/tstapler/stapler-squad/session/unfinished"
)

// ServerDependencies holds all wired service components for the HTTP server.
// Use BuildDependencies to construct and wire them in the correct order.
// See the initialization order comment on NewServer for dependency constraints.
type ServerDependencies struct {
	SessionService          *services.SessionService
	Storage                 *session.Storage
	Instances               []*session.Instance
	EventBus                *events.EventBus
	StatusManager           *session.InstanceStatusManager
	ReviewQueue             *session.ReviewQueue
	ReviewQueuePoller       *session.ReviewQueuePoller
	PRStatusPoller          *session.PRStatusPoller
	ReactiveQueueMgr        *ReactiveQueueManager
	ScrollbackManager       *scrollback.ScrollbackManager
	TmuxStreamerManager     *session.ExternalTmuxStreamerManager
	ExternalDiscovery       *session.ExternalSessionDiscovery
	ExternalApprovalMonitor *session.ExternalApprovalMonitor
	HistoryLinker           *session.HistoryLinker
	ErrorRegistry           *services.ErrorRegistry

	// Unfinished work scanning.
	UnfinishedScanner     *unfinished.Scanner
	UnfinishedStateStore  *unfinished.StateStore
	UnfinishedWorkService *services.UnfinishedWorkService

	// Analytics storage. Nil when the analytics DB failed to open (LogAnalyticsProvider
	// is used as a fallback in that case).
	AnalyticsEntClient *ent.Client
}

// ToServerDeps converts RuntimeDeps to the flat ServerDependencies struct consumed
// by NewServerWithDeps. This mirrors the projection done inside BuildDependencies.
func (rt *RuntimeDeps) ToServerDeps() *ServerDependencies {
	return &ServerDependencies{
		SessionService:          rt.SessionService,
		Storage:                 rt.Storage,
		Instances:               rt.Instances,
		EventBus:                rt.EventBus,
		StatusManager:           rt.StatusManager,
		ReviewQueue:             rt.ReviewQueue,
		ReviewQueuePoller:       rt.ReviewQueuePoller,
		PRStatusPoller:          rt.PRStatusPoller,
		ReactiveQueueMgr:        rt.ReactiveQueueMgr,
		ScrollbackManager:       rt.ScrollbackManager,
		TmuxStreamerManager:     rt.TmuxStreamerManager,
		ExternalDiscovery:       rt.ExternalDiscovery,
		ExternalApprovalMonitor: rt.ExternalApprovalMonitor,
		HistoryLinker:           rt.HistoryLinker,
		ErrorRegistry:           rt.ErrorRegistry,
		UnfinishedScanner:       rt.UnfinishedScanner,
		UnfinishedStateStore:    rt.UnfinishedStateStore,
		UnfinishedWorkService:   rt.UnfinishedWorkService,
		AnalyticsEntClient:      rt.AnalyticsEntClient,
	}
}

// BuildDependencies constructs and wires all server dependencies in the correct order.
// Returns an error only for unrecoverable failures (SessionService init, Storage start).
// Non-fatal failures (individual instance start) are logged and skipped.
//
// Delegates to the three-phase constructors: BuildCoreDeps -> BuildServiceDeps -> BuildRuntimeDeps.
func BuildDependencies() (*ServerDependencies, error) {
	// Phase 1 (core): SessionService, Storage, EventBus, ReviewQueue, ApprovalStore
	// was: step 1 - SessionService + getter calls
	core, err := BuildCoreDeps()
	if err != nil {
		return nil, fmt.Errorf("phase 1 (core): %w", err)
	}

	// Phase 2 (services): StatusManager, ReviewQueuePoller, wiring into SessionService
	// was: steps 2-3 - StatusManager, ReviewQueuePoller, SetApprovalProvider, SetStatusManager, SetReviewQueuePoller
	svc, err := BuildServiceDeps(core)
	if err != nil {
		return nil, fmt.Errorf("phase 2 (services): %w", err)
	}

	// Phase 3 (runtime): ensure tmux server running, then load instances.
	// EnsureServerRunning must precede BuildRuntimeDeps — the token enforces it.
	tmuxReady, err := tmux.EnsureServerRunning("")
	if err != nil {
		log.WarningLog.Printf("BuildDependencies: failed to ensure tmux server running: %v", err)
	}
	rt, err := BuildRuntimeDeps(tmuxReady, svc)
	if err != nil {
		return nil, fmt.Errorf("phase 3 (runtime): %w", err)
	}

	return rt.ToServerDeps(), nil
}

// syncOrphanedApprovalsToQueue adds review queue items for orphaned (persisted) approvals.
// This ensures sessions with known pending approvals appear in the queue immediately on startup,
// even before the first poll cycle detects them via terminal content scanning.
func syncOrphanedApprovalsToQueue(
	store *services.ApprovalStore,
	instances []*session.Instance,
	queue session.ReviewQueueWriter,
) {
	if store == nil {
		return
	}

	orphaned := store.ListAll()
	if len(orphaned) == 0 {
		return
	}

	// Build a lookup map for instances by title
	instMap := make(map[string]*session.Instance, len(instances))
	for _, inst := range instances {
		instMap[inst.Title] = inst
	}

	added := 0
	for _, approval := range orphaned {
		if !approval.Orphaned {
			continue
		}

		// Build context from approval metadata
		context := fmt.Sprintf("Permission required: %s", approval.ToolName)
		if cmd, ok := approval.ToolInput["command"].(string); ok && cmd != "" {
			if len(cmd) > 120 {
				context = cmd[:120] + "..."
			} else {
				context = cmd
			}
		}

		item := &session.ReviewItem{
			SessionID:   approval.SessionID,
			SessionName: approval.SessionID,
			Reason:      session.ReasonApprovalPending,
			Priority:    session.PriorityHigh,
			DetectedAt:  approval.CreatedAt,
			Context:     context,
			Metadata: map[string]string{
				"pending_approval_id": approval.ID,
				"tool_name":           approval.ToolName,
				"orphaned":            "true",
			},
			LastActivity: approval.CreatedAt,
		}

		// Enrich with instance data if available
		if inst, ok := instMap[approval.SessionID]; ok {
			item.Program = inst.Program
			item.Branch = inst.Branch
			item.Path = inst.Path
			item.WorkingDir = inst.WorkingDir
			item.Status = inst.Status.String()
			item.Tags = inst.Tags
			item.Category = inst.Category
			item.DiffStats = inst.GetDiffStats()
			if !inst.LastMeaningfulOutput.IsZero() {
				item.LastActivity = inst.LastMeaningfulOutput
			}
		}

		queue.Add(item)
		added++
		log.InfoLog.Printf("[ApprovalSync] Added orphaned approval to review queue: session=%s, tool=%s, approval_id=%s",
			approval.SessionID, approval.ToolName, approval.ID)
	}

	if added > 0 {
		log.InfoLog.Printf("[ApprovalSync] Synced %d orphaned approvals to review queue", added)
	}
}

// ---------------------------------------------------------------------------
// Phased dependency structs (Dependency Initialization Hardening)
//
// These types decompose BuildDependencies into three ordered phases:
//   Phase 1 (CoreDeps)    - foundational components with no external prerequisites
//   Phase 2 (ServiceDeps) - management components that depend on CoreDeps
//   Phase 3 (RuntimeDeps) - runtime components involving processes and I/O
//
// BuildDependencies delegates to BuildCoreDeps -> BuildServiceDeps -> BuildRuntimeDeps.
// ---------------------------------------------------------------------------

// CoreDeps holds the foundational dependencies created during Phase 1.
// These have no external prerequisites and form the base for all other components.
type CoreDeps struct {
	SessionService *services.SessionService
	Storage        *session.Storage
	EventBus       *events.EventBus
	ReviewQueue    *session.ReviewQueue
	ApprovalStore  *services.ApprovalStore
	ErrorRegistry  *services.ErrorRegistry
}

// BuildOptions carries optional overrides for BuildCoreDepsWithOptions.
// The zero value uses all defaults (equivalent to calling BuildCoreDeps).
type BuildOptions struct {
	// EntClient supplies a pre-opened *ent.Client, bypassing config-based DB path
	// discovery and schema migration. nil = open from config as usual.
	EntClient *ent.Client
}

// BuildCoreDepsWithOptions constructs Phase 1 dependencies with optional overrides.
// Use BuildOptions to inject a pre-built EntClient (for tests).
func BuildCoreDepsWithOptions(opts BuildOptions) (*CoreDeps, error) {
	var sessionService *services.SessionService
	var err error
	if opts.EntClient != nil {
		sessionService, err = services.NewSessionServiceWithEntClient(opts.EntClient)
	} else {
		sessionService, err = services.NewSessionServiceFromConfig()
	}
	if err != nil {
		return nil, fmt.Errorf("initialize SessionService: %w", err)
	}

	storage := sessionService.GetStorage()

	// Wire the ErrorRegistry using the existing ent client from Storage.
	// GetEntClient returns nil when storage is not ent-backed (e.g. in tests),
	// in which case ErrorRegistry gracefully disables itself.
	errorRegistry := services.NewErrorRegistry(storage.GetEntClient(), true)

	w := warren.NewWire("CoreDeps")
	warren.Set(w, "ErrorRegistry", sessionService.SetErrorRegistry, errorRegistry)
	if err := w.Validate(); err != nil {
		return nil, err
	}

	return &CoreDeps{
		SessionService: sessionService,
		Storage:        storage,
		EventBus:       sessionService.GetEventBus(),
		ReviewQueue:    sessionService.GetReviewQueueInstance(),
		ApprovalStore:  sessionService.GetApprovalStore(),
		ErrorRegistry:  errorRegistry,
	}, nil
}

// BuildCoreDeps constructs Phase 1 dependencies using config defaults.
// It is a thin wrapper around BuildCoreDepsWithOptions(BuildOptions{}).
func BuildCoreDeps() (*CoreDeps, error) {
	return BuildCoreDepsWithOptions(BuildOptions{})
}

// ServiceDeps holds Phase 2 dependencies: management components that depend on CoreDeps.
type ServiceDeps struct {
	*CoreDeps
	StatusManager     *session.InstanceStatusManager
	ReviewQueuePoller *session.ReviewQueuePoller
	PRStatusPoller    *session.PRStatusPoller
}

// BuildServiceDeps constructs Phase 2 dependencies using Phase 1 outputs.
// Compile-time guarantee: cannot be called without a *CoreDeps.
func BuildServiceDeps(core *CoreDeps) (*ServiceDeps, error) {
	if core == nil {
		return nil, fmt.Errorf("BuildServiceDeps: CoreDeps is nil (Phase 1 not completed)")
	}
	if core.Storage == nil || core.EventBus == nil || core.ReviewQueue == nil ||
		core.SessionService == nil || core.ApprovalStore == nil {
		return nil, fmt.Errorf("BuildServiceDeps: CoreDeps has nil fields")
	}

	statusManager := session.NewInstanceStatusManager()
	reviewQueuePoller := session.NewReviewQueuePoller(
		core.ReviewQueue, statusManager, core.Storage,
	)
	prStatusPoller := session.NewPRStatusPoller(core.Storage)

	w := warren.NewWire("ServiceDeps")
	warren.Set(w, "ApprovalProvider", reviewQueuePoller.SetApprovalProvider, session.ApprovalMetadataProvider(core.ApprovalStore))
	warren.Set(w, "StatusManager", core.SessionService.SetStatusManager, statusManager)
	warren.Set(w, "ReviewQueuePoller", core.SessionService.SetReviewQueuePoller, reviewQueuePoller)
	if err := w.Validate(); err != nil {
		return nil, err
	}

	return &ServiceDeps{
		CoreDeps:          core,
		StatusManager:     statusManager,
		ReviewQueuePoller: reviewQueuePoller,
		PRStatusPoller:    prStatusPoller,
	}, nil
}

// RuntimeDeps holds Phase 3 dependencies: runtime components that involve
// process creation, filesystem I/O, and callback wiring.
type RuntimeDeps struct {
	*ServiceDeps
	Instances               []*session.Instance
	ReactiveQueueMgr        *ReactiveQueueManager
	ScrollbackManager       *scrollback.ScrollbackManager
	TmuxStreamerManager     *session.ExternalTmuxStreamerManager
	ExternalDiscovery       *session.ExternalSessionDiscovery
	ExternalApprovalMonitor *session.ExternalApprovalMonitor
	PRStatusPoller          *session.PRStatusPoller
	HistoryLinker           *session.HistoryLinker
	ErrorRegistry           *services.ErrorRegistry

	// Unfinished work scanning.
	UnfinishedScanner     *unfinished.Scanner
	UnfinishedStateStore  *unfinished.StateStore
	UnfinishedWorkService *services.UnfinishedWorkService

	// Analytics storage.
	AnalyticsEntClient *ent.Client
}

// BuildRuntimeDeps constructs Phase 3 dependencies using Phase 2 outputs.
// This implements steps 5-12 from the original BuildDependencies:
//   - Step 5: LoadInstances + wire ReviewQueue/StatusManager on each instance
//   - Step 6: Start tmux sessions for loaded instances (non-fatal failures)
//   - Step 6.5: Persist auto-detected worktree info
//   - Step 7: Start controllers for running instances
//   - Step 7.5: Startup scan + orphaned approval sync
//   - Step 8: ReactiveQueueManager + wire into SessionService
//   - Step 9: ScrollbackManager (independent)
//   - Step 10: TmuxStreamerManager (independent)
//   - Step 11: ExternalDiscovery with session-added/removed callbacks
//   - Step 12: ExternalApprovalMonitor with approval-to-review-queue bridge
//   - SetExternalDiscovery on SessionService (moved from server.go)
//
// BuildRuntimeDeps requires a TmuxServerReady token to enforce that
// tmux.EnsureServerRunning was called before sessions are loaded. Without this
// ordering, DoesSessionExist() may trigger recoverFromServerFailure, which starts
// a fresh server that considers all sessions non-existent and cold-restores them.
func BuildRuntimeDeps(_ tmux.TmuxServerReady, svc *ServiceDeps) (*RuntimeDeps, error) {
	if svc == nil {
		return nil, fmt.Errorf("BuildRuntimeDeps: ServiceDeps is nil (Phase 2 not completed)")
	}

	// Alias embedded fields for readability (matches original BuildDependencies local vars).
	storage := svc.Storage
	reviewQueue := svc.ReviewQueue
	statusManager := svc.StatusManager
	reviewQueuePoller := svc.ReviewQueuePoller
	eventBus := svc.EventBus
	sessionService := svc.SessionService

	// Step 5: load instances from storage
	instances, err := storage.LoadInstances()
	if err != nil {
		return nil, fmt.Errorf("load instances: %w", err)
	}

	// Step 5 (continued): wire dependencies to each instance
	// inst.SetReviewQueue and inst.SetStatusManager are called per-instance in a loop;
	// Warren is designed for named scalar setters, not loop iterations. Left unwrapped.
	for _, inst := range instances {
		inst.SetReviewQueue(reviewQueue)
		inst.SetStatusManager(statusManager)
	}

	// Wire instances to pollers.
	// SetInstances accepts a slice (non-comparable) so use SetAlways (skips nil check).
	w2 := warren.NewWire("RuntimeDeps.Pollers")
	warren.SetAlways(w2, "ReviewQueuePoller.Instances", reviewQueuePoller.SetInstances, instances)
	warren.SetAlways(w2, "PRStatusPoller.Instances", svc.PRStatusPoller.SetInstances, instances)
	warren.SetAlways(w2, "PRStatusPoller.OnUpdated", svc.PRStatusPoller.SetOnUpdated, func(inst *session.Instance) {
		eventBus.Publish(events.NewSessionUpdatedEvent(inst, []string{"github_pr_priority", "github_pr_state"}))
	})
	if err := w2.Validate(); err != nil {
		return nil, err
	}

	// Perform heavy initialization (tmux starting, controllers, scanning) in the background
	// so the HTTP server can bind and start immediately.
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.ErrorLog.Printf("[startup] panic in background init goroutine: %v", r)
			}
		}()
		// Step 6: start tmux sessions for loaded instances (non-fatal failures).
		// Stagger starts by 200ms each to avoid a fork burst that saturates the
		// cgroup pids.max limit when many sessions restore simultaneously.
		for i, inst := range instances {
			if !inst.Started() {
				if i > 0 {
					time.Sleep(200 * time.Millisecond)
				}
				if err := inst.Start(false); err != nil {
					log.ErrorLog.Printf("Failed to start loaded instance '%s': %v", inst.Title, err)
				} else {
					log.InfoLog.Printf("Started loaded instance '%s'", inst.Title)
				}
			}
		}

		// Step 6b: reconcile Stopped sessions that have a live tmux session.
		// This handles the case where the server crashed or restarted while a session
		// was running — the DB recorded Stopped but the tmux session survived.
		// RecoverFromStopped resets the status to Ready (bypassing the terminal-state
		// guard) so Start(false) can hot-attach to the existing tmux session.
		for _, inst := range instances {
			if inst.Status == session.Stopped && inst.TmuxSessionExists() {
				log.InfoLog.Printf("Reconcile: session '%s' is Stopped in DB but tmux is alive — restoring", inst.Title)
				inst.RecoverFromStopped()
				if err := inst.Start(false); err != nil {
					log.WarningLog.Printf("Reconcile: hot-restore of '%s' failed: %v", inst.Title, err)
				} else {
					log.InfoLog.Printf("Reconcile: restored '%s' (was Stopped, now Running)", inst.Title)
				}
			}
		}

		// Step 6.5: Persist any auto-detected worktree info (must happen after Step 6)
		if len(instances) > 0 {
			if err := storage.SaveInstances(instances); err != nil {
				log.WarningLog.Printf("Failed to persist migrated instance data: %v", err)
			} else {
				log.InfoLog.Printf("Persisted migrated instance data for %d instances", len(instances))
			}
		}

		// Step 7: start controllers (requires started instances + StatusManager)
		log.InfoLog.Printf("Attempting controller startup for %d loaded instances", len(instances))
		for _, inst := range instances {
			started := inst.Started()
			paused := inst.Paused()
			if started && !paused && inst.Status != session.Stopped {
				if inst.GetController() == nil {
					if err := inst.StartController(); err != nil {
						log.WarningLog.Printf("Failed to start controller for '%s': %v", inst.Title, err)
					} else {
						log.InfoLog.Printf("Started controller for '%s'", inst.Title)
					}
				}
			}
		}

		// Step 7.5: Startup scan and orphaned approval sync
		// Brief settling delay to allow controllers to initialize their terminal readers.
		time.Sleep(500 * time.Millisecond)
		contentProvider := session.NewPollerContentProvider()
		scanner := session.NewStartupScanner(statusManager, contentProvider)
		scanner.Scan(instances, reviewQueue)
		syncOrphanedApprovalsToQueue(svc.ApprovalStore, instances, reviewQueue)
	}()

	// Step 8: ReactiveQueueManager
	reactiveQueueMgr := NewReactiveQueueManager(reviewQueue, reviewQueuePoller, eventBus, statusManager, storage)
	log.InfoLog.Printf("ReactiveQueueManager initialized")

	// Step 8.5: HistoryLinker — detects Claude JSONL files and links conversation
	// UUIDs to sessions so cold restore can use --resume on restart.
	historyLinker := session.NewHistoryLinkerFromRealInspector()
	log.InfoLog.Printf("HistoryLinker initialized with %d instances", len(instances))

	// Step 9: ScrollbackManager (independent of above)
	homeDir, _ := os.UserHomeDir()
	scrollbackPath := filepath.Join(homeDir, ".stapler-squad", "sessions")
	scrollbackConfig := scrollback.DefaultScrollbackConfig()
	scrollbackConfig.StoragePath = scrollbackPath
	scrollbackManager := scrollback.NewScrollbackManager(scrollbackConfig)
	log.InfoLog.Printf("Initialized ScrollbackManager: path=%s, compression=%s, maxLines=%d",
		scrollbackPath, scrollbackConfig.StoragePath, scrollbackConfig.MaxLines)

	// Step 10: TmuxStreamerManager (independent)
	tmuxStreamerManager := session.NewExternalTmuxStreamerManager()

	// Step 11: ExternalDiscovery with session-added/removed callbacks
	externalDiscovery := session.NewExternalSessionDiscovery()
	externalDiscovery.OnSessionAdded(func(instance *session.Instance) {
		if err := storage.AddInstance(instance); err != nil {
			log.ErrorLog.Printf("Failed to persist external session '%s': %v", instance.Title, err)
		} else {
			log.InfoLog.Printf("Persisted external session '%s' to storage", instance.Title)
		}
		// Wire dependencies so the external session appears in the review queue
		instance.SetReviewQueue(reviewQueue)
		instance.SetStatusManager(statusManager)
		reviewQueuePoller.AddInstance(instance)
		svc.PRStatusPoller.AddInstance(instance)
		historyLinker.AddInstance(instance)
		log.InfoLog.Printf("Added external session '%s' to review queue poller, PR status poller, and history linker", instance.Title)
	})
	externalDiscovery.OnSessionRemoved(func(instance *session.Instance) {
		reviewQueuePoller.RemoveInstance(instance.Title)
		svc.PRStatusPoller.RemoveInstance(instance.Title)
		historyLinker.RemoveInstance(instance.Title)
		log.InfoLog.Printf("Removed external session '%s' from review queue poller, PR status poller, and history linker", instance.Title)
		reviewQueue.Remove(instance.Title)
		if err := storage.DeleteInstance(instance.Title); err != nil {
			log.WarningLog.Printf("Failed to remove external session '%s' from storage: %v", instance.Title, err)
		} else {
			log.InfoLog.Printf("Removed external session '%s' from storage", instance.Title)
		}
	})

	// Step 12: ExternalApprovalMonitor — wire approval-to-review-queue bridge
	externalApprovalMonitor := session.NewExternalApprovalMonitor()
	externalApprovalMonitor.OnApproval(func(event *session.ExternalApprovalEvent) {
		if event == nil || event.Request == nil {
			return
		}
		// Resolve the instance (try tmux session name first, socket path as fallback)
		inst := externalDiscovery.GetSessionByTmux(event.SessionID)
		if inst == nil {
			inst = externalDiscovery.GetSession(event.SessionID)
		}

		context := event.Request.DetectedText
		if context == "" {
			context = "Permission request detected"
		}

		item := &session.ReviewItem{
			SessionID:   event.SessionTitle,
			SessionName: event.SessionTitle,
			Reason:      session.ReasonApprovalPending,
			Priority:    session.PriorityHigh,
			DetectedAt:  event.Request.Timestamp,
			Context:     context,
		}
		if inst != nil {
			item.Program = inst.Program
			item.Branch = inst.Branch
			item.Path = inst.Path
			item.WorkingDir = inst.WorkingDir
			item.Status = inst.Status.String()
			item.Tags = inst.Tags
			item.Category = inst.Category
			item.DiffStats = inst.GetDiffStats()
			item.LastActivity = inst.LastMeaningfulOutput
		}

		reviewQueue.Add(item)
		log.InfoLog.Printf("Added external session approval '%s' to review queue (type: %s, confidence: %.2f)",
			event.SessionTitle, event.Request.Type, event.Request.Confidence)
	})

	// Wire external discovery to SessionService for unified session listing
	// (moved from server.go to keep all dependency wiring in BuildRuntimeDeps)

	w3 := warren.NewWire("RuntimeDeps.SessionService")
	// ReactiveQueueManager is an exported interface; cast to infer correct type param.
	warren.Set(w3, "ReactiveQueueManager", sessionService.SetReactiveQueueManager, services.ReactiveQueueManager(reactiveQueueMgr))
	warren.Set(w3, "HistoryLinker", sessionService.SetHistoryLinker, historyLinker)
	// SetInstances accepts a slice (non-comparable) so use SetAlways.
	warren.SetAlways(w3, "HistoryLinker.Instances", historyLinker.SetInstances, instances)
	warren.Set(w3, "ScrollbackManager", sessionService.SetScrollbackManager, services.ScrollbackSequencer(scrollbackManager))
	warren.Set(w3, "ExternalDiscovery", sessionService.SetExternalDiscovery, externalDiscovery)
	// UnfinishedWorkService is optional — nil when config directory is unavailable.
	// Do not add to Warren Wire; nil is a valid production value documented on RuntimeDeps.
	if err := w3.Validate(); err != nil {
		return nil, err
	}

	// Initialize UnfinishedWork scanner and state store.
	var (
		unfinishedScanner    *unfinished.Scanner
		unfinishedStateStore *unfinished.StateStore
		unfinishedWorkSvc    *services.UnfinishedWorkService
	)
	if configDir, configErr := config.GetConfigDir(); configErr == nil {
		statePath := filepath.Join(configDir, "unfinished_state.json")
		unfinishedStateStore, _ = unfinished.NewStateStore(statePath)
		if unfinishedStateStore != nil {
			unfinishedScanner = unfinished.NewScanner(eventBus, unfinishedStateStore)
			unfinishedWorkSvc = services.NewUnfinishedWorkService(unfinishedScanner, unfinishedStateStore, eventBus, storage)
			log.InfoLog.Printf("UnfinishedWorkService initialized (state: %s)", statePath)
		}
	} else {
		log.WarningLog.Printf("Could not initialize UnfinishedWork state store: %v", configErr)
	}

	// Open the dedicated analytics database (non-fatal: fall back gracefully on failure).
	var analyticsClient *ent.Client
	if configDir, configErr := config.GetConfigDir(); configErr == nil {
		ctx := context.Background()
		if ac, acErr := analytics.OpenAnalyticsDB(ctx, configDir); acErr != nil {
			log.WarningLog.Printf("Could not open analytics DB (will use log-only fallback): %v", acErr)
		} else {
			analyticsClient = ac
			log.InfoLog.Printf("Analytics DB opened: %s/analytics.db", configDir)
		}
	} else {
		log.WarningLog.Printf("Could not determine config dir for analytics DB: %v", configErr)
	}

	return &RuntimeDeps{
		ServiceDeps:             svc,
		Instances:               instances,
		ReactiveQueueMgr:        reactiveQueueMgr,
		ScrollbackManager:       scrollbackManager,
		TmuxStreamerManager:     tmuxStreamerManager,
		ExternalDiscovery:       externalDiscovery,
		ExternalApprovalMonitor: externalApprovalMonitor,
		PRStatusPoller:          svc.PRStatusPoller,
		HistoryLinker:           historyLinker,
		ErrorRegistry:           svc.ErrorRegistry,
		UnfinishedScanner:       unfinishedScanner,
		UnfinishedStateStore:    unfinishedStateStore,
		UnfinishedWorkService:   unfinishedWorkSvc,
		AnalyticsEntClient:      analyticsClient,
	}, nil
}
