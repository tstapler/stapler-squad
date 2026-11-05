package session

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tstapler/stapler-squad/session/domain"
)

// Every StuckReason must be enumerated by AllStuckReasons and valid, so the
// selfHealStuck/proto mappings cannot silently miss the new reason.
func TestRepeatedNoopDispatchReasonRegistered(t *testing.T) {
	assert.Contains(t, domain.AllStuckReasons, domain.StuckReasonRepeatedNoopDispatch)
	assert.True(t, domain.StuckReasonRepeatedNoopDispatch.IsValid())
}
