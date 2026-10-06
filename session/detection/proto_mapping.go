package detection

import sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"

// DetectedStatusToProto converts the internal detection.DetectedStatus iota to the
// proto enum sessionv1.DetectedStatus. This is the single authoritative mapping;
// do not duplicate this logic in adapters or converters.
func DetectedStatusToProto(s DetectedStatus) sessionv1.DetectedStatus {
	switch s {
	case StatusIdle:
		return sessionv1.DetectedStatus_DETECTED_STATUS_IDLE
	case StatusProcessing:
		return sessionv1.DetectedStatus_DETECTED_STATUS_PROCESSING
	case StatusExecuting:
		return sessionv1.DetectedStatus_DETECTED_STATUS_EXECUTING
	case StatusNeedsApproval:
		return sessionv1.DetectedStatus_DETECTED_STATUS_NEEDS_APPROVAL
	case StatusInputRequired:
		return sessionv1.DetectedStatus_DETECTED_STATUS_INPUT_REQUIRED
	case StatusError:
		return sessionv1.DetectedStatus_DETECTED_STATUS_ERROR
	case StatusTestsFailing:
		return sessionv1.DetectedStatus_DETECTED_STATUS_TESTS_FAILING
	case StatusSuccess:
		return sessionv1.DetectedStatus_DETECTED_STATUS_SUCCESS
	case StatusUnknown:
		return sessionv1.DetectedStatus_DETECTED_STATUS_UNKNOWN
	case StatusReady:
		return sessionv1.DetectedStatus_DETECTED_STATUS_READY
	case StatusWaitingForAgent:
		return sessionv1.DetectedStatus_DETECTED_STATUS_WAITING_FOR_AGENT
	case StatusCompacting:
		return sessionv1.DetectedStatus_DETECTED_STATUS_COMPACTING
	default:
		return sessionv1.DetectedStatus_DETECTED_STATUS_UNSPECIFIED
	}
}

// DetectedStatusToSubStatus converts the internal detection.DetectedStatus iota to the
// proto enum sessionv1.SubStatus. This is the single authoritative mapping; do not
// duplicate this logic in adapters or converters — call this function instead.
//
// Callers that need additional context (e.g. gating on session.Status or rate-limit
// state) should apply that logic around a call to this function rather than
// reimplementing the DetectedStatus switch itself.
func DetectedStatusToSubStatus(s DetectedStatus) sessionv1.SubStatus {
	switch s {
	case StatusWaitingForAgent:
		return sessionv1.SubStatus_SUB_STATUS_WAITING_FOR_AGENT
	case StatusCompacting:
		return sessionv1.SubStatus_SUB_STATUS_COMPACTING
	case StatusProcessing, StatusExecuting:
		return sessionv1.SubStatus_SUB_STATUS_PROCESSING
	case StatusNeedsApproval:
		return sessionv1.SubStatus_SUB_STATUS_NEEDS_APPROVAL
	case StatusInputRequired:
		return sessionv1.SubStatus_SUB_STATUS_INPUT_REQUIRED
	case StatusError:
		return sessionv1.SubStatus_SUB_STATUS_ERROR
	case StatusTestsFailing:
		return sessionv1.SubStatus_SUB_STATUS_TESTS_FAILING
	case StatusReady:
		return sessionv1.SubStatus_SUB_STATUS_READY
	case StatusIdle:
		return sessionv1.SubStatus_SUB_STATUS_IDLE
	case StatusSuccess:
		return sessionv1.SubStatus_SUB_STATUS_SUCCESS
	case StatusUnknown:
		// Unknown / undetected — don't show a chip.
		return sessionv1.SubStatus_SUB_STATUS_UNSPECIFIED
	default:
		return sessionv1.SubStatus_SUB_STATUS_UNSPECIFIED
	}
}
