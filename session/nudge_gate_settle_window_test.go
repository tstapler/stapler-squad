package session

// nudge_gate_settle_window_test.go is the regression suite for gap #5:
// CheckNudgeEligible previously accepted StatusIdle||StatusReady||StatusSuccess
// on a single point-in-time poll, which (a) can't distinguish Claude Code's
// own idle prompt from a raw shell/vim prompt reporting the same StatusIdle
// value, and (b) treats one noisy poll as sufficient. These tests build a
// *ClaudeController against an in-memory buffer (newControllerWithMock, from
// claude_controller_test.go) instead of a real tmux session/PTY.

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/detection"
)

// nudgeGateTestInstance wires cc as inst's controller via the unexported
// controllerManager field (same-package test helper) so CheckNudgeEligible's
// inst.GetController() call resolves to a controllable, non-tmux controller.
func nudgeGateTestInstance(cc *ClaudeController) *Instance {
	inst := &Instance{Title: "nudge-gate-test", UUID: uuid.New().String()}
	inst.controllerManager.SetController(cc)
	return inst
}

// TestCheckNudgeEligibleWithSettleWindow_RejectsRawShellPrompt is the direct
// regression for dropping the old isIdleStatus(StatusIdle||StatusReady||
// StatusSuccess) check: a raw shell prompt reports the same detection.StatusIdle
// value as Claude Code's own idle prompt, so only the description allowlist
// (IsSafeSteerStatus) tells them apart.
func TestCheckNudgeEligibleWithSettleWindow_RejectsRawShellPrompt(t *testing.T) {
	t.Parallel()
	cc, _ := newControllerWithMock("tstapler@dev-box:~/stapler-squad$ ")
	status, _ := cc.GetCurrentStatus()
	require.Equal(t, detection.StatusIdle, status, "sanity: a raw shell prompt must still report StatusIdle for this test to be meaningful")

	inst := nudgeGateTestInstance(cc)
	err := CheckNudgeEligibleWithSettleWindow(inst, 0, time.Millisecond)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNudgeTargetNotIdle)
}

// TestCheckNudgeEligibleWithSettleWindow_AcceptsSafeIdlePromptAfterSettleWindow
// proves the settle window is actually honored (it waits, rather than
// returning on the first poll) and that a genuinely-idle Claude Code prompt
// held steady the whole time passes.
func TestCheckNudgeEligibleWithSettleWindow_AcceptsSafeIdlePromptAfterSettleWindow(t *testing.T) {
	t.Parallel()
	cc, _ := newControllerWithMock("I've fixed the null check in validator.go.\n> ")
	status, _ := cc.GetCurrentStatus()
	require.Equal(t, detection.StatusIdle, status)

	inst := nudgeGateTestInstance(cc)
	const settleWindow = 50 * time.Millisecond
	start := time.Now()
	err := CheckNudgeEligibleWithSettleWindow(inst, settleWindow, 10*time.Millisecond)
	elapsed := time.Since(start)

	require.NoError(t, err)
	assert.GreaterOrEqual(t, elapsed, settleWindow, "must actually wait out the settle window, not return on the first poll")
}

// newControllerWithSharedBuffer mirrors newControllerWithMock (claude_controller_test.go)
// but returns the backing *CircularBuffer too, so a test can append further
// output after construction to simulate the pane's content changing over
// time — newControllerWithMock's buffer is write-once and not exposed.
func newControllerWithSharedBuffer(content string) (*ClaudeController, *CircularBuffer) {
	inst := &mockInstance{title: "test", preview: content}
	cc := &ClaudeController{
		sessionName: "test",
		instance:    inst,
	}
	cc.statusDetector.Store(detection.NewStatusDetector())
	cc.idleDetector.Store(detection.NewIdleDetector("test", nil))
	buf := NewCircularBuffer(256 * 1024)
	if content != "" {
		_, _ = buf.Write([]byte(content))
	}
	cc.ptyAccess.Store(NewPTYAccess("test", nil, buf))
	return cc, buf
}

// TestCheckNudgeEligibleWithSettleWindow_RejectsWhenStatusFlipsDuringWindow is
// the direct sustained-idle regression: a status that starts safe but flips
// to a raw shell prompt partway through the settle window must be refused —
// a single early safe poll must not be enough.
func TestCheckNudgeEligibleWithSettleWindow_RejectsWhenStatusFlipsDuringWindow(t *testing.T) {
	t.Parallel()
	cc, buf := newControllerWithSharedBuffer("I've fixed the null check in validator.go.\n> ")
	inst := nudgeGateTestInstance(cc)

	const settleWindow = 200 * time.Millisecond
	const pollInterval = 10 * time.Millisecond
	flipAfter := settleWindow / 4

	go func() {
		time.Sleep(flipAfter) //nolint:notimesleeptest flips the pane mid-way through the production wall-clock settle-window poll loop; the window itself is real time
		_, _ = buf.Write([]byte("\ntstapler@dev-box:~/stapler-squad$ "))
	}()

	err := CheckNudgeEligibleWithSettleWindow(inst, settleWindow, pollInterval)
	require.Error(t, err, "a status flip partway through the settle window must refuse the nudge, not pass on the early safe poll")
	assert.ErrorIs(t, err, ErrNudgeTargetNotIdle)
}
