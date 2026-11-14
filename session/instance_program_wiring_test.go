package session

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tstapler/stapler-squad/session/instance"
)

// TestPortHistoryFailureIsExpected directly covers the Warn-vs-Error log-level selection used
// by SwitchProgram, independent of whether isClaudeAntigravityCrossSwitch/CanHandle parity
// currently makes the ErrNoHistoryAdapter branch reachable in production — this keeps the
// selection logic itself verified as defense-in-depth against future drift.
func TestPortHistoryFailureIsExpected(t *testing.T) {
	t.Parallel()
	assert.True(t, instance.PortHistoryFailureIsExpected(instance.ErrNoHistoryAdapter))
	assert.True(t, instance.PortHistoryFailureIsExpected(fmt.Errorf("wrapped: %w", instance.ErrNoHistoryAdapter)))
	assert.False(t, instance.PortHistoryFailureIsExpected(errors.New("disk full")))
}
