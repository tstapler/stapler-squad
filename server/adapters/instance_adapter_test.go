package adapters

import (
	"testing"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/detection"
	"github.com/tstapler/stapler-squad/session/detection/ratelimit"
)

func TestRateLimitStateToProto_AllStates(t *testing.T) {
	tests := []struct {
		name     string
		input    ratelimit.RateLimitState
		expected sessionv1.RateLimitState
	}{
		{"None", ratelimit.StateNone, sessionv1.RateLimitState_RATE_LIMIT_STATE_NONE},
		{"Waiting", ratelimit.StateWaiting, sessionv1.RateLimitState_RATE_LIMIT_STATE_WAITING},
		{"Recovering", ratelimit.StateRecovering, sessionv1.RateLimitState_RATE_LIMIT_STATE_RECOVERING},
		{"Recovered", ratelimit.StateRecovered, sessionv1.RateLimitState_RATE_LIMIT_STATE_RECOVERED},
		{"Failed", ratelimit.StateFailed, sessionv1.RateLimitState_RATE_LIMIT_STATE_FAILED},
		{"Unknown state defaults to None", ratelimit.RateLimitState(99), sessionv1.RateLimitState_RATE_LIMIT_STATE_NONE},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := rateLimitStateToProto(tc.input)
			if got != tc.expected {
				t.Errorf("rateLimitStateToProto(%v) = %v, want %v", tc.input, got, tc.expected)
			}
		})
	}
}

func TestInstanceToProto_NilReturnsNil(t *testing.T) {
	result := InstanceToProto(nil, nil)
	if result != nil {
		t.Error("expected nil for nil input, got non-nil")
	}
}

// TestInstanceToProto_RateLimitEnabled verifies that the RateLimitEnabled field
// is populated correctly from the Instance struct field.
func TestInstanceToProto_RateLimitEnabled_DefaultTrue(t *testing.T) {
	inst := &session.Instance{} // nil RateLimitAutoResume → defaults to true
	proto := InstanceToProto(inst, nil)
	if proto == nil {
		t.Fatal("expected non-nil proto for non-nil instance")
	}
	if !proto.RateLimitEnabled {
		t.Errorf("expected RateLimitEnabled=true (default), got false")
	}
}

func TestInstanceToProto_RateLimitEnabled_ExplicitFalse(t *testing.T) {
	disabled := false
	inst := &session.Instance{
		RateLimitAutoResume: &disabled,
	}
	proto := InstanceToProto(inst, nil)
	if proto == nil {
		t.Fatal("expected non-nil proto for non-nil instance")
	}
	if proto.RateLimitEnabled {
		t.Errorf("expected RateLimitEnabled=false when explicitly disabled, got true")
	}
}

func TestInstanceToProto_RateLimitState_DefaultNone(t *testing.T) {
	inst := &session.Instance{} // no controller → state is None
	proto := InstanceToProto(inst, nil)
	if proto == nil {
		t.Fatal("expected non-nil proto for non-nil instance")
	}
	if proto.RateLimitState != sessionv1.RateLimitState_RATE_LIMIT_STATE_NONE {
		t.Errorf("expected RATE_LIMIT_STATE_NONE for fresh instance, got %v", proto.RateLimitState)
	}
	if proto.RateLimitResetTime != nil {
		t.Errorf("expected nil RateLimitResetTime for fresh instance, got %v", proto.RateLimitResetTime)
	}
}

// ─── U-GO-35: TestInstanceToProto_includesGoalSummaryWhenSet ─────────────────

func TestInstanceToProto_includesGoalSummaryWhenSet(t *testing.T) {
	inst := &session.Instance{}
	goal := &session.SessionGoalData{
		UUID:        "goal-uuid",
		SessionUUID: "session-uuid",
		Goal:        "test goal",
		Status:      session.GoalStatusWorking,
		Tasks: []session.TaskNode{
			{ID: "t1", Title: "Task 1", Status: session.TaskStatusDone},
		},
	}
	inst.SetSessionGoalCached(goal)

	proto := InstanceToProto(inst, nil)
	if proto == nil {
		t.Fatal("expected non-nil proto")
	}
	if proto.Goal == nil {
		t.Fatal("expected Goal to be set when inst.SessionGoal is non-nil")
	}
	if proto.Goal.GoalText != "test goal" {
		t.Errorf("GoalText = %q, want %q", proto.Goal.GoalText, "test goal")
	}
	if proto.Goal.Status != session.GoalStatusWorking {
		t.Errorf("Status = %q, want %q", proto.Goal.Status, session.GoalStatusWorking)
	}
	if proto.Goal.TasksTotal != 1 {
		t.Errorf("TasksTotal = %d, want 1", proto.Goal.TasksTotal)
	}
	if proto.Goal.TasksDone != 1 {
		t.Errorf("TasksDone = %d, want 1", proto.Goal.TasksDone)
	}
	if proto.Goal.TasksJson == "" {
		t.Error("TasksJson should be non-empty when tasks are set")
	}
}

// TC-4: TestToProtoSubStatus_WaitingForAgent verifies that StatusWaitingForAgent
// maps to SUB_STATUS_PROCESSING in the toProtoSubStatus switch.
//
// Note: toProtoSubStatus reads DetectedStatus via inst.GetDetectedStatus(), which
// requires a running ClaudeController (not available in unit tests). The non-Active
// short-circuit path of toProtoSubStatus is verified here, and the StatusWaitingForAgent
// mapping is verified via subStatusFromItem (review_queue_adapter.go), which shares
// the same switch logic and is exercisable without a controller.
func TestToProtoSubStatus_WaitingForAgent(t *testing.T) {
	// Verify the non-Active short-circuit: a non-Active instance always returns UNSPECIFIED.
	inst := &session.Instance{Status: session.Paused}
	got := toProtoSubStatus(inst)
	if got != sessionv1.SubStatus_SUB_STATUS_UNSPECIFIED {
		t.Errorf("toProtoSubStatus(Paused) = %v, want SUB_STATUS_UNSPECIFIED", got)
	}

	// Verify the StatusWaitingForAgent → PROCESSING mapping via subStatusFromItem.
	// WorkingState derivation is now done client-side; WaitingForAgent maps to
	// SUB_STATUS_PROCESSING so the frontend deriveWorkingState() returns PROCESSING.
	item := &session.ReviewItem{ClaudeStatus: detection.StatusWaitingForAgent}
	gotSubStatus := subStatusFromItem(item)
	if gotSubStatus != sessionv1.SubStatus_SUB_STATUS_PROCESSING {
		t.Errorf("subStatusFromItem(StatusWaitingForAgent) = %v, want SUB_STATUS_PROCESSING", gotSubStatus)
	}

	// A fresh Active instance with no controller returns StatusUnknown → UNSPECIFIED,
	// confirming that any non-UNSPECIFIED result requires an explicit switch case.
	activeNoCtrl := &session.Instance{Status: session.Active}
	gotActive := toProtoSubStatus(activeNoCtrl)
	if gotActive != sessionv1.SubStatus_SUB_STATUS_UNSPECIFIED {
		// If this ever fires it means GetDetectedStatus() returned something other
		// than Unknown without a running controller — update this test accordingly.
		t.Logf("toProtoSubStatus(Active/no-controller) = %v (expected UNSPECIFIED)", gotActive)
	}
}

// ─── U-GO-36: TestInstanceToProto_omitsGoalSummaryWhenNil ─────────────────────

func TestStatusToProto_AllStates(t *testing.T) {
	tests := []struct {
		name     string
		input    session.Status
		expected sessionv1.SessionStatus
	}{
		{"Active", session.Active, sessionv1.SessionStatus_SESSION_STATUS_ACTIVE},
		{"Creating", session.Creating, sessionv1.SessionStatus_SESSION_STATUS_CREATING},
		{"Paused", session.Paused, sessionv1.SessionStatus_SESSION_STATUS_PAUSED},
		{"Stopped", session.Stopped, sessionv1.SessionStatus_SESSION_STATUS_STOPPED},
		{"Hibernated", session.Hibernated, sessionv1.SessionStatus_SESSION_STATUS_HIBERNATED},
		{"Restoring", session.Restoring, sessionv1.SessionStatus_SESSION_STATUS_RESTORING},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := StatusToProto(tc.input)
			if got != tc.expected {
				t.Errorf("StatusToProto(%v) = %v, want %v", tc.input, got, tc.expected)
			}
		})
	}
}

func TestProtoToStatus_AllStates(t *testing.T) {
	tests := []struct {
		name     string
		input    sessionv1.SessionStatus
		expected session.Status
	}{
		{"Active", sessionv1.SessionStatus_SESSION_STATUS_ACTIVE, session.Active},
		{"Creating", sessionv1.SessionStatus_SESSION_STATUS_CREATING, session.Creating},
		{"Paused", sessionv1.SessionStatus_SESSION_STATUS_PAUSED, session.Paused},
		{"Stopped", sessionv1.SessionStatus_SESSION_STATUS_STOPPED, session.Stopped},
		{"Hibernated", sessionv1.SessionStatus_SESSION_STATUS_HIBERNATED, session.Hibernated},
		{"Restoring", sessionv1.SessionStatus_SESSION_STATUS_RESTORING, session.Restoring},
		{"Unknown defaults to Creating", sessionv1.SessionStatus(99), session.Creating},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ProtoToStatus(tc.input)
			if got != tc.expected {
				t.Errorf("ProtoToStatus(%v) = %v, want %v", tc.input, got, tc.expected)
			}
		})
	}
}

func TestInstanceToProto_omitsGoalSummaryWhenNil(t *testing.T) {
	inst := &session.Instance{}
	// SessionGoal is nil by default.
	proto := InstanceToProto(inst, nil)
	if proto == nil {
		t.Fatal("expected non-nil proto")
	}
	if proto.Goal != nil {
		t.Errorf("expected Goal to be nil when inst.SessionGoal is nil, got %+v", proto.Goal)
	}
}
