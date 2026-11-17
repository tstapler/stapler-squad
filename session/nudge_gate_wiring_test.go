package session

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/instance"
)

// TestVerifyPaneOwnershipBeforeWrite_NoTmuxIdentity covers the case
// VerifyPaneOwnershipBeforeWrite's own guard names explicitly: an Instance
// with no live tmux session (GetTmuxSessionName() == "") has nothing to
// re-verify against, so it must fail closed rather than silently no-op.
func TestVerifyPaneOwnershipBeforeWrite_NoTmuxIdentity(t *testing.T) {
	inst := &instance.Instance{Title: "no-tmux-session", UUID: uuid.New().String()}

	err := instance.VerifyPaneOwnershipBeforeWrite(context.Background(), inst)

	require.Error(t, err)
	require.Contains(t, err.Error(), "no tmux identity to verify")
}

// TestCheckNudgeEligible_RejectsNilController covers CheckNudgeEligible's
// no-controller branch: a session with no running claudectl.ClaudeController (e.g.
// paused, or never started) can never be nudge-eligible regardless of its
// last-known status.
func TestCheckNudgeEligible_RejectsNilController(t *testing.T) {
	inst := &instance.Instance{Title: "no-controller", UUID: uuid.New().String()}

	err := CheckNudgeEligible(inst)

	require.Error(t, err)
	require.Contains(t, err.Error(), "no running controller")
}

// TestCheckNudgeEligible_RejectsNilInstance covers the defensive nil-Instance
// guard directly (a nil target should never reach here via the dispatch
// path's FindLiveInstance nil-check, but the exported function must fail
// closed on its own regardless of caller discipline).
func TestCheckNudgeEligible_RejectsNilInstance(t *testing.T) {
	err := CheckNudgeEligible(nil)

	require.Error(t, err)
}
