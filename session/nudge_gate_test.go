package session

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/executor/safeexec"
	"github.com/tstapler/stapler-squad/session/tmux"
)

// newRealTmuxInstanceForNudgeTest starts a real tmux session (skipping the
// test if tmux is unavailable, matching
// TestInstanceStartControlMode_should_NeverProduceTwoOwners_When_100GoroutinesRaceHubRegistryConcurrently's
// established pattern in instance_control_mode_ownership_test.go) and wires
// it into a bare *Instance the same way that test does, then stamps the pane's
// STAPLER_SESSION_UUID env var to envUUID — independent of instUUID, the
// Instance's own identity, so the two can be set to match or mismatch per test.
func newRealTmuxInstanceForNudgeTest(t *testing.T, instUUID, envUUID string) *Instance {
	t.Helper()
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not available")
	}

	name := fmt.Sprintf("ssq-nudge-gate-%d-%d", os.Getpid(), time.Now().UnixNano())
	tmuxSession, cleanup := tmux.NewTmuxSessionWithPrefixAndCleanup(name, "sleep 60", "staplersquad_test_")
	t.Cleanup(func() {
		if err := cleanup(); err != nil {
			t.Logf("tmux cleanup for %s: %v", name, err)
		}
	})
	require.NoError(t, tmuxSession.Start(t.TempDir()))

	inst := &Instance{Title: name, UUID: instUUID}
	_ = inst.pm() // lazily initializes processManager as *TmuxBackend before SetTmuxSession
	inst.SetTmuxSession(tmuxSession)

	tmuxName := inst.GetTmuxSessionName()
	require.NotEmpty(t, tmuxName, "expected Instance.GetTmuxSessionName() to be non-empty after SetTmuxSession")

	socketArgs := tmux.ResolveSocket("").Args
	setEnvArgs := socketArgs("set-environment", "-t", tmuxName, "STAPLER_SESSION_UUID", envUUID)
	out, err := safeexec.CommandContext(context.Background(), tmux.Binary(), setEnvArgs...).CombinedOutput()
	require.NoErrorf(t, err, "tmux set-environment failed: %s", out)

	return inst
}

// TestVerifyPaneOwnershipBeforeWrite_RejectsOnIdentityMismatch is the direct
// regression test validation.md's AC2 section calls out as required "before
// considering AC2 satisfied": the tmux pane this Instance believes it owns
// now reports a different STAPLER_SESSION_UUID (the ce71ad1a-shaped
// session-name-collision scenario this whole feature exists to guard
// against) — VerifyPaneOwnershipBeforeWrite must refuse rather than let a
// nudge write land in the wrong pane.
func TestVerifyPaneOwnershipBeforeWrite_RejectsOnIdentityMismatch(t *testing.T) {
	instUUID := uuid.New().String()
	otherUUID := uuid.New().String()
	inst := newRealTmuxInstanceForNudgeTest(t, instUUID, otherUUID)

	err := VerifyPaneOwnershipBeforeWrite(context.Background(), inst)

	require.Error(t, err, "must refuse to write when the pane's live owner marker no longer matches this Instance's UUID")
	require.Contains(t, err.Error(), "pane ownership mismatch")
}

// TestVerifyPaneOwnershipBeforeWrite_AllowsMatchingIdentity is the
// matching-case counterpart: when the pane's marker still agrees with the
// Instance's own UUID, the write-time check must not itself block a nudge.
func TestVerifyPaneOwnershipBeforeWrite_AllowsMatchingIdentity(t *testing.T) {
	instUUID := uuid.New().String()
	inst := newRealTmuxInstanceForNudgeTest(t, instUUID, instUUID)

	err := VerifyPaneOwnershipBeforeWrite(context.Background(), inst)

	require.NoError(t, err)
}

// TestVerifyPaneOwnershipBeforeWrite_NoTmuxIdentity covers the case
// VerifyPaneOwnershipBeforeWrite's own guard names explicitly: an Instance
// with no live tmux session (GetTmuxSessionName() == "") has nothing to
// re-verify against, so it must fail closed rather than silently no-op.
func TestVerifyPaneOwnershipBeforeWrite_NoTmuxIdentity(t *testing.T) {
	inst := &Instance{Title: "no-tmux-session", UUID: uuid.New().String()}

	err := VerifyPaneOwnershipBeforeWrite(context.Background(), inst)

	require.Error(t, err)
	require.Contains(t, err.Error(), "no tmux identity to verify")
}

// TestCheckNudgeEligible_RejectsNilController covers CheckNudgeEligible's
// no-controller branch: a session with no running ClaudeController (e.g.
// paused, or never started) can never be nudge-eligible regardless of its
// last-known status.
func TestCheckNudgeEligible_RejectsNilController(t *testing.T) {
	inst := &Instance{Title: "no-controller", UUID: uuid.New().String()}

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
