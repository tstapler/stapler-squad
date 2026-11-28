package adapters

import (
	"testing"

	"github.com/stretchr/testify/assert"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session/domain"
)

// TestFromProtoStuckReason_should_ReturnBounceCapExhausted_When_ProtoBounceCapExhausted
// verifies the inverse mapping for the new synthetic aggregate reason
// (backlog-bounce-escalation, Epic 1.1) recovers the correct domain constant.
func TestFromProtoStuckReason_should_ReturnBounceCapExhausted_When_ProtoBounceCapExhausted(t *testing.T) {
	t.Parallel()
	got := FromProtoStuckReason(sessionv1.StuckReason_STUCK_REASON_BOUNCE_CAP_EXHAUSTED)
	assert.Equal(t, domain.StuckReasonBounceCapExhausted, got)
}
