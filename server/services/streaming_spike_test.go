package services

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/gen/proto/go/session/v1/sessionv1connect"
)

// Story 0.1.1: abort must cancel the server context within one second, and a
// held-open stream must not starve a unary RPC on the same HTTP/1.1 origin.

type holdOpenStub struct {
	sessionv1connect.UnimplementedSessionServiceHandler
	started chan struct{}
	done    chan time.Time
	active  atomic.Int32
}

func (s *holdOpenStub) WatchSessions(ctx context.Context, _ *connect.Request[sessionv1.WatchSessionsRequest], st *connect.ServerStream[sessionv1.SessionEvent]) error {
	s.active.Add(1)
	defer s.active.Add(-1)
	if err := st.Send(&sessionv1.SessionEvent{}); err != nil {
		return err
	}
	s.started <- struct{}{}
	<-ctx.Done()
	s.done <- time.Now()
	return ctx.Err()
}

func newHoldOpenServer(t *testing.T) (*holdOpenStub, sessionv1connect.SessionServiceClient) {
	t.Helper()
	stub := &holdOpenStub{started: make(chan struct{}, 8), done: make(chan time.Time, 8)}
	_, h := sessionv1connect.NewSessionServiceHandler(stub)
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	// Browsers allow 6 HTTP/1.1 connections per origin; model that limit.
	hc := &http.Client{Transport: &http.Transport{MaxConnsPerHost: 6}}
	return stub, sessionv1connect.NewSessionServiceClient(hc, ts.URL)
}

func TestStreamingSpike_ShouldCancelServerContext_When_ClientAborts(t *testing.T) {
	stub, c := newHoldOpenServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	st, err := c.WatchSessions(ctx, connect.NewRequest(&sessionv1.WatchSessionsRequest{}))
	require.NoError(t, err)
	<-stub.started
	require.True(t, st.Receive())

	abortedAt := time.Now()
	cancel()
	select {
	case doneAt := <-stub.done:
		require.Less(t, doneAt.Sub(abortedAt), time.Second)
		t.Logf("server saw ctx.Done() %v after abort", doneAt.Sub(abortedAt))
	case <-time.After(5 * time.Second):
		t.Fatal("handler never observed ctx.Done() after the client aborted")
	}
}

func TestStreamingSpike_ShouldCompleteUnaryCall_When_TwoStreamsAreHeldOpenOnOneOrigin(t *testing.T) {
	stub, c := newHoldOpenServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for i := 0; i < 2; i++ {
		st, err := c.WatchSessions(ctx, connect.NewRequest(&sessionv1.WatchSessionsRequest{}))
		require.NoError(t, err)
		<-stub.started
		require.True(t, st.Receive())
	}
	require.EqualValues(t, 2, stub.active.Load())

	uctx, ucancel := context.WithTimeout(context.Background(), time.Second)
	defer ucancel()
	_, err := c.WriteToSession(uctx, connect.NewRequest(&sessionv1.WriteToSessionRequest{SessionId: "s", Input: "x"}))
	// The unimplemented stub answers immediately; a timeout would mean the origin was starved.
	require.Equal(t, connect.CodeUnimplemented, connect.CodeOf(err))
}
