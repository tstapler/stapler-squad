package tymux

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "github.com/tstapler/tymux/clients/go/gen/tymux/v1"

	"github.com/tstapler/stapler-squad/internal/termlat"
)

func TestCapturePane_should_RecordRenderTimeSeparatelyFromTheRPC(t *testing.T) {
	t.Parallel()
	transport := &fakeTransport{
		capturePaneFn: func(context.Context, *connect.Request[v1.CapturePaneRequest]) (*connect.Response[v1.PaneSnapshot], error) {
			return connect.NewResponse(&v1.PaneSnapshot{Grid: []*v1.Row{{Cells: []*v1.Cell{{Text: "h"}}}}, Modes: &v1.TerminalModes{}}), nil
		},
	}
	sess := NewTymuxGRPCSession(transport).(*tymuxGRPCSession)
	sess.paneID = testPaneID
	before := stageTotal(termlat.Default, termlat.StageCaptureRender)

	_, err := sess.CapturePaneContentRaw()
	require.NoError(t, err)
	_, _, _, err = sess.CapturePaneRedraw(context.Background())
	require.NoError(t, err)

	assert.GreaterOrEqual(t, stageTotal(termlat.Default, termlat.StageCaptureRender), before+2)
}

func TestCapturePane_should_RecordTheRevivePathWhenItRuns(t *testing.T) {
	t.Parallel()
	var captures int32
	transport := &fakeTransport{
		capturePaneFn: func(context.Context, *connect.Request[v1.CapturePaneRequest]) (*connect.Response[v1.PaneSnapshot], error) {
			if atomic.AddInt32(&captures, 1) == 1 {
				return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("pane is not running"))
			}
			return connect.NewResponse(activitySnapshot("hello")), nil
		},
		reviveSessionFn: func(context.Context, *connect.Request[v1.ReviveSessionRequest]) (*connect.Response[v1.ReviveSessionResponse], error) {
			return connect.NewResponse(&v1.ReviveSessionResponse{}), nil
		},
	}
	sess := NewTymuxGRPCSession(transport).(*tymuxGRPCSession)
	sess.paneID = testPaneID
	before := stageTotal(termlat.Default, termlat.StageCaptureRevive)

	_, err := sess.capturePane(context.Background(), 0)

	require.NoError(t, err)
	assert.Greater(t, stageTotal(termlat.Default, termlat.StageCaptureRevive), before)
}

func TestSendFrame_should_SplitLockWaitFromStreamSend(t *testing.T) {
	t.Parallel()
	gate := newStreamSendGate()
	stream := &fakeAttachStream{ctx: context.Background(), sendFn: func(*v1.AttachRequest) error { return nil }}
	waitBefore := stageTotal(termlat.Default, termlat.StageStreamLockWait)
	sendBefore := stageTotal(termlat.Default, termlat.StageStreamSend)

	require.NoError(t, sendFrame(context.Background(), stream, gate, &v1.AttachRequest{}, time.Second))

	assert.Greater(t, stageTotal(termlat.Default, termlat.StageStreamLockWait), waitBefore)
	assert.Greater(t, stageTotal(termlat.Default, termlat.StageStreamSend), sendBefore)
}
