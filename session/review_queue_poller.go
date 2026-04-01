package session

import (
	"context"
	"fmt"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/detection"
	"strings"
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
		InputWaitDuration:  3 * time.Second, // Flag if waiting for input > 3s (reduced from 5s)
		StalenessThreshold: 2 * time.Minute, // Flag if no meaningful output for 2 minutes (reduced from 5min)
	}
}

// ApprovalMetadata holds metadata about a pending approval for enriching review queue items.
type ApprovalMetadata struct {
	ApprovalID string
	ToolName   string
	ToolInput  map[string]interface{}
	Cwd        string
	Orphaned   bool
}

// ApprovalMetadataProvider provides approval metadata for enriching review queue items.
// This interface decouples the poller (session package) from the ApprovalStore (services package).
type ApprovalMetadataProvider interface {
	// GetApprovalMetadataBySession returns approval metadata for the given session ID.
	// Returns nil if no approvals exist for the session.
	GetApprovalMetadataBySession(sessionID string) []ApprovalMetadata
}

// ReviewQueuePoller automatically monitors sessions and adds them to the review queue
// when they become idle or need attention.
type ReviewQueuePoller struct {
	queue            *ReviewQueue
	statusManager    *InstanceStatusManager
	storage          *Storage
	instances        []*Instance
	config           ReviewQueuePollerConfig
	statusDetector   *detection.StatusDetector // For detecting status in sessions without ClaudeController
	approvalProvider ApprovalMetadataProvider  // Optional: enriches approval items with hook metadata

	// Content cache: avoids spawning a tmux capture-pane subprocess when the session
	// has not produced new output since the last poll. For sessions with an active
	// ClaudeController the idle detector's lastActivity timestamp (driven by PTY output
	// reading, no subprocess) is used as a change signal.
	cacheMu          sync.Mutex
	lastSeenActivity map[string]time.Time // per-session: last IdleDetector.lastActivity seen
	cachedContent    map[string]string    // per-session: content from last Preview() call

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
		queue:            queue,
		statusManager:    statusManager,
		storage:          storage,
		instances:        make([]*Instance, 0),
		config:           config,
		statusDetector:   detection.NewStatusDetector(), // For detecting status in sessions without ClaudeController
		lastSeenActivity: make(map[string]time.Time),
		cachedContent:    make(map[string]string),
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

	// Evict content cache for this session.
	rqp.cacheMu.Lock()
	delete(rqp.lastSeenActivity, instanceTitle)
	delete(rqp.cachedContent, instanceTitle)
	rqp.cacheMu.Unlock()
}

// SetApprovalProvider sets the approval metadata provider for enriching review queue items.
func (rqp *ReviewQueuePoller) SetApprovalProvider(provider ApprovalMetadataProvider) {
	rqp.mu.Lock()
	defer rqp.mu.Unlock()
	rqp.approvalProvider = provider
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

// detectProcessing checks if session is actively processing after user interaction.
// Uses multiple signals to determine if the session is responding to user input.
func detectProcessing(inst *Instance, content string, statusInfo InstanceStatusInfo) bool {
	// Signal 1: Status change from prompt state to active/processing
	if statusInfo.ClaudeStatus == detection.StatusActive ||
		statusInfo.ClaudeStatus == detection.StatusProcessing {
		return true
	}

	// Signal 2: Idle detector shows Active state
	if statusInfo.IdleState.State == detection.IdleStateActive {
		return true
	}

	// Signal 3: Recent terminal output (activity within 2 seconds)
	if time.Since(inst.LastMeaningfulOutput) < 2*time.Second {
		return true
	}

	// Signal 4: Processing patterns in recent content (last 50 lines)
	processingPatterns := []string{
		"Thinking...",
		"Processing...",
		"Executing...",
		"Running...",
		"Working...",
		"Analyzing...",
		"esc to interrupt",
		"Synthesizing",
	}

	// Only check recent content (last ~50 lines) to avoid false positives from old output
	lines := strings.Split(content, "\n")
	recentLines := lines
	if len(lines) > 50 {
		recentLines = lines[len(lines)-50:]
	}
	recentContent := strings.Join(recentLines, "\n")

	for _, pattern := range processingPatterns {
		if strings.Contains(recentContent, pattern) {
			return true
		}
	}

	return false
}

// getContent returns the terminal content for inst, using a cache to avoid
// spawning a subprocess when no new output has arrived since the last poll.
//
// For sessions with an active ClaudeController: the idle detector's lastActivity
// timestamp is used as a change signal. lastActivity is updated in real-time
// via PTY output reading (no subprocess), so a zero-cost check tells us whether
// any new bytes arrived. If nothing changed, the last captured content is returned
// directly, saving one `tmux capture-pane` subprocess per idle session per tick.
//
// For sessions without a ClaudeController: Preview() is always called (no change
// signal is available without the controller's PTY reader).
//
// The error case returns the last cached content (empty string on the first poll)
// so callers can continue with empty content as before.
func (rqp *ReviewQueuePoller) getContent(inst *Instance, statusInfo InstanceStatusInfo) string {
	if statusInfo.IsControllerActive {
		lastActivity := statusInfo.IdleState.LastActivity
		if !lastActivity.IsZero() {
			rqp.cacheMu.Lock()
			lastSeen := rqp.lastSeenActivity[inst.Title]
			cached := rqp.cachedContent[inst.Title]
			rqp.cacheMu.Unlock()

			if lastActivity.Equal(lastSeen) {
				log.DebugLog.Printf("[ReviewQueue] Session '%s': content cache hit (lastActivity=%s, %d bytes)",
					inst.Title, lastActivity.Format("15:04:05.000"), len(cached))
				return cached
			}
		}
	}

	content, err := inst.Preview()
	if err != nil {
		log.DebugLog.Printf("[ReviewQueue] Session '%s': Preview() error: %v", inst.Title, err)
		rqp.cacheMu.Lock()
		cached := rqp.cachedContent[inst.Title]
		rqp.cacheMu.Unlock()
		return cached
	}

	rqp.cacheMu.Lock()
	rqp.cachedContent[inst.Title] = content
	if statusInfo.IsControllerActive && !statusInfo.IdleState.LastActivity.IsZero() {
		rqp.lastSeenActivity[inst.Title] = statusInfo.IdleState.LastActivity
	}
	rqp.cacheMu.Unlock()

	return content
}

// checkSession checks a single session and adds/removes from queue as needed.
func (rqp *ReviewQueuePoller) checkSession(inst *Instance) {
	log.InfoLog.Printf("[ReviewQueue] === CHECKING SESSION '%s' === (started=%v, paused=%v)",
		inst.Title, inst.Started(), inst.Paused())

	if inst.LastMeaningfulOutput.IsZero() {
		log.DebugLog.Printf("[ReviewQueue] Session '%s': LastMeaningfulOutput is zero — processing without output timestamp", inst.Title)
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

	// STEP 1: Get terminal content for prompt detection.
	// Uses cached content when the controller reports no new activity since the last
	// poll — avoids a subprocess spawn on every tick for idle controller-managed sessions.
	content := rqp.getContent(inst, statusInfo)

	// STEP 2: Detect and track prompts
	isNewPrompt := inst.detectAndTrackPrompt(content, statusInfo)

	// STEP 3: Check if user responded to current prompt
	userRespondedToPrompt := inst.ReviewState.UserRespondedAfterPrompt()

	// STEP 4: Check if session is actively processing after user response
	isProcessing := false
	if userRespondedToPrompt && content != "" {
		isProcessing = detectProcessing(inst, content, statusInfo)
	}

	// STEP 5: Check grace period for temporary removal
	inGracePeriod := inst.ReviewState.IsInProcessingGracePeriod()

	log.InfoLog.Printf("[ReviewQueue] Session '%s': isNewPrompt=%v, userResponded=%v, isProcessing=%v, gracePeriod=%v",
		inst.Title, isNewPrompt, userRespondedToPrompt, isProcessing, inGracePeriod)

	// DECISION LOGIC:

	// If user responded and session is processing -> remove from queue
	if userRespondedToPrompt && isProcessing {
		log.InfoLog.Printf("[ReviewQueue] Session '%s': User responded and processing - removing from queue", inst.Title)
		rqp.queue.Remove(inst.Title)
		inst.ProcessingGraceUntil = time.Time{} // Clear grace period
		// Persist cleared grace period
		if rqp.storage != nil {
			if err := rqp.storage.UpdateInstanceProcessingGrace(inst.Title, inst.ProcessingGraceUntil); err != nil {
				log.ErrorLog.Printf("Failed to persist cleared ProcessingGraceUntil: %v", err)
			}
		}
		return
	}

	// If user responded but NOT processing yet -> grace period
	if userRespondedToPrompt && !isProcessing {
		if inGracePeriod {
			// Already in grace period - keep off queue
			log.DebugLog.Printf("[ReviewQueue] Session '%s': In grace period, keeping off queue", inst.Title)
			rqp.queue.Remove(inst.Title)
			return
		}

		if inst.ProcessingGraceUntil.IsZero() {
			// Fresh response - start grace period and remove from queue
			inst.ProcessingGraceUntil = time.Now().Add(10 * time.Second)
			log.InfoLog.Printf("[ReviewQueue] Session '%s': User responded, starting grace period until %v",
				inst.Title, inst.ProcessingGraceUntil)

			// Persist grace period
			if rqp.storage != nil {
				if err := rqp.storage.UpdateInstanceProcessingGrace(inst.Title, inst.ProcessingGraceUntil); err != nil {
					log.ErrorLog.Printf("Failed to persist ProcessingGraceUntil: %v", err)
				}
			}
			rqp.queue.Remove(inst.Title)
			return
		}

		// Grace period expired and still not processing
		// Clear grace period and fall through to add logic (will check if new prompt)
		log.InfoLog.Printf("[ReviewQueue] Session '%s': Grace period expired, session not responding", inst.Title)
		inst.ProcessingGraceUntil = time.Time{}
		if rqp.storage != nil {
			if err := rqp.storage.UpdateInstanceProcessingGrace(inst.Title, inst.ProcessingGraceUntil); err != nil {
				log.ErrorLog.Printf("Failed to persist cleared ProcessingGraceUntil: %v", err)
			}
		}
	}

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
				inst.Title, idleState.String(), detection.FormatDuration(time.Since(lastActivity)))

			// IMPORTANT: Check Claude status FIRST before idle state handling.
			// Status-based conditions (approval, input required, error) take priority over
			// idle state because they represent explicit user prompts that need attention,
			// even if terminal activity makes the session appear "active".

			// Check for approval needs (highest priority for user prompts)
			if statusInfo.ClaudeStatus == detection.StatusNeedsApproval || statusInfo.PendingApprovals > 0 {
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
			if statusInfo.ClaudeStatus == detection.StatusInputRequired {
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
			if statusInfo.ClaudeStatus == detection.StatusError {
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
			if statusInfo.ClaudeStatus == detection.StatusTestsFailing {
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
			if statusInfo.ClaudeStatus == detection.StatusSuccess {
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

			// Now handle idle state - but only if no status-based condition was detected above.
			// This ensures user prompts aren't hidden just because terminal is "active".
			if !shouldAdd {
				switch idleState {
				case detection.IdleStateActive:
					// Actively working, remove from queue (but only if no prompt detected above)
					log.DebugLog.Printf("[ReviewQueue] Session '%s': Active state with no prompts - removing from queue", inst.Title)
					rqp.queue.Remove(inst.Title)
					return

				case detection.IdleStateWaiting:
					// Normal idle state (e.g., INSERT mode) - don't add by default
					log.DebugLog.Printf("[ReviewQueue] Session '%s': Waiting state - will check for specific issues", inst.Title)
					shouldAdd = false

				case detection.IdleStateTimeout:
					// Definite timeout - been idle too long
					// Use semantic ReasonIdle instead of deprecated ReasonIdleTimeout
					reason = ReasonIdle
					priority = PriorityLow
					shouldAdd = true
					idleDuration := time.Since(lastActivity)
					context = "Session idle - ready for next task"
					log.DebugLog.Printf("[ReviewQueue] Session '%s': Idle detected - idle for %s", inst.Title, detection.FormatDuration(idleDuration))
				}
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
		// No controller - but we can still detect status from terminal content
		log.DebugLog.Printf("[ReviewQueue] Session '%s': No active controller - using terminal-based status detection", inst.Title)

		// IMPORTANT: Check terminal content for approval/input prompts.
		// 'content' was already fetched at STEP 1 via getContent(); for no-controller
		// sessions getContent() always calls Preview(), so no extra subprocess needed.
		if content != "" {
			// Detect status from terminal content using the shared status detector
			detectedStatus, statusContext := rqp.statusDetector.DetectWithContext([]byte(content))
			log.DebugLog.Printf("[ReviewQueue] Session '%s': Detected status=%s from terminal content",
				inst.Title, detectedStatus.String())

			// Check for approval needs (highest priority for user prompts)
			if detectedStatus == detection.StatusNeedsApproval {
				reason = ReasonApprovalPending
				priority = PriorityHigh
				shouldAdd = true
				if statusContext != "" {
					context = statusContext
				} else {
					context = "Waiting for approval to proceed"
				}
				log.InfoLog.Printf("[ReviewQueue] Session '%s': Approval needed (no controller) - %s", inst.Title, context)
			}

			// Check for input required (explicit prompts asking for user input)
			if detectedStatus == detection.StatusInputRequired {
				reason = ReasonInputRequired
				priority = PriorityMedium
				shouldAdd = true
				if statusContext != "" {
					context = statusContext
				} else {
					context = "Waiting for explicit user input"
				}
				log.InfoLog.Printf("[ReviewQueue] Session '%s': Input required (no controller) - %s", inst.Title, context)
			}

			// Check for errors (highest priority)
			if detectedStatus == detection.StatusError {
				reason = ReasonErrorState
				priority = PriorityUrgent
				shouldAdd = true
				if statusContext != "" {
					context = statusContext
				} else {
					context = "Error state detected"
				}
				log.InfoLog.Printf("[ReviewQueue] Session '%s': Error detected (no controller) - %s", inst.Title, context)
			}

			// If actively processing, don't add to queue
			if detectedStatus == detection.StatusActive || detectedStatus == detection.StatusProcessing {
				log.DebugLog.Printf("[ReviewQueue] Session '%s': Active/processing state detected - not adding to queue", inst.Title)
				rqp.queue.Remove(inst.Title)
				return
			}
		}

		// If no status-based condition was detected, fall back to time-based checks
		if !shouldAdd {
			// Check if session has been idle for a long time based on UpdatedAt
			idleDuration := time.Since(inst.UpdatedAt)
			const basicIdleThreshold = 5 * time.Second

			if idleDuration > basicIdleThreshold {
				// Use semantic ReasonIdle instead of deprecated ReasonIdleTimeout
				reason = ReasonIdle
				priority = PriorityLow
				shouldAdd = true
				context = "Session idle - ready for next task"
				log.DebugLog.Printf("[ReviewQueue] Session '%s': Basic idle check - %s since UpdatedAt",
					inst.Title, detection.FormatDuration(idleDuration))
			}
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
		inst.Title, detection.FormatDuration(timeSinceOutput), detection.FormatDuration(rqp.config.StalenessThreshold), shouldAdd, priority)

	// Check if user has acknowledged this session after it became stale
	// If acknowledged after last output, don't re-flag as stale
	alreadyAcknowledged := inst.ReviewState.IsAcknowledgedAfterOutput()

	if timeSinceOutput > rqp.config.StalenessThreshold {
		if alreadyAcknowledged {
			log.InfoLog.Printf("[ReviewQueue] Session '%s': STALE but already acknowledged - skipping staleness flag",
				inst.Title)
		} else {
			log.InfoLog.Printf("[ReviewQueue] Session '%s': STALENESS DETECTED - time since output (%s) > threshold (%s)",
				inst.Title, detection.FormatDuration(timeSinceOutput), detection.FormatDuration(rqp.config.StalenessThreshold))

			// Only override if we don't already have a higher-priority reason.
			// Only set stale if not already flagged with Medium priority or higher.
			if !shouldAdd || priority.IsLowerThan(PriorityMedium) {
				// Use semantic ReasonStale instead of deprecated ReasonIdleTimeout
				reason = ReasonStale
				priority = PriorityLow // Lower priority than approval/error, but should be reviewed
				shouldAdd = true
				context = fmt.Sprintf("No activity for %s - session may be stuck or waiting",
					detection.FormatDuration(timeSinceOutput))

				log.InfoLog.Printf("[ReviewQueue] Session '%s': SETTING shouldAdd=true - flagged as stale - %s since last meaningful output",
					inst.Title, detection.FormatDuration(timeSinceOutput))
			} else {
				log.InfoLog.Printf("[ReviewQueue] Session '%s': Stale but already has higher priority reason (%s)",
					inst.Title, reason.String())
			}
		}
	} else {
		log.InfoLog.Printf("[ReviewQueue] Session '%s': NOT STALE - time since output (%s) <= threshold (%s)",
			inst.Title, detection.FormatDuration(timeSinceOutput), detection.FormatDuration(rqp.config.StalenessThreshold))
	}

	// Acknowledgment snooze and grace-period checks.
	// For sessions with an active controller, high/medium-priority states (approval, error,
	// input required) bypass snooze so live Claude processes always surface to the user.
	// For sessions WITHOUT an active controller (orphaned/external/no-tty sessions), there is
	// no live process that can generate new approvals — detections come from static terminal
	// content. In that case, respect acknowledgment at any priority level so the user can
	// permanently dismiss stale entries.
	if !shouldAdd || priority == PriorityLow || !statusInfo.IsControllerActive {
		// Check if user dismissed this session.
		// Sessions are snoozed when LastAcknowledged is newer than LastMeaningfulOutput.
		if inst.ReviewState.IsAcknowledgedAfterOutput() {
			log.DebugLog.Printf("[ReviewQueue] Session '%s': User acknowledged (snoozed until new output), removing from queue", inst.Title)
			rqp.queue.Remove(inst.Title)
			return
		}

		// Grace period: Don't re-add for 5 minutes after acknowledgment, even with new output.
		// This prevents immediate re-notification after user dismisses a session.
		if !inst.LastAcknowledged.IsZero() {
			gracePeriod := 5 * time.Minute
			timeSinceAck := time.Since(inst.LastAcknowledged)
			if timeSinceAck < gracePeriod {
				log.DebugLog.Printf("[ReviewQueue] Session '%s': Still in grace period (%s / %s since acknowledgment), skipping queue add",
					inst.Title, detection.FormatDuration(timeSinceAck), detection.FormatDuration(gracePeriod))
				rqp.queue.Remove(inst.Title)
				return
			}
		}
	}

	// Prevent re-adding same prompt user already responded to
	// Only add if this is a NEW prompt OR user hasn't responded yet
	if shouldAdd && userRespondedToPrompt && !isNewPrompt {
		log.InfoLog.Printf("[ReviewQueue] Session '%s': User already responded to this prompt - removing from queue", inst.Title)
		rqp.queue.Remove(inst.Title)
		return
	}

	// Spam prevention: Enforce minimum re-add interval to prevent notification spam.
	// Only applies when the item is ALREADY in the queue (i.e., already visible to the user).
	// After a server restart the queue is empty, so LastAddedToQueue from before the restart
	// must not block urgent prompts from re-appearing — the session should always be re-added.
	if shouldAdd {
		minReAddInterval := 2 * time.Minute
		if !inst.LastAddedToQueue.IsZero() && time.Since(inst.LastAddedToQueue) < minReAddInterval {
			if existingItem, exists := rqp.queue.Get(inst.Title); exists {
				// Lower priority number = higher priority (Urgent=1 > High=2 > Medium=3 > Low=4)
				isEscalation := priority < existingItem.Priority
				if isEscalation {
					log.InfoLog.Printf("[ReviewQueue] Session '%s': Priority escalation (%s → %s) - bypassing rate limit",
						inst.Title, existingItem.Priority.String(), priority.String())
				} else {
					log.DebugLog.Printf("[ReviewQueue] Session '%s': Skipping queue add (too soon - last added %v ago, minimum %v)",
						inst.Title, time.Since(inst.LastAddedToQueue), minReAddInterval)
					return
				}
			}
			// Item not currently in queue (e.g., post-restart): bypass rate limit so the
			// session re-appears without waiting up to 2 minutes.
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

		// Use CreatedAt as fallback LastActivity when LastMeaningfulOutput hasn't been set yet
		// (new sessions, sessions where StartController failed before the migration ran).
		lastActivity := inst.LastMeaningfulOutput
		if lastActivity.IsZero() {
			lastActivity = inst.CreatedAt
		}

		item := &ReviewItem{
			SessionID:   inst.Title,
			SessionName: inst.Title,
			Reason:      reason,
			Priority:    priority,
			DetectedAt:  detectedAt,
			Context:     context,
			// Populate session details for rich display
			Program:      inst.Program,
			Branch:       inst.Branch,
			Path:         inst.Path,
			WorkingDir:   inst.WorkingDir,
			Status:       inst.Status.String(),
			Tags:         inst.Tags,
			Category:     inst.Category,
			DiffStats:    inst.GetDiffStats(),
			LastActivity: lastActivity,
		}

		// Enrich approval items with hook metadata from ApprovalStore (Story 3, Task 3.2).
		if reason == ReasonApprovalPending && rqp.approvalProvider != nil {
			if approvals := rqp.approvalProvider.GetApprovalMetadataBySession(inst.Title); len(approvals) > 0 {
				a := approvals[0] // Use the most recent/first approval
				if item.Metadata == nil {
					item.Metadata = make(map[string]string)
				}
				item.Metadata["pending_approval_id"] = a.ApprovalID
				item.Metadata["tool_name"] = a.ToolName
				if cmd, ok := a.ToolInput["command"].(string); ok && cmd != "" {
					item.Metadata["tool_input_command"] = cmd
				}
				if filePath, ok := a.ToolInput["file_path"].(string); ok && filePath != "" {
					item.Metadata["tool_input_file"] = filePath
				}
				if a.Cwd != "" {
					item.Metadata["cwd"] = a.Cwd
				}
				if a.Orphaned {
					item.Metadata["orphaned"] = "true"
				}
				log.InfoLog.Printf("[ReviewQueue] Session '%s': Enriched approval item with hook metadata (tool=%s, approval_id=%s)",
					inst.Title, a.ToolName, a.ApprovalID)
			}
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

// GetInstances returns a snapshot of all live in-memory instances held by the poller.
// Use this instead of LoadInstances() for read-only operations to avoid the side effect
// of FromInstanceData() calling Start() on every non-paused instance.
func (rqp *ReviewQueuePoller) GetInstances() []*Instance {
	rqp.mu.RLock()
	defer rqp.mu.RUnlock()
	result := make([]*Instance, len(rqp.instances))
	copy(result, rqp.instances)
	return result
}
