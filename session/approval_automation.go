package session

import (
	"context"
	"fmt"
	"github.com/tstapler/stapler-squad/session/detection"
	"sync"
	"time"
)

// ApprovalAutomation orchestrates automatic approval handling.
type ApprovalAutomation struct {
	sessionName   string
	detector      *detection.ApprovalDetector
	policyEngine  *PolicyEngine
	controller    *ClaudeController
	mu            sync.RWMutex
	ctx           context.Context
	cancel        context.CancelFunc
	running       bool
	approvalQueue []*PendingApproval
	queueMu       sync.Mutex
	subscribers   map[string]chan<- ApprovalEvent
	subMu         sync.RWMutex
}

// PendingApproval represents an approval request awaiting action.
type PendingApproval struct {
	Request      *detection.ApprovalRequest
	Decision     *PolicyDecision
	ReceivedAt   time.Time
	ExpiresAt    time.Time
	Status       PendingApprovalStatus
	UserResponse *detection.ApprovalResponse
}

// PendingApprovalStatus tracks the state of a pending approval.
type PendingApprovalStatus string

const (
	PendingStatusAwaiting  PendingApprovalStatus = "awaiting"
	PendingStatusProcessed PendingApprovalStatus = "processed"
	PendingStatusExpired   PendingApprovalStatus = "expired"
	PendingStatusCancelled PendingApprovalStatus = "cancelled"
)

// ApprovalEvent represents an event in the approval automation system.
type ApprovalEvent struct {
	Type      ApprovalEventType
	Request   *detection.ApprovalRequest
	Decision  *PolicyDecision
	Timestamp time.Time
	Details   string
}

// ApprovalEventType categorizes approval events.
type ApprovalEventType string

const (
	EventDetected      ApprovalEventType = "detected"
	EventAutoApproved  ApprovalEventType = "auto_approved"
	EventAutoRejected  ApprovalEventType = "auto_rejected"
	EventAwaitingUser  ApprovalEventType = "awaiting_user"
	EventUserApproved  ApprovalEventType = "user_approved"
	EventUserRejected  ApprovalEventType = "user_rejected"
	EventExpired       ApprovalEventType = "expired"
	EventExecuted      ApprovalEventType = "executed"
	EventExecutionFail ApprovalEventType = "execution_failed"
)

// ApprovalAutomationOptions configures approval automation behavior.
type ApprovalAutomationOptions struct {
	AutoExecute     bool          // Automatically execute approved commands
	UserTimeout     time.Duration // Time to wait for user response
	ProcessingDelay time.Duration // Delay between processing approvals
	MaxQueueSize    int           // Maximum pending approvals
	EnableAuditLog  bool          // Log all approval actions
}

// DefaultApprovalAutomationOptions returns sensible defaults.
func DefaultApprovalAutomationOptions() ApprovalAutomationOptions {
	return ApprovalAutomationOptions{
		AutoExecute:     true,
		UserTimeout:     5 * time.Minute,
		ProcessingDelay: 100 * time.Millisecond,
		MaxQueueSize:    100,
		EnableAuditLog:  true,
	}
}

// NewApprovalAutomation creates a new approval automation system.
func NewApprovalAutomation(sessionName string, controller *ClaudeController) *ApprovalAutomation {
	return &ApprovalAutomation{
		sessionName:   sessionName,
		detector:      detection.NewApprovalDetector(),
		policyEngine:  NewPolicyEngine(),
		controller:    controller,
		approvalQueue: make([]*PendingApproval, 0),
		subscribers:   make(map[string]chan<- ApprovalEvent),
	}
}

// Start begins the approval automation processing loop.
func (aa *ApprovalAutomation) Start(ctx context.Context, options ApprovalAutomationOptions) error {
	aa.mu.Lock()
	defer aa.mu.Unlock()

	if aa.running {
		return fmt.Errorf("approval automation already running")
	}

	aa.ctx, aa.cancel = context.WithCancel(ctx)
	aa.running = true

	// Subscribe to response stream for detection
	responseCh, err := aa.controller.Subscribe("approval-automation")
	if err != nil {
		return fmt.Errorf("failed to subscribe to response stream: %w", err)
	}

	// Start processing goroutines
	go aa.processResponseStream(responseCh, options)
	go aa.processApprovalQueue(options)

	return nil
}

// Stop halts the approval automation system.
func (aa *ApprovalAutomation) Stop() error {
	aa.mu.Lock()
	defer aa.mu.Unlock()

	if !aa.running {
		return fmt.Errorf("approval automation not running")
	}

	if aa.cancel != nil {
		aa.cancel()
	}

	aa.controller.Unsubscribe("approval-automation")
	aa.running = false

	return nil
}

// processResponseStream monitors the response stream for approval patterns.
func (aa *ApprovalAutomation) processResponseStream(responseCh <-chan ResponseChunk, options ApprovalAutomationOptions) {
	for {
		select {
		case <-aa.ctx.Done():
			return

		case chunk, ok := <-responseCh:
			if !ok {
				return
			}

			// Detect approval requests in the chunk
			request := aa.detector.DetectInChunk(chunk.Data, chunk.Error)
			if request == nil {
				continue
			}

			// Evaluate against policies
			decision, err := aa.policyEngine.Evaluate(request)
			if err != nil {
				continue
			}

			// Emit detection event
			aa.emitEvent(ApprovalEvent{
				Type:      EventDetected,
				Request:   request,
				Decision:  decision,
				Timestamp: time.Now(),
				Details:   fmt.Sprintf("Detected %s approval request", request.Type),
			})

			// Handle based on policy decision
			aa.handlePolicyDecision(request, decision, options)
		}
	}
}

// handlePolicyDecision processes a policy decision for an approval request.
func (aa *ApprovalAutomation) handlePolicyDecision(request *detection.ApprovalRequest, decision *PolicyDecision, options ApprovalAutomationOptions) {
	switch decision.Decision {
	case ActionAutoApprove:
		aa.handleAutoApprove(request, decision, options)

	case ActionAutoReject:
		aa.handleAutoReject(request, decision)

	case ActionPrompt:
		aa.handlePromptUser(request, decision, options)

	case ActionLog:
		// Just log, no action
		aa.emitEvent(ApprovalEvent{
			Type:      EventDetected,
			Request:   request,
			Decision:  decision,
			Timestamp: time.Now(),
			Details:   "Logged only, no action taken",
		})
	}
}

// handleAutoApprove automatically approves and optionally executes a request.
func (aa *ApprovalAutomation) handleAutoApprove(request *detection.ApprovalRequest, decision *PolicyDecision, options ApprovalAutomationOptions) {
	// Update request status
	response := &detection.ApprovalResponse{
		Approved:  true,
		Timestamp: time.Now(),
		UserInput: fmt.Sprintf("Auto-approved by policy: %s", decision.MatchedPolicy.Name),
	}
	aa.detector.UpdateRequestStatus(request.ID, detection.ApprovalApproved, response)

	// Emit auto-approval event
	aa.emitEvent(ApprovalEvent{
		Type:      EventAutoApproved,
		Request:   request,
		Decision:  decision,
		Timestamp: time.Now(),
		Details:   response.UserInput,
	})

	// Execute if enabled and applicable
	if options.AutoExecute && aa.canExecute(request) {
		aa.executeApproval(request, decision)
	}
}

// handleAutoReject automatically rejects a request.
func (aa *ApprovalAutomation) handleAutoReject(request *detection.ApprovalRequest, decision *PolicyDecision) {
	// Update request status
	response := &detection.ApprovalResponse{
		Approved:  false,
		Timestamp: time.Now(),
		UserInput: fmt.Sprintf("Auto-rejected by policy: %s", decision.MatchedPolicy.Name),
	}
	aa.detector.UpdateRequestStatus(request.ID, detection.ApprovalRejected, response)

	// Emit auto-rejection event
	aa.emitEvent(ApprovalEvent{
		Type:      EventAutoRejected,
		Request:   request,
		Decision:  decision,
		Timestamp: time.Now(),
		Details:   response.UserInput,
	})
}

// handlePromptUser adds a request to the queue for user action.
func (aa *ApprovalAutomation) handlePromptUser(request *detection.ApprovalRequest, decision *PolicyDecision, options ApprovalAutomationOptions) {
	pending := &PendingApproval{
		Request:    request,
		Decision:   decision,
		ReceivedAt: time.Now(),
		ExpiresAt:  time.Now().Add(options.UserTimeout),
		Status:     PendingStatusAwaiting,
	}

	aa.queueMu.Lock()
	defer aa.queueMu.Unlock()

	// Enforce max queue size
	if options.MaxQueueSize > 0 && len(aa.approvalQueue) >= options.MaxQueueSize {
		// Remove oldest
		aa.approvalQueue = aa.approvalQueue[1:]
	}

	aa.approvalQueue = append(aa.approvalQueue, pending)

	// Emit awaiting user event
	aa.emitEvent(ApprovalEvent{
		Type:      EventAwaitingUser,
		Request:   request,
		Decision:  decision,
		Timestamp: time.Now(),
		Details:   "Awaiting user response",
	})
}

// processApprovalQueue handles pending approvals and timeouts.
func (aa *ApprovalAutomation) processApprovalQueue(options ApprovalAutomationOptions) {
	ticker := time.NewTicker(options.ProcessingDelay)
	defer ticker.Stop()

	for {
		select {
		case <-aa.ctx.Done():
			return

		case <-ticker.C:
			aa.checkExpiredApprovals()
		}
	}
}

// checkExpiredApprovals marks expired approvals and removes them from queue.
func (aa *ApprovalAutomation) checkExpiredApprovals() {
	aa.queueMu.Lock()
	defer aa.queueMu.Unlock()

	now := time.Now()
	remaining := make([]*PendingApproval, 0, len(aa.approvalQueue))

	for _, pending := range aa.approvalQueue {
		if pending.Status != PendingStatusAwaiting {
			continue
		}

		if now.After(pending.ExpiresAt) {
			// Mark as expired
			pending.Status = PendingStatusExpired
			aa.detector.UpdateRequestStatus(pending.Request.ID, detection.ApprovalExpired, nil)

			// Emit expired event
			aa.emitEvent(ApprovalEvent{
				Type:      EventExpired,
				Request:   pending.Request,
				Decision:  pending.Decision,
				Timestamp: time.Now(),
				Details:   "User approval timeout",
			})
		} else {
			remaining = append(remaining, pending)
		}
	}

	aa.approvalQueue = remaining
}

// RespondToApproval processes a user response to a pending approval.
func (aa *ApprovalAutomation) RespondToApproval(requestID string, approved bool, userInput string, options ApprovalAutomationOptions) error {
	aa.queueMu.Lock()
	defer aa.queueMu.Unlock()

	for i, pending := range aa.approvalQueue {
		if pending.Request.ID == requestID && pending.Status == PendingStatusAwaiting {
			// Update response
			pending.UserResponse = &detection.ApprovalResponse{
				Approved:  approved,
				Timestamp: time.Now(),
				UserInput: userInput,
			}
			pending.Status = PendingStatusProcessed

			// Update detector status
			status := detection.ApprovalApproved
			if !approved {
				status = detection.ApprovalRejected
			}
			aa.detector.UpdateRequestStatus(requestID, status, pending.UserResponse)

			// Emit event
			eventType := EventUserApproved
			if !approved {
				eventType = EventUserRejected
			}
			aa.emitEvent(ApprovalEvent{
				Type:      eventType,
				Request:   pending.Request,
				Decision:  pending.Decision,
				Timestamp: time.Now(),
				Details:   userInput,
			})

			// Execute if approved and auto-execute enabled
			if approved && options.AutoExecute && aa.canExecute(pending.Request) {
				aa.executeApproval(pending.Request, pending.Decision)
			}

			// Remove from queue
			aa.approvalQueue = append(aa.approvalQueue[:i], aa.approvalQueue[i+1:]...)

			return nil
		}
	}

	return fmt.Errorf("approval request '%s' not found or already processed", requestID)
}

// canExecute determines if an approval request can be executed.
func (aa *ApprovalAutomation) canExecute(request *detection.ApprovalRequest) bool {
	// Only command approvals can be executed
	return request.Type == detection.ApprovalCommand
}

// executeApproval executes an approved command.
func (aa *ApprovalAutomation) executeApproval(request *detection.ApprovalRequest, decision *PolicyDecision) {
	// Extract command from request
	command, ok := request.ExtractedData["command"]
	if !ok {
		aa.emitEvent(ApprovalEvent{
			Type:      EventExecutionFail,
			Request:   request,
			Decision:  decision,
			Timestamp: time.Now(),
			Details:   "No command found in approval request",
		})
		return
	}

	// Send command via controller
	_, err := aa.controller.SendCommand(command, 100) // High priority
	if err != nil {
		aa.emitEvent(ApprovalEvent{
			Type:      EventExecutionFail,
			Request:   request,
			Decision:  decision,
			Timestamp: time.Now(),
			Details:   fmt.Sprintf("Failed to send command: %v", err),
		})
		return
	}

	// Emit execution event
	aa.emitEvent(ApprovalEvent{
		Type:      EventExecuted,
		Request:   request,
		Decision:  decision,
		Timestamp: time.Now(),
		Details:   fmt.Sprintf("Executed command: %s", command),
	})
}

// GetPendingApprovals returns all approvals awaiting user response.
func (aa *ApprovalAutomation) GetPendingApprovals() []*PendingApproval {
	aa.queueMu.Lock()
	defer aa.queueMu.Unlock()

	result := make([]*PendingApproval, 0)
	for _, pending := range aa.approvalQueue {
		if pending.Status == PendingStatusAwaiting {
			result = append(result, pending)
		}
	}

	return result
}

// GetDetector returns the approval detector for configuration.
func (aa *ApprovalAutomation) GetDetector() *detection.ApprovalDetector {
	return aa.detector
}

// GetPolicyEngine returns the policy engine for configuration.
func (aa *ApprovalAutomation) GetPolicyEngine() *PolicyEngine {
	return aa.policyEngine
}

// Subscribe creates a subscription for approval events.
func (aa *ApprovalAutomation) Subscribe(subscriberID string) <-chan ApprovalEvent {
	aa.subMu.Lock()
	defer aa.subMu.Unlock()

	ch := make(chan ApprovalEvent, 100)
	aa.subscribers[subscriberID] = ch

	return ch
}

// Unsubscribe removes a subscription.
func (aa *ApprovalAutomation) Unsubscribe(subscriberID string) {
	aa.subMu.Lock()
	defer aa.subMu.Unlock()

	if ch, exists := aa.subscribers[subscriberID]; exists {
		close(ch)
		delete(aa.subscribers, subscriberID)
	}
}

// emitEvent sends an event to all subscribers.
func (aa *ApprovalAutomation) emitEvent(event ApprovalEvent) {
	aa.subMu.RLock()
	defer aa.subMu.RUnlock()

	for _, ch := range aa.subscribers {
		select {
		case ch <- event:
		default:
			// Don't block if subscriber is slow
		}
	}
}

// IsRunning returns whether the automation is currently running.
func (aa *ApprovalAutomation) IsRunning() bool {
	aa.mu.RLock()
	defer aa.mu.RUnlock()
	return aa.running
}

// GetSessionName returns the session name.
func (aa *ApprovalAutomation) GetSessionName() string {
	return aa.sessionName
}
