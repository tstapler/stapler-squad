package session

import (
	"testing"

	"github.com/tstapler/stapler-squad/session/detection"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/queue"
)

// TestAttentionReasonFromDetected verifies every DetectedStatus maps to the expected reason.
func TestAttentionReasonFromDetected(t *testing.T) {
	t.Parallel()
	tests := []struct {
		detected detection.DetectedStatus
		want     queue.AttentionReason
	}{
		{detection.StatusNeedsApproval, queue.ReasonApprovalPending},
		{detection.StatusInputRequired, queue.ReasonInputRequired},
		{detection.StatusError, queue.ReasonErrorState},
		{detection.StatusTestsFailing, queue.ReasonTestsFailing},
		{detection.StatusSuccess, queue.ReasonTaskComplete},
		{detection.StatusIdle, queue.ReasonIdle},
		// States that do not require attention
		{detection.StatusExecuting, ""},
		{detection.StatusProcessing, ""},
		{detection.StatusReady, ""},
		{detection.StatusUnknown, ""},
		{detection.StatusCompacting, ""},
	}

	for _, tt := range tests {
		t.Run(tt.detected.String(), func(t *testing.T) {
			t.Parallel()
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
	t.Parallel()
	tests := []struct {
		detected detection.DetectedStatus
		want     domain.Status
	}{
		{detection.StatusReady, domain.Active},
		{detection.StatusIdle, domain.Active},
		{detection.StatusSuccess, domain.Active},
		{detection.StatusProcessing, domain.Active},
		{detection.StatusExecuting, domain.Active},
		{detection.StatusNeedsApproval, domain.Active},
		{detection.StatusInputRequired, domain.Active},
		{detection.StatusError, domain.Active},
		{detection.StatusTestsFailing, domain.Active},
		{detection.StatusUnknown, domain.Active},
	}

	for _, tt := range tests {
		t.Run(tt.detected.String(), func(t *testing.T) {
			t.Parallel()
			got := StatusFromDetected(tt.detected)
			if got != tt.want {
				t.Errorf("StatusFromDetected(%s) = %v, want %v",
					tt.detected, got, tt.want)
			}
		})
	}
}
