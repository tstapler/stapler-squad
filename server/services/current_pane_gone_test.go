package services

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/tstapler/stapler-squad/server/protocol"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

type paneAnswer func(ctx context.Context, req *sessionv1.CurrentPaneRequest) (*sessionv1.TerminalOutput, error)

func goneAnswer(context.Context, *sessionv1.CurrentPaneRequest) (*sessionv1.TerminalOutput, error) {
	return nil, wrapSessionGone(connect.NewError(connect.CodeNotFound, errors.New("no such pane")))
}

// A confirmed-gone pane must end the stream, so the SPA reconnects once, reads not_found and
// stops, instead of re-sending resyncs into a dead pane that never answers.
func TestCurrentPaneFrame_EndsStream_OnlyWhenSessionDefinitivelyGone(t *testing.T) {
	for _, tc := range []struct {
		name     string
		answer   paneAnswer
		wantGone bool
	}{
		{"confirmed gone", goneAnswer, true},
		{"unconfirmed not_found (backend alive)", func(context.Context, *sessionv1.CurrentPaneRequest) (*sessionv1.TerminalOutput, error) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("transient"))
		}, false},
		{"daemon unavailable", func(context.Context, *sessionv1.CurrentPaneRequest) (*sessionv1.TerminalOutput, error) {
			return nil, connect.NewError(connect.CodeUnavailable, errors.New("daemon restarting"))
		}, false},
		{"internal error", func(context.Context, *sessionv1.CurrentPaneRequest) (*sessionv1.TerminalOutput, error) {
			return nil, errors.New("boom")
		}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var settling atomic.Bool
			single := handleCurrentPaneRequestFrame(nil, "s", &sessionv1.CurrentPaneRequest{ResyncId: "r"}, tc.answer, &settling)
			batch := handleBatchedCurrentPaneRequestFrame(nil, "s", &sessionv1.BatchedCurrentPaneRequest{
				Requests: []*sessionv1.CurrentPaneRequest{{ResyncId: "a"}, {ResyncId: "b"}},
			}, tc.answer, &settling)
			for _, got := range []error{single, batch} {
				if tc.wantGone {
					require.ErrorIs(t, got, errSessionGone)
					assert.Equal(t, "not_found", endStreamErrorCode(got))
				} else {
					assert.NoError(t, got)
				}
			}
			assert.False(t, settling.Load(), "resize-settling must be released on every exit")
		})
	}
}

// A gone sibling must not stop the batch from answering the others (their resync ids are waited on).
func TestBatchedCurrentPane_AnswersLiveSiblings_WhenOneIsGone(t *testing.T) {
	answer := func(ctx context.Context, req *sessionv1.CurrentPaneRequest) (*sessionv1.TerminalOutput, error) {
		if req.GetResyncId() == "dead" {
			return goneAnswer(ctx, req)
		}
		return &sessionv1.TerminalOutput{ResyncId: req.GetResyncId()}, nil
	}
	outputs, err := handleBatchedCurrentPaneRequest("s", &sessionv1.BatchedCurrentPaneRequest{
		Requests: []*sessionv1.CurrentPaneRequest{{ResyncId: "a"}, {ResyncId: "dead"}, {ResyncId: "b"}},
	}, answer)
	require.ErrorIs(t, err, errSessionGone)
	require.Len(t, outputs, 2)
	assert.Equal(t, "a", outputs[0].GetResyncId())
	assert.Equal(t, "b", outputs[1].GetResyncId())
}

func answerCurrentPaneFor(t *testing.T, pane *paneFake) error {
	t.Helper()
	inst := firstPaintInstance(t, pane)
	_, err := answerCurrentPane(context.Background(), "s", inst, &sessionv1.CurrentPaneRequest{ResyncId: "r"}, nil)
	return err
}

// Drives the wiring (answerCurrentPane -> definitivelyGone -> IsBackendProcessAlive), not
// pre-wrapped answers: only a NotFound the backend agrees is final may end the stream.
func TestAnswerCurrentPane_should_WrapGone_OnlyWhenBackendConfirmsGone(t *testing.T) {
	notFound := connect.NewError(connect.CodeNotFound, errors.New("no such pane"))
	for _, tc := range []struct {
		name     string
		err      error
		dead     bool
		wantGone bool
	}{
		{"not_found, backend dead", notFound, true, true},
		{"not_found, backend alive", notFound, false, false},
		{"unavailable, backend dead", connect.NewError(connect.CodeUnavailable, errors.New("restarting")), true, false},
		{"plain error, backend dead", errors.New("boom"), true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := answerCurrentPaneFor(t, &paneFake{redrawOK: true, redrawErr: tc.err, textErr: tc.err, dead: tc.dead})
			if tc.wantGone {
				require.ErrorIs(t, err, errSessionGone)
				return
			}
			require.Error(t, err)
			assert.NotErrorIs(t, err, errSessionGone)
		})
	}
}

// A mid-stream resync the backend confirms gone must reach errChan, so the stream ends.
func TestRunInputReadLoop_should_PropagateSessionGone_ToErrChan(t *testing.T) {
	stream, clientConn, cleanup := createTestWebSocketPair(t)
	defer cleanup()
	notFound := connect.NewError(connect.CodeNotFound, errors.New("no such pane"))
	inst := firstPaintInstance(t, &paneFake{redrawOK: true, redrawErr: notFound, textErr: notFound, dead: true})

	errChan := make(chan error, 2)
	var settling atomic.Bool
	go runInputReadLoop(inputReadLoopParams{
		stream:         stream,
		doneChan:       make(chan struct{}),
		errChan:        errChan,
		sessionID:      "s",
		resizeSettling: &settling,
		onCurrentPaneRequest: func(ctx context.Context, req *sessionv1.CurrentPaneRequest) (*sessionv1.TerminalOutput, error) {
			return answerCurrentPane(ctx, "s", inst, req, nil)
		},
	})

	td := &sessionv1.TerminalData{Data: &sessionv1.TerminalData_CurrentPaneRequest{CurrentPaneRequest: &sessionv1.CurrentPaneRequest{ResyncId: "r"}}}
	payload, err := proto.Marshal(td)
	require.NoError(t, err)
	require.NoError(t, clientConn.WriteMessage(websocket.BinaryMessage, protocol.CreateEnvelope(0, payload)))

	select {
	case got := <-errChan:
		require.ErrorIs(t, got, errSessionGone)
	case <-time.After(10 * time.Second):
		t.Fatal("errSessionGone never reached errChan")
	}
}
