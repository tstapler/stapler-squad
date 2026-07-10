package session

import (
	"testing"

	"github.com/tstapler/stapler-squad/session/detection"
)

// TestAttentionReasonFromDetected verifies every DetectedStatus maps to the expected
// reason, priority, and a non-empty default context (empty context only for the
// no-attention-needed states).
func TestAttentionReasonFromDetected(t *testing.T) {
	tests := []struct {
		detected     detection.DetectedStatus
		want         AttentionReason
		wantPriority Priority
	}{
		{detection.StatusNeedsApproval, ReasonApprovalPending, PriorityHigh},
		{detection.StatusInputRequired, ReasonInputRequired, PriorityMedium},
		{detection.StatusError, ReasonErrorState, PriorityUrgent},
		{detection.StatusTestsFailing, ReasonTestsFailing, PriorityHigh},
		{detection.StatusSuccess, ReasonTaskComplete, PriorityLow},
		{detection.StatusIdle, ReasonIdle, PriorityLow},
		// States that do not require attention
		{detection.StatusExecuting, "", 0},
		{detection.StatusProcessing, "", 0},
		{detection.StatusWaitingForAgent, "", 0},
		{detection.StatusReady, "", 0},
		{detection.StatusUnknown, "", 0},
	}

	for _, tt := range tests {
		t.Run(tt.detected.String(), func(t *testing.T) {
			got, gotPriority, gotContext := AttentionReasonFromDetected(tt.detected)
			if got != tt.want {
				t.Errorf("AttentionReasonFromDetected(%s) reason = %q, want %q",
					tt.detected, got, tt.want)
			}
			if gotPriority != tt.wantPriority {
				t.Errorf("AttentionReasonFromDetected(%s) priority = %v, want %v",
					tt.detected, gotPriority, tt.wantPriority)
			}
			if tt.want == "" {
				if gotContext != "" {
					t.Errorf("AttentionReasonFromDetected(%s) context = %q, want empty",
						tt.detected, gotContext)
				}
			} else if gotContext == "" {
				t.Errorf("AttentionReasonFromDetected(%s) context is empty, want non-empty default context",
					tt.detected)
			}
		})
	}
}

// TestStatusFromDetected verifies every DetectedStatus maps to Active.
// In the 5-state model, all detected states indicate an active process —
// NeedsApproval, InputRequired, Error, etc. are sub-status signals that do
// not change the lifecycle state.
func TestStatusFromDetected(t *testing.T) {
	tests := []struct {
		detected detection.DetectedStatus
		want     Status
	}{
		{detection.StatusReady, Active},
		{detection.StatusIdle, Active},
		{detection.StatusSuccess, Active},
		{detection.StatusProcessing, Active},
		{detection.StatusExecuting, Active},
		{detection.StatusWaitingForAgent, Active},
		{detection.StatusNeedsApproval, Active},
		{detection.StatusInputRequired, Active},
		{detection.StatusError, Active},
		{detection.StatusTestsFailing, Active},
		{detection.StatusUnknown, Active},
	}

	for _, tt := range tests {
		t.Run(tt.detected.String(), func(t *testing.T) {
			got := StatusFromDetected(tt.detected)
			if got != tt.want {
				t.Errorf("StatusFromDetected(%s) = %v, want %v",
					tt.detected, got, tt.want)
			}
		})
	}
}
