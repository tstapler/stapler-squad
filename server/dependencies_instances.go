package server

import (
	"context"
	"time"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/log"
	warren "github.com/tstapler/stapler-squad/pkg/warren"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/server/reviewqueue"
	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/backlog"
	"github.com/tstapler/stapler-squad/session/cdp"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/importer"
	"github.com/tstapler/stapler-squad/session/instance"
	queuepkg "github.com/tstapler/stapler-squad/session/queue"
	"github.com/tstapler/stapler-squad/session/scrollback"
	"github.com/tstapler/stapler-squad/session/summaries"
)

// wireLoadedInstances attaches the shared collaborators to every loaded instance and
// registers the instance list with the pollers.
func wireLoadedInstances(svc *ServiceDeps, instances []*instance.Instance, backlogLifecycleListener *backlog.BacklogLifecycleListener,
	sessionSummaryGenerator *summaries.SessionSummaryGenerator, sessionTagPoller *session.SessionTagClassificationPoller) error {
	storage, reviewQueue, statusManager, reviewQueuePoller, eventBus := svc.Storage, svc.ReviewQueue, svc.StatusManager, svc.ReviewQueuePoller, svc.EventBus
	// Step 5 (continued): wire dependencies to each instance
	// inst.SetReviewQueue and inst.SetStatusManager are called per-instance in a loop;
	// Warren is designed for named scalar setters, not loop iterations. Left unwrapped.
	for _, inst := range instances {
		inst.SetReviewQueue(reviewQueue)
		inst.SetNotifier(&services.EventBusNotifier{Bus: eventBus})
		inst.SetStatusManager(statusManager)
		backlogLifecycleListener.WireToInstance(inst)
		if sessionSummaryGenerator != nil {
			summaries.WireSessionSummaryListener(sessionSummaryGenerator, inst)
		}
	}

	// Restore dirBaseSHA for directory-mode backlog sessions that were persisted
	// before this process started. One batch query replaces N individual lookups.
	{
		var dirUUIDs []string
		dirInstMap := make(map[string]*instance.Instance)
		for _, inst := range instances {
			if !inst.IsWorktree && inst.HasTag(instance.TagBacklogWork) {
				dirUUIDs = append(dirUUIDs, inst.UUID)
				dirInstMap[inst.UUID] = inst
			}
		}
		if len(dirUUIDs) > 0 {
			baseSHAs, _ := storage.GetBaseCommitSHAsForSessions(context.Background(), dirUUIDs)
			for uuid, sha := range baseSHAs {
				if inst, ok := dirInstMap[uuid]; ok {
					inst.SetDirBaseSHA(sha)
				}
			}
		}
	}

	// Wire instances to pollers.
	// SetInstances accepts a slice (non-comparable) so use SetAlways (skips nil check).
	w2 := warren.NewWire("RuntimeDeps.Pollers")
	warren.SetAlways(w2, "ReviewQueuePoller.Instances", reviewQueuePoller.SetInstances, instances)
	warren.SetAlways(w2, "PRStatusPoller.Instances", svc.PRStatusPoller.SetInstances, instances)
	warren.SetAlways(w2, "PRStatusPoller.OnUpdated", svc.PRStatusPoller.SetOnUpdated, func(inst *instance.Instance) {
		eventBus.Publish(events.NewSessionUpdatedEvent(inst, []string{"github_pr_priority", "github_pr_state", "github_check_conclusion"}))
	})
	if sessionTagPoller != nil {
		warren.SetAlways(w2, "SessionTagPoller.Instances", sessionTagPoller.SetInstances, instances)
	}
	if err := w2.Validate(); err != nil {
		return err
	}
	return nil
}

// startInstanceBootInit starts the heavy startup work (tmux starts, hot-restore
// reconciliation, controllers, startup scan) on a background goroutine so the HTTP server
// can bind immediately. Step order inside is load-bearing; see the step comments.
// The returned stop cancels the remaining steps and waits for the goroutine to exit, so
// an owner that tears down its state dir cannot be written into afterwards.
// gate, nil in production, runs first inside the goroutine so a test can hold it at a known point.
func startInstanceBootInit(svc *ServiceDeps, instances []*instance.Instance, gate func()) (stop func()) {
	storage, reviewQueue, statusManager, eventBus := svc.Storage, svc.ReviewQueue, svc.StatusManager, svc.EventBus
	// Perform heavy initialization (tmux starting, controllers, scanning) in the background
	// so the HTTP server can bind and start immediately.
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer func() {
			if r := recover(); r != nil {
				log.ErrorLog().Printf("[startup] panic in background init goroutine: %v", r)
			}
		}()
		if gate != nil {
			gate()
		}
		bootStartInstances(instances, eventBus)

		bootHotRestoreLive(instances, eventBus)
		if ctx.Err() != nil {
			return
		}

		// Step 6c: Reconcile custom shells — rebuild in-memory state from ent for
		// shells that were running when the server last shut down. Must run after
		// Step 6/6b so tmux sessions are attached before shell liveness probes fire.
		for _, inst := range instances {
			inst.ReconcileShells(context.Background())
		}

		// Step 6d: Kill orphaned tmux sessions — staplersquad_ sessions with no
		// matching DB record. These accumulate when DeleteSession removes the DB
		// row but the server restarted before the live in-memory instance was
		// available to call Destroy(). Must run after 6/6b so re-adopted sessions
		// are already registered and won't be mistaken for orphans.
		//
		// SKIP for any isolated-instance process: ReconcileOrphanedTmuxSessions calls plain
		// `tmux list-sessions` with no socket isolation -- it always targets the shared
		// default tmux socket, regardless of this process's own (isolated) DB/config
		// directory. This process's `instances` list only ever contains its own handful of
		// sessions, so every real session on the machine's shared tmux server -- including
		// production sessions from an entirely separate stapler-squad process -- looks like
		// an orphan and gets killed. This was the root cause of production sessions dying in
		// tight clusters whenever any integration test called BuildDependencies() on the same
		// machine, the E2E test harness's real-binary invocation (STAPLER_SQUAD_INSTANCE=
		// e2e-local, not caught by IsTestMode()), and the demo-server harness's --test-mode/
		// STAPLER_SQUAD_TEST_DIR invocation (also not caught by either IsTestMode() or
		// IsNamedInstance() alone). See config.IsIsolatedInstance's doc comment.
		if !config.IsIsolatedInstance() {
			// minAge=0: safe only here, before any new session creation is possible
			// in this startup sequence — see ReconcileOrphanedTmuxSessions' doc comment.
			session.ReconcileOrphanedTmuxSessions(instances, 0)
		}

		// Step 6.5: Persist any auto-detected worktree info (must happen after Step 6)
		if len(instances) > 0 {
			if err := storage.SaveInstances(instances); err != nil {
				log.Warn("failed to persist migrated instance data", "err", err)
			} else {
				log.Info("persisted migrated instance data", "count", len(instances))
			}
		}

		// Step 6.6: Reconcile CDP orphan wrapper directories.
		// Collect active session IDs, then remove any cdp-bins subdirectory that
		// does not belong to a known session (left behind by crashed or deleted sessions).
		// Both the real manager and the noop manager implement ReconcileOrphans with
		// filesystem-only cleanup (no Chrome binary required).
		activeSessionIDs := make([]string, 0, len(instances))
		for _, inst := range instances {
			activeSessionIDs = append(activeSessionIDs, inst.GetStableID())
		}
		cdpCleanupMgr := cdp.New(cdp.CDPConfig{}) // noop when Chrome is absent; still cleans up dirs
		if err := cdpCleanupMgr.ReconcileOrphans(activeSessionIDs); err != nil {
			log.Warn("cdp: orphan cleanup failed (non-fatal)", "err", err)
		}

		if ctx.Err() != nil {
			return
		}
		bootStartControllers(instances)

		if ctx.Err() != nil {
			return
		}
		bootResumeDrivers(instances)

		// Step 7.6: Startup scan and orphaned approval sync
		// Brief settling delay to allow controllers to initialize their terminal readers.
		select {
		case <-time.After(500 * time.Millisecond):
		case <-ctx.Done():
			return
		}
		contentProvider := session.NewPollerContentProvider()
		scanner := session.NewStartupScanner(statusManager, contentProvider)
		scanner.SetOptionsSource(svc.AttentionOptions)
		scanner.Scan(instances, reviewQueue)
		syncOrphanedApprovalsToQueue(svc.ApprovalStore, instances, reviewQueue)
	}()
	return func() {
		cancel()
		<-done
	}
}

// bootStartInstances is boot Step 6.
func bootStartInstances(instances []*instance.Instance, eventBus *events.EventBus) {
	// Step 6: start tmux sessions for loaded instances (non-fatal failures).
	// Stagger starts by 200ms each to avoid a fork burst that saturates the
	// cgroup pids.max limit when many sessions restore simultaneously.
	for i, inst := range instances {
		if !shouldBootStart(inst) {
			continue
		}
		if i > 0 {
			time.Sleep(200 * time.Millisecond)
			// A client's lazy restore (or an archive) may have landed during the stagger.
			if !shouldBootStart(inst) {
				continue
			}
		}
		if err := inst.Start(false); err != nil {
			log.Error("failed to start loaded instance", "session", inst.Title, "err", err)
		} else {
			log.Info("started loaded instance", "session", inst.Title)
			// Lets waitForInstanceStartedEvent (connectrpc_websocket.go) unblock a
			// connection that raced this loop's stagger, instead of only polling.
			eventBus.Publish(events.NewSessionUpdatedEvent(inst, []string{"status"}))
		}
	}
}

// bootHotRestoreLive is boot Step 6b.
func bootHotRestoreLive(instances []*instance.Instance, eventBus *events.EventBus) {
	// Step 6b: reconcile Stopped/PermanentlyFailed/Failed sessions that have a
	// live tmux session. This handles the case where the server crashed or
	// restarted while a session was running — the DB recorded a terminal
	// status but the tmux session survived. RecoverFromStopped resets the
	// status to Creating (bypassing the terminal-state guard) so Start(false)
	// can hot-attach to the existing tmux session. Without this, a session
	// that reached PermanentlyFailed/Failed before the restart is stuck
	// forever: Step 6 above skips it (already !inst.Started() but Start(false)
	// rejects the PermanentlyFailed/Failed -> Active transition), so every
	// capture/resync against it fails indefinitely even though the
	// underlying tmux session is fully functional. IsHotRestoreRecoverable is
	// the single source of truth for this status set — see its doc comment.
	// Archived sessions are skipped before TmuxSessionExists(): adopting one
	// flips it back off its terminal status, so it would resurrect on every
	// boot (see ADR-001, superseded-rework-session-retirement).
	for _, inst := range instances {
		if inst == nil || inst.IsArchived() {
			continue
		}
		title, status := inst.Title, inst.GetLifecycleStatus()
		outcome, err := instance.HotRestoreIfLive(inst, inst.IsLiveOrStoredSessionLive)
		switch {
		case err != nil:
			log.Warn("Reconcile: hot-restore failed", "session", title, "err", err)
		case outcome == instance.HotRestoreCleared:
			log.Info("Reconcile: restored session (was terminal, now Running)", "session", title, "was", status)
			eventBus.Publish(events.NewSessionUpdatedEvent(inst, []string{"status"}))
		}
	}
}

// bootStartControllers is boot Step 7.
func bootStartControllers(instances []*instance.Instance) {
	// Step 7: start controllers (requires started instances + StatusManager)
	log.Info("attempting controller startup", "instances", len(instances))
	for _, inst := range instances {
		started := inst.Started()
		paused := inst.Paused()
		if started && !paused && inst.GetLifecycleStatus() != domain.Stopped {
			if inst.GetController() == nil {
				if err := inst.StartController(); err != nil {
					log.Warn("failed to start controller", "session", inst.Title, "err", err)
				} else {
					log.Info("started controller", "session", inst.Title)
				}
			}
		}
	}
}

// bootResumeDrivers is boot Step 7.5.
func bootResumeDrivers(instances []*instance.Instance) {
	// Step 7.5: Resume session drivers for workflow sessions with an undelivered InitialPrompt.
	// After a service restart, drivers are not automatically restarted for loaded sessions.
	// Sessions created by the workflow scheduler that never had their prompt injected
	// (e.g., service restarted within 30 s of session creation) need the driver resumed.
	// This restarts the driver for every session with a non-empty InitialPrompt, not just
	// ones from the last 30s — safe because the driver itself checks Instance.InitialPromptSentAt
	// (persisted; set the moment a send actually happens) before falling back to the
	// output/JSONL heuristics, and skips re-sending if it's already set.
	for _, inst := range instances {
		if inst.InitialPrompt == "" {
			continue
		}
		status := inst.GetLifecycleStatus()
		if status == domain.Paused || status == domain.Stopped || status == domain.Hibernated {
			continue
		}
		session.StartSessionDriver(inst, inst.GetEffectiveRootDir())
	}
}

// buildReactiveQueue builds the reactive queue manager with its Slack notifier and the
// shared callback dispatcher.
func buildReactiveQueue(svc *ServiceDeps, cfg *config.Config) (*reviewqueue.ReactiveQueueManager, *services.SlackNotifier) {
	storage, reviewQueue, statusManager, reviewQueuePoller, eventBus, sessionService := svc.Storage, svc.ReviewQueue, svc.StatusManager, svc.ReviewQueuePoller, svc.EventBus, svc.SessionService
	// Step 8: ReactiveQueueManager
	reactiveQueueMgr := reviewqueue.NewReactiveQueueManager(reviewQueue, reviewQueuePoller, eventBus, statusManager, storage)
	// Wires the opt-in AutoCreatePR policy — sessionService is available this early
	// (constructed in BuildCoreDepsWithOptions, aliased above), so no setter-injection
	// race window like SetHeadlessPool's had.
	reactiveQueueMgr.SetOneShotRunner(sessionService)
	log.Info("ReactiveQueueManager initialized")

	// SlackNotifier (Epic 1.3, Story 1.3.1): a single shared instance, always
	// constructed and wired — self-gating per-call when unconfigured (see
	// slack_notifier.go's Notify*/MaybeNotify* no-op-when-unconfigured design),
	// so no conditional construction is needed here.
	slackNotifier := services.NewSlackNotifier()
	reactiveQueueMgr.SetSlackNotifier(slackNotifier)

	// CallbackDispatcher (webhook-triggers Phase 5, FR7-FR9): a single shared
	// instance fires on_session_complete/on_session_stale/on_queue_item_created.
	// Reads cfg.Callbacks and the webhook_triggers feature flag live on every
	// Dispatch call, so a CallbackConfigService.UpdateCallbackConfig save takes
	// effect immediately without a restart — same live-cfg-pointer shape as
	// SetSharedBacklogConfig below. Wired to both the EntRepository (via
	// Storage's forwarding setter, for on_session_complete/on_session_stale) and
	// ReactiveQueueManager (for on_queue_item_created).
	callbackDispatcher := services.NewCallbackDispatcher(cfg)
	storage.SetCallbackDispatcher(callbackDispatcher)
	reactiveQueueMgr.SetCallbackDispatcher(callbackDispatcher)
	// Share callbackDispatcher's live *config.Config instance (and its guarding
	// mutex) with CallbackConfigService so a Settings save of a callback URL
	// takes effect on the very next Dispatch call instead of requiring a
	// process restart (sdd:6-verify finding, mirrors PR #199 review F1's
	// SetSharedBacklogConfig pattern) — see
	// CallbackConfigService.SetSharedCallbackConfig's doc comment.
	sessionService.SetSharedCallbackConfig(cfg, callbackDispatcher.ConfigMu())
	log.Info("CallbackDispatcher initialized")

	return reactiveQueueMgr, slackNotifier
}

// wireExternalSessions builds external-session discovery and the external approval
// monitor, bridging both into the review queue, pollers and persistence.
func wireExternalSessions(svc *ServiceDeps, historyLinker *importer.HistoryLinker, sessionTagPoller *session.SessionTagClassificationPoller,
	backlogLifecycleListener *backlog.BacklogLifecycleListener, sessionSummaryGenerator *summaries.SessionSummaryGenerator) (*session.ExternalSessionDiscovery, *session.ExternalApprovalMonitor) {
	storage, reviewQueue, statusManager, reviewQueuePoller, eventBus, sessionService := svc.Storage, svc.ReviewQueue, svc.StatusManager, svc.ReviewQueuePoller, svc.EventBus, svc.SessionService
	// Step 11: ExternalDiscovery with session-added/removed callbacks
	externalDiscovery := session.NewExternalSessionDiscovery()
	externalDiscovery.OnSessionAdded(func(instance *instance.Instance) {
		if gate := sessionService.DeliveryGate(); gate != nil {
			gate.UpsertInstance(instance)
		}
		if err := storage.AddInstance(instance); err != nil {
			log.Error("failed to persist external session", "session", instance.Title, "err", err)
		} else {
			log.Info("persisted external session to storage", "session", instance.Title)
		}
		// Wire dependencies so the external session appears in the review queue
		instance.SetReviewQueue(reviewQueue)
		instance.SetNotifier(&services.EventBusNotifier{Bus: eventBus})
		instance.SetStatusManager(statusManager)
		reviewQueuePoller.AddInstance(instance)
		svc.PRStatusPoller.AddInstance(instance)
		historyLinker.AddInstance(instance)
		if sessionTagPoller != nil {
			sessionTagPoller.AddInstance(instance)
		}
		backlogLifecycleListener.WireToInstance(instance)
		if sessionSummaryGenerator != nil {
			summaries.WireSessionSummaryListener(sessionSummaryGenerator, instance)
		}
		log.Info("added external session to review queue poller, PR status poller, and history linker", "session", instance.Title)
	})
	externalDiscovery.OnSessionRemoved(func(instance *instance.Instance) {
		if gate := sessionService.DeliveryGate(); gate != nil {
			gate.RemoveSession(instance.Title)
		}
		reviewQueuePoller.RemoveInstance(instance.Title)
		svc.PRStatusPoller.RemoveInstance(instance.Title)
		historyLinker.RemoveInstance(instance.Title)
		if sessionTagPoller != nil {
			sessionTagPoller.RemoveInstance(instance.Title)
		}
		log.Info("removed external session from review queue poller, PR status poller, and history linker", "session", instance.Title)
		reviewQueue.Remove(instance.Title)
		if err := storage.DeleteInstance(instance.Title); err != nil {
			log.Warn("failed to remove external session from storage", "session", instance.Title, "err", err)
		} else {
			log.Info("removed external session from storage", "session", instance.Title)
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

		item := &queuepkg.ReviewItem{
			SessionID:   event.SessionTitle,
			SessionName: event.SessionTitle,
			Reason:      queuepkg.ReasonApprovalPending,
			Priority:    queuepkg.PriorityHigh,
			DetectedAt:  event.Request.Timestamp,
			Context:     context,
		}
		if inst != nil {
			snap := inst.Snapshot()
			item.Program = inst.Program
			item.Branch = inst.Branch
			item.Path = snap.Path
			item.WorkingDir = snap.WorkingDir
			item.Status = inst.GetLifecycleStatus().String()
			item.Tags = inst.Tags
			item.Category = inst.Category
			item.DiffStats = inst.GetDiffStats()
			item.LastActivity = inst.LastMeaningfulOutput
		}

		reviewQueue.Add(item)
		log.Info("added external session approval to review queue", "session", event.SessionTitle, "type", event.Request.Type, "confidence", event.Request.Confidence)
	})

	return externalDiscovery, externalApprovalMonitor
}

// wireSessionServiceRuntime hands the runtime collaborators to SessionService for
// unified session listing.
func wireSessionServiceRuntime(sessionService *services.SessionService, reactiveQueueMgr *reviewqueue.ReactiveQueueManager,
	historyLinker *importer.HistoryLinker, instances []*instance.Instance, scrollbackManager *scrollback.ScrollbackManager,
	externalDiscovery *session.ExternalSessionDiscovery) error {
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
		return err
	}
	return nil
}
