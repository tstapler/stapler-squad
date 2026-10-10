package services

// Story 5.1 (PR 5, streams): a hidden session's terminal stream refuses input
// and resize at every stream site. The capture-pane, shell and fallback rows are
// observed on the tmuxInputSender seam, never on an exec the test cannot see, so
// a "0 calls" assertion cannot pass vacuously: each row also asserts the visible
// attach reaches the same seam.

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/envtest"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/tmux"
)

func writerForHandler(t *testing.T, sender tmuxInputSender, inst *session.Instance) TerminalWriter {
	t.Helper()
	h := &ConnectRPCWebSocketHandler{tmuxSender: sender}
	return h.writerFor(inst)
}

func inputFrame(data string) *sessionv1.TerminalData {
	return &sessionv1.TerminalData{Data: &sessionv1.TerminalData_Input{Input: &sessionv1.TerminalInput{Data: []byte(data)}}}
}

func resizeFrame(cols, rows int32) *sessionv1.TerminalData {
	return &sessionv1.TerminalData{Data: &sessionv1.TerminalData_Resize{Resize: &sessionv1.TerminalResize{Cols: cols, Rows: rows}}}
}

func capturePaneParamsFor(inst *session.Instance, writer TerminalWriter, managed bool) capturePaneStreamParams {
	snap := *inst.Snapshot()
	snap.Permissions.CanSendCommand = true
	return capturePaneStreamParams{
		writer:   writer,
		instance: inst,
		snap:     &snap,
		cpt: capturePaneTarget{
			sessionID:        "ro-test",
			tmuxSessionName:  "ro-test-tmux",
			target:           inst,
			effectiveManaged: managed,
		},
	}
}

// T-RO-01, websocket:2701 row (dispatchInputReadLoopFrame).
func TestStreamSites_ShouldDropInputAndResizeForHiddenAndApplyForVisible_WhenFrameDispatched(t *testing.T) {
	for _, tc := range []struct {
		name   string
		hidden bool
		want   int
	}{
		{"hidden drops", true, 0},
		{"visible applies", false, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			inst := hiddenInstance(t, tc.hidden)
			writer := writerForHandler(t, &fakeTmuxSender{}, inst)
			var inputs, resizes atomic.Int32
			p := inputReadLoopParams{
				writer:   writer,
				onInput:  func([]byte) { inputs.Add(1) },
				onResize: func(int, int) { resizes.Add(1) },
			}
			dispatchInputReadLoopFrame(p, inputFrame("rm -rf"))
			dispatchInputReadLoopFrame(p, resizeFrame(200, 50))
			assert.EqualValues(t, tc.want, inputs.Load())
			assert.EqualValues(t, tc.want, resizes.Load())
		})
	}
}

// T-RO-04: the decision is made from the instance, not from anything the client sends.
func TestStream_ShouldStayReadOnly_WhenClientOmitsReadOnlyField(t *testing.T) {
	writer := writerForHandler(t, &fakeTmuxSender{}, hiddenInstance(t, true))
	assert.Nil(t, writer, "no client-supplied field can turn a hidden attach writable")
}

// T-RO-01 capture-pane row (:3337) and T-RO-48.
func TestCapturePane_ShouldSendZeroInputAndResizeCallsToTheSenderSeam_WhenAHiddenManagedSessionAttachesWithControlModeDisabledOrIsNotManaged(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_USE_CONTROL_MODE", "false")
	for _, managed := range []bool{true, false} {
		for _, hidden := range []bool{true, false} {
			name := map[bool]string{true: "managed", false: "unmanaged"}[managed] + "/" + map[bool]string{true: "hidden", false: "visible"}[hidden]
			t.Run(name, func(t *testing.T) {
				inst := hiddenInstance(t, hidden)
				sender := &fakeTmuxSender{}
				p := capturePaneParamsFor(inst, writerForHandler(t, sender, inst), managed)

				handleCapturePaneInput(p, []byte("rm -rf"))
				// Only the external (unmanaged) resize goes through the seam; the managed one
				// is a PTY resize on the target, which a nil writer must never reach either.
				handleCapturePaneResize(p, 200, 50)

				inputs, resizes := sender.counts()
				if hidden {
					assert.Equal(t, 0, inputs)
					assert.Equal(t, 0, resizes)
					return
				}
				assert.Equal(t, 1, inputs, "a visible attach reaches the seam, so the 0 above is not vacuous")
				if !managed {
					assert.Equal(t, 1, resizes)
				}
			})
		}
	}
}

// T-RO-01 shell row (:257, :304) and the shell resize closures.
func TestShellSite_ShouldDropInputAndResizeForHiddenAndApplyForVisible_WhenFramesDispatched(t *testing.T) {
	for _, hidden := range []bool{true, false} {
		name := map[bool]string{true: "hidden", false: "visible"}[hidden]
		t.Run(name, func(t *testing.T) {
			inst := hiddenInstance(t, hidden)
			inst.Permissions.CanSendCommand = true
			sender := &fakeTmuxSender{}
			p := shellStreamParams{
				writer:               writerForHandler(t, sender, inst),
				instance:             inst,
				shellSess:            tmux.NewTmuxSessionFromExisting("ro-shell-missing"),
				sessionID:            "ro-test",
				shellID:              "shell-1",
				shellTmuxSessionName: "ro-shell-missing",
				resizeCh:             make(chan shellResizeReq, 1),
			}

			handleShellInput(p, []byte("rm -rf"))
			dispatchShellResize(p, 200, 50)

			inputs, _ := sender.counts()
			if hidden {
				assert.Equal(t, 0, inputs)
				assert.Len(t, p.resizeCh, 0)
				return
			}
			assert.Equal(t, 1, inputs, "control mode is not running, so the visible attach falls back to the seam")
			assert.Len(t, p.resizeCh, 1)
		})
	}
}

// T-RO-02: resize paths that do not go through the frame dispatch.
func TestReadOnlyAttach_ShouldInvokeNoResizeClosure_WhenControlModeShellPTYOrHubResizePaths(t *testing.T) {
	target := &fakePanePTY{captureContent: "pane", cols: 80, rows: 24}

	t.Run("fastLaneStep refuses without a writer", func(t *testing.T) {
		for _, fastLane := range []bool{false, true} {
			step := fastLaneStep{target: target, ctx: context.Background(), fastLane: fastLane}
			assert.ErrorIs(t, step.resize(200, 50), ErrReadOnly)
		}
		assert.Equal(t, 0, target.resizePTYCalled)
	})

	t.Run("a resync request resizes nothing and still captures", func(t *testing.T) {
		req := &sessionv1.CurrentPaneRequest{TargetCols: ptr(int32(200)), TargetRows: ptr(int32(50))}
		out, err := handleCurrentPaneRequest(context.Background(), "ro-test", target, req, ResyncOptions{})
		require.NoError(t, err)
		assert.NotEmpty(t, out.Data)
		assert.Equal(t, 0, target.resizePTYCalled)
	})

	t.Run("shellPanePTY refuses without a writer", func(t *testing.T) {
		p := shellPanePTY{session: tmux.NewTmuxSessionFromExisting("ro-shell-missing")}
		assert.ErrorIs(t, p.ResizePTY(200, 50), ErrReadOnly)
		assert.ErrorIs(t, p.ResizePTYContext(context.Background(), 200, 50), ErrReadOnly)
	})

	t.Run("control-mode coalescer applies nothing", func(t *testing.T) {
		var settling atomic.Bool
		h := &ConnectRPCWebSocketHandler{}
		applied := h.applyOneControlModeResize(controlModeResizeCoalescerParams{resizeSettling: &settling}, resizeReq{200, 50})
		assert.False(t, applied)
		assert.False(t, settling.Load(), "settling is cleared on the early exit")
	})

	t.Run("initial nudge returns before touching the pane", func(t *testing.T) {
		h := &ConnectRPCWebSocketHandler{}
		cols, rows := int32(200), int32(50)
		// A nil instance would panic if the nudge went past the writer check.
		h.performInitialResizeNudge(nil, nil, "ro-test", 1, &sessionv1.CurrentPaneRequest{TargetCols: &cols, TargetRows: &rows}, nil)
	})

	t.Run("shell coalescer applies nothing", func(t *testing.T) {
		var settling atomic.Bool
		h := &ConnectRPCWebSocketHandler{}
		applied := h.applyOneShellResize(shellStreamParams{resizeSettling: &settling}, shellResizeReq{200, 50})
		assert.False(t, applied)
	})
}

// T-RO-41: scroll forwarding types PageUp into the pane, so a read-only attach never reaches it.
func TestReadOnlyAttach_ShouldNotInvokeForwardScrollAndShouldTakeTheTmuxNativeScrollbackPath_WhenAHiddenSessionStreamRequestsAppScroll(t *testing.T) {
	envtest.NewIsolatedStateDir(t)
	require.NoError(t, config.LoadConfig().SetFeatureFlag(terminalAppScrollForwardingClaudeFlagName, true))

	fallbackCalled := false
	// A nil instance panics as soon as the gate or ForwardScroll is reached, which is
	// what makes the read-only result decisive (see the existing flag-on test).
	result, err := scrollbackResultForRequest(scrollbackRequestParams{
		writer:    nil,
		instance:  nil,
		startLine: "-100",
		endLine:   "-1",
		logPrefix: "[test]",
		fallback: func(string, string) (string, error) {
			fallbackCalled = true
			return "tmux-native content", nil
		},
	})
	require.NoError(t, err)
	assert.True(t, fallbackCalled)
	assert.Nil(t, result.AppScroll)
	assert.Equal(t, "tmux-native content", result.Content)
}

// T-RO-20: a dropped frame is a debug line, never a client-visible error.
func TestDroppedFrame_ShouldNeverReachTheClient_WhenInputOrResizeDropped(t *testing.T) {
	// dispatchInputReadLoopFrame writes nothing to p.stream for a dropped frame; a nil
	// stream proves it (a write would panic).
	p := inputReadLoopParams{writer: nil}
	dispatchInputReadLoopFrame(p, inputFrame("x"))
	dispatchInputReadLoopFrame(p, resizeFrame(10, 10))
}

func ptr[T any](v T) *T { return &v }
