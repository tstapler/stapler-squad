package instance

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/domain"
)

// TestCaptureCurrentState_Paused_IsNoOp verifies that CaptureCurrentState
// returns nil without modifying WorkingDir when the instance is paused.
func TestCaptureCurrentState_Paused_IsNoOp(t *testing.T) {
	t.Parallel()
	inst := &Instance{Title: "test-session"}
	inst.started.Store(true)
	inst.Status = domain.Paused

	err := inst.CaptureCurrentState()

	require.NoError(t, err)
	assert.Empty(t, inst.WorkingDir, "WorkingDir should not be set for paused instance")
}

// TestCaptureCurrentState_TmuxSessionDead_IsNoOp verifies that CaptureCurrentState
// returns nil when the underlying tmux session does not exist (nil TmuxSession).
// processManager nil (uninitialized Instance) → IsAlive() returns false via nil guard.
func TestCaptureCurrentState_TmuxSessionDead_IsNoOp(t *testing.T) {
	t.Parallel()
	inst := &Instance{Title: "test-session"}
	inst.started.Store(true)
	// processManager is nil (zero-value interface) → CaptureCurrentState nil guard returns nil.

	err := inst.CaptureCurrentState()

	require.NoError(t, err)
	assert.Empty(t, inst.WorkingDir, "WorkingDir should not be set when tmux session is dead")
}
