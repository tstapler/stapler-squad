package session

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCaptureCurrentState_NotStarted_IsNoOp verifies that CaptureCurrentState
// returns nil without modifying WorkingDir when the instance has not been started.
func TestCaptureCurrentState_NotStarted_IsNoOp(t *testing.T) {
	inst := &Instance{Title: "test-session"}
	// inst.started == false by default

	err := inst.CaptureCurrentState()

	require.NoError(t, err)
	assert.Empty(t, inst.WorkingDir, "WorkingDir should not be set for unstarted instance")
}

// TestCaptureCurrentState_Paused_IsNoOp verifies that CaptureCurrentState
// returns nil without modifying WorkingDir when the instance is paused.
func TestCaptureCurrentState_Paused_IsNoOp(t *testing.T) {
	inst := &Instance{Title: "test-session"}
	inst.started.Store(true)
	inst.Status = Paused

	err := inst.CaptureCurrentState()

	require.NoError(t, err)
	assert.Empty(t, inst.WorkingDir, "WorkingDir should not be set for paused instance")
}

// TestCaptureCurrentState_TmuxSessionDead_IsNoOp verifies that CaptureCurrentState
// returns nil when the underlying tmux session does not exist (nil TmuxSession).
// processManager nil (uninitialized Instance) → IsAlive() returns false via nil guard.
func TestCaptureCurrentState_TmuxSessionDead_IsNoOp(t *testing.T) {
	inst := &Instance{Title: "test-session"}
	inst.started.Store(true)
	// processManager is nil (zero-value interface) → CaptureCurrentState nil guard returns nil.

	err := inst.CaptureCurrentState()

	require.NoError(t, err)
	assert.Empty(t, inst.WorkingDir, "WorkingDir should not be set when tmux session is dead")
}
