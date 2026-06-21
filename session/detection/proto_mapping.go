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
	default:
		return sessionv1.DetectedStatus_DETECTED_STATUS_UNSPECIFIED
	}
}
