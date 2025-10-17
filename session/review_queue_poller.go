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
	PollInterval      time.Duration // How often to check sessions
	IdleThreshold     time.Duration // Duration before considering session idle and adding to queue
	InputWaitDuration time.Duration // Time waiting for input before flagging
}

// DefaultReviewQueuePollerConfig returns sensible defaults for polling.
func DefaultReviewQueuePollerConfig() ReviewQueuePollerConfig {
	return ReviewQueuePollerConfig{
		PollInterval:      2 * time.Second,  // Poll every 2 seconds for immediate detection
		IdleThreshold:     30 * time.Second, // Add to queue after 30s idle (reduced from 2min)
		InputWaitDuration: 5 * time.Second,  // Flag if waiting for input > 5s (reduced from 30s)
	}
}

// ReviewQueuePoller automatically monitors sessions and adds them to the review queue
// when they become idle or need attention.
type ReviewQueuePoller struct {
	queue          *ReviewQueue
	statusManager  *InstanceStatusManager
	instances      []*Instance
	config         ReviewQueuePollerConfig

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	mu     sync.RWMutex
}

// NewReviewQueuePoller creates a new poller for automatically managing the review queue.
func NewReviewQueuePoller(queue *ReviewQueue, statusManager *InstanceStatusManager) *ReviewQueuePoller {
	return NewReviewQueuePollerWithConfig(queue, statusManager, DefaultReviewQueuePollerConfig())
}

// NewReviewQueuePollerWithConfig creates a poller with custom configuration.
func NewReviewQueuePollerWithConfig(queue *ReviewQueue, statusManager *InstanceStatusManager, config ReviewQueuePollerConfig) *ReviewQueuePoller {
	return &ReviewQueuePoller{
		queue:         queue,
		statusManager: statusManager,
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
	// Get comprehensive status
	statusInfo := rqp.statusManager.GetStatus(inst)

	if !statusInfo.IsControllerActive {
		// No controller active, remove from queue if present
		rqp.queue.Remove(inst.Title)
		return
	}

	// Get controller for idle detection
	controller, exists := rqp.statusManager.GetController(inst.Title)
	if !exists || controller == nil {
		rqp.queue.Remove(inst.Title)
		return
	}

	// Get idle state
	idleState, lastActivity := controller.GetIdleState()

	// Determine if needs attention and why
	var reason AttentionReason
	var priority Priority
	var shouldAdd bool
	var context string

	switch idleState {
	case IdleStateActive:
		// Actively working, remove from queue
		rqp.queue.Remove(inst.Title)
		return

	case IdleStateWaiting:
		// Normal idle state (e.g., INSERT mode) - don't add to queue by default
		// Only add if there are specific issues (approval, error) checked below
		shouldAdd = false

	case IdleStateTimeout:
		// Definite timeout - been idle too long
		reason = ReasonIdleTimeout
		priority = PriorityLow
		shouldAdd = true
		idleDuration := time.Since(lastActivity)
		context = fmt.Sprintf("Timed out after %s of inactivity", formatDuration(idleDuration))

	default:
		// Unknown state, remove from queue
		rqp.queue.Remove(inst.Title)
		return
	}

	// Check for approval needs (higher priority than idle)
	if statusInfo.ClaudeStatus == StatusNeedsApproval || statusInfo.PendingApprovals > 0 {
		reason = ReasonApprovalPending
		priority = PriorityHigh
		shouldAdd = true
		context = "Waiting for approval to proceed"
	}

	// Check for errors (highest priority)
	if statusInfo.ClaudeStatus == StatusError {
		reason = ReasonErrorState
		priority = PriorityUrgent
		shouldAdd = true
		context = "Error state detected"
	}

	// Add or update in queue
	if shouldAdd {
		// Check if item already exists and preserve DetectedAt if status hasn't changed
		detectedAt := time.Now()
		if existingItem, exists := rqp.queue.Get(inst.Title); exists {
			// Preserve original timestamp if meaningful fields haven't changed
			if existingItem.Reason == reason &&
				existingItem.Priority == priority &&
				existingItem.Context == context {
				detectedAt = existingItem.DetectedAt
			}
		}

		item := &ReviewItem{
			SessionID:   inst.Title,
			SessionName: inst.Title,
			Reason:      reason,
			Priority:    priority,
			DetectedAt:  detectedAt,
			Context:     context,
		}
		rqp.queue.Add(item)

		log.DebugLog.Printf("Added session '%s' to review queue: %s (priority: %s)",
			inst.Title, reason.String(), priority.String())
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
