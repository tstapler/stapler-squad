package session

import "testing"

// TestAttentionReasonFromDetected verifies every DetectedStatus maps to the expected reason.
func TestAttentionReasonFromDetected(t *testing.T) {
	tests := []struct {
		detected DetectedStatus
		want     AttentionReason
	}{
		{StatusNeedsApproval, ReasonApprovalPending},
		{StatusInputRequired, ReasonInputRequired},
		{StatusError, ReasonErrorState},
		{StatusTestsFailing, ReasonTestsFailing},
		{StatusSuccess, ReasonTaskComplete},
		{StatusIdle, ReasonIdle},
		// States that do not require attention
		{StatusActive, ""},
		{StatusProcessing, ""},
		{StatusReady, ""},
		{StatusUnknown, ""},
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

// TestStatusFromDetected verifies every DetectedStatus maps to the expected lifecycle Status.
func TestStatusFromDetected(t *testing.T) {
	tests := []struct {
		detected DetectedStatus
		want     Status
	}{
		{StatusReady, Ready},
		{StatusIdle, Ready},
		{StatusSuccess, Ready},
		{StatusProcessing, Running},
		{StatusActive, Running},
		{StatusNeedsApproval, NeedsApproval},
		{StatusInputRequired, NeedsApproval},
		// Error states keep Running at lifecycle level
		{StatusError, Running},
		{StatusTestsFailing, Running},
		{StatusUnknown, Running},
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
