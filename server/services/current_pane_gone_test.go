package services

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
