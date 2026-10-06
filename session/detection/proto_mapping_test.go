package detection

import (
	"testing"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

// TestDetectedStatusToProto verifies that every DetectedStatus value maps to its
// exact semantic counterpart in the proto DetectedStatus enum. DetectedStatusToProto
// is documented as "the single authoritative mapping" for this conversion; the
// exhaustive linter only guards against a missing case in the switch, not a wrong
// one (e.g. swapping two semantically adjacent statuses like NeedsApproval and
// InputRequired). This table pins down the full value-by-value correspondence.
func TestDetectedStatusToProto(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   DetectedStatus
		want sessionv1.DetectedStatus
	}{
		{"StatusUnknown", StatusUnknown, sessionv1.DetectedStatus_DETECTED_STATUS_UNKNOWN},
		{"StatusReady", StatusReady, sessionv1.DetectedStatus_DETECTED_STATUS_READY},
		{"StatusProcessing", StatusProcessing, sessionv1.DetectedStatus_DETECTED_STATUS_PROCESSING},
		{"StatusNeedsApproval", StatusNeedsApproval, sessionv1.DetectedStatus_DETECTED_STATUS_NEEDS_APPROVAL},
		{"StatusInputRequired", StatusInputRequired, sessionv1.DetectedStatus_DETECTED_STATUS_INPUT_REQUIRED},
		{"StatusError", StatusError, sessionv1.DetectedStatus_DETECTED_STATUS_ERROR},
		{"StatusTestsFailing", StatusTestsFailing, sessionv1.DetectedStatus_DETECTED_STATUS_TESTS_FAILING},
		{"StatusIdle", StatusIdle, sessionv1.DetectedStatus_DETECTED_STATUS_IDLE},
		{"StatusExecuting", StatusExecuting, sessionv1.DetectedStatus_DETECTED_STATUS_EXECUTING},
		{"StatusSuccess", StatusSuccess, sessionv1.DetectedStatus_DETECTED_STATUS_SUCCESS},
		{"StatusWaitingForAgent", StatusWaitingForAgent, sessionv1.DetectedStatus_DETECTED_STATUS_WAITING_FOR_AGENT},
		{"StatusCompacting", StatusCompacting, sessionv1.DetectedStatus_DETECTED_STATUS_COMPACTING},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectedStatusToProto(tt.in)
			if got != tt.want {
				t.Errorf("DetectedStatusToProto(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

// TestDetectedStatusToProto_OutOfRangeValue verifies that a DetectedStatus value
// outside the defined const set falls back to DETECTED_STATUS_UNSPECIFIED via the
// switch's default branch, rather than panicking or silently aliasing to a
// semantically meaningful status.
func TestDetectedStatusToProto_OutOfRangeValue(t *testing.T) {
	t.Parallel()
	got := DetectedStatusToProto(DetectedStatus(999))
	want := sessionv1.DetectedStatus_DETECTED_STATUS_UNSPECIFIED
	if got != want {
		t.Errorf("DetectedStatusToProto(999) = %v, want %v", got, want)
	}
}

// TestDetectedStatusToSubStatus verifies every DetectedStatus value against the
// single authoritative DetectedStatus → SubStatus mapping. This function replaces
// two previously-duplicated inline switches in server/adapters/instance_adapter.go
// (toProtoSubStatus) and server/adapters/review_queue_adapter.go (subStatusFromItem);
// both now call this function instead of reimplementing the switch. Exercising all
// DetectedStatus values here guards against the two call sites drifting apart again.
func TestDetectedStatusToSubStatus(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   DetectedStatus
		want sessionv1.SubStatus
	}{
		{"StatusUnknown", StatusUnknown, sessionv1.SubStatus_SUB_STATUS_UNSPECIFIED},
		{"StatusReady", StatusReady, sessionv1.SubStatus_SUB_STATUS_READY},
		{"StatusProcessing", StatusProcessing, sessionv1.SubStatus_SUB_STATUS_PROCESSING},
		{"StatusNeedsApproval", StatusNeedsApproval, sessionv1.SubStatus_SUB_STATUS_NEEDS_APPROVAL},
		{"StatusInputRequired", StatusInputRequired, sessionv1.SubStatus_SUB_STATUS_INPUT_REQUIRED},
		{"StatusError", StatusError, sessionv1.SubStatus_SUB_STATUS_ERROR},
		{"StatusTestsFailing", StatusTestsFailing, sessionv1.SubStatus_SUB_STATUS_TESTS_FAILING},
		{"StatusIdle", StatusIdle, sessionv1.SubStatus_SUB_STATUS_IDLE},
		{"StatusExecuting", StatusExecuting, sessionv1.SubStatus_SUB_STATUS_PROCESSING},
		{"StatusSuccess", StatusSuccess, sessionv1.SubStatus_SUB_STATUS_SUCCESS},
		{"StatusWaitingForAgent", StatusWaitingForAgent, sessionv1.SubStatus_SUB_STATUS_WAITING_FOR_AGENT},
		{"StatusCompacting", StatusCompacting, sessionv1.SubStatus_SUB_STATUS_COMPACTING},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectedStatusToSubStatus(tt.in)
			if got != tt.want {
				t.Errorf("DetectedStatusToSubStatus(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

// TestDetectedStatusToSubStatus_OutOfRangeValue verifies that a DetectedStatus value
// outside the defined const set falls back to SUB_STATUS_UNSPECIFIED via the switch's
// default branch, rather than panicking or silently aliasing to a meaningful status.
func TestDetectedStatusToSubStatus_OutOfRangeValue(t *testing.T) {
	t.Parallel()
	got := DetectedStatusToSubStatus(DetectedStatus(999))
	want := sessionv1.SubStatus_SUB_STATUS_UNSPECIFIED
	if got != want {
		t.Errorf("DetectedStatusToSubStatus(999) = %v, want %v", got, want)
	}
}
