package tokens

import sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"

// ContextHealthLevelToProto is the single authoritative mapping from
// ContextHealthLevel to the proto enum; do not duplicate it in adapters.
func ContextHealthLevelToProto(l ContextHealthLevel) sessionv1.ContextHealth {
	switch l {
	case HealthGreen:
		return sessionv1.ContextHealth_CONTEXT_HEALTH_GREEN
	case HealthAmber:
		return sessionv1.ContextHealth_CONTEXT_HEALTH_AMBER
	case HealthRed:
		return sessionv1.ContextHealth_CONTEXT_HEALTH_RED
	default:
		return sessionv1.ContextHealth_CONTEXT_HEALTH_UNSPECIFIED
	}
}
