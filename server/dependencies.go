package server

import (
	"claude-squad/log"
	"claude-squad/server/events"
	"claude-squad/server/services"
	"claude-squad/session"
	"claude-squad/session/scrollback"
	"fmt"
	"os"
	"path/filepath"
)

// ServerDependencies holds all wired service components for the HTTP server.
// Use BuildDependencies to construct and wire them in the correct order.
// See the initialization order comment on NewServer for dependency constraints.
type ServerDependencies struct {
	SessionService          *services.SessionService
	Storage                 *session.Storage
	EventBus                *events.EventBus
	StatusManager           *session.InstanceStatusManager
	ReviewQueue             *session.ReviewQueue
	ReviewQueuePoller       *session.ReviewQueuePoller
	ReactiveQueueMgr        *ReactiveQueueManager
	ScrollbackManager       *scrollback.ScrollbackManager
	TmuxStreamerManager     *session.ExternalTmuxStreamerManager
	ExternalDiscovery       *session.ExternalSessionDiscovery
	ExternalApprovalMonitor *session.ExternalApprovalMonitor
}

// BuildDependencies constructs and wires all server dependencies in the correct order.
// Returns an error only for unrecoverable failures (SessionService init, Storage start).
// Non-fatal failures (individual instance start) are logged and skipped.
func BuildDependencies() (*ServerDependencies, error) {
	// Step 1: SessionService (creates Storage, EventBus, ReviewQueue internally)
	sessionService, err := services.NewSessionServiceFromConfig()
	if err != nil {
		return nil, fmt.Errorf("initialize SessionService: %w", err)
	}

	storage := sessionService.GetStorage()
	eventBus := sessionService.GetEventBus()
	reviewQueue := sessionService.GetReviewQueueInstance()

	// Steps 2-3: StatusManager and ReviewQueuePoller (before storage starts)
	statusManager := session.NewInstanceStatusManager()
	reviewQueuePoller := session.NewReviewQueuePoller(reviewQueue, statusManager, storage)

	// Step 4: start storage (loads instances from disk)
	if err := storage.Start(); err != nil {
		return nil, fmt.Errorf("start storage: %w", err)
	}
	log.InfoLog.Printf("Storage started — instances loaded and ready")

	// Steps 5-7: load, wire, start instances and controllers
	instances, err := storage.LoadInstances()
	if err != nil {
		return nil, fmt.Errorf("load instances: %w", err)
	}

	// Step 5: wire dependencies to each instance
	for _, inst := range instances {
		inst.SetReviewQueue(reviewQueue)
		inst.SetStatusManager(statusManager)
	}
	reviewQueuePoller.SetInstances(instances)

	// Step 6: start tmux sessions for loaded instances
	for _, inst := range instances {
		if !inst.Started() {
			if err := inst.Start(false); err != nil {
				log.ErrorLog.Printf("Failed to start loaded instance '%s': %v", inst.Title, err)
			} else {
				log.InfoLog.Printf("Started loaded instance '%s'", inst.Title)
			}
		}
	}

	// Persist any auto-detected worktree info (must happen after Step 6)
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
		log.InfoLog.Printf("Instance '%s': Started()=%v, Paused()=%v", inst.Title, started, paused)
		if started && !paused {
			if inst.GetController() == nil {
				if err := inst.StartController(); err != nil {
					log.WarningLog.Printf("Failed to start controller for '%s': %v", inst.Title, err)
				} else {
					log.InfoLog.Printf("Started controller for '%s'", inst.Title)
				}
			} else {
				log.InfoLog.Printf("Instance '%s' already has active controller", inst.Title)
			}
		}
	}

	// Step 8: ReactiveQueueManager
	reactiveQueueMgr := NewReactiveQueueManager(reviewQueue, reviewQueuePoller, eventBus, statusManager, storage)
	sessionService.SetReactiveQueueManager(reactiveQueueMgr)
	log.InfoLog.Printf("ReactiveQueueManager initialized")

	// Step 9: ScrollbackManager (independent of above)
	homeDir, _ := os.UserHomeDir()
	scrollbackPath := filepath.Join(homeDir, ".claude-squad", "sessions")
	scrollbackConfig := scrollback.DefaultScrollbackConfig()
	scrollbackConfig.StoragePath = scrollbackPath
	scrollbackManager := scrollback.NewScrollbackManager(scrollbackConfig)
	log.InfoLog.Printf("Initialized ScrollbackManager: path=%s, compression=%s, maxLines=%d",
		scrollbackPath, scrollbackConfig.CompressionType, scrollbackConfig.MaxLines)

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
		log.InfoLog.Printf("Added external session '%s' to review queue poller", instance.Title)
	})
	externalDiscovery.OnSessionRemoved(func(instance *session.Instance) {
		reviewQueuePoller.RemoveInstance(instance.Title)
		log.InfoLog.Printf("Removed external session '%s' from review queue poller", instance.Title)
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
			item.Status = inst.Status
			item.Tags = inst.Tags
			item.Category = inst.Category
			item.DiffStats = inst.GetDiffStats()
			item.LastActivity = inst.LastMeaningfulOutput
		}

		reviewQueue.Add(item)
		log.InfoLog.Printf("Added external session approval '%s' to review queue (type: %s, confidence: %.2f)",
			event.SessionTitle, event.Request.Type, event.Request.Confidence)
	})

	return &ServerDependencies{
		SessionService:          sessionService,
		Storage:                 storage,
		EventBus:                eventBus,
		StatusManager:           statusManager,
		ReviewQueue:             reviewQueue,
		ReviewQueuePoller:       reviewQueuePoller,
		ReactiveQueueMgr:        reactiveQueueMgr,
		ScrollbackManager:       scrollbackManager,
		TmuxStreamerManager:     tmuxStreamerManager,
		ExternalDiscovery:       externalDiscovery,
		ExternalApprovalMonitor: externalApprovalMonitor,
	}, nil
}
