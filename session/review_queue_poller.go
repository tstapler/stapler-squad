package session

import "github.com/linkdata/deadlock"

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/detection"
	"github.com/tstapler/stapler-squad/session/tmux"
)

// ReviewQueuePollerConfig contains configuration for the review queue poller.
type ReviewQueuePollerConfig struct {
	PollInterval       time.Duration // How often to check sessions (fast path, default 2s)
	SlowPollInterval   time.Duration // Interval when review queue is empty (default 8s); 0 = no backoff
	IdleThreshold      time.Duration // Duration before considering session idle and adding to queue
	InputWaitDuration  time.Duration // Time waiting for input before flagging
	StalenessThreshold time.Duration // Duration since last meaningful output before considering stale
	ReconcileInterval  time.Duration // How often to reconcile in-memory state against tmux reality (0 = disabled)
}

// DefaultReviewQueuePollerConfig returns sensible defaults for polling.
func DefaultReviewQueuePollerConfig() ReviewQueuePollerConfig {
	return ReviewQueuePollerConfig{
		PollInterval:       2 * time.Second,  // Poll every 2 seconds for immediate detection
		SlowPollInterval:   8 * time.Second,  // Back off to 8s when queue is empty
		IdleThreshold:      5 * time.Second,  // Add to queue after 5s idle for immediate user notifications
		InputWaitDuration:  3 * time.Second,  // Flag if waiting for input > 3s (reduced from 5s)
		StalenessThreshold: 2 * time.Minute,  // Flag if no meaningful output for 2 minutes (reduced from 5min)
		ReconcileInterval:  30 * time.Second, // Reconcile against tmux reality every 30 seconds
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
	// has not produced new output since the last poll.
	//
	// For sessions with an active ClaudeController: the idle detector's lastActivity
	// timestamp (driven by PTY reads, no subprocess) is the change signal.
	//
	// For sessions without a ClaudeController: #{pane_last_activity} from a single
	// `tmux list-panes -a` call (fetched once per checkSessions tick) is the change
	// signal. A capture-pane subprocess is spawned only when that timestamp advances.
	// A 30s TTL acts as a safety fallback when list-panes is unavailable.
	cacheMu              deadlock.Mutex
	lastSeenActivity     map[string]time.Time // per-session: last IdleDetector.lastActivity seen
	lastSeenPaneActivity map[string]time.Time // per-session: last #{pane_last_activity} seen
	cachedContent        map[string]string    // per-session: content from last Preview() call
	lastPreviewTime      map[string]time.Time // per-session: fallback TTL timestamp

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	mu     deadlock.RWMutex

	// activityCh is an optional channel that signals the poll loop to snap back to
	// the fast interval. Wired by ReactiveQueueManager on EventApprovalResponse and
	// EventUserInteraction. A nil channel is never selected (safe in select statements).
	activityCh <-chan struct{}

	// Backoff state: tracks consecutive poll errors to apply exponential delay.
	consecutiveErrors int
	// tickCount counts poll loop iterations; atomic because pollLoop writes and tests read concurrently.
	tickCount atomic.Int64
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
		queue:                queue,
		statusManager:        statusManager,
		storage:              storage,
		instances:            make([]*Instance, 0),
		config:               config,
		statusDetector:       detection.NewStatusDetector(),
		lastSeenActivity:     make(map[string]time.Time),
		lastSeenPaneActivity: make(map[string]time.Time),
		cachedContent:        make(map[string]string),
		lastPreviewTime:      make(map[string]time.Time),
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
	var removedTitle string
	for _, inst := range rqp.instances {
		if inst.MatchesID(instanceTitle) {
			removedTitle = inst.Title
		} else {
			filtered = append(filtered, inst)
		}
	}
	rqp.instances = filtered

	// Evict content cache using the resolved title (MatchesID may have matched by UUID).
	evictKey := instanceTitle
	if removedTitle != "" {
		evictKey = removedTitle
	}
	rqp.cacheMu.Lock()
	delete(rqp.lastSeenActivity, evictKey)
	delete(rqp.lastSeenPaneActivity, evictKey)
	delete(rqp.cachedContent, evictKey)
	delete(rqp.lastPreviewTime, evictKey)
	rqp.cacheMu.Unlock()
}

// SetApprovalProvider sets the approval metadata provider for enriching review queue items.
func (rqp *ReviewQueuePoller) SetApprovalProvider(provider ApprovalMetadataProvider) {
	rqp.mu.Lock()
	defer rqp.mu.Unlock()
	rqp.approvalProvider = provider
}

// SetActivityChannel wires an external signal channel to the poll loop. When a signal
// arrives on ch, the loop snaps back to the fast interval (PollInterval). Must be called
// before Start(); subsequent calls have no effect once the loop is running.
func (rqp *ReviewQueuePoller) SetActivityChannel(ch <-chan struct{}) {
	rqp.mu.Lock()
	rqp.activityCh = ch
	rqp.mu.Unlock()
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
//
// Adaptive interval: when an activityCh is wired and the review queue becomes empty,
// the loop backs off to SlowPollInterval (8s by default). Any signal on activityCh
// snaps the interval back to PollInterval (2s) immediately.
func (rqp *ReviewQueuePoller) pollLoop() {
	defer rqp.wg.Done()

	fastInterval := rqp.config.PollInterval
	slowInterval := rqp.config.SlowPollInterval
	if slowInterval <= 0 {
		slowInterval = fastInterval
	}

	// Capture activityCh once; must be set before Start() for the snap behavior to work.
	// A nil channel is never selected in a select statement, so the adaptive path is
	// simply skipped when no activity channel is wired.
	rqp.mu.RLock()
	actCh := rqp.activityCh
	rqp.mu.RUnlock()

	interval := fastInterval
	timer := time.NewTimer(interval)
	defer timer.Stop()

	for {
		select {
		case <-rqp.ctx.Done():
			return

		case <-actCh:
			// Snap back to fast interval on external activity signal.
			if interval != fastInterval {
				interval = fastInterval
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(interval)
			}

		case <-timer.C:
			rqp.tickCount.Add(1)

			if err := rqp.checkSessionsSafe(); err != nil {
				rqp.consecutiveErrors++
				backoff := rqp.backoffDuration(rqp.consecutiveErrors)
				log.WarningLog.Printf("[ReviewQueuePoller] checkSessions error (consecutive: %d): %v — backing off %s",
					rqp.consecutiveErrors, err, backoff)
				select {
				case <-rqp.ctx.Done():
					return
				case <-time.After(backoff):
				}
				timer.Reset(interval)
				continue
			}
			rqp.consecutiveErrors = 0

			// Adaptive interval: back off when queue is empty and activity channel is wired.
			if actCh != nil && len(rqp.queue.List()) == 0 {
				interval = slowInterval
			} else {
				interval = fastInterval
			}
			timer.Reset(interval)

			// Periodic reconciliation: verify in-memory state matches tmux reality.
			// Runs every ReconcileInterval ticks; skipped when ReconcileInterval is 0.
			if rqp.config.ReconcileInterval > 0 {
				ticksPerReconcile := int(rqp.config.ReconcileInterval / rqp.config.PollInterval)
				if ticksPerReconcile < 1 {
					ticksPerReconcile = 1
				}
				if rqp.tickCount.Load()%int64(ticksPerReconcile) == 0 {
					rqp.reconcileSessions()
				}
			}
		}
	}
}

// checkSessionsSafe wraps checkSessions with panic recovery, returning an error on panic.
func (rqp *ReviewQueuePoller) checkSessionsSafe() (retErr error) {
	defer func() {
		if r := recover(); r != nil {
			retErr = fmt.Errorf("panic in checkSessions: %v", r)
			log.ErrorLog.Printf("[ReviewQueuePoller] panic recovered: %v", r)
		}
	}()
	rqp.checkSessions()
	return nil
}

// backoffDuration returns the exponential backoff duration for the given error count.
// Caps at 30 seconds: 2s, 4s, 8s, 16s, 30s, 30s, ...
func (rqp *ReviewQueuePoller) backoffDuration(consecutiveErrors int) time.Duration {
	const base = 2 * time.Second
	const maxBackoff = 30 * time.Second
	if consecutiveErrors <= 0 {
		return base
	}
	shift := consecutiveErrors - 1
	if shift > 10 {
		shift = 10 // prevent overflow
	}
	backoff := base * (1 << uint(shift))
	if backoff > maxBackoff {
		backoff = maxBackoff
	}
	return backoff
}

// ForceReconcile immediately runs session reconciliation outside the normal 30s cadence.
// Safe to call concurrently; typically used by the fork pressure monitor to rapidly clean
// up dead sessions when subprocess failures indicate stale Running/Ready states.
func (rqp *ReviewQueuePoller) ForceReconcile() {
	log.InfoLog.Printf("[ReviewQueuePoller] ForceReconcile triggered")
	rqp.reconcileSessions()
}

// reconcileSessions compares in-memory instances against live tmux sessions.
// - Running/Ready instances not found in tmux are transitioned to Stopped.
// - Stopped instances whose tmux session is found alive are revived to Running.
func (rqp *ReviewQueuePoller) reconcileSessions() {
	rqp.mu.RLock()
	instances := make([]*Instance, len(rqp.instances))
	copy(instances, rqp.instances)
	rqp.mu.RUnlock()

	if len(instances) == 0 {
		return
	}

	// Determine the server socket from the first managed instance (all share the same socket).
	serverSocket := ""
	for _, inst := range instances {
		if inst.IsManaged && inst.TmuxServerSocket != "" {
			serverSocket = inst.TmuxServerSocket
			break
		}
	}

	liveSessions, err := tmux.ListAllSessions(serverSocket)
	if err != nil {
		if err == tmux.ErrServerDown {
			log.WarningLog.Printf("[ReviewQueuePoller] reconcileSessions: tmux server is down — skipping reconciliation")
		} else {
			log.WarningLog.Printf("[ReviewQueuePoller] reconcileSessions: ListAllSessions error: %v", err)
		}
		return
	}

	for _, inst := range instances {
		if !inst.IsManaged {
			continue
		}
		sessionName := inst.GetTmuxSessionName()
		if sessionName == "" {
			continue
		}

		switch inst.Status {
		case Running, Ready:
			// Running/Ready but tmux session gone — mark Stopped.
			if !liveSessions[sessionName] {
				log.WarningLog.Printf("[ReviewQueuePoller] reconcileSessions: managed session '%s' (tmux: %s) not found in live sessions — transitioning to Stopped",
					inst.Title, sessionName)
				inst.stateMutex.Lock()
				switch inst.Status {
				case Running, Ready:
					if err := inst.transitionTo(Stopped); err != nil {
						log.WarningLog.Printf("[ReviewQueuePoller] reconcileSessions: transition to Stopped failed for '%s': %v — using setStatus", inst.Title, err)
						inst.setStatus(Stopped)
					}
				}
				inst.stateMutex.Unlock()
				inst.fireLifecycleEvent(EventExited, "reconcile-session-missing")
			}
		case Stopped:
			// Stopped but tmux session is alive — revive to Running.
			if liveSessions[sessionName] {
				log.InfoLog.Printf("[ReviewQueuePoller] reconcileSessions: stopped session '%s' (tmux: %s) found alive — reviving to Running",
					inst.Title, sessionName)
				inst.stateMutex.Lock()
				if inst.Status == Stopped {
					if err := inst.transitionTo(Running); err != nil {
						log.WarningLog.Printf("[ReviewQueuePoller] reconcileSessions: revival to Running failed for '%s': %v", inst.Title, err)
					}
				}
				inst.stateMutex.Unlock()
				inst.fireLifecycleEvent(EventStarted, "reconcile-session-revived")
			}
		}
	}
}

// checkSessionsConcurrency caps the number of sessions checked simultaneously,
// limiting concurrent subprocess (capture-pane) calls to avoid fork exhaustion
// on macOS (kern.maxprocperuid).
const checkSessionsConcurrency = 5

// checkSessions checks all instances and updates the review queue.
func (rqp *ReviewQueuePoller) checkSessions() {
	rqp.mu.RLock()
	instances := make([]*Instance, len(rqp.instances))
	copy(instances, rqp.instances)
	rqp.mu.RUnlock()

	// Fetch pane activity timestamps once for all sessions. This single subprocess call
	// replaces per-session capture-pane calls when content hasn't changed.
	paneActivity := batchPaneActivity("")

	sem := make(chan struct{}, checkSessionsConcurrency)
	var wg sync.WaitGroup
	for _, inst := range instances {
		sem <- struct{}{}
		wg.Add(1)
		go func(i *Instance) {
			defer wg.Done()
			defer func() { <-sem }()
			rqp.checkSession(i, paneActivity)
		}(inst)
	}
	wg.Wait()
}

// detectProcessing checks if session is actively processing after user interaction.
// Uses multiple signals to determine if the session is responding to user input.
// detector must be the caller's already-compiled StatusDetector (e.g. rqp.statusDetector).
func detectProcessing(inst *Instance, content string, statusInfo InstanceStatusInfo, detector *detection.StatusDetector) bool {
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

	// Signal 4: Detect active/processing status via ANSI-stripped tail window.
	// This replaces the previous hardcoded string-match list and ensures the same
	// detection pipeline (CR collapsing + ANSI stripping + regex) is used everywhere.
	if content != "" {
		status := detector.DetectRecent([]byte(content), detection.StatusDetectionTailBytes)
		if status == detection.StatusActive || status == detection.StatusProcessing {
			return true
		}
	}

	return false
}

// previewCacheTTL is the fallback maximum age of a cached Preview() result when
// pane activity timestamps are unavailable (e.g. tmux not running). The primary
// invalidation mechanism is #{pane_last_activity} from batchPaneActivity(); this
// TTL is only a safety net and is intentionally long.
const previewCacheTTL = 30 * time.Second

// getContent returns the terminal content for inst, using a cache to avoid
// spawning a subprocess when no new output has arrived since the last poll.
//
// For sessions with an active ClaudeController: the idle detector's lastActivity
// timestamp (driven by PTY reads, no subprocess) is the change signal.
//
// For sessions without a ClaudeController: #{pane_last_activity} from the
// paneActivity snapshot (one `tmux list-panes -a` call shared across all sessions)
// is the change signal. capture-pane is only called when that timestamp advances.
// A 30s TTL acts as a fallback when paneActivity is nil (tmux unavailable).
//
// On error, the last cached content is returned so callers see empty string only
// on the very first poll for a session.
func (rqp *ReviewQueuePoller) getContent(inst *Instance, statusInfo InstanceStatusInfo, paneActivity map[string]time.Time) string {
	if statusInfo.IsControllerActive {
		lastActivity := statusInfo.IdleState.LastActivity
		if !lastActivity.IsZero() {
			rqp.cacheMu.Lock()
			lastSeen := rqp.lastSeenActivity[inst.Title]
			cached := rqp.cachedContent[inst.Title]
			rqp.cacheMu.Unlock()

			if lastActivity.Equal(lastSeen) {
				return cached
			}
		}
	} else {
		rqp.cacheMu.Lock()
		cached := rqp.cachedContent[inst.Title]
		lastSeenPane := rqp.lastSeenPaneActivity[inst.Title]
		lastCall := rqp.lastPreviewTime[inst.Title]
		rqp.cacheMu.Unlock()

		if paneActivity != nil {
			// Primary: event-driven via #{pane_last_activity}.
			tmuxName := inst.GetTmuxSessionName()
			if currentActivity, ok := paneActivity[tmuxName]; ok {
				if !currentActivity.IsZero() && currentActivity.Equal(lastSeenPane) {
					return cached
				}
			}
		} else if !lastCall.IsZero() && time.Since(lastCall) < previewCacheTTL {
			return cached
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

	// Update LastMeaningfulOutput when new terminal content is detected.
	// This ensures sessions resurface in the review queue after producing new output,
	// even when the user hasn't visited them via WebSocket streaming.
	// The content-signature dedup in UpdateTimestamps() (persisted to DB) prevents
	// false positives: if content is cosmetically changed but semantically the same
	// as when the user last acknowledged, LastMeaningfulOutput is not updated and
	// the acknowledgment snooze is preserved.
	if content != "" {
		inst.UpdateTerminalTimestamps(content, false)
	}

	rqp.cacheMu.Lock()
	rqp.cachedContent[inst.Title] = content
	if statusInfo.IsControllerActive && !statusInfo.IdleState.LastActivity.IsZero() {
		rqp.lastSeenActivity[inst.Title] = statusInfo.IdleState.LastActivity
	} else {
		if paneActivity != nil {
			tmuxName := inst.GetTmuxSessionName()
			if currentActivity, ok := paneActivity[tmuxName]; ok && !currentActivity.IsZero() {
				rqp.lastSeenPaneActivity[inst.Title] = currentActivity
			}
		}
		rqp.lastPreviewTime[inst.Title] = time.Now()
	}
	rqp.cacheMu.Unlock()

	return content
}

// checkSession checks a single session and adds/removes from queue as needed.
// paneActivity is the snapshot from batchPaneActivity(); nil falls back to TTL cache.
func (rqp *ReviewQueuePoller) checkSession(inst *Instance, paneActivity map[string]time.Time) {
	// Skip paused, stopped, or unstarted sessions
	if !inst.Started() || inst.Paused() || inst.Status == Stopped {
		return
	}

	// Sessions with an active controller get status updates via events; skip fast-path poll.
	if inst.GetController() != nil {
		return
	}

	// Get comprehensive status
	statusInfo := rqp.statusManager.GetStatus(inst)

	// STEP 1: Get terminal content for prompt detection.
	// Uses cached content when the controller reports no new activity since the last
	// poll — avoids a subprocess spawn on every tick for idle controller-managed sessions.
	content := rqp.getContent(inst, statusInfo, paneActivity)

	// STEP 2: Detect and track prompts
	isNewPrompt := inst.detectAndTrackPrompt(content, statusInfo)

	// STEP 3: Check if user responded to current prompt
	userRespondedToPrompt := inst.UserRespondedAfterPrompt()

	// STEP 4: Check if session is actively processing after user response
	isProcessing := false
	if userRespondedToPrompt && content != "" {
		isProcessing = detectProcessing(inst, content, statusInfo, rqp.statusDetector)
	}

	// STEP 5: Check grace period for temporary removal
	inGracePeriod := inst.IsInProcessingGracePeriod()

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
	// claudeStatus captures the raw DetectedStatus from whichever detection path ran.
	// For controller sessions this is statusInfo.ClaudeStatus; for no-controller sessions
	// it is set inside the else block when content is available.
	claudeStatus := statusInfo.ClaudeStatus

	// Check for controller-based states if controller is active
	if statusInfo.IsControllerActive {
		controller, exists := rqp.statusManager.GetController(inst.Title)
		if exists && controller != nil {
			// Get idle state from controller
			idleState, _ := controller.GetIdleState()

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
					rqp.queue.Remove(inst.Title)
					return

				case detection.IdleStateWaiting:
					// Normal idle state (e.g., INSERT mode) - don't add by default
					shouldAdd = false

				case detection.IdleStateTimeout:
					// Definite timeout - been idle too long
					reason = ReasonIdle
					priority = PriorityLow
					shouldAdd = true
					context = "Session idle - ready for next task"
				}
			}

			// Check for uncommitted changes (informational - user may want to review and commit)
			// Only check if we don't already have a higher-priority reason
			if (!shouldAdd || priority == PriorityLow) && inst.HasGitWorktree() {
				worktree, err := inst.GetGitWorktree()
				if err != nil {
					log.WarningLog.Printf("[ReviewQueue] Session '%s': Failed to get git worktree: %v", inst.Title, err)
				} else if worktree != nil {
					isDirty, err := worktree.IsDirty()
					if err != nil {
						log.WarningLog.Printf("[ReviewQueue] Session '%s': Failed to check git status: %v", inst.Title, err)
						log.LogForSession(inst.Title, "warning", "Failed to check git status: %v", err)
					} else if isDirty {
						// Only override if we don't have a higher priority reason already
						if !shouldAdd || priority == PriorityLow {
							reason = ReasonUncommittedChanges
							priority = PriorityLow
							shouldAdd = true
							context = "Uncommitted changes ready to commit"
							log.InfoLog.Printf("[ReviewQueue] Session '%s': Uncommitted changes detected", inst.Title)
						}
					} else {
						// Worktree is clean — if session was queued solely for uncommitted
						// changes, remove it immediately so it doesn't persist through the
						// rate-limiter window after a commit.
						if existing, exists := rqp.queue.Get(inst.Title); exists && existing.Reason == ReasonUncommittedChanges {
							log.InfoLog.Printf("[ReviewQueue] Session '%s': Changes committed - removing UncommittedChanges entry", inst.Title)
							rqp.queue.Remove(inst.Title)
						}
					}
				}
			}
		}
	} else {
		// No controller - but we can still detect status from terminal content.

		// IMPORTANT: Check terminal content for approval/input prompts.
		// 'content' was already fetched at STEP 1 via getContent(); for no-controller
		// sessions getContent() always calls Preview(), so no extra subprocess needed.
		if content != "" {
			// Detect status from terminal content using the shared status detector
			detectedStatus, statusContext := rqp.statusDetector.DetectWithContext([]byte(content))
			claudeStatus = detectedStatus

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
				reason = ReasonIdle
				priority = PriorityLow
				shouldAdd = true
				context = "Session idle - ready for next task"
			}
		}

		// Check for uncommitted changes (informational - user may want to review and commit)
		// Only check if we don't already have a higher-priority reason
		if (!shouldAdd || priority == PriorityLow) && inst.HasGitWorktree() {
			worktree, err := inst.GetGitWorktree()
			if err != nil {
				log.WarningLog.Printf("[ReviewQueue] Session '%s': Failed to get git worktree: %v", inst.Title, err)
			} else if worktree != nil {
				isDirty, err := worktree.IsDirty()
				if err != nil {
					log.WarningLog.Printf("[ReviewQueue] Session '%s': Failed to check git status: %v", inst.Title, err)
					log.ForSession(inst.Title).Warning("Failed to check git status: %v", err)
				} else if isDirty {
					// Only override if we don't have a higher priority reason already
					if !shouldAdd || priority == PriorityLow {
						reason = ReasonUncommittedChanges
						priority = PriorityLow
						shouldAdd = true
						context = "Uncommitted changes ready to commit"
						log.InfoLog.Printf("[ReviewQueue] Session '%s': Uncommitted changes detected", inst.Title)
					}
				} else {
					// Worktree is clean — if session was queued solely for uncommitted
					// changes, remove it immediately so it doesn't persist through the
					// rate-limiter window after a commit.
					if existing, exists := rqp.queue.Get(inst.Title); exists && existing.Reason == ReasonUncommittedChanges {
						log.InfoLog.Printf("[ReviewQueue] Session '%s': Changes committed - removing UncommittedChanges entry", inst.Title)
						rqp.queue.Remove(inst.Title)
					}
				}
			}
		}
	}

	// LastMeaningfulOutput is updated by getContent() above via UpdateTerminalTimestamps()
	// when new terminal content is detected. The persisted content-signature dedup prevents
	// false positives: sessions stay snoozed after acknowledgment unless output genuinely changes.

	// Check for terminal staleness (no meaningful output for configured threshold)
	// This helps identify sessions that might be stuck or waiting without showing obvious idle state
	// IMPORTANT: Respect acknowledgment - don't flag as stale if user already acknowledged
	timeSinceOutput := inst.GetTimeSinceLastMeaningfulOutput()

	// Check if user has acknowledged this session after it became stale
	// If acknowledged after last output, don't re-flag as stale
	alreadyAcknowledged := inst.IsAcknowledgedAfterOutput()

	if timeSinceOutput > rqp.config.StalenessThreshold {
		if alreadyAcknowledged {
			if log.IsDebugEnabled() {
				log.DebugLog.Printf("[ReviewQueue] Session '%s': STALE but already acknowledged - skipping staleness flag",
					inst.Title)
			}
		} else {
			// Only override if we don't already have a higher-priority reason.
			// Only set stale if not already flagged with Medium priority or higher.
			if !shouldAdd || priority.IsLowerThan(PriorityMedium) {
				reason = ReasonStale
				priority = PriorityLow // Lower priority than approval/error, but should be reviewed
				shouldAdd = true
				context = fmt.Sprintf("No activity for %s - session may be stuck or waiting",
					detection.FormatDuration(timeSinceOutput))
				if log.IsDebugEnabled() {
					log.DebugLog.Printf("[ReviewQueue] Session '%s': STALENESS DETECTED - flagged as stale, %s since last meaningful output",
						inst.Title, detection.FormatDuration(timeSinceOutput))
				}
			} else if log.IsDebugEnabled() {
				log.DebugLog.Printf("[ReviewQueue] Session '%s': Stale but already has higher priority reason (%s)",
					inst.Title, reason.String())
			}
		}
	} else if log.IsDebugEnabled() {
		log.DebugLog.Printf("[ReviewQueue] Session '%s': NOT STALE - %s since last meaningful output (threshold: %s)",
			inst.Title, detection.FormatDuration(timeSinceOutput), detection.FormatDuration(rqp.config.StalenessThreshold))
	}

	// Acknowledgment snooze: applies to ALL sessions regardless of priority or controller state.
	// Sessions are snoozed when LastAcknowledged is newer than LastMeaningfulOutput.
	// When a live process generates a new prompt, LastMeaningfulOutput is updated, so
	// IsAcknowledgedAfterOutput() returns false and the session correctly resurfaces.
	if inst.IsAcknowledgedAfterOutput() {
		rqp.queue.Remove(inst.Title)
		return
	}

	// Grace period: Don't re-add for 5 minutes after acknowledgment, even with new output.
	// Scoped to low-priority or inactive-controller sessions only — high/medium priority sessions
	// with an active controller should resurface promptly when new output arrives.
	if !shouldAdd || priority == PriorityLow || !statusInfo.IsControllerActive {
		if !inst.LastAcknowledged.IsZero() {
			gracePeriod := 5 * time.Minute
			timeSinceAck := time.Since(inst.LastAcknowledged)
			if timeSinceAck < gracePeriod {
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
			// Populate idle state and raw detected status for WorkingState mapping.
			IdleState:    statusInfo.IdleState.State,
			ClaudeStatus: claudeStatus,
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
			}
		}

		if !isUpdate {
			log.InfoLog.Printf("[ReviewQueue] Session '%s': Successfully added to queue - %s (priority: %s, context: %s)",
				inst.Title, reason.String(), priority.String(), context)
		}
	} else {
		rqp.queue.Remove(inst.Title)
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
// Fetches a fresh pane activity snapshot for accurate cache invalidation.
func (rqp *ReviewQueuePoller) CheckSession(inst *Instance) {
	rqp.checkSession(inst, batchPaneActivity(""))
}

// FindInstance finds an instance by session ID (exported for ReactiveQueueManager).
// Returns nil if the instance is not found in the monitored list.
func (rqp *ReviewQueuePoller) FindInstance(sessionID string) *Instance {
	rqp.mu.RLock()
	defer rqp.mu.RUnlock()

	for _, inst := range rqp.instances {
		if inst.MatchesID(sessionID) {
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
