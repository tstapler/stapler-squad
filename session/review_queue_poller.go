package session

import (
	"claude-squad/log"
	"context"
	"fmt"
	"sync"
	"time"
)

// ReviewQueuePollerConfig contains configuration for the review queue poller.
type ReviewQueuePollerConfig struct {
	PollInterval       time.Duration // How often to check sessions
	IdleThreshold      time.Duration // Duration before considering session idle and adding to queue
	InputWaitDuration  time.Duration // Time waiting for input before flagging
	StalenessThreshold time.Duration // Duration since last meaningful output before considering stale
}

// DefaultReviewQueuePollerConfig returns sensible defaults for polling.
func DefaultReviewQueuePollerConfig() ReviewQueuePollerConfig {
	return ReviewQueuePollerConfig{
		PollInterval:       2 * time.Second, // Poll every 2 seconds for immediate detection
		IdleThreshold:      5 * time.Second, // Add to queue after 5s idle for immediate user notifications
		InputWaitDuration:  3 * time.Second,  // Flag if waiting for input > 3s (reduced from 5s)
		StalenessThreshold: 2 * time.Minute,  // Flag if no meaningful output for 2 minutes (reduced from 5min)
	}
}

// ReviewQueuePoller automatically monitors sessions and adds them to the review queue
// when they become idle or need attention.
type ReviewQueuePoller struct {
	queue         *ReviewQueue
	statusManager *InstanceStatusManager
	storage       *Storage
	instances     []*Instance
	config        ReviewQueuePollerConfig

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	mu     sync.RWMutex
}

// NewReviewQueuePoller creates a new poller for automatically managing the review queue.
// The storage parameter is optional (can be nil) but required for persisting LastAddedToQueue timestamps.
func NewReviewQueuePoller(queue *ReviewQueue, statusManager *InstanceStatusManager, storage *Storage) *ReviewQueuePoller {
	return NewReviewQueuePollerWithConfig(queue, statusManager, storage, DefaultReviewQueuePollerConfig())
}

// NewReviewQueuePollerWithConfig creates a poller with custom configuration.
// The storage parameter is optional (can be nil) but required for persisting LastAddedToQueue timestamps.
func NewReviewQueuePollerWithConfig(queue *ReviewQueue, statusManager *InstanceStatusManager, storage *Storage, config ReviewQueuePollerConfig) *ReviewQueuePoller {
	return &ReviewQueuePoller{
		queue:         queue,
		statusManager: statusManager,
		storage:       storage,
		instances:     make([]*Instance, 0),
		config:        config,
	}
}

// SetInstances sets the list of instances to monitor.
func (rqp *ReviewQueuePoller) SetInstances(instances []*Instance) {
	rqp.mu.Lock()
	defer rqp.mu.Unlock()
	rqp.instances = instances
}

// AddInstance adds a single instance to monitor.
func (rqp *ReviewQueuePoller) AddInstance(instance *Instance) {
	rqp.mu.Lock()
	defer rqp.mu.Unlock()
	rqp.instances = append(rqp.instances, instance)
}

// RemoveInstance removes an instance from monitoring.
func (rqp *ReviewQueuePoller) RemoveInstance(instanceTitle string) {
	rqp.mu.Lock()
	defer rqp.mu.Unlock()

	filtered := make([]*Instance, 0, len(rqp.instances))
	for _, inst := range rqp.instances {
		if inst.Title != instanceTitle {
			filtered = append(filtered, inst)
		}
	}
	rqp.instances = filtered
}

// Start begins polling for idle sessions.
func (rqp *ReviewQueuePoller) Start(ctx context.Context) {
	rqp.mu.Lock()
	if rqp.ctx != nil {
		rqp.mu.Unlock()
		log.InfoLog.Printf("ReviewQueuePoller already started")
		return
	}

	rqp.ctx, rqp.cancel = context.WithCancel(ctx)
	rqp.mu.Unlock()

	// STARTUP CLEANUP: Remove orphaned queue items with invalid timestamps
	// This handles items that were persisted before the LastMeaningfulOutput migration
	rqp.cleanupOrphanedItems()

	// Perform initial queue population immediately on startup
	// This ensures the queue is populated without waiting for the first poll interval
	rqp.checkSessions()

	rqp.wg.Add(1)
	go rqp.pollLoop()

	log.InfoLog.Printf("ReviewQueuePoller started (poll interval: %s)", rqp.config.PollInterval)
}

// Stop stops the poller.
func (rqp *ReviewQueuePoller) Stop() {
	rqp.mu.Lock()
	if rqp.cancel != nil {
		rqp.cancel()
	}
	rqp.mu.Unlock()

	rqp.wg.Wait()
	log.InfoLog.Printf("ReviewQueuePoller stopped")
}

// cleanupOrphanedItems removes queue items with zero or invalid LastActivity timestamps.
// This handles orphaned items that were persisted before the LastMeaningfulOutput migration
// and never got cleaned up. Should be called once during startup.
func (rqp *ReviewQueuePoller) cleanupOrphanedItems() {
	// Get all items currently in queue
	allItems := rqp.queue.List()

	// Timestamp validation threshold - any timestamp before this is considered invalid
	minValidTime := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)

	removedCount := 0
	for _, item := range allItems {
		// Remove items with zero or invalid LastActivity timestamps
		if item.LastActivity.IsZero() || item.LastActivity.Before(minValidTime) {
			log.InfoLog.Printf("[ReviewQueue] STARTUP CLEANUP: Removing orphaned item '%s' with invalid LastActivity (%v)",
				item.SessionID, item.LastActivity)
			rqp.queue.Remove(item.SessionID)
			removedCount++
		}
	}

	if removedCount > 0 {
		log.InfoLog.Printf("[ReviewQueue] STARTUP CLEANUP: Removed %d orphaned items with invalid timestamps", removedCount)
	} else {
		log.InfoLog.Printf("[ReviewQueue] STARTUP CLEANUP: No orphaned items found")
	}
}

// pollLoop is the main polling loop that runs in the background.
func (rqp *ReviewQueuePoller) pollLoop() {
	defer rqp.wg.Done()

	ticker := time.NewTicker(rqp.config.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-rqp.ctx.Done():
			return
		case <-ticker.C:
			rqp.checkSessions()
		}
	}
}

// checkSessions checks all instances and updates the review queue.
func (rqp *ReviewQueuePoller) checkSessions() {
	rqp.mu.RLock()
	instances := make([]*Instance, len(rqp.instances))
	copy(instances, rqp.instances)
	rqp.mu.RUnlock()

	for _, inst := range instances {
		rqp.checkSession(inst)
	}
}

// checkSession checks a single session and adds/removes from queue as needed.
func (rqp *ReviewQueuePoller) checkSession(inst *Instance) {
	log.InfoLog.Printf("[ReviewQueue] === CHECKING SESSION '%s' === (started=%v, paused=%v)",
		inst.Title, inst.Started(), inst.Paused())

	// MIGRATION CLEANUP: Remove any existing queue item with zero LastMeaningfulOutput.
	// This handles stale items that were added before the migration ran and never got cleaned up
	// because they didn't meet the criteria for re-adding (e.g., in grace period).
	if inst.LastMeaningfulOutput.IsZero() {
		if rqp.queue.Has(inst.Title) {
			log.InfoLog.Printf("[ReviewQueue] Session '%s': CLEANUP - Removing stale queue item with zero LastMeaningfulOutput", inst.Title)
			rqp.queue.Remove(inst.Title)
		}
		// Don't process this session further if it hasn't been migrated yet
		return
	}

	// Skip paused or unstarted sessions
	if !inst.Started() || inst.Paused() {
		log.InfoLog.Printf("[ReviewQueue] Session '%s': Skipping (started=%v, paused=%v)",
			inst.Title, inst.Started(), inst.Paused())
		rqp.queue.Remove(inst.Title)
		return
	}

	// Get comprehensive status
	statusInfo := rqp.statusManager.GetStatus(inst)

	// Determine if needs attention and why
	var reason AttentionReason
	var priority Priority
	var shouldAdd bool
	var context string

	// Check for controller-based states if controller is active
	if statusInfo.IsControllerActive {
		controller, exists := rqp.statusManager.GetController(inst.Title)
		if exists && controller != nil {
			log.DebugLog.Printf("[ReviewQueue] Session '%s': Checking idle state (controller active)", inst.Title)

			// Get idle state from controller
			idleState, lastActivity := controller.GetIdleState()
			log.DebugLog.Printf("[ReviewQueue] Session '%s': Detected idle state=%s, lastActivity=%s",
				inst.Title, idleState.String(), formatDuration(time.Since(lastActivity)))

			switch idleState {
			case IdleStateActive:
				// Actively working, remove from queue
				log.DebugLog.Printf("[ReviewQueue] Session '%s': Active state - removing from queue", inst.Title)
				rqp.queue.Remove(inst.Title)
				return

			case IdleStateWaiting:
				// Normal idle state (e.g., INSERT mode) - don't add by default
				log.DebugLog.Printf("[ReviewQueue] Session '%s': Waiting state - will check for specific issues", inst.Title)
				shouldAdd = false

			case IdleStateTimeout:
				// Definite timeout - been idle too long
				reason = ReasonIdleTimeout
				priority = PriorityLow
				shouldAdd = true
				idleDuration := time.Since(lastActivity)
				context = fmt.Sprintf("Timed out after %s of inactivity", formatDuration(idleDuration))
				log.DebugLog.Printf("[ReviewQueue] Session '%s': Timeout detected - idle for %s", inst.Title, formatDuration(idleDuration))
			}

			// Check for approval needs (higher priority than idle)
			if statusInfo.ClaudeStatus == StatusNeedsApproval || statusInfo.PendingApprovals > 0 {
				reason = ReasonApprovalPending
				priority = PriorityHigh
				shouldAdd = true
				// Use the detailed context from status detector if available
				if statusInfo.StatusContext != "" {
					context = statusInfo.StatusContext
				} else {
					context = "Waiting for approval to proceed"
				}
				log.DebugLog.Printf("[ReviewQueue] Session '%s': Approval needed (status=%s, pendingApprovals=%d) - %s",
					inst.Title, statusInfo.ClaudeStatus.String(), statusInfo.PendingApprovals, context)
			}

			// Check for input required (explicit prompts asking for user input)
			if statusInfo.ClaudeStatus == StatusInputRequired {
				reason = ReasonInputRequired
				priority = PriorityMedium
				shouldAdd = true
				// Use the detailed context from status detector if available
				if statusInfo.StatusContext != "" {
					context = statusInfo.StatusContext
				} else {
					context = "Waiting for explicit user input"
				}
				log.DebugLog.Printf("[ReviewQueue] Session '%s': Input required - %s", inst.Title, context)
			}

			// Check for errors (highest priority)
			if statusInfo.ClaudeStatus == StatusError {
				reason = ReasonErrorState
				priority = PriorityUrgent
				shouldAdd = true
				// Use the detailed context from status detector if available
				if statusInfo.StatusContext != "" {
					context = statusInfo.StatusContext
				} else {
					context = "Error state detected"
				}
				log.DebugLog.Printf("[ReviewQueue] Session '%s': Error detected - %s", inst.Title, context)
			}

			// Check for tests failing (high priority - actionable failures)
			if statusInfo.ClaudeStatus == StatusTestsFailing {
				reason = ReasonTestsFailing
				priority = PriorityHigh
				shouldAdd = true
				// Use the detailed context from status detector if available
				if statusInfo.StatusContext != "" {
					context = statusInfo.StatusContext
				} else {
					context = "Tests are failing"
				}
				log.InfoLog.Printf("[ReviewQueue] Session '%s': Tests failing - %s", inst.Title, context)
			}

			// Check for task completion (high priority - user wants to know when work is done)
			if statusInfo.ClaudeStatus == StatusSuccess {
				reason = ReasonTaskComplete
				priority = PriorityLow // Low priority since it's informational, not blocking
				shouldAdd = true
				// Use the detailed context from status detector if available
				if statusInfo.StatusContext != "" {
					context = statusInfo.StatusContext
				} else {
					context = "Task completed successfully"
				}
				log.InfoLog.Printf("[ReviewQueue] Session '%s': Task completion - %s", inst.Title, context)
			}

			// Check for uncommitted changes (informational - user may want to review and commit)
			// Only check if we don't already have a higher-priority reason
			if (!shouldAdd || priority == PriorityLow) && inst.HasGitWorktree() {
				worktree, err := inst.GetGitWorktree()
				if err != nil {
					log.DebugLog.Printf("[ReviewQueue] Session '%s': Failed to get git worktree: %v", inst.Title, err)
				} else if worktree != nil {
					isDirty, err := worktree.IsDirty()
					if err != nil {
						log.DebugLog.Printf("[ReviewQueue] Session '%s': Failed to check git status: %v", inst.Title, err)
					} else if isDirty {
						// Only override if we don't have a higher priority reason already
						if !shouldAdd || priority == PriorityLow {
							reason = ReasonUncommittedChanges
							priority = PriorityLow
							shouldAdd = true
							context = "Uncommitted changes ready to commit"
							log.InfoLog.Printf("[ReviewQueue] Session '%s': Uncommitted changes detected", inst.Title)
						}
					}
				}
			}
		}
	} else {
		// No controller - use basic time-based detection
		log.DebugLog.Printf("[ReviewQueue] Session '%s': No active controller - using basic time-based checks", inst.Title)

		// Check if session has been idle for a long time based on UpdatedAt
		idleDuration := time.Since(inst.UpdatedAt)
		const basicIdleThreshold = 5 * time.Second

		if idleDuration > basicIdleThreshold {
			reason = ReasonIdleTimeout
			priority = PriorityLow
			shouldAdd = true
			context = fmt.Sprintf("No controller activity for %s", formatDuration(idleDuration))
			log.DebugLog.Printf("[ReviewQueue] Session '%s': Basic idle check - %s since UpdatedAt",
				inst.Title, formatDuration(idleDuration))
		}

		// Check for uncommitted changes (informational - user may want to review and commit)
		// Only check if we don't already have a higher-priority reason
		if (!shouldAdd || priority == PriorityLow) && inst.HasGitWorktree() {
			worktree, err := inst.GetGitWorktree()
			if err != nil {
				log.DebugLog.Printf("[ReviewQueue] Session '%s': Failed to get git worktree: %v", inst.Title, err)
			} else if worktree != nil {
				isDirty, err := worktree.IsDirty()
				if err != nil {
					log.DebugLog.Printf("[ReviewQueue] Session '%s': Failed to check git status: %v", inst.Title, err)
				} else if isDirty {
					// Only override if we don't have a higher priority reason already
					if !shouldAdd || priority == PriorityLow {
						reason = ReasonUncommittedChanges
						priority = PriorityLow
						shouldAdd = true
						context = "Uncommitted changes ready to commit"
						log.InfoLog.Printf("[ReviewQueue] Session '%s': Uncommitted changes detected", inst.Title)
					}
				}
			}
		}
	}

	// NOTE: Preview() is now a read-only operation that does NOT update timestamps.
	// Timestamps are managed by:
	// 1. WebSocket streaming when users view the terminal in the web UI
	// 2. User interactions (typing, viewing) via UpdateTerminalTimestamps(forceUpdate=true)
	// 3. Automated checks in HasUpdated() which call UpdateTerminalTimestamps(forceUpdate=false)
	//
	// We deliberately avoid calling Preview() here because it would be an expensive operation
	// (blocking tmux capture) that doesn't provide value since it no longer updates timestamps.
	// Instead, we rely on the timestamps already set by the above mechanisms.
	//
	// This approach:
	// - Prevents breaking acknowledgment snooze (Preview() no longer updates LastMeaningfulOutput)
	// - Avoids expensive blocking tmux calls during polling
	// - Relies on WebSocket streaming or HasUpdated() for accurate timestamp management

	// Check for terminal staleness (no meaningful output for configured threshold)
	// This helps identify sessions that might be stuck or waiting without showing obvious idle state
	// IMPORTANT: Respect acknowledgment - don't flag as stale if user already acknowledged
	timeSinceOutput := inst.GetTimeSinceLastMeaningfulOutput()
	log.InfoLog.Printf("[ReviewQueue] Session '%s': Staleness check - %s since last meaningful output (threshold: %s, shouldAdd=%v, priority=%v)",
		inst.Title, formatDuration(timeSinceOutput), formatDuration(rqp.config.StalenessThreshold), shouldAdd, priority)

	// Check if user has acknowledged this session after it became stale
	// If acknowledged after last output, don't re-flag as stale
	alreadyAcknowledged := !inst.LastAcknowledged.IsZero() && inst.LastAcknowledged.After(inst.LastMeaningfulOutput)

	if timeSinceOutput > rqp.config.StalenessThreshold {
		if alreadyAcknowledged {
			log.InfoLog.Printf("[ReviewQueue] Session '%s': STALE but already acknowledged - skipping staleness flag",
				inst.Title)
		} else {
			log.InfoLog.Printf("[ReviewQueue] Session '%s': STALENESS DETECTED - time since output (%s) > threshold (%s)",
				inst.Title, formatDuration(timeSinceOutput), formatDuration(rqp.config.StalenessThreshold))

			// Only override if we don't already have a higher-priority reason
			if !shouldAdd || priority < PriorityMedium {
				reason = ReasonIdleTimeout // Reuse idle timeout reason for staleness
				priority = PriorityLow     // Lower priority than approval/error, but should be reviewed
				shouldAdd = true
				context = fmt.Sprintf("No meaningful output for %s (may be stuck or waiting)",
					formatDuration(timeSinceOutput))

				log.InfoLog.Printf("[ReviewQueue] Session '%s': SETTING shouldAdd=true - flagged as stale - %s since last meaningful output",
					inst.Title, formatDuration(timeSinceOutput))
			} else {
				log.InfoLog.Printf("[ReviewQueue] Session '%s': Stale but already has higher priority reason (%s)",
					inst.Title, reason.String())
			}
		}
	} else {
		log.InfoLog.Printf("[ReviewQueue] Session '%s': NOT STALE - time since output (%s) <= threshold (%s)",
			inst.Title, formatDuration(timeSinceOutput), formatDuration(rqp.config.StalenessThreshold))
	}

	// Check if user dismissed this session
	// Sessions are dismissed (snoozed) when LastAcknowledged is newer than LastMeaningfulOutput
	// This ensures sessions stay snoozed until NEW terminal output appears
	// (not just any save operation which updates UpdatedAt)
	if !inst.LastAcknowledged.IsZero() && inst.LastAcknowledged.After(inst.LastMeaningfulOutput) {
		log.DebugLog.Printf("[ReviewQueue] Session '%s': User acknowledged (snoozed until new output), removing from queue", inst.Title)
		rqp.queue.Remove(inst.Title)
		return
	}

	// Grace period: Don't re-add for 5 minutes after acknowledgment, even with new output
	// This prevents immediate re-notification after user dismisses a session
	if !inst.LastAcknowledged.IsZero() {
		gracePeriod := 5 * time.Minute
		timeSinceAck := time.Since(inst.LastAcknowledged)
		if timeSinceAck < gracePeriod {
			log.DebugLog.Printf("[ReviewQueue] Session '%s': Still in grace period (%s / %s since acknowledgment), skipping queue add",
				inst.Title, formatDuration(timeSinceAck), formatDuration(gracePeriod))
			rqp.queue.Remove(inst.Title)
			return
		}
	}

	// Spam prevention: Enforce minimum re-add interval to prevent notification spam
	// This prevents the same session from being added to the queue repeatedly every few seconds
	if shouldAdd {
		minReAddInterval := 2 * time.Minute
		if !inst.LastAddedToQueue.IsZero() && time.Since(inst.LastAddedToQueue) < minReAddInterval {
			log.DebugLog.Printf("[ReviewQueue] Session '%s': Skipping queue add (too soon - last added %v ago, minimum %v)",
				inst.Title, time.Since(inst.LastAddedToQueue), minReAddInterval)
			return
		}
	}

	// Add or update in queue
	log.InfoLog.Printf("[ReviewQueue] Session '%s': Final decision - shouldAdd=%v, reason=%s, priority=%s, context=%q",
		inst.Title, shouldAdd, reason.String(), priority.String(), context)

	if shouldAdd {
		// Check if item already exists and preserve DetectedAt if status hasn't changed
		detectedAt := time.Now()
		isUpdate := false
		if existingItem, exists := rqp.queue.Get(inst.Title); exists {
			isUpdate = true
			// Preserve original timestamp if meaningful fields haven't changed
			if existingItem.Reason == reason &&
				existingItem.Priority == priority &&
				existingItem.Context == context {
				detectedAt = existingItem.DetectedAt
				log.DebugLog.Printf("[ReviewQueue] Session '%s': Updating existing queue item (no changes, preserving timestamp)", inst.Title)
			} else {
				log.DebugLog.Printf("[ReviewQueue] Session '%s': Updating queue item (reason changed from %s to %s, priority %s to %s)",
					inst.Title, existingItem.Reason.String(), reason.String(), existingItem.Priority.String(), priority.String())
			}
		}

		// DO NOT update LastMeaningfulOutput here - it must reflect actual terminal output time
		// Updating it would defeat staleness detection by making the session appear fresh

		// MIGRATION: Skip adding items with zero LastMeaningfulOutput timestamps.
		// These are old sessions that haven't been migrated yet. They'll be re-added
		// automatically once the migration runs and gives them valid timestamps.
		if inst.LastMeaningfulOutput.IsZero() {
			log.InfoLog.Printf("[ReviewQueue] Session '%s': Skipping queue add - LastMeaningfulOutput is zero (needs migration)", inst.Title)
			// Remove any existing stale item with zero timestamp
			if rqp.queue.Has(inst.Title) {
				rqp.queue.Remove(inst.Title)
				log.InfoLog.Printf("[ReviewQueue] Session '%s': Removed stale queue item with zero timestamp", inst.Title)
			}
			return
		}

		item := &ReviewItem{
			SessionID:    inst.Title,
			SessionName:  inst.Title,
			Reason:       reason,
			Priority:     priority,
			DetectedAt:   detectedAt,
			Context:      context,
			// Populate session details for rich display
			Program:      inst.Program,
			Branch:       inst.Branch,
			Path:         inst.Path,
			WorkingDir:   inst.WorkingDir,
			Status:       inst.Status,
			Tags:         inst.Tags,
			Category:     inst.Category,
			DiffStats:    inst.GetDiffStats(),
			LastActivity: inst.LastMeaningfulOutput,
		}
		log.InfoLog.Printf("[ReviewQueue] Session '%s': ADDING TO QUEUE - reason=%s, priority=%s, context=%q",
			inst.Title, reason.String(), priority.String(), context)
		rqp.queue.Add(item)

		// Update spam prevention timestamp
		inst.LastAddedToQueue = time.Now()
		log.InfoLog.Printf("[ReviewQueue] Session '%s': Updated LastAddedToQueue timestamp to %v",
			inst.Title, inst.LastAddedToQueue)

		// CRITICAL: Persist LastAddedToQueue to database to prevent notification spam
		// Without persistence, this timestamp resets on app restart or instance reload,
		// causing the spam prevention check to fail and sessions to be re-added immediately
		// NOTE: Use UpdateInstanceLastAddedToQueue instead of SaveInstances to avoid
		// the merge logic which would restore deleted instances from disk.
		if rqp.storage != nil {
			if err := rqp.storage.UpdateInstanceLastAddedToQueue(inst.Title, inst.LastAddedToQueue); err != nil {
				log.ErrorLog.Printf("[ReviewQueue] Session '%s': Failed to persist LastAddedToQueue: %v", inst.Title, err)
			} else {
				log.DebugLog.Printf("[ReviewQueue] Session '%s': Successfully persisted LastAddedToQueue timestamp", inst.Title)
			}
		} else {
			log.DebugLog.Printf("[ReviewQueue] Session '%s': Storage not available, LastAddedToQueue will not persist", inst.Title)
		}

		if !isUpdate {
			log.InfoLog.Printf("[ReviewQueue] Session '%s': Successfully added to queue - %s (priority: %s, context: %s)",
				inst.Title, reason.String(), priority.String(), context)
		}
	} else {
		// Remove from queue - log only if it was actually in the queue
		if rqp.queue.Has(inst.Title) {
			log.DebugLog.Printf("[ReviewQueue] Session '%s': Removing from queue (shouldAdd=false)", inst.Title)
			rqp.queue.Remove(inst.Title)
		}
	}
}

// UpdateConfig updates the poller configuration.
func (rqp *ReviewQueuePoller) UpdateConfig(config ReviewQueuePollerConfig) {
	rqp.mu.Lock()
	defer rqp.mu.Unlock()
	rqp.config = config
	log.InfoLog.Printf("ReviewQueuePoller config updated: poll interval=%s, idle threshold=%s",
		config.PollInterval, config.IdleThreshold)
}

// GetConfig returns the current configuration.
func (rqp *ReviewQueuePoller) GetConfig() ReviewQueuePollerConfig {
	rqp.mu.RLock()
	defer rqp.mu.RUnlock()
	return rqp.config
}

// IsRunning returns true if the poller is currently running.
func (rqp *ReviewQueuePoller) IsRunning() bool {
	rqp.mu.RLock()
	defer rqp.mu.RUnlock()
	return rqp.ctx != nil && rqp.ctx.Err() == nil
}

// GetMonitoredCount returns the number of instances being monitored.
func (rqp *ReviewQueuePoller) GetMonitoredCount() int {
	rqp.mu.RLock()
	defer rqp.mu.RUnlock()
	return len(rqp.instances)
}

// CheckSession checks a single session immediately (exported for ReactiveQueueManager).
// This allows external components to trigger immediate re-evaluation without waiting for
// the next poll cycle, providing <100ms feedback on user interactions.
func (rqp *ReviewQueuePoller) CheckSession(inst *Instance) {
	rqp.checkSession(inst)
}

// FindInstance finds an instance by session ID (exported for ReactiveQueueManager).
// Returns nil if the instance is not found in the monitored list.
func (rqp *ReviewQueuePoller) FindInstance(sessionID string) *Instance {
	rqp.mu.RLock()
	defer rqp.mu.RUnlock()

	for _, inst := range rqp.instances {
		if inst.Title == sessionID {
			return inst
		}
	}
	return nil
}
