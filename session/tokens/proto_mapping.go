package tokens

import (
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session/domain"
)

// ContextHealthLevelToProto is the single authoritative mapping from
// domain.ContextHealthLevel to the proto enum; do not duplicate it in adapters.
func ContextHealthLevelToProto(l domain.ContextHealthLevel) sessionv1.ContextHealth {
	switch l {
	case domain.HealthGreen:
		return sessionv1.ContextHealth_CONTEXT_HEALTH_GREEN
	case domain.HealthAmber:
		return sessionv1.ContextHealth_CONTEXT_HEALTH_AMBER
	case domain.HealthRed:
		return sessionv1.ContextHealth_CONTEXT_HEALTH_RED
	default:
		return sessionv1.ContextHealth_CONTEXT_HEALTH_UNSPECIFIED
	}
}
