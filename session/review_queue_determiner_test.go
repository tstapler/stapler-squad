package session

import (
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/session/detection"
)

// TestDefaultStatusDeterminer_Determine verifies the pure detection logic of DefaultStatusDeterminer.
// These tests run without a real tmux session: all inputs are constructed in-memory.
func TestDefaultStatusDeterminer_Determine(t *testing.T) {
	detector := detection.NewStatusDetector()
	determiner := NewDefaultStatusDeterminer(DefaultReviewQueuePollerConfig())

	tests := []struct {
		name         string
		content      string
		statusInfo   InstanceStatusInfo
		instSetup    func(inst *Instance) // optional additional setup on the instance
		wantAction   DetectionAction
		checkAction  bool // set to true to assert wantAction
		wantReason   AttentionReason
		wantPriority Priority
	}{
		// --- Controller-active path ---
		{
			name: "active_controller_needs_approval",
			statusInfo: InstanceStatusInfo{
				IsControllerActive: true,
				ClaudeStatus:       detection.StatusNeedsApproval,
			},
			checkAction:  true,
			wantAction:   DetectionActionAdd,
			wantReason:   ReasonApprovalPending,
			wantPriority: PriorityHigh,
		},
		{
			name: "active_controller_needs_approval_via_pending_approvals_count",
			statusInfo: InstanceStatusInfo{
				IsControllerActive: true,
				ClaudeStatus:       detection.StatusUnknown,
				PendingApprovals:   1,
			},
			checkAction:  true,
			wantAction:   DetectionActionAdd,
			wantReason:   ReasonApprovalPending,
			wantPriority: PriorityHigh,
		},
		{
			name: "active_controller_input_required",
			statusInfo: InstanceStatusInfo{
				IsControllerActive: true,
				ClaudeStatus:       detection.StatusInputRequired,
			},
			checkAction:  true,
			wantAction:   DetectionActionAdd,
			wantReason:   ReasonInputRequired,
			wantPriority: PriorityMedium,
		},
		{
			name: "active_controller_error",
			statusInfo: InstanceStatusInfo{
				IsControllerActive: true,
				ClaudeStatus:       detection.StatusError,
			},
			checkAction:  true,
			wantAction:   DetectionActionAdd,
			wantReason:   ReasonErrorState,
			wantPriority: PriorityUrgent,
		},
		{
			name: "active_controller_tests_failing",
			statusInfo: InstanceStatusInfo{
				IsControllerActive: true,
				ClaudeStatus:       detection.StatusTestsFailing,
			},
			checkAction:  true,
			wantAction:   DetectionActionAdd,
			wantReason:   ReasonTestsFailing,
			wantPriority: PriorityHigh,
		},
		{
			name: "active_controller_task_complete",
			statusInfo: InstanceStatusInfo{
				IsControllerActive: true,
				ClaudeStatus:       detection.StatusSuccess,
			},
			checkAction:  true,
			wantAction:   DetectionActionAdd,
			wantReason:   ReasonTaskComplete,
			wantPriority: PriorityLow,
		},
		{
			name: "active_controller_idle_active_returns_remove",
			statusInfo: InstanceStatusInfo{
				IsControllerActive: true,
				ClaudeStatus:       detection.StatusUnknown,
				IdleState:          detection.IdleStateInfo{State: detection.IdleStateActive},
			},
			checkAction: true,
			wantAction:  DetectionActionRemove,
		},
		{
			name: "active_controller_idle_timeout_adds_idle",
			statusInfo: InstanceStatusInfo{
				IsControllerActive: true,
				ClaudeStatus:       detection.StatusUnknown,
				IdleState:          detection.IdleStateInfo{State: detection.IdleStateTimeout},
			},
			checkAction:  true,
			wantAction:   DetectionActionAdd,
			wantReason:   ReasonIdle,
			wantPriority: PriorityLow,
		},
		{
			name: "active_controller_waiting_state_skips",
			statusInfo: InstanceStatusInfo{
				IsControllerActive: true,
				ClaudeStatus:       detection.StatusUnknown,
				IdleState:          detection.IdleStateInfo{State: detection.IdleStateWaiting},
			},
			// No stale output → DetectionActionSkip (unless staleness kicks in)
			// Use a fresh LastMeaningfulOutput so staleness doesn't fire.
			instSetup: func(inst *Instance) {
				inst.LastMeaningfulOutput = time.Now().Add(-10 * time.Second)
			},
			checkAction: true,
			wantAction:  DetectionActionSkip,
		},

		// --- No-controller path (terminal content detection) ---
		{
			name:    "no_controller_approval_in_terminal",
			content: "Yes, allow reading /etc/hosts\nYes, allow once",
			statusInfo: InstanceStatusInfo{
				IsControllerActive: false,
			},
			checkAction:  true,
			wantAction:   DetectionActionAdd,
			wantReason:   ReasonApprovalPending,
			wantPriority: PriorityHigh,
		},
		{
			name:    "no_controller_content_does_not_panic",
			content: "? Do you want to continue",
			statusInfo: InstanceStatusInfo{
				IsControllerActive: false,
			},
			// Smoke test: detection depends on patterns, just verify no panic.
			// checkAction is false — don't assert specific action.
			instSetup: func(inst *Instance) {
				inst.LastMeaningfulOutput = time.Now().Add(-1 * time.Second)
				inst.UpdatedAt = time.Now()
			},
		},
		{
			name:    "no_controller_generic_content_smoke_test",
			content: "Error: command not found",
			statusInfo: InstanceStatusInfo{
				IsControllerActive: false,
			},
			// Smoke test — verify no panic. checkAction is false.
			instSetup: func(inst *Instance) {
				inst.LastMeaningfulOutput = time.Now().Add(-1 * time.Second)
				inst.UpdatedAt = time.Now()
			},
		},
		{
			name:    "no_controller_active_status_smoke_test",
			content: "esc to interrupt\n⏺ Recording",
			statusInfo: InstanceStatusInfo{
				IsControllerActive: false,
			},
			// Smoke test — verify no panic. checkAction is false.
			instSetup: func(inst *Instance) {
				inst.LastMeaningfulOutput = time.Now().Add(-1 * time.Second)
			},
		},
		{
			name:    "no_controller_empty_content_idle_after_threshold",
			content: "",
			statusInfo: InstanceStatusInfo{
				IsControllerActive: false,
			},
			instSetup: func(inst *Instance) {
				// UpdatedAt in the past → triggers basicIdleThreshold
				inst.UpdatedAt = time.Now().Add(-10 * time.Second)
				inst.LastMeaningfulOutput = time.Now().Add(-10 * time.Second)
			},
			checkAction:  true,
			wantAction:   DetectionActionAdd,
			wantReason:   ReasonIdle,
			wantPriority: PriorityLow,
		},
		{
			name:    "no_controller_fresh_session_skip",
			content: "",
			statusInfo: InstanceStatusInfo{
				IsControllerActive: false,
			},
			instSetup: func(inst *Instance) {
				// UpdatedAt just now → below basicIdleThreshold, no staleness
				inst.UpdatedAt = time.Now()
				inst.LastMeaningfulOutput = time.Now()
			},
			checkAction: true,
			wantAction:  DetectionActionSkip,
		},

		// --- Staleness path ---
		{
			name:    "stale_session_adds_stale_reason",
			content: "",
			statusInfo: InstanceStatusInfo{
				IsControllerActive: false,
			},
			instSetup: func(inst *Instance) {
				// Set LastMeaningfulOutput far in the past (beyond StalenessThreshold=2m)
				inst.LastMeaningfulOutput = time.Now().Add(-10 * time.Minute)
				inst.UpdatedAt = time.Now().Add(-10 * time.Minute)
			},
			checkAction:  true,
			wantAction:   DetectionActionAdd,
			wantReason:   ReasonStale,
			wantPriority: PriorityLow,
		},
		{
			name:    "acknowledged_stale_session_skips_stale",
			content: "",
			statusInfo: InstanceStatusInfo{
				IsControllerActive: false,
			},
			instSetup: func(inst *Instance) {
				// Output is old, but user acknowledged AFTER output → stale flag suppressed
				inst.LastMeaningfulOutput = time.Now().Add(-10 * time.Minute)
				inst.UpdatedAt = time.Now().Add(-10 * time.Minute)
				inst.LastAcknowledged = time.Now().Add(-5 * time.Minute) // after output
			},
			// stale is suppressed; idle fires because UpdatedAt > 5s ago
			checkAction:  true,
			wantAction:   DetectionActionAdd,
			wantReason:   ReasonIdle,
			wantPriority: PriorityLow,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inst := &Instance{
				Title:  "test-session",
				UUID:   "test-uuid",
				Status: Running,
			}
			inst.started = true
			// Set a sensible default so staleness doesn't fire unexpectedly
			if inst.LastMeaningfulOutput.IsZero() {
				inst.LastMeaningfulOutput = time.Now().Add(-1 * time.Second)
			}
			if inst.UpdatedAt.IsZero() {
				inst.UpdatedAt = time.Now()
			}

			if tt.instSetup != nil {
				tt.instSetup(inst)
			}

			result := determiner.Determine(inst, tt.content, tt.statusInfo, detector)

			// checkAction is true when this test explicitly asserts Action.
			if tt.checkAction && result.Action != tt.wantAction {
				t.Errorf("Action: got %v, want %v", result.Action, tt.wantAction)
			}
			if tt.wantReason != "" && result.Reason != tt.wantReason {
				t.Errorf("Reason: got %v, want %v", result.Reason, tt.wantReason)
			}
			if tt.wantPriority != 0 && result.Priority != tt.wantPriority {
				t.Errorf("Priority: got %v, want %v", result.Priority, tt.wantPriority)
			}
		})
	}
}

// TestDefaultStatusDeterminer_ControllerStatusTakesPriorityOverIdleActive verifies
// that when a controller reports StatusNeedsApproval AND IdleStateActive simultaneously,
// the approval wins (idle-active is only checked when no status-based condition is set).
func TestDefaultStatusDeterminer_ControllerStatusTakesPriorityOverIdleActive(t *testing.T) {
	detector := detection.NewStatusDetector()
	determiner := NewDefaultStatusDeterminer(DefaultReviewQueuePollerConfig())

	inst := &Instance{Title: "test", UUID: "uuid", Status: Running}
	inst.started = true
	inst.LastMeaningfulOutput = time.Now().Add(-1 * time.Second)

	statusInfo := InstanceStatusInfo{
		IsControllerActive: true,
		ClaudeStatus:       detection.StatusNeedsApproval,
		IdleState:          detection.IdleStateInfo{State: detection.IdleStateActive},
	}

	result := determiner.Determine(inst, "", statusInfo, detector)

	if result.Action != DetectionActionAdd {
		t.Errorf("expected Add (approval takes priority over idle-active), got %v", result.Action)
	}
	if result.Reason != ReasonApprovalPending {
		t.Errorf("expected ReasonApprovalPending, got %v", result.Reason)
	}
}

// TestDefaultStatusDeterminer_StatusContextPassedThrough verifies that the StatusContext
// from InstanceStatusInfo is used as the queue item context when non-empty.
func TestDefaultStatusDeterminer_StatusContextPassedThrough(t *testing.T) {
	detector := detection.NewStatusDetector()
	determiner := NewDefaultStatusDeterminer(DefaultReviewQueuePollerConfig())

	inst := &Instance{Title: "test", UUID: "uuid", Status: Running}
	inst.started = true
	inst.LastMeaningfulOutput = time.Now().Add(-1 * time.Second)

	customContext := "tool use blocked by policy xyz"
	statusInfo := InstanceStatusInfo{
		IsControllerActive: true,
		ClaudeStatus:       detection.StatusNeedsApproval,
		StatusContext:      customContext,
	}

	result := determiner.Determine(inst, "", statusInfo, detector)

	if result.Context != customContext {
		t.Errorf("expected context %q, got %q", customContext, result.Context)
	}
}

// TestDefaultStatusDeterminer_NeedsApprovalDefaultContext verifies that when StatusContext
// is empty, a non-empty default context is used.
func TestDefaultStatusDeterminer_NeedsApprovalDefaultContext(t *testing.T) {
	detector := detection.NewStatusDetector()
	determiner := NewDefaultStatusDeterminer(DefaultReviewQueuePollerConfig())

	inst := &Instance{Title: "test", UUID: "uuid", Status: Running}
	inst.started = true
	inst.LastMeaningfulOutput = time.Now().Add(-1 * time.Second)

	statusInfo := InstanceStatusInfo{
		IsControllerActive: true,
		ClaudeStatus:       detection.StatusNeedsApproval,
		StatusContext:      "",
	}

	result := determiner.Determine(inst, "", statusInfo, detector)

	if result.Context == "" {
		t.Error("expected non-empty default context for StatusNeedsApproval")
	}
}

// TestDefaultStatusDeterminer_InputRequired verifies StatusInputRequired mapping.
func TestDefaultStatusDeterminer_InputRequired(t *testing.T) {
	detector := detection.NewStatusDetector()
	determiner := NewDefaultStatusDeterminer(DefaultReviewQueuePollerConfig())

	inst := &Instance{Title: "test", UUID: "uuid", Status: Running}
	inst.started = true
	inst.LastMeaningfulOutput = time.Now().Add(-1 * time.Second)

	statusInfo := InstanceStatusInfo{
		IsControllerActive: true,
		ClaudeStatus:       detection.StatusInputRequired,
	}

	result := determiner.Determine(inst, "", statusInfo, detector)

	if result.Action != DetectionActionAdd {
		t.Errorf("expected Add, got %v", result.Action)
	}
	if result.Reason != ReasonInputRequired {
		t.Errorf("expected ReasonInputRequired, got %v", result.Reason)
	}
	if result.Priority != PriorityMedium {
		t.Errorf("expected PriorityMedium, got %v", result.Priority)
	}
}

// TestDefaultStatusDeterminer_Error verifies StatusError mapping.
func TestDefaultStatusDeterminer_Error(t *testing.T) {
	detector := detection.NewStatusDetector()
	determiner := NewDefaultStatusDeterminer(DefaultReviewQueuePollerConfig())

	inst := &Instance{Title: "test", UUID: "uuid", Status: Running}
	inst.started = true
	inst.LastMeaningfulOutput = time.Now().Add(-1 * time.Second)

	statusInfo := InstanceStatusInfo{
		IsControllerActive: true,
		ClaudeStatus:       detection.StatusError,
	}

	result := determiner.Determine(inst, "", statusInfo, detector)

	if result.Action != DetectionActionAdd {
		t.Errorf("expected Add, got %v", result.Action)
	}
	if result.Reason != ReasonErrorState {
		t.Errorf("expected ReasonErrorState, got %v", result.Reason)
	}
	if result.Priority != PriorityUrgent {
		t.Errorf("expected PriorityUrgent, got %v", result.Priority)
	}
}

// TestDefaultStatusDeterminer_UnknownStatusWithNoIdleStateSkips verifies that
// StatusUnknown with no idle-state information and a fresh session results in Skip.
func TestDefaultStatusDeterminer_UnknownStatusWithNoIdleStateSkips(t *testing.T) {
	detector := detection.NewStatusDetector()
	determiner := NewDefaultStatusDeterminer(DefaultReviewQueuePollerConfig())

	inst := &Instance{Title: "test", UUID: "uuid", Status: Running}
	inst.started = true
	// Fresh output — below staleness threshold and idle threshold
	inst.LastMeaningfulOutput = time.Now().Add(-1 * time.Second)

	statusInfo := InstanceStatusInfo{
		IsControllerActive: true,
		ClaudeStatus:       detection.StatusUnknown,
		IdleState:          detection.IdleStateInfo{State: detection.IdleStateWaiting},
	}

	result := determiner.Determine(inst, "", statusInfo, detector)

	if result.Action != DetectionActionSkip {
		t.Errorf("expected Skip for fresh session with no actionable status, got %v", result.Action)
	}
}

// TestDefaultStatusDeterminer_NoControllerApprovalInTerminal is the no-controller analog
// of the regression test for the bug: approval content in terminal must be detected even
// without a controller.
func TestDefaultStatusDeterminer_NoControllerApprovalInTerminal(t *testing.T) {
	detector := detection.NewStatusDetector()
	determiner := NewDefaultStatusDeterminer(DefaultReviewQueuePollerConfig())

	inst := &Instance{Title: "test", UUID: "uuid", Status: Running}
	inst.started = true
	inst.LastMeaningfulOutput = time.Now().Add(-1 * time.Second)

	approvalContent := "Yes, allow reading /etc/hosts\nYes, allow once"
	statusInfo := InstanceStatusInfo{
		IsControllerActive: false,
	}

	result := determiner.Determine(inst, approvalContent, statusInfo, detector)

	if result.Action != DetectionActionAdd {
		t.Errorf("expected Add for approval content without controller, got %v", result.Action)
	}
	if result.Reason != ReasonApprovalPending {
		t.Errorf("expected ReasonApprovalPending, got %v", result.Reason)
	}
	if result.Priority != PriorityHigh {
		t.Errorf("expected PriorityHigh, got %v", result.Priority)
	}
}
