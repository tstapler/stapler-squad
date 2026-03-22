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
		{detection.StatusActive, ""},
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

// TestStatusFromDetected verifies every DetectedStatus maps to the expected lifecycle Status.
func TestStatusFromDetected(t *testing.T) {
	tests := []struct {
		detected detection.DetectedStatus
		want     Status
	}{
		{detection.StatusReady, Ready},
		{detection.StatusIdle, Ready},
		{detection.StatusSuccess, Ready},
		{detection.StatusProcessing, Running},
		{detection.StatusActive, Running},
		{detection.StatusNeedsApproval, NeedsApproval},
		{detection.StatusInputRequired, NeedsApproval},
		// Error states keep Running at lifecycle level
		{detection.StatusError, Running},
		{detection.StatusTestsFailing, Running},
		{detection.StatusUnknown, Running},
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
