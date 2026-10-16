package tmux

import (
	"context"
	"io"
	"os/exec"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// countingWriteCloser records how many times Write is called, so a test can
// prove the control-mode path was never attempted (as opposed to attempted
// and merely failing).
type countingWriteCloser struct {
	mu     sync.Mutex
	writes int
}

func (w *countingWriteCloser) Write(p []byte) (int, error) {
	w.mu.Lock()
	w.writes++
	w.mu.Unlock()
	return len(p), nil
}

func (w *countingWriteCloser) Close() error { return nil }

func (w *countingWriteCloser) count() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.writes
}

// fakeDimensionsExecutor answers every Output call with a fixed
// "width height" payload, so GetPaneDimensionsPriority's subprocess fallback
// succeeds deterministically without a real tmux server.
type fakeDimensionsExecutor struct{ output string }

func (f fakeDimensionsExecutor) Run(cmd *exec.Cmd) error { return nil }
func (f fakeDimensionsExecutor) Output(cmd *exec.Cmd) ([]byte, error) {
	return []byte(f.output), nil
}
func (f fakeDimensionsExecutor) CombinedOutput(cmd *exec.Cmd) ([]byte, error) {
	return []byte(f.output), nil
}

// newDimensionsPriorityTestSession wires a TmuxSession with both a live
// control-mode sender goroutine (so cmEnabledForBackground() is true and
// pendingCmds is genuinely mutable) and a fast-lane exec-gate config, so
// GetPaneDimensionsPriority's full control-mode-then-subprocess-fallback path
// is exercisable without a real tmux binary.
func newDimensionsPriorityTestSession(t *testing.T, serverSocket string) (*TmuxSession, *countingWriteCloser) {
	t.Helper()
	stdin := &countingWriteCloser{}
	doneCh := make(chan struct{})
	sess := &TmuxSession{
		sanitizedName:    "test_session",
		serverSocket:     serverSocket,
		controlModeStdin: stdin,
		highPriSendCh:    make(chan cmSendReq, 64),
		normPriSendCh:    make(chan cmSendReq, 256),
		cmSenderExited:   make(chan struct{}),
		cmdExec:          fakeDimensionsExecutor{output: "80 24"},
	}
	go sess.runCMSender(doneCh, stdin, sess.highPriSendCh, sess.normPriSendCh, sess.cmSenderExited)
	t.Cleanup(func() { close(doneCh) })
	return sess, stdin
}

func TestPendingCommandDepth_ReflectsQueueLength(t *testing.T) {
	t.Parallel()
	sess, _ := newDispatchTestSession(t)

	assert.Equal(t, 0, sess.pendingCommandDepth())

	channels := enqueueChannels(sess, 5)
	assert.Equal(t, 5, sess.pendingCommandDepth())

	// Draining one (as the reader goroutine would on a %begin/%end pair)
	// drops the depth by exactly one.
	sess.controlModeSubMu.Lock()
	sess.pendingCmds = sess.pendingCmds[1:]
	sess.controlModeSubMu.Unlock()
	assert.Equal(t, 4, sess.pendingCommandDepth())
	_ = channels
}

func TestGetPaneDimensionsPriority_SkipsControlMode_WhenQueueBackedUp(t *testing.T) {
	serverSocket := setupExecGateTestConfig(t, 4, 4)
	sess, stdin := newDimensionsPriorityTestSession(t, serverSocket)

	// Simulate the confirmed-live incident shape: the pending queue is
	// already backed up well past controlModeQueueBackpressureThreshold.
	enqueueChannels(sess, controlModeQueueBackpressureThreshold+10)

	ctx, cancel := context.WithTimeout(context.Background(), ResyncFastLaneTimeout)
	defer cancel()

	start := time.Now()
	width, height, err := sess.GetPaneDimensionsPriority(ctx)
	elapsed := time.Since(start)

	require.NoError(t, err)
	assert.Equal(t, 80, width)
	assert.Equal(t, 24, height)
	assert.Equal(t, 0, stdin.count(), "control-mode stdin must never be written to when the queue is already backed up past the threshold")
	assert.Less(t, elapsed, fastLaneCMAttemptTimeout,
		"skipping the control-mode attempt must not cost even fastLaneCMAttemptTimeout's own bounded wait")
}

func TestGetPaneDimensionsPriority_AttemptsControlMode_WhenQueueIsHealthy(t *testing.T) {
	serverSocket := setupExecGateTestConfig(t, 4, 4)
	sess, stdin := newDimensionsPriorityTestSession(t, serverSocket)

	ctx, cancel := context.WithTimeout(context.Background(), ResyncFastLaneTimeout)
	defer cancel()

	// No backlog: the control-mode attempt should still be made (it will
	// time out here since nothing ever answers %begin/%end on this fake
	// pipe, then correctly fall through to the subprocess fallback).
	width, height, err := sess.GetPaneDimensionsPriority(ctx)

	require.NoError(t, err)
	assert.Equal(t, 80, width)
	assert.Equal(t, 24, height)
	assert.Greater(t, stdin.count(), 0, "a healthy (non-backed-up) queue must still attempt the control-mode path first")
}

// TestSetWindowSize_SkipsControlMode_WhenQueueBackedUp is SetWindowSize's
// analogue of TestGetPaneDimensionsPriority_SkipsControlMode_WhenQueueBackedUp:
// StreamHub.applyNegotiatedSize shares one deadline across SetWindowSizeContext
// and the CapturePaneContentRawContext call that follows it, so a backed-up
// control-mode queue burning SetWindowSize's old, independent 3s cmCtx()
// budget starved the capture step of any time at all (confirmed live for
// staplersquad_tymux and staplersquad_dotfiles: "streamhub: resize caller
// disconnected or timed out" immediately followed by "exec gate: context
// deadline exceeded" on the capture).
func TestSetWindowSize_SkipsControlMode_WhenQueueBackedUp(t *testing.T) {
	serverSocket := setupExecGateTestConfig(t, 4, 4)
	sess, stdin := newDimensionsPriorityTestSession(t, serverSocket)

	enqueueChannels(sess, controlModeQueueBackpressureThreshold+10)

	start := time.Now()
	err := sess.SetWindowSize(80, 24)
	elapsed := time.Since(start)

	require.NoError(t, err)
	assert.Equal(t, 0, stdin.count(), "control-mode stdin must never be written to when the queue is already backed up past the threshold")
	assert.Less(t, elapsed, fastLaneCMAttemptTimeout,
		"skipping the control-mode attempt must not cost even fastLaneCMAttemptTimeout's own bounded wait")
}

func TestSetWindowSize_AttemptsControlMode_WhenQueueIsHealthy(t *testing.T) {
	serverSocket := setupExecGateTestConfig(t, 4, 4)
	sess, stdin := newDimensionsPriorityTestSession(t, serverSocket)

	err := sess.SetWindowSize(80, 24)

	require.NoError(t, err)
	assert.Greater(t, stdin.count(), 0, "a healthy (non-backed-up) queue must still attempt the control-mode path first")
}

var _ io.WriteCloser = (*countingWriteCloser)(nil)
