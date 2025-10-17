package session

import (
	"fmt"
	"testing"
	"time"
)

func TestNewApprovalAutomation(t *testing.T) {
	instance := &Instance{Title: "test-session"}
	controller, _ := NewClaudeController(instance)

	automation := NewApprovalAutomation("test-session", controller)

	if automation == nil {
		t.Fatal("NewApprovalAutomation() returned nil")
	}

	if automation.GetSessionName() != "test-session" {
		t.Errorf("Session name = %q, expected %q", automation.GetSessionName(), "test-session")
	}

	if automation.IsRunning() {
		t.Error("Automation should not be running initially")
	}
}

func TestApprovalAutomation_GetDetector(t *testing.T) {
	instance := &Instance{Title: "test-session"}
	controller, _ := NewClaudeController(instance)
	automation := NewApprovalAutomation("test-session", controller)

	detector := automation.GetDetector()
	if detector == nil {
		t.Error("GetDetector() returned nil")
	}
}

func TestApprovalAutomation_GetPolicyEngine(t *testing.T) {
	instance := &Instance{Title: "test-session"}
	controller, _ := NewClaudeController(instance)
	automation := NewApprovalAutomation("test-session", controller)

	engine := automation.GetPolicyEngine()
	if engine == nil {
		t.Error("GetPolicyEngine() returned nil")
	}
}

func TestApprovalAutomation_GetPendingApprovals(t *testing.T) {
	instance := &Instance{Title: "test-session"}
	controller, _ := NewClaudeController(instance)
	automation := NewApprovalAutomation("test-session", controller)

	pending := automation.GetPendingApprovals()
	if len(pending) != 0 {
		t.Errorf("Expected 0 pending approvals, got %d", len(pending))
	}
}

func TestApprovalAutomation_Subscribe(t *testing.T) {
	instance := &Instance{Title: "test-session"}
	controller, _ := NewClaudeController(instance)
	automation := NewApprovalAutomation("test-session", controller)

	ch := automation.Subscribe("test-subscriber")
	if ch == nil {
		t.Fatal("Subscribe() returned nil channel")
	}

	automation.Unsubscribe("test-subscriber")
}

func TestApprovalAutomation_Unsubscribe(t *testing.T) {
	instance := &Instance{Title: "test-session"}
	controller, _ := NewClaudeController(instance)
	automation := NewApprovalAutomation("test-session", controller)

	ch := automation.Subscribe("test-subscriber")
	automation.Unsubscribe("test-subscriber")

	// Channel should be closed
	_, ok := <-ch
	if ok {
		t.Error("Channel should be closed after unsubscribe")
	}
}

func TestApprovalAutomation_EmitEvent(t *testing.T) {
	instance := &Instance{Title: "test-session"}
	controller, _ := NewClaudeController(instance)
	automation := NewApprovalAutomation("test-session", controller)

	ch := automation.Subscribe("test-subscriber")

	event := ApprovalEvent{
		Type:      EventDetected,
		Timestamp: time.Now(),
		Details:   "Test event",
	}

	go automation.emitEvent(event)

	select {
	case received := <-ch:
		if received.Type != EventDetected {
			t.Errorf("Event type = %v, expected EventDetected", received.Type)
		}
	case <-time.After(1 * time.Second):
		t.Error("Timed out waiting for event")
	}

	automation.Unsubscribe("test-subscriber")
}

func TestApprovalAutomation_RespondToApprovalNotFound(t *testing.T) {
	instance := &Instance{Title: "test-session"}
	controller, _ := NewClaudeController(instance)
	automation := NewApprovalAutomation("test-session", controller)

	options := DefaultApprovalAutomationOptions()
	err := automation.RespondToApproval("nonexistent", true, "test", options)
	if err == nil {
		t.Error("RespondToApproval() should fail for nonexistent request")
	}
}

func TestApprovalAutomation_HandlePromptUser(t *testing.T) {
	instance := &Instance{Title: "test-session"}
	controller, _ := NewClaudeController(instance)
	automation := NewApprovalAutomation("test-session", controller)

	request := &ApprovalRequest{
		ID:   "test-request",
		Type: ApprovalCommand,
	}

	decision := &PolicyDecision{
		Decision: ActionPrompt,
	}

	options := DefaultApprovalAutomationOptions()
	automation.handlePromptUser(request, decision, options)

	pending := automation.GetPendingApprovals()
	if len(pending) != 1 {
		t.Errorf("Expected 1 pending approval, got %d", len(pending))
	}

	if pending[0].Request.ID != "test-request" {
		t.Errorf("Pending approval ID = %q, expected %q", pending[0].Request.ID, "test-request")
	}
}

func TestApprovalAutomation_RespondToApprovalApprove(t *testing.T) {
	instance := &Instance{Title: "test-session"}
	controller, _ := NewClaudeController(instance)
	automation := NewApprovalAutomation("test-session", controller)

	request := &ApprovalRequest{
		ID:   "test-request",
		Type: ApprovalCommand,
	}

	decision := &PolicyDecision{
		Decision: ActionPrompt,
	}

	options := DefaultApprovalAutomationOptions()
	options.AutoExecute = false // Don't try to execute

	automation.handlePromptUser(request, decision, options)

	err := automation.RespondToApproval("test-request", true, "approved by user", options)
	if err != nil {
		t.Fatalf("RespondToApproval() failed: %v", err)
	}

	// Should be removed from queue
	pending := automation.GetPendingApprovals()
	if len(pending) != 0 {
		t.Errorf("Expected 0 pending approvals after response, got %d", len(pending))
	}
}

func TestApprovalAutomation_RespondToApprovalReject(t *testing.T) {
	instance := &Instance{Title: "test-session"}
	controller, _ := NewClaudeController(instance)
	automation := NewApprovalAutomation("test-session", controller)

	request := &ApprovalRequest{
		ID:   "test-request",
		Type: ApprovalCommand,
	}

	decision := &PolicyDecision{
		Decision: ActionPrompt,
	}

	options := DefaultApprovalAutomationOptions()
	automation.handlePromptUser(request, decision, options)

	err := automation.RespondToApproval("test-request", false, "rejected by user", options)
	if err != nil {
		t.Fatalf("RespondToApproval() failed: %v", err)
	}

	pending := automation.GetPendingApprovals()
	if len(pending) != 0 {
		t.Errorf("Expected 0 pending approvals after response, got %d", len(pending))
	}
}

func TestApprovalAutomation_HandleAutoApprove(t *testing.T) {
	instance := &Instance{Title: "test-session"}
	controller, _ := NewClaudeController(instance)
	automation := NewApprovalAutomation("test-session", controller)

	ch := automation.Subscribe("test-subscriber")
	defer automation.Unsubscribe("test-subscriber")

	request := &ApprovalRequest{
		ID:   "test-request",
		Type: ApprovalCommand,
	}

	decision := &PolicyDecision{
		Decision: ActionAutoApprove,
		MatchedPolicy: &ApprovalPolicy{
			Name: "test-policy",
		},
	}

	options := DefaultApprovalAutomationOptions()
	options.AutoExecute = false // Don't try to execute

	go automation.handleAutoApprove(request, decision, options)

	// Should receive auto-approve event
	select {
	case event := <-ch:
		if event.Type != EventAutoApproved {
			t.Errorf("Event type = %v, expected EventAutoApproved", event.Type)
		}
	case <-time.After(1 * time.Second):
		t.Error("Timed out waiting for auto-approve event")
	}
}

func TestApprovalAutomation_HandleAutoReject(t *testing.T) {
	instance := &Instance{Title: "test-session"}
	controller, _ := NewClaudeController(instance)
	automation := NewApprovalAutomation("test-session", controller)

	ch := automation.Subscribe("test-subscriber")
	defer automation.Unsubscribe("test-subscriber")

	request := &ApprovalRequest{
		ID:   "test-request",
		Type: ApprovalCommand,
	}

	decision := &PolicyDecision{
		Decision: ActionAutoReject,
		MatchedPolicy: &ApprovalPolicy{
			Name: "test-policy",
		},
	}

	go automation.handleAutoReject(request, decision)

	// Should receive auto-reject event
	select {
	case event := <-ch:
		if event.Type != EventAutoRejected {
			t.Errorf("Event type = %v, expected EventAutoRejected", event.Type)
		}
	case <-time.After(1 * time.Second):
		t.Error("Timed out waiting for auto-reject event")
	}
}

func TestApprovalAutomation_CanExecute(t *testing.T) {
	instance := &Instance{Title: "test-session"}
	controller, _ := NewClaudeController(instance)
	automation := NewApprovalAutomation("test-session", controller)

	// Command type can be executed
	commandRequest := &ApprovalRequest{
		Type: ApprovalCommand,
	}
	if !automation.canExecute(commandRequest) {
		t.Error("Command type should be executable")
	}

	// File write type cannot be executed
	fileRequest := &ApprovalRequest{
		Type: ApprovalFileWrite,
	}
	if automation.canExecute(fileRequest) {
		t.Error("File write type should not be executable")
	}
}

func TestApprovalAutomation_CheckExpiredApprovals(t *testing.T) {
	instance := &Instance{Title: "test-session"}
	controller, _ := NewClaudeController(instance)
	automation := NewApprovalAutomation("test-session", controller)

	ch := automation.Subscribe("test-subscriber")
	defer automation.Unsubscribe("test-subscriber")

	// Add pending approval with past expiry
	pending := &PendingApproval{
		Request: &ApprovalRequest{
			ID:   "expired-request",
			Type: ApprovalCommand,
		},
		Status:     PendingStatusAwaiting,
		ReceivedAt: time.Now().Add(-10 * time.Minute),
		ExpiresAt:  time.Now().Add(-5 * time.Minute), // Already expired
	}

	automation.queueMu.Lock()
	automation.approvalQueue = append(automation.approvalQueue, pending)
	automation.queueMu.Unlock()

	// Check for expired approvals
	go automation.checkExpiredApprovals()

	// Should receive expired event
	select {
	case event := <-ch:
		if event.Type != EventExpired {
			t.Errorf("Event type = %v, expected EventExpired", event.Type)
		}
	case <-time.After(1 * time.Second):
		t.Error("Timed out waiting for expired event")
	}

	// Approval should be removed from queue
	remainingPending := automation.GetPendingApprovals()
	if len(remainingPending) != 0 {
		t.Errorf("Expected 0 pending approvals after expiry, got %d", len(remainingPending))
	}
}

func TestApprovalAutomation_MaxQueueSize(t *testing.T) {
	instance := &Instance{Title: "test-session"}
	controller, _ := NewClaudeController(instance)
	automation := NewApprovalAutomation("test-session", controller)

	options := DefaultApprovalAutomationOptions()
	options.MaxQueueSize = 3

	decision := &PolicyDecision{
		Decision: ActionPrompt,
	}

	// Add 5 requests (should only keep last 3)
	for i := 0; i < 5; i++ {
		request := &ApprovalRequest{
			ID:   fmt.Sprintf("request-%d", i),
			Type: ApprovalCommand,
		}
		automation.handlePromptUser(request, decision, options)
	}

	pending := automation.GetPendingApprovals()
	if len(pending) != 3 {
		t.Errorf("Expected 3 pending approvals (max queue size), got %d", len(pending))
	}

	// Should have most recent 3
	if pending[0].Request.ID != "request-2" {
		t.Errorf("First pending ID = %q, expected request-2", pending[0].Request.ID)
	}
}

func TestDefaultApprovalAutomationOptions(t *testing.T) {
	options := DefaultApprovalAutomationOptions()

	if !options.AutoExecute {
		t.Error("AutoExecute should be true by default")
	}

	if options.UserTimeout <= 0 {
		t.Error("UserTimeout should be positive")
	}

	if options.ProcessingDelay <= 0 {
		t.Error("ProcessingDelay should be positive")
	}

	if options.MaxQueueSize <= 0 {
		t.Error("MaxQueueSize should be positive")
	}

	if !options.EnableAuditLog {
		t.Error("EnableAuditLog should be true by default")
	}
}

func TestPendingApproval_Fields(t *testing.T) {
	request := &ApprovalRequest{
		ID:   "test-request",
		Type: ApprovalCommand,
	}

	decision := &PolicyDecision{
		Decision: ActionPrompt,
	}

	pending := &PendingApproval{
		Request:    request,
		Decision:   decision,
		ReceivedAt: time.Now(),
		ExpiresAt:  time.Now().Add(5 * time.Minute),
		Status:     PendingStatusAwaiting,
	}

	if pending.Request.ID != "test-request" {
		t.Error("Request not set correctly")
	}

	if pending.Status != PendingStatusAwaiting {
		t.Error("Status not set correctly")
	}

	if pending.ExpiresAt.Before(pending.ReceivedAt) {
		t.Error("ExpiresAt should be after ReceivedAt")
	}
}

func TestApprovalEvent_Fields(t *testing.T) {
	event := ApprovalEvent{
		Type:      EventDetected,
		Timestamp: time.Now(),
		Details:   "Test event details",
	}

	if event.Type != EventDetected {
		t.Error("Event type not set correctly")
	}

	if event.Timestamp.IsZero() {
		t.Error("Timestamp should be set")
	}

	if event.Details == "" {
		t.Error("Details should be set")
	}
}

func Benchmark_ApprovalAutomation_EmitEvent(b *testing.B) {
	instance := &Instance{Title: "test-session"}
	controller, _ := NewClaudeController(instance)
	automation := NewApprovalAutomation("test-session", controller)

	// Create subscriber
	automation.Subscribe("bench-subscriber")

	event := ApprovalEvent{
		Type:      EventDetected,
		Timestamp: time.Now(),
		Details:   "Benchmark event",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		automation.emitEvent(event)
	}
}
