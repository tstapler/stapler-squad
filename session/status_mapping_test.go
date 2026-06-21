package session

import (
	"testing"

	"github.com/tstapler/stapler-squad/session/detection"
)

// TestAttentionReasonFromDetected verifies every DetectedStatus maps to the expected reason.
func TestAttentionReasonFromDetected(t *testing.T) {
	tests := []struct {
		detected detection.DetectedStatus
		want     AttentionReason
	}{
		{detection.StatusNeedsApproval, ReasonApprovalPending},
		{detection.StatusInputRequired, ReasonInputRequired},
		{detection.StatusError, ReasonErrorState},
		{detection.StatusTestsFailing, ReasonTestsFailing},
		{detection.StatusSuccess, ReasonTaskComplete},
		{detection.StatusIdle, ReasonIdle},
		// States that do not require attention
		{detection.StatusExecuting, ""},
		{detection.StatusProcessing, ""},
		{detection.StatusReady, ""},
		{detection.StatusUnknown, ""},
	}

	for _, tt := range tests {
		t.Run(tt.detected.String(), func(t *testing.T) {
			got := AttentionReasonFromDetected(tt.detected)
			if got != tt.want {
				t.Errorf("AttentionReasonFromDetected(%s) = %q, want %q",
					tt.detected, got, tt.want)
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
