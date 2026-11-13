package session

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestPortHistoryFailureIsExpected directly covers the Warn-vs-Error log-level selection used
// by SwitchProgram, independent of whether isClaudeAntigravityCrossSwitch/CanHandle parity
// currently makes the ErrNoHistoryAdapter branch reachable in production — this keeps the
// selection logic itself verified as defense-in-depth against future drift.
func TestPortHistoryFailureIsExpected(t *testing.T) {
	t.Parallel()
	assert.True(t, portHistoryFailureIsExpected(ErrNoHistoryAdapter))
	assert.True(t, portHistoryFailureIsExpected(fmt.Errorf("wrapped: %w", ErrNoHistoryAdapter)))
	assert.False(t, portHistoryFailureIsExpected(errors.New("disk full")))
}
